package routes

import (
	"net/http"

	"dahlia/commons/routes"
	"dahlia/internal/dto"
	"dahlia/internal/handler"
	"dahlia/internal/logger"

	"github.com/gin-gonic/gin"
)

func InitSchedulerRoutes(
	router *gin.Engine,
	schedulerHandler *handler.SchedulerHandler,
	log logger.Logger,
) {
	// Create API group
	apiV1 := routes.CreateAPIGroup(router, "v1")

	// Initialize route dependencies
	deps := routes.RouteDependencies{
		Logger: log,
	}

	// POST /api/v1/schedule - Schedule a job
	routes.RegisterRoute(
		apiV1,
		deps,
		routes.RouteOptions[dto.ScheduleJobRequest, dto.ScheduleJobResponse]{
			Path:        "/schedule",
			Method:      http.MethodPost,
			ServiceFunc: schedulerHandler.ScheduleService,
			RequireAuth: false,
		},
	)
}
