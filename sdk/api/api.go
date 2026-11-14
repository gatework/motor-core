package api

import (
	"errors"

	vd "github.com/bytedance/go-tagexpr/v2/validator"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"gorm.io/gorm"

	"github.com/gatework/motor-core/logger"
	"github.com/gatework/motor-core/response"
	"github.com/gatework/motor-core/sdk/service"
	"github.com/gatework/motor-core/storage"
	"github.com/gatework/motor-core/tools/language"
)

var DefaultLanguage = "zh-CN"

// Dependencies are supplied by the host when it creates a request handler.
type Dependencies struct {
	DB     *gorm.DB
	Logger logger.Logger
	Cache  storage.Cache
}

// API holds only dependencies belonging to one HTTP request.
type API struct {
	Context *gin.Context
	Logger  *logger.Helper
	DB      *gorm.DB
	Cache   storage.Cache
}

func New(c *gin.Context, deps Dependencies) (*API, error) {
	if c == nil || c.Request == nil {
		return nil, errors.New("api: request context is required")
	}
	l := deps.Logger
	if l == nil {
		l = logger.Default()
	}
	a := &API{Context: c, Logger: logger.NewHelper(l), Cache: deps.Cache}
	if deps.DB != nil {
		a.DB = deps.DB.WithContext(c.Request.Context())
	}
	return a, nil
}

// Service passes request-bound dependencies to an application service.
func (e *API) Service() service.Service {
	return service.Service{DB: e.DB, Logger: e.Logger, Cache: e.Cache}
}

// validate mirrors gin's internal binding.validate. binding.Validator is a
// public, assignable variable, and setting it to nil is the documented way to
// disable validation globally, so it must not be dereferenced unchecked.
func validate(obj any) error {
	if binding.Validator == nil {
		return nil
	}
	return binding.Validator.ValidateStruct(obj)
}

// Bind decodes query parameters, one content-type-selected body, and URI
// parameters in that order, then validates the completed DTO once. Form tags
// bind query and URL-encoded values. URI values take precedence over the body.
func (e *API) Bind(d any) error {
	if e == nil || e.Context == nil || e.Context.Request == nil {
		return errors.New("api: request context is required")
	}
	ctx := e.Context.Request.Context()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := decodeRequest(e.Context, d); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validate(d); err != nil {
		return TranslateValidationErrors(err, e.getAcceptLanguage())
	}
	return vd.Validate(d)
}

// Error 通常错误数据处理
func (e *API) Error(code int, err error, msg string) {
	response.Error(e.Context, code, err, msg)
}

// OK 通常成功数据处理
func (e *API) OK(data any, msg string) {
	response.OK(e.Context, data, msg)
}

// PageOK 分页数据处理
func (e *API) PageOK(result any, count int, pageIndex int, pageSize int, msg string) {
	response.PageOK(e.Context, result, count, pageIndex, pageSize, msg)
}

// Custom 兼容函数
func (e *API) Custom(data gin.H) {
	response.Custom(e.Context, data)
}

// getAcceptLanguage reports the language the request asked for, falling back
// to DefaultLanguage.
func (e API) getAcceptLanguage() string {
	languages := language.ParseAcceptLanguage(e.Context.GetHeader("Accept-Language"), nil)
	if len(languages) == 0 {
		return DefaultLanguage
	}
	return languages[0]
}
