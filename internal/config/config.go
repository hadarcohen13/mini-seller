package config

import (
	"fmt"
	"strconv"

	"github.com/hadarco13/mini-seller/internal/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type ServerConfig struct {
	Port     string `mapstructure:"port" yaml:"port"`
	Host     string `mapstructure:"host" yaml:"host"`
	LogLevel string `mapstructure:"log_level" yaml:"log_level"`
}

type DatabaseConfig struct {
	Host     string `mapstructure:"host" yaml:"host"`
	Port     int    `mapstructure:"port" yaml:"port"`
	User     string `mapstructure:"user" yaml:"user"`
	Password string `mapstructure:"password" yaml:"password"`
	Name     string `mapstructure:"name" yaml:"name"`
	SSLMode  string `mapstructure:"ssl_mode" yaml:"ssl_mode"`
}

type AppConfig struct {
	Environment string         `mapstructure:"environment" yaml:"environment"`
	Server      ServerConfig   `mapstructure:"server" yaml:"server"`
	Database    DatabaseConfig `mapstructure:"database" yaml:"database"`
	Debug       bool           `mapstructure:"debug" yaml:"debug"`
}

var Config *AppConfig

func LoadConfig() error {
	viper.SetConfigName("config")
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

	logrus.Infof("Configuration loaded successfully for environment: %s", Config.Environment)
	return nil
}

func setDefaults() {
	viper.SetDefault("environment", "development")
	viper.SetDefault("server.port", "8080")
	viper.SetDefault("server.host", "localhost")
	viper.SetDefault("server.log_level", "info")
	viper.SetDefault("database.host", "localhost")
	viper.SetDefault("database.port", 5432)
	viper.SetDefault("database.user", "postgres")
	viper.SetDefault("database.name", "mini_seller")
	viper.SetDefault("database.ssl_mode", "disable")
	viper.SetDefault("debug", false)
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
	if Config == nil {
		logrus.Fatal("Config not loaded. Call LoadConfig() first.")
	}
	return Config
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
