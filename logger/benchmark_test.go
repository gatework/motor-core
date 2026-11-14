package logger_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/gatework/motor-core/logger"
)

func BenchmarkDefaultLogger(b *testing.B) {
	log := logger.NewLogger(logger.WithOutput(io.Discard))
	defer log.Close()
	ctx := context.Background()
	fields := []logger.Field{logger.RequestID("req-12345"), logger.UserID(123), logger.Latency(100 * time.Millisecond)}
	b.ResetTimer()
	for b.Loop() {
		log.Log(ctx, logger.InfoLevel, "Request processed", fields...)
	}
}

func BenchmarkZapLogger(b *testing.B) {
	log, err := logger.NewZapLogger(logger.WithOutput(io.Discard), logger.WithFields(map[string]any{"app": "benchmark"}))
	if err != nil {
		b.Fatal(err)
	}
	defer log.Close()
	ctx := context.Background()
	fields := map[string]any{"request_id": "req-12345", "user_id": 123, "latency": 100 * time.Millisecond}
	b.ResetTimer()
	for b.Loop() {
		log.With(logger.FieldsFromMap(fields)...).Log(ctx, logger.InfoLevel, "Request processed")
	}
}

func BenchmarkZapLoggerStructured(b *testing.B) {
	log, err := logger.NewZapLogger(logger.WithOutput(io.Discard))
	if err != nil {
		b.Fatal(err)
	}
	defer log.Close()
	ctx := context.Background()
	fields := []logger.Field{logger.RequestID("req-12345"), logger.UserID(123), logger.Latency(100 * time.Millisecond)}
	b.ResetTimer()
	for b.Loop() {
		log.Log(ctx, logger.InfoLevel, "Request processed", fields...)
	}
}

func BenchmarkSamplingLogger(b *testing.B) {
	base, err := logger.NewZapLogger(logger.WithOutput(io.Discard))
	if err != nil {
		b.Fatal(err)
	}
	log := logger.NewSamplingLogger(base, logger.DefaultSamplingConfig)
	defer log.Close()
	ctx := context.Background()
	fields := []logger.Field{logger.RequestID("req-12345"), logger.UserID(123)}
	b.ResetTimer()
	for b.Loop() {
		log.Log(ctx, logger.InfoLevel, "Request processed", fields...)
	}
}
