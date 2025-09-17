package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/hadarco13/mini-seller/internal/middleware"
	"github.com/stretchr/testify/assert"
)

func TestRateLimiterMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		qps            float64
		burst          int
		requests       int
		requestDelay   time.Duration
		expectAllowed  int
		expectRejected int
	}{
		{
			name:           "Within Rate Limit",
			qps:            10.0,
			burst:          20,
			requests:       15,
			requestDelay:   50 * time.Millisecond, // Slower than rate limit
			expectAllowed:  15,
			expectRejected: 0,
		},
		{
			name:           "Exceeds Burst Limit",
			qps:            1.0,
			burst:          5,
			requests:       10,
			requestDelay:   0, // Immediate requests
			expectAllowed:  5,
			expectRejected: 5,
		},
		{
			name:           "High QPS Within Burst",
			qps:            100.0,
			burst:          50,
			requests:       40,
			requestDelay:   0, // Immediate requests
			expectAllowed:  40,
			expectRejected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create rate limiter middleware
			rateLimitMiddleware := middleware.RateLimiterMiddleware(tt.qps, tt.burst)

			// Create test handler
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			})

			// Wrap handler with middleware
			handler := rateLimitMiddleware(testHandler)

			allowed := 0
			rejected := 0
			var wg sync.WaitGroup
			var mu sync.Mutex

			// Make concurrent requests
			for i := 0; i < tt.requests; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()

					if tt.requestDelay > 0 {
						time.Sleep(time.Duration(i) * tt.requestDelay)
					}

					req := httptest.NewRequest(http.MethodGet, "/test", nil)
					req.RemoteAddr = "127.0.0.1:12345" // Set consistent IP
					w := httptest.NewRecorder()

					handler.ServeHTTP(w, req)

					mu.Lock()
					if w.Code == http.StatusOK {
						allowed++
					} else {
						rejected++ // Any non-200 status is considered rejected
					}
					mu.Unlock()
				}(i)
			}

			wg.Wait()

			assert.Equal(t, tt.expectAllowed, allowed, "Unexpected number of allowed requests")
			assert.Equal(t, tt.expectRejected, rejected, "Unexpected number of rejected requests")
		})
	}
}

func TestRateLimiterMiddleware_GlobalLimit(t *testing.T) {
	// Create rate limiter with low limits - this is a global limiter, not per-IP
	rateLimitMiddleware := middleware.RateLimiterMiddleware(1.0, 2)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := rateLimitMiddleware(testHandler)

	// Test that rate limit applies globally across all IPs
	ips := []string{"192.168.1.1:12345", "192.168.1.2:12345"}

	allowed := 0
	rejected := 0

	// Make requests from different IPs - they should share the same rate limit
	for _, ip := range ips {
		for j := 0; j < 2; j++ {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.RemoteAddr = ip
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code == http.StatusOK {
				allowed++
			} else {
				rejected++
			}
		}
	}

	// With burst=2, only 2 requests should be allowed regardless of IP
	assert.Equal(t, 2, allowed, "Only burst limit should be allowed")
	assert.Equal(t, 2, rejected, "Remaining requests should be rejected")
}

func TestRateLimiterMiddleware_RecoveryOverTime(t *testing.T) {
	// Create rate limiter that allows recovery
	rateLimitMiddleware := middleware.RateLimiterMiddleware(5.0, 1) // 5 QPS, burst of 1

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := rateLimitMiddleware(testHandler)

	// First request should be allowed
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req1.RemoteAddr = "127.0.0.1:12345"
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)

	// Second immediate request should be rate limited
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.RemoteAddr = "127.0.0.1:12345"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	assert.NotEqual(t, http.StatusOK, w2.Code) // Should be rate limited (any non-200)

	// Wait for rate limiter to recover (1/5 second = 200ms)
	time.Sleep(250 * time.Millisecond)

	// Third request after recovery should be allowed
	req3 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req3.RemoteAddr = "127.0.0.1:12345"
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, req3)
	assert.Equal(t, http.StatusOK, w3.Code)
}

// Benchmark tests for rate limiter performance
func BenchmarkRateLimiterMiddleware(b *testing.B) {
	rateLimitMiddleware := middleware.RateLimiterMiddleware(1000.0, 1000) // High limits to avoid rate limiting

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := rateLimitMiddleware(testHandler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
		}
	})
}
