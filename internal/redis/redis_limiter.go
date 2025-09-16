package redis

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

const (
	RateLimitPrefix = "rate_limit:"
)

// RedisRateLimiter implements rate limiting using Redis counters
type RedisRateLimiter struct {
	Key      string
	Limit    int
	Duration time.Duration
}

// NewRedisRateLimiter creates a new Redis-based rate limiter
func NewRedisRateLimiter(key string, limit int, duration time.Duration) *RedisRateLimiter {
	return &RedisRateLimiter{
		Key:      RateLimitPrefix + key,
		Limit:    limit,
		Duration: duration,
	}
}

// Allow checks if request is allowed and increments counter
func (r *RedisRateLimiter) Allow(ctx context.Context) (bool, error) {
	client := GetClient()

	// Get current count
	countStr, err := client.Get(ctx, r.Key).Result()
	if err != nil && err.Error() != "redis: nil" {
		return false, err
	}

	count := 0
	if countStr != "" {
		count, _ = strconv.Atoi(countStr)
	}

	// Check if limit exceeded
	if count >= r.Limit {
		return false, nil
	}

	// Increment counter
	pipe := client.Pipeline()
	pipe.Incr(ctx, r.Key)
	pipe.Expire(ctx, r.Key, r.Duration)
	_, err = pipe.Exec(ctx)

	if err != nil {
		return false, err
	}

	return true, nil
}

// GetCount returns current count for the key
func (r *RedisRateLimiter) GetCount(ctx context.Context) (int, error) {
	client := GetClient()

	countStr, err := client.Get(ctx, r.Key).Result()
	if err != nil && err.Error() != "redis: nil" {
		return 0, err
	}

	if countStr == "" {
		return 0, nil
	}

	count, err := strconv.Atoi(countStr)
	if err != nil {
		return 0, err
	}

	return count, nil
}

// Reset clears the counter
func (r *RedisRateLimiter) Reset(ctx context.Context) error {
	client := GetClient()
	return client.Del(ctx, r.Key).Err()
}

// GetTTL returns remaining time until reset
func (r *RedisRateLimiter) GetTTL(ctx context.Context) (time.Duration, error) {
	client := GetClient()
	return client.TTL(ctx, r.Key).Result()
}

// RateLimitByIP creates rate limiter for IP address
func RateLimitByIP(ip string, requestsPerMinute int) *RedisRateLimiter {
	key := fmt.Sprintf("ip:%s", ip)
	return NewRedisRateLimiter(key, requestsPerMinute, time.Minute)
}

// RateLimitByBuyer creates rate limiter for buyer endpoint
func RateLimitByBuyer(buyerEndpoint string, requestsPerSecond int) *RedisRateLimiter {
	key := fmt.Sprintf("buyer:%s", buyerEndpoint)
	return NewRedisRateLimiter(key, requestsPerSecond, time.Second)
}

// RateLimitByUser creates rate limiter for user
func RateLimitByUser(userID string, requestsPerHour int) *RedisRateLimiter {
	key := fmt.Sprintf("user:%s", userID)
	return NewRedisRateLimiter(key, requestsPerHour, time.Hour)
}
