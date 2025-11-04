// controllers/company_referral_controller.go
package controllers

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image/png"
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

// ReferralController handles referral related operations
type CompanyReferralController struct {
	DB *mongo.Client
}

// NewReferralController creates a new referral controller
func NewCompanyReferralController(db *mongo.Client) *CompanyReferralController {
	return &CompanyReferralController{DB: db}
}

// HandleReferral processes a user's referral code
func (rc *CompanyReferralController) HandleReferral(c echo.Context) error {
	// Get the user ID from the token
	userID, err := middleware.ExtractUserID(c)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Authentication failed",
		})
	}

	// Parse the request body
	var req models.ReferralRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request format",
		})
	}

	// Validate the request
	if req.ReferralCode == "" {
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
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve user information",
		})
	}

	// Handle company referrals
	if user.UserType == "company" {
		return rc.handleCompanyReferral(c, ctx, userObjID, req.ReferralCode)
	}

	// Handle regular user referrals
	return rc.handleUserReferral(c, ctx, userObjID, req.ReferralCode)
}

// handleCompanyReferral processes referrals between companies
func (rc *CompanyReferralController) handleCompanyReferral(c echo.Context, ctx context.Context, companyUserID primitive.ObjectID, referralCode string) error {
	// Collections needed
	companyCollection := rc.DB.Database("barrim").Collection("companies")

	// Find the referrer company by referral code
	var referrerCompany models.Company
	err := companyCollection.FindOne(ctx, bson.M{"referralCode": referralCode}).Decode(&referrerCompany)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Invalid referral code",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to verify referral code",
		})
	}

	// Find the referred company
	var currentCompany models.Company
	err = companyCollection.FindOne(ctx, bson.M{"userId": companyUserID}).Decode(&currentCompany)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve company information",
		})
	}

	// Check if referral is for the same company
	if referrerCompany.ID == currentCompany.ID {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Cannot use your own referral code",
		})
	}

	// Check if the company has already been referred
	for _, refID := range referrerCompany.Referrals {
		if refID == currentCompany.ID {
			return c.JSON(http.StatusBadRequest, models.Response{
				Status:  http.StatusBadRequest,
				Message: "This referral code has already been used",
			})
		}
	}

	// Update the referrer company - add points and add to referrals list
	const pointsToAdd = 5
	update := bson.M{
		"$inc":  bson.M{"points": pointsToAdd},
		"$push": bson.M{"referrals": currentCompany.ID},
		"$set":  bson.M{"updatedAt": time.Now()},
	}

	_, err = companyCollection.UpdateByID(ctx, referrerCompany.ID, update)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update referrer company",
		})
	}

	// Prepare the response
	response := models.CompanyReferralResponse{
		ReferrerID:      referrerCompany.ID,
		Referrer:        referrerCompany,
		NewCompany:      currentCompany,
		PointsAdded:     pointsToAdd,
		NewReferralCode: currentCompany.ReferralCode,
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Referral successfully applied",
		Data:    response,
	})
}

