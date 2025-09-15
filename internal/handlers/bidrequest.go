package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/bsm/openrtb"
	"github.com/hadarco13/mini-seller/internal/errors"
	"github.com/hadarco13/mini-seller/internal/middleware"
	"github.com/sirupsen/logrus"
)

// BidRequestHandler handles incoming OpenRTB bid requests
func BidRequestHandler(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r)

	logrus.WithField("request_id", requestID).Info("Received bid request")

	// Parse the bid request
	bidRequest, err := parseBidRequest(r)
	if err != nil {
		middleware.WriteErrorResponse(w, r, err)
		return
	}

	// Extract and log key fields
	extractAndLogBidRequestFields(bidRequest, requestID)

	// For now, return a simple acknowledgment
	// Later this will be replaced with actual bid response logic
	response := map[string]interface{}{
		"status":     "received",
		"request_id": requestID,
		"bid_id":     bidRequest.ID,
		"timestamp":  time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// parseBidRequest parses and validates the incoming bid request JSON
func parseBidRequest(r *http.Request) (*openrtb.BidRequest, error) {
	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, errors.NewValidationError("INVALID_REQUEST_BODY", "Failed to read request body").
			WithCause(err).
			WithContext("content_length", r.ContentLength)
	}
	defer r.Body.Close()

	// Check if body is empty
	if len(body) == 0 {
		return nil, errors.NewValidationError("EMPTY_REQUEST_BODY", "Request body cannot be empty").
			WithUserMessage("Please provide a valid OpenRTB bid request")
	}

	// Parse JSON into OpenRTB BidRequest
	var bidRequest openrtb.BidRequest
	if err := json.Unmarshal(body, &bidRequest); err != nil {
		return nil, errors.NewValidationError("INVALID_JSON_FORMAT", "Invalid JSON format in bid request").
			WithCause(err).
			WithContext("body_size", len(body)).
			WithUserMessage("Please provide a valid JSON formatted bid request")
	}

	// Basic validation
	if err := validateBidRequest(&bidRequest); err != nil {
		return nil, err
	}

	return &bidRequest, nil
}

// validateBidRequest performs basic validation on the bid request
func validateBidRequest(bidRequest *openrtb.BidRequest) error {
	// Check required fields according to OpenRTB spec
	if bidRequest.ID == "" {
		return errors.NewValidationError("MISSING_BID_REQUEST_ID", "Bid request ID is required").
			WithUserMessage("Bid request must include a unique ID")
	}

	if len(bidRequest.Imp) == 0 {
		return errors.NewValidationError("MISSING_IMPRESSIONS", "At least one impression is required").
			WithContext("bid_request_id", bidRequest.ID).
			WithUserMessage("Bid request must include at least one impression")
	}

	// Validate each impression
	for i, imp := range bidRequest.Imp {
		if imp.ID == "" {
			return errors.NewValidationError("MISSING_IMPRESSION_ID", "Impression ID is required").
				WithContext("impression_index", i).
				WithContext("bid_request_id", bidRequest.ID)
		}
	}

	return nil
}

