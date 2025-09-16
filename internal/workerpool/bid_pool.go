package workerpool

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bsm/openrtb"
	"github.com/hadarco13/mini-seller/internal/handlers"
	"github.com/sirupsen/logrus"
)

// BidWorkerPool specialized worker pool for bid requests
type BidWorkerPool struct {
	pool *WorkerPool
}

// BidTask represents a bid request task
type BidTask struct {
	BidRequest *openrtb.BidRequest
	RequestID  string
	Version    string
}

// BidTaskResult represents the result of processing a bid request
type BidTaskResult struct {
	RequestID   string
	BidResponse *openrtb.BidResponse
	Error       error
	Duration    time.Duration
}

// NewBidWorkerPool creates a new bid request worker pool
func NewBidWorkerPool(workers int) *BidWorkerPool {
	return &BidWorkerPool{
		pool: NewWorkerPool(workers),
	}
}

// Start starts the bid worker pool
func (bwp *BidWorkerPool) Start() {
	bwp.pool.Start()
	logrus.Info("Bid worker pool started")
}

// Stop stops the bid worker pool
func (bwp *BidWorkerPool) Stop() {
	bwp.pool.Stop()
	logrus.Info("Bid worker pool stopped")
}

// ProcessBidRequest submits a bid request for processing
func (bwp *BidWorkerPool) ProcessBidRequest(ctx context.Context, bidRequest *openrtb.BidRequest, requestID, version string) <-chan BidTaskResult {
	resultCh := make(chan BidTaskResult, 1)

	task := func(taskCtx context.Context) (interface{}, error) {
		start := time.Now()

		// Generate bid response
		bidResponse := handlers.GenerateBidResponse(bidRequest, version, requestID)

		return &BidTaskResult{
			RequestID:   requestID,
			BidResponse: bidResponse,
			Duration:    time.Since(start),
		}, nil
	}

	jobResultCh := bwp.pool.SubmitTaskWithContext(ctx, requestID, task)

	go func() {
		defer close(resultCh)
		jobResult := <-jobResultCh

		if jobResult.Error != nil {
			resultCh <- BidTaskResult{
				RequestID: requestID,
				Error:     jobResult.Error,
				Duration:  jobResult.Duration,
			}
			return
		}

		if bidTaskResult, ok := jobResult.Result.(*BidTaskResult); ok {
			resultCh <- *bidTaskResult
		}
	}()

	return resultCh
}

// ProcessMultipleBidRequests processes multiple bid requests concurrently with proper error handling
func (bwp *BidWorkerPool) ProcessMultipleBidRequests(ctx context.Context, bidRequests []*BidTask) map[string]BidTaskResult {
	results := make(map[string]BidTaskResult)
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Create error collector for tracking failures
	errorCount := 0
	totalCount := len(bidRequests)

	for _, bidTask := range bidRequests {
		wg.Add(1)
		go func(task *BidTask) {
			defer func() {
				if r := recover(); r != nil {
					logrus.WithFields(logrus.Fields{
						"request_id": task.RequestID,
						"panic":      r,
					}).Error("Bid processing goroutine panicked")

					mu.Lock()
					results[task.RequestID] = BidTaskResult{
						RequestID: task.RequestID,
						Error:     fmt.Errorf("processing panicked: %v", r),
					}
					errorCount++
					mu.Unlock()
				}
				wg.Done()
			}()

			resultCh := bwp.ProcessBidRequest(ctx, task.BidRequest, task.RequestID, task.Version)

			select {
			case result := <-resultCh:
				mu.Lock()
				results[task.RequestID] = result
				if result.Error != nil {
					errorCount++
				}
				mu.Unlock()
			case <-ctx.Done():
				mu.Lock()
				results[task.RequestID] = BidTaskResult{
					RequestID: task.RequestID,
					Error:     ctx.Err(),
				}
				errorCount++
				mu.Unlock()
			}
		}(bidTask)
	}

	wg.Wait()

	logrus.WithFields(logrus.Fields{
		"total_requests": totalCount,
		"errors":         errorCount,
		"success_rate":   float64(totalCount-errorCount) / float64(totalCount) * 100,
	}).Info("Batch bid processing completed")

	return results
}

// GetStatus returns bid worker pool status
func (bwp *BidWorkerPool) GetStatus() map[string]interface{} {
	status := bwp.pool.GetStatus()
	status["pool_type"] = "bid_requests"
	return status
}

// ProcessBatch processes a batch of bid requests with timeout
func (bwp *BidWorkerPool) ProcessBatch(ctx context.Context, bidRequests []*BidTask, timeout time.Duration) map[string]BidTaskResult {
	batchCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return bwp.ProcessMultipleBidRequests(batchCtx, bidRequests)
}
