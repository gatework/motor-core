package api

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/gin-gonic/gin/codec/json"
	"github.com/goccy/go-yaml"
)

type mappingKey struct {
	typ reflect.Type
	tag string
}

type mappingPlan struct {
	tagged bool
	cyclic bool
}

var mappingPlans sync.Map

func planMapping(typ reflect.Type, tag string) mappingPlan {
	key := mappingKey{typ: typ, tag: tag}
	if plan, ok := mappingPlans.Load(key); ok {
		cached, _ := plan.(mappingPlan)
		return cached
	}
	active := make(map[reflect.Type]bool)
	var walk func(reflect.Type) mappingPlan
	walk = func(t reflect.Type) mappingPlan {
		for t.Kind() == reflect.Pointer {
			if active[t] {
				return mappingPlan{cyclic: true}
			}
			active[t] = true
			defer delete(active, t)
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct {
			return mappingPlan{}
		}
		if active[t] {
			return mappingPlan{cyclic: true}
		}
		active[t] = true
		defer delete(active, t)
		var plan mappingPlan
		for field := range t.Fields() {
			if field.PkgPath != "" && !field.Anonymous {
				continue
			}
			value, tagged := field.Tag.Lookup(tag)
			if value == "-" {
				continue
			}
			child := walk(field.Type)
			plan.tagged = plan.tagged || tagged || child.tagged
			plan.cyclic = plan.cyclic || child.cyclic
		}
		return plan
	}
	plan := walk(typ)
	actual, _ := mappingPlans.LoadOrStore(key, plan)
	cached, _ := actual.(mappingPlan)
	return cached
}

func mapRequestValues(dst any, values map[string][]string, tag string) error {
	plan := planMapping(reflect.TypeOf(dst), tag)
	if !plan.tagged {
		return nil
	}
	// Gin allocates pointer children while walking struct fields, including
	// empty mappings. Reject recursive shapes before that walk can overflow.
	if plan.cyclic {
		return fmt.Errorf("recursive %s mapping is not supported", tag)
	}
	return binding.MapFormWithTag(dst, values, tag)
}

func decodeRequest(c *gin.Context, dst any) error {
	value := reflect.ValueOf(dst)
	if !value.IsValid() || value.Kind() != reflect.Pointer || value.IsNil() || value.Elem().Kind() != reflect.Struct {
		return errors.New("api: binding target must be a non-nil pointer to a struct")
	}
	if err := mapRequestValues(dst, c.Request.URL.Query(), "form"); err != nil {
		return fmt.Errorf("api: decode query: %w", err)
	}
	if err := decodeBody(c, dst); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("api: decode body: %w", err)
	}
	params := make(map[string][]string, len(c.Params))
	for _, param := range c.Params {
		params[param.Key] = []string{param.Value}
	}
	if err := mapRequestValues(dst, params, "uri"); err != nil {
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
		return mapRequestValues(dst, values, "form")
	default:
		return fmt.Errorf("unsupported content type %q", c.ContentType())
	}
}
