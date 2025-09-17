package buyers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/bsm/openrtb"
	"github.com/hadarco13/mini-seller/internal/buyers"
	"github.com/hadarco13/mini-seller/internal/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getTestLogger() *logrus.Entry {
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel) // Reduce log noise in tests
	return logger.WithField("test", true)
}

func setupTestConfig() {
	// Set minimal environment for testing
	os.Setenv("MINI_SELLER_ENVIRONMENT", "test")
	os.Setenv("MINI_SELLER_SERVER_PORT", "8080")
}

func resetPrometheusRegistry() {
	// Create a new registry to avoid metric collision between tests
	prometheus.DefaultRegisterer = prometheus.NewRegistry()
}

func TestNewBuyerClient(t *testing.T) {
	resetPrometheusRegistry()
	setupTestConfig()
	config.LoadConfig() // Ignore error for testing

	endpoint := "http://example.com/bid"
	qps := 10.0
	burst := 20
	timeout := 1 * time.Second

	client := buyers.NewBuyerClient(endpoint, qps, burst, timeout)

	assert.NotNil(t, client)
}

func TestBuyerClient_SendBidRequest_Success(t *testing.T) {
	resetPrometheusRegistry()
	setupTestConfig()
	config.LoadConfig() // Ignore error for testing

	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and content type
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Create mock bid response
		bidResponse := openrtb.BidResponse{
			ID:       "test-response-123",
			Currency: "USD",
			SeatBid: []openrtb.SeatBid{
				{
					Bid: []openrtb.Bid{
						{
							ID:    "bid-1",
							ImpID: "imp-1",
							Price: 2.5,
						},
					},
					Seat: "test-seat",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(bidResponse)
	}))
	defer server.Close()

	// Create buyer client
	client := buyers.NewBuyerClient(server.URL, 10.0, 20, 1*time.Second)

	// Create test bid request
	bidRequest := &openrtb.BidRequest{
		ID: "test-request-123",
		Imp: []openrtb.Impression{
			{
				ID: "imp-1",
				Banner: &openrtb.Banner{
					W: 300,
					H: 250,
				},
				BidFloor: 1.0,
			},
		},
	}

	ctx := context.Background()

	// Send bid request
	response, err := client.SendBidRequest(ctx, getTestLogger(), bidRequest)

	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, "test-response-123", response.ID)
	assert.Equal(t, "USD", response.Currency)
	assert.Len(t, response.SeatBid, 1)
	assert.Len(t, response.SeatBid[0].Bid, 1)
	assert.Equal(t, 2.5, response.SeatBid[0].Bid[0].Price)
}

func TestBuyerClient_SendBidRequest_HTTPError(t *testing.T) {
	resetPrometheusRegistry()
	setupTestConfig()
	config.LoadConfig()

	// Create mock server that returns HTTP error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	client := buyers.NewBuyerClient(server.URL, 10.0, 20, 1*time.Second)

	bidRequest := &openrtb.BidRequest{
		ID: "test-request-123",
		Imp: []openrtb.Impression{
			{
				ID: "imp-1",
				Banner: &openrtb.Banner{
					W: 300,
					H: 250,
				},
			},
		},
	}

	ctx := context.Background()

	response, err := client.SendBidRequest(ctx, getTestLogger(), bidRequest)

	assert.Error(t, err)
	assert.Nil(t, response)
}

func TestBuyerClient_SendBidRequest_InvalidJSON(t *testing.T) {
	resetPrometheusRegistry()
	setupTestConfig()
	config.LoadConfig()

	// Create mock server that returns invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	client := buyers.NewBuyerClient(server.URL, 10.0, 20, 1*time.Second)

	bidRequest := &openrtb.BidRequest{
		ID: "test-request-123",
		Imp: []openrtb.Impression{
			{
				ID: "imp-1",
				Banner: &openrtb.Banner{
					W: 300,
					H: 250,
				},
			},
		},
	}

	ctx := context.Background()

	response, err := client.SendBidRequest(ctx, getTestLogger(), bidRequest)

	assert.Error(t, err)
	assert.Nil(t, response)
}

