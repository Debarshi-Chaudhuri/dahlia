package routes

import (
	"net/http"

	"dahlia/commons/routes"
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
		routes.RouteOptions[handler.ScheduleRequest, handler.ScheduleResponse]{
			Path:        "/schedule",
			Method:      http.MethodPost,
			ServiceFunc: schedulerHandler.ScheduleService,
			RequireAuth: false,
		},
	)

	// GET /api/v1/jobs/:job_id - Get job status
	routes.RegisterRoute(
		apiV1,
		deps,
		routes.RouteOptions[struct{}, handler.JobStatusResponse]{
			Path:        "/jobs/:job_id",
			Method:      http.MethodGet,
			ServiceFunc: schedulerHandler.GetJobStatusService,
			RequireAuth: false,
		},
	)
}
