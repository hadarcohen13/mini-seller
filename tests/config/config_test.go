package config_test

import (
	"os"
	"testing"

	"github.com/hadarco13/mini-seller/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig_Development(t *testing.T) {
	os.Setenv("MINI_SELLER_ENVIRONMENT", "development")
	defer os.Unsetenv("MINI_SELLER_ENVIRONMENT")

	err := config.LoadConfig()
	require.NoError(t, err)

	cfg := config.GetConfig()
	assert.NotNil(t, cfg)
	assert.Equal(t, "development", cfg.Environment)
	assert.NotEmpty(t, cfg.Server.Port)
}

func TestLoadConfig_Production(t *testing.T) {
	os.Setenv("MINI_SELLER_ENVIRONMENT", "production")
	defer os.Unsetenv("MINI_SELLER_ENVIRONMENT")

	err := config.LoadConfig()
	require.NoError(t, err)

	cfg := config.GetConfig()
	assert.NotNil(t, cfg)
	assert.Equal(t, "production", cfg.Environment)
	assert.NotEmpty(t, cfg.Server.Port)
}

func TestLoadConfig_Test(t *testing.T) {
	os.Setenv("MINI_SELLER_ENVIRONMENT", "test")
	defer os.Unsetenv("MINI_SELLER_ENVIRONMENT")

	err := config.LoadConfig()
	require.NoError(t, err)

	cfg := config.GetConfig()
	assert.NotNil(t, cfg)
	assert.Equal(t, "test", cfg.Environment)
	assert.NotEmpty(t, cfg.Server.Port)
}

func TestLoadConfig_DefaultEnvironment(t *testing.T) {
	os.Unsetenv("MINI_SELLER_ENVIRONMENT")

	err := config.LoadConfig()
	require.NoError(t, err)

	cfg := config.GetConfig()
	assert.NotNil(t, cfg)
	assert.Equal(t, "development", cfg.Environment)
}

func TestGetConfig_BeforeLoad(t *testing.T) {
	config.ResetConfig()
	cfg := config.GetConfig()
	assert.Nil(t, cfg)
}

func TestGetConfig_AfterLoad(t *testing.T) {
	os.Setenv("MINI_SELLER_ENVIRONMENT", "test")
	defer os.Unsetenv("MINI_SELLER_ENVIRONMENT")

	err := config.LoadConfig()
	require.NoError(t, err)

	cfg := config.GetConfig()
	assert.NotNil(t, cfg)
	assert.Equal(t, "test", cfg.Environment)
}

func TestConfigStruct_ServerConfig(t *testing.T) {
	os.Setenv("MINI_SELLER_ENVIRONMENT", "test")
	defer os.Unsetenv("MINI_SELLER_ENVIRONMENT")

	err := config.LoadConfig()
	require.NoError(t, err)

	cfg := config.GetConfig()
	require.NotNil(t, cfg)

	assert.NotEmpty(t, cfg.Server.Port)
	assert.Greater(t, cfg.Server.ReadTimeoutMs, 0)
	assert.Greater(t, cfg.Server.WriteTimeoutMs, 0)
	assert.Greater(t, cfg.Server.ShutdownTimeoutMs, 0)
	assert.Greater(t, cfg.Server.HealthCheckTimeoutMs, 0)
	assert.Greater(t, cfg.Server.IdleConnTimeoutMs, 0)
	assert.Greater(t, cfg.Server.WorkerStopTimeoutMs, 0)
}

func TestConfigStruct_RedisConfig(t *testing.T) {
	os.Setenv("MINI_SELLER_ENVIRONMENT", "test")
	defer os.Unsetenv("MINI_SELLER_ENVIRONMENT")

	err := config.LoadConfig()
	require.NoError(t, err)

	cfg := config.GetConfig()
	require.NotNil(t, cfg)

	assert.NotEmpty(t, cfg.Redis.Host)
	assert.NotEmpty(t, cfg.Redis.Port)
	assert.GreaterOrEqual(t, cfg.Redis.DB, 0)
	assert.Greater(t, cfg.Redis.PoolSize, 0)
	assert.Greater(t, cfg.Redis.DialTimeoutMs, 0)
	assert.Greater(t, cfg.Redis.ReadTimeoutMs, 0)
	assert.Greater(t, cfg.Redis.WriteTimeoutMs, 0)
}

func TestConfigStruct_LoggingConfig(t *testing.T) {
	os.Setenv("MINI_SELLER_ENVIRONMENT", "test")
	defer os.Unsetenv("MINI_SELLER_ENVIRONMENT")

	err := config.LoadConfig()
	require.NoError(t, err)

	cfg := config.GetConfig()
	require.NotNil(t, cfg)

	assert.NotEmpty(t, cfg.Logging.Level)
	assert.NotEmpty(t, cfg.Logging.Format)
}

func TestConfigStruct_RateLimitingConfig(t *testing.T) {
	os.Setenv("MINI_SELLER_ENVIRONMENT", "test")
	defer os.Unsetenv("MINI_SELLER_ENVIRONMENT")

	err := config.LoadConfig()
	require.NoError(t, err)

	cfg := config.GetConfig()
	require.NotNil(t, cfg)

	assert.Greater(t, cfg.RateLimiting.RequestsPerSecond, float64(0))
	assert.Greater(t, cfg.RateLimiting.BurstSize, 0)
}

