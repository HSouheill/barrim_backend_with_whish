// controllers/unified_referral_controller.go
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
	"github.com/HSouheill/barrim_backend/utils"
	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/qr"
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// UnifiedReferralController handles referral operations for all user types
type UnifiedReferralController struct {
	DB *mongo.Client
}

// NewUnifiedReferralController creates a new unified referral controller
func NewUnifiedReferralController(db *mongo.Client) *UnifiedReferralController {
	return &UnifiedReferralController{DB: db}
}

// ReferralEntity represents any entity that can be referred or refer others
type ReferralEntity struct {
	ID           primitive.ObjectID   `json:"id" bson:"_id"`
	Type         string               `json:"type" bson:"type"` // "user", "company", "wholesaler", "serviceProvider"
	ReferralCode string               `json:"referralCode" bson:"referralCode"`
	Points       int                  `json:"points" bson:"points"`
	Referrals    []primitive.ObjectID `json:"referrals" bson:"referrals"`
	BusinessName string               `json:"businessName,omitempty" bson:"businessName,omitempty"`
	FullName     string               `json:"fullName,omitempty" bson:"fullName,omitempty"`
}

// HandleReferral processes referrals between any user types
func (rc *UnifiedReferralController) HandleReferral(c echo.Context) error {
	log.Printf("=== UNIFIED REFERRAL HANDLER CALLED ===")

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

	// Get the current user's information
	userCollection := rc.DB.Database("barrim").Collection("users")
	var currentUser models.User
	err = userCollection.FindOne(ctx, bson.M{"_id": userObjID}).Decode(&currentUser)
	if err != nil {
		log.Printf("ERROR: Failed to retrieve current user: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve user information",
		})
	}

	log.Printf("Current user type: %s", currentUser.UserType)

	// Find the referrer by referral code across all collections
	referrerEntity, err := rc.findReferrerByCode(ctx, req.ReferralCode)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			log.Printf("ERROR: Referral code not found")
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Invalid referral code",
			})
		}
		log.Printf("ERROR: Failed to find referrer: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to verify referral code",
		})
	}

	log.Printf("Found referrer - Type: %s, ID: %s", referrerEntity.Type, referrerEntity.ID.Hex())

	// Prevent self-referral
	if referrerEntity.ID == userObjID {
		log.Printf("ERROR: Self-referral attempted")
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Cannot use your own referral code",
		})
	}

	// Check if the current user has already been referred by this referrer
	for _, refID := range referrerEntity.Referrals {
		if refID == userObjID {
			log.Printf("ERROR: User already referred by this referrer")
			return c.JSON(http.StatusBadRequest, models.Response{
				Status:  http.StatusBadRequest,
				Message: "This referral code has already been used",
			})
		}
	}

	// Process the referral based on user types
	return rc.processReferral(c, ctx, currentUser, referrerEntity, userObjID)
}

// findReferrerByCode searches for a referrer across all collections
func (rc *UnifiedReferralController) findReferrerByCode(ctx context.Context, referralCode string) (*ReferralEntity, error) {
	// Search in users collection
	userCollection := rc.DB.Database("barrim").Collection("users")
	var user models.User
	err := userCollection.FindOne(ctx, bson.M{"referralCode": referralCode}).Decode(&user)
	if err == nil {
		return &ReferralEntity{
			ID:           user.ID,
			Type:         "user",
			ReferralCode: user.ReferralCode,
			Points:       user.Points,
			Referrals:    user.Referrals,
			FullName:     user.FullName,
		}, nil
	}

	// Search in companies collection
	companyCollection := rc.DB.Database("barrim").Collection("companies")
	var company models.Company
	err = companyCollection.FindOne(ctx, bson.M{"referralCode": referralCode}).Decode(&company)
	if err == nil {
		return &ReferralEntity{
			ID:           company.ID,
			Type:         "company",
			ReferralCode: company.ReferralCode,
			Points:       company.Points,
			Referrals:    company.Referrals,
			BusinessName: company.BusinessName,
		}, nil
	}

	// Search in wholesalers collection
	wholesalerCollection := rc.DB.Database("barrim").Collection("wholesalers")
	var wholesaler models.Wholesaler
	err = wholesalerCollection.FindOne(ctx, bson.M{"referralCode": referralCode}).Decode(&wholesaler)
	if err == nil {
		return &ReferralEntity{
			ID:           wholesaler.ID,
			Type:         "wholesaler",
			ReferralCode: wholesaler.ReferralCode,
			Points:       wholesaler.Points,
			Referrals:    wholesaler.Referrals,
			BusinessName: wholesaler.BusinessName,
		}, nil
	}

	// Search in serviceProviders collection
	serviceProviderCollection := rc.DB.Database("barrim").Collection("serviceProviders")
	var serviceProvider models.ServiceProvider
	err = serviceProviderCollection.FindOne(ctx, bson.M{"referralCode": referralCode}).Decode(&serviceProvider)
	if err == nil {
		return &ReferralEntity{
			ID:           serviceProvider.ID,
			Type:         "serviceProvider",
			ReferralCode: serviceProvider.ReferralCode,
			Points:       serviceProvider.Points,
			Referrals:    serviceProvider.Referrals,
			BusinessName: serviceProvider.BusinessName,
		}, nil
	}

	return nil, mongo.ErrNoDocuments
}

