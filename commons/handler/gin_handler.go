package handler

import (
	"bytes"
	"context"
	"io"
	"net/http"

	"dahlia/commons/error_handler"
	"dahlia/commons/response"
	"dahlia/internal/logger"

	"github.com/gin-gonic/gin"
)

type ServiceFunc[InputDto any, OutputDto any] func(
	ctx context.Context,
	ioutil *RequestIo[InputDto],
) (OutputDto, *error_handler.ErrorCollection)

func HandleFunc[InputDto any, OutputDto any](
	deps HandlerDependencies,
	serviceFunc ServiceFunc[InputDto, OutputDto],
) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		ioutil := BuildRequestIo[InputDto](c)

		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			deps.Logger.Error("unable to read request body", logger.Error(err))
			SendErrorResponse(c, *new(OutputDto), error_handler.NewErrorCollection().
				AddError(error_handler.CodeInternalServerError, "Unable to parse request body", nil))
			return
		}

		ioutil.RawBody = bodyBytes

		if len(bodyBytes) > 0 && (c.Request.Method == http.MethodPost || c.Request.Method == http.MethodPut || c.Request.Method == http.MethodPatch) {
			// Restore the body for ShouldBindJSON to read
			c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

			if err := c.ShouldBindJSON(&ioutil.Body); err != nil {
				deps.Logger.Error("unable to bind request body",
					logger.Error(err),
					logger.String("raw_body", string(bodyBytes)))
				SendErrorResponse(c, *new(OutputDto), error_handler.NewErrorCollection().
					AddError(error_handler.CodeValidationError, err.Error(), nil))
				return
			}
		}

		outputDto, errorCollection := serviceFunc(ctx, ioutil)

		if errorCollection != nil && errorCollection.HasErrors() {
			SendErrorResponse(c, outputDto, errorCollection)
		} else {
			SendSuccessResponse(c, outputDto)
		}
	}
}

func SendSuccessResponse[T any](c *gin.Context, data T) {
	standardResponse := response.StandardResponse{
		Status:    response.StatusSuccess,
		ErrorCode: 0,
		Message:   "Success",
		Data:      data,
		Errors:    []response.Errors{},
	}

	c.JSON(http.StatusOK, standardResponse)
}

func SendErrorResponse[T any](c *gin.Context, data T, errorCollection *error_handler.ErrorCollection) {
	status := response.StatusFailed
	httpStatus := errorCollection.GetHTTPStatus()
	errors := errorCollection.GetErrors()

	var primaryErrorCode int
	var primaryMessage string

	if len(errors) > 0 {
		primaryErrorCode = errors[0].ErrorCode
		primaryMessage = errors[0].Message
	} else {
		primaryErrorCode = error_handler.CodeInternalServerError
		primaryMessage = "Internal server error"
	}

	standardResponse := response.StandardResponse{
		Status:    status,
		ErrorCode: primaryErrorCode,
		Message:   primaryMessage,
		Data:      data,
		Errors:    errors,
	}

	c.JSON(httpStatus, standardResponse)
}
