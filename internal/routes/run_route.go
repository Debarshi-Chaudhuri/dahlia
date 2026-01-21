package routes

import (
	"net/http"

	"dahlia/commons/routes"
	"dahlia/internal/dto"
	"dahlia/internal/handler"
	"dahlia/internal/logger"

	"github.com/gin-gonic/gin"
)

func InitRunRoutes(
	router *gin.Engine,
	runHandler *handler.RunHandler,
	log logger.Logger,
) {
	// Create API group
	apiV1 := routes.CreateAPIGroup(router, "v1")

	// Initialize route dependencies
	deps := routes.RouteDependencies{
		Logger: log,
	}

	// Register list runs route
	routes.RegisterRoute(
		apiV1,
		deps,
		routes.RouteOptions[dto.ListRunsRequest, dto.ListRunsResponse]{
			Path:        "/runs",
			Method:      http.MethodGet,
			ServiceFunc: runHandler.ListRunsService,
			RequireAuth: false,
		},
	)

	// Register get run route
	routes.RegisterRoute(
		apiV1,
		deps,
		routes.RouteOptions[dto.GetRunRequest, dto.GetRunResponse]{
			Path:        "/runs/:id",
			Method:      http.MethodGet,
			ServiceFunc: runHandler.GetRunService,
			RequireAuth: false,
		},
	)
}
