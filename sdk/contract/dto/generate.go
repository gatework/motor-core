package dto

import (
	"net/http"

	vd "github.com/bytedance/go-tagexpr/v2/validator"
	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/sdk/api"
)

// ObjectByID binds either a single :id from the URI or, on DELETE, a JSON
// body carrying ids for a batch delete.
type ObjectByID struct {
	ID  int   `uri:"id"`
	IDs []int `json:"ids"`
}

func (s *ObjectByID) Bind(ctx *gin.Context) error {
	var err error
	log := api.GetRequestLogger(ctx)
	err = ctx.ShouldBindUri(s)
	if err != nil {
		log.Warnf("ShouldBindUri error: %s", err.Error())
		return err
	}
	if ctx.Request.Method == http.MethodDelete {
		err = ctx.ShouldBind(s)
		if err != nil {
			log.Warnf("ShouldBind error: %s", err.Error())
			return err
		}
		if len(s.IDs) > 0 {
			return nil
		}
		if s.IDs == nil {
			s.IDs = make([]int, 0)
		}
		if s.ID != 0 {
			s.IDs = append(s.IDs, s.ID)
		}
	}
	if err = vd.Validate(s); err != nil {
		log.Errorf("Validate error: %s", err.Error())
		return err
	}
	return err
}

// GetID returns the ids this request addresses: the batch from the body when
// there is one, otherwise the single id from the URI.
//
// It must not write back to the receiver. Appending to s.IDs in place made
// GetID non-idempotent - a second call returned the URI id twice - and an
// unconditional append put a 0 into the slice on any route that carries no
// :id, which is a dangerous value to hand to a caller that treats 0 as a
// sentinel.
func (s *ObjectByID) GetID() interface{} {
	if len(s.IDs) == 0 {
		return s.ID
	}
	if s.ID == 0 {
		return s.IDs
	}
	ids := make([]int, 0, len(s.IDs)+1)
	ids = append(ids, s.IDs...)
	return append(ids, s.ID)
}

// ObjectGetReq binds a single :id from the URI for a detail lookup.
type ObjectGetReq struct {
	ID int `uri:"id"`
}

func (s *ObjectGetReq) Bind(ctx *gin.Context) error {
	var err error
	log := api.GetRequestLogger(ctx)
	err = ctx.ShouldBindUri(s)
	if err != nil {
		log.Warnf("ShouldBindUri error: %s", err.Error())
		return err
	}
	if err = vd.Validate(s); err != nil {
		log.Errorf("Validate error: %s", err.Error())
		return err
	}
	return err
}

func (s *ObjectGetReq) GetID() interface{} {
	return s.ID
}

// ObjectDeleteReq binds a JSON body of ids for a batch delete.
type ObjectDeleteReq struct {
	IDs []int `json:"ids"`
}

func (s *ObjectDeleteReq) Bind(ctx *gin.Context) error {
	var err error
	log := api.GetRequestLogger(ctx)
	err = ctx.ShouldBind(s)
	if err != nil {
		log.Warnf("ShouldBind error: %s", err.Error())
		return err
	}
	if len(s.IDs) > 0 {
		return nil
	}
	if s.IDs == nil {
		s.IDs = make([]int, 0)
	}

	if err = vd.Validate(s); err != nil {
		log.Errorf("Validate error: %s", err.Error())
		return err
	}
	return err
}

func (s *ObjectDeleteReq) GetID() interface{} {
	return s.IDs
}
