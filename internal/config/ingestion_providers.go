package config

import (
	"dahlia/commons/routes"
	"dahlia/commons/server"
	"dahlia/internal/handler"
	"dahlia/internal/logger"
	internalRoutes "dahlia/internal/routes"

	"github.com/gin-gonic/gin"
)

// ProvideIngestionHealthHandler creates the health handler for ingestion service
func ProvideIngestionHealthHandler(log logger.Logger) *handler.HealthHandler {
	return handler.NewHealthHandler(log, "ingestion")
}

// ProvideIngestionRouterConfig creates router configuration for ingestion service
func ProvideIngestionRouterConfig(log logger.Logger) routes.RouterConfig {
	return routes.RouterConfig{
		ServiceName: "ingestion",
		Version:     "v1",
	}
}

// ProvideIngestionServerConfig creates server configuration for ingestion service
func ProvideIngestionServerConfig() server.ServerConfig {
	return server.ServerConfig{
		Port: "8090", // Temporary port for testing
	}
}

// ProvideIngestionRouteInitializer creates route initializer for ingestion service
func ProvideIngestionRouteInitializer(healthHandler *handler.HealthHandler) func(*gin.Engine, routes.RouteDependencies) {
	return func(router *gin.Engine, deps routes.RouteDependencies) {
		internalRoutes.InitHealthRoutes(router, healthHandler, deps.Logger)
	}
}
