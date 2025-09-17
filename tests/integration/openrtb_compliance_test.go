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

	"github.com/alicebob/miniredis/v2"
	"github.com/bsm/openrtb"
	"github.com/gorilla/mux"
	"github.com/hadarco13/mini-seller/internal/config"
	"github.com/hadarco13/mini-seller/internal/handlers"
	"github.com/hadarco13/mini-seller/internal/middleware"
	"github.com/hadarco13/mini-seller/internal/redis"
	"github.com/prometheus/client_golang/prometheus"
	redisClient "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupOpenRTBTestServer() (*miniredis.Miniredis, *httptest.Server, func()) {
	resetOpenRTBPrometheusRegistry()
	setupOpenRTBTestConfig()

	// Start miniredis
	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		panic(err)
	}

	// Update config to point to miniredis
	os.Setenv("MINI_SELLER_REDIS_HOST", strings.Split(mr.Addr(), ":")[0])
	os.Setenv("MINI_SELLER_REDIS_PORT", strings.Split(mr.Addr(), ":")[1])

	config.LoadConfig()

	// Create and set test Redis client
	testClient := redisClient.NewClient(&redisClient.Options{
		Addr: mr.Addr(),
		DB:   0,
	})
	redis.SetTestClient(testClient)

	// Setup HTTP server
	router := mux.NewRouter()
	router.Use(middleware.ErrorHandler)
	router.Use(middleware.RequestIDMiddleware)
	router.Use(middleware.CORSMiddleware)
	router.Use(middleware.LoggingMiddleware)

	router.HandleFunc("/bid/request", handlers.BidRequestHandler).Methods("POST")
	router.HandleFunc("/health", handlers.HealthHandler).Methods("GET")

	httpServer := httptest.NewServer(router)

	cleanup := func() {
		testClient.Close()
		mr.Close()
		httpServer.Close()
	}

	return mr, httpServer, cleanup
}

func resetOpenRTBPrometheusRegistry() {
	prometheus.DefaultRegisterer = prometheus.NewRegistry()
}

func setupOpenRTBTestConfig() {
	os.Setenv("MINI_SELLER_ENVIRONMENT", "test")
	os.Setenv("MINI_SELLER_SERVER_PORT", "8080")
	os.Setenv("MINI_SELLER_LOG_LEVEL", "info")
}

// createValidBidRequest creates a valid OpenRTB bid request for testing
func createValidBidRequest(requestID string, version string) *openrtb.BidRequest {
	bidRequest := &openrtb.BidRequest{
		ID: requestID,
		Imp: []openrtb.Impression{
			{
				ID:       fmt.Sprintf("imp-%s", requestID),
				BidFloor: 1.0,
				Banner: &openrtb.Banner{
					W: 300,
					H: 250,
				},
			},
		},
		Site: &openrtb.Site{
			Inventory: openrtb.Inventory{
				ID:     "test-site",
				Domain: "example.com",
			},
		},
		Device: &openrtb.Device{
			UA: "Mozilla/5.0 (compatible; test)",
			IP: "192.168.1.1",
		},
	}

	// Add version-specific fields
	switch version {
	case "2.0", "2.1":
		bidRequest.AuctionType = 1 // Required for early versions
	case "2.5", "2.6":
		// For newer versions, we'll rely on the built-in validation
		// Extensions are optional and complex to set up properly
	}

	return bidRequest
}

func TestOpenRTB_BasicCompliance(t *testing.T) {
	_, server, cleanup := setupOpenRTBTestServer()
	defer cleanup()

	t.Run("Valid_BidRequest_Returns_BidResponse", func(t *testing.T) {
		bidRequest := createValidBidRequest("test-basic-001", "2.5")
		requestBody, err := json.Marshal(bidRequest)
		require.NoError(t, err)

		req, err := http.NewRequest("POST", server.URL+"/bid/request", bytes.NewBuffer(requestBody))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-OpenRTB-Version", "2.5")

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

		var bidResponse openrtb.BidResponse
		err = json.NewDecoder(resp.Body).Decode(&bidResponse)
		require.NoError(t, err)

		// Validate bid response structure
		assert.Equal(t, bidRequest.ID, bidResponse.ID)
		assert.NotEmpty(t, bidResponse.BidID)
		assert.Equal(t, "USD", bidResponse.Currency)
		assert.Len(t, bidResponse.SeatBid, 1)
		assert.Equal(t, "mini-seller", bidResponse.SeatBid[0].Seat)
		assert.Len(t, bidResponse.SeatBid[0].Bid, 1)

		// Validate bid structure
		bid := bidResponse.SeatBid[0].Bid[0]
		assert.NotEmpty(t, bid.ID)
		assert.Equal(t, bidRequest.Imp[0].ID, bid.ImpID)
		assert.Greater(t, bid.Price, 0.0)
		assert.NotEmpty(t, bid.AdID)
		assert.NotEmpty(t, bid.CreativeID)
		assert.NotEmpty(t, bid.AdvDomain)
		assert.NotEmpty(t, bid.AdMarkup)
		assert.Equal(t, 300, bid.W)
		assert.Equal(t, 250, bid.H)
	})
}

