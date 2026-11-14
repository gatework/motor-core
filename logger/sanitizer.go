package logger

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
	"sync"
)

// SanitizerConfig 脱敏配置
type SanitizerConfig struct {
	// Enabled 是否启用脱敏
	Enabled bool
	// Rules 脱敏规则列表
	Rules []SanitizerRule
	// CustomMatcher 自定义字段匹配函数（可选）
	CustomMatcher func(fieldName string) *SanitizerRule
}

// SanitizerRule 脱敏规则
type SanitizerRule struct {
	// FieldPattern 字段名匹配（精确匹配或后缀匹配）
	// 例如："phone", "idcard", "*_token"（*表示后缀匹配）
	FieldPattern string
	// Strategy 脱敏策略
	// - "mask": 掩码（保留前后，中间替换为 *）
	// - "hash": SHA256 哈希
	// - "remove": 完全删除字段
	Strategy string
	// MaskChar 掩码字符（默认 "*"）
	MaskChar string
	// KeepPrefix 保留前缀长度（mask 策略）
	KeepPrefix int
	// KeepSuffix 保留后缀长度（mask 策略）
	KeepSuffix int
}

// DefaultSanitizerRules 默认脱敏规则（生产环境推荐）
var DefaultSanitizerRules = []SanitizerRule{
	// 手机号：138****5678
	{
		FieldPattern: "phone",
		Strategy:     "mask",
		MaskChar:     "*",
		KeepPrefix:   3,
		KeepSuffix:   4,
	},
	// 身份证：110***********1234
	{
		FieldPattern: "idcard",
		Strategy:     "mask",
		MaskChar:     "*",
		KeepPrefix:   3,
		KeepSuffix:   4,
	},
	{
		FieldPattern: "id_card",
		Strategy:     "mask",
		MaskChar:     "*",
		KeepPrefix:   3,
		KeepSuffix:   4,
	},
	// 密码：完全删除
	{
		FieldPattern: "password",
		Strategy:     "remove",
	},
	{
		FieldPattern: "passwd",
		Strategy:     "remove",
	},
	// Token：哈希
	{
		FieldPattern: "token",
		Strategy:     "hash",
	},
	{
		FieldPattern: "*_token", // 后缀匹配（access_token, refresh_token）
		Strategy:     "hash",
	},
	// API Key：哈希
	{
		FieldPattern: "api_key",
		Strategy:     "hash",
	},
	{
		FieldPattern: "apikey",
		Strategy:     "hash",
	},
	// 邮箱：部分掩码（zhang***@example.com）
	{
		FieldPattern: "email",
		Strategy:     "mask",
		MaskChar:     "*",
		KeepPrefix:   4,
		KeepSuffix:   12, // 保留 @example.com
	},
}

// DefaultSanitizerConfig 默认脱敏配置
var DefaultSanitizerConfig = SanitizerConfig{
	Enabled: true,
	Rules:   DefaultSanitizerRules,
}

// Sanitizer applies its rules at the single Log boundary, including fields
// supplied by With, Helper, and typed per-entry fields.
type Sanitizer struct {
	logger    Logger
	config    SanitizerConfig
	matcher   map[string]*SanitizerRule
	inherited []Field
	once      sync.Once
	closeErr  error
}

// NewSanitizerLogger constructs a sanitizing decorator with a snapshot of its rules.
func NewSanitizerLogger(log Logger, config SanitizerConfig) *Sanitizer {
	if log == nil {
		log = Default()
	}
	config.Rules = slices.Clone(config.Rules)
	matcher := make(map[string]*SanitizerRule, len(config.Rules))
	for i := range config.Rules {
		rule := &config.Rules[i]
		rule.FieldPattern = strings.ToLower(rule.FieldPattern)
		if rule.MaskChar == "" {
			rule.MaskChar = "*"
		}
		// Config.Validate rejects these at the configuration boundary. Direct
		// constructors still have to be safe for dynamically supplied rules.
		rule.KeepPrefix = max(rule.KeepPrefix, 0)
		rule.KeepSuffix = max(rule.KeepSuffix, 0)
		if !strings.HasPrefix(rule.FieldPattern, "*") {
			matcher[rule.FieldPattern] = rule
		}
	}
	return &Sanitizer{logger: log, config: config, matcher: matcher, inherited: slices.Clone(presetFields(log))}
}

// Enabled reports whether the wrapped logger accepts level.
func (s *Sanitizer) Enabled(level Level) bool { return s.logger.Enabled(level) }

// With returns a field view that borrows the logger resources.
func (s *Sanitizer) With(fields ...Field) Logger { return bind(s, fields) }

// Log records an entry with per-call context and structured fields.
func (s *Sanitizer) Log(ctx context.Context, level Level, message string, fields ...Field) {
	if !s.Enabled(level) {
		return
	}
	if s.config.Enabled {
		fields = mergeFields(s.inherited, mergeFields(contextFields(ctx), fields))
		sanitized := make([]Field, len(fields))
		for i, field := range fields {
			if rule := s.matchRule(field.Key); rule != nil {
				if value, ok := fieldValue(field).(string); ok {
					field = FieldString(field.Key, s.applySanitizer(value, rule))
				}
			}
			sanitized[i] = field
		}
		fields = sanitized
	}
	s.logger.Log(ctx, level, message, fields...)
}

// Sync waits for accepted entries and synchronizes the wrapped output.
func (s *Sanitizer) Sync() error { return syncLogger(s.logger) }

// Close completes owned work and closes the wrapped root once.
func (s *Sanitizer) Close() error {
	s.once.Do(func() { s.closeErr = closeLogger(s.logger) })
	return s.closeErr
}

func (s *Sanitizer) presetFields() []Field { return presetFields(s.logger) }

func (s *Sanitizer) matchRule(name string) *SanitizerRule {
	lowerName := strings.ToLower(name)
	if rule := s.matcher[lowerName]; rule != nil {
		return rule
	}
	if s.config.CustomMatcher != nil {
		if rule := s.config.CustomMatcher(name); rule != nil {
			return rule
		}
	}
	for i := range s.config.Rules {
		rule := &s.config.Rules[i]
		if strings.HasPrefix(rule.FieldPattern, "*") && strings.HasSuffix(lowerName, strings.TrimPrefix(rule.FieldPattern, "*")) {
			return rule
		}
	}
	return nil
}
func (s *Sanitizer) applySanitizer(value string, rule *SanitizerRule) string {
	switch rule.Strategy {
	case "mask":
		return s.maskString(value, rule)
	case "hash":
		return s.hashString(value)
	case "remove":
		return "[REDACTED]"
	default:
		return "[REDACTED]"
	}
}
func (s *Sanitizer) maskString(value string, rule *SanitizerRule) string {
	prefix := max(rule.KeepPrefix, 0)
	suffix := max(rule.KeepSuffix, 0)
	char := rule.MaskChar
	if char == "" {
		char = "*"
	}
	// Compare without adding potentially overflowing caller-supplied lengths.
	if prefix >= len(value) || suffix >= len(value)-prefix {
		return strings.Repeat(char, len(value))
	}
	return value[:prefix] + strings.Repeat(char, len(value)-prefix-suffix) + value[len(value)-suffix:]
}
func (s *Sanitizer) hashString(value string) string {
	hash := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(hash[:8])
}
