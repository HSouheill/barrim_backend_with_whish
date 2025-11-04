// controllers/wholesaler_referral_controller.go
package controllers

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image/png"
	"log"
	"net/http"
	"time"

	"github.com/HSouheill/barrim_backend/middleware"
	"github.com/HSouheill/barrim_backend/models"
	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/qr"
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// WholesalerReferralController handles referral related operations for wholesalers
type WholesalerReferralController struct {
	DB *mongo.Client
}

// NewWholesalerReferralController creates a new wholesaler referral controller
func NewWholesalerReferralController(db *mongo.Client) *WholesalerReferralController {
	return &WholesalerReferralController{DB: db}
}

// HandleReferral processes a wholesaler's referral code
func (rc *WholesalerReferralController) HandleReferral(c echo.Context) error {
	log.Printf("=== WHOLESALER REFERRAL HANDLER CALLED ===")

	// Get the user ID from the token
	userID, err := middleware.ExtractUserID(c)
	if err != nil {
		log.Printf("ERROR: Authentication failed: %v", err)
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Authentication failed",
		})
	}

	log.Printf("User ID from token: %s", userID)

	// Parse the request body
	var req models.ReferralRequest
	if err := c.Bind(&req); err != nil {
		log.Printf("ERROR: Invalid request format: %v", err)
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request format",
		})
	}

	log.Printf("Referral code received: %s", req.ReferralCode)

	// Validate the request
	if req.ReferralCode == "" {
		log.Printf("ERROR: Referral code is empty")
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Referral code is required",
		})
	}

	// Create a background context
	ctx := context.Background()

	// Convert userID string to ObjectID
	userObjID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		log.Printf("ERROR: Invalid user ID format: %v", err)
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	// Get the user's type
	userCollection := rc.DB.Database("barrim").Collection("users")
	var user models.User
	err = userCollection.FindOne(ctx, bson.M{"_id": userObjID}).Decode(&user)
	if err != nil {
		log.Printf("ERROR: Failed to retrieve user: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve user information",
		})
	}

	log.Printf("User type: %s", user.UserType)

	// Handle wholesaler referrals
	if user.UserType == "wholesaler" {
		log.Printf("Processing wholesaler referral...")
		return rc.handleWholesalerReferral(c, ctx, userObjID, req.ReferralCode)
	}

	// Return error if not a wholesaler
	log.Printf("ERROR: User is not a wholesaler")
	return c.JSON(http.StatusBadRequest, models.Response{
		Status:  http.StatusBadRequest,
		Message: "User is not a wholesaler",
	})
}

