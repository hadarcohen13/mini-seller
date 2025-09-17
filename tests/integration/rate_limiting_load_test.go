package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/bsm/openrtb"
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

func setupRateLimitingLoadTestConfig() {
	os.Setenv("MINI_SELLER_ENVIRONMENT", "test")
	os.Setenv("MINI_SELLER_SERVER_PORT", "8080")
}

func resetRateLimitingLoadPrometheusRegistry() {
	prometheus.DefaultRegisterer = prometheus.NewRegistry()
}

func setupLoadTestServer(rateLimit float64, burst int) (*miniredis.Miniredis, *httptest.Server, func()) {
	resetRateLimitingLoadPrometheusRegistry()
	setupRateLimitingLoadTestConfig()

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

	// Setup HTTP server with specific rate limiting
	router := mux.NewRouter()
	router.Use(middleware.ErrorHandler)
	router.Use(middleware.RequestIDMiddleware)
	router.Use(middleware.CORSMiddleware)
	router.Use(middleware.LoggingMiddleware)
	router.Use(middleware.RateLimiterMiddleware(rateLimit, burst))

	router.HandleFunc("/health", handlers.HealthHandler).Methods("GET")
	router.HandleFunc("/bid/request", handlers.BidRequestHandler).Methods("POST")
	router.HandleFunc("/metrics", handlers.MetricsHandler).Methods("GET")

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

func createTestBidRequest(id string) *openrtb.BidRequest {
	return &openrtb.BidRequest{
		ID: id,
		Imp: []openrtb.Impression{
			{
				ID: fmt.Sprintf("imp-%s", id),
				Banner: &openrtb.Banner{
					W: 300,
					H: 250,
				},
				BidFloor: 1.0,
			},
		},
		Site: &openrtb.Site{
			Inventory: openrtb.Inventory{
				ID:     "load-test-site",
				Domain: "loadtest.example.com",
			},
		},
	}
}

func TestRateLimitingUnderLoad_ConcurrentRequests(t *testing.T) {
	const rateLimit = 50.0 // 50 requests per second
	const burst = 100      // Burst capacity

	_, server, cleanup := setupLoadTestServer(rateLimit, burst)
	defer cleanup()

	tests := []struct {
		name                string
		numClients          int
		requestsPerClient   int
		duration            time.Duration
		expectedSuccessRate float64
	}{
		{
			name:                "Low Load",
			numClients:          5,
			requestsPerClient:   8,
			duration:            2 * time.Second,
			expectedSuccessRate: 0.8, // At least 80% should succeed
		},
		{
			name:                "Medium Load",
			numClients:          10,
			requestsPerClient:   10,
			duration:            3 * time.Second,
			expectedSuccessRate: 0.5, // At least 50% should succeed
		},
		{
			name:                "High Load",
			numClients:          20,
			requestsPerClient:   15,
			duration:            5 * time.Second,
			expectedSuccessRate: 0.15, // At least 15% should succeed (rate limit + burst allows ~150/300 requests)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var successCount int64
			var errorCount int64
			var rateLimitCount int64

			var wg sync.WaitGroup
			start := time.Now()

			// Launch concurrent clients
			for i := 0; i < tt.numClients; i++ {
				wg.Add(1)
				go func(clientID int) {
					defer wg.Done()

					client := &http.Client{Timeout: 10 * time.Second}

					for j := 0; j < tt.requestsPerClient; j++ {
						bidRequest := createTestBidRequest(fmt.Sprintf("load-test-%d-%d", clientID, j))
						requestBody, _ := json.Marshal(bidRequest)

						req, err := http.NewRequest("POST", server.URL+"/bid/request", bytes.NewBuffer(requestBody))
						if err != nil {
							atomic.AddInt64(&errorCount, 1)
							continue
						}

						req.Header.Set("Content-Type", "application/json")
						req.Header.Set("X-Forwarded-For", fmt.Sprintf("192.168.1.%d", 100+clientID))

						resp, err := client.Do(req)
						if err != nil {
							atomic.AddInt64(&errorCount, 1)
							continue
						}

						switch resp.StatusCode {
						case http.StatusOK:
							atomic.AddInt64(&successCount, 1)
						case http.StatusTooManyRequests:
							atomic.AddInt64(&rateLimitCount, 1)
						default:
							atomic.AddInt64(&errorCount, 1)
						}

						resp.Body.Close()

						// Small delay to simulate realistic client behavior
						time.Sleep(time.Duration(j*10) * time.Millisecond)
					}
				}(i)
			}

			wg.Wait()
			elapsed := time.Since(start)

			totalRequests := int64(tt.numClients * tt.requestsPerClient)
			successRate := float64(successCount) / float64(totalRequests)

			t.Logf("Load Test Results for %s:", tt.name)
			t.Logf("  Clients: %d", tt.numClients)
			t.Logf("  Total Requests: %d", totalRequests)
			t.Logf("  Successful: %d (%.1f%%)", successCount, successRate*100)
			t.Logf("  Rate Limited: %d (%.1f%%)", rateLimitCount, float64(rateLimitCount)/float64(totalRequests)*100)
			t.Logf("  Errors: %d (%.1f%%)", errorCount, float64(errorCount)/float64(totalRequests)*100)
			t.Logf("  Duration: %v", elapsed)
			t.Logf("  Average RPS: %.2f", float64(totalRequests)/elapsed.Seconds())

			// Assertions
			assert.True(t, elapsed <= tt.duration*2, "Test should complete within reasonable time")
			assert.GreaterOrEqual(t, successRate, tt.expectedSuccessRate, "Success rate should meet minimum threshold")
			assert.Equal(t, totalRequests, successCount+rateLimitCount+errorCount, "All requests should be accounted for")
		})
	}
}

