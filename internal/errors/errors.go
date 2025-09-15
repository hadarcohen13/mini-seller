package errors

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/sirupsen/logrus"
)

// ErrorType represents different categories of errors
type ErrorType string

const (
	ErrorTypeValidation    ErrorType = "validation"
	ErrorTypeConfiguration ErrorType = "configuration"
	ErrorTypeNetwork       ErrorType = "network"
	ErrorTypeDatabase      ErrorType = "database"
	ErrorTypeAuth          ErrorType = "authentication"
	ErrorTypeAuthorization ErrorType = "authorization"
	ErrorTypeNotFound      ErrorType = "not_found"
	ErrorTypeInternal      ErrorType = "internal"
	ErrorTypeExternal      ErrorType = "external"
)

// Severity levels for errors
type Severity string

const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

// AppError represents a structured application error
type AppError struct {
	Type        ErrorType              `json:"type"`
	Code        string                 `json:"code"`
	Message     string                 `json:"message"`
	Details     string                 `json:"details,omitempty"`
	Timestamp   time.Time              `json:"timestamp"`
	Severity    Severity               `json:"severity"`
	Context     map[string]interface{} `json:"context,omitempty"`
	StackTrace  string                 `json:"stack_trace,omitempty"`
	Cause       error                  `json:"-"` // Original error, not serialized
	HTTPStatus  int                    `json:"-"` // HTTP status code
	Retryable   bool                   `json:"retryable"`
	UserMessage string                 `json:"user_message,omitempty"` // Safe message for users
}

// Error implements the error interface
func (e *AppError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("%s: %s (%s)", e.Code, e.Message, e.Details)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// WithContext adds contextual information to the error
func (e *AppError) WithContext(key string, value interface{}) *AppError {
	if e.Context == nil {
		e.Context = make(map[string]interface{})
	}
	e.Context[key] = value
	return e
}

// WithCause sets the underlying cause of the error
func (e *AppError) WithCause(cause error) *AppError {
	e.Cause = cause
	return e
}

// WithDetails adds additional details to the error
func (e *AppError) WithDetails(details string) *AppError {
	e.Details = details
	return e
}

// WithUserMessage sets a user-friendly message
func (e *AppError) WithUserMessage(msg string) *AppError {
	e.UserMessage = msg
	return e
}

// New creates a new AppError
func New(errorType ErrorType, code, message string) *AppError {
	return &AppError{
		Type:       errorType,
		Code:       code,
		Message:    message,
		Timestamp:  time.Now(),
		Severity:   SeverityMedium,
		HTTPStatus: http.StatusInternalServerError,
		Retryable:  false,
	}
}

// Wrap wraps an existing error with AppError
func Wrap(err error, errorType ErrorType, code, message string) *AppError {
	appErr := New(errorType, code, message).WithCause(err)

	// Capture stack trace
	buf := make([]byte, 1024)
	runtime.Stack(buf, false)
	appErr.StackTrace = string(buf)

	return appErr
}

// Predefined error constructors

// NewValidationError creates a validation error
func NewValidationError(code, message string) *AppError {
	return New(ErrorTypeValidation, code, message).
		WithSeverity(SeverityLow).
		WithHTTPStatus(http.StatusBadRequest).
		WithUserMessage("Please check your input and try again")
}

// NewConfigurationError creates a configuration error
func NewConfigurationError(code, message string) *AppError {
	return New(ErrorTypeConfiguration, code, message).
		WithSeverity(SeverityCritical).
		WithHTTPStatus(http.StatusInternalServerError).
		WithUserMessage("Service is temporarily unavailable")
}

// NewNotFoundError creates a not found error
func NewNotFoundError(resource, identifier string) *AppError {
	return New(ErrorTypeNotFound, "RESOURCE_NOT_FOUND", fmt.Sprintf("%s not found", resource)).
		WithContext("resource", resource).
		WithContext("identifier", identifier).
		WithSeverity(SeverityLow).
		WithHTTPStatus(http.StatusNotFound).
		WithUserMessage(fmt.Sprintf("The requested %s was not found", resource))
}

// NewInternalError creates an internal server error
func NewInternalError(code, message string) *AppError {
	return New(ErrorTypeInternal, code, message).
		WithSeverity(SeverityHigh).
		WithHTTPStatus(http.StatusInternalServerError).
		WithUserMessage("An unexpected error occurred. Please try again later")
}

// NewNetworkError creates a network-related error
func NewNetworkError(code, message string) *AppError {
	return New(ErrorTypeNetwork, code, message).
		WithSeverity(SeverityMedium).
		WithHTTPStatus(http.StatusServiceUnavailable).
		WithRetryable(true).
		WithUserMessage("Network error occurred. Please try again")
}

// Helper methods for setting properties

func (e *AppError) WithSeverity(severity Severity) *AppError {
	e.Severity = severity
	return e
}

func (e *AppError) WithHTTPStatus(status int) *AppError {
	e.HTTPStatus = status
	return e
}

func (e *AppError) WithRetryable(retryable bool) *AppError {
	e.Retryable = retryable
	return e
}

// LogError logs the error with appropriate level based on severity
func (e *AppError) LogError() {
	fields := logrus.Fields{
		"error_type":  e.Type,
		"error_code":  e.Code,
		"severity":    e.Severity,
		"retryable":   e.Retryable,
		"http_status": e.HTTPStatus,
		"timestamp":   e.Timestamp,
	}

	// Add context fields
	for key, value := range e.Context {
		fields[key] = value
	}

	// Add cause if present
	if e.Cause != nil {
		fields["cause"] = e.Cause.Error()
	}

	entry := logrus.WithFields(fields)

	switch e.Severity {
	case SeverityLow:
		entry.Info(e.Message)
	case SeverityMedium:
		entry.Warn(e.Message)
	case SeverityHigh:
		entry.Error(e.Message)
	case SeverityCritical:
		entry.Fatal(e.Message)
	default:
		entry.Error(e.Message)
	}
}

// ToJSON converts the error to JSON for API responses
func (e *AppError) ToJSON() []byte {
	response := struct {
		Error struct {
			Type        ErrorType              `json:"type"`
			Code        string                 `json:"code"`
			Message     string                 `json:"message"`
			Timestamp   time.Time              `json:"timestamp"`
			Retryable   bool                   `json:"retryable"`
			UserMessage string                 `json:"user_message,omitempty"`
			Context     map[string]interface{} `json:"context,omitempty"`
		} `json:"error"`
	}{}

	response.Error.Type = e.Type
	response.Error.Code = e.Code
	response.Error.Message = e.UserMessage
	if response.Error.Message == "" {
		response.Error.Message = e.Message
	}
	response.Error.Timestamp = e.Timestamp
	response.Error.Retryable = e.Retryable
	response.Error.Context = e.Context

	jsonBytes, _ := json.Marshal(response)
	return jsonBytes
}

// IsType checks if the error is of a specific type
func IsType(err error, errorType ErrorType) bool {
	if appErr, ok := err.(*AppError); ok {
		return appErr.Type == errorType
	}
	return false
}

// GetHTTPStatus returns the HTTP status code for the error
func GetHTTPStatus(err error) int {
	if appErr, ok := err.(*AppError); ok {
		return appErr.HTTPStatus
	}
	return http.StatusInternalServerError
}

// IsRetryable checks if the error is retryable
func IsRetryable(err error) bool {
	if appErr, ok := err.(*AppError); ok {
		return appErr.Retryable
	}
	return false
}
