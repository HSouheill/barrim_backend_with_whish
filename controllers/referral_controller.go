package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/HSouheill/barrim_backend/models"
)

type ReferralController struct {
	db *mongo.Client
}

func NewReferralController(db *mongo.Client) *ReferralController {
	rand.Seed(time.Now().UnixNano())
	return &ReferralController{db: db}
}

// GenerateUniqueReferralCode generates a unique referral code
func generateUniqueReferralCode() string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	const length = 8
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

// HandleReferral handles new user registration with referral code
func (rc *ReferralController) HandleReferral(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get the new user ID from JWT or request
	userID, ok := c.Get("userID").(string)
	if !ok {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID in token",
		})
	}

	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID format",
		})
	}

	var req models.ReferralRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
			Data:    err.Error(),
		})
	}

	usersCollection := rc.db.Database("barrim").Collection("users")

	// If no referral code provided, generate one for the first user
	if req.ReferralCode == "" {
		// Check if this is the first user
		count, err := usersCollection.CountDocuments(ctx, bson.M{})
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to check user count",
			})
		}

		if count == 0 {
			// This is the first user, generate a unique referral code
			referralCode := generateUniqueReferralCode()
			_, err = usersCollection.UpdateByID(ctx, objID, bson.M{
				"$set": bson.M{
					"referralCode": referralCode,
					"points":       0,
				},
			})
			if err != nil {
				return c.JSON(http.StatusInternalServerError, models.Response{
					Status:  http.StatusInternalServerError,
					Message: "Failed to set referral code",
				})
			}

			return c.JSON(http.StatusOK, models.Response{
				Status:  http.StatusOK,
				Message: "First user referral code generated successfully",
				Data: bson.M{
					"referralCode": referralCode,
					"points":       0,
				},
			})
		}

		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Referral code is required for non-first users",
		})
	}

	// Find the referrer by referral code
	var referrer models.User
	err = usersCollection.FindOne(ctx, bson.M{"referralCode": req.ReferralCode}).Decode(&referrer)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Referral code not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Database error",
			Data:    err.Error(),
		})
	}

	// Prevent self-referral
	if referrer.ID == objID {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Cannot use your own referral code",
		})
	}

	// Generate a new referral code for the new user
	newReferralCode := generateUniqueReferralCode()

	// Update the referrer's points and referrals
	update := bson.M{
		"$inc":  bson.M{"points": 5}, // Add  points
		"$push": bson.M{"referrals": objID},
	}

	_, err = usersCollection.UpdateByID(ctx, referrer.ID, update)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update referrer",
			Data:    err.Error(),
		})
	}

	// Set the new user's referral code
	_, err = usersCollection.UpdateByID(ctx, objID, bson.M{
		"$set": bson.M{
			"referralCode": newReferralCode,
			"points":       0,
		},
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to set new user's referral code",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Referral processed successfully",
		Data: models.ReferralResponse{
			ReferrerID:      referrer.ID,
			PointsAdded:     10,
			NewReferralCode: newReferralCode,
		},
	})
}

// GetReferralData returns user's referral information
func (rc *ReferralController) GetReferralData(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user ID from context
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

	usersCollection := rc.db.Database("barrim").Collection("users")
	var user models.User
	err = usersCollection.FindOne(ctx, bson.M{"_id": objID}).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "User not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Database error",
			Data:    err.Error(),
		})
	}

	// Ensure referral code exists, generate if not
	if user.ReferralCode == "" {
		user.ReferralCode = generateUniqueReferralCode()
		_, err = usersCollection.UpdateByID(ctx, objID, bson.M{"$set": bson.M{"referralCode": user.ReferralCode}})
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to generate referral code",
			})
		}
	}

	referralCount := len(user.Referrals)
	if user.Points < 0 {
		user.Points = 0 // Ensure points don't go negative
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Referral data fetched successfully",
		Data: bson.M{
			"referralCode":  user.ReferralCode,
			"referralCount": referralCount,
			"points":        user.Points,
			"referralLink":  "https://barrim.com/register?ref=" + user.ReferralCode,
		},
	})
}

