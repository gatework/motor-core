package pkg

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Two helpers run on essentially every request. RequestID gives
// the request its correlation id, and GetORM is how handlers and services reach
// the database - go-admin calls it from fourteen places, several of them more
// than once per request.

func benchContext(withHeader bool) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	if withHeader {
		c.Request.Header.Set(TrafficKey, "11111111-2222-3333-4444-555555555555")
	}
	return c
}

// BenchmarkRequestIDNew measures a request with no cached correlation ID.
func BenchmarkRequestIDNew(b *testing.B) {
	c := benchContext(false)
	b.ReportAllocs()
	for b.Loop() {
		c.Keys = nil
		_ = RequestID(c)
	}
}

// BenchmarkRequestIDExisting is every later call, and the case
// where a gateway already supplied the id: a header read and nothing else.
func BenchmarkRequestIDExisting(b *testing.B) {
	c := benchContext(true)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = RequestID(c)
	}
}

func BenchmarkGetORM(b *testing.B) {
	c := benchContext(true)
	c.Set("db", new(gorm.DB))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := GetORM(c); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGetORMMissing is the error path. It matters because the handlers
// call GetORM before doing anything else, so a misconfigured request pays this
// on every route.
func BenchmarkGetORMMissing(b *testing.B) {
	c := benchContext(true)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := GetORM(c); err == nil {
			b.Fatal("expected an error")
		}
	}
}
