package main

import (
	"dahlia/commons/config"
	"dahlia/commons/server"
	internalConfig "dahlia/internal/config"
	executor_init "dahlia/internal/consumer/executor_queue/init"

	"go.uber.org/fx"
)

func main() {
	fx.New(
		fx.WithLogger(config.ProvideFxLogger),
		fx.Provide(
			config.ProvideLogger,
			config.ProvideRouteDependencies,
			config.ProvideSQSClient,
			config.ProvideSlackClient,
			config.ProvideZooKeeperCoordinator,
			config.ProvideRedisCache,
			internalConfig.ProvideIngestionHealthHandler,
			internalConfig.ProvideIngestionRouterConfig,
			internalConfig.ProvideIngestionServerConfig,
			internalConfig.ProvideIngestionRouteInitializer,
			config.ProvideRouter,
			server.NewHTTPServer,
		),
		executor_init.ExecutorQueueModule(),
		fx.Invoke(func(*server.HTTPServer) {}),
	).Run()
}
