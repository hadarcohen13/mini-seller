package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/hadarco13/mini-seller/internal/config"
	"github.com/hadarco13/mini-seller/internal/middleware"
)

func main() {

	if err := config.LoadConfig(); err != nil {
		log.Fatal("Failed to load configuration:", err)
	}

	cfg := config.GetConfig()

	muxRouter := mux.NewRouter()
	muxRouter.Use(middleware.LoggingMiddleware)

	muxRouter.HandleFunc("/healthCheck", healthCheckHandler).Methods("GET")

	serverAddr := fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port)
	fmt.Printf("Starting server in %s mode on %s\n", cfg.Environment, serverAddr)
	if err := http.ListenAndServe(serverAddr, muxRouter); err != nil {
		log.Fatal("Error starting server:", err)
	}

}

// this operation responds with "OK" for healthCheck
func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "OK")

}