// processReferral handles the actual referral processing
func (rc *UnifiedReferralController) processReferral(c echo.Context, ctx context.Context, currentUser models.User, referrerEntity *ReferralEntity, userObjID primitive.ObjectID) error {
	// All user types get 5 points for any referral
	pointsToAdd := rc.calculateReferralPoints(referrerEntity.Type, currentUser.UserType)

	log.Printf("Processing referral - Referrer: %s, Referee: %s, Points: %d",
		referrerEntity.Type, currentUser.UserType, pointsToAdd)

	// Update the referrer's points and add to referrals list
	err := rc.updateReferrerPoints(ctx, referrerEntity, userObjID, pointsToAdd)
	if err != nil {
		log.Printf("ERROR: Failed to update referrer points: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update referrer",
		})
	}

	// Generate referral code for the new user if they don't have one
	if currentUser.ReferralCode == "" {
		referralCode, err := utils.GenerateReferralCode(utils.UserType)
		if err != nil {
			log.Printf("ERROR: Failed to generate referral code: %v", err)
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to generate referral code",
			})
		}

		// Update user with referral code
		userCollection := rc.DB.Database("barrim").Collection("users")
		_, err = userCollection.UpdateByID(ctx, userObjID, bson.M{
			"$set": bson.M{
				"referralCode": referralCode,
				"updatedAt":    time.Now(),
			},
		})
		if err != nil {
			log.Printf("ERROR: Failed to set user referral code: %v", err)
		}
	}

	log.Printf("=== UNIFIED REFERRAL COMPLETED SUCCESSFULLY ===")

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Referral successfully applied",
		Data: map[string]interface{}{
			"referrerID":      referrerEntity.ID,
			"referrerType":    referrerEntity.Type,
			"pointsAdded":     pointsToAdd,
			"newReferralCode": currentUser.ReferralCode,
		},
	})
}

// calculateReferralPoints determines points based on referrer and referee types
func (rc *UnifiedReferralController) calculateReferralPoints(referrerType, refereeType string) int {
	// All user types get 5 points for any referral
	return 5
}

// updateReferrerPoints updates the referrer's points in the appropriate collection
func (rc *UnifiedReferralController) updateReferrerPoints(ctx context.Context, referrerEntity *ReferralEntity, refereeID primitive.ObjectID, pointsToAdd int) error {
	update := bson.M{
		"$inc":  bson.M{"points": pointsToAdd},
		"$push": bson.M{"referrals": refereeID},
		"$set":  bson.M{"updatedAt": time.Now()},
	}

	switch referrerEntity.Type {
	case "user":
		userCollection := rc.DB.Database("barrim").Collection("users")
		_, err := userCollection.UpdateByID(ctx, referrerEntity.ID, update)
		return err

	case "company":
		companyCollection := rc.DB.Database("barrim").Collection("companies")
		_, err := companyCollection.UpdateByID(ctx, referrerEntity.ID, update)
		return err

	case "wholesaler":
		wholesalerCollection := rc.DB.Database("barrim").Collection("wholesalers")
		_, err := wholesalerCollection.UpdateByID(ctx, referrerEntity.ID, update)
		return err

	case "serviceProvider":
		serviceProviderCollection := rc.DB.Database("barrim").Collection("serviceProviders")
		_, err := serviceProviderCollection.UpdateByID(ctx, referrerEntity.ID, update)
		return err

	default:
		return fmt.Errorf("unknown referrer type: %s", referrerEntity.Type)
	}
}

