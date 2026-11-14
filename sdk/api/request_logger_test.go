package api

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/gatework/motor-core/logger"
	"github.com/gatework/motor-core/sdk/pkg"
)

func TestRequestLoggerIsSharedWithinRequest(t *testing.T) {
	request := requestAPI(t, "/items/7", "", "")
	first := GetRequestLogger(request.Context)
	if second := GetRequestLogger(request.Context); second != first {
		t.Fatal("request logger was rebuilt")
	}
	SetRequestLogger(request.Context)
	if got := GetRequestLogger(request.Context); got != first {
		t.Fatal("middleware replaced the existing request logger")
	}
}

func TestRequestLoggerKeepsSuppliedHelper(t *testing.T) {
	request := requestAPI(t, "/items/7", "", "")
	custom := logger.NewHelper(logger.NewLogger(logger.WithOutput(io.Discard)))
	request.Context.Set(pkg.LoggerKey, custom)
	SetRequestLogger(request.Context)
	if got := GetRequestLogger(request.Context); got != custom {
		t.Fatal("middleware replaced the supplied helper")
	}
}

func TestRequestLoggerReplacesNilHelperAndIncludesRequestID(t *testing.T) {
	previous := logger.Default()
	defer logger.SetDefault(previous)
	var output bytes.Buffer
	logger.SetDefault(logger.NewLogger(logger.WithOutput(&output)))
	request := requestAPI(t, "/items/7", "", "")
	request.Context.Set(pkg.LoggerKey, (*logger.Helper)(nil))
	log := GetRequestLogger(request.Context)
	if log == nil {
		t.Fatal("returned a nil request logger")
	}
	log.Log(context.Background(), logger.InfoLevel, "entry")
	if id := pkg.RequestID(request.Context); !bytes.Contains(output.Bytes(), []byte(id)) {
		t.Fatalf("entry lost request ID: %s", output.String())
	}
}

var requestLoggerSink *logger.Helper

func BenchmarkRequestLogger(b *testing.B) {
	b.Run("first lookup", func(b *testing.B) {
		request := requestAPI(b, "/items/7", "", "")
		request.Context.Request.Header.Set(pkg.TrafficKey, "request-id")
		b.ReportAllocs()
		for b.Loop() {
			delete(request.Context.Keys, pkg.LoggerKey)
			requestLoggerSink = GetRequestLogger(request.Context)
		}
	})
	b.Run("repeated lookup", func(b *testing.B) {
		request := requestAPI(b, "/items/7", "", "")
		request.Context.Request.Header.Set(pkg.TrafficKey, "request-id")
		_ = GetRequestLogger(request.Context)
		b.ReportAllocs()
		for b.Loop() {
			requestLoggerSink = GetRequestLogger(request.Context)
		}
	})
}
