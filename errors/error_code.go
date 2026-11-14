package errors

import "net/http"

//go:generate stringer -type ErrorCode -output error_code_string.go

// ErrorCode identifies the HTTP status carried by a public error envelope.
type ErrorCode int32

// HTTP response codes supported by the public error envelope.
const (
	OK                  ErrorCode = http.StatusOK
	BadRequest          ErrorCode = http.StatusBadRequest
	Unauthorized        ErrorCode = http.StatusUnauthorized
	Forbidden           ErrorCode = http.StatusForbidden
	NotFound            ErrorCode = http.StatusNotFound
	MethodNotAllowed    ErrorCode = http.StatusMethodNotAllowed
	Timeout             ErrorCode = http.StatusRequestTimeout
	Conflict            ErrorCode = http.StatusConflict
	InternalServerError ErrorCode = http.StatusInternalServerError
)

// Code returns the numeric HTTP status represented by this error code.
func (e ErrorCode) Code() int32 {
	return int32(e)
}
