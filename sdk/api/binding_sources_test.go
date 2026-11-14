package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"reflect"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

func requestAPI(t testing.TB, target, contentType, body string) *API {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPut, target, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", contentType)
	c.Params = gin.Params{{Key: "id", Value: "7"}}
	a, err := New(c, Dependencies{})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestBindDecodesEverySourceBeforeValidation(t *testing.T) {
	var request struct {
		ID   int    `uri:"id" binding:"required"`
		Name string `json:"name" xml:"name" yaml:"name" form:"name" binding:"required"`
		Page int    `form:"page" binding:"required"`
	}
	for _, tc := range []struct{ name, contentType, body string }{
		{"json", "application/json", `{"name":"body"}`},
		{"xml", "application/xml", `<request><name>body</name></request>`},
		{"yaml", "application/yaml", "name: body\n"},
		{"form", "application/x-www-form-urlencoded", "name=body"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request.ID, request.Name, request.Page = 0, "", 0
			a := requestAPI(t, "/items/7?page=2&name=query", tc.contentType, tc.body)
			if err := a.Bind(&request); err != nil {
				t.Fatal(err)
			}
			if request.ID != 7 || request.Name != "body" || request.Page != 2 {
				t.Fatalf("request=%+v", request)
			}
		})
	}
}

func TestBindURIOverridesBodyAndQuery(t *testing.T) {
	var request struct {
		ID int `uri:"id" json:"id" form:"id"`
	}
	a := requestAPI(t, "/items/7?id=1", "application/json", `{"id":3}`)
	if err := a.Bind(&request); err != nil {
		t.Fatal(err)
	}
	if request.ID != 7 {
		t.Fatalf("ID=%d, want URI ID 7", request.ID)
	}
}

type countingValidator struct {
	binding.StructValidator
	calls int
}

func (v *countingValidator) ValidateStruct(value any) error {
	v.calls++
	return v.StructValidator.ValidateStruct(value)
}
func TestBindValidatesOnce(t *testing.T) {
	original := binding.Validator
	v := &countingValidator{StructValidator: original}
	binding.Validator = v
	t.Cleanup(func() { binding.Validator = original })
	var request struct {
		ID   int    `uri:"id"`
		Name string `json:"name"`
	}
	if err := requestAPI(t, "/items/7", "application/json", `{"name":"body"}`).Bind(&request); err != nil {
		t.Fatal(err)
	}
	if v.calls != 1 {
		t.Fatalf("validator called %d times, want 1", v.calls)
	}
}

func TestBindRejectsInvalidTargetAndCanceledRequest(t *testing.T) {
	a := requestAPI(t, "/items/7", "application/json", `{}`)
	for _, value := range []any{nil, (*struct{})(nil), struct{}{}, new(int)} {
		if err := a.Bind(value); err == nil {
			t.Fatalf("accepted target %T", value)
		}
	}
	ctx, cancel := context.WithCancel(a.Context.Request.Context())
	cancel()
	a.Context.Request = a.Context.Request.WithContext(ctx)
	if err := a.Bind(&struct{}{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
}

func TestBindJSONOnlyRecursiveDTO(t *testing.T) {
	if os.Getenv("MOTOR_TEST_RECURSIVE_BIND") != "1" {
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(executable, "-test.run=^TestBindJSONOnlyRecursiveDTO$")
		cmd.Env = append(os.Environ(), "MOTOR_TEST_RECURSIVE_BIND=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("recursive DTO binding failed: %v\n%s", err, output)
		}
		return
	}
	// Bound the regression's stack growth so a broken mapper fails promptly
	// inside this subprocess instead of consuming the test runner's stack.
	debug.SetMaxStack(64 << 10)
	type node struct {
		Name string `json:"name"`
		Next *node  `json:"next"`
	}
	request := requestAPI(t, "/items/7?ignored=value", "application/json", `{"name":"first","next":{"name":"second"}}`)
	var dst node
	if err := request.Bind(&dst); err != nil {
		t.Fatal(err)
	}
	if dst.Name != "first" || dst.Next == nil || dst.Next.Name != "second" {
		t.Fatalf("decoded=%+v", dst)
	}
}

func TestBindRejectsRecursiveTaggedMapping(t *testing.T) {
	type formNode struct {
		Name string    `form:"name"`
		Next *formNode `json:"next"`
	}
	type uriNode struct {
		ID   int      `uri:"id"`
		Next *uriNode `json:"next"`
	}
	for _, dst := range []any{&formNode{}, &uriNode{}} {
		request := requestAPI(t, "/items/7", "application/json", `{}`)
		if err := request.Bind(dst); err == nil {
			t.Fatalf("accepted recursive tagged mapping %T", dst)
		}
	}
}

func TestPlanMappingRejectsPointerTypeCycle(t *testing.T) {
	type link *link
	typ := reflect.TypeOf(struct {
		Value link `form:"value"`
	}{})
	plan := planMapping(typ, "form")
	if !plan.tagged || !plan.cyclic {
		t.Fatalf("pointer cycle plan=%+v", plan)
	}
}
