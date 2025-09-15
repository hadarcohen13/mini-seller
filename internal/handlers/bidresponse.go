package handlers

import (
	"fmt"
	"time"

	"github.com/bsm/openrtb"
	"github.com/sirupsen/logrus"
)

// GenerateBidResponse creates a simple bid response for the given bid request
func GenerateBidResponse(bidRequest *openrtb.BidRequest, version string, requestID string) *openrtb.BidResponse {
	logrus.WithFields(logrus.Fields{
		"request_id":       requestID,
		"bid_request_id":   bidRequest.ID,
		"impression_count": len(bidRequest.Imp),
	}).Info("Generating bid response")

	// Create bid response
	bidResponse := &openrtb.BidResponse{
		ID:       bidRequest.ID,
		Currency: "USD",
		SeatBid:  []openrtb.SeatBid{},
	}

	// Create seat bid with bids for each impression
	seatBid := openrtb.SeatBid{
		Seat: "mini-seller",
		Bid:  []openrtb.Bid{},
	}

	// Generate a simple bid for each impression
	for _, imp := range bidRequest.Imp {
		bid := generateSimpleBid(&imp, requestID)
		seatBid.Bid = append(seatBid.Bid, bid)
	}

	bidResponse.SeatBid = append(bidResponse.SeatBid, seatBid)

	logrus.WithFields(logrus.Fields{
		"request_id":     requestID,
		"bid_request_id": bidRequest.ID,
		"bids_generated": len(seatBid.Bid),
	}).Info("Bid response generated")

	return bidResponse
}

// generateSimpleBid creates a basic bid for an impression
func generateSimpleBid(imp *openrtb.Impression, requestID string) openrtb.Bid {
	// Simple pricing logic: use bid floor + 10% or default $1.50
	price := 1.50
	if imp.BidFloor > 0 {
		price = imp.BidFloor * 1.1
	}

	bid := openrtb.Bid{
		ID:         fmt.Sprintf("bid-%s-%d", imp.ID, time.Now().Unix()),
		ImpID:      imp.ID,
		Price:      price,
		AdID:       fmt.Sprintf("ad-%s", imp.ID),
		CreativeID: "creative-123",
		AdvDomain:  []string{"mini-seller.com"},
	}

	// Add simple creative based on impression type
	if imp.Banner != nil {
		bid.AdMarkup = fmt.Sprintf(`<div style="width:%dpx;height:%dpx;background:#007cba;color:white;text-align:center;line-height:%dpx;">Mini Seller Ad</div>`,
			int(imp.Banner.W), int(imp.Banner.H), int(imp.Banner.H))
		bid.W = int(imp.Banner.W)
		bid.H = int(imp.Banner.H)
	}

	return bid
}