// HandleCompanyReferral handles new company registration with referral code
// HandleCompanyReferral handles new company registration with referral code
func (rc *ReferralController) HandleCompanyReferral(c echo.Context) error {
	log.Printf("JWT User context: userID=%v, userId=%v",
		c.Get("userID"), c.Get("userId"))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get the new company ID from JWT or request
	userID, ok := c.Get("userID").(string)
	if !ok {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID in token",
		})
	}

	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID format",
		})
	}

	var req models.CompanyReferralRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
			Data:    err.Error(),
		})
	}

	companiesCollection := rc.db.Database("barrim").Collection("companies")
	usersCollection := rc.db.Database("barrim").Collection("users")

	// Find the company by userID
	var company models.Company
	err = companiesCollection.FindOne(ctx, bson.M{"userId": objID}).Decode(&company)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Company not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Database error",
			Data:    err.Error(),
		})
	}

	// If no referral code provided, generate one for the first company
	if req.ReferralCode == "" {
		// Check if this is the first company
		count, err := companiesCollection.CountDocuments(ctx, bson.M{})
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to check company count",
			})
		}

		if count <= 1 { // This company is the first/only one
			// Generate a unique referral code
			referralCode := generateUniqueReferralCode()
			_, err = companiesCollection.UpdateByID(ctx, company.ID, bson.M{
				"$set": bson.M{
					"referralCode": referralCode,
					"points":       0,
					"referrals":    []primitive.ObjectID{},
				},
			})
			if err != nil {
				return c.JSON(http.StatusInternalServerError, models.Response{
					Status:  http.StatusInternalServerError,
					Message: "Failed to set referral code",
				})
			}

			return c.JSON(http.StatusOK, models.Response{
				Status:  http.StatusOK,
				Message: "First company referral code generated successfully",
				Data: bson.M{
					"referralCode": referralCode,
					"points":       0,
				},
			})
		}

		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Referral code is required for non-first companies",
		})
	}

	// Try to find the referrer company by referral code
	var referrerCompany models.Company
	err = companiesCollection.FindOne(ctx, bson.M{"referralCode": req.ReferralCode}).Decode(&referrerCompany)

	// If not found as company, try to find in users (for cross-type referrals)
	var referrerUser models.User
	var isCompanyReferrer = true

	if err == mongo.ErrNoDocuments {
		err = usersCollection.FindOne(ctx, bson.M{"referralCode": req.ReferralCode}).Decode(&referrerUser)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				return c.JSON(http.StatusNotFound, models.Response{
					Status:  http.StatusNotFound,
					Message: "Referral code not found",
				})
			}
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Database error",
				Data:    err.Error(),
			})
		}
		isCompanyReferrer = false
	} else if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Database error",
			Data:    err.Error(),
		})
	}

	// Prevent self-referral
	if (isCompanyReferrer && referrerCompany.ID == company.ID) ||
		(!isCompanyReferrer && referrerUser.ID == objID) {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Cannot use your own referral code",
		})
	}

	// Generate a new referral code for the new company
	newReferralCode := generateUniqueReferralCode()

	// All referrals award 5 points regardless of referrer type
	pointsToAdd := 5

	if isCompanyReferrer {
		// Update referring company with 5 points
		result, err := companiesCollection.UpdateByID(ctx, referrerCompany.ID, bson.M{
			"$inc":  bson.M{"points": pointsToAdd},
			"$push": bson.M{"referrals": company.ID},
		})
		log.Printf("Updated company referrer: %v, error: %v", result, err)
	} else {
		// Update referring user with 5 points
		result, err := usersCollection.UpdateByID(ctx, referrerUser.ID, bson.M{
			"$inc":  bson.M{"points": pointsToAdd},
			"$push": bson.M{"referrals": company.ID},
		})
		log.Printf("Updated user referrer: %v, error: %v", result, err)
	}

	// Update the referrer's points and referrals
	if isCompanyReferrer {
		// Update referring company with 5 points
		_, err = companiesCollection.UpdateByID(ctx, referrerCompany.ID, bson.M{
			"$inc":  bson.M{"points": pointsToAdd},
			"$push": bson.M{"referrals": company.ID},
		})
	} else {
		// Update referring user with 5 points
		_, err = usersCollection.UpdateByID(ctx, referrerUser.ID, bson.M{
			"$inc":  bson.M{"points": pointsToAdd},
			"$push": bson.M{"referrals": company.ID},
		})
	}

	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update referrer",
			Data:    err.Error(),
		})
	}

	// Set the new company's referral code
	_, err = companiesCollection.UpdateByID(ctx, company.ID, bson.M{
		"$set": bson.M{
			"referralCode": newReferralCode,
			"points":       0,
			"referrals":    []primitive.ObjectID{},
		},
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to set new company's referral code",
		})
	}

	referrerID := referrerUser.ID
	if isCompanyReferrer {
		referrerID = referrerCompany.ID
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Company referral processed successfully",
		Data: bson.M{
			"referrerID":      referrerID,
			"pointsAdded":     pointsToAdd,
			"newReferralCode": newReferralCode,
		},
	})
}

