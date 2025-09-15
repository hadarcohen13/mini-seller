package middleware

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
)

// RequestIDMiddleware adds a unique request ID to each request
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if request ID already exists in header
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = generateUUID()
		}

		// Add to response header
		w.Header().Set("X-Request-ID", requestID)

		// Add to request context
		ctx := context.WithValue(r.Context(), "request_id", requestID)
		r = r.WithContext(ctx)

		next.ServeHTTP(w, r)
	})
}

// generateUUID generates a simple UUID-like string
func generateUUID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		bytes[0:4],
		bytes[4:6],
		bytes[6:8],
		bytes[8:10],
		bytes[10:16])
}
