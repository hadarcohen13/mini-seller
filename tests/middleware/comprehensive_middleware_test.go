package middleware_test

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/hadarco13/mini-seller/internal/config"
	"github.com/hadarco13/mini-seller/internal/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

func setupMiddlewareTestConfig() {
	os.Setenv("MINI_SELLER_ENVIRONMENT", "test")
	os.Setenv("MINI_SELLER_SERVER_PORT", "8080")
}

func resetMiddlewarePrometheusRegistry() {
	prometheus.DefaultRegisterer = prometheus.NewRegistry()
}

func TestCORSMiddleware(t *testing.T) {
	resetMiddlewarePrometheusRegistry()
	setupMiddlewareTestConfig()
	config.LoadConfig()

	// Create test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Create CORS middleware
	handler := middleware.CORSMiddleware(testHandler)

	tests := []struct {
		name         string
		method       string
		origin       string
		expectCORS   bool
		expectStatus int
	}{
		{
			name:         "Preflight OPTIONS request",
			method:       http.MethodOptions,
			origin:       "https://example.com",
			expectCORS:   true,
			expectStatus: http.StatusOK,
		},
		{
			name:         "Regular GET request",
			method:       http.MethodGet,
			origin:       "https://example.com",
			expectCORS:   true,
			expectStatus: http.StatusOK,
		},
		{
			name:         "POST request without origin",
			method:       http.MethodPost,
			origin:       "",
			expectCORS:   true, // CORS headers should still be set
			expectStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/test", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if tt.method == http.MethodOptions {
				req.Header.Set("Access-Control-Request-Method", "POST")
				req.Header.Set("Access-Control-Request-Headers", "Content-Type")
			}

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.expectStatus, w.Code)

			if tt.expectCORS {
				assert.NotEmpty(t, w.Header().Get("Access-Control-Allow-Origin"))
				assert.NotEmpty(t, w.Header().Get("Access-Control-Allow-Methods"))
				assert.NotEmpty(t, w.Header().Get("Access-Control-Allow-Headers"))
			}
		})
	}
}

func TestErrorHandler_PanicRecovery(t *testing.T) {
	resetMiddlewarePrometheusRegistry()
	setupMiddlewareTestConfig()
	config.LoadConfig()

	// Create test handler that panics
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	// Create error middleware
	handler := middleware.ErrorHandler(panicHandler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	// Should not panic, should recover and return 500
	assert.NotPanics(t, func() {
		handler.ServeHTTP(w, req)
	})

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "error")
}

func TestErrorHandler_NormalExecution(t *testing.T) {
	resetMiddlewarePrometheusRegistry()
	setupMiddlewareTestConfig()
	config.LoadConfig()

	// Create normal test handler
	normalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Success"))
	})

	// Create error middleware
	handler := middleware.ErrorHandler(normalHandler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "Success", w.Body.String())
}

func TestLoggingMiddleware(t *testing.T) {
	resetMiddlewarePrometheusRegistry()
	setupMiddlewareTestConfig()
	config.LoadConfig()

	// Create test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("Created"))
	})

	// Create logging middleware
	handler := middleware.LoggingMiddleware(testHandler)

	tests := []struct {
		name         string
		method       string
		path         string
		userAgent    string
		expectStatus int
	}{
		{
			name:         "GET request",
			method:       http.MethodGet,
			path:         "/api/test",
			userAgent:    "Test-Agent/1.0",
			expectStatus: http.StatusCreated,
		},
		{
			name:         "POST request",
			method:       http.MethodPost,
			path:         "/api/create",
			userAgent:    "Mozilla/5.0",
			expectStatus: http.StatusCreated,
		},
		{
			name:         "Request without User-Agent",
			method:       http.MethodDelete,
			path:         "/api/delete",
			userAgent:    "",
			expectStatus: http.StatusCreated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			req.RemoteAddr = "192.168.1.100:12345"
			if tt.userAgent != "" {
				req.Header.Set("User-Agent", tt.userAgent)
			}

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.expectStatus, w.Code)
			assert.Equal(t, "Created", w.Body.String())
		})
	}
}

func TestRequestIDMiddleware(t *testing.T) {
	resetMiddlewarePrometheusRegistry()
	setupMiddlewareTestConfig()
	config.LoadConfig()

	// Create test handler that checks for request ID
	var capturedRequestID string
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedRequestID = r.Header.Get("X-Request-ID")
		w.WriteHeader(http.StatusOK)
	})

	// Create request ID middleware
	handler := middleware.RequestIDMiddleware(testHandler)

	tests := []struct {
		name              string
		providedRequestID string
		expectNewID       bool
	}{
		{
			name:              "Request without ID - should generate new",
			providedRequestID: "",
			expectNewID:       true,
		},
		{
			name:              "Request with existing ID - should keep it",
			providedRequestID: "existing-request-123",
			expectNewID:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.providedRequestID != "" {
				req.Header.Set("X-Request-ID", tt.providedRequestID)
			}

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			if tt.expectNewID {
				assert.NotEmpty(t, capturedRequestID)
				assert.NotEqual(t, tt.providedRequestID, capturedRequestID)
			} else {
				assert.Equal(t, tt.providedRequestID, capturedRequestID)
			}

			// Check response header also has the request ID
			responseRequestID := w.Header().Get("X-Request-ID")
			assert.Equal(t, capturedRequestID, responseRequestID)
		})
	}
}

func TestMiddleware_ChainedExecution(t *testing.T) {
	resetMiddlewarePrometheusRegistry()
	setupMiddlewareTestConfig()
	config.LoadConfig()

	// Create test handler
	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check that all middleware has been applied
		requestID := r.Header.Get("X-Request-ID")
		assert.NotEmpty(t, requestID)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("All middleware applied"))
	})

	// Chain multiple middlewares
	handler := middleware.CORSMiddleware(
		middleware.ErrorHandler(
			middleware.RequestIDMiddleware(
				middleware.LoggingMiddleware(finalHandler),
			),
		),
	)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "All middleware applied", w.Body.String())

	// Check CORS headers are present
	assert.NotEmpty(t, w.Header().Get("Access-Control-Allow-Origin"))
	// Check Request ID header is present
	assert.NotEmpty(t, w.Header().Get("X-Request-ID"))
}

func TestMiddleware_ErrorScenarios(t *testing.T) {
	resetMiddlewarePrometheusRegistry()
	setupMiddlewareTestConfig()
	config.LoadConfig()

	t.Run("Panic with custom error", func(t *testing.T) {
		panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic(errors.New("custom panic error"))
		})

		handler := middleware.ErrorHandler(panicHandler)
		req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewBuffer([]byte("test")))
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("Panic with string", func(t *testing.T) {
		panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic("string panic")
		})

		handler := middleware.ErrorHandler(panicHandler)
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

// Benchmark tests
func BenchmarkCORSMiddleware(b *testing.B) {
	resetMiddlewarePrometheusRegistry()
	setupMiddlewareTestConfig()
	config.LoadConfig()

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware.CORSMiddleware(testHandler)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://example.com")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}
}

func BenchmarkErrorHandler(b *testing.B) {
	resetMiddlewarePrometheusRegistry()
	setupMiddlewareTestConfig()
	config.LoadConfig()

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware.ErrorHandler(testHandler)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}
}

func BenchmarkRequestIDMiddleware(b *testing.B) {
	resetMiddlewarePrometheusRegistry()
	setupMiddlewareTestConfig()
	config.LoadConfig()

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware.RequestIDMiddleware(testHandler)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}
}
