package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/hadarco13/mini-seller/internal/middleware"
)

func main() {

	muxRouter := mux.NewRouter()

	muxRouter.Use(middleware.LoggingMiddleware)
	
	muxRouter.HandleFunc("/healthCheck", healthCheckHandler).Methods("GET")

	port := "8080"
	fmt.Println("Starting server on port " + port)
	if err := http.ListenAndServe(":"+port, muxRouter); err != nil {
		log.Fatal("Error starting server, ListenAndServe: ", err)
	}

}

// this operation responds with "OK" for healthCheck
func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "OK")

}
