package logger

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// ANSI 颜色常量（与 GORM 一致）
// 设计原则：深色背景优化，清晰可读，专业简洁
const (
	// 控制符
	Reset = "\033[0m"

	// 基础颜色（用于日志级别、路径等）
	Red         = "\033[31m"
	Green       = "\033[32m"
	Yellow      = "\033[33m"
	Blue        = "\033[34m" // Caller and GORM paths
	Magenta     = "\033[35m"
	Cyan        = "\033[36m"
	White       = "\033[37m"
	Gray        = "\033[90m" // Secondary details
	BrightWhite = "\033[97m" // Debug level

	// 加粗颜色（用于高优先级信息）
	BlueBold    = "\033[34;1m" // GORM row count
	MagentaBold = "\033[35;1m"
	RedBold     = "\033[31;1m" // FATAL/PANIC
	YellowBold  = "\033[33;1m"
	GreenBold   = "\033[32;1m"
	CyanBold    = "\033[36;1m"
)

// ConsoleFormatter 自定义控制台格式化器 - 简洁优雅（GORM 风格）
type ConsoleFormatter struct {
	TimestampFormat string
	ForceColors     bool
}

// Format renders an entry, reusing the buffer supplied by Logrus when present.
func (f *ConsoleFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	b := entry.Buffer
	if b == nil {
		b = &bytes.Buffer{}
	}
	timestampFormat := f.TimestampFormat
	if timestampFormat == "" {
		timestampFormat = "2006-01-02 15:04:05.000"
	}
	fmt.Fprintf(b, "%s ", entry.Time.Format(timestampFormat))
	levelText := strings.ToUpper(entry.Level.String())
	fmt.Fprintf(b, "%s%-5s%s ", getLevelColor(entry.Level), levelText, Reset)

	if isGORMMessage(entry.Message) {
		b.WriteString(formatGormLog(entry.Message))
	} else {
		writeConsoleCaller(b, entry.Data)
		b.WriteString(entry.Message)
	}
	writeConsoleFields(b, entry)
	b.WriteByte('\n')
	return b.Bytes(), nil
}

func isGORMMessage(message string) bool {
	return strings.Contains(message, "/") &&
		(strings.Contains(message, ".go:") || strings.Contains(message, "[") && strings.Contains(message, "ms]"))
}

func writeConsoleCaller(b *bytes.Buffer, fields logrus.Fields) {
	caller, ok := fields["caller"].(string)
	if !ok || caller == "" {
		return
	}
	fmt.Fprintf(b, "%s%s%s ", Blue, caller, Reset)
	if requestID, ok := fields["x-request-id"].(string); ok && requestID != "" {
		fmt.Fprintf(b, "%sreq:%s%s ", Yellow, requestID[:min(len(requestID), 8)], Reset)
	}
}

func writeConsoleFields(b *bytes.Buffer, entry *logrus.Entry) {
	var keyFields []string
	skipMethodPath := strings.Contains(entry.Message, " /")
	fieldOrder := [...]string{"status", "latency", "method", "path", "ip", "user_id", "user_name"}
	for _, key := range fieldOrder {
		if skipMethodPath && (key == "method" || key == "path") {
			continue
		}
		if value, ok := entry.Data[key]; ok {
			keyFields = append(keyFields, formatField(key, value))
		}
	}
	if len(keyFields) > 0 {
		fmt.Fprintf(b, " %s", strings.Join(keyFields, " "))
	}
	var otherFields []string
	for key, value := range entry.Data {
		if isKeyField(key) || key == "logger" || key == "caller" || key == "x-request-id" {
			continue
		}
		otherFields = append(otherFields, fmt.Sprintf("%s%s=%s%v", Gray, key, Reset, value))
	}
	if len(otherFields) > 0 {
		sort.Strings(otherFields)
		fmt.Fprintf(b, "\n      %s", strings.Join(otherFields, " "))
	}
}

// getLevelColor 获取日志级别对应的颜色（深色背景优化）
func getLevelColor(level logrus.Level) string {
	switch level {
	case logrus.DebugLevel:
		return BrightWhite // 亮白色 - 深色背景清晰
	case logrus.TraceLevel:
		return Gray // 灰色 - 最低优先级
	case logrus.InfoLevel:
		return Green // 绿色 - 正常状态
	case logrus.WarnLevel:
		return Yellow // 黄色 - 潜在问题
	case logrus.ErrorLevel:
		return Red // 红色 - 错误
	case logrus.FatalLevel, logrus.PanicLevel:
		return RedBold // 红色加粗 - 致命错误
	default:
		return Reset
	}
}

