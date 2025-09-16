package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/bsm/openrtb"
	"github.com/hadarco13/mini-seller/internal/errors"
	"github.com/hadarco13/mini-seller/internal/logging"
	"github.com/hadarco13/mini-seller/internal/middleware"
	"github.com/sirupsen/logrus"
)

// BidRequestHandler handles incoming OpenRTB bid requests
func BidRequestHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	requestID := getRequestIDFromContext(r)

	// Create structured logger for this request
	logger := logging.NewLoggerFromContext(r.Context(), "bid_handler").
		WithRequestID(requestID).
		WithUserAgent(r.UserAgent()).
		WithIPAddress(r.RemoteAddr)

	logger.WithOperation("receive_bid_request").Info("Processing incoming bid request")

	// Parse the bid request (this will detect and store the version)
	bidRequest, version, err := parseBidRequestWithVersion(r)
	if err != nil {
		logger.WithOperation("parse_bid_request").
			WithDuration(time.Since(start)).
			WithError(err).
			Error("Failed to parse bid request")
		middleware.WriteErrorResponse(w, r, err)
		return
	}

	// Add version to request context for logging
	ctx := context.WithValue(r.Context(), "openrtb_version", version)
	r = r.WithContext(ctx)

	// Log successful parsing
	logger.WithOperation("parse_bid_request").
		WithBidRequestContext(bidRequest.ID, len(bidRequest.Imp)).
		WithDuration(time.Since(start)).
		Info("Bid request parsed successfully")

	// Extract and log key fields
	extractAndLogBidRequestFields(bidRequest, requestID, version)

	// Generate bid response
	responseStart := time.Now()
	bidResponse := GenerateBidResponse(bidRequest, version, requestID)

	// Log bid response generation
	bidCount := 0
	totalPrice := 0.0
	for _, seatBid := range bidResponse.SeatBid {
		bidCount += len(seatBid.Bid)
		for _, bid := range seatBid.Bid {
			totalPrice += bid.Price
		}
	}

	logger.InfoBidResponse(bidRequest.ID, bidCount, totalPrice, time.Since(responseStart))

	// Return OpenRTB bid response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(bidResponse)

	// Log complete request processing
	logger.InfoRequest(r.Method, r.URL.Path, http.StatusOK, time.Since(start))
}

// parseBidRequestWithVersion parses and validates the incoming bid request JSON, returning the detected version
func parseBidRequestWithVersion(r *http.Request) (*openrtb.BidRequest, string, error) {
	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, "", errors.NewValidationError("INVALID_REQUEST_BODY", "Failed to read request body").
			WithCause(err).
			WithContext("content_length", r.ContentLength)
	}
	defer r.Body.Close()

	// Check if body is empty
	if len(body) == 0 {
		return nil, "", errors.NewValidationError("EMPTY_REQUEST_BODY", "Request body cannot be empty").
			WithUserMessage("Please provide a valid OpenRTB bid request")
	}

	// Detect OpenRTB version from headers or default to 2.5
	version := detectOpenRTBVersion(r, body)

	// Parse JSON into OpenRTB BidRequest
	var bidRequest openrtb.BidRequest
	if err := json.Unmarshal(body, &bidRequest); err != nil {
		return nil, version, errors.NewValidationError("INVALID_JSON_FORMAT", "Invalid JSON format in bid request").
			WithCause(err).
			WithContext("body_size", len(body)).
			WithContext("detected_version", version).
			WithUserMessage("Please provide a valid JSON formatted bid request")
	}

	// Basic validation with version context
	if err := validateBidRequest(&bidRequest, version); err != nil {
		return nil, version, err
	}

	return &bidRequest, version, nil
}

// detectOpenRTBVersion detects the OpenRTB version from headers or request content
func detectOpenRTBVersion(r *http.Request, body []byte) string {
	// Check for version in custom headers
	if version := r.Header.Get("X-OpenRTB-Version"); version != "" {
		return version
	}
	if version := r.Header.Get("OpenRTB-Version"); version != "" {
		return version
	}

	// Parse JSON to look for version field in extensions or other indicators
	var rawRequest map[string]interface{}
	if err := json.Unmarshal(body, &rawRequest); err == nil {
		// Check for version in ext field
		if ext, ok := rawRequest["ext"].(map[string]interface{}); ok {
			if version, ok := ext["version"].(string); ok {
				return version
			}
			if version, ok := ext["openrtb_version"].(string); ok {
				return version
			}
		}

		// Infer version from field presence (simple heuristics)
		if _, hasSource := rawRequest["source"]; hasSource {
			return "2.5" // Source object introduced in 2.5
		}
		if _, hasRegs := rawRequest["regs"]; hasRegs {
			return "2.4" // Regulations object common in 2.4+
		}
		if imp, ok := rawRequest["imp"].([]interface{}); ok && len(imp) > 0 {
			if impMap, ok := imp[0].(map[string]interface{}); ok {
				if _, hasRwdd := impMap["rwdd"]; hasRwdd {
					return "2.6" // Rewarded video introduced in 2.6
				}
			}
		}
	}

	// Default to 2.5 if no version detected
	return "2.5"
}

// validateBidRequest performs basic validation on the bid request
func validateBidRequest(bidRequest *openrtb.BidRequest, version string) error {
	// Validate OpenRTB version if present
	if err := validateOpenRTBVersion(bidRequest, version); err != nil {
		return err
	}

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

// validateOpenRTBVersion validates and handles different OpenRTB versions
func validateOpenRTBVersion(bidRequest *openrtb.BidRequest, version string) error {

	// Log the version being processed
	logrus.WithFields(logrus.Fields{
		"bid_request_id":  bidRequest.ID,
		"openrtb_version": version,
	}).Info("Processing OpenRTB bid request")

	// Validate supported versions
	supportedVersions := map[string]bool{
		"2.0": true,
		"2.1": true,
		"2.2": true,
		"2.3": true,
		"2.4": true,
		"2.5": true,
		"2.6": true,
	}

	if !supportedVersions[version] {
		return errors.NewValidationError("UNSUPPORTED_OPENRTB_VERSION", "Unsupported OpenRTB version").
			WithContext("version", version).
			WithContext("supported_versions", []string{"2.0", "2.1", "2.2", "2.3", "2.4", "2.5", "2.6"}).
			WithUserMessage("Please use a supported OpenRTB version (2.0-2.6)")
	}

	// Apply version-specific validation rules
	switch version {
	case "2.0", "2.1":
		// Earlier versions have stricter requirements
		if bidRequest.AuctionType == 0 {
			return errors.NewValidationError("MISSING_AUCTION_TYPE", "Auction type is required for OpenRTB 2.0/2.1").
				WithContext("version", version).
				WithUserMessage("Auction type (at) field is required")
		}
	case "2.2", "2.3", "2.4", "2.5", "2.6":
		// Later versions are more flexible
		// AuctionType defaults to 1 (first-price auction) if not specified
		if bidRequest.AuctionType == 0 {
			logrus.WithField("bid_request_id", bidRequest.ID).Debug("Defaulting auction type to first-price for OpenRTB 2.2+")
		}
	}

	return nil
}

// extractAndLogBidRequestFields extracts and logs key fields from the bid request
func extractAndLogBidRequestFields(bidRequest *openrtb.BidRequest, requestID string, version string) {
	// Create base log fields

	logFields := logrus.Fields{
		"request_id":      requestID,
		"bid_request_id":  bidRequest.ID,
		"openrtb_version": version,
		"timestamp":       time.Now().Format(time.RFC3339),
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
