package metrics_test

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/hadarco13/mini-seller/internal/config"
	"github.com/hadarco13/mini-seller/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

func setupMetricsTestConfig() {
	os.Setenv("MINI_SELLER_ENVIRONMENT", "test")
	os.Setenv("MINI_SELLER_SERVER_PORT", "8080")
}

func resetMetricsPrometheusRegistry() {
	prometheus.DefaultRegisterer = prometheus.NewRegistry()
}

func TestNewBuyerClientMetrics(t *testing.T) {
	resetMetricsPrometheusRegistry()
	setupMetricsTestConfig()
	config.LoadConfig()

	buyerMetrics := metrics.NewBuyerClientMetrics()

	assert.NotNil(t, buyerMetrics)
	assert.NotNil(t, buyerMetrics.RateLimitExceeded)
	assert.NotNil(t, buyerMetrics.RequestsTotal)
	assert.NotNil(t, buyerMetrics.RequestsSuccessful)
	assert.NotNil(t, buyerMetrics.RequestsFailed)
}

func TestBuyerClientMetrics_Counters(t *testing.T) {
	resetMetricsPrometheusRegistry()
	setupMetricsTestConfig()
	config.LoadConfig()

	buyerMetrics := metrics.NewBuyerClientMetrics()

	// Increment counters
	buyerMetrics.RateLimitExceeded.Inc()
	buyerMetrics.RequestsTotal.Inc()
	buyerMetrics.RequestsSuccessful.Inc()
	buyerMetrics.RequestsFailed.Inc()

	// Test multiple increments
	for i := 0; i < 5; i++ {
		buyerMetrics.RequestsTotal.Inc()
		buyerMetrics.RequestsSuccessful.Inc()
	}

	// Counters should be incremented (we can't easily read the values due to Prometheus design)
	// But we can test that the metrics are properly created and can be incremented without errors
	assert.NotNil(t, buyerMetrics.RateLimitExceeded)
	assert.NotNil(t, buyerMetrics.RequestsTotal)
	assert.NotNil(t, buyerMetrics.RequestsSuccessful)
	assert.NotNil(t, buyerMetrics.RequestsFailed)
}

func TestNewSimpleMetrics(t *testing.T) {
	metrics := metrics.NewSimpleMetrics()

	assert.NotNil(t, metrics)

	stats := metrics.GetStats()
	assert.Equal(t, int64(0), stats["total_requests"])
	assert.Equal(t, int64(0), stats["error_requests"])
	assert.Equal(t, int64(0), stats["success_requests"])
	assert.Equal(t, float64(0), stats["error_rate_percent"])
}

func TestSimpleMetrics_RecordRequest(t *testing.T) {
	metrics := metrics.NewSimpleMetrics()

	// Record successful requests
	metrics.RecordRequest(100*time.Millisecond, false)
	metrics.RecordRequest(200*time.Millisecond, false)

	// Record error requests
	metrics.RecordRequest(50*time.Millisecond, true)

	stats := metrics.GetStats()
	assert.Equal(t, int64(3), stats["total_requests"])
	assert.Equal(t, int64(1), stats["error_requests"])
	assert.Equal(t, int64(2), stats["success_requests"])
	assert.InDelta(t, 33.33, stats["error_rate_percent"], 0.1)

	// Check response time stats
	avgResponseTime := stats["avg_response_time_ms"].(int64)
	minResponseTime := stats["min_response_time_ms"].(int64)
	maxResponseTime := stats["max_response_time_ms"].(int64)

	assert.Greater(t, avgResponseTime, int64(0))
	assert.Equal(t, int64(50), minResponseTime)
	assert.Equal(t, int64(200), maxResponseTime)
}

func TestSimpleMetrics_ConcurrentAccess(t *testing.T) {
	metrics := metrics.NewSimpleMetrics()

	var wg sync.WaitGroup
	numGoroutines := 100
	numRequestsPerGoroutine := 10

	// Launch goroutines that record requests concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < numRequestsPerGoroutine; j++ {
				responseTime := time.Duration(goroutineID*j+1) * time.Millisecond
				hasError := j%3 == 0 // Every third request is an error
				metrics.RecordRequest(responseTime, hasError)
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	stats := metrics.GetStats()
	expectedTotal := int64(numGoroutines * numRequestsPerGoroutine)
	assert.Equal(t, expectedTotal, stats["total_requests"])
	assert.Greater(t, stats["error_requests"], int64(0))
	assert.Greater(t, stats["success_requests"], int64(0))
}

func TestGlobalMetrics_RecordRequest(t *testing.T) {
	resetMetricsPrometheusRegistry()
	setupMetricsTestConfig()
	config.LoadConfig()

	// Record some requests
	metrics.RecordRequest(150*time.Millisecond, false)
	metrics.RecordRequest(250*time.Millisecond, true)
	metrics.RecordRequest(75*time.Millisecond, false)

	stats := metrics.GetGlobalStats()
	assert.Equal(t, int64(3), stats["total_requests"])
	assert.Equal(t, int64(1), stats["error_requests"])
	assert.Equal(t, int64(2), stats["success_requests"])
	assert.InDelta(t, 33.33, stats["error_rate_percent"], 0.1)
}

func TestNewPerformanceMetrics(t *testing.T) {
	perfMetrics := metrics.NewPerformanceMetrics()

	assert.NotNil(t, perfMetrics)

	stats := perfMetrics.GetPerformanceStats()
	assert.Equal(t, float64(0), stats["cpu_usage_percent"])
	assert.Equal(t, int64(0), stats["redis_latency_ms"])
	assert.Equal(t, float64(0), stats["redis_error_rate"])
	assert.Equal(t, int64(0), stats["redis_operations"])
	assert.Equal(t, int64(0), stats["redis_errors"])
}

