package logger

import (
	"github.com/sirupsen/logrus"
)

// piiMaskHook 对 logrus.Entry 做 PII 脱敏。
// 复用 pii_mask.go 的全局 Masker；未注册时 Fire 立即返回，0 开销。
type piiMaskHook struct{}

// Levels returns the Logrus levels handled by this hook.
func (piiMaskHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// Fire processes a Logrus entry through this hook.
func (piiMaskHook) Fire(e *logrus.Entry) error {
	m := GetMasker()
	if m == nil {
		return nil
	}
	if e.Message != "" {
		e.Message = m(e.Message)
	}
	for k, v := range e.Data {
		if s, ok := v.(string); ok && s != "" {
			e.Data[k] = m(s)
		}
	}
	return nil
}
