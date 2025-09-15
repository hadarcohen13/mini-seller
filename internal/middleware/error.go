package middleware

import (
	"net/http"
	"runtime/debug"
	"time"

	"github.com/hadarco13/mini-seller/internal/errors"
	"github.com/sirupsen/logrus"
)

// ErrorHandler is a middleware that handles panics and errors
func ErrorHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				// Handle panic
				handlePanic(w, r, err)
			}
		}()

		// Create a custom response writer to capture errors
		wrapper := &errorResponseWriter{
			ResponseWriter: w,
			request:        r,
		}

		next.ServeHTTP(wrapper, r)
	})
}

// errorResponseWriter wraps http.ResponseWriter to capture errors
type errorResponseWriter struct {
	http.ResponseWriter
	request *http.Request
}

// WriteErrorResponse writes an error response with proper formatting
func WriteErrorResponse(w http.ResponseWriter, r *http.Request, err error) {
	var appErr *errors.AppError

	// Convert to AppError if it's not already
	if ae, ok := err.(*errors.AppError); ok {
		appErr = ae
	} else {
		// Wrap unknown errors
		appErr = errors.Wrap(err, errors.ErrorTypeInternal, "INTERNAL_ERROR", "An unexpected error occurred").
			WithContext("path", r.URL.Path).
			WithContext("method", r.Method).
			WithContext("user_agent", r.UserAgent())
	}

	// Add request context
	appErr.WithContext("request_id", getRequestID(r)).
		WithContext("timestamp", time.Now())

	// Log the error
	appErr.LogError()

	// Set appropriate headers
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(appErr.HTTPStatus)

	// Write JSON response
	w.Write(appErr.ToJSON())
}

// handlePanic handles recovered panics
func handlePanic(w http.ResponseWriter, r *http.Request, panicErr interface{}) {
	stackTrace := string(debug.Stack())

	appErr := errors.NewInternalError("PANIC_RECOVERED", "Application panic occurred").
		WithContext("panic_value", panicErr).
		WithContext("path", r.URL.Path).
		WithContext("method", r.Method).
		WithContext("user_agent", r.UserAgent()).
		WithContext("request_id", getRequestID(r))

	appErr.StackTrace = stackTrace

	logrus.WithFields(logrus.Fields{
		"panic_value": panicErr,
		"stack_trace": stackTrace,
		"path":        r.URL.Path,
		"method":      r.Method,
		"request_id":  getRequestID(r),
	}).Error("Panic recovered")

	WriteErrorResponse(w, r, appErr)
}

// getRequestID extracts request ID from context or generates one
func getRequestID(r *http.Request) string {
	// Try to get request ID from header first
	if requestID := r.Header.Get("X-Request-ID"); requestID != "" {
		return requestID
	}

	// Try to get from context (if set by another middleware)
	if requestID := r.Context().Value("request_id"); requestID != nil {
		if id, ok := requestID.(string); ok {
			return id
		}
	}

	// Generate a simple request ID (in production, use UUID)
	return generateRequestID()
}

// generateRequestID generates a simple request ID
func generateRequestID() string {
	return time.Now().Format("20060102150405") + "-" + "req"
}
