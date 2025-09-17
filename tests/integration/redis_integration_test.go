package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gorilla/mux"
	"github.com/hadarco13/mini-seller/internal/config"
	"github.com/hadarco13/mini-seller/internal/handlers"
	"github.com/hadarco13/mini-seller/internal/middleware"
	"github.com/hadarco13/mini-seller/internal/redis"
	"github.com/prometheus/client_golang/prometheus"
	redisClient "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupRedisIntegrationTestConfig() {
	os.Setenv("MINI_SELLER_ENVIRONMENT", "test")
	os.Setenv("MINI_SELLER_SERVER_PORT", "8080")
}

func resetRedisIntegrationPrometheusRegistry() {
	prometheus.DefaultRegisterer = prometheus.NewRegistry()
}

func setupTestRedisServer() (*miniredis.Miniredis, *httptest.Server, func()) {
	resetRedisIntegrationPrometheusRegistry()
	setupRedisIntegrationTestConfig()

	// Start miniredis
	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		panic(err)
	}

	// Update config to point to miniredis
	os.Setenv("MINI_SELLER_REDIS_HOST", strings.Split(mr.Addr(), ":")[0])
	os.Setenv("MINI_SELLER_REDIS_PORT", strings.Split(mr.Addr(), ":")[1])

	config.LoadConfig()

	// Create and set test Redis client
	testClient := redisClient.NewClient(&redisClient.Options{
		Addr: mr.Addr(),
		DB:   0,
	})
	redis.SetTestClient(testClient)

	// Setup HTTP server
	router := mux.NewRouter()
	router.Use(middleware.ErrorHandler)
	router.Use(middleware.RequestIDMiddleware)
	router.Use(middleware.CORSMiddleware)
	router.Use(middleware.LoggingMiddleware)
	router.Use(middleware.RateLimiterMiddleware(100.0, 200))

	router.HandleFunc("/health", handlers.HealthHandler).Methods("GET")
	router.HandleFunc("/bid/request", handlers.BidRequestHandler).Methods("POST")

	httpServer := httptest.NewServer(router)

	cleanup := func() {
		testClient.Close()
		mr.Close()
		httpServer.Close()
		os.Unsetenv("MINI_SELLER_REDIS_HOST")
		os.Unsetenv("MINI_SELLER_REDIS_PORT")
	}

	return mr, httpServer, cleanup
}

func TestRedisIntegration_Connection(t *testing.T) {
	mr, _, cleanup := setupTestRedisServer()
	defer cleanup()

	ctx := context.Background()
	client := redis.GetClient()
	require.NotNil(t, client)

	// Test basic ping
	pong, err := client.Ping(ctx).Result()
	require.NoError(t, err)
	assert.Equal(t, "PONG", pong)

	// Test basic operations
	err = client.Set(ctx, "test_key", "test_value", time.Minute).Err()
	require.NoError(t, err)

	val, err := client.Get(ctx, "test_key").Result()
	require.NoError(t, err)
	assert.Equal(t, "test_value", val)

	// Verify in miniredis
	mrVal, err2 := mr.Get("test_key")
	assert.NoError(t, err2)
	assert.Equal(t, "test_value", mrVal)
}

func TestRedisIntegration_RateLimiting(t *testing.T) {
	mr, _, cleanup := setupTestRedisServer()
	defer cleanup()

	ctx := context.Background()

	tests := []struct {
		name           string
		requests       int
		expectedStatus []int
		description    string
	}{
		{
			name:           "Within Rate Limit",
			requests:       5,
			expectedStatus: []int{200, 200, 200, 200, 200},
			description:    "All requests should succeed when within rate limit",
		},
		{
			name:     "Exceed Rate Limit",
			requests: 10,
			expectedStatus: []int{
				200, 200, 200, 200, 200, // First 5 should pass
				429, 429, 429, 429, 429, // Next 5 should be rate limited
			},
			description: "Requests should be rate limited after threshold",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear Redis before each test
			client := redis.GetClient()
			client.FlushDB(ctx)
			mr.FlushDB()

			// Create rate limiter
			limiter := redis.RateLimitByIP("192.168.1.100", 5) // 5 requests per minute

			for i := 0; i < tt.requests; i++ {
				allowed, err := limiter.Allow(ctx)
				require.NoError(t, err)

				if i < 5 {
					assert.True(t, allowed, "Request %d should be allowed", i+1)
				} else {
					assert.False(t, allowed, "Request %d should be rate limited", i+1)
				}
			}

			// Verify final count
			count, err := limiter.GetCount(ctx)
			require.NoError(t, err)
			expectedCount := min(tt.requests, 5)
			assert.Equal(t, expectedCount, count)
		})
	}
}

