package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/hadarco13/mini-seller/internal/errors"
	"github.com/hadarco13/mini-seller/internal/logging"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type ServerConfig struct {
	Port                 string `mapstructure:"port" yaml:"port"`
	Host                 string `mapstructure:"host" yaml:"host"`
	LogLevel             string `mapstructure:"log_level" yaml:"log_level"`
	ReadTimeoutMs        int    `mapstructure:"read_timeout_ms" yaml:"read_timeout_ms"`
	WriteTimeoutMs       int    `mapstructure:"write_timeout_ms" yaml:"write_timeout_ms"`
	ShutdownTimeoutMs    int    `mapstructure:"shutdown_timeout_ms" yaml:"shutdown_timeout_ms"`
	HealthCheckTimeoutMs int    `mapstructure:"health_check_timeout_ms" yaml:"health_check_timeout_ms"`
	IdleConnTimeoutMs    int    `mapstructure:"idle_conn_timeout_ms" yaml:"idle_conn_timeout_ms"`
	WorkerStopTimeoutMs  int    `mapstructure:"worker_stop_timeout_ms" yaml:"worker_stop_timeout_ms"`
}

type RateLimitConfig struct {
	QPS   float64 `mapstructure:"qps" yaml:"qps"`
	Burst int     `mapstructure:"burst" yaml:"burst"`
}

type RateLimitingConfig struct {
	RequestsPerSecond float64 `mapstructure:"requests_per_second" yaml:"requests_per_second"`
	BurstSize         int     `mapstructure:"burst_size" yaml:"burst_size"`
}

type AppConfig struct {
	Environment  string             `mapstructure:"environment" yaml:"environment"`
	Server       ServerConfig       `mapstructure:"server" yaml:"server"`
	Redis        RedisConfig        `mapstructure:"redis" yaml:"redis"`
	Debug        bool               `mapstructure:"debug" yaml:"debug"`
	Buyers       []BuyerConfig      `mapstructure:"buyers" yaml:"buyers"`
	Logging      LoggingConfig      `mapstructure:"logging" yaml:"logging"`
	RateLimit    RateLimitConfig    `mapstructure:"rate_limit" yaml:"rate_limit"`
	RateLimiting RateLimitingConfig `mapstructure:"rate_limiting" yaml:"rate_limiting"`
	Buyer        BuyerConfig        `mapstructure:"buyer" yaml:"buyer"`
}

type LoggingConfig struct {
	Level  string `mapstructure:"level" yaml:"level"`   // trace, debug, info, warn, error, fatal
	Format string `mapstructure:"format" yaml:"format"` // json, text
}

type RedisConfig struct {
	Host           string `mapstructure:"host" yaml:"host"`
	Port           string `mapstructure:"port" yaml:"port"`
	Password       string `mapstructure:"password" yaml:"password"`
	DB             int    `mapstructure:"db" yaml:"db"`
	PoolSize       int    `mapstructure:"pool_size" yaml:"pool_size"`
	DialTimeoutMs  int    `mapstructure:"dial_timeout_ms" yaml:"dial_timeout_ms"`
	ReadTimeoutMs  int    `mapstructure:"read_timeout_ms" yaml:"read_timeout_ms"`
	WriteTimeoutMs int    `mapstructure:"write_timeout_ms" yaml:"write_timeout_ms"`
}

type BuyerConfig struct {
	Name      string  `mapstructure:"name" yaml:"name"`
	Endpoint  string  `mapstructure:"endpoint" yaml:"endpoint"`
	QPS       float64 `mapstructure:"qps" yaml:"qps"`
	Burst     int     `mapstructure:"burst" yaml:"burst"`
	TimeoutMs int     `mapstructure:"timeout_ms" yaml:"timeout_ms"`
}

var Config *AppConfig

func LoadConfig() error {
	// Determine environment-specific config file
	env := os.Getenv("MINI_SELLER_ENVIRONMENT")
	if env == "" {
		env = "development" // default to development
	}

	viper.SetConfigName(env) // loads development.yaml, production.yaml, or test.yaml
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./configs")
	viper.AddConfigPath("/etc/mini-seller/")

	viper.AutomaticEnv()
	viper.SetEnvPrefix("MINI_SELLER")

	// Bind specific environment variables
	viper.BindEnv("server.port", "MINI_SELLER_SERVER_PORT")
	viper.BindEnv("server.host", "MINI_SELLER_SERVER_HOST")
	viper.BindEnv("environment", "MINI_SELLER_ENVIRONMENT")
	viper.BindEnv("redis.host", "MINI_SELLER_REDIS_HOST")
	viper.BindEnv("redis.port", "MINI_SELLER_REDIS_PORT")
	viper.BindEnv("redis.db", "MINI_SELLER_REDIS_DB")

	setDefaults()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			logrus.Info("No config file found, using defaults and environment variables")
		} else {
			return errors.NewConfigurationError("CONFIG_READ_ERROR", "Failed to read configuration file").
				WithCause(err).
				WithDetails(fmt.Sprintf("Error reading config: %v", err))
		}
	} else {
		logrus.Infof("Using config file: %s", viper.ConfigFileUsed())
	}

	var config AppConfig
	if err := viper.Unmarshal(&config); err != nil {
		return errors.NewConfigurationError("CONFIG_UNMARSHAL_ERROR", "Failed to parse configuration").
			WithCause(err).
			WithDetails(fmt.Sprintf("Error unmarshaling config: %v", err))
	}

	Config = &config

	if err := validateConfig(); err != nil {
		return err // validateConfig now returns proper AppError
	}

	// Configure logging based on config
	configureLogging()

	logrus.Infof("Configuration loaded successfully for environment: %s", Config.Environment)
	return nil
}

