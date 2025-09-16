package buyers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/bsm/openrtb"
	"github.com/hadarco13/mini-seller/internal/metrics"
	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
)

type BuyerClient struct {
	httpClient *http.Client
	endpoint   string
	limiter    *rate.Limiter
	metrics    *metrics.BuyerClientMetrics
}

func NewBuyerClient(endpoint string, qps float64, burst int, timeout time.Duration) *BuyerClient {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
	}

	httpClient := &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}

	limiter := rate.NewLimiter(rate.Limit(qps), burst)

	return &BuyerClient{
		httpClient: httpClient,
		endpoint:   endpoint,
		limiter:    limiter,
		metrics:    metrics.NewBuyerClientMetrics(),
	}
}

// SendBidRequest sends an OpenRTB bid request to the external buyer.
func (c *BuyerClient) SendBidRequest(ctx context.Context, logEntry *logrus.Entry, bidRequest *openrtb.BidRequest) (*openrtb.BidResponse, error) {
	requestStart := time.Now()

	// Increment total requests counter
	c.metrics.RequestsTotal.Inc()

	// Log outgoing request details
	logEntry.WithFields(logrus.Fields{
		"direction":         "outgoing",
		"buyer_endpoint":    c.endpoint,
		"bid_request_id":    bidRequest.ID,
		"impression_count":  len(bidRequest.Imp),
		"auction_type":      bidRequest.AuctionType,
		"timeout_ms":        bidRequest.TMax,
		"request_timestamp": requestStart.Format(time.RFC3339Nano),
	}).Info("Preparing outgoing bid request")

	// Log detailed bid request structure
	c.logBidRequestDetails(logEntry, bidRequest)

	if err := c.limiter.Wait(ctx); err != nil {
		c.metrics.RateLimitExceeded.Inc()
		c.metrics.RequestsFailed.Inc()
		logEntry.WithFields(logrus.Fields{
			"direction":      "outgoing",
			"buyer_endpoint": c.endpoint,
			"error":          err.Error(),
			"duration_ms":    time.Since(requestStart).Milliseconds(),
		}).Warn("Rate limiting: Failed to get token for outgoing request")
		return nil, fmt.Errorf("rate limit exceeded for buyer %s", c.endpoint)
	}

	payload, err := json.Marshal(bidRequest)
	if err != nil {
		c.metrics.RequestsFailed.Inc()
		logEntry.WithFields(logrus.Fields{
			"direction":      "outgoing",
			"buyer_endpoint": c.endpoint,
			"error":          err.Error(),
			"duration_ms":    time.Since(requestStart).Milliseconds(),
		}).Error("Failed to marshal bid request")
		return nil, fmt.Errorf("failed to marshal bid request: %w", err)
	}

	// Log request payload size
	logEntry.WithFields(logrus.Fields{
		"direction":      "outgoing",
		"buyer_endpoint": c.endpoint,
		"payload_size":   len(payload),
	}).Debug("Bid request payload prepared")

	const maxRetries = 2
	for i := 0; i < maxRetries; i++ {
		req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint, bytes.NewBuffer(payload))
		if err != nil {
			logEntry.WithFields(logrus.Fields{
				"direction":      "outgoing",
				"buyer_endpoint": c.endpoint,
				"attempt":        i + 1,
				"error":          err.Error(),
			}).Error("Failed to create HTTP request")
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "mini-seller/1.0")
		req.Header.Set("X-OpenRTB-Version", "2.5")

		logEntry.WithFields(logrus.Fields{
			"direction":      "outgoing",
			"buyer_endpoint": c.endpoint,
			"attempt":        i + 1,
			"max_retries":    maxRetries,
			"method":         "POST",
			"headers": map[string]string{
				"Content-Type":      "application/json",
				"User-Agent":        "mini-seller/1.0",
				"X-OpenRTB-Version": "2.5",
			},
		}).Info("Sending bid request to buyer")

		start := time.Now()
		resp, err := c.httpClient.Do(req)
		latency := time.Since(start)
		totalDuration := time.Since(requestStart)

		if err != nil {
			logEntry.WithFields(logrus.Fields{
				"direction":        "outgoing",
				"buyer_endpoint":   c.endpoint,
				"attempt":          i + 1,
				"max_retries":      maxRetries,
				"error":            err.Error(),
				"attempt_duration": latency.Milliseconds(),
				"total_duration":   totalDuration.Milliseconds(),
			}).Warn("Attempt to send bid request failed")
			continue
		}
		defer resp.Body.Close()

		// Read response body for logging
		responseBody := &bytes.Buffer{}
		responseReader := http.MaxBytesReader(nil, resp.Body, 1024*1024) // 1MB limit
		_, readErr := responseBody.ReadFrom(responseReader)

		logEntry.WithFields(logrus.Fields{
			"direction":        "outgoing",
			"buyer_endpoint":   c.endpoint,
			"attempt":          i + 1,
			"status_code":      resp.StatusCode,
			"status":           resp.Status,
			"attempt_duration": latency.Milliseconds(),
			"total_duration":   totalDuration.Milliseconds(),
			"response_size":    responseBody.Len(),
			"content_type":     resp.Header.Get("Content-Type"),
		}).Info("Received response from buyer")

		// Log response headers
		logEntry.WithFields(logrus.Fields{
			"direction":        "outgoing",
			"buyer_endpoint":   c.endpoint,
			"response_headers": resp.Header,
		}).Debug("Response headers received")

		if readErr != nil {
			logEntry.WithFields(logrus.Fields{
				"direction":      "outgoing",
				"buyer_endpoint": c.endpoint,
				"error":          readErr.Error(),
				"status_code":    resp.StatusCode,
			}).Error("Failed to read response body")
			return nil, fmt.Errorf("failed to read response body: %w", readErr)
		}

		// Handle different response scenarios
		if resp.StatusCode == http.StatusNoContent {
			logEntry.WithFields(logrus.Fields{
				"direction":      "outgoing",
				"buyer_endpoint": c.endpoint,
				"status_code":    resp.StatusCode,
				"result":         "no_bid",
				"total_duration": totalDuration.Milliseconds(),
			}).Info("Buyer returned no bid (204)")

			return &openrtb.BidResponse{
				ID:  bidRequest.ID,
				NBR: 1, // No bid reason: no inventory
			}, nil
		}

		if resp.StatusCode != http.StatusOK {
			logEntry.WithFields(logrus.Fields{
				"direction":      "outgoing",
				"buyer_endpoint": c.endpoint,
				"status_code":    resp.StatusCode,
				"response_body":  responseBody.String(),
				"total_duration": totalDuration.Milliseconds(),
			}).Error("Buyer returned non-success status code")
			return nil, fmt.Errorf("received non-200 status code: %d", resp.StatusCode)
		}

		// Parse and log successful bid response
		var bidResponse openrtb.BidResponse
		if err := json.Unmarshal(responseBody.Bytes(), &bidResponse); err != nil {
			logEntry.WithFields(logrus.Fields{
				"direction":      "outgoing",
				"buyer_endpoint": c.endpoint,
				"error":          err.Error(),
				"response_body":  responseBody.String(),
				"total_duration": totalDuration.Milliseconds(),
			}).Error("Failed to decode bid response")
			return nil, fmt.Errorf("failed to decode bid response: %w", err)
		}

		// Log detailed bid response
		c.logBidResponseDetails(logEntry, &bidResponse, totalDuration)

		// Increment success counter
		c.metrics.RequestsSuccessful.Inc()

		return &bidResponse, nil
	}

	logEntry.WithFields(logrus.Fields{
		"direction":      "outgoing",
		"buyer_endpoint": c.endpoint,
		"max_retries":    maxRetries,
		"total_duration": time.Since(requestStart).Milliseconds(),
		"result":         "all_attempts_failed",
	}).Error("All attempts to send bid request failed")

	// Increment failure counter
	c.metrics.RequestsFailed.Inc()

	return nil, fmt.Errorf("all %d attempts to send bid request failed", maxRetries)
}

