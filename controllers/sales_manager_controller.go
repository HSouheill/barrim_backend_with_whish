package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/crypto/bcrypt"

	"github.com/HSouheill/barrim_backend/middleware"
	"github.com/HSouheill/barrim_backend/models"
	"github.com/HSouheill/barrim_backend/utils"
)

type SalesManagerController struct {
	db *mongo.Database
}

func NewSalesManagerController(db *mongo.Database) *SalesManagerController {
	return &SalesManagerController{db: db}
}

// CreateSalesperson creates a new salesperson
func (smc *SalesManagerController) CreateSalesperson(c echo.Context) error {
	var req struct {
		FullName          string  `json:"fullName"`
		Email             string  `json:"email"`
		Password          string  `json:"password"`
		PhoneNumber       string  `json:"phoneNumber"`
		CommissionPercent float64 `json:"commissionPercent"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}
	if req.CommissionPercent < 0 || req.CommissionPercent > 100 {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Commission percent must be between 0 and 100",
		})
	}

	// Check if email already exists
	var existingSalesperson models.Salesperson
	err := smc.db.Collection("salespersons").FindOne(
		context.Background(),
		bson.M{"email": req.Email},
	).Decode(&existingSalesperson)

	if err == nil {
		return c.JSON(http.StatusConflict, models.Response{
			Status:  http.StatusConflict,
			Message: "Email already exists",
		})
	} else if err != mongo.ErrNoDocuments {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Error checking email existence",
		})
	}

	// Get sales manager ID from JWT token
	userID := c.Get("userId")
	if userID == nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "User ID not found in token",
		})
	}

	salesManagerID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Invalid sales manager ID",
		})
	}

	// Hash the password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to hash password",
		})
	}

	// Generate referral code for salesperson
	referralCode, err := utils.GenerateSalespersonReferralCode()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to generate referral code",
		})
	}

	salesperson := models.Salesperson{
		FullName:          req.FullName,
		Email:             req.Email,
		Password:          string(hashedPassword),
		PhoneNumber:       req.PhoneNumber,
		CommissionPercent: req.CommissionPercent,
		ReferralCode:      referralCode,
		SalesManagerID:    salesManagerID,
		CreatedBy:         salesManagerID,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	result, err := smc.db.Collection("salespersons").InsertOne(context.Background(), salesperson)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to create salesperson",
		})
	}

	salesperson.ID = result.InsertedID.(primitive.ObjectID)
	salesperson.Password = "" // Remove password from response

	// Create user entry in users collection
	user := models.User{
		ID:           salesperson.ID,
		FullName:     salesperson.FullName,
		Email:        salesperson.Email,
		Password:     string(hashedPassword),
		UserType:     "salesperson",
		Phone:        salesperson.PhoneNumber,
		ReferralCode: referralCode,
		IsActive:     true,
		Status:       "active",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	_, err = smc.db.Collection("users").InsertOne(context.Background(), user)
	if err != nil {
		log.Printf("Failed to create user entry for salesperson: %v", err)
		// Don't fail the creation, just log the error
	}

	return c.JSON(http.StatusCreated, models.Response{
		Status:  http.StatusCreated,
		Message: "Salesperson created successfully",
		Data:    salesperson,
	})
}

// GetAllSalespersons retrieves all salespersons for the current sales manager
func (smc *SalesManagerController) GetAllSalespersons(c echo.Context) error {
	userID := c.Get("userId")
	if userID == nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "User ID not found in token",
		})
	}

	salesManagerID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Invalid sales manager ID",
		})
	}

	var salespersons []models.Salesperson
	cursor, err := smc.db.Collection("salespersons").Find(
		context.Background(),
		bson.M{"salesManagerId": salesManagerID},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch salespersons",
		})
	}
	defer cursor.Close(context.Background())

	if err := cursor.All(context.Background(), &salespersons); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode salespersons",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Salespersons retrieved successfully",
		Data:    salespersons,
	})
}

// GetSalesperson retrieves a specific salesperson by ID
func (smc *SalesManagerController) GetSalesperson(c echo.Context) error {
	salesManagerID, err := primitive.ObjectIDFromHex(c.Get("user_id").(string))
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Invalid sales manager ID",
		})
	}

	salespersonID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid salesperson ID",
		})
	}

	var salesperson models.Salesperson
	err = smc.db.Collection("salespersons").FindOne(
		context.Background(),
		bson.M{
			"_id":            salespersonID,
			"salesManagerId": salesManagerID,
		},
	).Decode(&salesperson)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Salesperson not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch salesperson",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Salesperson retrieved successfully",
		Data:    salesperson,
	})
}

// UpdateSalesperson updates a specific salesperson
func (smc *SalesManagerController) UpdateSalesperson(c echo.Context) error {
	// Get sales manager ID from JWT token
	userID := c.Get("userId")
	if userID == nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "User ID not found in token",
		})
	}

	salesManagerID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Invalid sales manager ID",
		})
	}

	salespersonID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid salesperson ID",
		})
	}

	// First verify that the salesperson belongs to this sales manager
	var existingSalesperson models.Salesperson
	err = smc.db.Collection("salespersons").FindOne(
		context.Background(),
		bson.M{
			"_id":            salespersonID,
			"salesManagerId": salesManagerID,
		},
	).Decode(&existingSalesperson)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Salesperson not found or you don't have permission to update this salesperson",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to verify salesperson",
		})
	}

	// Parse request body as raw JSON first for debugging
	body, err := ioutil.ReadAll(c.Request().Body)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Failed to read request body",
		})
	}

	// Log the raw request body for debugging
	fmt.Printf("Raw request body: %s\n", string(body))

	// Reset the request body for binding
	c.Request().Body = ioutil.NopCloser(bytes.NewBuffer(body))

	// Create a flexible update structure
	var updateRequest map[string]interface{}
	if err := json.Unmarshal(body, &updateRequest); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: fmt.Sprintf("Invalid JSON format: %v", err),
		})
	}

	// Log parsed request for debugging
	fmt.Printf("Parsed request: %+v\n", updateRequest)

	// If email is being updated, check for uniqueness
	if email, exists := updateRequest["email"].(string); exists && email != "" && email != existingSalesperson.Email {
		var emailCheck models.Salesperson
		err := smc.db.Collection("salespersons").FindOne(
			context.Background(),
			bson.M{
				"email": email,
				"_id":   bson.M{"$ne": salespersonID},
			},
		).Decode(&emailCheck)

		if err == nil {
			return c.JSON(http.StatusConflict, models.Response{
				Status:  http.StatusConflict,
				Message: "Email already exists",
			})
		} else if err != mongo.ErrNoDocuments {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Error checking email existence",
			})
		}
	}

	// Prepare update data
	updateData := bson.M{
		"updatedAt": time.Now(),
	}

	// Map the fields from the request to update data
	fieldMappings := map[string]string{
		"fullName":          "fullName",
		"email":             "email",
		"phoneNumber":       "phoneNumber",
		"status":            "status",
		"Image":             "Image",             // Note: capital I to match your Go struct
		"image":             "Image",             // Also handle lowercase for flexibility
		"commissionPercent": "commissionPercent", // Added commissionPercent
	}

	for requestField, dbField := range fieldMappings {
		if value, exists := updateRequest[requestField]; exists {
			if requestField == "commissionPercent" {
				// Handle as float64
				if floatValue, ok := value.(float64); ok {
					updateData[dbField] = floatValue
				}
			} else if strValue, ok := value.(string); ok && strValue != "" {
				updateData[dbField] = strValue
			}
		}
	}

	// Handle password separately (hash it if provided)
	if password, exists := updateRequest["password"].(string); exists && password != "" {
		// Hash the password before storing (you should implement proper password hashing)
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to hash password",
			})
		}
		updateData["password"] = string(hashedPassword)
	}

	// Log what we're about to update
	fmt.Printf("Update data: %+v\n", updateData)

	// Update the salesperson
	result, err := smc.db.Collection("salespersons").UpdateOne(
		context.Background(),
		bson.M{
			"_id":            salespersonID,
			"salesManagerId": salesManagerID,
		},
		bson.M{"$set": updateData},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: fmt.Sprintf("Failed to update salesperson: %v", err),
		})
	}

	if result.MatchedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Salesperson not found",
		})
	}

	// Get updated salesperson
	var updatedSalesperson models.Salesperson
	err = smc.db.Collection("salespersons").FindOne(
		context.Background(),
		bson.M{"_id": salespersonID},
	).Decode(&updatedSalesperson)

	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch updated salesperson",
		})
	}

	// Remove password from response
	updatedSalesperson.Password = ""

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Salesperson updated successfully",
		Data:    updatedSalesperson,
	})
}

// DeleteSalesperson deletes a specific salesperson
func (smc *SalesManagerController) DeleteSalesperson(c echo.Context) error {
	// Get sales manager ID from JWT token
	userID := c.Get("userId")
	if userID == nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "User ID not found in token",
		})
	}

	salesManagerID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Invalid sales manager ID",
		})
	}

	salespersonID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid salesperson ID",
		})
	}

	// First verify that the salesperson belongs to this sales manager
	var salesperson models.Salesperson
	err = smc.db.Collection("salespersons").FindOne(
		context.Background(),
		bson.M{
			"_id":            salespersonID,
			"salesManagerId": salesManagerID,
		},
	).Decode(&salesperson)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Salesperson not found or you don't have permission to delete this salesperson",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to verify salesperson",
		})
	}

	// Delete the salesperson
	result, err := smc.db.Collection("salespersons").DeleteOne(
		context.Background(),
		bson.M{
			"_id":            salespersonID,
			"salesManagerId": salesManagerID,
		},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to delete salesperson",
		})
	}

	if result.DeletedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Salesperson not found",
		})
	}

	// Delete associated user entry from users collection
	_, err = smc.db.Collection("users").DeleteOne(context.Background(), bson.M{
		"$or": []bson.M{
			{"email": salesperson.Email},
			{"userType": "salesperson", "email": salesperson.Email},
		},
	})
	if err != nil {
		log.Printf("Failed to delete associated user account for salesperson %s: %v", salesperson.Email, err)
	} else {
		log.Printf("Successfully deleted associated user account for salesperson %s", salesperson.Email)
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Salesperson deleted successfully",
	})
}

// Login handles sales manager authentication
func (smc *SalesManagerController) Login(c echo.Context) error {
	var loginReq struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	if err := c.Bind(&loginReq); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}

	// Find sales manager by email
	var salesManager models.SalesManager
	err := smc.db.Collection("sales_managers").FindOne(
		context.Background(),
		bson.M{"email": loginReq.Email},
	).Decode(&salesManager)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusUnauthorized, models.Response{
				Status:  http.StatusUnauthorized,
				Message: "Invalid email or password",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find sales manager",
		})
	}

	// Check password
	err = utils.CheckPassword(loginReq.Password, salesManager.Password)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Invalid email or password",
		})
	}

	// Generate JWT token
	token, refreshToken, err := middleware.GenerateJWT(salesManager.ID.Hex(), salesManager.Email, "sales_manager")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to generate token",
		})
	}

	// Update last login time
	_, err = smc.db.Collection("sales_managers").UpdateOne(
		context.Background(),
		bson.M{"_id": salesManager.ID},
		bson.M{"$set": bson.M{"updatedAt": time.Now()}},
	)
	if err != nil {
		// Log the error but don't fail the login
		log.Printf("Failed to update last login time: %v", err)
	}

	// Remove password from response
	salesManager.Password = ""

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Login successful",
		Data: map[string]interface{}{
			"token":        token,
			"refreshToken": refreshToken,
			"user":         salesManager,
		},
	})
}

// ForgotPassword initiates the password reset process
func (smc *SalesManagerController) ForgotPassword(c echo.Context) error {
	var req struct {
		Phone string `json:"phone"`
	}

	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}

	// Find sales manager by phone number
	var salesManager models.SalesManager
	err := smc.db.Collection("sales_managers").FindOne(
		context.Background(),
		bson.M{"phoneNumber": req.Phone},
	).Decode(&salesManager)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "No account found with this phone number",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find sales manager",
		})
	}

	// Generate OTP
	otp := generateAuthOTP()
	expiresAt := time.Now().Add(10 * time.Minute)

	// Store OTP in database
	otpCollection := smc.db.Collection("password_reset_otps")
	otpDoc := bson.M{
		"phone":     req.Phone,
		"otp":       otp,
		"expiresAt": expiresAt,
		"verified":  false,
		"createdAt": time.Now(),
	}

	// Delete any existing OTPs for this phone number
	_, err = otpCollection.DeleteMany(context.Background(), bson.M{"phone": req.Phone})
	if err != nil {
		log.Printf("Failed to delete existing OTPs: %v", err)
	}

	// Insert new OTP
	_, err = otpCollection.InsertOne(context.Background(), otpDoc)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to store OTP",
		})
	}

	// Send OTP via SMS
	// Note: You'll need to implement the actual SMS sending logic
	// This is a placeholder for the SMS sending functionality
	err = sendOTP(req.Phone, otp)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to send OTP",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "OTP sent successfully",
		Data: map[string]interface{}{
			"phone":     req.Phone,
			"expiresAt": expiresAt,
		},
	})
}

// ResetPassword verifies OTP and resets the password
func (smc *SalesManagerController) ResetPassword(c echo.Context) error {
	var req struct {
		Phone       string `json:"phone"`
		OTP         string `json:"otp"`
		NewPassword string `json:"newPassword"`
	}

	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}

	// Verify OTP
	otpCollection := smc.db.Collection("password_reset_otps")
	var otpDoc bson.M
	err := otpCollection.FindOne(
		context.Background(),
		bson.M{
			"phone": req.Phone,
			"otp":   req.OTP,
		},
	).Decode(&otpDoc)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusBadRequest, models.Response{
				Status:  http.StatusBadRequest,
				Message: "Invalid OTP",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to verify OTP",
		})
	}

	// Check if OTP is expired
	expiresAt := otpDoc["expiresAt"].(time.Time)
	if time.Now().After(expiresAt) {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "OTP expired",
		})
	}

	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to hash password",
		})
	}

	// Update password
	_, err = smc.db.Collection("sales_managers").UpdateOne(
		context.Background(),
		bson.M{"phoneNumber": req.Phone},
		bson.M{"$set": bson.M{
			"password":  string(hashedPassword),
			"updatedAt": time.Now(),
		}},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update password",
		})
	}

	// Delete used OTP
	_, err = otpCollection.DeleteOne(context.Background(), bson.M{"phone": req.Phone})
	if err != nil {
		log.Printf("Failed to delete used OTP: %v", err)
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Password reset successfully",
	})
}

// Helper function to send OTP via SMS
func sendOTP(phone, otp string) error {
	// TODO: Implement actual SMS sending logic
	// This is a placeholder that should be replaced with your SMS service implementation
	log.Printf("Sending OTP %s to phone number %s", otp, phone)
	return nil
}

// GetSalespersonsByCreator retrieves all salespersons created by the current sales manager
func (smc *SalesManagerController) GetSalespersonsByCreator(c echo.Context) error {
	// Get sales manager ID from JWT token
	userID := c.Get("userId")
	if userID == nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "User ID not found in token",
		})
	}

	salesManagerID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Invalid sales manager ID",
		})
	}

	var salespersons []models.Salesperson
	cursor, err := smc.db.Collection("salespersons").Find(
		context.Background(),
		bson.M{"createdBy": salesManagerID},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch salespersons",
		})
	}
	defer cursor.Close(context.Background())

	if err := cursor.All(context.Background(), &salespersons); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode salespersons",
		})
	}

	// Remove passwords from response
	for i := range salespersons {
		salespersons[i].Password = ""
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Salespersons retrieved successfully",
		Data:    salespersons,
	})
}

// Pending Company Creations
func (smc *SalesManagerController) GetPendingCompanyCreations(c echo.Context) error {
	userID := c.Get("userId")
	if userID == nil {
		userID = c.Get("user_id")
	}
	if userID == nil {
		return c.JSON(401, map[string]string{"message": "User ID not found in token"})
	}
	salesManagerID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		return c.JSON(401, map[string]string{"message": "Invalid sales manager ID"})
	}

	// Use aggregation to join with salesperson collection to get name and email
	pipeline := []bson.M{
		{"$match": bson.M{"salesManagerId": salesManagerID}},
		{"$lookup": bson.M{
			"from":         "salespersons",
			"localField":   "salesPersonId",
			"foreignField": "_id",
			"as":           "salesperson",
		}},
		{"$unwind": bson.M{
			"path":                       "$salesperson",
			"preserveNullAndEmptyArrays": true,
		}},
		{"$addFields": bson.M{
			"salespersonName":  "$salesperson.fullName",
			"salespersonEmail": "$salesperson.email",
		}},
		{"$project": bson.M{
			"salesperson": 0, // Remove the full salesperson object
		}},
	}

	coll := smc.db.Collection("pending_company_requests")
	cursor, err := coll.Aggregate(c.Request().Context(), pipeline)
	if err != nil {
		return c.JSON(500, map[string]string{"message": "Failed to fetch pending company requests"})
	}
	defer cursor.Close(c.Request().Context())

	var results []bson.M
	if err := cursor.All(c.Request().Context(), &results); err != nil {
		return c.JSON(500, map[string]string{"message": "Failed to decode pending company requests"})
	}
	return c.JSON(200, results)
}

// GetCreatedCompanies retrieves all companies created by salespersons under this sales manager
func (smc *SalesManagerController) GetCreatedCompanies(c echo.Context) error {
	userID := c.Get("userId")
	if userID == nil {
		userID = c.Get("user_id")
	}
	if userID == nil {
		return c.JSON(401, map[string]string{"message": "User ID not found in token"})
	}
	salesManagerID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		return c.JSON(401, map[string]string{"message": "Invalid sales manager ID"})
	}

	// Get all salespersons under this sales manager
	var salespersons []models.Salesperson
	salespersonColl := smc.db.Collection("salespersons")
	cursor, err := salespersonColl.Find(c.Request().Context(), bson.M{"salesManagerId": salesManagerID})
	if err != nil {
		return c.JSON(500, map[string]string{"message": "Failed to fetch salespersons"})
	}
	if err := cursor.All(c.Request().Context(), &salespersons); err != nil {
		return c.JSON(500, map[string]string{"message": "Failed to decode salespersons"})
	}

	// Get salesperson IDs
	var salespersonIDs []primitive.ObjectID
	for _, sp := range salespersons {
		salespersonIDs = append(salespersonIDs, sp.ID)
	}

	// Get companies created by these salespersons
	companyColl := smc.db.Collection("companies")
	var companies []models.Company
	companyCursor, err := companyColl.Find(c.Request().Context(), bson.M{"createdBy": bson.M{"$in": salespersonIDs}})
	if err != nil {
		return c.JSON(500, map[string]string{"message": "Failed to fetch companies"})
	}
	if err := companyCursor.All(c.Request().Context(), &companies); err != nil {
		return c.JSON(500, map[string]string{"message": "Failed to decode companies"})
	}

	return c.JSON(200, map[string]interface{}{
		"companies": companies,
		"count":     len(companies),
	})
}
func (smc *SalesManagerController) ApprovePendingCompany(c echo.Context) error {
	id := c.Param("id")
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return c.JSON(400, map[string]string{"message": "Invalid request ID"})
	}

	// Get sales manager ID from token
	userID := c.Get("userId")
	if userID == nil {
		userID = c.Get("user_id")
	}
	if userID == nil {
		return c.JSON(401, map[string]string{"message": "User ID not found in token"})
	}

	// Get the pending request to get company details before approval
	coll := smc.db.Collection("pending_company_requests")
	var pendingDoc models.PendingCompanyRequest
	err = coll.FindOne(c.Request().Context(), bson.M{"_id": objID}).Decode(&pendingDoc)
	if err != nil {
		return c.JSON(500, map[string]string{"message": "Failed to get pending request details"})
	}

	err = utils.ApprovePendingRequestByManager(smc.db.Client(), objID, "company")
	if err != nil {
		return c.JSON(500, map[string]string{"message": err.Error()})
	}

	// Send notification to salesperson
	if !pendingDoc.SalesPersonID.IsZero() {
		title := "Company Request Approved"
		msg := "Your company creation request has been approved."
		_ = utils.SaveNotification(smc.db.Client(), pendingDoc.SalesPersonID, title, msg, "company_approval", map[string]interface{}{"companyName": pendingDoc.Company.BusinessName})
	}

	// Delete the pending request after successful processing
	_, err = coll.DeleteOne(c.Request().Context(), bson.M{"_id": objID})
	if err != nil {
		log.Printf("Failed to delete pending company request: %v", err)
		// Don't fail the approval, just log the error
	}

	return c.JSON(200, map[string]string{"message": "Company request approved"})
}
func (smc *SalesManagerController) RejectPendingCompany(c echo.Context) error {
	id := c.Param("id")
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return c.JSON(400, map[string]string{"message": "Invalid request ID"})
	}

	// Get sales manager ID from token
	userID := c.Get("userId")
	if userID == nil {
		userID = c.Get("user_id")
	}
	if userID == nil {
		return c.JSON(401, map[string]string{"message": "User ID not found in token"})
	}
	salesManagerID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		return c.JSON(401, map[string]string{"message": "Invalid sales manager ID"})
	}

	err = utils.RejectPendingRequestByManager(smc.db.Client(), objID, "company")
	if err != nil {
		return c.JSON(500, map[string]string{"message": err.Error()})
	}

	// Get the pending request to get company details
	coll := smc.db.Collection("pending_company_requests")
	var pendingDoc models.PendingCompanyRequest
	err = coll.FindOne(c.Request().Context(), bson.M{"_id": objID}).Decode(&pendingDoc)
	if err != nil {
		return c.JSON(500, map[string]string{"message": "Failed to get pending request details"})
	}

	// Update CreationRequest status in the main companies collection (if it exists)
	if !pendingDoc.Company.ID.IsZero() {
		companyColl := smc.db.Collection("companies")
		updateResult, err := companyColl.UpdateOne(
			c.Request().Context(),
			bson.M{"_id": pendingDoc.Company.ID},
			bson.M{
				"$set": bson.M{
					"CreationRequest":            "rejected",
					"CreationRequest.reviewedBy": salesManagerID,
					"CreationRequest.reviewedAt": time.Now(),
					"CreationRequest.reason":     pendingDoc.Reason,
				},
			},
		)
		if err != nil {
			// Log error but don't fail the request since the entity might not be in main collection yet
			log.Printf("Failed to update company creation request status: %v", err)
		}
		if updateResult != nil && updateResult.MatchedCount == 0 {
			// Entity not found in main collection, which is expected for rejected requests
			log.Printf("Company not found in main collection for rejection (expected)")
		}
	}

	// Send notification to salesperson
	if !pendingDoc.SalesPersonID.IsZero() {
		title := "Company Request Rejected"
		msg := "Your company creation request has been rejected."
		_ = utils.SaveNotification(smc.db.Client(), pendingDoc.SalesPersonID, title, msg, "company_rejection", map[string]interface{}{"companyName": pendingDoc.Company.BusinessName})
	}

	// Delete the pending request after successful processing
	_, err = coll.DeleteOne(c.Request().Context(), bson.M{"_id": objID})
	if err != nil {
		log.Printf("Failed to delete pending company request: %v", err)
		// Don't fail the rejection, just log the error
	}

	return c.JSON(200, map[string]string{"message": "Company request rejected"})
}

// Pending Wholesaler Creations
func (smc *SalesManagerController) GetPendingWholesalerCreations(c echo.Context) error {
	userID := c.Get("userId")
	if userID == nil {
		userID = c.Get("user_id")
	}
	if userID == nil {
		return c.JSON(401, map[string]string{"message": "User ID not found in token"})
	}
	salesManagerID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		return c.JSON(401, map[string]string{"message": "Invalid sales manager ID"})
	}

	// Use aggregation to join with salesperson collection to get name and email
	pipeline := []bson.M{
		{"$match": bson.M{"salesManagerId": salesManagerID}},
		{"$lookup": bson.M{
			"from":         "salespersons",
			"localField":   "salesPersonId",
			"foreignField": "_id",
			"as":           "salesperson",
		}},
		{"$unwind": bson.M{
			"path":                       "$salesperson",
			"preserveNullAndEmptyArrays": true,
		}},
		{"$addFields": bson.M{
			"salespersonName":  "$salesperson.fullName",
			"salespersonEmail": "$salesperson.email",
		}},
		{"$project": bson.M{
			"salesperson": 0, // Remove the full salesperson object
		}},
	}

	coll := smc.db.Collection("pending_wholesaler_requests")
	cursor, err := coll.Aggregate(c.Request().Context(), pipeline)
	if err != nil {
		return c.JSON(500, map[string]string{"message": "Failed to fetch pending wholesaler requests"})
	}
	defer cursor.Close(c.Request().Context())

	var results []bson.M
	if err := cursor.All(c.Request().Context(), &results); err != nil {
		return c.JSON(500, map[string]string{"message": "Failed to decode pending wholesaler requests"})
	}
	return c.JSON(200, results)
}
func (smc *SalesManagerController) ApprovePendingWholesaler(c echo.Context) error {
	id := c.Param("id")
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return c.JSON(400, map[string]string{"message": "Invalid request ID"})
	}

	// Get sales manager ID from token
	userID := c.Get("userId")
	if userID == nil {
		userID = c.Get("user_id")
	}
	if userID == nil {
		return c.JSON(401, map[string]string{"message": "User ID not found in token"})
	}

	// Get the pending request to get wholesaler details before approval
	coll := smc.db.Collection("pending_wholesaler_requests")
	var pendingDoc models.PendingWholesalerRequest
	err = coll.FindOne(c.Request().Context(), bson.M{"_id": objID}).Decode(&pendingDoc)
	if err != nil {
		return c.JSON(500, map[string]string{"message": "Failed to get pending request details"})
	}

	err = utils.ApprovePendingRequestByManager(smc.db.Client(), objID, "wholesaler")
	if err != nil {
		return c.JSON(500, map[string]string{"message": err.Error()})
	}

	// Send notification to salesperson
	if !pendingDoc.SalesPersonID.IsZero() {
		title := "Wholesaler Request Approved"
		msg := "Your wholesaler creation request has been approved."
		_ = utils.SaveNotification(smc.db.Client(), pendingDoc.SalesPersonID, title, msg, "wholesaler_approval", map[string]interface{}{"wholesalerName": pendingDoc.Wholesaler.BusinessName})
	}

	// Delete the pending request after successful processing
	_, err = coll.DeleteOne(c.Request().Context(), bson.M{"_id": objID})
	if err != nil {
		log.Printf("Failed to delete pending wholesaler request: %v", err)
		// Don't fail the approval, just log the error
	}

	return c.JSON(200, map[string]string{"message": "Wholesaler request approved"})
}
func (smc *SalesManagerController) RejectPendingWholesaler(c echo.Context) error {
	id := c.Param("id")
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return c.JSON(400, map[string]string{"message": "Invalid request ID"})
	}

	// Get sales manager ID from token
	userID := c.Get("userId")
	if userID == nil {
		userID = c.Get("user_id")
	}
	if userID == nil {
		return c.JSON(401, map[string]string{"message": "User ID not found in token"})
	}
	salesManagerID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		return c.JSON(401, map[string]string{"message": "Invalid sales manager ID"})
	}

	err = utils.RejectPendingRequestByManager(smc.db.Client(), objID, "wholesaler")
	if err != nil {
		return c.JSON(500, map[string]string{"message": err.Error()})
	}

	// Get the pending request to get wholesaler details
	coll := smc.db.Collection("pending_wholesaler_requests")
	var pendingDoc models.PendingWholesalerRequest
	err = coll.FindOne(c.Request().Context(), bson.M{"_id": objID}).Decode(&pendingDoc)
	if err != nil {
		return c.JSON(500, map[string]string{"message": "Failed to get pending request details"})
	}

	// Update CreationRequest status in the main wholesalers collection (if it exists)
	if !pendingDoc.Wholesaler.ID.IsZero() {
		wholesalerColl := smc.db.Collection("wholesalers")
		updateResult, err := wholesalerColl.UpdateOne(
			c.Request().Context(),
			bson.M{"_id": pendingDoc.Wholesaler.ID},
			bson.M{
				"$set": bson.M{
					"CreationRequest":            "rejected",
					"CreationRequest.reviewedBy": salesManagerID,
					"CreationRequest.reviewedAt": time.Now(),
					"CreationRequest.reason":     pendingDoc.Reason,
				},
			},
		)
		if err != nil {
			// Log error but don't fail the request since the entity might not be in main collection yet
			log.Printf("Failed to update wholesaler creation request status: %v", err)
		}
		if updateResult != nil && updateResult.MatchedCount == 0 {
			// Entity not found in main collection, which is expected for rejected requests
			log.Printf("Wholesaler not found in main collection for rejection (expected)")
		}
	}

	// Send notification to salesperson
	if !pendingDoc.SalesPersonID.IsZero() {
		title := "Wholesaler Request Rejected"
		msg := "Your wholesaler creation request has been rejected."
		_ = utils.SaveNotification(smc.db.Client(), pendingDoc.SalesPersonID, title, msg, "wholesaler_rejection", map[string]interface{}{"wholesalerName": pendingDoc.Wholesaler.BusinessName})
	}

	// Delete the pending request after successful processing
	_, err = coll.DeleteOne(c.Request().Context(), bson.M{"_id": objID})
	if err != nil {
		log.Printf("Failed to delete pending wholesaler request: %v", err)
		// Don't fail the rejection, just log the error
	}

	return c.JSON(200, map[string]string{"message": "Wholesaler request rejected"})
}

// Pending Service Provider Creations
func (smc *SalesManagerController) GetPendingServiceProviderCreations(c echo.Context) error {
	userID := c.Get("userId")
	if userID == nil {
		userID = c.Get("user_id")
	}
	if userID == nil {
		return c.JSON(401, map[string]string{"message": "User ID not found in token"})
	}
	salesManagerID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		return c.JSON(401, map[string]string{"message": "Invalid sales manager ID"})
	}

	// Use aggregation to join with salesperson collection to get name and email
	pipeline := []bson.M{
		{"$match": bson.M{"salesManagerId": salesManagerID}},
		{"$lookup": bson.M{
			"from":         "salespersons",
			"localField":   "salesPersonId",
			"foreignField": "_id",
			"as":           "salesperson",
		}},
		{"$unwind": bson.M{
			"path":                       "$salesperson",
			"preserveNullAndEmptyArrays": true,
		}},
		{"$addFields": bson.M{
			"salespersonName":  "$salesperson.fullName",
			"salespersonEmail": "$salesperson.email",
		}},
		{"$project": bson.M{
			"salesperson": 0, // Remove the full salesperson object
		}},
	}

	coll := smc.db.Collection("pending_serviceProviders_requests")
	cursor, err := coll.Aggregate(c.Request().Context(), pipeline)
	if err != nil {
		return c.JSON(500, map[string]string{"message": "Failed to fetch pending service provider requests"})
	}
	defer cursor.Close(c.Request().Context())

	var results []bson.M
	if err := cursor.All(c.Request().Context(), &results); err != nil {
		return c.JSON(500, map[string]string{"message": "Failed to decode pending service provider requests"})
	}
	return c.JSON(200, results)
}
func (smc *SalesManagerController) ApprovePendingServiceProvider(c echo.Context) error {
	id := c.Param("id")
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return c.JSON(400, map[string]string{"message": "Invalid request ID"})
	}

	// Get sales manager ID from token
	userID := c.Get("userId")
	if userID == nil {
		userID = c.Get("user_id")
	}
	if userID == nil {
		return c.JSON(401, map[string]string{"message": "User ID not found in token"})
	}

	// Get the pending request to get service provider details before approval
	coll := smc.db.Collection("pending_serviceProviders_requests")
	var pendingDoc models.PendingServiceProviderRequest
	err = coll.FindOne(c.Request().Context(), bson.M{"_id": objID}).Decode(&pendingDoc)
	if err != nil {
		return c.JSON(500, map[string]string{"message": "Failed to get pending request details"})
	}

	err = utils.ApprovePendingRequestByManager(smc.db.Client(), objID, "serviceProvider")
	if err != nil {
		return c.JSON(500, map[string]string{"message": err.Error()})
	}

	// Send notification to salesperson
	if !pendingDoc.SalesPersonID.IsZero() {
		title := "Service Provider Request Approved"
		msg := "Your service provider creation request has been approved."
		_ = utils.SaveNotification(smc.db.Client(), pendingDoc.SalesPersonID, title, msg, "serviceProviders_approval", map[string]interface{}{"serviceProviderName": pendingDoc.ServiceProvider.BusinessName})
	}

	// Delete the pending request after successful processing
	_, err = coll.DeleteOne(c.Request().Context(), bson.M{"_id": objID})
	if err != nil {
		log.Printf("Failed to delete pending service provider request: %v", err)
		// Don't fail the approval, just log the error
	}

	return c.JSON(200, map[string]string{"message": "Service provider request approved"})
}
func (smc *SalesManagerController) RejectPendingServiceProvider(c echo.Context) error {
	id := c.Param("id")
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return c.JSON(400, map[string]string{"message": "Invalid request ID"})
	}

	// Get sales manager ID from token
	userID := c.Get("userId")
	if userID == nil {
		userID = c.Get("user_id")
	}
	if userID == nil {
		return c.JSON(401, map[string]string{"message": "User ID not found in token"})
	}
	salesManagerID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		return c.JSON(401, map[string]string{"message": "Invalid sales manager ID"})
	}

	err = utils.RejectPendingRequestByManager(smc.db.Client(), objID, "serviceProvider")
	if err != nil {
		return c.JSON(500, map[string]string{"message": err.Error()})
	}

	// Get the pending request to get service provider details
	coll := smc.db.Collection("pending_serviceProviders_requests")
	var pendingDoc models.PendingServiceProviderRequest
	err = coll.FindOne(c.Request().Context(), bson.M{"_id": objID}).Decode(&pendingDoc)
	if err != nil {
		return c.JSON(500, map[string]string{"message": "Failed to get pending request details"})
	}

	// Update CreationRequest status in the main service providers collection (if it exists)
	if !pendingDoc.ServiceProvider.ID.IsZero() {
		spColl := smc.db.Collection("serviceProviders")
		updateResult, err := spColl.UpdateOne(
			c.Request().Context(),
			bson.M{"_id": pendingDoc.ServiceProvider.ID},
			bson.M{
				"$set": bson.M{
					"CreationRequest":            "rejected",
					"CreationRequest.reviewedBy": salesManagerID,
					"CreationRequest.reviewedAt": time.Now(),
					"CreationRequest.reason":     pendingDoc.Reason,
				},
			},
		)
		if err != nil {
			// Log error but don't fail the request since the entity might not be in main collection yet
			log.Printf("Failed to update service provider creation request status: %v", err)
		}
		if updateResult != nil && updateResult.MatchedCount == 0 {
			// Entity not found in main collection, which is expected for rejected requests
			log.Printf("Service provider not found in main collection for rejection (expected)")
		}
	}

	// Send notification to salesperson
	if !pendingDoc.SalesPersonID.IsZero() {
		title := "Service Provider Request Rejected"
		msg := "Your service provider creation request has been rejected."
		_ = utils.SaveNotification(smc.db.Client(), pendingDoc.SalesPersonID, title, msg, "serviceProviders_rejection", map[string]interface{}{"serviceProviderName": pendingDoc.ServiceProvider.BusinessName})
	}

	// Delete the pending request after successful processing
	_, err = coll.DeleteOne(c.Request().Context(), bson.M{"_id": objID})
	if err != nil {
		log.Printf("Failed to delete pending service provider request: %v", err)
		// Don't fail the rejection, just log the error
	}

	return c.JSON(200, map[string]string{"message": "Service provider request rejected"})
}

// ProcessSubscriptionRequest handles the approval or rejection of a subscription request by sales manager
func (smc *SalesManagerController) ProcessSubscriptionRequest(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user information from token
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "sales_manager" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only sales managers can access this endpoint",
		})
	}

	// Get request ID from URL parameter
	requestID := c.Param("id")
	if requestID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Request ID is required",
		})
	}

	// Convert request ID to ObjectID
	requestObjectID, err := primitive.ObjectIDFromHex(requestID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request ID format",
		})
	}

	// Parse approval request body
	var approvalReq models.SubscriptionApprovalRequest
	if err := c.Bind(&approvalReq); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}

	// Validate status
	if approvalReq.Status != "approved" && approvalReq.Status != "rejected" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid status. Must be 'approved' or 'rejected'",
		})
	}

	// Get the subscription request
	subscriptionRequestsCollection := smc.db.Collection("subscription_requests")
	var subscriptionRequest models.SubscriptionRequest
	err = subscriptionRequestsCollection.FindOne(ctx, bson.M{"_id": requestObjectID}).Decode(&subscriptionRequest)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Subscription request not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find subscription request",
		})
	}

	// Check if request is already processed
	if subscriptionRequest.Status != "pending" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Subscription request is already processed",
		})
	}

	// If approved, create the subscription and save user to database
	if approvalReq.Status == "approved" {
		// Get plan details to calculate end date
		var plan models.SubscriptionPlan
		err = smc.db.Collection("subscription_plans").FindOne(ctx, bson.M{"_id": subscriptionRequest.PlanID}).Decode(&plan)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to get plan details",
			})
		}

		// Calculate end date based on plan duration
		startDate := time.Now()
		var endDate time.Time
		switch plan.Duration {
		case 1: // Monthly
			endDate = startDate.AddDate(0, 1, 0)
		case 6: // 6 Months
			endDate = startDate.AddDate(0, 6, 0)
		case 12: // 1 Year
			endDate = startDate.AddDate(1, 0, 0)
		default:
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Invalid plan duration",
			})
		}

		// Create subscription based on entity type
		if !subscriptionRequest.CompanyID.IsZero() {
			// Create company subscription
			subscription := models.CompanySubscription{
				ID:        primitive.NewObjectID(),
				CompanyID: subscriptionRequest.CompanyID,
				PlanID:    subscriptionRequest.PlanID,
				StartDate: startDate,
				EndDate:   endDate,
				Status:    "active",
				AutoRenew: false,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}

			// Save subscription
			subscriptionsCollection := smc.db.Collection("company_subscriptions")
			_, err = subscriptionsCollection.InsertOne(ctx, subscription)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, models.Response{
					Status:  http.StatusInternalServerError,
					Message: "Failed to create company subscription",
				})
			}

			// Update company status to active
			companyCollection := smc.db.Collection("companies")
			_, err = companyCollection.UpdateOne(ctx, bson.M{"_id": subscriptionRequest.CompanyID}, bson.M{"$set": bson.M{"status": "active"}})
			if err != nil {
				log.Printf("Failed to update company status: %v", err)
			}

		} else if !subscriptionRequest.ServiceProviderID.IsZero() {
			// Create service provider subscription
			subscription := models.ServiceProviderSubscription{
				ID:                primitive.NewObjectID(),
				ServiceProviderID: subscriptionRequest.ServiceProviderID,
				PlanID:            subscriptionRequest.PlanID,
				StartDate:         startDate,
				EndDate:           endDate,
				Status:            "active",
				AutoRenew:         false,
				CreatedAt:         time.Now(),
				UpdatedAt:         time.Now(),
			}

			// Save subscription
			subscriptionsCollection := smc.db.Collection("serviceProviders_subscriptions")
			_, err = subscriptionsCollection.InsertOne(ctx, subscription)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, models.Response{
					Status:  http.StatusInternalServerError,
					Message: "Failed to create service provider subscription",
				})
			}

			// Update service provider status to active
			serviceProviderCollection := smc.db.Collection("serviceProviders")
			_, err = serviceProviderCollection.UpdateOne(ctx, bson.M{"_id": subscriptionRequest.ServiceProviderID}, bson.M{"$set": bson.M{"status": "active"}})
			if err != nil {
				log.Printf("Failed to update service provider status: %v", err)
			}
		}

		// Send approval notification
		if !subscriptionRequest.CompanyID.IsZero() {
			var company models.Company
			err = smc.db.Collection("companies").FindOne(ctx, bson.M{"_id": subscriptionRequest.CompanyID}).Decode(&company)
			if err == nil {
				// Send notification to company
				log.Printf("Company subscription approved: %s", company.BusinessName)
			}
		} else if !subscriptionRequest.ServiceProviderID.IsZero() {
			var serviceProvider models.ServiceProvider
			err = smc.db.Collection("serviceProviders").FindOne(ctx, bson.M{"_id": subscriptionRequest.ServiceProviderID}).Decode(&serviceProvider)
			if err == nil {
				// Send notification to service provider
				log.Printf("Service provider subscription approved: %s", serviceProvider.BusinessName)
			}
		}
	} else {
		// If rejected, send rejection notification
		if !subscriptionRequest.CompanyID.IsZero() {
			var company models.Company
			err = smc.db.Collection("companies").FindOne(ctx, bson.M{"_id": subscriptionRequest.CompanyID}).Decode(&company)
			if err == nil {
				log.Printf("Company subscription rejected: %s, Reason: %s", company.BusinessName, approvalReq.AdminNote)
			}
		} else if !subscriptionRequest.ServiceProviderID.IsZero() {
			var serviceProvider models.ServiceProvider
			err = smc.db.Collection("serviceProviders").FindOne(ctx, bson.M{"_id": subscriptionRequest.ServiceProviderID}).Decode(&serviceProvider)
			if err == nil {
				log.Printf("Service provider subscription rejected: %s, Reason: %s", serviceProvider.BusinessName, approvalReq.AdminNote)
			}
		}
	}

	// Delete the subscription request from database after processing
	_, err = subscriptionRequestsCollection.DeleteOne(ctx, bson.M{"_id": requestObjectID})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to delete subscription request",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: fmt.Sprintf("Subscription request %s successfully", approvalReq.Status),
	})
}

// GetPendingSubscriptionRequests retrieves all pending subscription requests for sales manager
func (smc *SalesManagerController) GetPendingSubscriptionRequests(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user information from token
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "sales_manager" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only sales managers can access this endpoint",
		})
	}

	// Get pending subscription requests
	subscriptionRequestsCollection := smc.db.Collection("subscription_requests")
	cursor, err := subscriptionRequestsCollection.Find(ctx, bson.M{"status": "pending"})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch pending subscription requests",
		})
	}
	defer cursor.Close(ctx)

	var subscriptionRequests []models.SubscriptionRequest
	if err := cursor.All(ctx, &subscriptionRequests); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode subscription requests",
		})
	}

	// Enrich requests with entity details
	var enrichedRequests []map[string]interface{}
	for _, request := range subscriptionRequests {
		enrichedRequest := map[string]interface{}{
			"request": request,
		}

		// Add entity details based on type
		if !request.CompanyID.IsZero() {
			var company models.Company
			err = smc.db.Collection("companies").FindOne(ctx, bson.M{"_id": request.CompanyID}).Decode(&company)
			if err == nil {
				enrichedRequest["entity"] = company
				enrichedRequest["entityType"] = "company"
			}
		} else if !request.ServiceProviderID.IsZero() {
			var serviceProvider models.ServiceProvider
			err = smc.db.Collection("serviceProviders").FindOne(ctx, bson.M{"_id": request.ServiceProviderID}).Decode(&serviceProvider)
			if err == nil {
				enrichedRequest["entity"] = serviceProvider
				enrichedRequest["entityType"] = "serviceProvider"
			}
		}

		// Add plan details
		var plan models.SubscriptionPlan
		err = smc.db.Collection("subscription_plans").FindOne(ctx, bson.M{"_id": request.PlanID}).Decode(&plan)
		if err == nil {
			enrichedRequest["plan"] = plan
		}

		enrichedRequests = append(enrichedRequests, enrichedRequest)
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Pending subscription requests retrieved successfully",
		Data:    enrichedRequests,
	})
}

// GetCommissionAndWithdrawalHistory retrieves all commission and withdrawal records for the authenticated sales manager
func (smc *SalesManagerController) GetCommissionAndWithdrawalHistory(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Invalid user ID: " + err.Error(),
		})
	}

	// Fetch commission records for sales manager
	// Try both field name formats to handle legacy data
	commissionFilter := bson.M{
		"$or": []bson.M{
			{"salesManagerID": userID}, // New format (uppercase ID)
			{"salesManagerId": userID}, // Legacy format (lowercase i)
		},
	}
	commissionCursor, err := smc.db.Collection("commissions").Find(
		context.Background(),
		commissionFilter,
		&options.FindOptions{Sort: bson.M{"createdAt": -1}},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch commission history: " + err.Error(),
		})
	}
	var commissions []models.Commission
	if err := commissionCursor.All(context.Background(), &commissions); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode commission history: " + err.Error(),
		})
	}

	// Fetch withdrawal records for sales manager
	withdrawalFilter := bson.M{"userId": userID, "userType": "sales_manager"}
	withdrawalCursor, err := smc.db.Collection("withdrawals").Find(
		context.Background(),
		withdrawalFilter,
		&options.FindOptions{Sort: bson.M{"createdAt": -1}},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch withdrawal history: " + err.Error(),
		})
	}
	var withdrawals []models.Withdrawal
	if err := withdrawalCursor.All(context.Background(), &withdrawals); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode withdrawal history: " + err.Error(),
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Commission and withdrawal history retrieved successfully",
		Data: map[string]interface{}{
			"commissions": commissions,
			"withdrawals": withdrawals,
		},
	})
}