func TestRedisIntegration_HealthCheck(t *testing.T) {
	mr, httpServer, cleanup := setupTestRedisServer()
	defer cleanup()

	t.Run("Redis Available", func(t *testing.T) {
		resp, err := http.Get(httpServer.URL + "/health")
		require.NoError(t, err)
		defer resp.Body.Close()

		// Should return healthy or warning status (not critical)
		assert.Contains(t, []int{http.StatusOK, http.StatusPartialContent}, resp.StatusCode)
	})

	t.Run("Redis Unavailable", func(t *testing.T) {
		// Stop miniredis to simulate Redis failure
		mr.Close()

		resp, err := http.Get(httpServer.URL + "/health")
		require.NoError(t, err)
		defer resp.Body.Close()

		// Should return critical status when Redis is down
		assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	})
}

func TestRedisIntegration_ConcurrentRateLimiting(t *testing.T) {
	_, _, cleanup := setupTestRedisServer()
	defer cleanup()

	ctx := context.Background()
	const numGoroutines = 10
	const requestsPerGoroutine = 5
	const rateLimit = 20

	limiter := redis.RateLimitByIP("concurrent-test", rateLimit)

	var wg sync.WaitGroup
	var mu sync.Mutex
	allowedCount := 0
	deniedCount := 0

	// Launch concurrent goroutines
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < requestsPerGoroutine; j++ {
				allowed, err := limiter.Allow(ctx)
				require.NoError(t, err)

				mu.Lock()
				if allowed {
					allowedCount++
				} else {
					deniedCount++
				}
				mu.Unlock()

				// Small delay to simulate real usage
				time.Sleep(1 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()

	totalRequests := numGoroutines * requestsPerGoroutine
	assert.Equal(t, totalRequests, allowedCount+deniedCount)
	assert.Equal(t, rateLimit, allowedCount, "Should allow exactly the rate limit number of requests")
	assert.Equal(t, totalRequests-rateLimit, deniedCount)

	// Verify Redis counter
	count, err := limiter.GetCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, rateLimit, count)
}

func TestRedisIntegration_RateLimitTypes(t *testing.T) {
	_, _, cleanup := setupTestRedisServer()
	defer cleanup()

	ctx := context.Background()

	t.Run("IP Rate Limiting", func(t *testing.T) {
		limiter := redis.RateLimitByIP("192.168.1.1", 3)

		// Allow 3 requests
		for i := 0; i < 3; i++ {
			allowed, err := limiter.Allow(ctx)
			require.NoError(t, err)
			assert.True(t, allowed)
		}

		// 4th should be denied
		allowed, err := limiter.Allow(ctx)
		require.NoError(t, err)
		assert.False(t, allowed)
	})

	t.Run("Buyer Rate Limiting", func(t *testing.T) {
		buyerEndpoint := "https://buyer.example.com/bid"
		limiter := redis.RateLimitByBuyer(buyerEndpoint, 2)

		// Allow 2 requests
		for i := 0; i < 2; i++ {
			allowed, err := limiter.Allow(ctx)
			require.NoError(t, err)
			assert.True(t, allowed)
		}

		// 3rd should be denied
		allowed, err := limiter.Allow(ctx)
		require.NoError(t, err)
		assert.False(t, allowed)

		// Verify correct key structure
		assert.Contains(t, limiter.Key, "buyer:")
		assert.Contains(t, limiter.Key, buyerEndpoint)
	})

	t.Run("User Rate Limiting", func(t *testing.T) {
		userID := "user12345"
		limiter := redis.RateLimitByUser(userID, 5)

		// Allow 5 requests
		for i := 0; i < 5; i++ {
			allowed, err := limiter.Allow(ctx)
			require.NoError(t, err)
			assert.True(t, allowed)
		}

		// 6th should be denied
		allowed, err := limiter.Allow(ctx)
		require.NoError(t, err)
		assert.False(t, allowed)

		// Verify correct key structure
		assert.Contains(t, limiter.Key, "user:")
		assert.Contains(t, limiter.Key, userID)
	})
}

func TestRedisIntegration_TTLAndExpiration(t *testing.T) {
	mr, _, cleanup := setupTestRedisServer()
	defer cleanup()

	ctx := context.Background()

	t.Run("TTL Management", func(t *testing.T) {
		limiter := redis.RateLimitByIP("ttl-test", 5)

		// Make a request to create the key
		allowed, err := limiter.Allow(ctx)
		require.NoError(t, err)
		assert.True(t, allowed)

		// Check TTL exists
		ttl, err := limiter.GetTTL(ctx)
		require.NoError(t, err)
		assert.True(t, ttl > 0 && ttl <= time.Minute)

		// Verify in miniredis
		mrTTL := mr.TTL(limiter.Key)
		assert.True(t, mrTTL > 0)
	})

	t.Run("Key Expiration", func(t *testing.T) {
		limiter := redis.RateLimitByIP("expiration-test", 3)

		// Use up the limit
		for i := 0; i < 3; i++ {
			allowed, err := limiter.Allow(ctx)
			require.NoError(t, err)
			assert.True(t, allowed)
		}

		// Should be denied
		allowed, err := limiter.Allow(ctx)
		require.NoError(t, err)
		assert.False(t, allowed)

		// Fast-forward time in miniredis to simulate expiration
		mr.FastForward(time.Minute + time.Second)

		// Should be allowed again after expiration
		allowed, err = limiter.Allow(ctx)
		require.NoError(t, err)
		assert.True(t, allowed)

		// Count should reset to 1
		count, err := limiter.GetCount(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})
}

func TestRedisIntegration_ResetAndManagement(t *testing.T) {
	_, _, cleanup := setupTestRedisServer()
	defer cleanup()

	ctx := context.Background()

	t.Run("Reset Functionality", func(t *testing.T) {
		limiter := redis.RateLimitByIP("reset-test", 2)

		// Use up the limit
		limiter.Allow(ctx)
		limiter.Allow(ctx)

		// Should be denied
		allowed, err := limiter.Allow(ctx)
		require.NoError(t, err)
		assert.False(t, allowed)

		// Reset the counter
		err = limiter.Reset(ctx)
		require.NoError(t, err)

		// Should be allowed again
		allowed, err = limiter.Allow(ctx)
		require.NoError(t, err)
		assert.True(t, allowed)
	})

	t.Run("Multiple Limiter Independence", func(t *testing.T) {
		limiter1 := redis.RateLimitByIP("192.168.1.1", 2)
		limiter2 := redis.RateLimitByIP("192.168.1.2", 2)
		limiter3 := redis.RateLimitByBuyer("buyer1.com", 2)

		// Use up limiter1
		limiter1.Allow(ctx)
		limiter1.Allow(ctx)

		// limiter1 should be denied
		allowed, err := limiter1.Allow(ctx)
		require.NoError(t, err)
		assert.False(t, allowed)

		// limiter2 and limiter3 should still be allowed
		allowed, err = limiter2.Allow(ctx)
		require.NoError(t, err)
		assert.True(t, allowed)

		allowed, err = limiter3.Allow(ctx)
		require.NoError(t, err)
		assert.True(t, allowed)

		// Verify independent counts
		count1, _ := limiter1.GetCount(ctx)
		count2, _ := limiter2.GetCount(ctx)
		count3, _ := limiter3.GetCount(ctx)

		assert.Equal(t, 2, count1)
		assert.Equal(t, 1, count2)
		assert.Equal(t, 1, count3)
	})
}

func TestRedisIntegration_ErrorScenarios(t *testing.T) {
	setupRedisIntegrationTestConfig()
	resetRedisIntegrationPrometheusRegistry()

	t.Run("Redis Connection Failure", func(t *testing.T) {
		// Create a client pointing to non-existent Redis
		disconnectedClient := redisClient.NewClient(&redisClient.Options{
			Addr: "localhost:9999", // Non-existent Redis instance
			DB:   0,
		})
		redis.SetTestClient(disconnectedClient)
		defer disconnectedClient.Close()

		ctx := context.Background()
		limiter := redis.NewRedisRateLimiter("error_test", 5, time.Minute)

		// Operations should handle errors gracefully
		allowed, err := limiter.Allow(ctx)
		assert.Error(t, err)
		assert.False(t, allowed)

		_, err = limiter.GetCount(ctx)
		assert.Error(t, err)

		_, err = limiter.GetTTL(ctx)
		assert.Error(t, err)

		err = limiter.Reset(ctx)
		assert.Error(t, err)
	})

	t.Run("Invalid Redis Data", func(t *testing.T) {
		mr, _, cleanup := setupTestRedisServer()
		defer cleanup()

		ctx := context.Background()
		limiter := redis.NewRedisRateLimiter("invalid_data", 5, time.Minute)

		// Set invalid data directly in Redis
		mr.Set(limiter.Key, "invalid_number")

		// Should return an error for invalid data
		count, err := limiter.GetCount(ctx)
		assert.Error(t, err)
		assert.Equal(t, 0, count) // Should return 0 when there's an error
		assert.Contains(t, err.Error(), "invalid syntax")
	})
}

func TestRedisIntegration_Performance(t *testing.T) {
	_, _, cleanup := setupTestRedisServer()
	defer cleanup()

	ctx := context.Background()
	limiter := redis.RateLimitByIP("performance-test", 10000)

	// Measure performance of rate limiter operations
	start := time.Now()
	const numOperations = 1000

	for i := 0; i < numOperations; i++ {
		allowed, err := limiter.Allow(ctx)
		require.NoError(t, err)
		assert.True(t, allowed)
	}

	duration := time.Since(start)
	avgDuration := duration / numOperations

	t.Logf("Average duration per Allow() call: %v", avgDuration)

	// Each operation should be reasonably fast
	assert.Less(t, avgDuration, 5*time.Millisecond, "Rate limiter operations should be fast")

	// Verify all operations were recorded
	count, err := limiter.GetCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, numOperations, count)
}

func TestRedisIntegration_RealWorldScenario(t *testing.T) {
	mr, _, cleanup := setupTestRedisServer()
	defer cleanup()

	ctx := context.Background()

	// Simulate a real-world scenario with multiple IPs, buyers, and users
	scenario := struct {
		ips    []string
		buyers []string
		users  []string
	}{
		ips:    []string{"192.168.1.10", "192.168.1.11", "192.168.1.12"},
		buyers: []string{"buyer1.com", "buyer2.com", "buyer3.com"},
		users:  []string{"user1", "user2", "user3"},
	}

	// Create limiters for each entity
	ipLimiters := make([]*redis.RedisRateLimiter, len(scenario.ips))
	buyerLimiters := make([]*redis.RedisRateLimiter, len(scenario.buyers))
	userLimiters := make([]*redis.RedisRateLimiter, len(scenario.users))

	for i, ip := range scenario.ips {
		ipLimiters[i] = redis.RateLimitByIP(ip, 10)
	}
	for i, buyer := range scenario.buyers {
		buyerLimiters[i] = redis.RateLimitByBuyer(buyer, 5)
	}
	for i, user := range scenario.users {
		userLimiters[i] = redis.RateLimitByUser(user, 20)
	}

	// Simulate traffic
	var wg sync.WaitGroup
	const requestsPerEntity = 8

	// IP requests
	for i, limiter := range ipLimiters {
		wg.Add(1)
		go func(idx int, l *redis.RedisRateLimiter) {
			defer wg.Done()
			allowedCount := 0
			for j := 0; j < requestsPerEntity; j++ {
				if allowed, err := l.Allow(ctx); err == nil && allowed {
					allowedCount++
				}
			}
			t.Logf("IP %s: %d/%d requests allowed", scenario.ips[idx], allowedCount, requestsPerEntity)
		}(i, limiter)
	}

	// Buyer requests
	for i, limiter := range buyerLimiters {
		wg.Add(1)
		go func(idx int, l *redis.RedisRateLimiter) {
			defer wg.Done()
			allowedCount := 0
			for j := 0; j < requestsPerEntity; j++ {
				if allowed, err := l.Allow(ctx); err == nil && allowed {
					allowedCount++
				}
			}
			t.Logf("Buyer %s: %d/%d requests allowed", scenario.buyers[idx], allowedCount, requestsPerEntity)
		}(i, limiter)
	}

	// User requests
	for i, limiter := range userLimiters {
		wg.Add(1)
		go func(idx int, l *redis.RedisRateLimiter) {
			defer wg.Done()
			allowedCount := 0
			for j := 0; j < requestsPerEntity; j++ {
				if allowed, err := l.Allow(ctx); err == nil && allowed {
					allowedCount++
				}
			}
			t.Logf("User %s: %d/%d requests allowed", scenario.users[idx], allowedCount, requestsPerEntity)
		}(i, limiter)
	}

	wg.Wait()

	// Verify Redis state
	keys := mr.Keys()
	t.Logf("Total Redis keys created: %d", len(keys))

	// Should have keys for all entities
	assert.GreaterOrEqual(t, len(keys), 9) // 3 IPs + 3 buyers + 3 users

	// Verify key patterns
	ipKeys := 0
	buyerKeys := 0
	userKeys := 0

	for _, key := range keys {
		if strings.Contains(key, "rate_limit:ip:") {
			ipKeys++
		} else if strings.Contains(key, "rate_limit:buyer:") {
			buyerKeys++
		} else if strings.Contains(key, "rate_limit:user:") {
			userKeys++
		}
	}

	assert.Equal(t, 3, ipKeys)
	assert.Equal(t, 3, buyerKeys)
	assert.Equal(t, 3, userKeys)
}

// Helper function for Go versions without min built-in
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Benchmark tests
func BenchmarkRedisIntegration_RateLimitAllow(b *testing.B) {
	_, _, cleanup := setupTestRedisServer()
	defer cleanup()

	ctx := context.Background()
	limiter := redis.RateLimitByIP("benchmark", 1000000) // High limit to avoid blocking

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Allow(ctx)
	}
}

func BenchmarkRedisIntegration_ConcurrentAccess(b *testing.B) {
	_, _, cleanup := setupTestRedisServer()
	defer cleanup()

	ctx := context.Background()
	limiter := redis.RateLimitByIP("concurrent-benchmark", 1000000)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			limiter.Allow(ctx)
		}
	})
}
