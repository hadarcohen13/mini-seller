package server

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gorilla/mux"
	"github.com/hadarco13/mini-seller/internal/middleware"
	"github.com/hadarco13/mini-seller/internal/models"
)

type Server struct {
	conf          *models.ServerConfig
	doneCleanChan chan struct{}
	httpServer    http.Server
	signals       chan os.Signal
}

// healthCheckHandler is a simple handler to check server status.
func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "OK")
}

func NewServer(conf *models.ServerConfig) (*Server, error) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	return &Server{
		conf:          conf,
		signals:       signals,
		doneCleanChan: make(chan struct{}, 1),
	}, nil
}
func (s *Server) Start() error {
	//init router
	r := mux.NewRouter()

	// Register our first route: a health check endpoint
	r.HandleFunc("/healthCheck", healthCheckHandler).Methods("GET")

	// Apply middleware globally in the correct order
	//r.Use(middleware.RequestIDMiddleware)
	//r.Use(middleware.CORSMiddleware)
	r.Use(middleware.LoggingMiddleware)

	// Apply rate limiting using values from our config
	//r.Use(middleware.RateLimiterMiddleware(config.Config.RateLimit.RPS, config.Config.RateLimit.Burst))

	// Start the server
	port := s.conf.Port
	fmt.Printf("Server starting on port %s...\n", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatal("ListenAndServe:", err)
		return err
	}

	<-s.doneCleanChan
	log.Println("Received cleaning done signal")
	close(s.doneCleanChan)
	log.Println("Server shutdown successfully")
	return nil
}
