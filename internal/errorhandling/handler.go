package errorhandling

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hadarco13/mini-seller/internal/errors"
	"github.com/sirupsen/logrus"
)

// ErrorHandler provides comprehensive error handling
type ErrorHandler struct {
	maxRetries     int
	backoffFactor  time.Duration
	circuitBreaker *CircuitBreaker
}

// NewErrorHandler creates a new error handler
func NewErrorHandler(maxRetries int, backoffFactor time.Duration) *ErrorHandler {
	return &ErrorHandler{
		maxRetries:     maxRetries,
		backoffFactor:  backoffFactor,
		circuitBreaker: NewCircuitBreaker(5, time.Minute), // 5 failures per minute
	}
}

// ExecuteWithRetry executes operation with retry logic
func (eh *ErrorHandler) ExecuteWithRetry(ctx context.Context, operation func() error, operationName string) error {
	var lastErr error

	for attempt := 0; attempt <= eh.maxRetries; attempt++ {
		// Check circuit breaker
		if !eh.circuitBreaker.Allow() {
			return errors.NewNetworkError("CIRCUIT_BREAKER_OPEN",
				fmt.Sprintf("Circuit breaker open for operation: %s", operationName))
		}

		// Execute operation
		err := operation()
		if err == nil {
			eh.circuitBreaker.RecordSuccess()
			return nil
		}

		eh.circuitBreaker.RecordFailure()
		lastErr = err

		// Log retry attempt
		logrus.WithFields(logrus.Fields{
			"operation":   operationName,
			"attempt":     attempt + 1,
			"max_retries": eh.maxRetries,
			"error":       err.Error(),
		}).Warn("Operation failed, retrying")

		// Check if we should retry
		if !eh.shouldRetry(err) {
			logrus.WithField("operation", operationName).Info("Non-retryable error, stopping")
			break
		}

		// Wait before retry (exponential backoff)
		if attempt < eh.maxRetries {
			backoff := eh.calculateBackoff(attempt)

			select {
			case <-time.After(backoff):
				// Continue to retry
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	return fmt.Errorf("operation '%s' failed after %d retries: %w", operationName, eh.maxRetries, lastErr)
}

// shouldRetry determines if an error is retryable
func (eh *ErrorHandler) shouldRetry(err error) bool {
	// Don't retry validation errors
	if appErr, ok := err.(*errors.AppError); ok {
		return appErr.Type != "validation"
	}

	// Retry network errors and temporary failures
	return true
}

// calculateBackoff calculates exponential backoff with jitter
func (eh *ErrorHandler) calculateBackoff(attempt int) time.Duration {
	backoff := eh.backoffFactor * time.Duration(1<<uint(attempt)) // Exponential

	// Add jitter (up to 50% of backoff time)
	jitter := time.Duration(float64(backoff) * 0.5 * (0.5 + 0.5*float64(attempt%2)))

	return backoff + jitter
}

// CircuitBreaker implements circuit breaker pattern
type CircuitBreaker struct {
	mu           sync.RWMutex
	failures     int
	maxFailures  int
	resetTimeout time.Duration
	lastFailure  time.Time
	state        CircuitState
}

// CircuitState represents circuit breaker states
type CircuitState int

const (
	Closed CircuitState = iota
	Open
	HalfOpen
)

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(maxFailures int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		maxFailures:  maxFailures,
		resetTimeout: resetTimeout,
		state:        Closed,
	}
}

// Allow checks if operation is allowed
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	switch cb.state {
	case Closed:
		return true
	case Open:
		// Check if reset timeout has passed
		if time.Since(cb.lastFailure) > cb.resetTimeout {
			cb.mu.RUnlock()
			cb.mu.Lock()
			if cb.state == Open && time.Since(cb.lastFailure) > cb.resetTimeout {
				cb.state = HalfOpen
				logrus.Info("Circuit breaker moving to half-open state")
			}
			cb.mu.Unlock()
			cb.mu.RLock()
			return cb.state == HalfOpen
		}
		return false
	case HalfOpen:
		return true
	default:
		return false
	}
}

// RecordSuccess records a successful operation
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures = 0
	if cb.state == HalfOpen {
		cb.state = Closed
		logrus.Info("Circuit breaker closed after successful operation")
	}
}

// RecordFailure records a failed operation
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailure = time.Now()

	if cb.failures >= cb.maxFailures {
		cb.state = Open
		logrus.WithFields(logrus.Fields{
			"failures":     cb.failures,
			"max_failures": cb.maxFailures,
		}).Warn("Circuit breaker opened due to failures")
	}
}

// GetState returns current circuit breaker state
func (cb *CircuitBreaker) GetState() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// ErrorRecovery provides error recovery mechanisms
type ErrorRecovery struct {
	fallbackHandlers map[string]func() (interface{}, error)
	mu               sync.RWMutex
}

// NewErrorRecovery creates a new error recovery handler
func NewErrorRecovery() *ErrorRecovery {
	return &ErrorRecovery{
		fallbackHandlers: make(map[string]func() (interface{}, error)),
	}
}

// RegisterFallback registers a fallback handler for specific operation
func (er *ErrorRecovery) RegisterFallback(operation string, handler func() (interface{}, error)) {
	er.mu.Lock()
	defer er.mu.Unlock()
	er.fallbackHandlers[operation] = handler
}

// ExecuteWithFallback executes operation with fallback on failure
func (er *ErrorRecovery) ExecuteWithFallback(operation string, primaryFunc func() (interface{}, error)) (interface{}, error) {
	// Try primary operation
	result, err := primaryFunc()
	if err == nil {
		return result, nil
	}

	logrus.WithFields(logrus.Fields{
		"operation": operation,
		"error":     err.Error(),
	}).Warn("Primary operation failed, trying fallback")

	// Try fallback
	er.mu.RLock()
	fallback, exists := er.fallbackHandlers[operation]
	er.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("no fallback available for operation '%s': %w", operation, err)
	}

	fallbackResult, fallbackErr := fallback()
	if fallbackErr != nil {
		return nil, fmt.Errorf("both primary and fallback failed for operation '%s': primary=%w, fallback=%w",
			operation, err, fallbackErr)
	}

	logrus.WithField("operation", operation).Info("Fallback operation succeeded")
	return fallbackResult, nil
}

// TimeoutHandler handles operation timeouts
type TimeoutHandler struct {
	defaultTimeout time.Duration
}

// NewTimeoutHandler creates a new timeout handler
func NewTimeoutHandler(defaultTimeout time.Duration) *TimeoutHandler {
	return &TimeoutHandler{
		defaultTimeout: defaultTimeout,
	}
}

// ExecuteWithTimeout executes operation with timeout
func (th *TimeoutHandler) ExecuteWithTimeout(ctx context.Context, operation func(ctx context.Context) error, timeout time.Duration) error {
	if timeout == 0 {
		timeout = th.defaultTimeout
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	done := make(chan error, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("panic in operation: %v", r)
			}
		}()
		done <- operation(timeoutCtx)
	}()

	select {
	case err := <-done:
		return err
	case <-timeoutCtx.Done():
		return fmt.Errorf("operation timeout after %v: %w", timeout, timeoutCtx.Err())
	}
}
