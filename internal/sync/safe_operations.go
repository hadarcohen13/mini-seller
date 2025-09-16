package sync

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// SafeCounter provides thread-safe counter operations
type SafeCounter struct {
	mu    sync.RWMutex
	value int64
}

// NewSafeCounter creates a new thread-safe counter
func NewSafeCounter() *SafeCounter {
	return &SafeCounter{}
}

// Increment safely increments the counter
func (sc *SafeCounter) Increment() int64 {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.value++
	return sc.value
}

// Decrement safely decrements the counter
func (sc *SafeCounter) Decrement() int64 {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.value--
	return sc.value
}

// Get safely gets the current value
func (sc *SafeCounter) Get() int64 {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.value
}

// Set safely sets the value
func (sc *SafeCounter) Set(value int64) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.value = value
}

// SafeMap provides thread-safe map operations
type SafeMap struct {
	mu   sync.RWMutex
	data map[string]interface{}
}

// NewSafeMap creates a new thread-safe map
func NewSafeMap() *SafeMap {
	return &SafeMap{
		data: make(map[string]interface{}),
	}
}

// Set safely sets a key-value pair
func (sm *SafeMap) Set(key string, value interface{}) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.data[key] = value
}

// Get safely gets a value by key
func (sm *SafeMap) Get(key string) (interface{}, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	value, exists := sm.data[key]
	return value, exists
}

// Delete safely deletes a key
func (sm *SafeMap) Delete(key string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.data, key)
}

// Keys safely returns all keys
func (sm *SafeMap) Keys() []string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	keys := make([]string, 0, len(sm.data))
	for k := range sm.data {
		keys = append(keys, k)
	}
	return keys
}

// Size safely returns the map size
func (sm *SafeMap) Size() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.data)
}

// ErrorCollector collects errors from multiple goroutines safely
type ErrorCollector struct {
	mu     sync.Mutex
	errors []error
}

// NewErrorCollector creates a new error collector
func NewErrorCollector() *ErrorCollector {
	return &ErrorCollector{
		errors: make([]error, 0),
	}
}

// Add safely adds an error to the collection
func (ec *ErrorCollector) Add(err error) {
	if err == nil {
		return
	}

	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.errors = append(ec.errors, err)
}

// GetErrors safely returns all collected errors
func (ec *ErrorCollector) GetErrors() []error {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	// Return a copy to prevent external modification
	result := make([]error, len(ec.errors))
	copy(result, ec.errors)
	return result
}

// HasErrors safely checks if there are any errors
func (ec *ErrorCollector) HasErrors() bool {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	return len(ec.errors) > 0
}

// FirstError returns the first error or nil
func (ec *ErrorCollector) FirstError() error {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	if len(ec.errors) == 0 {
		return nil
	}
	return ec.errors[0]
}

// Clear safely clears all errors
func (ec *ErrorCollector) Clear() {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.errors = ec.errors[:0]
}

// SafeExecution provides safe execution with timeout and recovery
type SafeExecution struct {
	timeout time.Duration
	retries int
}

// NewSafeExecution creates a new safe execution handler
func NewSafeExecution(timeout time.Duration, retries int) *SafeExecution {
	return &SafeExecution{
		timeout: timeout,
		retries: retries,
	}
}

// Execute safely executes a function with timeout and retry
func (se *SafeExecution) Execute(ctx context.Context, operation func(ctx context.Context) error) error {
	var lastErr error

	for attempt := 0; attempt <= se.retries; attempt++ {
		// Create timeout context
		timeoutCtx, cancel := context.WithTimeout(ctx, se.timeout)

		// Execute with panic recovery
		err := se.executeWithRecover(timeoutCtx, operation)
		cancel()

		if err == nil {
			return nil
		}

		lastErr = err

		if attempt < se.retries {
			logrus.WithFields(logrus.Fields{
				"attempt": attempt + 1,
				"retries": se.retries,
				"error":   err.Error(),
			}).Warn("Operation failed, retrying")

			// Exponential backoff
			backoff := time.Duration(attempt+1) * 100 * time.Millisecond
			time.Sleep(backoff)
		}
	}

	return fmt.Errorf("operation failed after %d retries: %w", se.retries, lastErr)
}

// executeWithRecover executes function with panic recovery
func (se *SafeExecution) executeWithRecover(ctx context.Context, operation func(ctx context.Context) error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic recovered: %v", r)
			logrus.WithField("panic", r).Error("Operation panicked")
		}
	}()

	return operation(ctx)
}

// Coordinator coordinates multiple operations with proper synchronization
type Coordinator struct {
	wg     sync.WaitGroup
	errors *ErrorCollector
	ctx    context.Context
	cancel context.CancelFunc
}

// NewCoordinator creates a new operation coordinator
func NewCoordinator(ctx context.Context) *Coordinator {
	coordCtx, cancel := context.WithCancel(ctx)

	return &Coordinator{
		errors: NewErrorCollector(),
		ctx:    coordCtx,
		cancel: cancel,
	}
}

// Go executes a function in a goroutine with proper error handling
func (c *Coordinator) Go(operation func(ctx context.Context) error) {
	c.wg.Add(1)

	go func() {
		defer c.wg.Done()

		if err := operation(c.ctx); err != nil {
			c.errors.Add(err)
			logrus.WithError(err).Error("Coordinated operation failed")
		}
	}()
}

// Wait waits for all operations to complete and returns any errors
func (c *Coordinator) Wait() error {
	c.wg.Wait()

	if c.errors.HasErrors() {
		errors := c.errors.GetErrors()
		return fmt.Errorf("coordination failed with %d errors: %v", len(errors), errors)
	}

	return nil
}

// Cancel cancels all operations
func (c *Coordinator) Cancel() {
	c.cancel()
}

// WaitWithTimeout waits for operations with timeout
func (c *Coordinator) WaitWithTimeout(timeout time.Duration) error {
	done := make(chan error, 1)

	go func() {
		done <- c.Wait()
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		c.Cancel()
		return fmt.Errorf("coordination timeout after %v", timeout)
	}
}
