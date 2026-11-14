package search

import (
	"testing"
	"time"
)

// ResolveSearchQuery runs on every list request: it walks the search DTO by
// reflection and turns the struct tags into WHERE clauses. The DTO shape below
// matches what go-admin generates for a CRUD module - a handful of optional
// filters plus an embedded pagination struct.
//
// The interesting case is the empty one. A list page opened without filters
// sends no search values at all, so every field is zero and the whole walk
// produces nothing. That is the common request, not the exceptional one.

type BenchPagination struct {
	PageIndex int `search:"-"`
	PageSize  int `search:"-"`
}

type benchSearchDTO struct {
	BenchPagination
	Username  string    `search:"type:contains;column:username;table:sys_user"`
	NickName  string    `search:"type:contains;column:nick_name;table:sys_user"`
	Phone     string    `search:"type:exact;column:phone;table:sys_user"`
	Email     string    `search:"type:exact;column:email;table:sys_user"`
	Status    string    `search:"type:exact;column:status;table:sys_user"`
	DeptId    string    `search:"type:exact;column:dept_id;table:sys_user"`
	RoleId    string    `search:"type:exact;column:role_id;table:sys_user"`
	BeginTime time.Time `search:"type:gte;column:created_at;table:sys_user"`
	EndTime   time.Time `search:"type:lte;column:created_at;table:sys_user"`
	Order     string    `search:"type:order;column:created_at;table:sys_user"`
	Ignored   string    `search:"-"`
}

func benchResolve(b *testing.B, q any) {
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, _ = Resolve(MySQL, q)
	}
}

// BenchmarkResolveEmpty is a list page opened with no filters.
func BenchmarkResolveEmpty(b *testing.B) {
	benchResolve(b, benchSearchDTO{PageIndex: 1, PageSize: 10})
}

// BenchmarkResolveTypical is a search with two filters filled in.
func BenchmarkResolveTypical(b *testing.B) {
	benchResolve(b, benchSearchDTO{
		PageIndex: 1, PageSize: 10,
		Username: "admin",
		Status:   "2",
	})
}

// BenchmarkResolveFull fills everything, which is the upper bound.
func BenchmarkResolveFull(b *testing.B) {
	benchResolve(b, benchSearchDTO{
		PageIndex: 1, PageSize: 10,
		Username:  "admin",
		NickName:  "administrator",
		Phone:     "13800000000",
		Email:     "a@b.c",
		Status:    "2",
		DeptId:    "1",
		RoleId:    "1",
		BeginTime: time.Now().Add(-24 * time.Hour),
		EndTime:   time.Now(),
		Order:     "desc",
	})
}
