package redis_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	redisClient "github.com/hadarco13/mini-seller/internal/redis"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupRedisLimiterTest() (*miniredis.Miniredis, func()) {
	// Start miniredis
	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		panic(err)
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

func TestNewRedisRateLimiter(t *testing.T) {
	limiter := redisClient.NewRedisRateLimiter("test_key", 10, time.Minute)

	assert.NotNil(t, limiter)
	assert.Equal(t, "rate_limit:test_key", limiter.Key)
	assert.Equal(t, 10, limiter.Limit)
	assert.Equal(t, time.Minute, limiter.Duration)
}

func TestRedisRateLimiter_Allow(t *testing.T) {
	mr, cleanup := setupRedisLimiterTest()
	defer cleanup()

	ctx := context.Background()
	limiter := redisClient.NewRedisRateLimiter("test_allow", 3, time.Minute)

	// First requests should be allowed
	allowed, err := limiter.Allow(ctx)
	require.NoError(t, err)
	assert.True(t, allowed)

	allowed, err = limiter.Allow(ctx)
	require.NoError(t, err)
	assert.True(t, allowed)

	allowed, err = limiter.Allow(ctx)
	require.NoError(t, err)
	assert.True(t, allowed)

	// Fourth request should be denied (limit is 3)
	allowed, err = limiter.Allow(ctx)
	require.NoError(t, err)
	assert.False(t, allowed)

	// Check that counter was set correctly
	count, err := limiter.GetCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	// Verify TTL was set
	ttl, err := limiter.GetTTL(ctx)
	require.NoError(t, err)
	assert.True(t, ttl > 0 && ttl <= time.Minute)

	// Verify Redis state
	val, _ := mr.Get(limiter.Key)
	assert.Equal(t, "3", val)
}

func TestRedisRateLimiter_GetCount(t *testing.T) {
	_, cleanup := setupRedisLimiterTest()
	defer cleanup()

	ctx := context.Background()
	limiter := redisClient.NewRedisRateLimiter("test_count", 5, time.Minute)

	// Initially should be 0
	count, err := limiter.GetCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// After some requests
	limiter.Allow(ctx)
	limiter.Allow(ctx)

	count, err = limiter.GetCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestRedisRateLimiter_Reset(t *testing.T) {
	_, cleanup := setupRedisLimiterTest()
	defer cleanup()

	ctx := context.Background()
	limiter := redisClient.NewRedisRateLimiter("test_reset", 2, time.Minute)

	// Make some requests
	limiter.Allow(ctx)
	limiter.Allow(ctx)

	count, err := limiter.GetCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	// Reset the counter
	err = limiter.Reset(ctx)
	require.NoError(t, err)

	// Count should be 0
	count, err = limiter.GetCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Should be able to make requests again
	allowed, err := limiter.Allow(ctx)
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestRedisRateLimiter_GetTTL(t *testing.T) {
	_, cleanup := setupRedisLimiterTest()
	defer cleanup()

	ctx := context.Background()
	limiter := redisClient.NewRedisRateLimiter("test_ttl", 5, time.Minute)

	// No TTL initially (key doesn't exist)
	ttl, err := limiter.GetTTL(ctx)
	require.NoError(t, err)
	assert.Equal(t, time.Duration(-2), ttl) // Redis returns -2 for non-existent keys

	// After first request, TTL should be set
	limiter.Allow(ctx)

	ttl, err = limiter.GetTTL(ctx)
	require.NoError(t, err)
	assert.True(t, ttl > 0 && ttl <= time.Minute)
}

func TestRateLimitByIP(t *testing.T) {
	limiter := redisClient.RateLimitByIP("192.168.1.1", 60)

	assert.NotNil(t, limiter)
	assert.Equal(t, "rate_limit:ip:192.168.1.1", limiter.Key)
	assert.Equal(t, 60, limiter.Limit)
	assert.Equal(t, time.Minute, limiter.Duration)
}

func TestRateLimitByBuyer(t *testing.T) {
	buyerEndpoint := "https://buyer.example.com/bid"
	limiter := redisClient.RateLimitByBuyer(buyerEndpoint, 10)

	assert.NotNil(t, limiter)
	assert.Equal(t, "rate_limit:buyer:https://buyer.example.com/bid", limiter.Key)
	assert.Equal(t, 10, limiter.Limit)
	assert.Equal(t, time.Second, limiter.Duration)
}

func TestRateLimitByUser(t *testing.T) {
	limiter := redisClient.RateLimitByUser("user123", 1000)

	assert.NotNil(t, limiter)
	assert.Equal(t, "rate_limit:user:user123", limiter.Key)
	assert.Equal(t, 1000, limiter.Limit)
	assert.Equal(t, time.Hour, limiter.Duration)
}

func TestRedisRateLimiter_IntegrationScenarios(t *testing.T) {
	_, cleanup := setupRedisLimiterTest()
	defer cleanup()

	ctx := context.Background()

	t.Run("Multiple limiters with different keys", func(t *testing.T) {
		limiter1 := redisClient.RateLimitByIP("192.168.1.1", 2)
		limiter2 := redisClient.RateLimitByIP("192.168.1.2", 2)

		// Both should allow requests independently
		allowed, err := limiter1.Allow(ctx)
		require.NoError(t, err)
		assert.True(t, allowed)

		allowed, err = limiter2.Allow(ctx)
		require.NoError(t, err)
		assert.True(t, allowed)

		// Each should track separately
		count1, _ := limiter1.GetCount(ctx)
		count2, _ := limiter2.GetCount(ctx)
		assert.Equal(t, 1, count1)
		assert.Equal(t, 1, count2)
	})

	t.Run("Rate limiting across different time windows", func(t *testing.T) {
		buyerLimiter := redisClient.RateLimitByBuyer("buyer1", 1) // 1 per second
		userLimiter := redisClient.RateLimitByUser("user1", 1)    // 1 per hour

		// Both should allow first request
		allowed, err := buyerLimiter.Allow(ctx)
		require.NoError(t, err)
		assert.True(t, allowed)

		allowed, err = userLimiter.Allow(ctx)
		require.NoError(t, err)
		assert.True(t, allowed)

		// Both should deny second request
		allowed, err = buyerLimiter.Allow(ctx)
		require.NoError(t, err)
		assert.False(t, allowed)

		allowed, err = userLimiter.Allow(ctx)
		require.NoError(t, err)
		assert.False(t, allowed)
	})

	t.Run("Reset functionality", func(t *testing.T) {
		limiter := redisClient.RateLimitByIP("test-reset-ip", 1)

		// Use up the limit
		allowed, err := limiter.Allow(ctx)
		require.NoError(t, err)
		assert.True(t, allowed)

		// Should be blocked
		allowed, err = limiter.Allow(ctx)
		require.NoError(t, err)
		assert.False(t, allowed)

		// Reset and try again
		err = limiter.Reset(ctx)
		require.NoError(t, err)

		// Should be allowed again
		allowed, err = limiter.Allow(ctx)
		require.NoError(t, err)
		assert.True(t, allowed)
	})
}

// Error scenarios
func TestRedisRateLimiter_ErrorHandling(t *testing.T) {
	// This test uses a real (disconnected) client to test error handling
	disconnectedClient := redis.NewClient(&redis.Options{
		Addr: "localhost:9999", // Non-existent Redis instance
		DB:   0,
	})
	redisClient.SetTestClient(disconnectedClient)
	defer disconnectedClient.Close()

	ctx := context.Background()
	limiter := redisClient.NewRedisRateLimiter("error_test", 5, time.Minute)

	// Should handle connection errors gracefully
	allowed, err := limiter.Allow(ctx)
	assert.Error(t, err)
	assert.False(t, allowed)

	_, err = limiter.GetCount(ctx)
	assert.Error(t, err)

	err = limiter.Reset(ctx)
	assert.Error(t, err)

	_, err = limiter.GetTTL(ctx)
	assert.Error(t, err)
}

// Benchmark tests
func BenchmarkRedisRateLimiter_Allow(b *testing.B) {
	_, cleanup := setupRedisLimiterTest()
	defer cleanup()

	ctx := context.Background()
	limiter := redisClient.NewRedisRateLimiter("benchmark", 1000000, time.Hour)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Allow(ctx)
	}
}

func BenchmarkRedisRateLimiter_GetCount(b *testing.B) {
	_, cleanup := setupRedisLimiterTest()
	defer cleanup()

	ctx := context.Background()
	limiter := redisClient.NewRedisRateLimiter("benchmark_count", 1000, time.Hour)

	// Pre-populate some data
	limiter.Allow(ctx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.GetCount(ctx)
	}
}
