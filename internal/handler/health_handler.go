package handler

import (
	"context"

	"dahlia/commons/error_handler"
	"dahlia/commons/handler"
	"dahlia/internal/logger"
)

type HealthHandler struct {
	logger      logger.Logger
	serviceName string
}

type HealthRequest struct{}

type HealthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

func NewHealthHandler(log logger.Logger, serviceName string) *HealthHandler {
	return &HealthHandler{
		logger:      log.With(logger.String("component", "health_handler")),
		serviceName: serviceName,
	}
}

func (h *HealthHandler) HealthService(
	ctx context.Context,
	ioutil *handler.RequestIo[HealthRequest],
) (HealthResponse, *error_handler.ErrorCollection) {
	h.logger.Debug("health check requested")

	response := HealthResponse{
		Status:  "healthy",
		Service: h.serviceName,
	}

	return response, nil
}
