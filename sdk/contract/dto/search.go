package dto

import (
	"gorm.io/gorm"

	"github.com/gatework/motor-core/tools/search"
)

// GeneralDelDto binds either a single :id or a JSON body of ids for a batch
// delete, matching what ObjectByID does for the request shapes that predate
// it.
//
// Deprecated: prefer ObjectByID (single-or-batch) or ObjectDeleteReq
// (batch only). GeneralDelDto binds through both a `uri:"id"` and a
// `json:"id"` tag with `validate:"required"`, and relies on the caller's own
// binder plus that tag to reject a missing id; the newer types bind through
// `uri` alone and implement their own Bind method. GeneralDelDto is not
// being removed - the host has callers depending on this exact shape - but
// new code should not add to them.
type GeneralDelDto struct {
	ID  int   `uri:"id" json:"id" validate:"required"`
	IDs []int `json:"ids"`
}

func (g GeneralDelDto) GetIDs() []int {
	ids := make([]int, 0)
	// ID used to be appended again inside an else branch below: passing only
	// ID produced [5 5], and the same row got deleted twice.
	if g.ID > 0 {
		ids = append(ids, g.ID)
	}
	for _, id := range g.IDs {
		if id > 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		// The convention for "nothing selected": delete everything.
		ids = append(ids, 0)
	}
	return ids
}

// GeneralGetDto binds a single :id for a detail lookup.
//
// Deprecated: prefer ObjectGetReq, which binds the same :id through `uri`
// alone and implements its own Bind method instead of relying on the
// caller's own binder plus a `validate:"required"` tag.
type GeneralGetDto struct {
	ID int `uri:"id" json:"id" validate:"required"`
}

// MakeCondition turns q's `search` struct tags into a GORM scope.
//
// The dialect it resolves tags against comes from db.Dialector.Name() inside
// the returned closure, not from a package-level variable: the closure
// already holds the *gorm.DB for this call, and db.Dialector.Name() reports
// exactly the dialect that db is bound to ("mysql", "postgres", "sqlite"),
// which is correct even when a multi-tenant host has more than one database
// open with different drivers. A host-level driver variable can only ever
// hold one of them.
func MakeCondition(q any) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		condition, err := search.Resolve(db.Name(), q)
		if err != nil {
			db.AddError(err)
			return db
		}
		apply := func(c search.Condition) {
			for _, v := range c.Where {
				db = db.Where(v.SQL, v.Args...)
			}
			for _, v := range c.Or {
				db = db.Or(v.SQL, v.Args...)
			}
			for _, v := range c.Order {
				db = db.Order(v)
			}
		}
		for _, join := range condition.Join {
			db = db.Joins(join.SQL)
			apply(join.Condition)
		}
		apply(condition)
		return db
	}
}

// Paginate is a GORM scope applying offset/limit for the given page.
func Paginate(pageSize, pageIndex int) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		offset := max((pageIndex-1)*pageSize, 0)
		return db.Offset(offset).Limit(pageSize)
	}
}
