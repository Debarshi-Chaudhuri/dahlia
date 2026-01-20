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
			config.ProvideLogger,
			config.ProvideRouteDependencies,
			internalConfig.ProvideIngestionHealthHandler,
			internalConfig.ProvideIngestionRouterConfig,
			internalConfig.ProvideIngestionServerConfig,
			internalConfig.ProvideIngestionRouteInitializer,
			config.ProvideRouter,
			server.NewHTTPServer,
		),
		fx.Invoke(func(*server.HTTPServer) {}),
	).Run()
}
