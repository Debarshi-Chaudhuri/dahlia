package handler

import (
	"dahlia/internal/logger"

	"github.com/gin-gonic/gin"
)

type RequestIo[T any] struct {
	Body       T
	RawBody    []byte
	PathParams map[string]string
	QueryParams map[string]string
	Headers    map[string]string
}

type HandlerDependencies struct {
	Logger logger.Logger
}

func BuildRequestIo[T any](c *gin.Context) *RequestIo[T] {
	return &RequestIo[T]{
		PathParams:  extractPathParams(c),
		QueryParams: extractQueryParams(c),
		Headers:     extractHeaders(c),
	}
}

func extractPathParams(c *gin.Context) map[string]string {
	params := make(map[string]string)
	for _, param := range c.Params {
		params[param.Key] = param.Value
	}
	return params
}

func extractQueryParams(c *gin.Context) map[string]string {
	params := make(map[string]string)
	for key, values := range c.Request.URL.Query() {
		if len(values) > 0 {
			params[key] = values[0]
		}
	}
	return params
}

func extractHeaders(c *gin.Context) map[string]string {
	headers := make(map[string]string)
	for key, values := range c.Request.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}
	return headers
}