// configureLogging sets up logging based on configuration
func configureLogging() {
	// Set defaults if not configured
	if Config.Logging.Level == "" {
		if Config.Debug {
			Config.Logging.Level = "debug"
		} else {
			Config.Logging.Level = "info"
		}
	}

	if Config.Logging.Format == "" {
		if Config.Environment == "development" {
			Config.Logging.Format = "text"
		} else {
			Config.Logging.Format = "json"
		}
	}

	// Configure global logger
	level := logging.GetLogLevelFromString(Config.Logging.Level)
	logging.ConfigureGlobalLogger(level, Config.Logging.Format)

	logrus.WithFields(logrus.Fields{
		"level":  Config.Logging.Level,
		"format": Config.Logging.Format,
	}).Info("Logging configured")
}

func setDefaults() {
	viper.SetDefault("environment", "development")
	viper.SetDefault("server.port", "8080")
	viper.SetDefault("server.host", "localhost")
	viper.SetDefault("server.log_level", "info")
	viper.SetDefault("server.read_timeout_ms", 5000)
	viper.SetDefault("server.write_timeout_ms", 10000)
	viper.SetDefault("server.shutdown_timeout_ms", 30000)
	viper.SetDefault("server.health_check_timeout_ms", 5000)
	viper.SetDefault("server.idle_conn_timeout_ms", 90000)
	viper.SetDefault("server.worker_stop_timeout_ms", 5000)
	viper.SetDefault("redis.host", "localhost")
	viper.SetDefault("redis.port", "6379")
	viper.SetDefault("redis.password", "")
	viper.SetDefault("redis.db", 0)
	viper.SetDefault("debug", false)
	viper.SetDefault("logging.level", "info")
	viper.SetDefault("logging.format", "json")
	viper.SetDefault("rate_limit.qps", 10.0)
	viper.SetDefault("rate_limit.burst", 20)
	viper.SetDefault("rate_limiting.requests_per_second", 10.0)
	viper.SetDefault("rate_limiting.burst_size", 20)
	viper.SetDefault("buyer.qps", 5.0)
	viper.SetDefault("buyer.burst", 10)
	viper.SetDefault("buyer.timeout_ms", 1000)
	viper.SetDefault("redis.pool_size", 10)
	viper.SetDefault("redis.dial_timeout_ms", 5000)
	viper.SetDefault("redis.read_timeout_ms", 3000)
	viper.SetDefault("redis.write_timeout_ms", 3000)
}

func validateConfig() error {
	if Config.Server.Port == "" {
		return errors.NewValidationError("EMPTY_SERVER_PORT", "Server port cannot be empty")
	}

	if port, err := strconv.Atoi(Config.Server.Port); err != nil || port <= 0 || port > 65535 {
		return errors.NewValidationError("INVALID_SERVER_PORT", "Server port must be a valid port number").
			WithContext("port", Config.Server.Port).
			WithDetails("Port must be between 1 and 65535")
	}

	if Config.Environment == "" {
		return errors.NewValidationError("EMPTY_ENVIRONMENT", "Environment cannot be empty")
	}

	validEnvs := []string{"development", "staging", "production", "test"}
	validEnv := false
	for _, env := range validEnvs {
		if Config.Environment == env {
			validEnv = true
			break
		}
	}
	if !validEnv {
		return errors.NewValidationError("INVALID_ENVIRONMENT", "Invalid environment specified").
			WithContext("environment", Config.Environment).
			WithContext("valid_environments", validEnvs).
			WithDetails(fmt.Sprintf("Environment must be one of: %v", validEnvs))
	}

	return nil
}

func GetConfig() *AppConfig {
	return Config
}

// ResetConfig resets the global config (for testing)
func ResetConfig() {
	Config = nil
}

func IsDevelopment() bool {
	return Config != nil && Config.Environment == "development"
}

func IsProduction() bool {
	return Config != nil && Config.Environment == "production"
}

func GetServerAddress() string {
	if Config == nil {
		return "localhost:8080"
	}
	return Config.Server.Host + ":" + Config.Server.Port
}
