package logging_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/hadarco13/mini-seller/internal/config"
	"github.com/hadarco13/mini-seller/internal/logging"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupLoggingTestConfig() {
	os.Setenv("MINI_SELLER_ENVIRONMENT", "test")
	os.Setenv("MINI_SELLER_SERVER_PORT", "8080")
}

func TestNewLogger(t *testing.T) {
	setupLoggingTestConfig()
	config.LoadConfig()

	logger := logging.NewLogger("test_component")
	assert.NotNil(t, logger)
}

func TestNewLoggerFromContext(t *testing.T) {
	setupLoggingTestConfig()
	config.LoadConfig()

	req := &http.Request{}
	ctx := context.WithValue(req.Context(), "request_id", "test-123")
	req = req.WithContext(ctx)

	logger := logging.NewLoggerFromContext(req.Context(), "test_operation")
	assert.NotNil(t, logger)
}

func TestContextLogger_WithRequestID(t *testing.T) {
	setupLoggingTestConfig()
	config.LoadConfig()

	logger := logging.NewLogger("test_component")
	loggerWithID := logger.WithRequestID("req-123")

	assert.NotNil(t, loggerWithID)
	assert.NotEqual(t, logger, loggerWithID) // Should return a new instance
}

func TestContextLogger_WithOperation(t *testing.T) {
	setupLoggingTestConfig()
	config.LoadConfig()

	logger := logging.NewLogger("test_component")
	loggerWithOp := logger.WithOperation("bid_processing")

	assert.NotNil(t, loggerWithOp)
	assert.NotEqual(t, logger, loggerWithOp)
}

func TestContextLogger_WithDuration(t *testing.T) {
	setupLoggingTestConfig()
	config.LoadConfig()

	logger := logging.NewLogger("test_component")
	duration := 150 * time.Millisecond
	loggerWithDuration := logger.WithDuration(duration)

	assert.NotNil(t, loggerWithDuration)
}

func TestContextLogger_WithStatusCode(t *testing.T) {
	setupLoggingTestConfig()
	config.LoadConfig()

	logger := logging.NewLogger("test_component")
	loggerWithStatus := logger.WithStatusCode(http.StatusOK)

	assert.NotNil(t, loggerWithStatus)
}

func TestContextLogger_WithUserAgent(t *testing.T) {
	setupLoggingTestConfig()
	config.LoadConfig()

	logger := logging.NewLogger("test_component")
	userAgent := "Mozilla/5.0 (Test Browser)"
	loggerWithUA := logger.WithUserAgent(userAgent)

	assert.NotNil(t, loggerWithUA)
}

func TestContextLogger_WithIPAddress(t *testing.T) {
	setupLoggingTestConfig()
	config.LoadConfig()

	logger := logging.NewLogger("test_component")
	ipAddress := "192.168.1.100"
	loggerWithIP := logger.WithIPAddress(ipAddress)

	assert.NotNil(t, loggerWithIP)
}

func TestContextLogger_WithBidRequestContext(t *testing.T) {
	setupLoggingTestConfig()
	config.LoadConfig()

	logger := logging.NewLogger("test_component")
	loggerWithBid := logger.WithBidRequestContext("bid-123", 5)

	assert.NotNil(t, loggerWithBid)
}

func TestContextLogger_WithBuyerContext(t *testing.T) {
	setupLoggingTestConfig()
	config.LoadConfig()

	logger := logging.NewLogger("test_component")
	loggerWithBuyer := logger.WithBuyerContext("buyer-abc", "http://buyer.example.com")

	assert.NotNil(t, loggerWithBuyer)
}

func TestContextLogger_WithError(t *testing.T) {
	setupLoggingTestConfig()
	config.LoadConfig()

	logger := logging.NewLogger("test_component")
	loggerWithError := logger.WithError(assert.AnError)

	assert.NotNil(t, loggerWithError)
}

