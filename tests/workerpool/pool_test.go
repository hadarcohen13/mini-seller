package workerpool_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/hadarco13/mini-seller/internal/config"
	"github.com/hadarco13/mini-seller/internal/workerpool"
	"github.com/stretchr/testify/assert"
)

func setupWorkerpoolTestConfig() {
	os.Setenv("MINI_SELLER_ENVIRONMENT", "test")
	os.Setenv("MINI_SELLER_SERVER_PORT", "8080")
}

func TestNewWorkerPool(t *testing.T) {
	setupWorkerpoolTestConfig()
	config.LoadConfig()

	wp := workerpool.NewWorkerPool(5)
	assert.NotNil(t, wp)

	status := wp.GetStatus()
	assert.Equal(t, 5, status["workers"])
	assert.False(t, status["is_running"].(bool))
}

func TestNewWorkerPool_InvalidWorkers(t *testing.T) {
	setupWorkerpoolTestConfig()
	config.LoadConfig()

	wp := workerpool.NewWorkerPool(0)
	assert.NotNil(t, wp)

	status := wp.GetStatus()
	assert.Equal(t, 10, status["workers"]) // Default value
}

func TestWorkerPool_StartStop(t *testing.T) {
	setupWorkerpoolTestConfig()
	config.LoadConfig()

	wp := workerpool.NewWorkerPool(3)

	// Initially not running
	status := wp.GetStatus()
	assert.False(t, status["is_running"].(bool))

	// Start
	wp.Start()
	status = wp.GetStatus()
	assert.True(t, status["is_running"].(bool))

	// Stop
	wp.Stop()
	status = wp.GetStatus()
	assert.False(t, status["is_running"].(bool))
}

func TestWorkerPool_StartTwice(t *testing.T) {
	setupWorkerpoolTestConfig()
	config.LoadConfig()

	wp := workerpool.NewWorkerPool(2)

	wp.Start()
	wp.Start() // Should not panic or create duplicate workers

	status := wp.GetStatus()
	assert.True(t, status["is_running"].(bool))
	assert.Equal(t, 2, status["workers"])

	wp.Stop()
}

func TestWorkerPool_StopTwice(t *testing.T) {
	setupWorkerpoolTestConfig()
	config.LoadConfig()

	wp := workerpool.NewWorkerPool(2)

	wp.Start()
	wp.Stop()
	wp.Stop() // Should not panic

	status := wp.GetStatus()
	assert.False(t, status["is_running"].(bool))
}

func TestWorkerPool_SubmitTask(t *testing.T) {
	setupWorkerpoolTestConfig()
	config.LoadConfig()

	wp := workerpool.NewWorkerPool(2)
	wp.Start()
	defer wp.Stop()

	resultCh := wp.SubmitTask("test-task", func(ctx context.Context) (interface{}, error) {
		return "test-result", nil
	})

	select {
	case result := <-resultCh:
		assert.NoError(t, result.Error)
		assert.Equal(t, "test-task", result.ID)
		assert.Equal(t, "test-result", result.Result)
		assert.Greater(t, result.Duration, time.Duration(0))
	case <-time.After(time.Second):
		t.Fatal("Task did not complete within timeout")
	}
}

func TestWorkerPool_SubmitTaskWithError(t *testing.T) {
	setupWorkerpoolTestConfig()
	config.LoadConfig()

	wp := workerpool.NewWorkerPool(2)
	wp.Start()
	defer wp.Stop()

	expectedError := errors.New("task failed")
	resultCh := wp.SubmitTask("error-task", func(ctx context.Context) (interface{}, error) {
		return nil, expectedError
	})

	select {
	case result := <-resultCh:
		assert.Error(t, result.Error)
		assert.Equal(t, expectedError, result.Error)
		assert.Equal(t, "error-task", result.ID)
		assert.Nil(t, result.Result)
	case <-time.After(time.Second):
		t.Fatal("Task did not complete within timeout")
	}
}

func TestWorkerPool_SubmitTaskPanic(t *testing.T) {
	setupWorkerpoolTestConfig()
	config.LoadConfig()

	wp := workerpool.NewWorkerPool(2)
	wp.Start()
	defer wp.Stop()

	resultCh := wp.SubmitTask("panic-task", func(ctx context.Context) (interface{}, error) {
		panic("test panic")
	})

	select {
	case result := <-resultCh:
		assert.Error(t, result.Error)
		assert.Contains(t, result.Error.Error(), "task panicked")
		assert.Equal(t, "panic-task", result.ID)
		assert.Nil(t, result.Result)
	case <-time.After(time.Second):
		t.Fatal("Task did not complete within timeout")
	}
}

