package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"webhook-processor/api/router"
	"webhook-processor/config"
	"webhook-processor/internal/queue"
	"webhook-processor/pkg/logger"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

type Server struct {
	httpServer    *http.Server
	metricsServer *http.Server
	logger        *logger.Logger
	publisher     queue.Publisher
}

func NewServer(cfg *config.Config, logger *logger.Logger) *Server {
	publisher, err := queue.NewRabbitMQ(cfg.RabbitMQ, logger.Desugar())
	if err != nil {
		logger.Fatalf("failed to create rabbitmq publisher: %v", err)
	}

	r := router.Setup(logger, publisher, cfg)

	// Create metrics server
	metricsAddr := fmt.Sprintf(":%d", cfg.Monitoring.PrometheusPort)
	metricsServer := &http.Server{
		Addr:    metricsAddr,
		Handler: promhttp.Handler(),
	}

	return &Server{
		httpServer: &http.Server{
			Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
			Handler: r,
		},
		metricsServer: metricsServer,
		logger:        logger,
		publisher:     publisher,
	}
}

func (s *Server) Start() error {
	// Start metrics server in a goroutine
	go func() {
		s.logger.Info("Metrics server starting on port " + s.metricsServer.Addr)
		if err := s.metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Errorf("metrics server error: %v", err)
		}
	}()

	// Start main HTTP server
	s.logger.Info("Server starting on port " + s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown() error {
	s.logger.Info("Server shutting down")
	if err := s.publisher.Close(); err != nil {
		s.logger.Error("failed to close publisher", zap.Error(err))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}
