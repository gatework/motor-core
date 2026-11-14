package poster

import (
	"context"
	"fmt"
	"image"
	"net/http"
	"os"
	"strings"
	"time"
)

// GetImage decodes an image from a local path or an HTTP URL. Remote requests
// verify TLS certificates and honour ctx and a bounded client timeout.
func GetImage(ctx context.Context, src string) (image.Image, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		return remoteImage(ctx, src)
	}
	file, err := os.Open(src)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	img, _, err := image.Decode(file)
	return img, err
}

func remoteImage(ctx context.Context, src string) (image.Image, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 30 * time.Second}
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("poster: image response status %d", response.StatusCode)
	}
	// Decoders consume a stream; no second full copy of an image is retained.
	img, _, err := image.Decode(response.Body)
	return img, err
}
