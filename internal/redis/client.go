package redis

import (
	"context"
	"fmt"

	"github.com/hadarco13/mini-seller/internal/config"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

var Client *redis.Client

// Connect establishes Redis connection
func Connect() error {
	cfg := config.GetConfig()

	addr := fmt.Sprintf("%s:%s", cfg.Redis.Host, cfg.Redis.Port)

	Client = redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	// Test connection
	ctx := context.Background()
	_, err := Client.Ping(ctx).Result()
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"address": addr,
			"error":   err.Error(),
		}).Error("Failed to connect to Redis")
		return fmt.Errorf("redis connection failed: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"address": addr,
		"db":      cfg.Redis.DB,
	}).Info("Redis connected successfully")

	return nil
}

// Close closes Redis connection
func Close() error {
	if Client != nil {
		return Client.Close()
	}
	return nil
}

// GetClient returns Redis client instance
func GetClient() *redis.Client {
	return Client
}