func TestWorkerPool_SubmitTaskWithContext(t *testing.T) {
	setupWorkerpoolTestConfig()
	config.LoadConfig()

	wp := workerpool.NewWorkerPool(2)
	wp.Start()
	defer wp.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	resultCh := wp.SubmitTaskWithContext(ctx, "timeout-task", func(ctx context.Context) (interface{}, error) {
		// Simulate long-running task
		time.Sleep(200 * time.Millisecond)
		return "should-not-reach", nil
	})

	select {
	case result := <-resultCh:
		assert.Error(t, result.Error)
		assert.Equal(t, context.DeadlineExceeded, result.Error)
		assert.Equal(t, "timeout-task", result.ID)
	case <-time.After(time.Second):
		t.Fatal("Task did not complete within timeout")
	}
}

func TestWorkerPool_SubmitTaskWhenNotRunning(t *testing.T) {
	setupWorkerpoolTestConfig()
	config.LoadConfig()

	wp := workerpool.NewWorkerPool(2)
	// Don't start the worker pool

	resultCh := wp.SubmitTask("not-running-task", func(ctx context.Context) (interface{}, error) {
		return "should-not-execute", nil
	})

	select {
	case result := <-resultCh:
		assert.Error(t, result.Error)
		assert.Contains(t, result.Error.Error(), "worker pool full or not running")
		assert.Equal(t, "not-running-task", result.ID)
	case <-time.After(time.Second):
		t.Fatal("Task did not complete within timeout")
	}
}

func TestWorkerPool_ConcurrentTasks(t *testing.T) {
	setupWorkerpoolTestConfig()
	config.LoadConfig()

	wp := workerpool.NewWorkerPool(5)
	wp.Start()
	defer wp.Stop()

	const numTasks = 10
	results := make([]<-chan workerpool.JobResult, numTasks)

	// Submit multiple tasks concurrently
	for i := 0; i < numTasks; i++ {
		taskID := fmt.Sprintf("concurrent-task-%d", i)
		results[i] = wp.SubmitTask(taskID, func(ctx context.Context) (interface{}, error) {
			time.Sleep(10 * time.Millisecond) // Small delay to simulate work
			return fmt.Sprintf("result-%s", taskID), nil
		})
	}

	// Wait for all results
	for i, resultCh := range results {
		select {
		case result := <-resultCh:
			assert.NoError(t, result.Error)
			assert.Contains(t, result.ID, "concurrent-task-")
			assert.Contains(t, result.Result.(string), "result-concurrent-task-")
		case <-time.After(time.Second):
			t.Fatalf("Task %d did not complete within timeout", i)
		}
	}
}

func TestWorkerPool_GetStatus(t *testing.T) {
	setupWorkerpoolTestConfig()
	config.LoadConfig()

	wp := workerpool.NewWorkerPool(3)

	status := wp.GetStatus()
	assert.Equal(t, 3, status["workers"])
	assert.False(t, status["is_running"].(bool))
	assert.Equal(t, 0, status["queue_size"])
	assert.Greater(t, status["queue_cap"], 0)

	wp.Start()
	status = wp.GetStatus()
	assert.True(t, status["is_running"].(bool))

	wp.Stop()
}

