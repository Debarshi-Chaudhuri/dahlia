package config

import (
	"context"

	"dahlia/commons/routes"
	"dahlia/commons/server"
	cache "dahlia/internal/cache/iface"
	executor "dahlia/internal/consumer/executor_queue/iface"
	executorImpl "dahlia/internal/consumer/executor_queue/impl"
	coordinator "dahlia/internal/coordinator/iface"
	"dahlia/internal/handler"
	"dahlia/internal/logger"
	queue "dahlia/internal/queue/iface"
	"dahlia/internal/queue/sqs"
	"dahlia/internal/repository/dynamodb"
	repository "dahlia/internal/repository/iface"
	internalRoutes "dahlia/internal/routes"
	"dahlia/internal/service"
	"dahlia/internal/slack"

	awsdynamodb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/gin-gonic/gin"
	"go.uber.org/fx"
)

// Repository Providers

func ProvideSignalRepository(client *awsdynamodb.Client, log logger.Logger) repository.SignalRepository {
	return dynamodb.NewSignalRepository(client, log)
}

func ProvideWorkflowRepository(client *awsdynamodb.Client, log logger.Logger) repository.WorkflowRepository {
	return dynamodb.NewWorkflowRepository(client, log)
}

func ProvideRunRepository(client *awsdynamodb.Client, log logger.Logger) repository.RunRepository {
	return dynamodb.NewRunRepository(client, log)
}

func ProvideActionLogRepository(client *awsdynamodb.Client, log logger.Logger) repository.ActionLogRepository {
	return dynamodb.NewActionLogRepository(client, log)
}

// Service Providers

func ProvideIngestionScheduler(
	cache cache.Cache,
	jobRepo repository.JobRepository,
	sqsClient *awssqs.Client,
	log logger.Logger,
) service.IScheduler {
	nodeID := "INGESTION-NODE-1" // Can be made configurable
	return service.NewScheduler(cache, jobRepo, sqsClient, nodeID, log)
}

func ProvideConditionEvaluator(signalRepo repository.SignalRepository, log logger.Logger) service.ConditionEvaluator {
	return service.NewConditionEvaluator(signalRepo, log)
}

func ProvideWorkflowManager(
	workflowRepo repository.WorkflowRepository,
	coordinator coordinator.Coordinator,
	log logger.Logger,
) service.WorkflowManager {
	return service.NewWorkflowManager(workflowRepo, coordinator, log)
}

func ProvideActionExecutor(
	slackClient slack.Client,
	scheduler service.IScheduler,
	log logger.Logger,
) service.ActionExecutor {
	return service.NewActionExecutor(slackClient, scheduler, log)
}

func ProvideWorkflowExecutor(
	signalRepo repository.SignalRepository,
	workflowRepo repository.WorkflowRepository,
	runRepo repository.RunRepository,
	actionLogRepo repository.ActionLogRepository,
	workflowManager service.WorkflowManager,
	conditionEval service.ConditionEvaluator,
	actionExecutor service.ActionExecutor,
	log logger.Logger,
) service.WorkflowExecutor {
	return service.NewWorkflowExecutor(
		signalRepo,
		workflowRepo,
		runRepo,
		actionLogRepo,
		workflowManager,
		conditionEval,
		actionExecutor,
		log,
	)
}

// Executor Queue Providers

type ExecutorQueueResult struct {
	fx.Out
	Queue    queue.Queue
	Consumer executor.ExecutorConsumer
}

func ProvideExecutorQueueAndConsumer(
	sqsClient *awssqs.Client,
	workflowExec service.WorkflowExecutor,
	signalRepo repository.SignalRepository,
	log logger.Logger,
) ExecutorQueueResult {
	// Use a placeholder consumer function that will be set after creation
	var consumer executor.ExecutorConsumer

	// Create the queue with a processor that calls the consumer
	q := sqs.NewSQSQueue(
		sqsClient,
		sqs.QueueConfig{
			QueueURL:        "http://localhost:4566/000000000000/executor-queue",
			WorkerCount:     1,
			MaxMessages:     1,
			WaitTimeSeconds: 20,
		},
		queue.MessageProcessorFunc[executor.ExecutorMessage](func(ctx context.Context, msg executor.ExecutorMessage) bool {
			return consumer.ProcessMessage(ctx, msg)
		}),
		log,
	)

	// Create the consumer with the queue and signal repository
	consumer = executorImpl.NewExecutorConsumer(log, q, workflowExec, signalRepo)

	return ExecutorQueueResult{
		Queue:    q,
		Consumer: consumer,
	}
}