// handleUserReferral processes referrals for regular users
func (rc *CompanyReferralController) handleUserReferral(c echo.Context, ctx context.Context, userID primitive.ObjectID, referralCode string) error {
	// Collections needed
	userCollection := rc.DB.Database("barrim").Collection("users")

	// Find the referrer by referral code
	var referrer models.User
	err := userCollection.FindOne(ctx, bson.M{"referralCode": referralCode}).Decode(&referrer)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Invalid referral code",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to verify referral code",
		})
	}

	// Get the current user
	var currentUser models.User
	err = userCollection.FindOne(ctx, bson.M{"_id": userID}).Decode(&currentUser)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve user information",
		})
	}

	// Check if referral is for the same user
	if referrer.ID == currentUser.ID {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Cannot use your own referral code",
		})
	}

	// Check if the user has already been referred
	for _, refID := range referrer.Referrals {
		if refID == currentUser.ID {
			return c.JSON(http.StatusBadRequest, models.Response{
				Status:  http.StatusBadRequest,
				Message: "This referral code has already been used",
			})
		}
	}

	// Update the referrer - add points and add to referrals list
	const pointsToAdd = 5
	update := bson.M{
		"$inc":  bson.M{"points": pointsToAdd},
		"$push": bson.M{"referrals": currentUser.ID},
		"$set":  bson.M{"updatedAt": time.Now()},
	}

	_, err = userCollection.UpdateByID(ctx, referrer.ID, update)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update referrer",
		})
	}

	// Prepare the response
	response := models.ReferralResponse{
		ReferrerID:      referrer.ID,
		Referrer:        referrer,
		NewUser:         currentUser,
		PointsAdded:     pointsToAdd,
		NewReferralCode: currentUser.ReferralCode,
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Referral successfully applied",
		Data:    response,
	})
}

// GetReferralData fetches referral statistics for the current user
func (rc *CompanyReferralController) GetReferralData(c echo.Context) error {
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

	// Get the user type
	ctx := context.Background()
	userCollection := rc.DB.Database("barrim").Collection("users")

	var user models.User
	err = userCollection.FindOne(ctx, bson.M{"_id": userObjID}).Decode(&user)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve user",
		})
	}

	// Handle company referral data
	if user.UserType == "company" {
		return rc.getCompanyReferralData(c, ctx, userObjID)
	}

	// Handle regular user referral data
	return rc.getUserReferralData(c, ctx, user)
}

// getCompanyReferralData fetches referral statistics for companies
func (rc *CompanyReferralController) getCompanyReferralData(c echo.Context, ctx context.Context, userID primitive.ObjectID) error {
	companyCollection := rc.DB.Database("barrim").Collection("companies")

	var company models.Company
	err := companyCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&company)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve company information",
		})
	}

	// Generate QR code for company referral
	qrCode, err := rc.GenerateReferralQRCode(company.ReferralCode)
	if err != nil {
		// Log the error but continue, just won't have QR in response
		fmt.Printf("Failed to generate QR code: %v\n", err)
	}

	// Create response with referral data
	referralData := models.CompanyReferralData{
		ReferralCode:  company.ReferralCode,
		ReferralCount: len(company.Referrals),
		Points:        company.Points,
		ReferralLink:  fmt.Sprintf("https://barrim.com/referral?code=%s", company.ReferralCode),
	}

	responseData := map[string]interface{}{
		"referralData": referralData,
		"qrCode":       qrCode,
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Referral data retrieved successfully",
		Data:    responseData,
	})
}

// getUserReferralData fetches referral statistics for regular users
func (rc *CompanyReferralController) getUserReferralData(c echo.Context, ctx context.Context, user models.User) error {
	// Generate QR code for user referral
	qrCode, err := rc.GenerateReferralQRCode(user.ReferralCode)
	if err != nil {
		// Log the error but continue
		fmt.Printf("Failed to generate QR code: %v\n", err)
	}

	// Create response with referral data
	referralData := map[string]interface{}{
		"referralCode":  user.ReferralCode,
		"referralCount": len(user.Referrals),
		"points":        user.Points,
		"referralLink":  fmt.Sprintf("https://barrim.com/referral?code=%s", user.ReferralCode),
		"qrCode":        qrCode,
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Referral data retrieved successfully",
		Data:    referralData,
	})
}

// GenerateReferralQRCode creates a QR code image for a referral code
func (rc *CompanyReferralController) GenerateReferralQRCode(referralCode string) (string, error) {
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

// GetReferralQRCode endpoint to get QR code for a referral code
func (rc *CompanyReferralController) GetCompanyReferralQRCode(c echo.Context) error {
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

	if user.UserType == "company" {
		// Get company referral code
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
	} else {
		// Use user referral code
		referralCode = user.ReferralCode
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