// formatField 格式化字段（GORM 风格）
func formatField(key string, val any) string {
	switch key {
	case "status":
		status := fmt.Sprintf("%v", val)
		// 状态码着色（2xx绿/4xx黄/5xx红）
		if strings.HasPrefix(status, "2") {
			return fmt.Sprintf("%s%s%s", Green, status, Reset)
		} else if strings.HasPrefix(status, "4") {
			return fmt.Sprintf("%s%s%s", Yellow, status, Reset)
		} else if strings.HasPrefix(status, "5") {
			return fmt.Sprintf("%s%s%s", Red, status, Reset)
		}
		return status
	case "latency":
		// 耗时格式化（黄色 - GORM 风格）
		if duration, ok := val.(time.Duration); ok {
			return formatDuration(duration)
		}
		return fmt.Sprintf("%s[%v]%s", Yellow, val, Reset)
	case "method":
		// HTTP 方法（不着色，保持简洁）
		return fmt.Sprintf("%v", val)
	case "path":
		// 路径（不着色）
		return fmt.Sprintf("%v", val)
	case "ip":
		// IP 地址（灰色）
		return fmt.Sprintf("%sip:%v%s", Gray, val, Reset)
	case "user_id", "user_name":
		// 用户信息（灰色）
		return fmt.Sprintf("%s%s:%v%s", Gray, key, val, Reset)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// formatDuration 格式化耗时（GORM 风格 - 黄色）
// 性能分级：< 1s 黄色（正常），>= 1s 红色（警示）
func formatDuration(d time.Duration) string {
	ms := float64(d.Nanoseconds()) / 1e6
	if ms < 1000 {
		return fmt.Sprintf("%s[%.3fms]%s", Yellow, ms, Reset)
	}
	return fmt.Sprintf("%s[%.3fs]%s", Red, ms/1000, Reset)
}

// isKeyField 判断是否为关键字段
func isKeyField(key string) bool {
	keyFields := map[string]bool{
		"status":    true,
		"latency":   true,
		"method":    true,
		"path":      true,
		"ip":        true,
		"user_id":   true,
		"user_name": true,
	}
	return keyFields[key]
}

// formatGormLog 格式化 GORM 日志（彩色分段）
func formatGormLog(msg string) string {
	// 先清除 GORM 自带的 ANSI 转义码
	msg = stripAnsiCodes(msg)

	// GORM 格式：/path/file.go:123 [12.5ms] [rows:10] SELECT ...
	// 优化为：路径(蓝色) [耗时](黄色) [rows](蓝色) SQL(白色)

	// 1. 提取文件路径（到 .go:行号）
	pathEnd := strings.Index(msg, ".go:")
	if pathEnd == -1 {
		return msg // 不是标准 GORM 格式，直接返回
	}

	// 找到行号结束位置
	lineEnd := pathEnd + 4 // ".go:"
	for lineEnd < len(msg) && msg[lineEnd] >= '0' && msg[lineEnd] <= '9' {
		lineEnd++
	}

	path := msg[:lineEnd]
	rest := msg[lineEnd:]

	// 2. 提取 [耗时]
	var latency string
	if idx := strings.Index(rest, "["); idx != -1 {
		if endIdx := strings.Index(rest[idx:], "]"); endIdx != -1 {
			latency = rest[idx : idx+endIdx+1]
			rest = rest[idx+endIdx+1:]
		}
	}

	// 3. 提取 [rows:N]
	var rows string
	if idx := strings.Index(rest, "[rows:"); idx != -1 {
		if endIdx := strings.Index(rest[idx:], "]"); endIdx != -1 {
			rows = rest[idx : idx+endIdx+1]
			rest = rest[idx+endIdx+1:]
		}
	}

	// 4. 组装彩色输出
	var result strings.Builder
	result.WriteString(Blue) // 与 GORM caller 颜色一致
	result.WriteString(path)
	result.WriteString(Reset)

	if latency != "" {
		result.WriteString(" ")
		result.WriteString(Yellow)
		result.WriteString(latency)
		result.WriteString(Reset)
	}

	if rows != "" {
		result.WriteString(" ")
		result.WriteString(BlueBold)
		result.WriteString(rows)
		result.WriteString(Reset)
	}

	if rest != "" {
		result.WriteString(rest) // SQL 语句保持原色
	}

	return result.String()
}

// stripAnsiCodes 移除字符串中的 ANSI 转义码
func stripAnsiCodes(s string) string {
	// 匹配 ANSI 转义序列：ESC [ ... m
	var result strings.Builder
	i := 0
	for i < len(s) {
		if i < len(s)-1 && s[i] == '\033' && s[i+1] == '[' {
			// 找到 'm' 结束符
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			if j < len(s) {
				i = j + 1 // 跳过整个转义序列
				continue
			}
		}
		result.WriteByte(s[i])
		i++
	}
	return result.String()
}