// handleWholesalerReferral processes referrals between wholesalers
func (rc *WholesalerReferralController) handleWholesalerReferral(c echo.Context, ctx context.Context, wholesalerUserID primitive.ObjectID, referralCode string) error {
	log.Printf("=== HANDLING WHOLESALER REFERRAL ===")
	log.Printf("Wholesaler User ID: %s", wholesalerUserID.Hex())
	log.Printf("Referral Code: %s", referralCode)

	// Collections needed
	wholesalerCollection := rc.DB.Database("barrim").Collection("wholesalers")

	// Find the referrer wholesaler by referral code
	var referrerWholesaler models.Wholesaler
	err := wholesalerCollection.FindOne(ctx, bson.M{"referralCode": referralCode}).Decode(&referrerWholesaler)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			log.Printf("ERROR: Invalid referral code - not found in database")
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Invalid referral code",
			})
		}
		log.Printf("ERROR: Failed to verify referral code: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to verify referral code",
		})
	}

	log.Printf("Found referrer wholesaler - ID: %s, Business: %s, Current Points: %d",
		referrerWholesaler.ID.Hex(), referrerWholesaler.BusinessName, referrerWholesaler.Points)

	// Find the referred wholesaler
	var currentWholesaler models.Wholesaler
	err = wholesalerCollection.FindOne(ctx, bson.M{"userId": wholesalerUserID}).Decode(&currentWholesaler)
	if err != nil {
		log.Printf("ERROR: Failed to retrieve current wholesaler: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve wholesaler information",
		})
	}

	log.Printf("Found current wholesaler - ID: %s, Business: %s",
		currentWholesaler.ID.Hex(), currentWholesaler.BusinessName)

	// Check if referral is for the same wholesaler
	if referrerWholesaler.ID == currentWholesaler.ID {
		log.Printf("ERROR: Self-referral attempted")
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Cannot use your own referral code",
		})
	}

	// Check if the wholesaler has already been referred
	for _, refID := range referrerWholesaler.Referrals {
		if refID == currentWholesaler.ID {
			log.Printf("ERROR: Referral code already used by this wholesaler")
			return c.JSON(http.StatusBadRequest, models.Response{
				Status:  http.StatusBadRequest,
				Message: "This referral code has already been used",
			})
		}
	}

	// Update the referrer wholesaler - add points and add to referrals list
	const pointsToAdd = 5
	update := bson.M{
		"$inc":  bson.M{"points": pointsToAdd},
		"$push": bson.M{"referrals": currentWholesaler.ID},
		"$set":  bson.M{"updatedAt": time.Now()},
	}

	log.Printf("Updating wholesaler %s with points increment: %d", referrerWholesaler.ID.Hex(), pointsToAdd)

	_, err = wholesalerCollection.UpdateByID(ctx, referrerWholesaler.ID, update)
	if err != nil {
		log.Printf("ERROR: Failed to update wholesaler points: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update referrer wholesaler",
		})
	}

	log.Printf("Successfully updated wholesaler points in database")

	// Fetch the updated wholesaler data to get the new points
	var updatedReferrerWholesaler models.Wholesaler
	err = wholesalerCollection.FindOne(ctx, bson.M{"_id": referrerWholesaler.ID}).Decode(&updatedReferrerWholesaler)
	if err != nil {
		log.Printf("ERROR: Failed to fetch updated wholesaler data: %v", err)
		// Continue with original data if fetch fails
		updatedReferrerWholesaler = referrerWholesaler
	}

	log.Printf("Wholesaler %s points updated from %d to %d",
		referrerWholesaler.ID.Hex(), referrerWholesaler.Points, updatedReferrerWholesaler.Points)

	// Prepare the response
	response := models.WholesalerReferralResponse{
		ReferrerID:      updatedReferrerWholesaler.ID,
		Referrer:        updatedReferrerWholesaler,
		NewWholesaler:   currentWholesaler,
		PointsAdded:     pointsToAdd,
		NewReferralCode: currentWholesaler.ReferralCode,
	}

	log.Printf("=== WHOLESALER REFERRAL COMPLETED SUCCESSFULLY ===")

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Referral successfully applied",
		Data:    response,
	})
}

// GetReferralData fetches referral statistics for the current wholesaler
func (rc *WholesalerReferralController) GetReferralData(c echo.Context) error {
	log.Printf("=== GETTING WHOLESALER REFERRAL DATA ===")

	// Get the user ID from the token
	userID, err := middleware.ExtractUserID(c)
	if err != nil {
		log.Printf("ERROR: Authentication failed: %v", err)
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Authentication failed",
		})
	}

	log.Printf("User ID: %s", userID)

	// Convert userID string to ObjectID
	userObjID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		log.Printf("ERROR: Invalid user ID format: %v", err)
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	// Get the user type
	ctx := context.Background()
	userCollection := rc.DB.Database("barrim").Collection("users")

	var user models.User
	err = userCollection.FindOne(ctx, bson.M{"_id": userObjID}).Decode(&user)
	if err != nil {
		log.Printf("ERROR: Failed to retrieve user: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve user",
		})
	}

	log.Printf("User type: %s", user.UserType)

	// Verify that the user is a wholesaler
	if user.UserType != "wholesaler" {
		log.Printf("ERROR: User is not a wholesaler")
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "User is not a wholesaler",
		})
	}

	// Handle wholesaler referral data
	return rc.getWholesalerReferralData(c, ctx, userObjID)
}

