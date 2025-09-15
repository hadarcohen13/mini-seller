package main

import (
	"log"

	"github.com/hadarco13/mini-seller/cmd/server"
	"github.com/hadarco13/mini-seller/internal/config"
)

func main() {
	// Load configuration
	if err := config.LoadConfig(); err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Create server
	srv, err := server.NewServer()
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Start server (blocks until shutdown signal)
	if err := srv.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
