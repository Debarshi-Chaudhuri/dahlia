package routes

import (
	"net/http"

	"dahlia/commons/handler"
	"dahlia/internal/logger"

	"github.com/gin-gonic/gin"
)

type RouterConfig struct {
	ServiceName string
	Version     string
}

type RouteDependencies struct {
	Logger logger.Logger
}

type RouteOptions[InputDto any, OutputDto any] struct {
	Path        string
	Method      string
	ServiceFunc handler.ServiceFunc[InputDto, OutputDto]
	RequireAuth bool
}

func NewRouter(config RouterConfig, deps RouteDependencies) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	// Add global middlewares
	r.Use(handler.LoggingMiddleware(deps.Logger))
	r.Use(handler.ErrorHandlingMiddleware(deps.Logger))
	r.Use(handler.CORSMiddleware())

	// Set custom handlers for routing errors
	r.NoRoute(handler.NoRouteHandler())
	r.NoMethod(handler.NoMethodHandler())

	return r
}

func RegisterRoute[InputDto any, OutputDto any](
	group gin.IRouter,
	deps RouteDependencies,
	options RouteOptions[InputDto, OutputDto],
) {
	handlerDeps := handler.HandlerDependencies{
		Logger: deps.Logger,
	}

	ginHandler := handler.HandleFunc(handlerDeps, options.ServiceFunc)

	switch options.Method {
	case http.MethodGet:
		group.GET(options.Path, ginHandler)
	case http.MethodPost:
		group.POST(options.Path, ginHandler)
	case http.MethodPut:
		group.PUT(options.Path, ginHandler)
	case http.MethodDelete:
		group.DELETE(options.Path, ginHandler)
	case http.MethodPatch:
		group.PATCH(options.Path, ginHandler)
	default:
		deps.Logger.Error("unsupported HTTP method",
			logger.String("method", options.Method),
			logger.String("path", options.Path))
	}
}

func CreateAPIGroup(router *gin.Engine, version string) *gin.RouterGroup {
	return router.Group("/api/" + version)
}