// logBidRequestDetails logs detailed information about the outgoing bid request
func (c *BuyerClient) logBidRequestDetails(logEntry *logrus.Entry, bidRequest *openrtb.BidRequest) {
	// Log impression details
	impressionDetails := make([]map[string]interface{}, len(bidRequest.Imp))
	for i, imp := range bidRequest.Imp {
		impDetail := map[string]interface{}{
			"id":       imp.ID,
			"bidfloor": imp.BidFloor,
			"secure":   imp.Secure,
		}

		if imp.Banner != nil {
			impDetail["banner"] = map[string]interface{}{
				"w": imp.Banner.W,
				"h": imp.Banner.H,
			}
		}
		if imp.Video != nil {
			impDetail["video"] = map[string]interface{}{
				"w": imp.Video.W,
				"h": imp.Video.H,
			}
		}

		impressionDetails[i] = impDetail
	}

	logEntry.WithFields(logrus.Fields{
		"direction":      "outgoing",
		"buyer_endpoint": c.endpoint,
		"bid_request_id": bidRequest.ID,
		"impressions":    impressionDetails,
		"device_present": bidRequest.Device != nil,
		"user_present":   bidRequest.User != nil,
		"site_present":   bidRequest.Site != nil,
		"app_present":    bidRequest.App != nil,
	}).Info("Outgoing bid request details")
}

// logBidResponseDetails logs detailed information about the received bid response
func (c *BuyerClient) logBidResponseDetails(logEntry *logrus.Entry, bidResponse *openrtb.BidResponse, duration time.Duration) {
	totalBids := 0
	totalPrice := 0.0
	seatBidDetails := make([]map[string]interface{}, len(bidResponse.SeatBid))

	for i, seatBid := range bidResponse.SeatBid {
		bidDetails := make([]map[string]interface{}, len(seatBid.Bid))
		seatTotalPrice := 0.0

		for j, bid := range seatBid.Bid {
			bidDetails[j] = map[string]interface{}{
				"id":    bid.ID,
				"impid": bid.ImpID,
				"price": bid.Price,
				"adid":  bid.AdID,
				"crid":  bid.CreativeID,
				"w":     bid.W,
				"h":     bid.H,
			}
			seatTotalPrice += bid.Price
		}

		seatBidDetails[i] = map[string]interface{}{
			"seat":        seatBid.Seat,
			"bid_count":   len(seatBid.Bid),
			"total_price": seatTotalPrice,
			"bids":        bidDetails,
		}

		totalBids += len(seatBid.Bid)
		totalPrice += seatTotalPrice
	}

	logEntry.WithFields(logrus.Fields{
		"direction":       "incoming",
		"buyer_endpoint":  c.endpoint,
		"bid_response_id": bidResponse.ID,
		"currency":        bidResponse.Currency,
		"seat_count":      len(bidResponse.SeatBid),
		"total_bids":      totalBids,
		"total_price":     totalPrice,
		"duration_ms":     duration.Milliseconds(),
		"nbr":             bidResponse.NBR,
		"seat_bids":       seatBidDetails,
	}).Info("Received detailed bid response from buyer")
}
