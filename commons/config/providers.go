package config

import (
	"dahlia/commons/routes"
	"dahlia/internal/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/fx/fxevent"
)

// ProvideLogger creates and configures the logger for the application
func ProvideLogger() (logger.Logger, error) {
	return logger.NewZapLoggerForDev()
}

// ProvideFxLogger creates the FX event logger using the application logger
func ProvideFxLogger(log logger.Logger) fxevent.Logger {
	return &fxevent.ZapLogger{
		Logger: log.(*logger.ZapLogger).Logger(),
	}
}

// ProvideRouteDependencies creates route dependencies
func ProvideRouteDependencies(log logger.Logger) routes.RouteDependencies {
	return routes.RouteDependencies{
		Logger: log,
	}
}

// ProvideRouter creates and configures the Gin router with all routes
func ProvideRouter(
	config routes.RouterConfig,
	deps routes.RouteDependencies,
	routeInitializer func(*gin.Engine, routes.RouteDependencies),
) *gin.Engine {
	router := routes.NewRouter(config, deps)
	routeInitializer(router, deps)
	return router
}
