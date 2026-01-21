package routes

import (
	"net/http"

	"dahlia/commons/routes"
	"dahlia/internal/dto"
	"dahlia/internal/handler"
	"dahlia/internal/logger"

	"github.com/gin-gonic/gin"
)

func InitWorkflowRoutes(
	router *gin.Engine,
	workflowHandler *handler.WorkflowHandler,
	log logger.Logger,
) {
	// Create API group
	apiV1 := routes.CreateAPIGroup(router, "v1")

	// Initialize route dependencies
	deps := routes.RouteDependencies{
		Logger: log,
	}

	// Register create workflow route
	routes.RegisterRoute(
		apiV1,
		deps,
		routes.RouteOptions[dto.CreateWorkflowRequest, dto.CreateWorkflowResponse]{
			Path:        "/workflows",
			Method:      http.MethodPost,
			ServiceFunc: workflowHandler.CreateWorkflowService,
			RequireAuth: false,
		},
	)

	// Register get workflow route
	routes.RegisterRoute(
		apiV1,
		deps,
		routes.RouteOptions[dto.GetWorkflowRequest, dto.GetWorkflowResponse]{
			Path:        "/workflows/:id",
			Method:      http.MethodGet,
			ServiceFunc: workflowHandler.GetWorkflowService,
			RequireAuth: false,
		},
	)

	// Register list workflows route
	routes.RegisterRoute(
		apiV1,
		deps,
		routes.RouteOptions[dto.ListWorkflowsRequest, dto.ListWorkflowsResponse]{
			Path:        "/workflows",
			Method:      http.MethodGet,
			ServiceFunc: workflowHandler.ListWorkflowsService,
			RequireAuth: false,
		},
	)
}
