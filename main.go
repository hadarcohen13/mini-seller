package main

import (
	"github.com/hadarco13/mini-seller/cmd/server"
	"github.com/hadarco13/mini-seller/internal/config"
	"github.com/sirupsen/logrus"
)

func main() {

	// Load configuration
	if err := config.LoadConfig(); err != nil {
		logrus.Fatalf("Failed to load configuration: %v", err)
	}

	// Create server
	srv, err := server.NewServer()
	if err != nil {
		logrus.Fatalf("Failed to create server: %v", err)
	}

	// Start server (blocks until shutdown signal)
	if err := srv.Start(); err != nil {
		logrus.Fatalf("Server error: %v", err)
	}
}

func init() {
	// Set the formatter to JSON. This is crucial for structured logging.
	logrus.SetFormatter(&logrus.JSONFormatter{})

	// Set the log level. We'll start with INFO.
	logrus.SetLevel(logrus.InfoLevel)
}
