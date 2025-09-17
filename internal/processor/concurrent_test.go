package processor

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/bsm/openrtb"
	"github.com/hadarco13/mini-seller/internal/buyers"
	"github.com/hadarco13/mini-seller/internal/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

func setupProcessorTestConfig() {
	os.Setenv("MINI_SELLER_ENVIRONMENT", "test")
	os.Setenv("MINI_SELLER_SERVER_PORT", "8080")
}

func resetProcessorPrometheusRegistry() {
	prometheus.DefaultRegisterer = prometheus.NewRegistry()
}

func TestNewBidProcessor(t *testing.T) {
	resetProcessorPrometheusRegistry()
	setupProcessorTestConfig()
	config.LoadConfig()

	buyerClients := []*buyers.BuyerClient{
		buyers.NewBuyerClient("http://buyer1.com", 10.0, 20, time.Second),
	}

	processor := NewBidProcessor(buyerClients, 5)
	assert.NotNil(t, processor)
}

func TestNewBidProcessor_InvalidWorkers(t *testing.T) {
	resetProcessorPrometheusRegistry()
	setupProcessorTestConfig()
	config.LoadConfig()

	buyerClients := []*buyers.BuyerClient{
		buyers.NewBuyerClient("http://buyer1.com", 10.0, 20, time.Second),
	}

	// Test with 0 workers - should default to 10
	processor := NewBidProcessor(buyerClients, 0)
	assert.NotNil(t, processor)

	// Test with negative workers - should default to 10
	processor2 := NewBidProcessor(buyerClients, -5)
	assert.NotNil(t, processor2)
}

func TestBidProcessor_StartStop(t *testing.T) {
	resetProcessorPrometheusRegistry()
	setupProcessorTestConfig()
	config.LoadConfig()

	buyerClients := []*buyers.BuyerClient{
		buyers.NewBuyerClient("http://buyer1.com", 10.0, 20, time.Second),
	}

	processor := NewBidProcessor(buyerClients, 2)

	// Start the processor
	processor.Start()

	// Stop the processor
	processor.Stop()

	// Should not hang or panic
}

func TestBidProcessor_ProcessBidRequest_NoBuyers(t *testing.T) {
	resetProcessorPrometheusRegistry()
	setupProcessorTestConfig()
	config.LoadConfig()

	processor := NewBidProcessor([]*buyers.BuyerClient{}, 2)

	bidRequest := &openrtb.BidRequest{
		ID: "test-request-1",
		Imp: []openrtb.Impression{
			{ID: "imp-1"},
		},
	}

	ctx := context.Background()
	results, err := processor.ProcessBidRequest(ctx, bidRequest, "test-request-1")

	assert.NoError(t, err)
	assert.Nil(t, results)
}

func TestBidProcessor_ProcessBidRequest_WithTimeout(t *testing.T) {
	resetProcessorPrometheusRegistry()
	setupProcessorTestConfig()
	config.LoadConfig()

	// Create buyer clients that will timeout
	buyerClients := []*buyers.BuyerClient{
		buyers.NewBuyerClient("http://non-existent-buyer1.com", 10.0, 20, 50*time.Millisecond),
	}

	processor := NewBidProcessor(buyerClients, 2)

	bidRequest := &openrtb.BidRequest{
		ID: "test-request-timeout",
		Imp: []openrtb.Impression{
			{ID: "imp-1"},
		},
	}

	// Short context timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	results, err := processor.ProcessBidRequest(ctx, bidRequest, "test-request-timeout")

	assert.NoError(t, err)
	assert.Len(t, results, 1)

	// Request should have error due to timeout
	for _, result := range results {
		assert.NotNil(t, result)
		if result != nil {
			assert.Error(t, result.Error)
			assert.Nil(t, result.Response)
			assert.NotEmpty(t, result.BuyerEndpoint)
		}
	}
}

func TestBidProcessor_SubmitBidRequest(t *testing.T) {
	resetProcessorPrometheusRegistry()
	setupProcessorTestConfig()
	config.LoadConfig()

	buyerClients := []*buyers.BuyerClient{
		buyers.NewBuyerClient("http://buyer1.com", 10.0, 20, 100*time.Millisecond),
	}

	processor := NewBidProcessor(buyerClients, 2)
	processor.Start()
	defer processor.Stop()

	bidRequest := &openrtb.BidRequest{
		ID: "submit-test-1",
		Imp: []openrtb.Impression{
			{ID: "imp-1"},
		},
	}

	responseCh := processor.SubmitBidRequest(bidRequest, "submit-test-1")

	select {
	case result := <-responseCh:
		// Should get a result (likely an error since buyer doesn't exist)
		assert.NotNil(t, result)
	case <-time.After(3 * time.Second):
		t.Fatal("Did not receive response within timeout")
	}
}

func TestBidProcessor_SubmitBidRequest_QueueFull(t *testing.T) {
	resetProcessorPrometheusRegistry()
	setupProcessorTestConfig()
	config.LoadConfig()

	buyerClients := []*buyers.BuyerClient{
		buyers.NewBuyerClient("http://buyer1.com", 10.0, 20, time.Second),
	}

	// Create processor with very small queue
	processor := NewBidProcessor(buyerClients, 1)
	// Don't start the processor so queue gets full

	bidRequest := &openrtb.BidRequest{
		ID: "queue-full-test",
		Imp: []openrtb.Impression{
			{ID: "imp-1"},
		},
	}

	// Fill up the queue
	for i := 0; i < 10; i++ {
		responseCh := processor.SubmitBidRequest(bidRequest, "queue-full-test")

		select {
		case result := <-responseCh:
			if result.Error != nil && result.Error.Error() == "processor queue full" {
				// Expected behavior when queue is full
				break
			}
		case <-time.After(10 * time.Millisecond):
			// Queue might not be full yet
		}
	}
}

