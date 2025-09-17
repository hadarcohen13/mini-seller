package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bsm/openrtb"
	"github.com/hadarco13/mini-seller/internal/handlers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBidRequestHandler(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    interface{}
		expectedStatus int
		expectError    bool
	}{
		{
			name: "Valid OpenRTB 2.5 Request",
			requestBody: openrtb.BidRequest{
				ID: "test-request-123",
				Imp: []openrtb.Impression{
					{
						ID: "test-imp-1",
						Banner: &openrtb.Banner{
							W: 300,
							H: 250,
						},
						BidFloor: 1.0,
					},
				},
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "Invalid JSON Request",
			requestBody:    "invalid json",
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name:        "Missing Required Fields",
			requestBody: openrtb.BidRequest{
				// Missing ID and Imp fields
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name: "Empty Impressions Array",
			requestBody: openrtb.BidRequest{
				ID:  "test-request-empty-imp",
				Imp: []openrtb.Impression{}, // Empty impressions
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body bytes.Buffer
			if str, ok := tt.requestBody.(string); ok {
				body.WriteString(str)
			} else {
				err := json.NewEncoder(&body).Encode(tt.requestBody)
				require.NoError(t, err)
			}

			req := httptest.NewRequest(http.MethodPost, "/bid/request", &body)
			req.Header.Set("Content-Type", "application/json")

			// Add request ID to context
			ctx := context.WithValue(req.Context(), "request_id", "test-request-123")
			req = req.WithContext(ctx)

			w := httptest.NewRecorder()

			// Call the handler
			handlers.BidRequestHandler(w, req)

			// Check status code
			assert.Equal(t, tt.expectedStatus, w.Code)

			// Check response body structure
			if !tt.expectError {
				var bidResponse openrtb.BidResponse
				err := json.Unmarshal(w.Body.Bytes(), &bidResponse)
				assert.NoError(t, err)
				assert.NotEmpty(t, bidResponse.ID)
			}
		})
	}
}

func TestConcurrentRequestTracking(t *testing.T) {
	// Create valid request body
	bidReq := openrtb.BidRequest{
		ID: "concurrent-test",
		Imp: []openrtb.Impression{
			{
				ID: "test-imp",
				Banner: &openrtb.Banner{
					W: 300,
					H: 250,
				},
			},
		},
	}

	body, err := json.Marshal(bidReq)
	require.NoError(t, err)

	// Make concurrent requests
	concurrency := 5
	done := make(chan bool, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(i int) {
			req := httptest.NewRequest(http.MethodPost, "/bid/request", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			ctx := context.WithValue(req.Context(), "request_id", fmt.Sprintf("concurrent-%d", i))
			req = req.WithContext(ctx)

			w := httptest.NewRecorder()
			handlers.BidRequestHandler(w, req)

			done <- true
		}(i)
	}

	// Wait for all requests to complete
	for i := 0; i < concurrency; i++ {
		<-done
	}

	// Test passes if no panics occurred
	assert.True(t, true)
}
