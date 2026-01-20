package config

import (
	"dahlia/commons/routes"
	"dahlia/commons/server"
	"dahlia/internal/handler"
	"dahlia/internal/logger"
	internalRoutes "dahlia/internal/routes"

	"github.com/gin-gonic/gin"
)

// ProvideSchedulerHealthHandler creates the health handler for scheduler service
func ProvideSchedulerHealthHandler(log logger.Logger) *handler.HealthHandler {
	return handler.NewHealthHandler(log, "scheduler")
}

// ProvideSchedulerRouterConfig creates router configuration for scheduler service
func ProvideSchedulerRouterConfig(log logger.Logger) routes.RouterConfig {
	return routes.RouterConfig{
		ServiceName: "scheduler",
		Version:     "v1",
	}
}

// ProvideSchedulerServerConfig creates server configuration for scheduler service
func ProvideSchedulerServerConfig() server.ServerConfig {
	return server.ServerConfig{
		Port: "8091",
	}
}

// ProvideSchedulerRouteInitializer creates route initializer for scheduler service
func ProvideSchedulerRouteInitializer(healthHandler *handler.HealthHandler) func(*gin.Engine, routes.RouteDependencies) {
	return func(router *gin.Engine, deps routes.RouteDependencies) {
		internalRoutes.InitHealthRoutes(router, healthHandler, deps.Logger)
	}
}
