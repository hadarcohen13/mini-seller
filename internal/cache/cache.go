package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/bsm/openrtb"
	"github.com/hadarco13/mini-seller/internal/metrics"
	"github.com/hadarco13/mini-seller/internal/redis"
)

const (
	BidRequestPrefix  = "bid_request:"
	BidResponsePrefix = "bid_response:"
	DefaultTTL        = 5 * time.Minute
)

// SetBidRequest caches a bid request with performance monitoring
func SetBidRequest(ctx context.Context, requestID string, bidRequest *openrtb.BidRequest) error {
	start := time.Now()
	key := BidRequestPrefix + requestID
	data, err := json.Marshal(bidRequest)
	if err != nil {
		return err
	}

	client := redis.GetClient()
	err = client.Set(ctx, key, data, DefaultTTL).Err()

	// Record Redis performance metrics
	latency := time.Since(start)
	metrics.RecordRedisOp(latency, err == nil)

	return err
}

// GetBidRequest retrieves a cached bid request with performance monitoring
func GetBidRequest(ctx context.Context, requestID string) (*openrtb.BidRequest, error) {
	start := time.Now()
	key := BidRequestPrefix + requestID
	client := redis.GetClient()

	data, err := client.Get(ctx, key).Result()
	latency := time.Since(start)
	metrics.RecordRedisOp(latency, err == nil)

	if err != nil {
		return nil, err
	}

	var bidRequest openrtb.BidRequest
	err = json.Unmarshal([]byte(data), &bidRequest)
	if err != nil {
		return nil, err
	}

	return &bidRequest, nil
}

// SetBidResponse caches a bid response
func SetBidResponse(ctx context.Context, requestID string, bidResponse *openrtb.BidResponse) error {
	key := BidResponsePrefix + requestID
	data, err := json.Marshal(bidResponse)
	if err != nil {
		return err
	}

	client := redis.GetClient()
	return client.Set(ctx, key, data, DefaultTTL).Err()
}

// GetBidResponse retrieves a cached bid response
func GetBidResponse(ctx context.Context, requestID string) (*openrtb.BidResponse, error) {
	key := BidResponsePrefix + requestID
	client := redis.GetClient()

	data, err := client.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	var bidResponse openrtb.BidResponse
	err = json.Unmarshal([]byte(data), &bidResponse)
	if err != nil {
		return nil, err
	}

	return &bidResponse, nil
}

// DeleteBidRequest removes a cached bid request
func DeleteBidRequest(ctx context.Context, requestID string) error {
	key := BidRequestPrefix + requestID
	client := redis.GetClient()
	return client.Del(ctx, key).Err()
}

// DeleteBidResponse removes a cached bid response
func DeleteBidResponse(ctx context.Context, requestID string) error {
	key := BidResponsePrefix + requestID
	client := redis.GetClient()
	return client.Del(ctx, key).Err()
}

// Exists checks if a key exists in cache
func Exists(ctx context.Context, requestID string, prefix string) (bool, error) {
	key := prefix + requestID
	client := redis.GetClient()

	count, err := client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}

	return count > 0, nil
}
