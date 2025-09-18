package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/hadarco13/mini-seller/internal/config"
	"github.com/hadarco13/mini-seller/internal/errors"
	"github.com/hadarco13/mini-seller/internal/handlers"
	"github.com/hadarco13/mini-seller/internal/middleware"
	"github.com/sirupsen/logrus"
)

type Server struct {
	config     *config.AppConfig
	httpServer *http.Server
	signals    chan os.Signal
}

func NewServer() (*Server, error) {
	cfg := config.GetConfig()
	if cfg == nil {
		return nil, errors.NewConfigurationError("CONFIG_NOT_LOADED", "configuration not loaded")
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	s := &Server{
		config:  cfg,
		signals: signals,
	}

	s.setupRoutes()

	return s, nil
}

func (s *Server) Start() error {
	serverAddr := fmt.Sprintf("%s:%s", s.config.Server.Host, s.config.Server.Port)

	logrus.WithFields(logrus.Fields{
		"mode": s.config.Environment,
		"addr": serverAddr,
	}).Info("Server starting")

	// Start server in a goroutine
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logrus.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-s.signals
	logrus.Info("Shutdown signal received, starting graceful shutdown...")

	// Create shutdown context with timeout
	shutdownTimeout := time.Duration(s.config.Server.ShutdownTimeoutMs) * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	// Shutdown server
	if err := s.httpServer.Shutdown(ctx); err != nil {
		logrus.Errorf("Server forced to shutdown: %v", err)
		return err
	}

	logrus.Info("Server shutdown successfully")
	return nil
}

// setupMiddleware applies all middleware to the router.
func (s *Server) setupMiddleware(r *mux.Router) {
	r.Use(middleware.ErrorHandler)

	// Apply all other middleware in the correct order
	r.Use(middleware.RequestIDMiddleware)
	r.Use(middleware.CORSMiddleware)
	r.Use(middleware.LoggingMiddleware)

	// Use config to apply rate limiting
	rateLimit := s.config.RateLimit.QPS
	burst := s.config.RateLimit.Burst
	if s.config.Debug {
		rateLimit = rateLimit * 10 // 10x higher in debug mode
		burst = burst * 10
	}
	r.Use(middleware.RateLimiterMiddleware(rateLimit, burst))
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}

// setupRoutes initializes the router with middleware and handlers
func (s *Server) setupRoutes() {
	r := mux.NewRouter()

	// Apply all middleware
	s.setupMiddleware(r)

	// Register routes
	r.HandleFunc("/health", handlers.HealthHandler).Methods("GET")
	r.HandleFunc("/metrics", handlers.MetricsHandler).Methods("GET")
	r.HandleFunc("/bid/request", handlers.BidRequestHandler).Methods("POST")
	r.HandleFunc("/bid/test", handlers.BidRequestHandler).Methods("POST")

	// Create HTTP server
	serverAddr := fmt.Sprintf("%s:%s", s.config.Server.Host, s.config.Server.Port)
	s.httpServer = &http.Server{
		Addr:         serverAddr,
		Handler:      r,
		ReadTimeout:  time.Duration(s.config.Server.ReadTimeoutMs) * time.Millisecond,
		WriteTimeout: time.Duration(s.config.Server.WriteTimeoutMs) * time.Millisecond,
	}
}
