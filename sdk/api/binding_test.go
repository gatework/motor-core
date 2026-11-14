package api

import "testing"

type Pagination struct {
	PageIndex int `form:"pageIndex"`
	PageSize  int `form:"pageSize"`
}

type UserOrder struct {
	Name string `form:"nameOrder"`
}

func TestBindEmbeddedFields(t *testing.T) {
	type search struct {
		Pagination
		*UserOrder
		Name string `form:"name"`
	}
	var request search
	a := requestAPI(t, "/users?pageIndex=2&pageSize=25&nameOrder=desc&name=alice", "", "")
	if err := a.Bind(&request); err != nil {
		t.Fatal(err)
	}
	if request.PageIndex != 2 || request.PageSize != 25 || request.UserOrder == nil || request.UserOrder.Name != "desc" || request.Name != "alice" {
		t.Fatalf("request=%+v", request)
	}
}
