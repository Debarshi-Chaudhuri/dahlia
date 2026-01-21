package main

import (
	"context"

	"dahlia/commons/config"
	"dahlia/commons/server"
	internalConfig "dahlia/internal/config"
	"dahlia/internal/service"

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
			config.ProvideRedisCache,
			config.ProvideDynamoDBClient,
			internalConfig.ProvideJobRepository,
			internalConfig.ProvideScheduler,
			internalConfig.ProvideSchedulerHandler,
			internalConfig.ProvideSchedulerHealthHandler,
			internalConfig.ProvideSchedulerRouterConfig,
			internalConfig.ProvideSchedulerServerConfig,
			internalConfig.ProvideSchedulerRouteInitializer,
			config.ProvideRouter,
			server.NewHTTPServer,
		),
		fx.Invoke(func(lc fx.Lifecycle, scheduler *service.Scheduler, srv *server.HTTPServer) {
			lc.Append(fx.Hook{
				OnStart: func(ctx context.Context) error {
					// Start scheduler cron
					return scheduler.Start(ctx)
				},
				OnStop: func(ctx context.Context) error {
					// Stop scheduler cron
					return scheduler.Stop(ctx)
				},
			})
		}),
	).Run()
}