// extractAndLogBidRequestFields extracts and logs key fields from the bid request
func extractAndLogBidRequestFields(bidRequest *openrtb.BidRequest, requestID string) {
	// Create base log fields
	logFields := logrus.Fields{
		"request_id":     requestID,
		"bid_request_id": bidRequest.ID,
		"timestamp":      time.Now().Format(time.RFC3339),
	}

	// Extract impression details
	impressions := make([]map[string]interface{}, len(bidRequest.Imp))
	for i, imp := range bidRequest.Imp {
		impData := map[string]interface{}{
			"id":       imp.ID,
			"secure":   imp.Secure,
			"bidfloor": imp.BidFloor,
		}

		// Add banner info if present
		if imp.Banner != nil {
			impData["banner"] = map[string]interface{}{
				"w":   imp.Banner.W,
				"h":   imp.Banner.H,
				"pos": imp.Banner.Pos,
			}
		}

		// Add video info if present
		if imp.Video != nil {
			impData["video"] = map[string]interface{}{
				"w":           imp.Video.W,
				"h":           imp.Video.H,
				"minduration": imp.Video.MinDuration,
				"maxduration": imp.Video.MaxDuration,
			}
		}

		impressions[i] = impData
	}
	logFields["impressions"] = impressions

	// Extract user information
	if bidRequest.User != nil {
		userInfo := map[string]interface{}{
			"id":       bidRequest.User.ID,
			"buyeruid": bidRequest.User.BuyerUID,
		}
		if bidRequest.User.Geo != nil {
			userInfo["geo"] = map[string]interface{}{
				"country": bidRequest.User.Geo.Country,
				"region":  bidRequest.User.Geo.Region,
				"city":    bidRequest.User.Geo.City,
			}
		}
		logFields["user"] = userInfo
	}

	// Extract device information
	if bidRequest.Device != nil {
		deviceInfo := map[string]interface{}{
			"ua":             bidRequest.Device.UA,
			"ip":             bidRequest.Device.IP,
			"devicetype":     bidRequest.Device.DeviceType,
			"make":           bidRequest.Device.Make,
			"model":          bidRequest.Device.Model,
			"os":             bidRequest.Device.OS,
			"osv":            bidRequest.Device.OSVer,
			"connectiontype": bidRequest.Device.ConnType,
		}
		if bidRequest.Device.Geo != nil {
			deviceInfo["geo"] = map[string]interface{}{
				"country": bidRequest.Device.Geo.Country,
				"region":  bidRequest.Device.Geo.Region,
				"city":    bidRequest.Device.Geo.City,
			}
		}
		logFields["device"] = deviceInfo
	}

	// Extract site information
	if bidRequest.Site != nil {
		siteInfo := map[string]interface{}{
			"id":     bidRequest.Site.ID,
			"name":   bidRequest.Site.Name,
			"domain": bidRequest.Site.Domain,
			"cat":    bidRequest.Site.Cat,
			"page":   bidRequest.Site.Page,
			"ref":    bidRequest.Site.Ref,
		}
		logFields["site"] = siteInfo
	}

	// Extract app information
	if bidRequest.App != nil {
		appInfo := map[string]interface{}{
			"id":     bidRequest.App.ID,
			"name":   bidRequest.App.Name,
			"bundle": bidRequest.App.Bundle,
			"domain": bidRequest.App.Domain,
			"cat":    bidRequest.App.Cat,
		}
		logFields["app"] = appInfo
	}

	// Extract auction parameters
	auctionParams := map[string]interface{}{
		"at":      bidRequest.AuctionType,
		"tmax":    bidRequest.TMax,
		"wseat":   bidRequest.WSeat,
		"bseat":   bidRequest.BSeat,
		"allimps": bidRequest.AllImps,
		"cur":     bidRequest.Cur,
	}
	logFields["auction_params"] = auctionParams

	// Log all the extracted information
	logrus.WithFields(logFields).Info("Bid request details extracted")

	// Log a summary
	logrus.WithFields(logrus.Fields{
		"request_id":       requestID,
		"bid_request_id":   bidRequest.ID,
		"impression_count": len(bidRequest.Imp),
		"has_user":         bidRequest.User != nil,
		"has_device":       bidRequest.Device != nil,
		"has_site":         bidRequest.Site != nil,
		"has_app":          bidRequest.App != nil,
		"auction_type":     bidRequest.AuctionType,
		"timeout_ms":       bidRequest.TMax,
	}).Info("Bid request summary")
}

// getRequestIDFromContext extracts request ID from context
func getRequestIDFromContext(r *http.Request) string {
	if requestID := r.Context().Value("request_id"); requestID != nil {
		if id, ok := requestID.(string); ok {
			return id
		}
	}
	return "unknown"
}
