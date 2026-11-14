package utils

import (
	"net/http"
	"time"
)

// APIError carries an HTTP error response envelope.
type APIError struct {
	Code      int    `json:"code"`
	Success   bool   `json:"success"`
	Msg       string `json:"msg"`
	Timestamp int64  `json:"timestamp"`
	Result    any    `json:"result"`
}

// Error returns the response message.
func (e *APIError) Error() string {
	return e.Msg
}

func newAPIException(code int, msg string, data any, success bool) *APIError {
	return &APIError{
		Code:      code,
		Success:   success,
		Msg:       msg,
		Timestamp: time.Now().Unix(),
		Result:    data,
	}
}

// ServerError returns an internal-server-error envelope.
func ServerError() *APIError {
	return newAPIException(http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError), nil, false)
}

// NotFound returns a not-found envelope.
func NotFound() *APIError {
	return newAPIException(http.StatusNotFound, http.StatusText(http.StatusNotFound), nil, false)
}

// UnknownError returns a forbidden response with the supplied message.
func UnknownError(message string) *APIError {
	return newAPIException(http.StatusForbidden, message, nil, false)
}

// ParameterError returns a bad-request response with the supplied message.
func ParameterError(message string) *APIError {
	return newAPIException(http.StatusBadRequest, message, nil, false)
}

// AuthError returns an authentication-error envelope.
func AuthError(message string) *APIError {
	return newAPIException(http.StatusBadRequest, message, nil, false)
}

// ResponseJSON returns a success-status response with the supplied payload.
func ResponseJSON(message string, data any, success bool) *APIError {
	return newAPIException(http.StatusOK, message, data, success)
}
