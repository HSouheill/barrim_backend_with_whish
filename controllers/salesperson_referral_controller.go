package controllers

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/HSouheill/barrim_backend/models"
	"github.com/HSouheill/barrim_backend/utils"
)

type SalespersonReferralController struct {
	DB *mongo.Database
}

func NewSalespersonReferralController(db *mongo.Database) *SalespersonReferralController {
	return &SalespersonReferralController{DB: db}
}

// HandleReferral handles when a user uses a salesperson's referral code
func (src *SalespersonReferralController) HandleReferral(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var req struct {
		ReferralCode string `json:"referralCode"`
	}

	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}

	if req.ReferralCode == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Referral code is required",
		})
	}

	// Get user ID from context (assuming user is authenticated)
	userID, ok := c.Get("userId").(string)
	if !ok {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "User ID not found in context",
		})
	}

	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID format",
		})
	}

	// Collections
	usersCollection := src.DB.Collection("users")
	salespersonsCollection := src.DB.Collection("salespersons")

	// Find the salesperson by referral code
	var salesperson models.Salesperson
	err = salespersonsCollection.FindOne(ctx, bson.M{"referralCode": req.ReferralCode}).Decode(&salesperson)
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
	var user models.User
	err = usersCollection.FindOne(ctx, bson.M{"_id": objID}).Decode(&user)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve user information",
		})
	}

	// Check if user has already used a referral code
	if user.ReferralCode != "" {
		return c.JSON(http.StatusConflict, models.Response{
			Status:  http.StatusConflict,
			Message: "You have already used a referral code",
		})
	}

	// Check if user is trying to use their own referral code (if they're a salesperson)
	if user.UserType == "salesperson" && user.ID == salesperson.ID {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Cannot use your own referral code",
		})
	}

	// Generate a new referral code for the user
	referralCode, err := utils.GenerateSalespersonReferralCode()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to generate referral code",
		})
	}

	// Update the salesperson with $1 referral reward and add user to referrals
	referralReward := 1.0 // $1 reward
	update := bson.M{
		"$inc": bson.M{
			"referralBalance": referralReward,
		},
		"$push": bson.M{
			"referrals": user.ID,
		},
		"$set": bson.M{
			"updatedAt": time.Now(),
		},
	}

	_, err = salespersonsCollection.UpdateByID(ctx, salesperson.ID, update)
	if err != nil {
		log.Printf("Failed to update salesperson referral balance: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update salesperson referral balance",
		})
	}

	// Update the user with their new referral code
	_, err = usersCollection.UpdateByID(ctx, user.ID, bson.M{
		"$set": bson.M{
			"referralCode": referralCode,
			"updatedAt":    time.Now(),
		},
	})
	if err != nil {
		log.Printf("Failed to update user referral code: %v", err)
		// Don't fail the entire operation, just log the error
	}

	// Create a referral commission record
	referralCommission := models.ReferralCommission{
		ID:            primitive.NewObjectID(),
		SalespersonID: salesperson.ID,
		UserID:        user.ID,
		Amount:        referralReward,
		ReferralCode:  req.ReferralCode,
		Status:        "earned",
		CreatedAt:     time.Now(),
	}

	_, err = src.DB.Collection("referral_commissions").InsertOne(ctx, referralCommission)
	if err != nil {
		log.Printf("Failed to create referral commission record: %v", err)
		// Don't fail the entire operation, just log the error
	}

	// Fetch updated salesperson data
	var updatedSalesperson models.Salesperson
	err = salespersonsCollection.FindOne(ctx, bson.M{"_id": salesperson.ID}).Decode(&updatedSalesperson)
	if err != nil {
		log.Printf("Failed to fetch updated salesperson data: %v", err)
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Referral processed successfully",
		Data: map[string]interface{}{
			"referralReward": referralReward,
			"salesperson": map[string]interface{}{
				"id":              salesperson.ID.Hex(),
				"fullName":        salesperson.FullName,
				"referralBalance": updatedSalesperson.ReferralBalance,
			},
			"userReferralCode": referralCode,
		},
	})
}

// GetSalespersonReferralData returns referral information for a salesperson
func (src *SalespersonReferralController) GetSalespersonReferralData(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get salesperson ID from context
	salespersonID, ok := c.Get("userId").(string)
	if !ok {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "User ID not found in context",
		})
	}

	objID, err := primitive.ObjectIDFromHex(salespersonID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid salesperson ID format",
		})
	}

	salespersonsCollection := src.DB.Collection("salespersons")
	var salesperson models.Salesperson
	err = salespersonsCollection.FindOne(ctx, bson.M{"_id": objID}).Decode(&salesperson)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Salesperson not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Database error",
			Data:    err.Error(),
		})
	}

	// Ensure referral code exists, generate if not
	if salesperson.ReferralCode == "" {
		referralCode, err := utils.GenerateSalespersonReferralCode()
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to generate referral code",
			})
		}

		_, err = salespersonsCollection.UpdateByID(ctx, objID, bson.M{
			"$set": bson.M{
				"referralCode": referralCode,
				"updatedAt":    time.Now(),
			},
		})
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to update referral code",
			})
		}
		salesperson.ReferralCode = referralCode
	}

	referralCount := len(salesperson.Referrals)
	if salesperson.ReferralBalance < 0 {
		salesperson.ReferralBalance = 0 // Ensure balance doesn't go negative
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Referral data fetched successfully",
		Data: map[string]interface{}{
			"referralCode":    salesperson.ReferralCode,
			"referralCount":   referralCount,
			"referralBalance": salesperson.ReferralBalance,
			"referralLink":    "https://barrim.com/register?ref=" + salesperson.ReferralCode,
		},
	})
}

// GetReferralCommissions returns referral commission history for a salesperson
func (src *SalespersonReferralController) GetReferralCommissions(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get salesperson ID from context
	salespersonID, ok := c.Get("userId").(string)
	if !ok {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "User ID not found in context",
		})
	}

	objID, err := primitive.ObjectIDFromHex(salespersonID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid salesperson ID format",
		})
	}

	// Get pagination parameters
	page := 1
	limit := 20
	if pageStr := c.QueryParam("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if limitStr := c.QueryParam("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	skip := (page - 1) * limit

	// Find referral commissions for this salesperson
	cursor, err := src.DB.Collection("referral_commissions").Find(
		ctx,
		bson.M{"salespersonID": objID},
		options.Find().SetSort(bson.M{"createdAt": -1}).SetSkip(int64(skip)).SetLimit(int64(limit)),
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch referral commissions",
		})
	}
	defer cursor.Close(ctx)

	var commissions []models.ReferralCommission
	if err = cursor.All(ctx, &commissions); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode referral commissions",
		})
	}

	// Get total count
	totalCount, err := src.DB.Collection("referral_commissions").CountDocuments(ctx, bson.M{"salespersonID": objID})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to count referral commissions",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Referral commissions fetched successfully",
		Data: map[string]interface{}{
			"commissions": commissions,
			"pagination": map[string]interface{}{
				"page":       page,
				"limit":      limit,
				"total":      totalCount,
				"totalPages": (totalCount + int64(limit) - 1) / int64(limit),
			},
		},
	})
}