// getWholesalerReferralData fetches referral statistics for wholesalers
func (rc *WholesalerReferralController) getWholesalerReferralData(c echo.Context, ctx context.Context, userID primitive.ObjectID) error {
	log.Printf("Fetching wholesaler data for user ID: %s", userID.Hex())

	wholesalerCollection := rc.DB.Database("barrim").Collection("wholesalers")

	var wholesaler models.Wholesaler
	err := wholesalerCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&wholesaler)
	if err != nil {
		log.Printf("ERROR: Failed to retrieve wholesaler information: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve wholesaler information",
		})
	}

	log.Printf("Retrieved wholesaler data - ID: %s, Business: %s, Points: %d, Referrals: %d",
		wholesaler.ID.Hex(), wholesaler.BusinessName, wholesaler.Points, len(wholesaler.Referrals))

	// Generate QR code for wholesaler referral
	qrCode, err := rc.GenerateReferralQRCode(wholesaler.ReferralCode)
	if err != nil {
		// Log the error but continue, just won't have QR in response
		log.Printf("WARNING: Failed to generate QR code: %v", err)
	}

	// Create response with referral data
	referralData := models.WholesalerReferralData{
		ReferralCode:  wholesaler.ReferralCode,
		ReferralCount: len(wholesaler.Referrals),
		Points:        wholesaler.Points,
		ReferralLink:  fmt.Sprintf("https://barrim.com/referral?code=%s", wholesaler.ReferralCode),
	}

	responseData := map[string]interface{}{
		"referralData": referralData,
		"qrCode":       qrCode,
	}

	log.Printf("Returning referral data - Points: %d, ReferralCount: %d", referralData.Points, referralData.ReferralCount)

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Referral data retrieved successfully",
		Data:    responseData,
	})
}

// GenerateReferralQRCode creates a QR code image for a referral code
func (rc *WholesalerReferralController) GenerateReferralQRCode(referralCode string) (string, error) {
	// Create the QR code content - usually a URL or the code itself
	content := fmt.Sprintf("https://barrim.com/referral?code=%s", referralCode)

	// Generate the QR code
	qrCode, err := qr.Encode(content, qr.M, qr.Auto)
	if err != nil {
		return "", err
	}

	// Scale the QR code to a reasonable size (300x300 pixels)
	qrCode, err = barcode.Scale(qrCode, 300, 300)
	if err != nil {
		return "", err
	}

	// Convert the QR code to a PNG image
	var buf bytes.Buffer
	err = png.Encode(&buf, qrCode)
	if err != nil {
		return "", err
	}

	// Convert to base64 for embedding in responses
	base64QR := base64.StdEncoding.EncodeToString(buf.Bytes())
	return "data:image/png;base64," + base64QR, nil
}

// GetWholesalerReferralQRCode endpoint to get QR code for a wholesaler referral code
func (rc *WholesalerReferralController) GetWholesalerReferralQRCode(c echo.Context) error {
	// Get the user ID from the token
	userID, err := middleware.ExtractUserID(c)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Authentication failed",
		})
	}

	// Convert userID string to ObjectID
	userObjID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	ctx := context.Background()
	userCollection := rc.DB.Database("barrim").Collection("users")

	// Get user to determine type
	var user models.User
	err = userCollection.FindOne(ctx, bson.M{"_id": userObjID}).Decode(&user)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve user",
		})
	}

	// Verify that the user is a wholesaler
	if user.UserType != "wholesaler" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "User is not a wholesaler",
		})
	}

	// Get wholesaler referral code
	wholesalerCollection := rc.DB.Database("barrim").Collection("wholesalers")
	var wholesaler models.Wholesaler
	err = wholesalerCollection.FindOne(ctx, bson.M{"userId": userObjID}).Decode(&wholesaler)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve wholesaler information",
		})
	}

	referralCode := wholesaler.ReferralCode

	// Generate QR code
	qrCodeBase64, err := rc.GenerateReferralQRCode(referralCode)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to generate QR code",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "QR code generated successfully",
		Data: map[string]interface{}{
			"qrCode":       qrCodeBase64,
			"referralCode": referralCode,
		},
	})
}
