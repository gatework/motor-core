package api

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/gin-gonic/gin/codec/json"
	"github.com/goccy/go-yaml"
)

func decodeRequest(c *gin.Context, dst any) error {
	value := reflect.ValueOf(dst)
	if !value.IsValid() || value.Kind() != reflect.Pointer || value.IsNil() || value.Elem().Kind() != reflect.Struct {
		return errors.New("api: binding target must be a non-nil pointer to a struct")
	}
	if err := binding.MapFormWithTag(dst, c.Request.URL.Query(), "form"); err != nil {
		return fmt.Errorf("api: decode query: %w", err)
	}
	if err := decodeBody(c, dst); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("api: decode body: %w", err)
	}
	params := make(map[string][]string, len(c.Params))
	for _, param := range c.Params {
		params[param.Key] = []string{param.Value}
	}
	if err := binding.MapFormWithTag(dst, params, "uri"); err != nil {
		return fmt.Errorf("api: decode uri: %w", err)
	}
	return nil
}

func decodeBody(c *gin.Context, dst any) error {
	r := c.Request
	if r.Body == nil || r.Body == http.NoBody || r.ContentLength == 0 {
		return nil
	}
	switch c.ContentType() {
	case binding.MIMEJSON:
		decoder := json.API.NewDecoder(r.Body)
		if binding.EnableDecoderUseNumber {
			decoder.UseNumber()
		}
		if binding.EnableDecoderDisallowUnknownFields {
			decoder.DisallowUnknownFields()
		}
		return decoder.Decode(dst)
	case binding.MIMEXML, binding.MIMEXML2:
		return xml.NewDecoder(r.Body).Decode(dst)
	case binding.MIMEYAML, "application/yaml", "text/yaml":
		return yaml.NewDecoder(r.Body).Decode(dst)
	case "", binding.MIMEPOSTForm:
		// Match net/http's default form limit without merging query parameters
		// back over the body, including on methods ParseForm does not decode.
		const maxFormBytes = 10 << 20
		data, err := io.ReadAll(io.LimitReader(r.Body, maxFormBytes+1))
		if err != nil {
			return err
		}
		if len(data) > maxFormBytes {
			return errors.New("form body exceeds 10 mib")
		}
		values, err := url.ParseQuery(string(data))
		if err != nil {
			return err
		}
		return binding.MapFormWithTag(dst, values, "form")
	default:
		return fmt.Errorf("unsupported content type %q", c.ContentType())
	}
}
