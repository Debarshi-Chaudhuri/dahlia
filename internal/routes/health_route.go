package routes

import (
	"net/http"

	"dahlia/commons/routes"
	"dahlia/internal/handler"
	"dahlia/internal/logger"

	"github.com/gin-gonic/gin"
)

func InitHealthRoutes(
	router *gin.Engine,
	healthHandler *handler.HealthHandler,
	log logger.Logger,
) {
	// Create API group
	apiV1 := routes.CreateAPIGroup(router, "v1")

	// Initialize route dependencies
	deps := routes.RouteDependencies{
		Logger: log,
	}

	// Register health route using the generic route registration
	routes.RegisterRoute(
		apiV1,
		deps,
		routes.RouteOptions[handler.HealthRequest, handler.HealthResponse]{
			Path:        "/health",
			Method:      http.MethodGet,
			ServiceFunc: healthHandler.HealthService,
			RequireAuth: false,
		},
	)
}
