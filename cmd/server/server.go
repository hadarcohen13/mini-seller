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
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

type Server struct {
	config     *config.AppConfig
	httpServer *http.Server
	signals    chan os.Signal
}

// healthCheckHandler is a simple handler to check server status.
func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "OK")
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

// setupMiddleware applies all middleware to the router.
func (s *Server) setupMiddleware(r *mux.Router) {
	// Your custom error handler should go first
	r.Use(middleware.ErrorHandler)

	// Apply all other middleware in the correct order
	r.Use(middleware.RequestIDMiddleware)
	r.Use(middleware.CORSMiddleware)
	r.Use(middleware.LoggingMiddleware)

	// Use config to apply rate limiting
	rateLimit := 10.0 // 10 requests per second
	burst := 20       // allow bursts of up to 20 requests
	if s.config.Debug {
		rateLimit = 100.0
		burst = 200
	}
	r.Use(middleware.RateLimiterMiddleware(rateLimit, burst))
}

func (s *Server) Start() error {
	r := mux.NewRouter()

	s.setupMiddleware(r)

	r.HandleFunc("/health", healthCheckHandler).Methods("GET")

	r.HandleFunc("/bid/request", handlers.BidRequestHandler).Methods("POST")
	r.HandleFunc("/bid/test", handlers.BidRequestHandler).Methods("POST")

	r.Handle("/metrics", promhttp.Handler()).Methods("GET")

	serverAddr := fmt.Sprintf("%s:%s", s.config.Server.Host, s.config.Server.Port)
	s.httpServer = &http.Server{
		Addr:    serverAddr,
		Handler: r,
	}

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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown server
	if err := s.httpServer.Shutdown(ctx); err != nil {
		logrus.Errorf("Server forced to shutdown: %v", err)
		return err
	}

	logrus.Info("Server shutdown successfully")
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}

// setupRoutes is now a method that correctly initializes the router and handlers
func (s *Server) setupRoutes() {
	r := mux.NewRouter()

	// Apply all middleware *before* registering any handlers.
	// The order here matters.
	s.setupMiddleware(r)

	// Register routes
	r.HandleFunc("/health", handlers.HealthHandler).Methods("GET")
	r.HandleFunc("/metrics", handlers.MetricsHandler).Methods("GET")
	r.HandleFunc("/bid/request", handlers.BidRequestHandler).Methods("POST")
	r.HandleFunc("/bid/test", handlers.BidRequestHandler).Methods("POST")

	serverAddr := fmt.Sprintf("%s:%s", s.config.Server.Host, s.config.Server.Port)
	s.httpServer = &http.Server{
		Addr:    serverAddr,
		Handler: r,
	}
}
