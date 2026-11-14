package poster_test

import (
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gatework/motor-core/tools/poster"
)

func TestGetImageRejectsUntrustedTLS(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_ = png.Encode(w, image.NewRGBA(image.Rect(0, 0, 1, 1)))
	}))
	t.Cleanup(server.Close)
	if _, err := poster.GetImage(t.Context(), server.URL); err == nil {
		t.Fatal("an untrusted image server's certificate was accepted")
	}
}
