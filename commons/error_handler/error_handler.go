package error_handler

import (
	"dahlia/commons/response"
	"net/http"
)

type ErrorCollection struct {
	errors []response.Errors
}

func NewErrorCollection() *ErrorCollection {
	return &ErrorCollection{
		errors: make([]response.Errors, 0),
	}
}

func (ec *ErrorCollection) AddError(code int, message string, data any) *ErrorCollection {
	ec.errors = append(ec.errors, response.Errors{
		ErrorCode: code,
		Message:   message,
		Data:      data,
	})
	return ec
}

func (ec *ErrorCollection) HasErrors() bool {
	return len(ec.errors) > 0
}

func (ec *ErrorCollection) GetErrors() []response.Errors {
	return ec.errors
}

func (ec *ErrorCollection) GetHTTPStatus() int {
	if !ec.HasErrors() {
		return http.StatusOK
	}
	
	for _, err := range ec.errors {
		if err.ErrorCode >= 500 {
			return http.StatusInternalServerError
		}
		if err.ErrorCode >= 400 {
			return http.StatusBadRequest
		}
	}
	
	return http.StatusBadRequest
}

// Common error codes
const (
	CodeValidationError    = 400
	CodeNotFound          = 404
	CodeInternalServerError = 500
)

// Helper functions for common errors
func GetValidationError(message string) response.Errors {
	return response.Errors{
		ErrorCode: CodeValidationError,
		Message:   message,
		Data:      nil,
	}
}

func GetNotFoundError(message string) response.Errors {
	return response.Errors{
		ErrorCode: CodeNotFound,
		Message:   message,
		Data:      nil,
	}
}

func GetInternalServerError(message string) response.Errors {
	return response.Errors{
		ErrorCode: CodeInternalServerError,
		Message:   message,
		Data:      nil,
	}
}