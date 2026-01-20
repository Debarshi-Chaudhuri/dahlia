package server

import (
	"context"
	"net/http"

	"dahlia/internal/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/fx"
)

type HTTPServer struct {
	server *http.Server
	logger logger.Logger
}

type ServerConfig struct {
	Port string
}

func NewHTTPServer(
	lc fx.Lifecycle,
	router *gin.Engine,
	config ServerConfig,
	log logger.Logger,
) *HTTPServer {
	srv := &http.Server{
		Addr:    ":" + config.Port,
		Handler: router,
	}

	httpServer := &HTTPServer{
		server: srv,
		logger: log.With(logger.String("component", "http_server")),
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			httpServer.logger.Info("starting HTTP server", logger.String("addr", srv.Addr))
			go func() {
				if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					httpServer.logger.Fatal("failed to start HTTP server", logger.Error(err))
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			httpServer.logger.Info("shutting down HTTP server")
			return srv.Shutdown(ctx)
		},
	})

	return httpServer
}

func (s *HTTPServer) GetServer() *http.Server {
	return s.server
}