package errors_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/hadarco13/mini-seller/internal/errors"
	"github.com/stretchr/testify/assert"
)

func TestNewValidationError(t *testing.T) {
	err := errors.NewValidationError("INVALID_INPUT", "Input validation failed")

	assert.NotNil(t, err)
	assert.Equal(t, "INVALID_INPUT", err.Code)
	assert.Contains(t, err.Error(), "Input validation failed")
	assert.Equal(t, "validation", string(err.Type))
	assert.Equal(t, http.StatusBadRequest, err.HTTPStatus)
	assert.False(t, err.Retryable) // Default is false
}

func TestNewNetworkError(t *testing.T) {
	err := errors.NewNetworkError("CONNECTION_FAILED", "Failed to connect to service")

	assert.NotNil(t, err)
	assert.Equal(t, "CONNECTION_FAILED", err.Code)
	assert.Contains(t, err.Error(), "Failed to connect to service")
	assert.Equal(t, "network", string(err.Type))
	assert.Equal(t, http.StatusServiceUnavailable, err.HTTPStatus)
	assert.True(t, err.Retryable)
}

func TestNewConfigurationError(t *testing.T) {
	err := errors.NewConfigurationError("INVALID_CONFIG", "Configuration is invalid")

	assert.NotNil(t, err)
	assert.Equal(t, "INVALID_CONFIG", err.Code)
	assert.Contains(t, err.Error(), "Configuration is invalid")
	assert.Equal(t, "configuration", string(err.Type))
	assert.Equal(t, http.StatusInternalServerError, err.HTTPStatus)
	assert.False(t, err.Retryable)
}

func TestNewNotFoundError(t *testing.T) {
	err := errors.NewNotFoundError("user", "123")

	assert.NotNil(t, err)
	assert.Equal(t, "RESOURCE_NOT_FOUND", err.Code)
	assert.Contains(t, err.Error(), "user not found")
	assert.Equal(t, "not_found", string(err.Type))
	assert.Equal(t, http.StatusNotFound, err.HTTPStatus)
	assert.False(t, err.Retryable)
}

func TestNewInternalError(t *testing.T) {
	err := errors.NewInternalError("INTERNAL_ERROR", "Internal server error")

	assert.NotNil(t, err)
	assert.Equal(t, "INTERNAL_ERROR", err.Code)
	assert.Contains(t, err.Error(), "Internal server error")
	assert.Equal(t, "internal", string(err.Type))
	assert.Equal(t, http.StatusInternalServerError, err.HTTPStatus)
	assert.False(t, err.Retryable)
}

func TestAppError_WithContext(t *testing.T) {
	err := errors.NewValidationError("TEST_ERROR", "Test error").
		WithContext("field", "username").
		WithContext("value", "invalid_user")

	assert.Contains(t, err.Context, "field")
	assert.Contains(t, err.Context, "value")
	assert.Equal(t, "username", err.Context["field"])
	assert.Equal(t, "invalid_user", err.Context["value"])
}

func TestAppError_WithCause(t *testing.T) {
	originalErr := fmt.Errorf("original error")
	err := errors.NewValidationError("TEST_ERROR", "Test error").
		WithCause(originalErr)

	assert.Equal(t, originalErr, err.Cause)
}

func TestAppError_WithUserMessage(t *testing.T) {
	userMessage := "Please provide a valid username"
	err := errors.NewValidationError("TEST_ERROR", "Test error").
		WithUserMessage(userMessage)

	assert.Equal(t, userMessage, err.UserMessage)
}

func TestAppError_WithDetails(t *testing.T) {
	details := "Field validation failed: username must be alphanumeric"
	err := errors.NewValidationError("TEST_ERROR", "Test error").
		WithDetails(details)

	assert.Equal(t, details, err.Details)
}

func TestAppError_WithSeverity(t *testing.T) {
	err := errors.NewValidationError("TEST_ERROR", "Test error").
		WithSeverity("high")

	assert.Equal(t, "high", string(err.Severity))
}

func TestAppError_WithHTTPStatus(t *testing.T) {
	err := errors.NewValidationError("TEST_ERROR", "Test error").
		WithHTTPStatus(http.StatusConflict)

	assert.Equal(t, http.StatusConflict, err.HTTPStatus)
}

func TestAppError_WithRetryable(t *testing.T) {
	err := errors.NewValidationError("TEST_ERROR", "Test error").
		WithRetryable(true)

	assert.True(t, err.Retryable)
}