func TestBuyerClient_SendBidRequest_Timeout(t *testing.T) {
	resetPrometheusRegistry()
	setupTestConfig()
	config.LoadConfig()

	// Create mock server with delay
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond) // Delay longer than client timeout
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create client with short timeout
	client := buyers.NewBuyerClient(server.URL, 10.0, 20, 100*time.Millisecond)

	bidRequest := &openrtb.BidRequest{
		ID: "test-request-123",
		Imp: []openrtb.Impression{
			{
				ID: "imp-1",
				Banner: &openrtb.Banner{
					W: 300,
					H: 250,
				},
			},
		},
	}

	ctx := context.Background()

	response, err := client.SendBidRequest(ctx, getTestLogger(), bidRequest)

	assert.Error(t, err)
	assert.Nil(t, response)
}

func TestBuyerClient_SendBidRequest_ContextCancellation(t *testing.T) {
	resetPrometheusRegistry()
	setupTestConfig()
	config.LoadConfig()

	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := buyers.NewBuyerClient(server.URL, 10.0, 20, 1*time.Second)

	bidRequest := &openrtb.BidRequest{
		ID: "test-request-123",
		Imp: []openrtb.Impression{
			{
				ID: "imp-1",
				Banner: &openrtb.Banner{
					W: 300,
					H: 250,
				},
			},
		},
	}

	// Create context that cancels quickly
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	response, err := client.SendBidRequest(ctx, getTestLogger(), bidRequest)

	assert.Error(t, err)
	assert.Nil(t, response)
}

func TestBuyerClient_SendBidRequest_EmptyResponse(t *testing.T) {
	resetPrometheusRegistry()
	setupTestConfig()
	config.LoadConfig()

	// Create mock server that returns no bids
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bidResponse := openrtb.BidResponse{
			ID:       "test-response-123",
			Currency: "USD",
			SeatBid:  []openrtb.SeatBid{}, // No bids
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(bidResponse)
	}))
	defer server.Close()

	client := buyers.NewBuyerClient(server.URL, 10.0, 20, 1*time.Second)

	bidRequest := &openrtb.BidRequest{
		ID: "test-request-123",
		Imp: []openrtb.Impression{
			{
				ID: "imp-1",
				Banner: &openrtb.Banner{
					W: 300,
					H: 250,
				},
			},
		},
	}

	ctx := context.Background()

	response, err := client.SendBidRequest(ctx, getTestLogger(), bidRequest)

	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, "test-response-123", response.ID)
	assert.Len(t, response.SeatBid, 0) // No seat bids
}

func TestBuyerClient_GetEndpoint(t *testing.T) {
	resetPrometheusRegistry()
	setupTestConfig()
	config.LoadConfig()

	endpoint := "https://buyer.example.com/bid"
	client := buyers.NewBuyerClient(endpoint, 5.0, 10, time.Second*5)

	assert.Equal(t, endpoint, client.GetEndpoint())
}

func TestBuyerClient_SendBidRequest_LargePayload(t *testing.T) {
	resetPrometheusRegistry()
	setupTestConfig()
	config.LoadConfig()

	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify we can handle large requests
		var receivedRequest openrtb.BidRequest
		err := json.NewDecoder(r.Body).Decode(&receivedRequest)
		require.NoError(t, err)

		assert.Len(t, receivedRequest.Imp, 100) // Should receive all impressions

		bidResponse := openrtb.BidResponse{
			ID:       "test-response-123",
			Currency: "USD",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(bidResponse)
	}))
	defer server.Close()

	client := buyers.NewBuyerClient(server.URL, 10.0, 20, 5*time.Second) // Longer timeout for large payload

	// Create bid request with many impressions
	impressions := make([]openrtb.Impression, 100)
	for i := 0; i < 100; i++ {
		impressions[i] = openrtb.Impression{
			ID: fmt.Sprintf("imp-%d", i),
			Banner: &openrtb.Banner{
				W: 300,
				H: 250,
			},
			BidFloor: 1.0,
		}
	}

	bidRequest := &openrtb.BidRequest{
		ID:  "test-request-123",
		Imp: impressions,
	}

	ctx := context.Background()

	response, err := client.SendBidRequest(ctx, getTestLogger(), bidRequest)

	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, "test-response-123", response.ID)
}