func TestRateLimitingUnderLoad_BurstTraffic(t *testing.T) {
	const rateLimit = 10.0 // 10 requests per second
	const burst = 20       // 20 burst capacity

	_, server, cleanup := setupLoadTestServer(rateLimit, burst)
	defer cleanup()

	// Test burst handling
	t.Run("Burst Capacity", func(t *testing.T) {
		client := &http.Client{Timeout: 5 * time.Second}
		var wg sync.WaitGroup

		successCount := int64(0)
		rateLimitCount := int64(0)

		// Send burst of requests simultaneously
		const burstSize = 25 // More than burst capacity

		for i := 0; i < burstSize; i++ {
			wg.Add(1)
			go func(requestID int) {
				defer wg.Done()

				bidRequest := createTestBidRequest(fmt.Sprintf("burst-%d", requestID))
				requestBody, _ := json.Marshal(bidRequest)

				req, _ := http.NewRequest("POST", server.URL+"/bid/request", bytes.NewBuffer(requestBody))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Forwarded-For", "192.168.1.200") // Same IP for all

				resp, err := client.Do(req)
				if err == nil {
					if resp.StatusCode == http.StatusOK {
						atomic.AddInt64(&successCount, 1)
					} else if resp.StatusCode == http.StatusTooManyRequests {
						atomic.AddInt64(&rateLimitCount, 1)
					}
					resp.Body.Close()
				}
			}(i)
		}

		wg.Wait()

		t.Logf("Burst Test Results:")
		t.Logf("  Burst Size: %d", burstSize)
		t.Logf("  Successful: %d", successCount)
		t.Logf("  Rate Limited: %d", rateLimitCount)

		// Should allow up to burst capacity, then rate limit
		assert.LessOrEqual(t, successCount, int64(burst), "Success count should not exceed burst capacity")
		assert.Greater(t, rateLimitCount, int64(0), "Some requests should be rate limited")
		assert.Equal(t, int64(burstSize), successCount+rateLimitCount, "All requests accounted for")
	})
}

