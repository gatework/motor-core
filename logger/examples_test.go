package logger_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/gatework/motor-core/logger"
)

func ExampleNewLogrusLogger() {
	log, err := logger.NewLogrusLogger(logger.WithLevel(logger.InfoLevel))
	if err != nil {
		panic(err)
	}
	defer log.Close()
	ctx := context.Background()
	log.Log(ctx, logger.InfoLevel, "Application started")
	log.Log(ctx, logger.WarnLevel, "This is a warning")
	log.Log(ctx, logger.InfoLevel, "User logged in", logger.Int("user_id", 123), logger.FieldString("action", "login"))
}

func ExampleNewFromConfig() {
	cfg := logger.DefaultConfig()
	cfg.Core.Name = "my-service"
	cfg.Output.File.Enabled = false
	log, err := logger.NewFromConfig(cfg)
	if err != nil {
		panic(err)
	}
	defer log.(io.Closer).Close()
	log.Log(context.Background(), logger.InfoLevel, "Logger created from config")
}

func ExampleNewDevelopmentLogger() {
	log, err := logger.NewDevelopmentLogger()
	if err != nil {
		panic(err)
	}
	defer log.Close()
	log.Log(context.Background(), logger.DebugLevel, "Debug information")
	log.Log(context.Background(), logger.InfoLevel, "Application running")
	log.Log(context.Background(), logger.ErrorLevel, "Something went wrong")
}

func ExampleNewProductionLogger() {
	log, err := logger.NewProductionLogger("logs/production.log")
	if err != nil {
		panic(err)
	}
	defer log.Close()
	ctx := context.Background()
	for range 1000 {
		log.Log(ctx, logger.InfoLevel, "Processing request")
	}
	// Error and Fatal always bypass the sampling budget.
	log.Log(ctx, logger.ErrorLevel, "Critical error occurred")
}

func ExampleNewLogrusLogger_structuredFields() {
	log, err := logger.NewLogrusLogger()
	if err != nil {
		panic(err)
	}
	defer log.Close()
	log.Log(
		context.Background(),
		logger.InfoLevel,
		"HTTP Request",
		logger.RequestID("req-12345"),
		logger.UserID(123),
		logger.Method("GET"),
		logger.URI("/api/users"),
		logger.StatusCode(200),
		logger.Latency(100*time.Millisecond),
		logger.ClientIP("192.168.1.1"),
	)
}

func ExampleNewLogrusLogger_addHook() {
	log, err := logger.NewLogrusLogger()
	if err != nil {
		panic(err)
	}
	defer log.Close()
	// Concrete adapters expose backend-specific capabilities without assertions.
	log.AddHook(logger.NewMetricsHook())
	log.Log(context.Background(), logger.ErrorLevel, "This error will trigger hooks")
}

func ExampleRegisterAdapter() {
	logger.RegisterAdapter("custom", func(opts ...logger.Option) (logger.Logger, error) {
		return logger.NewLogger(opts...), nil
	})
	cfg := logger.DefaultConfig()
	cfg.Adapter.Type = "custom"
	cfg.Output.File.Enabled = false
	log, err := logger.NewFromConfig(cfg)
	if err != nil {
		panic(err)
	}
	defer log.(io.Closer).Close()
	log.Log(context.Background(), logger.InfoLevel, "Using custom adapter")
}

func BenchmarkLogrusLogger(b *testing.B) {
	log, err := logger.NewLogrusLogger(logger.WithOutput(io.Discard), logger.WithLevel(logger.InfoLevel))
	if err != nil {
		b.Fatal(err)
	}
	defer log.Close()
	ctx := context.Background()
	b.ResetTimer()
	for b.Loop() {
		log.Log(ctx, logger.InfoLevel, "Request processed", logger.RequestID("req-12345"), logger.Int("user_id", 123))
	}
}
