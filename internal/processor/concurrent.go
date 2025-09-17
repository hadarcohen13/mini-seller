package processor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bsm/openrtb"
	"github.com/hadarco13/mini-seller/internal/buyers"
	"github.com/sirupsen/logrus"
)

// BidProcessor handles concurrent bid request processing
type BidProcessor struct {
	buyers      []*buyers.BuyerClient
	maxWorkers  int
	requestChan chan *BidRequest
	wg          sync.WaitGroup
}

// BidRequest wraps a bid request with response channel
type BidRequest struct {
	Request    *openrtb.BidRequest
	RequestID  string
	ResponseCh chan *BidResult
}

// BidResult contains the result from a buyer
type BidResult struct {
	BuyerEndpoint string
	Response      *openrtb.BidResponse
	Error         error
	Duration      time.Duration
}

// NewBidProcessor creates a new concurrent bid processor
func NewBidProcessor(buyers []*buyers.BuyerClient, maxWorkers int) *BidProcessor {
	if maxWorkers <= 0 {
		maxWorkers = 10 // Default
	}

	return &BidProcessor{
		buyers:      buyers,
		maxWorkers:  maxWorkers,
		requestChan: make(chan *BidRequest, maxWorkers*2),
	}
}

// Start starts the worker goroutines
func (bp *BidProcessor) Start() {
	for i := 0; i < bp.maxWorkers; i++ {
		bp.wg.Add(1)
		go bp.worker(i)
	}

	logrus.WithField("workers", bp.maxWorkers).Info("Bid processor started")
}

// Stop stops the processor and waits for workers to finish
func (bp *BidProcessor) Stop() {
	close(bp.requestChan)
	bp.wg.Wait()
	logrus.Info("Bid processor stopped")
}

// ProcessBidRequest processes a bid request concurrently across all buyers
func (bp *BidProcessor) ProcessBidRequest(ctx context.Context, bidRequest *openrtb.BidRequest, requestID string) ([]*BidResult, error) {
	if len(bp.buyers) == 0 {
		return nil, nil
	}

	// Create response channels for each buyer
	results := make([]*BidResult, len(bp.buyers))
	var wg sync.WaitGroup

	// Send requests to all buyers concurrently
	for i, buyer := range bp.buyers {
		wg.Add(1)
		go func(index int, buyerClient *buyers.BuyerClient) {
			defer wg.Done()

			start := time.Now()
			logEntry := logrus.WithFields(logrus.Fields{
				"request_id": requestID,
				"buyer_id":   index,
			})

			response, err := buyerClient.SendBidRequest(ctx, logEntry, bidRequest)
			duration := time.Since(start)

			results[index] = &BidResult{
				BuyerEndpoint: buyerClient.GetEndpoint(),
				Response:      response,
				Error:         err,
				Duration:      duration,
			}
		}(i, buyer)
	}

	// Wait for all buyers to respond or timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All buyers responded
	case <-ctx.Done():
		// Context timeout
		logrus.WithField("request_id", requestID).Warn("Bid request processing timeout")
	}

	return results, nil
}

// worker processes bid requests from the channel
func (bp *BidProcessor) worker(workerID int) {
	defer bp.wg.Done()

	logrus.WithField("worker_id", workerID).Debug("Worker started")

	for bidReq := range bp.requestChan {
		ctx := context.Background()
		results, err := bp.ProcessBidRequest(ctx, bidReq.Request, bidReq.RequestID)

		// Send results back
		var sentResult bool
		if len(results) > 0 {
			// Send first successful result
			for _, result := range results {
				if result.Error == nil && result.Response != nil {
					bidReq.ResponseCh <- result
					sentResult = true
					break
				}
			}
			// If no successful result found, send the first result (even if it's an error)
			if !sentResult {
				bidReq.ResponseCh <- results[0]
				sentResult = true
			}
		}

		if err != nil && !sentResult {
			bidReq.ResponseCh <- &BidResult{Error: err}
		}

		close(bidReq.ResponseCh)
	}

	logrus.WithField("worker_id", workerID).Debug("Worker stopped")
}

// SubmitBidRequest submits a bid request for processing
func (bp *BidProcessor) SubmitBidRequest(bidRequest *openrtb.BidRequest, requestID string) <-chan *BidResult {
	responseCh := make(chan *BidResult, 1)

	req := &BidRequest{
		Request:    bidRequest,
		RequestID:  requestID,
		ResponseCh: responseCh,
	}

	select {
	case bp.requestChan <- req:
		// Request submitted
	default:
		// Channel full, reject request
		go func() {
			responseCh <- &BidResult{Error: fmt.Errorf("processor queue full")}
			close(responseCh)
		}()
	}

	return responseCh
}

// ProcessBidRequestsConcurrently processes multiple bid requests at once
func ProcessBidRequestsConcurrently(ctx context.Context, bidRequests []*openrtb.BidRequest, buyers []*buyers.BuyerClient) map[string][]*BidResult {
	results := make(map[string][]*BidResult)
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Process each bid request concurrently
	for _, bidRequest := range bidRequests {
		wg.Add(1)
		go func(req *openrtb.BidRequest) {
			defer wg.Done()

			processor := NewBidProcessor(buyers, len(buyers))
			bidResults, err := processor.ProcessBidRequest(ctx, req, req.ID)

			mu.Lock()
			if err != nil {
				results[req.ID] = []*BidResult{{Error: err}}
			} else {
				results[req.ID] = bidResults
			}
			mu.Unlock()
		}(bidRequest)
	}

	wg.Wait()
	return results
}