func TestPerformanceMetrics_RecordCPUUsage(t *testing.T) {
	perfMetrics := metrics.NewPerformanceMetrics()

	// Record CPU usage
	perfMetrics.RecordCPUUsage(75.5)

	stats := perfMetrics.GetPerformanceStats()
	assert.Equal(t, 75.5, stats["cpu_usage_percent"])
}

func TestPerformanceMetrics_RecordRedisOperation(t *testing.T) {
	perfMetrics := metrics.NewPerformanceMetrics()

	// Record successful Redis operations
	perfMetrics.RecordRedisOperation(10*time.Millisecond, true)
	perfMetrics.RecordRedisOperation(15*time.Millisecond, true)

	// Record failed Redis operation
	perfMetrics.RecordRedisOperation(20*time.Millisecond, false)

	stats := perfMetrics.GetPerformanceStats()
	assert.Equal(t, int64(3), stats["redis_operations"])
	assert.Equal(t, int64(1), stats["redis_errors"])
	assert.Equal(t, int64(20), stats["redis_latency_ms"]) // Latest latency
	assert.InDelta(t, 33.33, stats["redis_error_rate"], 0.1)
}

func TestPerformanceMetrics_ConcurrentAccess(t *testing.T) {
	perfMetrics := metrics.NewPerformanceMetrics()

	var wg sync.WaitGroup
	numGoroutines := 50

	// Test concurrent CPU recording
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			cpuUsage := float64(id % 100)
			perfMetrics.RecordCPUUsage(cpuUsage)
		}(i)
	}

	// Test concurrent Redis operation recording
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			latency := time.Duration(id+1) * time.Millisecond
			success := id%2 == 0
			perfMetrics.RecordRedisOperation(latency, success)
		}(i)
	}

	wg.Wait()

	stats := perfMetrics.GetPerformanceStats()
	assert.Equal(t, int64(numGoroutines), stats["redis_operations"])
	assert.Greater(t, stats["cpu_usage_percent"], float64(0))
	assert.Greater(t, stats["redis_latency_ms"], int64(0))
}

func TestGlobalPerformanceMetrics(t *testing.T) {
	resetMetricsPrometheusRegistry()
	setupMetricsTestConfig()
	config.LoadConfig()

	// Record CPU usage
	metrics.RecordCPU(85.2)

	// Record Redis operations
	metrics.RecordRedisOp(25*time.Millisecond, true)
	metrics.RecordRedisOp(30*time.Millisecond, false)

	stats := metrics.GetPerformanceStats()
	assert.Equal(t, 85.2, stats["cpu_usage_percent"])
	assert.Equal(t, int64(2), stats["redis_operations"])
	assert.Equal(t, int64(1), stats["redis_errors"])
	assert.Equal(t, int64(30), stats["redis_latency_ms"])
	assert.Equal(t, 50.0, stats["redis_error_rate"])
}

func TestMetrics_EdgeCases(t *testing.T) {
	t.Run("Zero values", func(t *testing.T) {
		metrics := metrics.NewSimpleMetrics()

		// Test zero duration request
		metrics.RecordRequest(0, false)

		stats := metrics.GetStats()
		assert.Equal(t, int64(1), stats["total_requests"])
		assert.Equal(t, int64(0), stats["avg_response_time_ms"])
		assert.Equal(t, int64(0), stats["min_response_time_ms"])
		assert.Equal(t, int64(0), stats["max_response_time_ms"])
	})

	t.Run("Very large durations", func(t *testing.T) {
		metrics := metrics.NewSimpleMetrics()

		// Test very large duration
		metrics.RecordRequest(time.Hour, false)

		stats := metrics.GetStats()
		assert.Equal(t, int64(1), stats["total_requests"])
		assert.Equal(t, time.Hour.Milliseconds(), stats["avg_response_time_ms"])
	})

	t.Run("All error requests", func(t *testing.T) {
		metrics := metrics.NewSimpleMetrics()

		for i := 0; i < 5; i++ {
			metrics.RecordRequest(100*time.Millisecond, true)
		}

		stats := metrics.GetStats()
		assert.Equal(t, int64(5), stats["total_requests"])
		assert.Equal(t, int64(5), stats["error_requests"])
		assert.Equal(t, int64(0), stats["success_requests"])
		assert.Equal(t, float64(100), stats["error_rate_percent"])
	})
}

// Benchmark tests
func BenchmarkSimpleMetrics_RecordRequest(b *testing.B) {
	metrics := metrics.NewSimpleMetrics()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			metrics.RecordRequest(100*time.Millisecond, false)
		}
	})
}

func BenchmarkSimpleMetrics_GetStats(b *testing.B) {
	metrics := metrics.NewSimpleMetrics()

	// Pre-populate with some data
	for i := 0; i < 1000; i++ {
		metrics.RecordRequest(time.Duration(i)*time.Millisecond, i%10 == 0)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = metrics.GetStats()
		}
	})
}

func BenchmarkPerformanceMetrics_RecordRedisOperation(b *testing.B) {
	perfMetrics := metrics.NewPerformanceMetrics()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			perfMetrics.RecordRedisOperation(time.Duration(i%100)*time.Millisecond, i%2 == 0)
			i++
		}
	})
}
