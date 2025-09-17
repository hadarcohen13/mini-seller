package workerpool

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hadarco13/mini-seller/internal/config"
	"github.com/hadarco13/mini-seller/internal/monitoring"
	"github.com/sirupsen/logrus"
)

// Job represents a unit of work
type Job struct {
	ID      string
	Task    func(ctx context.Context) (interface{}, error)
	Result  chan JobResult
	Context context.Context
}

// JobResult holds the result of a job
type JobResult struct {
	ID       string
	Result   interface{}
	Error    error
	Duration time.Duration
}

// WorkerPool manages a pool of workers with goroutine monitoring
type WorkerPool struct {
	jobs      chan Job
	workers   int
	wg        sync.WaitGroup
	quit      chan bool
	isRunning bool
	mu        sync.RWMutex
	monitor   *monitoring.GoroutineMonitor
}

// NewWorkerPool creates a new worker pool with goroutine monitoring
func NewWorkerPool(workers int) *WorkerPool {
	if workers <= 0 {
		workers = 10 // Default
	}

	// Create goroutine monitor with reasonable limits
	monitor := monitoring.NewGoroutineMonitor(workers*10, 30*time.Second)

	return &WorkerPool{
		jobs:    make(chan Job, workers*2), // Buffer jobs
		workers: workers,
		quit:    make(chan bool),
		monitor: monitor,
	}
}

// Start starts the worker pool with monitoring
func (wp *WorkerPool) Start() {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	if wp.isRunning {
		return
	}

	wp.isRunning = true

	// Start goroutine monitor
	wp.monitor.Start()

	for i := 0; i < wp.workers; i++ {
		wp.wg.Add(1)
		wp.monitor.TrackGoroutineStart("worker")
		go wp.worker(i)
	}

	logrus.WithField("workers", wp.workers).Info("Worker pool started with monitoring")
}

// Stop stops the worker pool and cleanup goroutines
func (wp *WorkerPool) Stop() {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	if !wp.isRunning {
		return
	}

	// Signal workers to stop
	close(wp.quit)

	// Wait for workers to finish with timeout
	done := make(chan struct{})
	go func() {
		wp.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logrus.Info("All workers stopped gracefully")
	case <-time.After(time.Duration(config.GetConfig().Server.WorkerStopTimeoutMs) * time.Millisecond):
		logrus.Warn("Worker pool stop timeout - some workers may not have stopped")
	}

	wp.isRunning = false

	// Stop goroutine monitor
	wp.monitor.Stop()

	logrus.Info("Worker pool stopped with cleanup")
}

// Submit submits a job to the worker pool
func (wp *WorkerPool) Submit(job Job) bool {
	wp.mu.RLock()
	defer wp.mu.RUnlock()

	if !wp.isRunning {
		return false
	}

	select {
	case wp.jobs <- job:
		return true
	default:
		// Pool is full
		return false
	}
}

// SubmitTask submits a simple task and returns result channel
func (wp *WorkerPool) SubmitTask(id string, task func(ctx context.Context) (interface{}, error)) <-chan JobResult {
	resultCh := make(chan JobResult, 1)

	job := Job{
		ID:      id,
		Task:    task,
		Result:  resultCh,
		Context: context.Background(),
	}

	if !wp.Submit(job) {
		// Failed to submit
		go func() {
			resultCh <- JobResult{
				ID:    id,
				Error: fmt.Errorf("worker pool full or not running"),
			}
			close(resultCh)
		}()
	}

	return resultCh
}

// SubmitTaskWithContext submits task with custom context
func (wp *WorkerPool) SubmitTaskWithContext(ctx context.Context, id string, task func(ctx context.Context) (interface{}, error)) <-chan JobResult {
	resultCh := make(chan JobResult, 1)

	job := Job{
		ID:      id,
		Task:    task,
		Result:  resultCh,
		Context: ctx,
	}

	if !wp.Submit(job) {
		go func() {
			resultCh <- JobResult{
				ID:    id,
				Error: fmt.Errorf("worker pool full or not running"),
			}
			close(resultCh)
		}()
	}

	return resultCh
}