func TestOpenRTB_VersionSupport(t *testing.T) {
	_, server, cleanup := setupOpenRTBTestServer()
	defer cleanup()

	supportedVersions := []string{"2.0", "2.1", "2.2", "2.3", "2.4", "2.5", "2.6"}

	for _, version := range supportedVersions {
		t.Run(fmt.Sprintf("Version_%s_Support", strings.Replace(version, ".", "_", -1)), func(t *testing.T) {
			bidRequest := createValidBidRequest(fmt.Sprintf("test-version-%s", version), version)
			requestBody, err := json.Marshal(bidRequest)
			require.NoError(t, err)

			req, err := http.NewRequest("POST", server.URL+"/bid/request", bytes.NewBuffer(requestBody))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-OpenRTB-Version", version)

			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var bidResponse openrtb.BidResponse
			err = json.NewDecoder(resp.Body).Decode(&bidResponse)
			require.NoError(t, err)

			assert.Equal(t, bidRequest.ID, bidResponse.ID)
			assert.Len(t, bidResponse.SeatBid, 1)
		})
	}

	t.Run("Unsupported_Version_Returns_Error", func(t *testing.T) {
		bidRequest := createValidBidRequest("test-unsupported", "3.0")
		requestBody, err := json.Marshal(bidRequest)
		require.NoError(t, err)

		req, err := http.NewRequest("POST", server.URL+"/bid/request", bytes.NewBuffer(requestBody))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-OpenRTB-Version", "3.0")

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

func TestOpenRTB_RequiredFields(t *testing.T) {
	_, server, cleanup := setupOpenRTBTestServer()
	defer cleanup()

	client := &http.Client{Timeout: 5 * time.Second}

	t.Run("Missing_BidRequest_ID_Returns_Error", func(t *testing.T) {
		bidRequest := createValidBidRequest("test-missing-id", "2.5")
		bidRequest.ID = "" // Remove required field

		requestBody, err := json.Marshal(bidRequest)
		require.NoError(t, err)

		req, err := http.NewRequest("POST", server.URL+"/bid/request", bytes.NewBuffer(requestBody))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("Missing_Impressions_Returns_Error", func(t *testing.T) {
		bidRequest := createValidBidRequest("test-missing-imp", "2.5")
		bidRequest.Imp = []openrtb.Impression{} // Remove required field

		requestBody, err := json.Marshal(bidRequest)
		require.NoError(t, err)

		req, err := http.NewRequest("POST", server.URL+"/bid/request", bytes.NewBuffer(requestBody))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("Missing_Impression_ID_Returns_Error", func(t *testing.T) {
		bidRequest := createValidBidRequest("test-missing-imp-id", "2.5")
		bidRequest.Imp[0].ID = "" // Remove required field

		requestBody, err := json.Marshal(bidRequest)
		require.NoError(t, err)

		req, err := http.NewRequest("POST", server.URL+"/bid/request", bytes.NewBuffer(requestBody))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

func TestOpenRTB_ImpressionTypes(t *testing.T) {
	_, server, cleanup := setupOpenRTBTestServer()
	defer cleanup()

	client := &http.Client{Timeout: 5 * time.Second}

	t.Run("Banner_Impression_Returns_Banner_Ad", func(t *testing.T) {
		bidRequest := &openrtb.BidRequest{
			ID: "test-banner-001",
			Imp: []openrtb.Impression{
				{
					ID:       "imp-banner-001",
					BidFloor: 2.0,
					Banner: &openrtb.Banner{
						W: 728,
						H: 90,
					},
				},
			},
		}

		requestBody, err := json.Marshal(bidRequest)
		require.NoError(t, err)

		req, err := http.NewRequest("POST", server.URL+"/bid/request", bytes.NewBuffer(requestBody))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var bidResponse openrtb.BidResponse
		err = json.NewDecoder(resp.Body).Decode(&bidResponse)
		require.NoError(t, err)

		bid := bidResponse.SeatBid[0].Bid[0]
		assert.Equal(t, 728, bid.W)
		assert.Equal(t, 90, bid.H)
		assert.Contains(t, bid.AdMarkup, "728px")
		assert.Contains(t, bid.AdMarkup, "90px")
		assert.Contains(t, bid.AdMarkup, "Mini Seller Ad")
		assert.Greater(t, bid.Price, 2.0) // Should be bid floor + 10%
	})

	t.Run("Video_Impression_Returns_Video_Ad", func(t *testing.T) {
		bidRequest := &openrtb.BidRequest{
			ID: "test-video-001",
			Imp: []openrtb.Impression{
				{
					ID:       "imp-video-001",
					BidFloor: 3.0,
					Video: &openrtb.Video{
						W:           640,
						H:           480,
						MinDuration: 15,
						MaxDuration: 30,
						Mimes:       []string{"video/mp4"},
					},
				},
			},
		}

		requestBody, err := json.Marshal(bidRequest)
		require.NoError(t, err)

		req, err := http.NewRequest("POST", server.URL+"/bid/request", bytes.NewBuffer(requestBody))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var bidResponse openrtb.BidResponse
		err = json.NewDecoder(resp.Body).Decode(&bidResponse)
		require.NoError(t, err)

		bid := bidResponse.SeatBid[0].Bid[0]
		assert.Equal(t, 640, bid.W)
		assert.Equal(t, 480, bid.H)
		assert.Contains(t, bid.AdMarkup, "VAST")
		assert.Contains(t, bid.AdMarkup, "Mini Seller")
		assert.Contains(t, bid.AdMarkup, "640")
		assert.Contains(t, bid.AdMarkup, "480")
		assert.Greater(t, bid.Price, 3.0) // Should be bid floor + 10%
	})

	t.Run("Multiple_Impressions_Returns_Multiple_Bids", func(t *testing.T) {
		bidRequest := &openrtb.BidRequest{
			ID: "test-multi-001",
			Imp: []openrtb.Impression{
				{
					ID:       "imp-banner-multi",
					BidFloor: 1.5,
					Banner: &openrtb.Banner{
						W: 300,
						H: 250,
					},
				},
				{
					ID:       "imp-video-multi",
					BidFloor: 2.5,
					Video: &openrtb.Video{
						W:           640,
						H:           360,
						MinDuration: 10,
						MaxDuration: 60,
					},
				},
			},
		}

		requestBody, err := json.Marshal(bidRequest)
		require.NoError(t, err)

		req, err := http.NewRequest("POST", server.URL+"/bid/request", bytes.NewBuffer(requestBody))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var bidResponse openrtb.BidResponse
		err = json.NewDecoder(resp.Body).Decode(&bidResponse)
		require.NoError(t, err)

		assert.Len(t, bidResponse.SeatBid[0].Bid, 2)

		// Find banner and video bids
		var bannerBid, videoBid *openrtb.Bid
		for _, bid := range bidResponse.SeatBid[0].Bid {
			if bid.ImpID == "imp-banner-multi" {
				bannerBid = &bid
			} else if bid.ImpID == "imp-video-multi" {
				videoBid = &bid
			}
		}

		require.NotNil(t, bannerBid)
		require.NotNil(t, videoBid)

		// Validate banner bid
		assert.Equal(t, 300, bannerBid.W)
		assert.Equal(t, 250, bannerBid.H)
		assert.Contains(t, bannerBid.AdMarkup, "Mini Seller Ad")

		// Validate video bid
		assert.Equal(t, 640, videoBid.W)
		assert.Equal(t, 360, videoBid.H)
		assert.Contains(t, videoBid.AdMarkup, "VAST")
	})
}

func TestOpenRTB_ErrorHandling(t *testing.T) {
	_, server, cleanup := setupOpenRTBTestServer()
	defer cleanup()

	client := &http.Client{Timeout: 5 * time.Second}

	t.Run("Empty_Request_Body_Returns_Error", func(t *testing.T) {
		req, err := http.NewRequest("POST", server.URL+"/bid/request", bytes.NewBuffer([]byte{}))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("Invalid_JSON_Returns_Error", func(t *testing.T) {
		invalidJSON := `{"id": "test", "imp": [`

		req, err := http.NewRequest("POST", server.URL+"/bid/request", bytes.NewBuffer([]byte(invalidJSON)))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("Malformed_BidRequest_Returns_Error", func(t *testing.T) {
		malformedJSON := `{"invalid": "structure"}`

		req, err := http.NewRequest("POST", server.URL+"/bid/request", bytes.NewBuffer([]byte(malformedJSON)))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

func TestOpenRTB_VersionDetection(t *testing.T) {
	_, server, cleanup := setupOpenRTBTestServer()
	defer cleanup()

	client := &http.Client{Timeout: 5 * time.Second}

	t.Run("Header_Version_Detection", func(t *testing.T) {
		bidRequest := createValidBidRequest("test-header-version", "2.4")
		requestBody, err := json.Marshal(bidRequest)
		require.NoError(t, err)

		// Test X-OpenRTB-Version header
		req, err := http.NewRequest("POST", server.URL+"/bid/request", bytes.NewBuffer(requestBody))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-OpenRTB-Version", "2.4")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// Test OpenRTB-Version header
		req2, err := http.NewRequest("POST", server.URL+"/bid/request", bytes.NewBuffer(requestBody))
		require.NoError(t, err)
		req2.Header.Set("Content-Type", "application/json")
		req2.Header.Set("OpenRTB-Version", "2.4")

		resp2, err := client.Do(req2)
		require.NoError(t, err)
		defer resp2.Body.Close()

		assert.Equal(t, http.StatusOK, resp2.StatusCode)
	})

	t.Run("Default_Version_When_No_Header", func(t *testing.T) {
		bidRequest := createValidBidRequest("test-default-version", "2.5")
		requestBody, err := json.Marshal(bidRequest)
		require.NoError(t, err)

		req, err := http.NewRequest("POST", server.URL+"/bid/request", bytes.NewBuffer(requestBody))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		// No version header - should default to 2.5

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

func TestOpenRTB_BidPricing(t *testing.T) {
	_, server, cleanup := setupOpenRTBTestServer()
	defer cleanup()

	client := &http.Client{Timeout: 5 * time.Second}

	t.Run("Bid_Floor_Respected", func(t *testing.T) {
		bidRequest := &openrtb.BidRequest{
			ID: "test-pricing-001",
			Imp: []openrtb.Impression{
				{
					ID:       "imp-pricing-001",
					BidFloor: 5.0,
					Banner: &openrtb.Banner{
						W: 300,
						H: 250,
					},
				},
			},
		}

		requestBody, err := json.Marshal(bidRequest)
		require.NoError(t, err)

		req, err := http.NewRequest("POST", server.URL+"/bid/request", bytes.NewBuffer(requestBody))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		var bidResponse openrtb.BidResponse
		err = json.NewDecoder(resp.Body).Decode(&bidResponse)
		require.NoError(t, err)

		bid := bidResponse.SeatBid[0].Bid[0]
		assert.Greater(t, bid.Price, 5.0) // Should be above bid floor
		assert.Equal(t, 5.5, bid.Price)   // Should be bid floor + 10%
	})

	t.Run("Default_Price_When_No_Floor", func(t *testing.T) {
		bidRequest := &openrtb.BidRequest{
			ID: "test-pricing-002",
			Imp: []openrtb.Impression{
				{
					ID:       "imp-pricing-002",
					BidFloor: 0, // No bid floor
					Banner: &openrtb.Banner{
						W: 300,
						H: 250,
					},
				},
			},
		}

		requestBody, err := json.Marshal(bidRequest)
		require.NoError(t, err)

		req, err := http.NewRequest("POST", server.URL+"/bid/request", bytes.NewBuffer(requestBody))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		var bidResponse openrtb.BidResponse
		err = json.NewDecoder(resp.Body).Decode(&bidResponse)
		require.NoError(t, err)

		bid := bidResponse.SeatBid[0].Bid[0]
		assert.Equal(t, 1.5, bid.Price) // Should be default price
	})
}

func TestOpenRTB_ResponseValidation(t *testing.T) {
	_, server, cleanup := setupOpenRTBTestServer()
	defer cleanup()

	client := &http.Client{Timeout: 5 * time.Second}

	t.Run("Response_Contains_Required_Fields", func(t *testing.T) {
		bidRequest := createValidBidRequest("test-response-validation", "2.5")
		requestBody, err := json.Marshal(bidRequest)
		require.NoError(t, err)

		req, err := http.NewRequest("POST", server.URL+"/bid/request", bytes.NewBuffer(requestBody))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		var bidResponse openrtb.BidResponse
		err = json.NewDecoder(resp.Body).Decode(&bidResponse)
		require.NoError(t, err)

		// Validate required bid response fields
		assert.NotEmpty(t, bidResponse.ID, "Bid response ID is required")
		assert.NotEmpty(t, bidResponse.BidID, "Bid response BidID is required")
		assert.NotEmpty(t, bidResponse.Currency, "Currency is required")
		assert.NotEmpty(t, bidResponse.SeatBid, "At least one seat bid is required")

		// Validate seat bid fields
		seatBid := bidResponse.SeatBid[0]
		assert.NotEmpty(t, seatBid.Seat, "Seat identifier is required")
		assert.NotEmpty(t, seatBid.Bid, "At least one bid is required")

		// Validate bid fields
		bid := seatBid.Bid[0]
		assert.NotEmpty(t, bid.ID, "Bid ID is required")
		assert.NotEmpty(t, bid.ImpID, "Impression ID reference is required")
		assert.Greater(t, bid.Price, 0.0, "Bid price must be greater than 0")
		assert.NotEmpty(t, bid.AdID, "Ad ID is required")
		assert.NotEmpty(t, bid.CreativeID, "Creative ID is required")
		assert.NotEmpty(t, bid.AdvDomain, "Advertiser domain is required")
	})

	t.Run("Response_Currency_Is_USD", func(t *testing.T) {
		bidRequest := createValidBidRequest("test-currency", "2.5")
		requestBody, err := json.Marshal(bidRequest)
		require.NoError(t, err)

		req, err := http.NewRequest("POST", server.URL+"/bid/request", bytes.NewBuffer(requestBody))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		var bidResponse openrtb.BidResponse
		err = json.NewDecoder(resp.Body).Decode(&bidResponse)
		require.NoError(t, err)

		assert.Equal(t, "USD", bidResponse.Currency)
	})
}

func TestOpenRTB_StressTest(t *testing.T) {
	_, server, cleanup := setupOpenRTBTestServer()
	defer cleanup()

	client := &http.Client{Timeout: 10 * time.Second}

	t.Run("Concurrent_Requests_Handle_OpenRTB_Compliance", func(t *testing.T) {
		const numRequests = 20
		results := make(chan error, numRequests)

		for i := 0; i < numRequests; i++ {
			go func(index int) {
				bidRequest := createValidBidRequest(fmt.Sprintf("test-concurrent-%d", index), "2.5")
				requestBody, err := json.Marshal(bidRequest)
				if err != nil {
					results <- err
					return
				}

				req, err := http.NewRequest("POST", server.URL+"/bid/request", bytes.NewBuffer(requestBody))
				if err != nil {
					results <- err
					return
				}
				req.Header.Set("Content-Type", "application/json")

				resp, err := client.Do(req)
				if err != nil {
					results <- err
					return
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					results <- fmt.Errorf("unexpected status code: %d", resp.StatusCode)
					return
				}

				var bidResponse openrtb.BidResponse
				if err := json.NewDecoder(resp.Body).Decode(&bidResponse); err != nil {
					results <- err
					return
				}

				// Validate response structure
				if bidResponse.ID != bidRequest.ID {
					results <- fmt.Errorf("response ID mismatch: expected %s, got %s", bidRequest.ID, bidResponse.ID)
					return
				}

				results <- nil
			}(i)
		}

		// Collect all results
		for i := 0; i < numRequests; i++ {
			err := <-results
			assert.NoError(t, err, "Request %d failed", i)
		}
	})
}
