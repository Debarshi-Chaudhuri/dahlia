package routes

import (
	"net/http"

	"dahlia/commons/routes"
	"dahlia/internal/dto"
	"dahlia/internal/handler"
	"dahlia/internal/logger"

	"github.com/gin-gonic/gin"
)

func InitSignalRoutes(
	router *gin.Engine,
	signalHandler *handler.SignalHandler,
	log logger.Logger,
) {
	// Create API group
	apiV1 := routes.CreateAPIGroup(router, "v1")

	// Initialize route dependencies
	deps := routes.RouteDependencies{
		Logger: log,
	}

	// Register signal route
	routes.RegisterRoute(
		apiV1,
		deps,
		routes.RouteOptions[dto.CreateSignalRequest, dto.CreateSignalResponse]{
			Path:        "/signals",
			Method:      http.MethodPost,
			ServiceFunc: signalHandler.CreateSignalService,
			RequireAuth: false,
		},
	)
}
