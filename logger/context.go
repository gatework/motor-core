package logger

import "context"

type loggerKey struct{}

func FromContext(ctx context.Context) *Helper {
	if ctx != nil {
		if log, ok := ctx.Value(loggerKey{}).(*Helper); ok && log != nil {
			return log
		}
	}
	return NewHelper(Default())
}

func NewContext(ctx context.Context, log *Helper) context.Context {
	return context.WithValue(ctx, loggerKey{}, log)
}

func contextFields(ctx context.Context) []Field {
	if ctx == nil {
		return nil
	}
	var fields []Field
	for _, key := range [...]string{"trace_id", "user_id", "request_id"} {
		if value := ctx.Value(key); value != nil {
			fields = append(fields, Any(key, value))
		}
	}
	return fields
}
