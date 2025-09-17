package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bsm/openrtb"
	"github.com/gorilla/mux"
	"github.com/hadarco13/mini-seller/internal/config"
	"github.com/hadarco13/mini-seller/internal/handlers"
	"github.com/hadarco13/mini-seller/internal/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupIntegrationTestConfig() {
	os.Setenv("MINI_SELLER_ENVIRONMENT", "test")
	os.Setenv("MINI_SELLER_SERVER_PORT", "8080")
	os.Setenv("MINI_SELLER_REDIS_HOST", "localhost")
	os.Setenv("MINI_SELLER_REDIS_PORT", "6379")
}

func resetIntegrationPrometheusRegistry() {
	prometheus.DefaultRegisterer = prometheus.NewRegistry()
}

func setupTestServer() *httptest.Server {
	resetIntegrationPrometheusRegistry()
	setupIntegrationTestConfig()
	config.LoadConfig()

	router := mux.NewRouter()

	// Apply middleware chain (same as server.go)
	router.Use(middleware.ErrorHandler)
	router.Use(middleware.RequestIDMiddleware)
	router.Use(middleware.CORSMiddleware)
	router.Use(middleware.LoggingMiddleware)

	// Configure rate limiting with test values
	router.Use(middleware.RateLimiterMiddleware(100.0, 200))

	// Set up routes
	router.HandleFunc("/health", handlers.HealthHandler).Methods("GET")
	router.HandleFunc("/bid/request", handlers.BidRequestHandler).Methods("POST")
	router.HandleFunc("/bid/test", handlers.BidRequestHandler).Methods("POST")

	return httptest.NewServer(router)
}

