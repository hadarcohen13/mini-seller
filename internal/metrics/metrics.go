package metrics

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// BuyerClientMetrics holds all the metrics for our outgoing buyer client.
type BuyerClientMetrics struct {
	// A Counter is a cumulative metric that only goes up. Perfect for counting events.
	RateLimitExceeded  prometheus.Counter
	RequestsTotal      prometheus.Counter
	RequestsSuccessful prometheus.Counter
	RequestsFailed     prometheus.Counter
}

// NewBuyerClientMetrics initializes and registers our Prometheus metrics.
func NewBuyerClientMetrics() *BuyerClientMetrics {
	return &BuyerClientMetrics{
		RateLimitExceeded: promauto.NewCounter(prometheus.CounterOpts{
			Name: "buyer_rate_limit_exceeded_total",
			Help: "The total number of times the outgoing rate limit was exceeded.",
		}),
		RequestsTotal: promauto.NewCounter(prometheus.CounterOpts{
			Name: "buyer_requests_total",
			Help: "The total number of requests sent to buyers.",
		}),
		RequestsSuccessful: promauto.NewCounter(prometheus.CounterOpts{
			Name: "buyer_requests_successful_total",
			Help: "The total number of successful requests to buyers.",
		}),
		RequestsFailed: promauto.NewCounter(prometheus.CounterOpts{
			Name: "buyer_requests_failed_total",
			Help: "The total number of failed requests to buyers.",
		}),
	}
}

// SimpleMetrics tracks basic application metrics
type SimpleMetrics struct {
	mu sync.RWMutex

	// Request counts
	totalRequests int64
	errorRequests int64

	// Response times
	totalResponseTime time.Duration
	minResponseTime   time.Duration
	maxResponseTime   time.Duration
}

// NewSimpleMetrics creates a new simple metrics tracker
func NewSimpleMetrics() *SimpleMetrics {
	return &SimpleMetrics{
		minResponseTime: time.Hour, // Start with high value
	}
}

// RecordRequest records a request with response time and error
func (sm *SimpleMetrics) RecordRequest(responseTime time.Duration, hasError bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.totalRequests++
	sm.totalResponseTime += responseTime

	// Track min/max response times
	if responseTime < sm.minResponseTime {
		sm.minResponseTime = responseTime
	}
	if responseTime > sm.maxResponseTime {
		sm.maxResponseTime = responseTime
	}

	if hasError {
		sm.errorRequests++
	}
}

// GetStats returns current metrics
func (sm *SimpleMetrics) GetStats() map[string]interface{} {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	avgResponseTime := time.Duration(0)
	errorRate := float64(0)

	if sm.totalRequests > 0 {
		avgResponseTime = sm.totalResponseTime / time.Duration(sm.totalRequests)
		errorRate = float64(sm.errorRequests) / float64(sm.totalRequests) * 100
	}

	return map[string]interface{}{
		"total_requests":       sm.totalRequests,
		"error_requests":       sm.errorRequests,
		"success_requests":     sm.totalRequests - sm.errorRequests,
		"error_rate_percent":   errorRate,
		"avg_response_time_ms": avgResponseTime.Milliseconds(),
		"min_response_time_ms": sm.minResponseTime.Milliseconds(),
		"max_response_time_ms": sm.maxResponseTime.Milliseconds(),
	}
}

// Global metrics instance
var globalMetrics = NewSimpleMetrics()

// RecordRequest records a request globally
func RecordRequest(responseTime time.Duration, hasError bool) {
	globalMetrics.RecordRequest(responseTime, hasError)
}

// GetGlobalStats returns global metrics
func GetGlobalStats() map[string]interface{} {
	return globalMetrics.GetStats()
}