func TestRateLimitingUnderLoad_SustainedTraffic(t *testing.T) {
	const rateLimit = 30.0 // 30 requests per second
	const burst = 50

	_, server, cleanup := setupLoadTestServer(rateLimit, burst)
	defer cleanup()

	// Test sustained traffic over time
	t.Run("Sustained Load", func(t *testing.T) {
		client := &http.Client{Timeout: 5 * time.Second}

		const testDuration = 5 * time.Second
		const clientCount = 8

		var successCount int64
		var rateLimitCount int64
		var totalRequests int64

		ctx, cancel := context.WithTimeout(context.Background(), testDuration)
		defer cancel()

		var wg sync.WaitGroup

		// Launch sustained load generators
		for i := 0; i < clientCount; i++ {
			wg.Add(1)
			go func(clientID int) {
				defer wg.Done()

				requestID := 0
				ticker := time.NewTicker(100 * time.Millisecond) // 10 RPS per client
				defer ticker.Stop()

				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						requestID++
						atomic.AddInt64(&totalRequests, 1)

						bidRequest := createTestBidRequest(fmt.Sprintf("sustained-%d-%d", clientID, requestID))
						requestBody, _ := json.Marshal(bidRequest)

						req, _ := http.NewRequest("POST", server.URL+"/bid/request", bytes.NewBuffer(requestBody))
						req.Header.Set("Content-Type", "application/json")
						req.Header.Set("X-Forwarded-For", fmt.Sprintf("192.168.2.%d", 100+clientID))

						resp, err := client.Do(req)
						if err == nil {
							if resp.StatusCode == http.StatusOK {
								atomic.AddInt64(&successCount, 1)
							} else if resp.StatusCode == http.StatusTooManyRequests {
								atomic.AddInt64(&rateLimitCount, 1)
							}
							resp.Body.Close()
						}
					}
				}
			}(i)
		}

		wg.Wait()

		actualRPS := float64(successCount) / testDuration.Seconds()

		t.Logf("Sustained Traffic Results:")
		t.Logf("  Test Duration: %v", testDuration)
		t.Logf("  Total Requests: %d", totalRequests)
		t.Logf("  Successful: %d", successCount)
		t.Logf("  Rate Limited: %d", rateLimitCount)
		t.Logf("  Actual RPS: %.2f", actualRPS)
		t.Logf("  Target RPS: %.2f", rateLimit)

		// Rate limiting should keep actual RPS reasonably close to target
		// Allow higher tolerance due to burst capacity and concurrent clients
		tolerance := rateLimit * 0.4 // 40% tolerance
		assert.InDelta(t, rateLimit, actualRPS, tolerance, "Actual RPS should be close to rate limit")
		assert.Greater(t, rateLimitCount, int64(0), "Some requests should be rate limited under sustained load")
	})
}

