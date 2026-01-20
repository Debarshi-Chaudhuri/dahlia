package handler

import (
	"fmt"
	"net/http"

	"dahlia/commons/error_handler"
	"dahlia/commons/response"
	"dahlia/internal/logger"

	"github.com/gin-gonic/gin"
)

func ErrorHandlingMiddleware(log logger.Logger) gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		if recovered != nil {
			log.Error("panic recovered in middleware",
				logger.String("path", c.Request.URL.Path),
				logger.String("method", c.Request.Method),
				logger.Any("panic", recovered))

			standardResponse := response.StandardResponse{
				Status:    response.StatusFailed,
				ErrorCode: error_handler.CodeInternalServerError,
				Message:   "Internal server error",
				Data:      nil,
				Errors: []response.Errors{
					error_handler.GetInternalServerError("An unexpected error occurred"),
				},
			}

			c.JSON(http.StatusInternalServerError, standardResponse)
			c.Abort()
		}
	})
}

func LoggingMiddleware(log logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Info("request started",
			logger.String("method", c.Request.Method),
			logger.String("path", c.Request.URL.Path),
			logger.String("user_agent", c.GetHeader("User-Agent")),
			logger.String("remote_addr", c.ClientIP()))

		c.Next()

		log.Info("request completed",
			logger.String("method", c.Request.Method),
			logger.String("path", c.Request.URL.Path),
			logger.Int("status_code", c.Writer.Status()))
	}
}

func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Header("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, PATCH")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

func NoRouteHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		standardResponse := response.StandardResponse{
			Status:    response.StatusFailed,
			ErrorCode: error_handler.CodeNotFound,
			Message:   "Route not found",
			Data:      nil,
			Errors: []response.Errors{
				{
					ErrorCode: error_handler.CodeNotFound,
					Message:   fmt.Sprintf("The requested route '%s %s' was not found", c.Request.Method, c.Request.URL.Path),
					Data:      nil,
				},
			},
		}

		c.JSON(http.StatusNotFound, standardResponse)
	}
}

func NoMethodHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		standardResponse := response.StandardResponse{
			Status:    response.StatusFailed,
			ErrorCode: error_handler.CodeValidationError,
			Message:   "Method not allowed",
			Data:      nil,
			Errors: []response.Errors{
				{
					ErrorCode: error_handler.CodeValidationError,
					Message:   fmt.Sprintf("Method '%s' is not allowed for route '%s'", c.Request.Method, c.Request.URL.Path),
					Data:      nil,
				},
			},
		}

		c.JSON(http.StatusMethodNotAllowed, standardResponse)
	}
}