// worker processes jobs from the job queue with proper error handling and monitoring
func (wp *WorkerPool) worker(id int) {
	defer func() {
		wp.monitor.TrackGoroutineEnd("worker")
		if r := recover(); r != nil {
			logrus.WithFields(logrus.Fields{
				"worker_id": id,
				"panic":     r,
			}).Error("Worker panicked, recovering")
		}
		wp.wg.Done()
	}()

	logrus.WithField("worker_id", id).Debug("Worker started")

	for {
		select {
		case job := <-wp.jobs:
			wp.processJobSafely(id, job)

		case <-wp.quit:
			logrus.WithField("worker_id", id).Debug("Worker stopping")
			return
		}
	}
}

// processJobSafely processes a job with proper error handling and recovery
func (wp *WorkerPool) processJobSafely(workerID int, job Job) {
	defer func() {
		if r := recover(); r != nil {
			jobResult := JobResult{
				ID:    job.ID,
				Error: fmt.Errorf("job panicked: %v", r),
			}

			logrus.WithFields(logrus.Fields{
				"worker_id": workerID,
				"job_id":    job.ID,
				"panic":     r,
			}).Error("Job panicked")

			// Send error result
			select {
			case job.Result <- jobResult:
			default:
				// Channel might be closed, ignore
			}
			close(job.Result)
		}
	}()

	start := time.Now()

	logrus.WithFields(logrus.Fields{
		"worker_id": workerID,
		"job_id":    job.ID,
	}).Debug("Processing job")

	// Execute job with timeout protection
	done := make(chan struct {
		result interface{}
		err    error
	}, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- struct {
					result interface{}
					err    error
				}{nil, fmt.Errorf("task panicked: %v", r)}
			}
		}()

		result, err := job.Task(job.Context)
		done <- struct {
			result interface{}
			err    error
		}{result, err}
	}()

	var result interface{}
	var err error

	select {
	case taskResult := <-done:
		result = taskResult.result
		err = taskResult.err
	case <-job.Context.Done():
		err = job.Context.Err()
	}

	duration := time.Since(start)

	jobResult := JobResult{
		ID:       job.ID,
		Result:   result,
		Error:    err,
		Duration: duration,
	}

	// Send result back safely
	select {
	case job.Result <- jobResult:
		close(job.Result)
	default:
		// Channel might be closed or blocked
		logrus.WithFields(logrus.Fields{
			"worker_id": workerID,
			"job_id":    job.ID,
		}).Warn("Failed to send job result")
	}

	if err != nil {
		logrus.WithFields(logrus.Fields{
			"worker_id": workerID,
			"job_id":    job.ID,
			"error":     err.Error(),
			"duration":  duration,
		}).Error("Job failed")
	} else {
		logrus.WithFields(logrus.Fields{
			"worker_id": workerID,
			"job_id":    job.ID,
			"duration":  duration,
		}).Debug("Job completed")
	}
}

// GetStatus returns worker pool status with goroutine monitoring
func (wp *WorkerPool) GetStatus() map[string]interface{} {
	wp.mu.RLock()
	defer wp.mu.RUnlock()

	status := map[string]interface{}{
		"workers":    wp.workers,
		"is_running": wp.isRunning,
		"queue_size": len(wp.jobs),
		"queue_cap":  cap(wp.jobs),
	}

	// Add goroutine monitoring stats
	if wp.monitor != nil {
		goroutineStats := wp.monitor.GetGoroutineStats()
		status["goroutine_stats"] = goroutineStats

		// Check for leaks
		leaks := wp.monitor.CheckForLeaks()
		if len(leaks) > 0 {
			status["potential_leaks"] = leaks
		}
	}

	return status
}

// ProcessBatch processes multiple jobs and waits for all to complete
func (wp *WorkerPool) ProcessBatch(ctx context.Context, tasks map[string]func(ctx context.Context) (interface{}, error)) map[string]JobResult {
	results := make(map[string]JobResult)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for id, task := range tasks {
		wg.Add(1)
		wp.monitor.TrackGoroutineStart("batch_processor")
		go func(taskID string, taskFunc func(ctx context.Context) (interface{}, error)) {
			defer func() {
				wp.monitor.TrackGoroutineEnd("batch_processor")
				wg.Done()
			}()

			resultCh := wp.SubmitTaskWithContext(ctx, taskID, taskFunc)

			select {
			case result := <-resultCh:
				mu.Lock()
				results[taskID] = result
				mu.Unlock()
			case <-ctx.Done():
				mu.Lock()
				results[taskID] = JobResult{
					ID:    taskID,
					Error: ctx.Err(),
				}
				mu.Unlock()
			}
		}(id, task)
	}

	wg.Wait()
	return results
}