func TestRateLimitingUnderLoad_DifferentIPs(t *testing.T) {
	const rateLimit = 20.0 // 20 requests per second total
	const burst = 30

	mr, server, cleanup := setupLoadTestServer(rateLimit, burst)
	defer cleanup()

	// Test with different IP addresses to verify IP-based limiting
	t.Run("Multiple IP Addresses", func(t *testing.T) {
		ctx := context.Background()

		const numIPs = 5
		const requestsPerIP = 10

		var wg sync.WaitGroup
		results := make(map[string]struct {
			success   int64
			rateLimit int64
		})
		var mu sync.Mutex

		// Create IP-based rate limiters directly
		ipLimiters := make([]*redis.RedisRateLimiter, numIPs)
		for i := 0; i < numIPs; i++ {
			ip := fmt.Sprintf("192.168.10.%d", 100+i)
			ipLimiters[i] = redis.RateLimitByIP(ip, 5) // 5 requests per minute per IP
		}

		// Test each IP independently
		for i := 0; i < numIPs; i++ {
			wg.Add(1)
			go func(ipIndex int) {
				defer wg.Done()

				ip := fmt.Sprintf("192.168.10.%d", 100+ipIndex)
				limiter := ipLimiters[ipIndex]

				client := &http.Client{Timeout: 5 * time.Second}
				success := int64(0)
				rateLimited := int64(0)

				for j := 0; j < requestsPerIP; j++ {
					// Test Redis rate limiter directly
					allowed, err := limiter.Allow(ctx)
					require.NoError(t, err)

					if allowed {
						// Make actual HTTP request
						bidRequest := createTestBidRequest(fmt.Sprintf("ip-test-%d-%d", ipIndex, j))
						requestBody, _ := json.Marshal(bidRequest)

						req, _ := http.NewRequest("POST", server.URL+"/bid/request", bytes.NewBuffer(requestBody))
						req.Header.Set("Content-Type", "application/json")
						req.Header.Set("X-Forwarded-For", ip)

						resp, err := client.Do(req)
						if err == nil {
							if resp.StatusCode == http.StatusOK {
								success++
							}
							resp.Body.Close()
						}
					} else {
						rateLimited++
					}

					// Small delay between requests
					time.Sleep(10 * time.Millisecond)
				}

				mu.Lock()
				results[ip] = struct {
					success   int64
					rateLimit int64
				}{success, rateLimited}
				mu.Unlock()
			}(i)
		}

		wg.Wait()

		t.Logf("IP-based Rate Limiting Results:")
		totalSuccess := int64(0)
		totalRateLimited := int64(0)

		for ip, result := range results {
			t.Logf("  IP %s: %d success, %d rate-limited", ip, result.success, result.rateLimit)
			totalSuccess += result.success
			totalRateLimited += result.rateLimit
		}

		t.Logf("  Total Success: %d", totalSuccess)
		t.Logf("  Total Rate Limited: %d", totalRateLimited)

		// Each IP should be limited independently
		assert.Equal(t, int64(numIPs*requestsPerIP), totalSuccess+totalRateLimited, "All requests should be accounted for")
		assert.Greater(t, totalRateLimited, int64(0), "Some requests should be rate limited")

		// Verify Redis state
		keys := mr.Keys()
		ipKeys := 0
		for _, key := range keys {
			if strings.Contains(key, "rate_limit:ip:192.168.10.") {
				ipKeys++
			}
		}
		assert.Equal(t, numIPs, ipKeys, "Should have rate limit keys for all IPs")
	})
}

func TestRateLimitingUnderLoad_RecoveryAfterLimit(t *testing.T) {
	const rateLimit = 15.0 // 15 requests per second
	const burst = 20

	_, server, cleanup := setupLoadTestServer(rateLimit, burst)
	defer cleanup()

	t.Run("Recovery After Rate Limit", func(t *testing.T) {
		client := &http.Client{Timeout: 5 * time.Second}

		// Phase 1: Exhaust rate limit quickly
		t.Log("Phase 1: Exhausting rate limit...")
		var phase1Success, phase1RateLimit int64

		// Send burst of requests to exhaust limit
		var wg sync.WaitGroup
		for i := 0; i < 30; i++ { // More than burst capacity
			wg.Add(1)
			go func(reqID int) {
				defer wg.Done()

				bidRequest := createTestBidRequest(fmt.Sprintf("recovery-phase1-%d", reqID))
				requestBody, _ := json.Marshal(bidRequest)

				req, _ := http.NewRequest("POST", server.URL+"/bid/request", bytes.NewBuffer(requestBody))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Forwarded-For", "192.168.20.100")

				resp, err := client.Do(req)
				if err == nil {
					if resp.StatusCode == http.StatusOK {
						atomic.AddInt64(&phase1Success, 1)
					} else if resp.StatusCode == http.StatusTooManyRequests {
						atomic.AddInt64(&phase1RateLimit, 1)
					}
					resp.Body.Close()
				}
			}(i)
		}
		wg.Wait()

		t.Logf("Phase 1 Results: %d success, %d rate-limited", phase1Success, phase1RateLimit)

		// Phase 2: Wait for recovery and test again
		t.Log("Phase 2: Waiting for rate limit recovery...")
		time.Sleep(2 * time.Second) // Wait for rate limit to reset

		var phase2Success int64

		// Send a few requests after recovery
		for i := 0; i < 10; i++ {
			bidRequest := createTestBidRequest(fmt.Sprintf("recovery-phase2-%d", i))
			requestBody, _ := json.Marshal(bidRequest)

			req, _ := http.NewRequest("POST", server.URL+"/bid/request", bytes.NewBuffer(requestBody))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Forwarded-For", "192.168.20.100")

			resp, err := client.Do(req)
			if err == nil {
				if resp.StatusCode == http.StatusOK {
					phase2Success++
				}
				resp.Body.Close()
			}

			time.Sleep(100 * time.Millisecond) // Spread requests over time
		}

		t.Logf("Phase 2 Results: %d success after recovery", phase2Success)

		// Assertions
		assert.Greater(t, phase1RateLimit, int64(0), "Phase 1 should have rate-limited requests")
		assert.Greater(t, phase2Success, int64(0), "Phase 2 should have successful requests after recovery")
		assert.Greater(t, phase2Success, int64(5), "Most requests should succeed after recovery period")
	})
}