func TestFullBidRequestResponseFlow(t *testing.T) {
	server := setupTestServer()
	defer server.Close()

	tests := []struct {
		name           string
		bidRequest     *openrtb.BidRequest
		expectedStatus int
		validateFunc   func(t *testing.T, response *openrtb.BidResponse, responseBody string)
	}{
		{
			name: "Valid OpenRTB 2.5 Banner Request",
			bidRequest: &openrtb.BidRequest{
				ID: "test-bid-request-1",
				Imp: []openrtb.Impression{
					{
						ID: "imp-1",
						Banner: &openrtb.Banner{
							W: 300,
							H: 250,
						},
						BidFloor: 1.0,
					},
				},
				Site: &openrtb.Site{
					Inventory: openrtb.Inventory{
						ID:     "site-123",
						Name:   "Test Site",
						Domain: "test.example.com",
					},
				},
				User: &openrtb.User{
					ID: "user-456",
				},
				Device: &openrtb.Device{
					UA: "Mozilla/5.0 (Test Browser)",
					IP: "192.168.1.1",
				},
				Test:  0,
				TMax:  120,
				WSeat: []string{"seat1"},
			},
			expectedStatus: http.StatusOK,
			validateFunc: func(t *testing.T, response *openrtb.BidResponse, responseBody string) {
				assert.NotNil(t, response)
				assert.Equal(t, "test-bid-request-1", response.ID)
				assert.NotEmpty(t, response.BidID)
				assert.Len(t, response.SeatBid, 1)
				assert.Len(t, response.SeatBid[0].Bid, 1)

				bid := response.SeatBid[0].Bid[0]
				assert.Equal(t, "imp-1", bid.ImpID)
				assert.Greater(t, bid.Price, 0.0)
				assert.NotEmpty(t, bid.AdMarkup)
				assert.Contains(t, bid.AdMarkup, "<div")
				assert.Contains(t, bid.AdMarkup, "300")
				assert.Contains(t, bid.AdMarkup, "250")
			},
		},
		{
			name: "Valid OpenRTB 2.6 Video Request",
			bidRequest: &openrtb.BidRequest{
				ID: "test-bid-request-video",
				Imp: []openrtb.Impression{
					{
						ID: "imp-video-1",
						Video: &openrtb.Video{
							Mimes:       []string{"video/mp4"},
							MinDuration: 15,
							MaxDuration: 30,
							W:           640,
							H:           480,
							Protocols:   []int{2, 3},
						},
						BidFloor: 2.0,
					},
				},
				Site: &openrtb.Site{
					Inventory: openrtb.Inventory{
						ID:     "video-site-123",
						Name:   "Video Test Site",
						Domain: "video.example.com",
					},
				},
				Test:  0,
				TMax:  100,
				WSeat: []string{"video-seat"},
			},
			expectedStatus: http.StatusOK,
			validateFunc: func(t *testing.T, response *openrtb.BidResponse, responseBody string) {
				assert.NotNil(t, response)
				assert.Equal(t, "test-bid-request-video", response.ID)
				assert.NotEmpty(t, response.BidID)
				assert.Len(t, response.SeatBid, 1)
				assert.Len(t, response.SeatBid[0].Bid, 1)

				bid := response.SeatBid[0].Bid[0]
				assert.Equal(t, "imp-video-1", bid.ImpID)
				assert.Greater(t, bid.Price, 0.0)
				// Video ads should have different content
				assert.NotEmpty(t, bid.AdMarkup)
			},
		},
		{
			name: "Multiple Impressions Request",
			bidRequest: &openrtb.BidRequest{
				ID: "test-multi-imp",
				Imp: []openrtb.Impression{
					{
						ID: "imp-banner-1",
						Banner: &openrtb.Banner{
							W: 728,
							H: 90,
						},
						BidFloor: 0.5,
					},
					{
						ID: "imp-banner-2",
						Banner: &openrtb.Banner{
							W: 300,
							H: 600,
						},
						BidFloor: 1.5,
					},
				},
				Site: &openrtb.Site{
					Inventory: openrtb.Inventory{
						ID:     "multi-site",
						Name:   "Multi Impression Site",
						Domain: "multi.example.com",
					},
				},
				Test: 0,
				TMax: 150,
			},
			expectedStatus: http.StatusOK,
			validateFunc: func(t *testing.T, response *openrtb.BidResponse, responseBody string) {
				assert.NotNil(t, response)
				assert.Equal(t, "test-multi-imp", response.ID)
				assert.NotEmpty(t, response.BidID)
				assert.Len(t, response.SeatBid, 1)
				assert.Len(t, response.SeatBid[0].Bid, 2)

				// Check both bids
				bidsByImpID := make(map[string]openrtb.Bid)
				for _, bid := range response.SeatBid[0].Bid {
					bidsByImpID[bid.ImpID] = bid
				}

				// Validate first impression bid
				bid1, exists := bidsByImpID["imp-banner-1"]
				assert.True(t, exists)
				assert.Greater(t, bid1.Price, 0.0)
				assert.Contains(t, bid1.AdMarkup, "728")
				assert.Contains(t, bid1.AdMarkup, "90")

				// Validate second impression bid
				bid2, exists := bidsByImpID["imp-banner-2"]
				assert.True(t, exists)
				assert.Greater(t, bid2.Price, 0.0)
				assert.Contains(t, bid2.AdMarkup, "300")
				assert.Contains(t, bid2.AdMarkup, "600")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Serialize bid request
			requestBody, err := json.Marshal(tt.bidRequest)
			require.NoError(t, err)

			// Create HTTP request
			req, err := http.NewRequest("POST", server.URL+"/bid/request", bytes.NewBuffer(requestBody))
			require.NoError(t, err)

			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("User-Agent", "TestClient/1.0")
			req.Header.Set("X-Forwarded-For", "203.0.113.1")

			// Send request
			client := &http.Client{Timeout: 10 * time.Second}
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Validate response status
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			// Validate headers
			assert.NotEmpty(t, resp.Header.Get("X-Request-ID"))
			assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
			assert.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))

			// Read response body
			var responseBody bytes.Buffer
			_, err = responseBody.ReadFrom(resp.Body)
			require.NoError(t, err)

			if tt.expectedStatus == http.StatusOK {
				// Parse bid response
				var bidResponse openrtb.BidResponse
				err = json.Unmarshal(responseBody.Bytes(), &bidResponse)
				require.NoError(t, err, "Response should be valid JSON: %s", responseBody.String())

				// Run custom validation
				tt.validateFunc(t, &bidResponse, responseBody.String())
			}
		})
	}
}

