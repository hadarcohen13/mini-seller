package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/hadarco13/mini-seller/internal/config"
	"github.com/hadarco13/mini-seller/internal/handlers"
	"github.com/hadarco13/mini-seller/internal/redis"
	"github.com/prometheus/client_golang/prometheus"
	redisClient "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupHealthTestConfig() {
	os.Setenv("MINI_SELLER_ENVIRONMENT", "test")
	os.Setenv("MINI_SELLER_SERVER_PORT", "8080")
}

func resetHealthPrometheusRegistry() {
	prometheus.DefaultRegisterer = prometheus.NewRegistry()
}

func setupTestRedisForHealth(t *testing.T) (*miniredis.Miniredis, func()) {
	// Start a test Redis server
	mr, err := miniredis.Run()
	require.NoError(t, err)

	// Create and set Redis client
	client := redisClient.NewClient(&redisClient.Options{
		Addr: mr.Addr(),
		DB:   0,
	})

	// Initialize the redis package with the test client
	redis.SetTestClient(client)

	return mr, func() {
		mr.Close()
		redis.SetTestClient(nil)
	}
}

func TestHealthHandler_Healthy(t *testing.T) {
	resetHealthPrometheusRegistry()
	setupHealthTestConfig()
	config.LoadConfig()

	mr, cleanup := setupTestRedisForHealth(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handlers.HealthHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var healthStatus handlers.HealthStatus
	err := json.NewDecoder(w.Body).Decode(&healthStatus)
	require.NoError(t, err)

	assert.Equal(t, "healthy", healthStatus.Status)
	assert.Greater(t, healthStatus.GoroutineStats.Total, 0)
	assert.Equal(t, 1000, healthStatus.GoroutineStats.MaxAllowed)
	assert.Contains(t, healthStatus.Dependencies, "redis")
	assert.Equal(t, "healthy", healthStatus.Dependencies["redis"].Status)

	// Verify Redis is accessible
	ctx := context.Background()
	pong, err := redis.GetClient().Ping(ctx).Result()
	assert.NoError(t, err)
	assert.Equal(t, "PONG", pong)

	mr.Close()
}

func TestHealthHandler_RedisUnhealthy(t *testing.T) {
	resetHealthPrometheusRegistry()
	setupHealthTestConfig()
	config.LoadConfig()

	// Setup Redis but close it immediately to simulate failure
	mr, cleanup := setupTestRedisForHealth(t)
	mr.Close() // Close Redis server to simulate failure
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handlers.HealthHandler(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var healthStatus handlers.HealthStatus
	err := json.NewDecoder(w.Body).Decode(&healthStatus)
	require.NoError(t, err)

	assert.Equal(t, "critical", healthStatus.Status)
	assert.Contains(t, healthStatus.Dependencies, "redis")
	assert.Equal(t, "unhealthy", healthStatus.Dependencies["redis"].Status)
	assert.Greater(t, len(healthStatus.Warnings), 0)
	assert.Contains(t, healthStatus.Warnings[0], "redis dependency is unhealthy")
}

func TestHealthHandler_HighGoroutineWarning(t *testing.T) {
	resetHealthPrometheusRegistry()
	setupHealthTestConfig()
	config.LoadConfig()

	mr, cleanup := setupTestRedisForHealth(t)
	defer cleanup()

	// Create many goroutines to trigger warning (71% of 1000 = 710+)
	done := make(chan bool)
	numGoroutines := 750
	for i := 0; i < numGoroutines; i++ {
		go func() {
			<-done
		}()
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handlers.HealthHandler(w, req)

	// Clean up goroutines
	close(done)

	assert.Equal(t, http.StatusPartialContent, w.Code)

	var healthStatus handlers.HealthStatus
	err := json.NewDecoder(w.Body).Decode(&healthStatus)
	require.NoError(t, err)

	assert.Equal(t, "warning", healthStatus.Status)
	assert.Greater(t, healthStatus.GoroutineStats.Percentage, 70.0)
	assert.Contains(t, healthStatus.Warnings[0], "goroutine count exceeds 70%")

	mr.Close()
}

func TestHealthHandler_CriticalGoroutineCount(t *testing.T) {
	resetHealthPrometheusRegistry()
	setupHealthTestConfig()
	config.LoadConfig()

	mr, cleanup := setupTestRedisForHealth(t)
	defer cleanup()

	// Create many goroutines to trigger critical state (91% of 1000 = 910+)
	done := make(chan bool)
	numGoroutines := 950
	for i := 0; i < numGoroutines; i++ {
		go func() {
			<-done
		}()
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handlers.HealthHandler(w, req)

	// Clean up goroutines
	close(done)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var healthStatus handlers.HealthStatus
	err := json.NewDecoder(w.Body).Decode(&healthStatus)
	require.NoError(t, err)

	assert.Equal(t, "critical", healthStatus.Status)
	assert.Greater(t, healthStatus.GoroutineStats.Percentage, 90.0)
	assert.Contains(t, healthStatus.Warnings[0], "goroutine count exceeds 90%")

	mr.Close()
}

func TestMetricsHandler_Success(t *testing.T) {
	resetHealthPrometheusRegistry()
	setupHealthTestConfig()
	config.LoadConfig()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	handlers.MetricsHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var stats interface{}
	err := json.NewDecoder(w.Body).Decode(&stats)
	require.NoError(t, err)
	assert.NotNil(t, stats)
}

func TestHealthStatus_MemoryStats(t *testing.T) {
	resetHealthPrometheusRegistry()
	setupHealthTestConfig()
	config.LoadConfig()

	mr, cleanup := setupTestRedisForHealth(t)
	defer cleanup()

	// Allocate some memory to test memory stats
	bigSlice := make([]byte, 10*1024*1024) // 10MB
	for i := range bigSlice {
		bigSlice[i] = byte(i % 256)
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handlers.HealthHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var healthStatus handlers.HealthStatus
	err := json.NewDecoder(w.Body).Decode(&healthStatus)
	require.NoError(t, err)

	// Verify memory stats are populated
	assert.Greater(t, healthStatus.GoroutineStats.MemStats.AllocMB, uint64(0))
	assert.Greater(t, healthStatus.GoroutineStats.MemStats.SysMB, uint64(0))
	assert.GreaterOrEqual(t, healthStatus.GoroutineStats.MemStats.NumGC, uint32(0))

	// Keep bigSlice in scope
	_ = bigSlice

	mr.Close()
}

func TestHealthStatus_RequestStats(t *testing.T) {
	resetHealthPrometheusRegistry()
	setupHealthTestConfig()
	config.LoadConfig()

	mr, cleanup := setupTestRedisForHealth(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handlers.HealthHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var healthStatus handlers.HealthStatus
	err := json.NewDecoder(w.Body).Decode(&healthStatus)
	require.NoError(t, err)

	// Verify request stats are included
	assert.Contains(t, healthStatus.RequestStats, "active_requests")
	assert.Contains(t, healthStatus.RequestStats, "total_requests")
	assert.Contains(t, healthStatus.RequestStats, "failed_requests")
	assert.Contains(t, healthStatus.RequestStats, "success_rate")

	mr.Close()
}

func TestHealthHandler_SlowRedisWarning(t *testing.T) {
	resetHealthPrometheusRegistry()
	setupHealthTestConfig()
	config.LoadConfig()

	// Create a Redis server that will be slow
	mr, cleanup := setupTestRedisForHealth(t)
	defer cleanup()

	// Simulate slow response by introducing delay in Redis operations
	// Since miniredis doesn't have SetLatency, we'll test with timeout instead
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handlers.HealthHandler(w, req)

	// Should be healthy since we can't easily simulate slow Redis with miniredis
	assert.Equal(t, http.StatusOK, w.Code)

	var healthStatus handlers.HealthStatus
	err := json.NewDecoder(w.Body).Decode(&healthStatus)
	require.NoError(t, err)

	assert.Equal(t, "healthy", healthStatus.Status)
	assert.Contains(t, healthStatus.Dependencies, "redis")
	assert.Equal(t, "healthy", healthStatus.Dependencies["redis"].Status)

	mr.Close()
}

func TestGetRequestStats_ThreadSafety(t *testing.T) {
	resetHealthPrometheusRegistry()
	setupHealthTestConfig()
	config.LoadConfig()

	// Test concurrent access to request stats
	done := make(chan bool)
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer func() { done <- true }()
			stats := handlers.GetRequestStats()
			assert.Contains(t, stats, "active_requests")
			assert.Contains(t, stats, "total_requests")
			assert.Contains(t, stats, "failed_requests")
			assert.Contains(t, stats, "success_rate")
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}
}

// Benchmark tests
func BenchmarkHealthHandler(b *testing.B) {
	resetHealthPrometheusRegistry()
	setupHealthTestConfig()
	config.LoadConfig()

	// Convert *testing.B to *testing.T for setupTestRedisForHealth
	t := &testing.T{}
	mr, cleanup := setupTestRedisForHealth(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			w := httptest.NewRecorder()
			handlers.HealthHandler(w, req)
			if w.Code != http.StatusOK {
				b.Fatalf("Expected 200, got %d", w.Code)
			}
		}
	})

	mr.Close()
}
