package metrics

import (
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