func TestBidRequestResponseFlow_ErrorCases(t *testing.T) {
	server := setupTestServer()
	defer server.Close()

	tests := []struct {
		name           string
		requestBody    string
		expectedStatus int
		validateFunc   func(t *testing.T, responseBody string)
	}{
		{
			name:           "Invalid JSON",
			requestBody:    `{"invalid": json}`,
			expectedStatus: http.StatusBadRequest,
			validateFunc: func(t *testing.T, responseBody string) {
				var errorResponse map[string]interface{}
				err := json.Unmarshal([]byte(responseBody), &errorResponse)
				require.NoError(t, err)

				errorData := errorResponse["error"].(map[string]interface{})
				assert.Equal(t, "INVALID_JSON_FORMAT", errorData["code"])
				assert.Equal(t, "validation", errorData["type"])
			},
		},
		{
			name:           "Empty Request Body",
			requestBody:    ``,
			expectedStatus: http.StatusBadRequest,
			validateFunc: func(t *testing.T, responseBody string) {
				var errorResponse map[string]interface{}
				err := json.Unmarshal([]byte(responseBody), &errorResponse)
				require.NoError(t, err)

				errorData := errorResponse["error"].(map[string]interface{})
				assert.Equal(t, "EMPTY_REQUEST_BODY", errorData["code"])
				assert.Equal(t, "validation", errorData["type"])
			},
		},
		{
			name: "Missing Required Fields",
			requestBody: `{
				"id": "",
				"imp": []
			}`,
			expectedStatus: http.StatusBadRequest,
			validateFunc: func(t *testing.T, responseBody string) {
				var errorResponse map[string]interface{}
				err := json.Unmarshal([]byte(responseBody), &errorResponse)
				require.NoError(t, err)

				errorData := errorResponse["error"].(map[string]interface{})
				assert.Equal(t, "MISSING_BID_REQUEST_ID", errorData["code"])
				assert.Equal(t, "validation", errorData["type"])
			},
		},
		{
			name: "Unsupported OpenRTB Version",
			requestBody: `{
				"id": "test-unsupported",
				"imp": [{
					"id": "imp-1",
					"banner": {
						"w": 300,
						"h": 250
					}
				}],
				"ext": {
					"version": "1.0"
				}
			}`,
			expectedStatus: http.StatusBadRequest,
			validateFunc: func(t *testing.T, responseBody string) {
				var errorResponse map[string]interface{}
				err := json.Unmarshal([]byte(responseBody), &errorResponse)
				require.NoError(t, err)

				errorData := errorResponse["error"].(map[string]interface{})
				assert.Equal(t, "UNSUPPORTED_OPENRTB_VERSION", errorData["code"])
				assert.Equal(t, "validation", errorData["type"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("POST", server.URL+"/bid/request", strings.NewReader(tt.requestBody))
			require.NoError(t, err)

			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			var responseBody bytes.Buffer
			_, err = responseBody.ReadFrom(resp.Body)
			require.NoError(t, err)

			tt.validateFunc(t, responseBody.String())
		})
	}
}

func TestBidRequestResponseFlow_Middleware(t *testing.T) {
	server := setupTestServer()
	defer server.Close()

	bidRequest := &openrtb.BidRequest{
		ID: "middleware-test",
		Imp: []openrtb.Impression{
			{
				ID: "imp-middleware",
				Banner: &openrtb.Banner{
					W: 320,
					H: 50,
				},
				BidFloor: 0.1,
			},
		},
		Site: &openrtb.Site{
			Inventory: openrtb.Inventory{
				ID:     "middleware-site",
				Domain: "middleware.example.com",
			},
		},
	}

	requestBody, err := json.Marshal(bidRequest)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", server.URL+"/bid/request", bytes.NewBuffer(requestBody))
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "MiddlewareTestClient/1.0")
	req.Header.Set("Origin", "https://test.example.com")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Test middleware functionality
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// RequestID middleware
	requestID := resp.Header.Get("X-Request-ID")
	assert.NotEmpty(t, requestID)
	assert.Len(t, requestID, 36) // UUID format

	// CORS middleware
	assert.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))
	assert.Contains(t, resp.Header.Get("Access-Control-Allow-Methods"), "POST")
	assert.Contains(t, resp.Header.Get("Access-Control-Allow-Headers"), "Content-Type")

	// Content type should be set by handler
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
}

