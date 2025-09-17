package redis_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hadarco13/mini-seller/internal/config"
	redisClient "github.com/hadarco13/mini-seller/internal/redis"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	// Start miniredis server
	mr, err := miniredis.Run()
	require.NoError(t, err)

	// Create Redis client
	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
		DB:   0,
	})

	return mr, client
}

func setupRedisTestConfig() {
	os.Setenv("MINI_SELLER_ENVIRONMENT", "test")
	os.Setenv("MINI_SELLER_SERVER_PORT", "8080")
}

func TestRedisConnect(t *testing.T) {
	setupRedisTestConfig()
	config.LoadConfig()

	// Override config to point to miniredis
	mr := miniredis.NewMiniRedis()
	err := mr.Start()
	require.NoError(t, err)
	defer mr.Close()

	cfg := config.GetConfig()
	cfg.Redis.Host = "localhost"
	cfg.Redis.Port = mr.Port()

	err = redisClient.Connect()
	assert.NoError(t, err)

	// Verify connection works
	client := redisClient.GetClient()
	assert.NotNil(t, client)

	// Test ping
	result, err := client.Ping(context.Background()).Result()
	assert.NoError(t, err)
	assert.Equal(t, "PONG", result)

	// Close connection
	err = redisClient.Close()
	assert.NoError(t, err)
}

func TestRedisConnect_InvalidConfig(t *testing.T) {
	setupRedisTestConfig()
	config.LoadConfig()

	// Set invalid Redis config
	cfg := config.GetConfig()
	cfg.Redis.Host = "invalid-host"
	cfg.Redis.Port = "99999"

	err := redisClient.Connect()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "redis connection failed")
}

func TestRedisClose_NoConnection(t *testing.T) {
	// Test closing when no connection exists
	err := redisClient.Close()
	assert.NoError(t, err) // Should not error
}

func TestRedisConnection(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()

	// Test ping
	pong, err := client.Ping(ctx).Result()
	assert.NoError(t, err)
	assert.Equal(t, "PONG", pong)
}

func TestRedisSetGet(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()

	// Test set and get
	err := client.Set(ctx, "test-key", "test-value", 0).Err()
	assert.NoError(t, err)

	val, err := client.Get(ctx, "test-key").Result()
	assert.NoError(t, err)
	assert.Equal(t, "test-value", val)
}

func TestRedisExpiration(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()

	// Set key with expiration
	err := client.Set(ctx, "expire-key", "expire-value", 100*time.Millisecond).Err()
	assert.NoError(t, err)

	// Key should exist initially
	val, err := client.Get(ctx, "expire-key").Result()
	assert.NoError(t, err)
	assert.Equal(t, "expire-value", val)

	// Fast forward time in miniredis
	mr.FastForward(200 * time.Millisecond)

	// Key should be expired
	_, err = client.Get(ctx, "expire-key").Result()
	assert.Error(t, err)
	assert.Equal(t, redis.Nil, err)
}

func TestRedisHashOperations(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()

	// Test hash set
	err := client.HSet(ctx, "test-hash", "field1", "value1", "field2", "value2").Err()
	assert.NoError(t, err)

	// Test hash get
	val1, err := client.HGet(ctx, "test-hash", "field1").Result()
	assert.NoError(t, err)
	assert.Equal(t, "value1", val1)

	val2, err := client.HGet(ctx, "test-hash", "field2").Result()
	assert.NoError(t, err)
	assert.Equal(t, "value2", val2)

	// Test hash get all
	all, err := client.HGetAll(ctx, "test-hash").Result()
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{
		"field1": "value1",
		"field2": "value2",
	}, all)
}

func TestRedisListOperations(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()

	// Test list push
	err := client.LPush(ctx, "test-list", "item1", "item2", "item3").Err()
	assert.NoError(t, err)

	// Test list length
	length, err := client.LLen(ctx, "test-list").Result()
	assert.NoError(t, err)
	assert.Equal(t, int64(3), length)

	// Test list pop
	item, err := client.RPop(ctx, "test-list").Result()
	assert.NoError(t, err)
	assert.Equal(t, "item1", item)

	// Test list range
	items, err := client.LRange(ctx, "test-list", 0, -1).Result()
	assert.NoError(t, err)
	assert.Equal(t, []string{"item3", "item2"}, items)
}

func TestRedisIncrement(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()

	// Test increment on non-existent key
	val, err := client.Incr(ctx, "counter").Result()
	assert.NoError(t, err)
	assert.Equal(t, int64(1), val)

	// Test increment on existing key
	val, err = client.Incr(ctx, "counter").Result()
	assert.NoError(t, err)
	assert.Equal(t, int64(2), val)

	// Test increment by value
	val, err = client.IncrBy(ctx, "counter", 5).Result()
	assert.NoError(t, err)
	assert.Equal(t, int64(7), val)
}

func TestRedisBidCacheOperations(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()

	// Test storing bid request data (simulating real usage)
	bidRequestID := "test-bid-123"
	timestamp := time.Now().Unix()

	// Store as hash (like the app might do)
	err := client.HSet(ctx, "bid:"+bidRequestID,
		"request_id", bidRequestID,
		"timestamp", timestamp,
		"impression_count", 2,
		"status", "processed",
	).Err()
	assert.NoError(t, err)

	// Retrieve data
	data, err := client.HGetAll(ctx, "bid:"+bidRequestID).Result()
	assert.NoError(t, err)
	assert.Equal(t, bidRequestID, data["request_id"])
	assert.Equal(t, "2", data["impression_count"])
	assert.Equal(t, "processed", data["status"])
}

func TestRedisConnectionFailure(t *testing.T) {
	// Create client with invalid address
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:9999", // Non-existent Redis server
		DB:   0,
	})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Test connection failure
	_, err := client.Ping(ctx).Result()
	assert.Error(t, err)
}

func TestRedisTransactions(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()

	// Test Redis transaction (MULTI/EXEC)
	pipe := client.TxPipeline()

	pipe.Set(ctx, "key1", "value1", 0)
	pipe.Set(ctx, "key2", "value2", 0)
	pipe.Incr(ctx, "counter")

	// Execute transaction
	cmds, err := pipe.Exec(ctx)
	assert.NoError(t, err)
	assert.Len(t, cmds, 3)

	// Verify results
	val1, err := client.Get(ctx, "key1").Result()
	assert.NoError(t, err)
	assert.Equal(t, "value1", val1)

	val2, err := client.Get(ctx, "key2").Result()
	assert.NoError(t, err)
	assert.Equal(t, "value2", val2)

	counter, err := client.Get(ctx, "counter").Result()
	assert.NoError(t, err)
	assert.Equal(t, "1", counter)
}