func TestConfigStruct_BuyerConfig(t *testing.T) {
	os.Setenv("MINI_SELLER_ENVIRONMENT", "test")
	defer os.Unsetenv("MINI_SELLER_ENVIRONMENT")

	err := config.LoadConfig()
	require.NoError(t, err)

	cfg := config.GetConfig()
	require.NotNil(t, cfg)

	assert.Greater(t, cfg.Buyer.QPS, float64(0))
	assert.Greater(t, cfg.Buyer.Burst, 0)
	assert.Greater(t, cfg.Buyer.TimeoutMs, 0)
}

func TestEnvironmentVariableOverrides(t *testing.T) {
	os.Setenv("MINI_SELLER_ENVIRONMENT", "test")
	os.Setenv("MINI_SELLER_SERVER_PORT", "9999")
	os.Setenv("MINI_SELLER_REDIS_HOST", "custom-redis-host")
	os.Setenv("MINI_SELLER_REDIS_DB", "5")

	defer func() {
		os.Unsetenv("MINI_SELLER_ENVIRONMENT")
		os.Unsetenv("MINI_SELLER_SERVER_PORT")
		os.Unsetenv("MINI_SELLER_REDIS_HOST")
		os.Unsetenv("MINI_SELLER_REDIS_DB")
	}()

	err := config.LoadConfig()
	require.NoError(t, err)

	cfg := config.GetConfig()
	require.NotNil(t, cfg)

	assert.Equal(t, "9999", cfg.Server.Port)
	assert.Equal(t, "custom-redis-host", cfg.Redis.Host)
	assert.Equal(t, 5, cfg.Redis.DB)
}

func TestConfigValidation_InvalidPort(t *testing.T) {
	os.Setenv("MINI_SELLER_ENVIRONMENT", "test")
	os.Setenv("MINI_SELLER_SERVER_PORT", "invalid")
	defer func() {
		os.Unsetenv("MINI_SELLER_ENVIRONMENT")
		os.Unsetenv("MINI_SELLER_SERVER_PORT")
	}()

	err := config.LoadConfig()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "port")
}

func TestConfigValidation_InvalidEnvironment(t *testing.T) {
	os.Setenv("MINI_SELLER_ENVIRONMENT", "invalid_env")
	defer os.Unsetenv("MINI_SELLER_ENVIRONMENT")

	err := config.LoadConfig()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "environment")
}

func TestConfigUtilityFunctions(t *testing.T) {
	os.Setenv("MINI_SELLER_ENVIRONMENT", "development")
	config.LoadConfig()
	assert.True(t, config.IsDevelopment())
	assert.False(t, config.IsProduction())

	os.Setenv("MINI_SELLER_ENVIRONMENT", "production")
	config.LoadConfig()
	assert.False(t, config.IsDevelopment())
	assert.True(t, config.IsProduction())

	address := config.GetServerAddress()
	assert.Contains(t, address, ":")
	assert.NotEmpty(t, address)

	os.Unsetenv("MINI_SELLER_ENVIRONMENT")
}

func TestGetServerAddress_NoConfig(t *testing.T) {
	config.ResetConfig()
	address := config.GetServerAddress()
	assert.Equal(t, "localhost:8080", address)
}

func TestBuyerConfigStructure(t *testing.T) {
	buyer := config.BuyerConfig{
		Name:      "test-buyer",
		Endpoint:  "http://buyer.example.com/bid",
		QPS:       10.0,
		Burst:     20,
		TimeoutMs: 100,
	}

	assert.Equal(t, "test-buyer", buyer.Name)
	assert.Equal(t, "http://buyer.example.com/bid", buyer.Endpoint)
	assert.Equal(t, 10.0, buyer.QPS)
	assert.Equal(t, 20, buyer.Burst)
	assert.Equal(t, 100, buyer.TimeoutMs)
}

func TestServerConfigStructure(t *testing.T) {
	server := config.ServerConfig{
		Port:                 "8080",
		Host:                 "localhost",
		LogLevel:             "info",
		ReadTimeoutMs:        5000,
		WriteTimeoutMs:       10000,
		ShutdownTimeoutMs:    30000,
		HealthCheckTimeoutMs: 5000,
		IdleConnTimeoutMs:    90000,
		WorkerStopTimeoutMs:  5000,
	}

	assert.Equal(t, "8080", server.Port)
	assert.Equal(t, "localhost", server.Host)
	assert.Equal(t, 5000, server.ReadTimeoutMs)
	assert.Equal(t, 10000, server.WriteTimeoutMs)
	assert.Equal(t, 30000, server.ShutdownTimeoutMs)
	assert.Equal(t, 5000, server.HealthCheckTimeoutMs)
	assert.Equal(t, 90000, server.IdleConnTimeoutMs)
	assert.Equal(t, 5000, server.WorkerStopTimeoutMs)
}

// Benchmark tests
func BenchmarkGetConfig(b *testing.B) {
	os.Setenv("MINI_SELLER_ENVIRONMENT", "test")
	defer os.Unsetenv("MINI_SELLER_ENVIRONMENT")
	config.LoadConfig()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			cfg := config.GetConfig()
			if cfg == nil {
				b.Fatal("Config is nil")
			}
		}
	})
}

func BenchmarkLoadConfig(b *testing.B) {
	os.Setenv("MINI_SELLER_ENVIRONMENT", "test")
	defer os.Unsetenv("MINI_SELLER_ENVIRONMENT")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := config.LoadConfig()
		if err != nil {
			b.Fatalf("LoadConfig failed: %v", err)
		}
	}
}