func TestBidRequestResponseFlow_Performance(t *testing.T) {
	server := setupTestServer()
	defer server.Close()

	bidRequest := &openrtb.BidRequest{
		ID: "perf-test",
		Imp: []openrtb.Impression{
			{
				ID: "imp-perf",
				Banner: &openrtb.Banner{
					W: 300,
					H: 250,
				},
				BidFloor: 1.0,
			},
		},
		Site: &openrtb.Site{
			Inventory: openrtb.Inventory{
				ID:     "perf-site",
				Domain: "performance.example.com",
			},
		},
		TMax: 50, // 50ms timeout
	}

	requestBody, err := json.Marshal(bidRequest)
	require.NoError(t, err)

	// Measure response time
	start := time.Now()

	req, err := http.NewRequest("POST", server.URL+"/bid/request", bytes.NewBuffer(requestBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 1 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	duration := time.Since(start)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Less(t, duration, 100*time.Millisecond, "Response should be fast")

	var responseBody bytes.Buffer
	_, err = responseBody.ReadFrom(resp.Body)
	require.NoError(t, err)

	var bidResponse openrtb.BidResponse
	err = json.Unmarshal(responseBody.Bytes(), &bidResponse)
	require.NoError(t, err)

	assert.Equal(t, "perf-test", bidResponse.ID)
	assert.NotEmpty(t, bidResponse.BidID)
}

func TestBidRequestResponseFlow_ConcurrentRequests(t *testing.T) {
	server := setupTestServer()
	defer server.Close()

	const numRequests = 10
	results := make(chan struct {
		statusCode int
		requestID  string
		err        error
	}, numRequests)

	// Send concurrent requests
	for i := 0; i < numRequests; i++ {
		go func(requestNum int) {
			bidRequest := &openrtb.BidRequest{
				ID: fmt.Sprintf("concurrent-test-%d", requestNum),
				Imp: []openrtb.Impression{
					{
						ID: fmt.Sprintf("imp-concurrent-%d", requestNum),
						Banner: &openrtb.Banner{
							W: 300,
							H: 250,
						},
						BidFloor: 1.0,
					},
				},
				Site: &openrtb.Site{
					Inventory: openrtb.Inventory{
						ID:     "concurrent-site",
						Domain: "concurrent.example.com",
					},
				},
			}

			requestBody, err := json.Marshal(bidRequest)
			if err != nil {
				results <- struct {
					statusCode int
					requestID  string
					err        error
				}{0, "", err}
				return
			}

			req, err := http.NewRequest("POST", server.URL+"/bid/request", bytes.NewBuffer(requestBody))
			if err != nil {
				results <- struct {
					statusCode int
					requestID  string
					err        error
				}{0, "", err}
				return
			}
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				results <- struct {
					statusCode int
					requestID  string
					err        error
				}{0, "", err}
				return
			}
			defer resp.Body.Close()

			results <- struct {
				statusCode int
				requestID  string
				err        error
			}{resp.StatusCode, resp.Header.Get("X-Request-ID"), nil}
		}(i)
	}

	// Collect results
	successCount := 0
	requestIDs := make(map[string]bool)

	for i := 0; i < numRequests; i++ {
		result := <-results
		require.NoError(t, result.err)

		if result.statusCode == http.StatusOK {
			successCount++
		}

		// Ensure unique request IDs
		assert.NotEmpty(t, result.requestID)
		assert.False(t, requestIDs[result.requestID], "Request ID should be unique: %s", result.requestID)
		requestIDs[result.requestID] = true
	}

	assert.Equal(t, numRequests, successCount, "All concurrent requests should succeed")
	assert.Len(t, requestIDs, numRequests, "All request IDs should be unique")
}

func TestHealthCheckEndpoint(t *testing.T) {
	server := setupTestServer()
	defer server.Close()

	req, err := http.NewRequest("GET", server.URL+"/health", nil)
	require.NoError(t, err)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Health endpoint may return different status codes depending on dependencies
	assert.Contains(t, []int{http.StatusOK, http.StatusPartialContent, http.StatusServiceUnavailable}, resp.StatusCode)
	assert.NotEmpty(t, resp.Header.Get("X-Request-ID"))

	var responseBody bytes.Buffer
	_, err = responseBody.ReadFrom(resp.Body)
	require.NoError(t, err)

	var healthResponse map[string]interface{}
	err = json.Unmarshal(responseBody.Bytes(), &healthResponse)
	require.NoError(t, err)

	// Status can be healthy, warning, or critical depending on dependencies
	status := healthResponse["status"].(string)
	assert.Contains(t, []string{"healthy", "warning", "critical"}, status)
	assert.NotNil(t, healthResponse["goroutine_stats"])
	assert.NotNil(t, healthResponse["dependencies"])
}

func TestTestBidEndpoint(t *testing.T) {
	server := setupTestServer()
	defer server.Close()

	// Send a proper OpenRTB bid request to /bid/test endpoint
	bidRequest := &openrtb.BidRequest{
		ID: "test-bid-request",
		Imp: []openrtb.Impression{
			{
				ID: "test-imp-1",
				Banner: &openrtb.Banner{
					W: 320,
					H: 50,
				},
				BidFloor: 0.5,
			},
		},
		Site: &openrtb.Site{
			Inventory: openrtb.Inventory{
				ID:     "test-site",
				Domain: "test.example.com",
			},
		},
		Test: 1, // Mark as test request
	}

	requestBody, err := json.Marshal(bidRequest)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", server.URL+"/bid/test", bytes.NewBuffer(requestBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotEmpty(t, resp.Header.Get("X-Request-ID"))

	var responseBody bytes.Buffer
	_, err = responseBody.ReadFrom(resp.Body)
	require.NoError(t, err)

	// Parse as OpenRTB bid response since it uses the same handler
	var bidResponse openrtb.BidResponse
	err = json.Unmarshal(responseBody.Bytes(), &bidResponse)
	require.NoError(t, err)

	assert.Equal(t, "test-bid-request", bidResponse.ID)
	assert.NotEmpty(t, bidResponse.BidID)
	assert.Len(t, bidResponse.SeatBid, 1)
	assert.Len(t, bidResponse.SeatBid[0].Bid, 1)
}

// Benchmark test for performance measurement
func BenchmarkFullBidRequestResponseFlow(b *testing.B) {
	server := setupTestServer()
	defer server.Close()

	bidRequest := &openrtb.BidRequest{
		ID: "benchmark-request",
		Imp: []openrtb.Impression{
			{
				ID: "imp-benchmark",
				Banner: &openrtb.Banner{
					W: 300,
					H: 250,
				},
				BidFloor: 1.0,
			},
		},
		Site: &openrtb.Site{
			Inventory: openrtb.Inventory{
				ID:     "benchmark-site",
				Domain: "benchmark.example.com",
			},
		},
	}

	requestBody, err := json.Marshal(bidRequest)
	require.NoError(b, err)

	client := &http.Client{Timeout: 1 * time.Second}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("POST", server.URL+"/bid/request", bytes.NewBuffer(requestBody))
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			b.Fatal(err)
		}
		resp.Body.Close()
	}
}
