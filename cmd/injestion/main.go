package main

import (
	"dahlia/commons/config"
	"dahlia/commons/server"
	internalConfig "dahlia/internal/config"

	"go.uber.org/fx"
)

func main() {
	fx.New(
		fx.WithLogger(config.ProvideFxLogger),
		fx.Provide(
			// Infrastructure
			config.ProvideLogger,
			config.ProvideRouteDependencies,
			config.ProvideSQSClient,
			config.ProvideDynamoDBClient,
			config.ProvideSlackClient,
			config.ProvideZooKeeperCoordinator,
			config.ProvideRedisCache,

			// Repositories
			internalConfig.ProvideSignalRepository,
			internalConfig.ProvideWorkflowRepository,
			internalConfig.ProvideRunRepository,
			internalConfig.ProvideActionLogRepository,
			internalConfig.ProvideJobRepository,

			// Services
			internalConfig.ProvideIngestionScheduler,
			internalConfig.ProvideConditionEvaluator,
			internalConfig.ProvideWorkflowManager,
			internalConfig.ProvideActionExecutor,
			internalConfig.ProvideWorkflowExecutor,

			// Executor Queue
			internalConfig.ProvideExecutorQueueAndConsumer,

			// HTTP
			internalConfig.ProvideIngestionHealthHandler,
			internalConfig.ProvideIngestionRouterConfig,
			internalConfig.ProvideIngestionServerConfig,
			internalConfig.ProvideIngestionRouteInitializer,
			config.ProvideRouter,
			server.NewHTTPServer,
		),
		fx.Invoke(
			internalConfig.ManageExecutorQueueLifecycle,
		),
	).Run()
}