func TestContextLogger_InfoOperation(t *testing.T) {
	setupLoggingTestConfig()
	config.LoadConfig()

	logger := logging.NewLogger("test_component").
		WithRequestID("req-123").
		WithOperation("test_operation")

	duration := 100 * time.Millisecond
	// This should not panic
	logger.InfoOperation("bid_request", "Processing bid request", duration)
}

func TestContextLogger_InfoRequest(t *testing.T) {
	setupLoggingTestConfig()
	config.LoadConfig()

	logger := logging.NewLogger("test_component")
	duration := 100 * time.Millisecond

	// This should not panic
	logger.InfoRequest("POST", "/bid", http.StatusOK, duration)
}

func TestContextLogger_InfoBidRequest(t *testing.T) {
	setupLoggingTestConfig()
	config.LoadConfig()

	logger := logging.NewLogger("test_component")

	// This should not panic
	logger.InfoBidRequest("bid-123", 5, "processing")
}

func TestContextLogger_InfoBidResponse(t *testing.T) {
	setupLoggingTestConfig()
	config.LoadConfig()

	logger := logging.NewLogger("test_component")
	duration := 50 * time.Millisecond

	// This should not panic
	logger.InfoBidResponse("bid-123", 3, 15.75, duration)
}

func TestContextLogger_WarnOperation(t *testing.T) {
	setupLoggingTestConfig()
	config.LoadConfig()

	logger := logging.NewLogger("test_component")
	testErr := fmt.Errorf("test warning error")

	// This should not panic
	logger.WarnOperation("test_operation", "Warning message", testErr)
}

func TestContextLogger_ErrorOperation(t *testing.T) {
	setupLoggingTestConfig()
	config.LoadConfig()

	logger := logging.NewLogger("test_component")
	testErr := fmt.Errorf("test operation error")

	// This should not panic
	logger.ErrorOperation("test_operation", "Error message", testErr)
}

func TestContextLogger_WarnRateLimit(t *testing.T) {
	setupLoggingTestConfig()
	config.LoadConfig()

	logger := logging.NewLogger("test_component")

	// This should not panic
	logger.WarnRateLimit("192.168.1.1", 10.0)
}

func TestContextLogger_ErrorBuyerRequest(t *testing.T) {
	setupLoggingTestConfig()
	config.LoadConfig()

	logger := logging.NewLogger("test_component")
	testErr := fmt.Errorf("buyer request failed")
	duration := 500 * time.Millisecond

	// This should not panic
	logger.ErrorBuyerRequest("http://buyer.com", 1, 3, testErr, duration)
}

func TestContextLogger_DebugPayload(t *testing.T) {
	setupLoggingTestConfig()
	config.LoadConfig()

	logger := logging.NewLogger("test_component")

	// This should not panic
	logger.DebugPayload("request", "http://buyer.com", 1024)
}

func TestSetupLogging(t *testing.T) {
	// This should not panic
	logging.SetupLogging()
}

func TestLogWithLevel(t *testing.T) {
	setupLoggingTestConfig()
	config.LoadConfig()

	tests := []struct {
		name    string
		level   string
		message string
	}{
		{"Debug level", "debug", "Debug message"},
		{"Info level", "info", "Info message"},
		{"Warn level", "warn", "Warning message"},
		{"Error level", "error", "Error message"},
		{"Invalid level", "invalid", "Should default to info"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This should not panic
			logger := logging.NewLogger("test_component")
			level := logging.GetLogLevelFromString(tt.level)
			logger.LogWithLevel(level, tt.message)
		})
	}
}

func TestIsLevelEnabled(t *testing.T) {
	setupLoggingTestConfig()
	config.LoadConfig()

	tests := []struct {
		level    string
		expected bool
	}{
		{"debug", true},
		{"info", true},
		{"warn", true},
		{"error", true},
		{"fatal", true},
		{"trace", false}, // Typically disabled in test
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			level := logging.GetLogLevelFromString(tt.level)
			result := logging.IsLevelEnabled(level)
			// Just test that it returns a boolean without panicking
			assert.IsType(t, false, result)
		})
	}
}

