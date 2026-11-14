package dto

import (
	"bytes"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
)

func deleteCtx(t *testing.T, uriID, body string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("DELETE", "/x/"+uriID, bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: uriID}}
	return c
}

// Binding through &s handed gin a **ObjectByID. A body of `null` then set
// that inner pointer to nil, and gin's validator panicked reaching through
// it - a request body a client fully controls taking the handler down.
func TestObjectByIDBindSurvivesANullBody(t *testing.T) {
	s := &ObjectByID{}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("a null body panicked: %v", r)
		}
	}()
	_ = s.Bind(deleteCtx(t, "1", "null"))
}

func TestObjectDeleteReqBindSurvivesANullBody(t *testing.T) {
	s := &ObjectDeleteReq{}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("a null body panicked: %v", r)
		}
	}()
	_ = s.Bind(deleteCtx(t, "1", "null"))
}

func TestObjectByIDBindKeepsBothTheUriIdAndTheBodyIds(t *testing.T) {
	s := &ObjectByID{}
	if err := s.Bind(deleteCtx(t, "7", `{"ids":[2,3]}`)); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if s.ID != 7 {
		t.Fatalf("ID = %d, want the id bound from the URI (7)", s.ID)
	}
	if !reflect.DeepEqual(s.IDs, []int{2, 3}) {
		t.Fatalf("IDs = %v, want [2 3]", s.IDs)
	}
}

func TestObjectByIDBindKeepsTheURISeparateFromTheBatch(t *testing.T) {
	s := &ObjectByID{}
	if err := s.Bind(deleteCtx(t, "7", `{}`)); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if len(s.IDs) != 0 || s.GetID() != 7 {
		t.Fatalf("IDs = %v, selected ID = %v; want URI ID 7 only", s.IDs, s.GetID())
	}
}

// GetID used to append to s.IDs in place, so calling it twice returned the
// URI id twice - and every caller that reads a request twice, or retries,
// widened the delete each time.
func TestGetIDIsIdempotent(t *testing.T) {
	s := &ObjectByID{ID: 7, IDs: []int{2, 3}}
	first := s.GetID()
	second := s.GetID()
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("GetID returned %v then %v", first, second)
	}
	if !reflect.DeepEqual(s.IDs, []int{2, 3}) {
		t.Fatalf("GetID wrote back to the receiver: IDs = %v, want [2 3]", s.IDs)
	}
}

// A route with no :id leaves ID at 0. Appending it put a 0 in the slice,
// which a caller treating 0 as "unset" can read as something else entirely.
func TestGetIDOmitsAZeroUriId(t *testing.T) {
	s := &ObjectByID{ID: 0, IDs: []int{2, 3}}
	got := s.GetID()
	if !reflect.DeepEqual(got, []int{2, 3}) {
		t.Fatalf("GetID = %v, want [2 3]", got)
	}
}

func TestGetIDReturnsTheSingleIdWhenTheBodyCarriesNone(t *testing.T) {
	s := &ObjectByID{ID: 7}
	if got := s.GetID(); got != 7 {
		t.Fatalf("GetID = %v, want 7", got)
	}
}

func TestObjectByIDBindAllowsAnEmptyOptionalBody(t *testing.T) {
	request := &ObjectByID{}
	if err := request.Bind(deleteCtx(t, "7", "")); err != nil {
		t.Fatal(err)
	}
	if request.GetID() != 7 {
		t.Fatalf("selected ID=%v", request.GetID())
	}
}
func TestSelectedIDsDoNotAliasTheRequest(t *testing.T) {
	requests := []struct {
		name     string
		ids      []int
		selected func() any
	}{
		{name: "single or batch"}, {name: "batch only"},
	}
	first := &ObjectByID{IDs: []int{2, 3}}
	second := &ObjectDeleteReq{IDs: []int{2, 3}}
	requests[0].ids, requests[0].selected = first.IDs, first.GetID
	requests[1].ids, requests[1].selected = second.IDs, second.GetID
	for _, request := range requests {
		t.Run(request.name, func(t *testing.T) {
			selected := request.selected().([]int)
			selected[0] = 99
			if request.ids[0] != 2 {
				t.Fatalf("selected IDs changed request: %v", request.ids)
			}
		})
	}
}
