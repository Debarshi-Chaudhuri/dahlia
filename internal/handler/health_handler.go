package handler

import (
	"context"

	"dahlia/commons/error_handler"
	"dahlia/commons/handler"
	"dahlia/internal/dto"
	"dahlia/internal/logger"
)

type HealthHandler struct {
	logger      logger.Logger
	serviceName string
}

func NewHealthHandler(log logger.Logger, serviceName string) *HealthHandler {
	return &HealthHandler{
		logger:      log.With(logger.String("component", "health_handler")),
		serviceName: serviceName,
	}
}

func (h *HealthHandler) HealthService(
	ctx context.Context,
	ioutil *handler.RequestIo[dto.HealthCheckRequest],
) (dto.HealthCheckResponse, *error_handler.ErrorCollection) {
	h.logger.Debug("health check requested")

	response := dto.HealthCheckResponse{
		Status:  "healthy",
		Service: h.serviceName,
	}

	return response, nil
}