func TestProcessBidRequestsConcurrently(t *testing.T) {
	resetProcessorPrometheusRegistry()
	setupProcessorTestConfig()
	config.LoadConfig()

	buyerClients := []*buyers.BuyerClient{
		buyers.NewBuyerClient("http://buyer1.com", 10.0, 20, 50*time.Millisecond),
	}

	bidRequests := []*openrtb.BidRequest{
		{
			ID: "request-1",
			Imp: []openrtb.Impression{
				{ID: "imp-1"},
			},
		},
		{
			ID: "request-2",
			Imp: []openrtb.Impression{
				{ID: "imp-2"},
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	results := ProcessBidRequestsConcurrently(ctx, bidRequests, buyerClients)

	assert.Len(t, results, 2)
	assert.Contains(t, results, "request-1")
	assert.Contains(t, results, "request-2")

	// Each request should have results from buyer
	assert.Len(t, results["request-1"], 1)
	assert.Len(t, results["request-2"], 1)
}

func TestProcessBidRequestsConcurrently_EmptyRequests(t *testing.T) {
	resetProcessorPrometheusRegistry()
	setupProcessorTestConfig()
	config.LoadConfig()

	buyerClients := []*buyers.BuyerClient{
		buyers.NewBuyerClient("http://buyer1.com", 10.0, 20, time.Second),
	}

	bidRequests := []*openrtb.BidRequest{}

	ctx := context.Background()
	results := ProcessBidRequestsConcurrently(ctx, bidRequests, buyerClients)

	assert.Empty(t, results)
}

func TestProcessBidRequestsConcurrently_NoBuyers(t *testing.T) {
	resetProcessorPrometheusRegistry()
	setupProcessorTestConfig()
	config.LoadConfig()

	buyerClients := []*buyers.BuyerClient{}

	bidRequests := []*openrtb.BidRequest{
		{
			ID: "request-1",
			Imp: []openrtb.Impression{
				{ID: "imp-1"},
			},
		},
	}

	ctx := context.Background()
	results := ProcessBidRequestsConcurrently(ctx, bidRequests, buyerClients)

	assert.Len(t, results, 1)
	assert.Contains(t, results, "request-1")
	assert.Nil(t, results["request-1"])
}

// Integration test simulating real bid processing
func TestBidProcessor_IntegrationScenario(t *testing.T) {
	resetProcessorPrometheusRegistry()
	setupProcessorTestConfig()
	config.LoadConfig()

	buyerClients := []*buyers.BuyerClient{
		buyers.NewBuyerClient("http://buyer1.com", 5.0, 10, 100*time.Millisecond),
	}

	processor := NewBidProcessor(buyerClients, 3)
	processor.Start()
	defer processor.Stop()

	// Submit multiple bid requests
	const numRequests = 5
	responseChannels := make([]<-chan *BidResult, numRequests)

	for i := 0; i < numRequests; i++ {
		bidRequest := &openrtb.BidRequest{
			ID: fmt.Sprintf("integration-request-%d", i),
			Imp: []openrtb.Impression{
				{
					ID: fmt.Sprintf("imp-%d", i),
					Banner: &openrtb.Banner{
						W: 300,
						H: 250,
					},
				},
			},
		}

		responseChannels[i] = processor.SubmitBidRequest(bidRequest, bidRequest.ID)
	}

	// Wait for all responses
	for i, respCh := range responseChannels {
		select {
		case result := <-respCh:
			assert.NotNil(t, result, "Request %d should return a result", i)
			// Results will likely have errors since buyers don't exist, but should not be nil
		case <-time.After(1 * time.Second):
			t.Fatalf("Request %d did not complete within timeout", i)
		}
	}
}

// Benchmark tests
func BenchmarkBidProcessor_ProcessBidRequest(b *testing.B) {
	resetProcessorPrometheusRegistry()
	setupProcessorTestConfig()
	config.LoadConfig()

	buyerClients := []*buyers.BuyerClient{
		buyers.NewBuyerClient("http://buyer1.com", 10.0, 20, 50*time.Millisecond),
	}

	processor := NewBidProcessor(buyerClients, 5)

	bidRequest := &openrtb.BidRequest{
		ID: "benchmark-request",
		Imp: []openrtb.Impression{
			{ID: "imp-1"},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		processor.ProcessBidRequest(ctx, bidRequest, "benchmark-request")
	}
}

func BenchmarkProcessBidRequestsConcurrently(b *testing.B) {
	resetProcessorPrometheusRegistry()
	setupProcessorTestConfig()
	config.LoadConfig()

	buyerClients := []*buyers.BuyerClient{
		buyers.NewBuyerClient("http://buyer1.com", 10.0, 20, 20*time.Millisecond),
	}

	bidRequests := []*openrtb.BidRequest{
		{ID: "bench-request-1", Imp: []openrtb.Impression{{ID: "imp-1"}}},
		{ID: "bench-request-2", Imp: []openrtb.Impression{{ID: "imp-2"}}},
		{ID: "bench-request-3", Imp: []openrtb.Impression{{ID: "imp-3"}}},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ProcessBidRequestsConcurrently(ctx, bidRequests, buyerClients)
	}
}
