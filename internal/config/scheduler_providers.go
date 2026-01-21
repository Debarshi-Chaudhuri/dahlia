package config

import (
	"dahlia/commons/routes"
	"dahlia/commons/server"
	cache "dahlia/internal/cache/iface"
	"dahlia/internal/handler"
	"dahlia/internal/logger"
	"dahlia/internal/repository/dynamodb"
	repository "dahlia/internal/repository/iface"
	internalRoutes "dahlia/internal/routes"
	"dahlia/internal/service"

	awsdynamodb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/gin-gonic/gin"
)

// ProvideJobRepository provides job repository
func ProvideJobRepository(client *awsdynamodb.Client, log logger.Logger) repository.JobRepository {
	return dynamodb.NewJobRepository(client, log)
}

// ProvideScheduler provides scheduler service
func ProvideScheduler(
	cache cache.Cache,
	jobRepo repository.JobRepository,
	sqsClient *sqs.Client,
	log logger.Logger,
) *service.Scheduler {
	nodeID := "NODE1" // Can be made configurable via env var
	return service.NewScheduler(cache, jobRepo, sqsClient, nodeID, log)
}

// ProvideSchedulerHandler provides scheduler handler
func ProvideSchedulerHandler(scheduler *service.Scheduler, log logger.Logger) *handler.SchedulerHandler {
	return handler.NewSchedulerHandler(scheduler, log)
}

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
func ProvideSchedulerRouteInitializer(
	healthHandler *handler.HealthHandler,
	schedulerHandler *handler.SchedulerHandler,
) func(*gin.Engine, routes.RouteDependencies) {
	return func(router *gin.Engine, deps routes.RouteDependencies) {
		internalRoutes.InitHealthRoutes(router, healthHandler, deps.Logger)
		internalRoutes.InitSchedulerRoutes(router, schedulerHandler, deps.Logger)
	}
}