// GetReferralData fetches referral statistics for the current user
func (rc *UnifiedReferralController) GetReferralData(c echo.Context) error {
	log.Printf("=== GETTING UNIFIED REFERRAL DATA ===")

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

	// Get referral data based on user type
	return rc.getReferralDataByType(c, ctx, user, userObjID)
}

// getReferralDataByType fetches referral data based on user type
func (rc *UnifiedReferralController) getReferralDataByType(c echo.Context, ctx context.Context, user models.User, userObjID primitive.ObjectID) error {
	var referralCode string
	var points int
	var referralCount int

	switch user.UserType {
	case "user":
		referralCode = user.ReferralCode
		points = user.Points
		referralCount = len(user.Referrals)

	case "company":
		companyCollection := rc.DB.Database("barrim").Collection("companies")
		var company models.Company
		err := companyCollection.FindOne(ctx, bson.M{"userId": userObjID}).Decode(&company)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to retrieve company information",
			})
		}
		referralCode = company.ReferralCode
		points = company.Points
		referralCount = len(company.Referrals)

	case "wholesaler":
		wholesalerCollection := rc.DB.Database("barrim").Collection("wholesalers")
		var wholesaler models.Wholesaler
		err := wholesalerCollection.FindOne(ctx, bson.M{"userId": userObjID}).Decode(&wholesaler)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to retrieve wholesaler information",
			})
		}
		referralCode = wholesaler.ReferralCode
		points = wholesaler.Points
		referralCount = len(wholesaler.Referrals)

	case "serviceProvider":
		serviceProviderCollection := rc.DB.Database("barrim").Collection("serviceProviders")
		var serviceProvider models.ServiceProvider
		err := serviceProviderCollection.FindOne(ctx, bson.M{"userId": userObjID}).Decode(&serviceProvider)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to retrieve service provider information",
			})
		}
		referralCode = serviceProvider.ReferralCode
		points = serviceProvider.Points
		referralCount = len(serviceProvider.Referrals)

	default:
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user type",
		})
	}

	// Generate QR code for referral
	qrCode, err := rc.GenerateReferralQRCode(referralCode)
	if err != nil {
		log.Printf("WARNING: Failed to generate QR code: %v", err)
	}

	// Create response with referral data
	referralData := map[string]interface{}{
		"referralCode":  referralCode,
		"referralCount": referralCount,
		"points":        points,
		"referralLink":  fmt.Sprintf("https://barrim.com/referral?code=%s", referralCode),
		"qrCode":        qrCode,
		"userType":      user.UserType,
	}

	log.Printf("Returning referral data - Points: %d, ReferralCount: %d", points, referralCount)

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Referral data retrieved successfully",
		Data:    referralData,
	})
}

// GenerateReferralQRCode creates a QR code image for a referral code
func (rc *UnifiedReferralController) GenerateReferralQRCode(referralCode string) (string, error) {
	// Create the QR code content
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

// GetReferralQRCode endpoint to get QR code for a referral code
func (rc *UnifiedReferralController) GetReferralQRCode(c echo.Context) error {
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

	// Get user to determine type and referral code
	var user models.User
	err = userCollection.FindOne(ctx, bson.M{"_id": userObjID}).Decode(&user)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve user",
		})
	}

	var referralCode string

	switch user.UserType {
	case "user":
		referralCode = user.ReferralCode

	case "company":
		companyCollection := rc.DB.Database("barrim").Collection("companies")
		var company models.Company
		err = companyCollection.FindOne(ctx, bson.M{"userId": userObjID}).Decode(&company)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to retrieve company information",
			})
		}
		referralCode = company.ReferralCode

	case "wholesaler":
		wholesalerCollection := rc.DB.Database("barrim").Collection("wholesalers")
		var wholesaler models.Wholesaler
		err = wholesalerCollection.FindOne(ctx, bson.M{"userId": userObjID}).Decode(&wholesaler)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to retrieve wholesaler information",
			})
		}
		referralCode = wholesaler.ReferralCode

	case "serviceProvider":
		serviceProviderCollection := rc.DB.Database("barrim").Collection("serviceProviders")
		var serviceProvider models.ServiceProvider
		err = serviceProviderCollection.FindOne(ctx, bson.M{"userId": userObjID}).Decode(&serviceProvider)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to retrieve service provider information",
			})
		}
		referralCode = serviceProvider.ReferralCode

	default:
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user type",
		})
	}

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
