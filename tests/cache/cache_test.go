package cache_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/bsm/openrtb"
	"github.com/hadarco13/mini-seller/internal/cache"
	"github.com/hadarco13/mini-seller/internal/config"
	redisClient "github.com/hadarco13/mini-seller/internal/redis"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupCacheTestConfig() {
	os.Setenv("MINI_SELLER_ENVIRONMENT", "test")
	os.Setenv("MINI_SELLER_SERVER_PORT", "8080")
}

func resetCachePrometheusRegistry() {
	prometheus.DefaultRegisterer = prometheus.NewRegistry()
}

func setupCacheTestRedis(t *testing.T) (*miniredis.Miniredis, func()) {
	// Start miniredis
	mr := miniredis.NewMiniRedis()
	err := mr.Start()
	require.NoError(t, err)

	// Create Redis client pointing to miniredis
	testClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
		DB:   0,
	})

	// Set the test client
	redisClient.SetTestClient(testClient)

	cleanup := func() {
		testClient.Close()
		mr.Close()
	}

	return mr, cleanup
}

func TestSetBidRequest(t *testing.T) {
	resetCachePrometheusRegistry()
	setupCacheTestConfig()
	config.LoadConfig()

	_, cleanup := setupCacheTestRedis(t)
	defer cleanup()

	ctx := context.Background()
	requestID := "test-request-123"

	bidRequest := &openrtb.BidRequest{
		ID: requestID,
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

	err := cache.SetBidRequest(ctx, requestID, bidRequest)
	assert.NoError(t, err)

	// Verify it was stored
	client := redisClient.GetClient()
	key := "bid_request:" + requestID
	exists := client.Exists(ctx, key).Val()
	assert.Equal(t, int64(1), exists)
}

func TestGetBidRequest(t *testing.T) {
	resetCachePrometheusRegistry()
	setupCacheTestConfig()
	config.LoadConfig()

	_, cleanup := setupCacheTestRedis(t)
	defer cleanup()

	ctx := context.Background()
	requestID := "test-request-456"

	originalBidRequest := &openrtb.BidRequest{
		ID: requestID,
		Imp: []openrtb.Impression{
			{
				ID: "imp-2",
				Banner: &openrtb.Banner{
					W: 728,
					H: 90,
				},
			},
		},
	}

	// Store bid request
	err := cache.SetBidRequest(ctx, requestID, originalBidRequest)
	require.NoError(t, err)

	// Retrieve bid request
	retrievedBidRequest, err := cache.GetBidRequest(ctx, requestID)
	assert.NoError(t, err)
	assert.NotNil(t, retrievedBidRequest)
	assert.Equal(t, requestID, retrievedBidRequest.ID)
	assert.Len(t, retrievedBidRequest.Imp, 1)
	assert.Equal(t, "imp-2", retrievedBidRequest.Imp[0].ID)
}

func TestGetBidRequest_NotFound(t *testing.T) {
	resetCachePrometheusRegistry()
	setupCacheTestConfig()
	config.LoadConfig()

	_, cleanup := setupCacheTestRedis(t)
	defer cleanup()

	ctx := context.Background()
	requestID := "non-existent-request"

	retrievedBidRequest, err := cache.GetBidRequest(ctx, requestID)
	assert.Error(t, err)
	assert.Nil(t, retrievedBidRequest)
}

func TestSetBidResponse(t *testing.T) {
	resetCachePrometheusRegistry()
	setupCacheTestConfig()
	config.LoadConfig()

	_, cleanup := setupCacheTestRedis(t)
	defer cleanup()

	ctx := context.Background()
	requestID := "test-response-123"

	bidResponse := &openrtb.BidResponse{
		ID:       requestID,
		Currency: "USD",
		SeatBid: []openrtb.SeatBid{
			{
				Bid: []openrtb.Bid{
					{
						ID:    "bid-1",
						ImpID: "imp-1",
						Price: 1.25,
					},
				},
			},
		},
	}

	err := cache.SetBidResponse(ctx, requestID, bidResponse)
	assert.NoError(t, err)

	// Verify it was stored
	client := redisClient.GetClient()
	key := "bid_response:" + requestID
	exists := client.Exists(ctx, key).Val()
	assert.Equal(t, int64(1), exists)
}

func TestGetBidResponse(t *testing.T) {
	resetCachePrometheusRegistry()
	setupCacheTestConfig()
	config.LoadConfig()

	_, cleanup := setupCacheTestRedis(t)
	defer cleanup()

	ctx := context.Background()
	requestID := "test-response-456"

	originalBidResponse := &openrtb.BidResponse{
		ID:       requestID,
		Currency: "EUR",
		SeatBid: []openrtb.SeatBid{
			{
				Bid: []openrtb.Bid{
					{
						ID:    "bid-2",
						ImpID: "imp-2",
						Price: 2.50,
					},
				},
			},
		},
	}

	// Store bid response
	err := cache.SetBidResponse(ctx, requestID, originalBidResponse)
	require.NoError(t, err)

	// Retrieve bid response
	retrievedBidResponse, err := cache.GetBidResponse(ctx, requestID)
	assert.NoError(t, err)
	assert.NotNil(t, retrievedBidResponse)
	assert.Equal(t, requestID, retrievedBidResponse.ID)
	assert.Equal(t, "EUR", retrievedBidResponse.Currency)
	assert.Len(t, retrievedBidResponse.SeatBid, 1)
	assert.Len(t, retrievedBidResponse.SeatBid[0].Bid, 1)
	assert.Equal(t, "bid-2", retrievedBidResponse.SeatBid[0].Bid[0].ID)
}

func TestGetBidResponse_NotFound(t *testing.T) {
	resetCachePrometheusRegistry()
	setupCacheTestConfig()
	config.LoadConfig()

	_, cleanup := setupCacheTestRedis(t)
	defer cleanup()

	ctx := context.Background()
	requestID := "non-existent-response"

	retrievedBidResponse, err := cache.GetBidResponse(ctx, requestID)
	assert.Error(t, err)
	assert.Nil(t, retrievedBidResponse)
}

func TestDeleteBidRequest(t *testing.T) {
	resetCachePrometheusRegistry()
	setupCacheTestConfig()
	config.LoadConfig()

	_, cleanup := setupCacheTestRedis(t)
	defer cleanup()

	ctx := context.Background()
	requestID := "test-delete-request"

	bidRequest := &openrtb.BidRequest{
		ID: requestID,
		Imp: []openrtb.Impression{
			{ID: "imp-1"},
		},
	}

	// Store bid request
	err := cache.SetBidRequest(ctx, requestID, bidRequest)
	require.NoError(t, err)

	// Verify it exists
	client := redisClient.GetClient()
	key := "bid_request:" + requestID
	exists := client.Exists(ctx, key).Val()
	assert.Equal(t, int64(1), exists)

	// Delete bid request
	err = cache.DeleteBidRequest(ctx, requestID)
	assert.NoError(t, err)

	// Verify it's gone
	exists = client.Exists(ctx, key).Val()
	assert.Equal(t, int64(0), exists)
}

func TestDeleteBidResponse(t *testing.T) {
	resetCachePrometheusRegistry()
	setupCacheTestConfig()
	config.LoadConfig()

	_, cleanup := setupCacheTestRedis(t)
	defer cleanup()

	ctx := context.Background()
	requestID := "test-delete-response"

	bidResponse := &openrtb.BidResponse{
		ID:       requestID,
		Currency: "USD",
	}

	// Store bid response
	err := cache.SetBidResponse(ctx, requestID, bidResponse)
	require.NoError(t, err)

	// Verify it exists
	client := redisClient.GetClient()
	key := "bid_response:" + requestID
	exists := client.Exists(ctx, key).Val()
	assert.Equal(t, int64(1), exists)

	// Delete bid response
	err = cache.DeleteBidResponse(ctx, requestID)
	assert.NoError(t, err)

	// Verify it's gone
	exists = client.Exists(ctx, key).Val()
	assert.Equal(t, int64(0), exists)
}

func TestExists(t *testing.T) {
	resetCachePrometheusRegistry()
	setupCacheTestConfig()
	config.LoadConfig()

	_, cleanup := setupCacheTestRedis(t)
	defer cleanup()

	ctx := context.Background()
	requestID := "test-exists"

	// Check non-existent key
	exists, err := cache.Exists(ctx, requestID, "bid_request:")
	assert.NoError(t, err)
	assert.False(t, exists)

	// Store bid request
	bidRequest := &openrtb.BidRequest{
		ID:  requestID,
		Imp: []openrtb.Impression{{ID: "imp-1"}},
	}
	err = cache.SetBidRequest(ctx, requestID, bidRequest)
	require.NoError(t, err)

	// Check existing key
	exists, err = cache.Exists(ctx, requestID, "bid_request:")
	assert.NoError(t, err)
	assert.True(t, exists)

	// Check with wrong prefix
	exists, err = cache.Exists(ctx, requestID, "bid_response:")
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestCache_InvalidJSON(t *testing.T) {
	resetCachePrometheusRegistry()
	setupCacheTestConfig()
	config.LoadConfig()

	_, cleanup := setupCacheTestRedis(t)
	defer cleanup()

	ctx := context.Background()
	requestID := "test-invalid-json"

	// Manually store invalid JSON
	client := redisClient.GetClient()
	key := "bid_request:" + requestID
	err := client.Set(ctx, key, "invalid json", time.Minute).Err()
	require.NoError(t, err)

	// Try to retrieve - should fail with JSON error
	retrievedBidRequest, err := cache.GetBidRequest(ctx, requestID)
	assert.Error(t, err)
	assert.Nil(t, retrievedBidRequest)
}

func TestCache_IntegrationScenario(t *testing.T) {
	resetCachePrometheusRegistry()
	setupCacheTestConfig()
	config.LoadConfig()

	_, cleanup := setupCacheTestRedis(t)
	defer cleanup()

	ctx := context.Background()
	requestID := "integration-test-123"

	// Create bid request
	bidRequest := &openrtb.BidRequest{
		ID: requestID,
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

	// Create bid response
	bidResponse := &openrtb.BidResponse{
		ID:       requestID,
		Currency: "USD",
		SeatBid: []openrtb.SeatBid{
			{
				Bid: []openrtb.Bid{
					{
						ID:    "bid-1",
						ImpID: "imp-1",
						Price: 1.50,
					},
				},
			},
		},
	}

	// Store both
	err := cache.SetBidRequest(ctx, requestID, bidRequest)
	assert.NoError(t, err)

	err = cache.SetBidResponse(ctx, requestID, bidResponse)
	assert.NoError(t, err)

	// Verify both exist
	exists, err := cache.Exists(ctx, requestID, "bid_request:")
	assert.NoError(t, err)
	assert.True(t, exists)

	exists, err = cache.Exists(ctx, requestID, "bid_response:")
	assert.NoError(t, err)
	assert.True(t, exists)

	// Retrieve both
	retrievedRequest, err := cache.GetBidRequest(ctx, requestID)
	assert.NoError(t, err)
	assert.Equal(t, requestID, retrievedRequest.ID)

	retrievedResponse, err := cache.GetBidResponse(ctx, requestID)
	assert.NoError(t, err)
	assert.Equal(t, requestID, retrievedResponse.ID)

	// Clean up
	err = cache.DeleteBidRequest(ctx, requestID)
	assert.NoError(t, err)

	err = cache.DeleteBidResponse(ctx, requestID)
	assert.NoError(t, err)

	// Verify cleanup
	exists, err = cache.Exists(ctx, requestID, "bid_request:")
	assert.NoError(t, err)
	assert.False(t, exists)

	exists, err = cache.Exists(ctx, requestID, "bid_response:")
	assert.NoError(t, err)
	assert.False(t, exists)
}

// Benchmark tests
func BenchmarkSetBidRequest(b *testing.B) {
	resetCachePrometheusRegistry()
	setupCacheTestConfig()
	config.LoadConfig()

	_, cleanup := setupCacheTestRedisBench(b)
	defer cleanup()

	ctx := context.Background()
	bidRequest := &openrtb.BidRequest{
		ID: "benchmark-request",
		Imp: []openrtb.Impression{
			{ID: "imp-1"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.SetBidRequest(ctx, "benchmark-request", bidRequest)
	}
}

func BenchmarkGetBidRequest(b *testing.B) {
	resetCachePrometheusRegistry()
	setupCacheTestConfig()
	config.LoadConfig()

	_, cleanup := setupCacheTestRedisBench(b)
	defer cleanup()

	ctx := context.Background()
	bidRequest := &openrtb.BidRequest{
		ID: "benchmark-request",
		Imp: []openrtb.Impression{
			{ID: "imp-1"},
		},
	}

	// Pre-populate
	cache.SetBidRequest(ctx, "benchmark-request", bidRequest)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.GetBidRequest(ctx, "benchmark-request")
	}
}

func setupCacheTestRedisBench(b *testing.B) (*miniredis.Miniredis, func()) {
	// Start miniredis
	mr := miniredis.NewMiniRedis()
	err := mr.Start()
	if err != nil {
		b.Fatal(err)
	}

	// Create Redis client pointing to miniredis
	testClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
		DB:   0,
	})

	// Set the test client
	redisClient.SetTestClient(testClient)

	cleanup := func() {
		testClient.Close()
		mr.Close()
	}

	return mr, cleanup
}