// GetCompanyReferralData returns company's referral information
func (rc *ReferralController) GetCompanyReferralData(c echo.Context) error {

	log.Printf("JWT User context: userID=%v, userId=%v",
		c.Get("userID"), c.Get("userId"))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user ID from context (company user)
	userID, ok := c.Get("userID").(string)
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

	companiesCollection := rc.db.Database("barrim").Collection("companies")
	var company models.Company

	// Find company by the userId field
	err = companiesCollection.FindOne(ctx, bson.M{"userId": objID}).Decode(&company)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Company not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Database error",
			Data:    err.Error(),
		})
	}

	// Ensure referral code exists, generate if not
	if company.ReferralCode == "" {
		company.ReferralCode = generateUniqueReferralCode()
		_, err = companiesCollection.UpdateByID(ctx, company.ID, bson.M{
			"$set": bson.M{
				"referralCode": company.ReferralCode,
				"referrals":    []primitive.ObjectID{},
			},
		})
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to generate referral code",
			})
		}
	}

	referralCount := 0
	if company.Referrals != nil {
		referralCount = len(company.Referrals)
	}

	if company.Points < 0 {
		company.Points = 0 // Ensure points don't go negative
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Company referral data fetched successfully",
		Data: models.CompanyReferralData{
			ReferralCode:  company.ReferralCode,
			ReferralCount: referralCount,
			Points:        company.Points,
			ReferralLink:  "https://barrim.com/company/register?ref=" + company.ReferralCode,
		},
	})
}

func generateReferralCodesForExistingCompanies(db *mongo.Client) {
	ctx := context.Background()
	companiesCollection := db.Database("barrim").Collection("companies")

	cursor, err := companiesCollection.Find(ctx, bson.M{"referralCode": bson.M{"$exists": false}})
	if err != nil {
		log.Fatalf("Error finding companies: %v", err)
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var company models.Company
		if err := cursor.Decode(&company); err != nil {
			log.Printf("Error decoding company: %v", err)
			continue
		}

		referralCode := generateUniqueReferralCode() // Your existing function

		_, err := companiesCollection.UpdateByID(ctx, company.ID, bson.M{
			"$set": bson.M{
				"referralCode": referralCode,
				"referrals":    []primitive.ObjectID{},
			},
		})

		if err != nil {
			log.Printf("Error updating company %s: %v", company.ID, err)
		} else {
			log.Printf("Generated referral code %s for company %s", referralCode, company.ID)
		}
	}
}

// Create a function to automatically generate a referral code after company creation
func generateReferralCodeForNewCompany(userID primitive.ObjectID, token string, baseURL string) error {
	// Create HTTP client
	client := &http.Client{}

	// Create empty request body (for first company scenario)
	reqBody, err := json.Marshal(map[string]string{})
	if err != nil {
		return err
	}

	// Create the request
	req, err := http.NewRequest("POST", baseURL+"/api/company/handle-referral", bytes.NewBuffer(reqBody))
	if err != nil {
		return err
	}

	// Add headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}