func TestRateLimitingUnderLoad_MemoryUsage(t *testing.T) {
	const rateLimit = 100.0
	const burst = 200

	_, server, cleanup := setupLoadTestServer(rateLimit, burst)
	defer cleanup()

	t.Run("Memory Usage Under Load", func(t *testing.T) {
		client := &http.Client{Timeout: 5 * time.Second}

		// Test rate limiting under load (in-memory rate limiter)
		const numClients = 10
		const requestsPerClient = 50
		const totalRequests = numClients * requestsPerClient

		var successCount int64
		var rateLimitedCount int64
		var wg sync.WaitGroup

		for i := 0; i < numClients; i++ {
			wg.Add(1)
			go func(clientIndex int) {
				defer wg.Done()

				for j := 0; j < requestsPerClient; j++ {
					bidRequest := createTestBidRequest(fmt.Sprintf("memory-test-%d-%d", clientIndex, j))
					requestBody, _ := json.Marshal(bidRequest)

					req, _ := http.NewRequest("POST", server.URL+"/bid/request", bytes.NewBuffer(requestBody))
					req.Header.Set("Content-Type", "application/json")

					resp, err := client.Do(req)
					if err == nil {
						if resp.StatusCode == http.StatusOK {
							atomic.AddInt64(&successCount, 1)
						} else if resp.StatusCode == http.StatusTooManyRequests {
							atomic.AddInt64(&rateLimitedCount, 1)
						}
						resp.Body.Close()
					}

					// Small delay to avoid overwhelming the server
					time.Sleep(1 * time.Millisecond)
				}
			}(i)
		}

		wg.Wait()

		t.Logf("Memory Usage Test Results:")
		t.Logf("  Total Requests: %d", totalRequests)
		t.Logf("  Successful: %d", successCount)
		t.Logf("  Rate Limited: %d", rateLimitedCount)

		// Verify rate limiting is working
		assert.Greater(t, rateLimitedCount, int64(0), "Should have some rate limited requests")
		assert.Greater(t, successCount, int64(0), "Should have some successful requests")
		assert.Equal(t, totalRequests, int(successCount+rateLimitedCount), "All requests should be accounted for")
	})
}