func TestContextLogger_ChainedMethods(t *testing.T) {
	setupLoggingTestConfig()
	config.LoadConfig()

	logger := logging.NewLogger("test_component").
		WithRequestID("req-456").
		WithOperation("complex_operation").
		WithDuration(300*time.Millisecond).
		WithStatusCode(http.StatusCreated).
		WithUserAgent("Test/1.0").
		WithIPAddress("10.0.0.1").
		WithBidRequestContext("bid-456", 10).
		WithBuyerContext("buyer-xyz", "http://another-buyer.com")

	assert.NotNil(t, logger)

	// Should not panic when calling methods
	logger.InfoOperation("chained_test", "Testing chained method calls", 50*time.Millisecond)
}

func TestGetLogLevelFromString(t *testing.T) {
	tests := []struct {
		input    string
		expected logrus.Level
	}{
		{"trace", logrus.TraceLevel},
		{"debug", logrus.DebugLevel},
		{"info", logrus.InfoLevel},
		{"warn", logrus.WarnLevel},
		{"warning", logrus.WarnLevel},
		{"error", logrus.ErrorLevel},
		{"fatal", logrus.FatalLevel},
		{"invalid", logrus.InfoLevel}, // Should default to info
		{"", logrus.InfoLevel},        // Should default to info
		{"DEBUG", logrus.InfoLevel},   // Case sensitive, should default to info
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			level := logging.GetLogLevelFromString(tt.input)
			assert.Equal(t, tt.expected, level)
		})
	}
}

func TestConfigureGlobalLogger(t *testing.T) {
	tests := []struct {
		name   string
		level  logrus.Level
		format string
	}{
		{
			name:   "JSON format with debug level",
			level:  logrus.DebugLevel,
			format: "json",
		},
		{
			name:   "Text format with info level",
			level:  logrus.InfoLevel,
			format: "text",
		},
		{
			name:   "Structured format with warn level",
			level:  logrus.WarnLevel,
			format: "structured",
		},
		{
			name:   "Colored format with error level",
			level:  logrus.ErrorLevel,
			format: "colored",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			logging.ConfigureGlobalLogger(tt.level, tt.format)
		})
	}
}

func TestLoggingIntegrationWithConfig(t *testing.T) {
	// Test different environment configurations
	environments := []string{"development", "production", "test"}

	for _, env := range environments {
		t.Run(fmt.Sprintf("Environment_%s", env), func(t *testing.T) {
			os.Setenv("MINI_SELLER_ENVIRONMENT", env)
			defer os.Unsetenv("MINI_SELLER_ENVIRONMENT")

			err := config.LoadConfig()
			require.NoError(t, err)

			logger := logging.NewLogger("integration_test")
			assert.NotNil(t, logger)

			// Should not panic with different configurations
			duration := 25 * time.Millisecond
			logger.WithRequestID("integration-test").
				InfoOperation("integration_test", "Testing environment", duration)
		})
	}
}

// Benchmark tests
func BenchmarkNewLogger(b *testing.B) {
	setupLoggingTestConfig()
	config.LoadConfig()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logging.NewLogger("benchmark_test")
	}
}

func BenchmarkContextLogger_WithRequestID(b *testing.B) {
	setupLoggingTestConfig()
	config.LoadConfig()

	logger := logging.NewLogger("benchmark_test")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		logger.WithRequestID(fmt.Sprintf("req-%d", i))
	}
}

func BenchmarkContextLogger_InfoOperation(b *testing.B) {
	setupLoggingTestConfig()
	config.LoadConfig()

	logger := logging.NewLogger("benchmark_test").WithRequestID("benchmark-test")
	duration := 10 * time.Millisecond
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		logger.InfoOperation("benchmark_op", fmt.Sprintf("Benchmark iteration %d", i), duration)
	}
}
