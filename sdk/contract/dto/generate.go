package dto

import (
	"slices"

	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/sdk/api"
)

// ObjectByID addresses a single URI ID and an optional JSON batch.
type ObjectByID struct {
	ID  int   `uri:"id"`
	IDs []int `json:"ids"`
}

// Bind decodes and validates the request into the receiver.
func (s *ObjectByID) Bind(ctx *gin.Context) error {
	request, err := api.New(ctx, api.Dependencies{})
	if err != nil {
		return err
	}
	return request.Bind(s)
}

// GetID returns the selected resource identity.
func (s *ObjectByID) GetID() any {
	if len(s.IDs) == 0 {
		return s.ID
	}
	if s.ID == 0 {
		return slices.Clone(s.IDs)
	}
	ids := make([]int, 0, len(s.IDs)+1)
	ids = append(ids, s.IDs...)
	return append(ids, s.ID)
}

// ObjectGetReq addresses a single URI ID.
type ObjectGetReq struct {
	ID int `uri:"id"`
}

// Bind decodes and validates the URI parameters.
func (s *ObjectGetReq) Bind(ctx *gin.Context) error {
	request, err := api.New(ctx, api.Dependencies{})
	if err != nil {
		return err
	}
	return request.Bind(s)
}

// GetID returns the resource identity.
func (s *ObjectGetReq) GetID() any {
	return s.ID
}

// ObjectDeleteReq addresses a JSON batch of IDs.
type ObjectDeleteReq struct {
	IDs []int `json:"ids"`
}

// Bind decodes and validates the deletion request body.
func (s *ObjectDeleteReq) Bind(ctx *gin.Context) error {
	request, err := api.New(ctx, api.Dependencies{})
	if err != nil {
		return err
	}
	return request.Bind(s)
}

// GetID returns a copy of the selected resource identities.
func (s *ObjectDeleteReq) GetID() any {
	return slices.Clone(s.IDs)
}