func TestWrap(t *testing.T) {
	originalErr := fmt.Errorf("database connection failed")
	wrappedErr := errors.Wrap(originalErr, errors.ErrorTypeNetwork, "DB_CONNECTION_ERROR", "Database operation failed")

	assert.NotNil(t, wrappedErr)
	assert.Equal(t, "DB_CONNECTION_ERROR", wrappedErr.Code)
	assert.Contains(t, wrappedErr.Error(), "Database operation failed")
	assert.Equal(t, originalErr, wrappedErr.Cause)
}

func TestGetHTTPStatus(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected int
	}{
		{
			name:     "Validation error",
			err:      errors.NewValidationError("VALIDATION_ERROR", "Validation failed"),
			expected: http.StatusBadRequest,
		},
		{
			name:     "Network error",
			err:      errors.NewNetworkError("NETWORK_ERROR", "Network failed"),
			expected: http.StatusServiceUnavailable,
		},
		{
			name:     "Not found error",
			err:      errors.NewNotFoundError("resource", "123"),
			expected: http.StatusNotFound,
		},
		{
			name:     "Regular error",
			err:      fmt.Errorf("regular error"),
			expected: http.StatusInternalServerError,
		},
		{
			name:     "Nil error",
			err:      nil,
			expected: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, errors.GetHTTPStatus(tt.err))
		})
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "Retryable network error",
			err:      errors.NewNetworkError("NETWORK_ERROR", "Network failed"),
			expected: true,
		},
		{
			name:     "Non-retryable validation error",
			err:      errors.NewValidationError("VALIDATION_ERROR", "Validation failed"),
			expected: false,
		},
		{
			name:     "Regular error defaults to false",
			err:      fmt.Errorf("regular error"),
			expected: false,
		},
		{
			name:     "Nil error defaults to false",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, errors.IsRetryable(tt.err))
		})
	}
}

func TestIsType(t *testing.T) {
	validationErr := errors.NewValidationError("TEST_ERROR", "Test error")
	networkErr := errors.NewNetworkError("NETWORK_ERROR", "Network error")

	assert.True(t, errors.IsType(validationErr, errors.ErrorTypeValidation))
	assert.False(t, errors.IsType(validationErr, errors.ErrorTypeNetwork))
	assert.True(t, errors.IsType(networkErr, errors.ErrorTypeNetwork))
	assert.False(t, errors.IsType(networkErr, errors.ErrorTypeValidation))

	// Test with non-AppError
	regularErr := fmt.Errorf("regular error")
	assert.False(t, errors.IsType(regularErr, errors.ErrorTypeValidation))

	// Test with nil
	assert.False(t, errors.IsType(nil, errors.ErrorTypeValidation))
}

func TestAppError_LogError(t *testing.T) {
	err := errors.NewValidationError("TEST_ERROR", "Test error").
		WithContext("field", "username")

	// This should not panic
	err.LogError()
}

func TestAppError_ToJSON(t *testing.T) {
	err := errors.NewValidationError("TEST_ERROR", "Test error").
		WithContext("field", "username").
		WithDetails("Validation failed")

	jsonBytes := err.ToJSON()
	assert.NotNil(t, jsonBytes)
	assert.Greater(t, len(jsonBytes), 0)
}

func TestAppError_ChainedMethods(t *testing.T) {
	originalErr := fmt.Errorf("original cause")

	err := errors.NewValidationError("CHAINED_ERROR", "Base error").
		WithContext("field", "email").
		WithContext("reason", "invalid format").
		WithCause(originalErr).
		WithUserMessage("Please provide a valid email address").
		WithDetails("Email validation failed: format should be user@domain.com").
		WithSeverity("medium").
		WithHTTPStatus(http.StatusUnprocessableEntity).
		WithRetryable(true)

	assert.Equal(t, "CHAINED_ERROR", err.Code)
	assert.Contains(t, err.Error(), "Base error")
	assert.Equal(t, "validation", string(err.Type))
	assert.Equal(t, originalErr, err.Cause)
	assert.Equal(t, "Please provide a valid email address", err.UserMessage)
	assert.Equal(t, "Email validation failed: format should be user@domain.com", err.Details)
	assert.Equal(t, "email", err.Context["field"])
	assert.Equal(t, "invalid format", err.Context["reason"])
	assert.Equal(t, "medium", string(err.Severity))
	assert.Equal(t, http.StatusUnprocessableEntity, err.HTTPStatus)
	assert.True(t, err.Retryable)
}

// Benchmark tests
func BenchmarkNewValidationError(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		errors.NewValidationError("TEST_ERROR", "Test error message")
	}
}

func BenchmarkAppError_WithContext(b *testing.B) {
	err := errors.NewValidationError("TEST_ERROR", "Test error")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		err.WithContext("key", "value")
	}
}