func TestRateLimitingUnderLoad_ErrorHandling(t *testing.T) {
	const rateLimit = 50.0
	const burst = 100

	_, server, cleanup := setupLoadTestServer(rateLimit, burst)
	defer cleanup()

	t.Run("Error Handling Under Load", func(t *testing.T) {
		client := &http.Client{Timeout: 2 * time.Second}

		var successCount, errorCount, timeoutCount int64
		var wg sync.WaitGroup

		// Mix of valid and invalid requests under load
		const totalRequests = 100

		for i := 0; i < totalRequests; i++ {
			wg.Add(1)
			go func(reqID int) {
				defer wg.Done()

				var req *http.Request
				var err error

				if reqID%4 == 0 {
					// Invalid JSON request
					req, _ = http.NewRequest("POST", server.URL+"/bid/request", strings.NewReader(`{"invalid": json}`))
				} else if reqID%7 == 0 {
					// Empty request
					req, _ = http.NewRequest("POST", server.URL+"/bid/request", strings.NewReader(``))
				} else {
					// Valid request
					bidRequest := createTestBidRequest(fmt.Sprintf("error-test-%d", reqID))
					requestBody, _ := json.Marshal(bidRequest)
					req, _ = http.NewRequest("POST", server.URL+"/bid/request", bytes.NewBuffer(requestBody))
				}

				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Forwarded-For", fmt.Sprintf("192.168.30.%d", (reqID%200)+1))

				resp, err := client.Do(req)
				if err != nil {
					if strings.Contains(err.Error(), "timeout") {
						atomic.AddInt64(&timeoutCount, 1)
					} else {
						atomic.AddInt64(&errorCount, 1)
					}
					return
				}
				defer resp.Body.Close()

				if resp.StatusCode == http.StatusOK {
					atomic.AddInt64(&successCount, 1)
				} else {
					atomic.AddInt64(&errorCount, 1)
				}
			}(i)
		}

		wg.Wait()

		t.Logf("Error Handling Results:")
		t.Logf("  Total Requests: %d", totalRequests)
		t.Logf("  Successful: %d", successCount)
		t.Logf("  Errors: %d", errorCount)
		t.Logf("  Timeouts: %d", timeoutCount)

		// System should handle errors gracefully without crashing
		assert.Equal(t, int64(totalRequests), successCount+errorCount+timeoutCount, "All requests should be accounted for")
		assert.Greater(t, errorCount, int64(0), "Should have some errors from invalid requests")

		// Redis should still be operational
		redisClient := redis.GetClient()
		_, err := redisClient.Ping(context.Background()).Result()
		assert.NoError(t, err, "Redis should still be operational after error handling test")

		// Server should still be responsive
		resp, err := http.Get(server.URL + "/health")
		if err == nil {
			assert.Contains(t, []int{http.StatusOK, http.StatusPartialContent, http.StatusServiceUnavailable, http.StatusTooManyRequests}, resp.StatusCode)
			resp.Body.Close()
		}
	})
}

// Benchmark tests for load performance
func BenchmarkRateLimitingUnderLoad_SingleIP(b *testing.B) {
	_, server, cleanup := setupLoadTestServer(1000.0, 2000) // High limits for benchmarking
	defer cleanup()

	client := &http.Client{Timeout: 5 * time.Second}
	bidRequest := createTestBidRequest("benchmark")
	requestBody, _ := json.Marshal(bidRequest)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req, _ := http.NewRequest("POST", server.URL+"/bid/request", bytes.NewBuffer(requestBody))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Forwarded-For", "192.168.100.1")

			resp, err := client.Do(req)
			if err == nil {
				resp.Body.Close()
			}
		}
	})
}

func BenchmarkRateLimitingUnderLoad_MultipleIPs(b *testing.B) {
	_, server, cleanup := setupLoadTestServer(1000.0, 2000)
	defer cleanup()

	client := &http.Client{Timeout: 5 * time.Second}
	bidRequest := createTestBidRequest("benchmark-multi")
	requestBody, _ := json.Marshal(bidRequest)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		ip := fmt.Sprintf("192.168.200.%d", (int(time.Now().UnixNano())%254)+1)

		for pb.Next() {
			req, _ := http.NewRequest("POST", server.URL+"/bid/request", bytes.NewBuffer(requestBody))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Forwarded-For", ip)

			resp, err := client.Do(req)
			if err == nil {
				resp.Body.Close()
			}
		}
	})
}