func TestWorkerPool_ProcessBatch(t *testing.T) {
	setupWorkerpoolTestConfig()
	config.LoadConfig()

	wp := workerpool.NewWorkerPool(3)
	wp.Start()
	defer wp.Stop()

	tasks := map[string]func(ctx context.Context) (interface{}, error){
		"task1": func(ctx context.Context) (interface{}, error) {
			return "result1", nil
		},
		"task2": func(ctx context.Context) (interface{}, error) {
			return "result2", nil
		},
		"task3": func(ctx context.Context) (interface{}, error) {
			return nil, errors.New("task3 failed")
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	results := wp.ProcessBatch(ctx, tasks)

	assert.Len(t, results, 3)

	assert.NoError(t, results["task1"].Error)
	assert.Equal(t, "result1", results["task1"].Result)

	assert.NoError(t, results["task2"].Error)
	assert.Equal(t, "result2", results["task2"].Result)

	assert.Error(t, results["task3"].Error)
	assert.Equal(t, "task3 failed", results["task3"].Error.Error())
}

func TestWorkerPool_ProcessBatchWithTimeout(t *testing.T) {
	setupWorkerpoolTestConfig()
	config.LoadConfig()

	wp := workerpool.NewWorkerPool(2)
	wp.Start()
	defer wp.Stop()

	tasks := map[string]func(ctx context.Context) (interface{}, error){
		"fast_task": func(ctx context.Context) (interface{}, error) {
			return "fast_result", nil
		},
		"slow_task": func(ctx context.Context) (interface{}, error) {
			time.Sleep(200 * time.Millisecond)
			return "slow_result", nil
		},
	}

	// Short timeout that should cancel the slow task
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	results := wp.ProcessBatch(ctx, tasks)

	assert.Len(t, results, 2)

	// Fast task should complete
	assert.NoError(t, results["fast_task"].Error)
	assert.Equal(t, "fast_result", results["fast_task"].Result)

	// Slow task should be canceled
	assert.Error(t, results["slow_task"].Error)
	assert.Equal(t, context.DeadlineExceeded, results["slow_task"].Error)
}

func TestWorkerPool_QueueFull(t *testing.T) {
	setupWorkerpoolTestConfig()
	config.LoadConfig()

	// Create small pool to test queue limits
	wp := workerpool.NewWorkerPool(1)
	wp.Start()
	defer wp.Stop()

	// Block the single worker
	blockingTask := wp.SubmitTask("blocking-task", func(ctx context.Context) (interface{}, error) {
		time.Sleep(200 * time.Millisecond)
		return "blocked", nil
	})

	// Try to overwhelm the queue
	var failedTasks int
	const numOverwhelm = 10

	for i := 0; i < numOverwhelm; i++ {
		resultCh := wp.SubmitTask(fmt.Sprintf("overflow-task-%d", i), func(ctx context.Context) (interface{}, error) {
			return "overflow", nil
		})

		select {
		case result := <-resultCh:
			if result.Error != nil && result.Error.Error() == "worker pool full or not running" {
				failedTasks++
			}
		case <-time.After(10 * time.Millisecond):
			// Some tasks might not complete quickly due to queue size
		}
	}

	// Wait for blocking task to complete
	<-blockingTask

	// Should have some failed submissions due to queue limits
	// (exact number depends on timing and queue capacity)
	assert.Greater(t, failedTasks, 0)
}

// Integration test simulating real workload
func TestWorkerPool_RealWorldScenario(t *testing.T) {
	setupWorkerpoolTestConfig()
	config.LoadConfig()

	wp := workerpool.NewWorkerPool(5)
	wp.Start()
	defer wp.Stop()

	var wg sync.WaitGroup
	const numClients = 10
	const tasksPerClient = 5

	// Simulate multiple clients submitting tasks
	for client := 0; client < numClients; client++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()

			for task := 0; task < tasksPerClient; task++ {
				taskID := fmt.Sprintf("client-%d-task-%d", clientID, task)

				// Add some backpressure by spacing out submissions slightly
				time.Sleep(time.Duration(task) * time.Millisecond)

				resultCh := wp.SubmitTask(taskID, func(ctx context.Context) (interface{}, error) {
					// Simulate varying work loads
					workTime := time.Duration(clientID*5+task) * time.Millisecond
					time.Sleep(workTime)
					return fmt.Sprintf("processed-%s", taskID), nil
				})

				// Don't block waiting for result in this test
				go func(ch <-chan workerpool.JobResult, id string) {
					select {
					case result := <-ch:
						// Allow "worker pool full" errors as they're expected under load
						if result.Error != nil && result.Error.Error() == "worker pool full or not running" {
							t.Logf("Task %s was rejected due to full queue (expected under high load)", id)
						} else {
							assert.NoError(t, result.Error, "Task %s should not have unexpected errors", id)
							if result.Result != nil {
								assert.Contains(t, result.Result.(string), "processed-", "Task %s should return processed result", id)
							}
						}
					case <-time.After(time.Second):
						t.Errorf("Task %s did not complete within timeout", id)
					}
				}(resultCh, taskID)
			}
		}(client)
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond) // Allow processing to complete

	status := wp.GetStatus()
	assert.True(t, status["is_running"].(bool))
	assert.Equal(t, 5, status["workers"])
}

// Benchmark tests
func BenchmarkWorkerPool_SubmitTask(b *testing.B) {
	setupWorkerpoolTestConfig()
	config.LoadConfig()

	wp := workerpool.NewWorkerPool(10)
	wp.Start()
	defer wp.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wp.SubmitTask(fmt.Sprintf("bench-task-%d", i), func(ctx context.Context) (interface{}, error) {
			return i, nil
		})
	}
}

func BenchmarkWorkerPool_ProcessBatch(b *testing.B) {
	setupWorkerpoolTestConfig()
	config.LoadConfig()

	wp := workerpool.NewWorkerPool(10)
	wp.Start()
	defer wp.Stop()

	tasks := make(map[string]func(ctx context.Context) (interface{}, error))
	for i := 0; i < 100; i++ {
		taskID := fmt.Sprintf("batch-task-%d", i)
		tasks[taskID] = func(ctx context.Context) (interface{}, error) {
			return taskID, nil
		}
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wp.ProcessBatch(ctx, tasks)
	}
}