// HTTP Providers

func ProvideIngestionHealthHandler(log logger.Logger) *handler.HealthHandler {
	return handler.NewHealthHandler(log, "ingestion")
}

func ProvideSignalHandler(
	log logger.Logger,
	signalRepo repository.SignalRepository,
	workflowMgr service.WorkflowManager,
	executorConsumer executor.ExecutorConsumer,
) *handler.SignalHandler {
	return handler.NewSignalHandler(log, signalRepo, workflowMgr, executorConsumer)
}

func ProvideWorkflowHandler(
	log logger.Logger,
	workflowRepo repository.WorkflowRepository,
	zkCoord coordinator.Coordinator,
) *handler.WorkflowHandler {
	return handler.NewWorkflowHandler(log, workflowRepo, zkCoord)
}

func ProvideRunHandler(
	log logger.Logger,
	runRepo repository.RunRepository,
) *handler.RunHandler {
	return handler.NewRunHandler(log, runRepo)
}

func ProvideIngestionRouterConfig(log logger.Logger) routes.RouterConfig {
	return routes.RouterConfig{
		ServiceName: "ingestion",
		Version:     "v1",
	}
}

func ProvideIngestionServerConfig() server.ServerConfig {
	return server.ServerConfig{
		Port: "8090",
	}
}

func ProvideIngestionRouteInitializer(
	healthHandler *handler.HealthHandler,
	signalHandler *handler.SignalHandler,
	workflowHandler *handler.WorkflowHandler,
	runHandler *handler.RunHandler,
) func(*gin.Engine, routes.RouteDependencies) {
	return func(router *gin.Engine, deps routes.RouteDependencies) {
		internalRoutes.InitHealthRoutes(router, healthHandler, deps.Logger)
		internalRoutes.InitSignalRoutes(router, signalHandler, deps.Logger)
		internalRoutes.InitWorkflowRoutes(router, workflowHandler, deps.Logger)
		internalRoutes.InitRunRoutes(router, runHandler, deps.Logger)
	}
}

// Lifecycle Management
func ManageExecutorQueueLifecycle(lc fx.Lifecycle, q queue.Queue, srv *server.HTTPServer, log logger.Logger) {
	// The HTTP server's lifecycle hooks are automatically managed by Uber FX
	// when it's passed as a parameter here. We just need to ensure it's in the dependency graph.
	_ = srv // Explicitly reference to ensure it's invoked

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			log.Info("starting executor queue consumer")
			return q.StartConsumer(ctx)
		},
		OnStop: func(ctx context.Context) error {
			log.Info("stopping executor queue consumer")
			return q.StopConsumer(ctx)
		},
	})
}

func ManageWorkflowManagerLifecycle(lc fx.Lifecycle, workflowMgr service.WorkflowManager, log logger.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			log.Info("starting workflow manager")
			return workflowMgr.Start(ctx)
		},
		OnStop: func(ctx context.Context) error {
			log.Info("stopping workflow manager")
			return workflowMgr.Stop(ctx)
		},
	})
}

func ManageIngestionSchedulerLifecycle(lc fx.Lifecycle, scheduler service.IScheduler, srv *server.HTTPServer, log logger.Logger) {
	// The HTTP server's lifecycle hooks are automatically managed by Uber FX
	// when it's passed as a parameter here. We just need to ensure it's in the dependency graph.
	_ = srv // Explicitly reference to ensure it's invoked

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			log.Info("starting ingestion scheduler cron jobs")
			return scheduler.Start(ctx)
		},
		OnStop: func(ctx context.Context) error {
			log.Info("stopping ingestion scheduler")
			return scheduler.Stop(ctx)
		},
	})
}
