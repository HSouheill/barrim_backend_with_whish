package controllers

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/HSouheill/barrim_backend/middleware"
	"github.com/HSouheill/barrim_backend/models"
	"github.com/HSouheill/barrim_backend/utils"
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/crypto/bcrypt"
)

// Helper function to get map keys
func getMapKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// SalesPersonController handles sales person related operations
type SalesPersonController struct {
	DB *mongo.Client
}

// NewSalesPersonController creates a new sales person controller
func NewSalesPersonController(db *mongo.Client) *SalesPersonController {
	return &SalesPersonController{DB: db}
}

// Login handles sales person authentication
func (spc *SalesPersonController) Login(c echo.Context) error {
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

	// Find sales person by email
	var salesPerson models.Salesperson
	err := spc.DB.Database("barrim").Collection("salespersons").FindOne(
		context.Background(),
		bson.M{"email": loginReq.Email},
	).Decode(&salesPerson)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusUnauthorized, models.Response{
				Status:  http.StatusUnauthorized,
				Message: "Invalid email or password",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find sales person",
		})
	}

	// Check password
	err = bcrypt.CompareHashAndPassword([]byte(salesPerson.Password), []byte(loginReq.Password))
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Invalid email or password",
		})
	}

	// Generate JWT token
	token, refreshToken, err := middleware.GenerateJWT(salesPerson.ID.Hex(), salesPerson.Email, "salesperson")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to generate token",
		})
	}

	// Update last login time
	_, err = spc.DB.Database("barrim").Collection("salespersons").UpdateOne(
		context.Background(),
		bson.M{"_id": salesPerson.ID},
		bson.M{"$set": bson.M{"updatedAt": time.Now()}},
	)
	if err != nil {
		log.Printf("Failed to update last login time: %v", err)
	}

	// Remove password from response
	salesPerson.Password = ""

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Login successful",
		Data: map[string]interface{}{
			"token":        token,
			"refreshToken": refreshToken,
			"user":         salesPerson,
		},
	})
}

// ForgotPassword initiates the password reset process
func (spc *SalesPersonController) ForgotPassword(c echo.Context) error {
	var req struct {
		Phone string `json:"phone"`
	}

	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}

	// Find sales person by phone number
	var salesPerson models.Salesperson
	err := spc.DB.Database("barrim").Collection("salespersons").FindOne(
		context.Background(),
		bson.M{"phoneNumber": req.Phone},
	).Decode(&salesPerson)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "No account found with this phone number",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find sales person",
		})
	}

	// Generate OTP
	otp := generateAuthOTP()
	expiresAt := time.Now().Add(10 * time.Minute)

	// Store OTP in database
	otpCollection := spc.DB.Database("barrim").Collection("password_reset_otps")
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

// CreateCompany creates a new company
func (spc *SalesPersonController) CreateCompany(c echo.Context) error {
	// Get sales person ID from token
	claims := middleware.GetUserFromToken(c)
	log.Printf("Claims from token: %+v", claims)

	salesPersonID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		log.Printf("Error converting sales person ID: %v", err)
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Invalid sales person ID",
		})
	}
	log.Printf("Sales person ID: %v", salesPersonID)

	// Parse form data
	form, err := c.MultipartForm()
	if err != nil {
		log.Printf("Error parsing multipart form: %v", err)
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid form data: " + err.Error(),
		})
	}
	log.Printf("Form data received: %+v", form.Value)

	// Debug: Print all form keys and values
	log.Printf("Form keys: %v", getMapKeys(form.Value))
	for key, values := range form.Value {
		log.Printf("Form field '%s': %v", key, values)
	}

	// Helper function to safely get form values
	getFormValue := func(key string) (string, error) {
		if values, ok := form.Value[key]; ok && len(values) > 0 {
			return values[0], nil
		}
		return "", fmt.Errorf("missing required field: %s", key)
	}

	// Get and validate form values
	// Parse geographic coordinates if provided
	var lat, lng float64

	// Helper function to safely get optional form values
	getOptionalFormValue := func(key string) (string, bool) {
		if values, ok := form.Value[key]; ok && len(values) > 0 {
			return values[0], true
		}
		return "", false
	}

	latStr, hasLat := getOptionalFormValue("lat")
	if hasLat {
		log.Printf("Raw lat string: '%s' (length: %d)", latStr, len(latStr))
		// Trim any whitespace
		latStr = strings.TrimSpace(latStr)
		log.Printf("Trimmed lat string: '%s' (length: %d)", latStr, len(latStr))
		if latParsed, parseErr := strconv.ParseFloat(latStr, 64); parseErr == nil {
			lat = latParsed
			log.Printf("Successfully parsed lat: %f", lat)
		} else {
			log.Printf("Warning: Failed to parse lat value '%s': %v", latStr, parseErr)
		}
	} else {
		log.Printf("Warning: lat field not provided or empty")
	}

	lngStr, hasLng := getOptionalFormValue("lng")
	if hasLng {
		log.Printf("Raw lng string: '%s' (length: %d)", lngStr, len(lngStr))
		// Trim any whitespace
		lngStr = strings.TrimSpace(lngStr)
		log.Printf("Trimmed lng string: '%s' (length: %d)", lngStr, len(lngStr))
		if lngParsed, parseErr := strconv.ParseFloat(lngStr, 64); parseErr == nil {
			lng = lngParsed
			log.Printf("Successfully parsed lng: %f", lng)
		} else {
			log.Printf("Warning: Failed to parse lng value '%s': %v", lngStr, parseErr)
		}
	} else {
		log.Printf("Warning: lng field not provided or empty")
	}

	log.Printf("Final lat: %f, lng: %f", lat, lng)
	businessName, err := getFormValue("businessName")
	if err != nil {
		log.Printf("Error getting businessName: %v", err)
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: err.Error(),
		})
	}

	category, err := getFormValue("category")
	if err != nil {
		log.Printf("Error getting category: %v", err)
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: err.Error(),
		})
	}

	subcategory, _ := getFormValue("subcategory") // Subcategory is optional

	email, _ := getFormValue("email") // Email is optional

	phone, err := getFormValue("phone")
	if err != nil {
		log.Printf("Error getting phone: %v", err)
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: err.Error(),
		})
	}

	password, err := getFormValue("password")
	if err != nil {
		log.Printf("Error getting password: %v", err)
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: err.Error(),
		})
	}

	contactPhone, err := getFormValue("contactPhone")
	if err != nil {
		log.Printf("Error getting contactPhone: %v", err)
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: err.Error(),
		})
	}

	contactPerson, err := getFormValue("contactPerson")
	if err != nil {
		log.Printf("Error getting contactPerson: %v", err)
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: err.Error(),
		})
	}

	country, err := getFormValue("country")
	if err != nil {
		log.Printf("Error getting country: %v", err)
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: err.Error(),
		})
	}
	district, err := getFormValue("district")
	if err != nil {
		log.Printf("Error getting district: %v", err)
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: err.Error(),
		})
	}
	city, err := getFormValue("city")
	if err != nil {
		log.Printf("Error getting city: %v", err)
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: err.Error(),
		})
	}
	governorate, err := getFormValue("governorate")
	if err != nil {
		log.Printf("Error getting street: %v", err)
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: err.Error(),
		})
	}

	// Handle logo upload
	var logoURL string
	if files, ok := form.File["logo"]; ok && len(files) > 0 {
		file := files[0]
		log.Printf("Processing logo file: %s", file.Filename)

		src, err := file.Open()
		if err != nil {
			log.Printf("Error opening uploaded file: %v", err)
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to open uploaded file: " + err.Error(),
			})
		}
		defer src.Close()

		// Create uploads directory if it doesn't exist
		uploadDir := "uploads/logo"
		if err := os.MkdirAll(uploadDir, 0755); err != nil {
			log.Printf("Error creating upload directory: %v", err)
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to create upload directory: " + err.Error(),
			})
		}

		// Generate unique filename
		filename := fmt.Sprintf("%d_%s", time.Now().UnixNano(), file.Filename)
		filepath := filepath.Join(uploadDir, filename)
		log.Printf("Saving logo to: %s", filepath)

		// Create destination file
		dst, err := os.Create(filepath)
		if err != nil {
			log.Printf("Error creating destination file: %v", err)
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to create destination file: " + err.Error(),
			})
		}
		defer dst.Close()

		// Copy file contents
		if _, err = io.Copy(dst, src); err != nil {
			log.Printf("Error copying file contents: %v", err)
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to save file: " + err.Error(),
			})
		}

		logoURL = filepath
	}

	// Handle profile picture upload
	var profilePicURL string
	if files, ok := form.File["profilePic"]; ok && len(files) > 0 {
		file := files[0]
		log.Printf("Processing profile picture file: %s", file.Filename)

		src, err := file.Open()
		if err != nil {
			log.Printf("Error opening uploaded profile picture file: %v", err)
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to open uploaded profile picture file: " + err.Error(),
			})
		}
		defer src.Close()

		// Create uploads directory if it doesn't exist
		uploadDir := "uploads/profiles"
		if err := os.MkdirAll(uploadDir, 0755); err != nil {
			log.Printf("Error creating profile upload directory: %v", err)
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to create profile upload directory: " + err.Error(),
			})
		}

		// Generate unique filename
		filename := fmt.Sprintf("profile_%d_%s", time.Now().UnixNano(), file.Filename)
		filepath := filepath.Join(uploadDir, filename)
		log.Printf("Saving profile picture to: %s", filepath)

		// Create destination file
		dst, err := os.Create(filepath)
		if err != nil {
			log.Printf("Error creating destination profile picture file: %v", err)
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to create destination profile picture file: " + err.Error(),
			})
		}
		defer dst.Close()

		// Copy file contents
		if _, err = io.Copy(dst, src); err != nil {
			log.Printf("Error copying profile picture file contents: %v", err)
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to save profile picture file: " + err.Error(),
			})
		}

		profilePicURL = filepath
	}

	// Handle additional emails and phones
	additionalEmails := []string{}
	if emails, ok := form.Value["additionalEmails"]; ok {
		for _, email := range emails {
			if strings.TrimSpace(email) != "" {
				additionalEmails = append(additionalEmails, strings.TrimSpace(email))
			}
		}
	}

	additionalPhones := []string{}
	if phones, ok := form.Value["additionalPhones"]; ok {
		for _, phone := range phones {
			if strings.TrimSpace(phone) != "" {
				additionalPhones = append(additionalPhones, strings.TrimSpace(phone))
			}
		}
	}

	// Hash password for storage
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("Error hashing password: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to hash password: " + err.Error(),
		})
	}

	// Generate referral code
	referralCode, err := utils.GenerateCompanyReferralCode()
	if err != nil {
		log.Printf("Error generating referral code: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to generate referral code",
		})
	}

	// Create the main branch for the company
	branch := models.Branch{
		ID:   primitive.NewObjectID(),
		Name: businessName,
		Location: models.Address{
			Country:     country,
			District:    district,
			City:        city,
			Governorate: governorate,
			Lat:         lat,
			Lng:         lng,
		},
		Phone:       phone,
		Category:    category,
		SubCategory: subcategory, // Use the subcategory from form data
		Images:      []string{},
		Videos:      []string{},
		Status:      "inactive", // Set to inactive until subscription approval
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Add logo to branch images if provided
	if logoURL != "" {
		branch.Images = append(branch.Images, logoURL)
	}

	// Create company object
	companyID := primitive.NewObjectID()
	userID := primitive.NewObjectID()

	company := models.Company{
		ID:               companyID,
		UserID:           userID,
		Email:            email, // Add the missing email field
		BusinessName:     businessName,
		Category:         category,
		SubCategory:      subcategory,
		LogoURL:          logoURL,
		ProfilePicURL:    profilePicURL,
		ReferralCode:     referralCode,
		ContactPerson:    contactPerson,
		AdditionalPhones: additionalPhones,
		AdditionalEmails: additionalEmails,
		ContactInfo: models.ContactInfo{
			Phone:    phone,
			WhatsApp: contactPhone,
			Address: models.Address{
				Country:     country,
				District:    district,
				City:        city,
				Governorate: governorate,
				Lat:         lat,
				Lng:         lng,
			},
		},
		Branches:        []models.Branch{branch}, // Add the created branch
		CreatedBy:       salesPersonID,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		CreationRequest: "pending", // Set to pending for sales manager approval
	}

	pendingRequest := models.PendingCompanyRequest{
		ID:               primitive.NewObjectID(),
		Company:          company,
		Email:            email,
		AdditionalEmails: additionalEmails,
		Password:         string(hashedPassword),
		SalesPersonID:    salesPersonID,
		SalesManagerID:   primitive.NilObjectID, // Set later if needed
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	// Set correct SalesManagerID
	var salesperson models.Salesperson
	err = spc.DB.Database("barrim").Collection("salespersons").FindOne(context.Background(), bson.M{"_id": salesPersonID}).Decode(&salesperson)
	if err == nil {
		pendingRequest.SalesManagerID = salesperson.SalesManagerID
	}

	_, err = spc.DB.Database("barrim").Collection("pending_company_requests").InsertOne(context.Background(), pendingRequest)
	if err != nil {
		log.Printf("Error inserting pending company request: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to submit company for approval: " + err.Error(),
		})
	}

	// Notify sales manager (log error but do not fail request)
	if notifyErr := utils.NotifySalesManagerOfRequest(spc.DB, salesPersonID, "Company", businessName); notifyErr != nil {
		log.Printf("Failed to notify sales manager: %v", notifyErr)
	}

	return c.JSON(http.StatusCreated, models.Response{
		Status:  http.StatusCreated,
		Message: "Company submitted for approval",
		Data:    pendingRequest,
	})
}

// GetCompanies retrieves all companies created by the sales person
func (spc *SalesPersonController) GetCompanies(c echo.Context) error {
	// Get sales person ID from token
	claims := middleware.GetUserFromToken(c)
	salesPersonID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Invalid sales person ID",
		})
	}

	// Find all companies created by this sales person
	var companies []models.Company
	cursor, err := spc.DB.Database("barrim").Collection("companies").Find(
		context.Background(),
		bson.M{"createdBy": salesPersonID},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch companies",
		})
	}
	defer cursor.Close(context.Background())

	if err := cursor.All(context.Background(), &companies); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode companies",
		})
	}

	// Get user information for contact details
	db := spc.DB.Database("barrim")
	userIDs := make([]primitive.ObjectID, 0, len(companies))
	for _, company := range companies {
		userIDs = append(userIDs, company.UserID)
	}

	userMap := make(map[string]models.User)
	if len(userIDs) > 0 {
		userCursor, err := db.Collection("users").Find(
			context.Background(),
			bson.M{"_id": bson.M{"$in": userIDs}},
		)
		if err == nil {
			var users []models.User
			_ = userCursor.All(context.Background(), &users)
			userCursor.Close(context.Background())
			for _, user := range users {
				userMap[user.ID.Hex()] = user
			}
		}
	}

	// Get subscription information for each company
	var enrichedCompanies []map[string]interface{}
	for _, company := range companies {
		user := userMap[company.UserID.Hex()]

		// Get active subscription for this company
		var planTitle string
		var planPrice float64
		var planDuration int
		var planType string
		var planBenefits interface{}
		var subscriptionStartDate time.Time
		var subscriptionEndDate time.Time
		var subscriptionLevel string
		var remainingDays int
		var hasActiveSubscription bool
		var totalActiveSubscriptions int
		var totalSubscriptionValue float64

		// First check for company-level subscriptions
		var companySubscription models.CompanySubscription
		err := db.Collection("company_subscriptions").FindOne(
			context.Background(),
			bson.M{
				"companyId": company.ID,
				"status":    "active",
				"endDate":   bson.M{"$gt": time.Now()},
			},
		).Decode(&companySubscription)

		// Count total active company subscriptions
		if err == nil {
			totalActiveSubscriptions++
		}

		if err == nil {
			// Get plan details for company subscription
			var plan models.SubscriptionPlan
			err = db.Collection("subscription_plans").FindOne(
				context.Background(),
				bson.M{"_id": companySubscription.PlanID},
			).Decode(&plan)

			if err == nil {
				planTitle = plan.Title
				planPrice = plan.Price
				planDuration = plan.Duration
				planType = plan.Type
				planBenefits = plan.Benefits
				subscriptionStartDate = companySubscription.StartDate
				subscriptionEndDate = companySubscription.EndDate
				subscriptionLevel = "company"
				hasActiveSubscription = true
				// Calculate remaining days
				remainingDays = int(time.Until(companySubscription.EndDate).Hours() / 24)
				// Add to total subscription value
				totalSubscriptionValue += planPrice
			}
		}

		// If no company subscription found, check for branch subscriptions
		// Also check if there are multiple active subscriptions for better reporting
		if !hasActiveSubscription && len(company.Branches) > 0 {
			// Get all branch IDs for this company
			branchIDs := make([]primitive.ObjectID, 0, len(company.Branches))
			for _, branch := range company.Branches {
				branchIDs = append(branchIDs, branch.ID)
			}

			if len(branchIDs) > 0 {
				// Check for active branch subscriptions
				var branchSubscription models.BranchSubscription
				err := db.Collection("branch_subscriptions").FindOne(
					context.Background(),
					bson.M{
						"branchId": bson.M{"$in": branchIDs},
						"status":   "active",
						"endDate":  bson.M{"$gt": time.Now()},
					},
				).Decode(&branchSubscription)

				// Count total active branch subscriptions
				if err == nil {
					totalActiveSubscriptions++
				}

				if err == nil {
					// Get plan details for branch subscription
					var plan models.SubscriptionPlan
					err = db.Collection("subscription_plans").FindOne(
						context.Background(),
						bson.M{"_id": branchSubscription.PlanID},
					).Decode(&plan)

					if err == nil {
						planTitle = plan.Title
						planPrice = plan.Price
						planDuration = plan.Duration
						planType = plan.Type
						planBenefits = plan.Benefits
						subscriptionStartDate = branchSubscription.StartDate
						subscriptionEndDate = branchSubscription.EndDate
						subscriptionLevel = "branch"
						hasActiveSubscription = true
						// Calculate remaining days
						remainingDays = int(time.Until(branchSubscription.EndDate).Hours() / 24)
						// Add to total subscription value
						totalSubscriptionValue += planPrice
					}
				}
			}
		}

		enrichedCompany := map[string]interface{}{
			"id":            company.ID,
			"businessName":  company.BusinessName,
			"category":      company.Category,
			"logoURL":       company.LogoURL,
			"contactPerson": company.ContactPerson,
			"contactPhone":  company.ContactInfo.WhatsApp,
			"phone":         company.ContactInfo.Phone,
			"email":         user.Email,
			"address":       company.ContactInfo.Address,
			"createdAt":     company.CreatedAt,
			"status":        company.CreationRequest,
			"subscription": map[string]interface{}{
				"planTitle":                planTitle,
				"planPrice":                planPrice,
				"planDuration":             planDuration,
				"planType":                 planType,
				"planBenefits":             planBenefits,
				"startDate":                subscriptionStartDate,
				"endDate":                  subscriptionEndDate,
				"subscriptionLevel":        subscriptionLevel,
				"remainingDays":            remainingDays,
				"totalActiveSubscriptions": totalActiveSubscriptions,
				"totalSubscriptionValue":   totalSubscriptionValue,
				"hasActiveSubscription":    hasActiveSubscription,
			},
		}

		enrichedCompanies = append(enrichedCompanies, enrichedCompany)
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Companies retrieved successfully",
		Data:    enrichedCompanies,
	})
}

// GetCompany retrieves a specific company
func (spc *SalesPersonController) GetCompany(c echo.Context) error {
	companyID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid company ID",
		})
	}

	var company models.Company
	err = spc.DB.Database("barrim").Collection("companies").FindOne(
		context.Background(),
		bson.M{"_id": companyID},
	).Decode(&company)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Company not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch company",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Company retrieved successfully",
		Data:    company,
	})
}

// UpdateCompany updates a specific company
func (spc *SalesPersonController) UpdateCompany(c echo.Context) error {
	companyID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid company ID",
		})
	}

	// Parse form data
	form, err := c.MultipartForm()
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid form data",
		})
	}

	// Handle additional emails and phones
	additionalEmails := []string{}
	if emails, ok := form.Value["additionalEmails"]; ok {
		for _, email := range emails {
			if strings.TrimSpace(email) != "" {
				additionalEmails = append(additionalEmails, strings.TrimSpace(email))
			}
		}
	}

	additionalPhones := []string{}
	if phones, ok := form.Value["additionalPhones"]; ok {
		for _, phone := range phones {
			if strings.TrimSpace(phone) != "" {
				additionalPhones = append(additionalPhones, strings.TrimSpace(phone))
			}
		}
	}

	// Get form values
	updates := bson.M{
		"businessName":     form.Value["businessName"][0],
		"category":         form.Value["category"][0],
		"subCategory":      form.Value["subcategory"][0],
		"additionalEmails": additionalEmails,
		"additionalPhones": additionalPhones,
		"contactInfo": bson.M{
			"phone":    form.Value["phone"][0],
			"whatsapp": form.Value["contactPhone"][0],
			"address": bson.M{
				"country":  form.Value["country"][0],
				"district": form.Value["district"][0],
				"city":     form.Value["city"][0],
				"street":   form.Value["street"][0],
			},
		},
		"updatedAt": time.Now(),
	}

	// Handle logo upload if provided
	if files, ok := form.File["logo"]; ok && len(files) > 0 {
		file := files[0]
		src, err := file.Open()
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to open uploaded file",
			})
		}
		defer src.Close()

		// Create uploads directory if it doesn't exist
		uploadDir := "uploads/logo"
		if err := os.MkdirAll(uploadDir, 0755); err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to create upload directory",
			})
		}

		// Generate unique filename
		filename := fmt.Sprintf("%d_%s", time.Now().UnixNano(), file.Filename)
		filepath := filepath.Join(uploadDir, filename)

		// Create destination file
		dst, err := os.Create(filepath)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to create destination file",
			})
		}
		defer dst.Close()

		// Copy file contents
		if _, err = io.Copy(dst, src); err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to save file",
			})
		}

		updates["logoUrl"] = filepath
	}

	// Handle profile picture upload if provided
	if files, ok := form.File["profilePic"]; ok && len(files) > 0 {
		file := files[0]
		src, err := file.Open()
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to open uploaded profile picture file",
			})
		}
		defer src.Close()

		// Create uploads directory if it doesn't exist
		uploadDir := "uploads/profiles"
		if err := os.MkdirAll(uploadDir, 0755); err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to create profile upload directory",
			})
		}

		// Generate unique filename
		filename := fmt.Sprintf("profile_%d_%s", time.Now().UnixNano(), file.Filename)
		filepath := filepath.Join(uploadDir, filename)

		// Create destination file
		dst, err := os.Create(filepath)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to create destination profile picture file",
			})
		}
		defer dst.Close()

		// Copy file contents
		if _, err = io.Copy(dst, src); err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to save profile picture file",
			})
		}

		updates["profilePicUrl"] = filepath
	}

	// Update company in database
	result, err := spc.DB.Database("barrim").Collection("companies").UpdateOne(
		context.Background(),
		bson.M{"_id": companyID},
		bson.M{"$set": updates},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update company",
		})
	}

	if result.MatchedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Company not found",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Company updated successfully",
	})
}

// DeleteCompany deletes a specific company
func (spc *SalesPersonController) DeleteCompany(c echo.Context) error {
	companyID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid company ID",
		})
	}

	// Delete company from database
	result, err := spc.DB.Database("barrim").Collection("companies").DeleteOne(
		context.Background(),
		bson.M{"_id": companyID},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to delete company",
		})
	}

	if result.DeletedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Company not found",
		})
	}

	// Delete associated user account
	_, err = spc.DB.Database("barrim").Collection("users").DeleteOne(
		context.Background(),
		bson.M{"companyId": companyID},
	)
	if err != nil {
		log.Printf("Failed to delete associated user account: %v", err)
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Company deleted successfully",
	})
}

// CreateWholesaler creates a new wholesaler
func (spc *SalesPersonController) CreateWholesaler(c echo.Context) error {
	// Get sales person ID from token
	claims := middleware.GetUserFromToken(c)
	salesPersonID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Invalid sales person ID",
		})
	}

	// Parse form data
	form, err := c.MultipartForm()
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid form data",
		})
	}

	// Helper function to safely get form values
	getFormValue := func(key string) (string, error) {
		if values, ok := form.Value[key]; ok && len(values) > 0 {
			return values[0], nil
		}
		return "", fmt.Errorf("missing required field: %s", key)
	}

	// Get and validate form values
	// Parse geographic coordinates if provided
	latStr, err := getFormValue("lat")
	var lat float64
	if err == nil {
		if latParsed, parseErr := strconv.ParseFloat(latStr, 64); parseErr == nil {
			lat = latParsed
		} else {
			log.Printf("Warning: Failed to parse lat value '%s': %v", latStr, parseErr)
		}
	} else {
		log.Printf("Warning: lat field not provided or empty")
	}

	lngStr, err := getFormValue("lng")
	var lng float64
	if err == nil {
		if lngParsed, parseErr := strconv.ParseFloat(lngStr, 64); parseErr == nil {
			lng = lngParsed
		} else {
			log.Printf("Warning: Failed to parse lng value '%s': %v", lngStr, parseErr)
		}
	} else {
		log.Printf("Warning: lng field not provided or empty")
	}
	businessName, err := getFormValue("businessName")
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: err.Error(),
		})
	}
	category, err := getFormValue("category")
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: err.Error(),
		})
	}
	subcategory, _ := getFormValue("subcategory") // Subcategory is optional
	email, _ := getFormValue("email")             // Email is optional
	phone, err := getFormValue("phone")
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: err.Error(),
		})
	}
	password, err := getFormValue("password")
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: err.Error(),
		})
	}
	contactPhone, err := getFormValue("contactPhone")
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: err.Error(),
		})
	}
	contactPerson, err := getFormValue("contactPerson")
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: err.Error(),
		})
	}
	country, err := getFormValue("country")
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: err.Error(),
		})
	}
	district, err := getFormValue("district")
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: err.Error(),
		})
	}
	city, err := getFormValue("city")
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: err.Error(),
		})
	}
	governorate, err := getFormValue("governorate")
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: err.Error(),
		})
	}

	// Handle logo upload
	var logoURL string
	if files, ok := form.File["logo"]; ok && len(files) > 0 {
		file := files[0]
		src, err := file.Open()
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to open uploaded file",
			})
		}
		defer src.Close()
		uploadDir := "uploads/logo"
		if err := os.MkdirAll(uploadDir, 0755); err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to create upload directory",
			})
		}
		filename := fmt.Sprintf("%d_%s", time.Now().UnixNano(), file.Filename)
		filepath := filepath.Join(uploadDir, filename)
		dst, err := os.Create(filepath)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to create destination file",
			})
		}
		defer dst.Close()
		if _, err = io.Copy(dst, src); err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to save file",
			})
		}
		logoURL = filepath
	}

	// Handle profile picture upload
	var profilePicURL string
	if files, ok := form.File["profilePic"]; ok && len(files) > 0 {
		file := files[0]
		log.Printf("Processing profile picture file: %s", file.Filename)

		src, err := file.Open()
		if err != nil {
			log.Printf("Error opening uploaded profile picture file: %v", err)
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to open uploaded profile picture file: " + err.Error(),
			})
		}
		defer src.Close()

		// Create uploads directory if it doesn't exist
		uploadDir := "uploads/profiles"
		if err := os.MkdirAll(uploadDir, 0755); err != nil {
			log.Printf("Error creating profile upload directory: %v", err)
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to create profile upload directory: " + err.Error(),
			})
		}

		// Generate unique filename
		filename := fmt.Sprintf("profile_%d_%s", time.Now().UnixNano(), file.Filename)
		filepath := filepath.Join(uploadDir, filename)
		log.Printf("Saving profile picture to: %s", filepath)

		// Create destination file
		dst, err := os.Create(filepath)
		if err != nil {
			log.Printf("Error creating destination profile picture file: %v", err)
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to create destination profile picture file: " + err.Error(),
			})
		}
		defer dst.Close()

		// Copy file contents
		if _, err = io.Copy(dst, src); err != nil {
			log.Printf("Error copying profile picture file contents: %v", err)
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to save profile picture file: " + err.Error(),
			})
		}

		profilePicURL = filepath
	}

	// Handle additional emails and phones
	additionalEmails := []string{}
	if emails, ok := form.Value["additionalEmails"]; ok {
		for _, email := range emails {
			if strings.TrimSpace(email) != "" {
				additionalEmails = append(additionalEmails, strings.TrimSpace(email))
			}
		}
	}

	additionalPhones := []string{}
	if phones, ok := form.Value["additionalPhones"]; ok {
		for _, phone := range phones {
			if strings.TrimSpace(phone) != "" {
				additionalPhones = append(additionalPhones, strings.TrimSpace(phone))
			}
		}
	}

	// Hash password for storage
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to hash password",
		})
	}

	// Generate referral code
	referralCode, err := utils.GenerateWholesalerReferralCode()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to generate referral code",
		})
	}

	// Create the main branch for the wholesaler
	wholesalerBranch := models.Branch{
		ID:   primitive.NewObjectID(),
		Name: businessName,
		Location: models.Address{
			Country:     country,
			District:    district,
			City:        city,
			Governorate: governorate,
			Lat:         lat,
			Lng:         lng,
		},
		Phone:       phone,
		Category:    category,
		SubCategory: subcategory, // Use the subcategory from form data
		Images:      []string{},
		Videos:      []string{},
		Status:      "inactive", // Set to inactive until subscription approval
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Add logo to branch images if provided
	if logoURL != "" {
		wholesalerBranch.Images = append(wholesalerBranch.Images, logoURL)
	}

	wholesaler := models.Wholesaler{
		ID:               primitive.NewObjectID(),
		UserID:           primitive.NewObjectID(),
		BusinessName:     businessName,
		Category:         category,
		SubCategory:      subcategory, // Set subcategory at wholesaler root level
		Phone:            phone,
		LogoURL:          logoURL,
		ProfilePicURL:    profilePicURL,
		ReferralCode:     referralCode,
		ContactPerson:    contactPerson,
		AdditionalPhones: additionalPhones,
		AdditionalEmails: additionalEmails,
		ContactInfo: models.ContactInfo{
			Phone:    phone,
			WhatsApp: contactPhone,
			Address: models.Address{
				Country:     country,
				District:    district,
				City:        city,
				Governorate: governorate,
				Lat:         lat,
				Lng:         lng,
			},
		},
		Branches:        []models.Branch{wholesalerBranch}, // Add the created branch
		CreatedBy:       salesPersonID,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		CreationRequest: "pending",
	}

	pendingRequest := models.PendingWholesalerRequest{
		ID:               primitive.NewObjectID(),
		Wholesaler:       wholesaler,
		Email:            email,
		AdditionalEmails: additionalEmails,
		Password:         string(hashedPassword),
		SalesPersonID:    salesPersonID,
		SalesManagerID:   primitive.NilObjectID, // Set later if needed
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	// Set correct SalesManagerID
	var salesperson models.Salesperson
	err = spc.DB.Database("barrim").Collection("salespersons").FindOne(context.Background(), bson.M{"_id": salesPersonID}).Decode(&salesperson)
	if err == nil {
		pendingRequest.SalesManagerID = salesperson.SalesManagerID
	}
	_, err = spc.DB.Database("barrim").Collection("pending_wholesaler_requests").InsertOne(context.Background(), pendingRequest)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to submit wholesaler for approval: " + err.Error(),
		})
	}
	// Notify sales manager (log error but do not fail request)
	if notifyErr := utils.NotifySalesManagerOfRequest(spc.DB, salesPersonID, "Wholesaler", businessName); notifyErr != nil {
		log.Printf("Failed to notify sales manager: %v", notifyErr)
	}
	return c.JSON(http.StatusCreated, models.Response{
		Status:  http.StatusCreated,
		Message: "Wholesaler submitted for approval",
		Data:    pendingRequest,
	})
}

// GetWholesalers retrieves all wholesalers created by the sales person
func (spc *SalesPersonController) GetWholesalers(c echo.Context) error {
	// Get sales person ID from token
	claims := middleware.GetUserFromToken(c)
	salesPersonID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Invalid sales person ID",
		})
	}

	// Find all wholesalers created by this sales person
	var wholesalers []models.Wholesaler
	cursor, err := spc.DB.Database("barrim").Collection("wholesalers").Find(
		context.Background(),
		bson.M{"createdBy": salesPersonID},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch wholesalers",
		})
	}
	defer cursor.Close(context.Background())

	if err := cursor.All(context.Background(), &wholesalers); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode wholesalers",
		})
	}

	// Get user information for contact details
	db := spc.DB.Database("barrim")
	userIDs := make([]primitive.ObjectID, 0, len(wholesalers))
	for _, wholesaler := range wholesalers {
		userIDs = append(userIDs, wholesaler.UserID)
	}

	userMap := make(map[string]models.User)
	if len(userIDs) > 0 {
		userCursor, err := db.Collection("users").Find(
			context.Background(),
			bson.M{"_id": bson.M{"$in": userIDs}},
		)
		if err == nil {
			var users []models.User
			_ = userCursor.All(context.Background(), &users)
			userCursor.Close(context.Background())
			for _, user := range users {
				userMap[user.ID.Hex()] = user
			}
		}
	}

	// Get subscription information for each wholesaler
	var enrichedWholesalers []map[string]interface{}
	for _, wholesaler := range wholesalers {
		user := userMap[wholesaler.UserID.Hex()]

		// Get active subscription for this wholesaler
		var planTitle string
		var planPrice float64
		var planDuration int
		var planType string
		var planBenefits interface{}
		var subscriptionStartDate time.Time
		var subscriptionEndDate time.Time
		var subscriptionLevel string
		var remainingDays int
		var hasActiveSubscription bool
		var totalActiveSubscriptions int
		var totalSubscriptionValue float64

		// First check for wholesaler-level subscriptions
		var wholesalerSubscription models.WholesalerSubscription
		err := db.Collection("wholesaler_subscriptions").FindOne(
			context.Background(),
			bson.M{
				"wholesalerId": wholesaler.ID,
				"status":       "active",
				"endDate":      bson.M{"$gt": time.Now()},
			},
		).Decode(&wholesalerSubscription)

		// Count total active wholesaler subscriptions
		if err == nil {
			totalActiveSubscriptions++
		}

		if err == nil {
			// Get plan details for wholesaler subscription
			var plan models.SubscriptionPlan
			err = db.Collection("subscription_plans").FindOne(
				context.Background(),
				bson.M{"_id": wholesalerSubscription.PlanID},
			).Decode(&plan)

			if err == nil {
				planTitle = plan.Title
				planPrice = plan.Price
				planDuration = plan.Duration
				planType = plan.Type
				planBenefits = plan.Benefits
				subscriptionStartDate = wholesalerSubscription.StartDate
				subscriptionEndDate = wholesalerSubscription.EndDate
				subscriptionLevel = "wholesaler"
				hasActiveSubscription = true
				// Calculate remaining days
				remainingDays = int(time.Until(wholesalerSubscription.EndDate).Hours() / 24)
				// Add to total subscription value
				totalSubscriptionValue += planPrice
			}
		}

		// If no wholesaler subscription found, check for branch subscriptions
		// Also check if there are multiple active subscriptions for better reporting
		if !hasActiveSubscription && len(wholesaler.Branches) > 0 {
			// Get all branch IDs for this wholesaler
			branchIDs := make([]primitive.ObjectID, 0, len(wholesaler.Branches))
			for _, branch := range wholesaler.Branches {
				branchIDs = append(branchIDs, branch.ID)
			}

			if len(branchIDs) > 0 {
				// Check for active branch subscriptions
				var branchSubscription models.WholesalerBranchSubscription
				err := db.Collection("wholesaler_branch_subscriptions").FindOne(
					context.Background(),
					bson.M{
						"branchId": bson.M{"$in": branchIDs},
						"status":   "active",
						"endDate":  bson.M{"$gt": time.Now()},
					},
				).Decode(&branchSubscription)

				// Count total active branch subscriptions
				if err == nil {
					totalActiveSubscriptions++
				}

				if err == nil {
					// Get plan details for branch subscription
					var plan models.SubscriptionPlan
					err = db.Collection("subscription_plans").FindOne(
						context.Background(),
						bson.M{"_id": branchSubscription.PlanID},
					).Decode(&plan)

					if err == nil {
						planTitle = plan.Title
						planPrice = plan.Price
						planDuration = plan.Duration
						planType = plan.Type
						planBenefits = plan.Benefits
						subscriptionStartDate = branchSubscription.StartDate
						subscriptionEndDate = branchSubscription.EndDate
						subscriptionLevel = "branch"
						hasActiveSubscription = true
						// Calculate remaining days
						remainingDays = int(time.Until(branchSubscription.EndDate).Hours() / 24)
						// Add to total subscription value
						totalSubscriptionValue += planPrice
					}
				}
			}
		}

		enrichedWholesaler := map[string]interface{}{
			"id":            wholesaler.ID,
			"businessName":  wholesaler.BusinessName,
			"category":      wholesaler.Category,
			"subCategory":   wholesaler.SubCategory, // Add subcategory to response
			"logoURL":       wholesaler.LogoURL,
			"contactPerson": wholesaler.ContactPerson,
			"contactPhone":  wholesaler.ContactInfo.WhatsApp,
			"phone":         wholesaler.ContactInfo.Phone,
			"email":         user.Email,
			"address":       wholesaler.ContactInfo.Address,
			"createdAt":     wholesaler.CreatedAt,
			"status":        wholesaler.CreationRequest,
			"subscription": map[string]interface{}{
				"planTitle":                planTitle,
				"planPrice":                planPrice,
				"planDuration":             planDuration,
				"planType":                 planType,
				"planBenefits":             planBenefits,
				"startDate":                subscriptionStartDate,
				"endDate":                  subscriptionEndDate,
				"subscriptionLevel":        subscriptionLevel,
				"remainingDays":            remainingDays,
				"totalActiveSubscriptions": totalActiveSubscriptions,
				"totalSubscriptionValue":   totalSubscriptionValue,
				"hasActiveSubscription":    hasActiveSubscription,
			},
		}

		enrichedWholesalers = append(enrichedWholesalers, enrichedWholesaler)
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Wholesalers retrieved successfully",
		Data:    enrichedWholesalers,
	})
}

// GetWholesaler retrieves a specific wholesaler
func (spc *SalesPersonController) GetWholesaler(c echo.Context) error {
	wholesalerID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid wholesaler ID",
		})
	}

	var wholesaler models.Wholesaler
	err = spc.DB.Database("barrim").Collection("wholesalers").FindOne(
		context.Background(),
		bson.M{"_id": wholesalerID},
	).Decode(&wholesaler)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Wholesaler not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch wholesaler",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Wholesaler retrieved successfully",
		Data:    wholesaler,
	})
}

// UpdateWholesaler updates a specific wholesaler
func (spc *SalesPersonController) UpdateWholesaler(c echo.Context) error {
	wholesalerID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid wholesaler ID",
		})
	}

	// Parse form data
	form, err := c.MultipartForm()
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid form data",
		})
	}

	// Get form values
	updates := bson.M{
		"businessName": form.Value["businessName"][0],
		"category":     form.Value["category"][0],
		"subCategory":  form.Value["subcategory"][0], // Add subcategory support
		"contactInfo": bson.M{
			"phone":    form.Value["phone"][0],
			"whatsapp": form.Value["contactPhone"][0],
			"address": bson.M{
				"country":  form.Value["country"][0],
				"district": form.Value["district"][0],
				"city":     form.Value["city"][0],
				"street":   form.Value["street"][0],
			},
		},
		"updatedAt": time.Now(),
	}

	// Handle logo upload if provided
	if files, ok := form.File["logo"]; ok && len(files) > 0 {
		file := files[0]
		src, err := file.Open()
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to open uploaded file",
			})
		}
		defer src.Close()

		// Create uploads directory if it doesn't exist
		uploadDir := "uploads/logo"
		if err := os.MkdirAll(uploadDir, 0755); err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to create upload directory",
			})
		}

		// Generate unique filename
		filename := fmt.Sprintf("%d_%s", time.Now().UnixNano(), file.Filename)
		filepath := filepath.Join(uploadDir, filename)

		// Create destination file
		dst, err := os.Create(filepath)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to create destination file",
			})
		}
		defer dst.Close()

		// Copy file contents
		if _, err = io.Copy(dst, src); err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to save file",
			})
		}

		updates["logoUrl"] = filepath
	}

	// Update wholesaler in database
	result, err := spc.DB.Database("barrim").Collection("wholesalers").UpdateOne(
		context.Background(),
		bson.M{"_id": wholesalerID},
		bson.M{"$set": updates},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update wholesaler",
		})
	}

	if result.MatchedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Wholesaler not found",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Wholesaler updated successfully",
	})
}

// DeleteWholesaler deletes a specific wholesaler
func (spc *SalesPersonController) DeleteWholesaler(c echo.Context) error {
	wholesalerID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid wholesaler ID",
		})
	}

	// Delete wholesaler from database
	result, err := spc.DB.Database("barrim").Collection("wholesalers").DeleteOne(
		context.Background(),
		bson.M{"_id": wholesalerID},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to delete wholesaler",
		})
	}

	if result.DeletedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Wholesaler not found",
		})
	}

	// Delete associated user account
	_, err = spc.DB.Database("barrim").Collection("users").DeleteOne(
		context.Background(),
		bson.M{"wholesalerId": wholesalerID},
	)
	if err != nil {
		log.Printf("Failed to delete associated user account: %v", err)
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Wholesaler deleted successfully",
	})
}

// CreateServiceProvider creates a new service provider
func (spc *SalesPersonController) CreateServiceProvider(c echo.Context) error {
	// Get sales person ID from token
	claims := middleware.GetUserFromToken(c)
	salesPersonID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Invalid sales person ID",
		})
	}

	// Parse form data
	form, err := c.MultipartForm()
	if err != nil {
		log.Printf("Error parsing multipart form: %v", err)
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid form data: " + err.Error(),
		})
	}

	// Helper function to safely get form values
	getFormValue := func(key string) (string, error) {
		if values, ok := form.Value[key]; ok && len(values) > 0 {
			return values[0], nil
		}
		return "", fmt.Errorf("missing required field: %s", key)
	}

	// Get and validate form values
	// Parse geographic coordinates if provided
	latStr, err := getFormValue("lat")
	var lat float64
	if err == nil {
		if latParsed, parseErr := strconv.ParseFloat(latStr, 64); parseErr == nil {
			lat = latParsed
		} else {
			log.Printf("Warning: Failed to parse lat value '%s': %v", latStr, parseErr)
		}
	} else {
		log.Printf("Warning: lat field not provided or empty")
	}

	lngStr, err := getFormValue("lng")
	var lng float64
	if err == nil {
		if lngParsed, parseErr := strconv.ParseFloat(lngStr, 64); parseErr == nil {
			lng = lngParsed
		} else {
			log.Printf("Warning: Failed to parse lng value '%s': %v", lngStr, parseErr)
		}
	} else {
		log.Printf("Warning: lng field not provided or empty")
	}
	businessName, err := getFormValue("businessName")
	if err != nil {
		log.Printf("Error getting businessName: %v", err)
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: err.Error(),
		})
	}

	category, err := getFormValue("category")
	if err != nil {
		log.Printf("Error getting category: %v", err)
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: err.Error(),
		})
	}

	email, _ := getFormValue("email") // Email is optional
	log.Printf("DEBUG: Email value from form: '%s' (length: %d)", email, len(email))

	phone, err := getFormValue("phone")
	if err != nil {
		log.Printf("Error getting phone: %v", err)
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: err.Error(),
		})
	}

	password, err := getFormValue("password")
	if err != nil {
		log.Printf("Error getting password: %v", err)
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: err.Error(),
		})
	}

	contactPhone, err := getFormValue("contactPhone")
	if err != nil {
		log.Printf("Error getting contactPhone: %v", err)
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: err.Error(),
		})
	}

	contactPerson, err := getFormValue("contactPerson")
	if err != nil {
		log.Printf("Error getting contactPerson: %v", err)
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: err.Error(),
		})
	}

	country, err := getFormValue("country")
	if err != nil {
		log.Printf("Error getting country: %v", err)
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: err.Error(),
		})
	}
	district, err := getFormValue("district")
	if err != nil {
		log.Printf("Error getting district: %v", err)
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: err.Error(),
		})
	}
	city, err := getFormValue("city")
	if err != nil {
		log.Printf("Error getting city: %v", err)
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: err.Error(),
		})
	}
	governorate, err := getFormValue("governorate")
	if err != nil {
		log.Printf("Error getting street: %v", err)
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: err.Error(),
		})
	}

	// Handle logo upload
	var logoURL string
	if files, ok := form.File["logo"]; ok && len(files) > 0 {
		file := files[0]
		log.Printf("Processing logo file: %s", file.Filename)

		src, err := file.Open()
		if err != nil {
			log.Printf("Error opening uploaded file: %v", err)
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to open uploaded file: " + err.Error(),
			})
		}
		defer src.Close()

		// Create uploads directory if it doesn't exist
		uploadDir := "uploads/logo"
		if err := os.MkdirAll(uploadDir, 0755); err != nil {
			log.Printf("Error creating upload directory: %v", err)
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to create upload directory: " + err.Error(),
			})
		}

		// Generate unique filename
		filename := fmt.Sprintf("%d_%s", time.Now().UnixNano(), file.Filename)
		filepath := filepath.Join(uploadDir, filename)
		log.Printf("Saving logo to: %s", filepath)

		// Create destination file
		dst, err := os.Create(filepath)
		if err != nil {
			log.Printf("Error creating destination file: %v", err)
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to create destination file: " + err.Error(),
			})
		}
		defer dst.Close()

		// Copy file contents
		if _, err = io.Copy(dst, src); err != nil {
			log.Printf("Error copying file contents: %v", err)
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to save file: " + err.Error(),
			})
		}

		logoURL = filepath
	}

	// Handle profile picture upload
	var profilePicURL string
	if files, ok := form.File["profilePic"]; ok && len(files) > 0 {
		file := files[0]
		log.Printf("Processing profile picture file: %s", file.Filename)

		src, err := file.Open()
		if err != nil {
			log.Printf("Error opening uploaded profile picture file: %v", err)
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to open uploaded profile picture file: " + err.Error(),
			})
		}
		defer src.Close()

		// Create uploads directory if it doesn't exist
		uploadDir := "uploads/profiles"
		if err := os.MkdirAll(uploadDir, 0755); err != nil {
			log.Printf("Error creating profile upload directory: %v", err)
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to create profile upload directory: " + err.Error(),
			})
		}

		// Generate unique filename
		filename := fmt.Sprintf("profile_%d_%s", time.Now().UnixNano(), file.Filename)
		filepath := filepath.Join(uploadDir, filename)
		log.Printf("Saving profile picture to: %s", filepath)

		// Create destination file
		dst, err := os.Create(filepath)
		if err != nil {
			log.Printf("Error creating destination profile picture file: %v", err)
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to create destination profile picture file: " + err.Error(),
			})
		}
		defer dst.Close()

		// Copy file contents
		if _, err = io.Copy(dst, src); err != nil {
			log.Printf("Error copying profile picture file contents: %v", err)
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to save profile picture file: " + err.Error(),
			})
		}

		profilePicURL = filepath
	}

	// Handle additional emails and phones
	additionalEmails := []string{}
	if emails, ok := form.Value["additionalEmails"]; ok {
		for _, email := range emails {
			if strings.TrimSpace(email) != "" {
				additionalEmails = append(additionalEmails, strings.TrimSpace(email))
			}
		}
	}

	additionalPhones := []string{}
	if phones, ok := form.Value["additionalPhones"]; ok {
		for _, phone := range phones {
			if strings.TrimSpace(phone) != "" {
				additionalPhones = append(additionalPhones, strings.TrimSpace(phone))
			}
		}
	}

	// Hash password (store in pending request for later use)
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("Error hashing password: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to hash password: " + err.Error(),
		})
	}

	// Generate referral code
	referralCode, err := utils.GenerateServiceProviderReferralCode()
	if err != nil {
		log.Printf("Error generating referral code: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to generate referral code",
		})
	}

	// Create service provider object in flat format
	serviceProviderID := primitive.NewObjectID()
	userID := primitive.NewObjectID()

	serviceProvider := models.ServiceProvider{
		ID:               serviceProviderID,
		UserID:           userID,
		BusinessName:     businessName,
		Category:         category,
		Email:            email,
		Phone:            phone,
		Password:         string(hashedPassword),
		ContactPerson:    contactPerson,
		ContactPhone:     contactPhone,
		Country:          country,
		District:         district,
		City:             city,
		Governorate:      governorate,
		LogoURL:          logoURL,
		ProfilePicURL:    profilePicURL,
		AdditionalPhones: additionalPhones,
		AdditionalEmails: additionalEmails,
		ContactInfo: models.ContactInfo{
			Phone:    phone,
			WhatsApp: contactPhone,
			Address: models.Address{
				Country:     country,
				District:    district,
				City:        city,
				Governorate: governorate,
				Lat:         lat,
				Lng:         lng,
			},
		},
		ReferralCode:    referralCode,
		CreatedBy:       salesPersonID,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		Status:          "inactive",
		CreationRequest: "pending",
	}

	// Insert service provider into serviceProviders collection
	_, err = spc.DB.Database("barrim").Collection("serviceProviders").InsertOne(context.Background(), serviceProvider)
	if err != nil {
		log.Printf("Error inserting service provider: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to create service provider: " + err.Error(),
		})
	}

	// Create user account for the service provider (using email if provided, otherwise phone)
	var userEmail string
	if email != "" {
		userEmail = email
		log.Printf("DEBUG: Creating user with email: %s", email)
	} else {
		// Use phone number as email when email is not provided
		userEmail = phone + "@serviceprovider.local"
		log.Printf("DEBUG: No email provided, using phone as email: %s", userEmail)
	}

	user := models.User{
		ID:                userID,
		Email:             userEmail,
		Password:          string(hashedPassword),
		FullName:          businessName,
		UserType:          "serviceProvider",
		Phone:             phone,
		ContactPerson:     contactPerson,
		ContactPhone:      contactPhone,
		ServiceProviderID: &serviceProviderID,
		IsActive:          true,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	// Insert user into users collection
	log.Printf("DEBUG: Attempting to insert user with email: %s, userID: %s", userEmail, userID.Hex())
	_, err = spc.DB.Database("barrim").Collection("users").InsertOne(context.Background(), user)
	if err != nil {
		log.Printf("ERROR inserting user: %v", err)
		// If user creation fails, clean up the service provider
		spc.DB.Database("barrim").Collection("serviceProviders").DeleteOne(context.Background(), bson.M{"_id": serviceProviderID})
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to create user account: " + err.Error(),
		})
	}
	log.Printf("SUCCESS: Created user for service provider with email: %s, userID: %s", userEmail, userID.Hex())

	pendingRequest := models.PendingServiceProviderRequest{
		ID:                    primitive.NewObjectID(),
		ServiceProvider:       serviceProvider,
		Email:                 email,
		AdditionalEmails:      additionalEmails,
		Password:              string(hashedPassword),
		CreationRequestStatus: "pending",
		SalesPersonID:         salesPersonID,
		SalesManagerID:        primitive.NilObjectID, // Set later if needed
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}
	// Set correct SalesManagerID
	var salesperson models.Salesperson
	err = spc.DB.Database("barrim").Collection("salespersons").FindOne(context.Background(), bson.M{"_id": salesPersonID}).Decode(&salesperson)
	if err == nil {
		pendingRequest.SalesManagerID = salesperson.SalesManagerID
	}
	_, err = spc.DB.Database("barrim").Collection("pending_serviceProviders_requests").InsertOne(context.Background(), pendingRequest)
	if err != nil {
		log.Printf("Error inserting pending service provider request: %v", err)
		// If pending request creation fails, clean up the service provider and user
		spc.DB.Database("barrim").Collection("serviceProviders").DeleteOne(context.Background(), bson.M{"_id": serviceProviderID})
		spc.DB.Database("barrim").Collection("users").DeleteOne(context.Background(), bson.M{"_id": userID})
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to submit service provider for approval: " + err.Error(),
		})
	}
	// Notify sales manager (log error but do not fail request)
	if notifyErr := utils.NotifySalesManagerOfRequest(spc.DB, salesPersonID, "Service Provider", businessName); notifyErr != nil {
		log.Printf("Failed to notify sales manager: %v", notifyErr)
	}
	return c.JSON(http.StatusCreated, models.Response{
		Status:  http.StatusCreated,
		Message: "Service provider submitted for approval",
		Data:    pendingRequest,
	})
}

// GetAllCreatedUsers retrieves all companies, service providers, and wholesalers created by the salesperson, or all if admin, manager, or sales_manager, and includes salesperson info for each entity
func (spc *SalesPersonController) GetAllCreatedUsers(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	db := spc.DB.Database("barrim")

	if claims.UserType == "admin" || claims.UserType == "manager" || claims.UserType == "sales_manager" {
		// Fetch all companies, service providers, and wholesalers created by any salesperson
		companyCursor, err := db.Collection("companies").Find(
			context.Background(),
			bson.M{"createdBy": bson.M{"$ne": nil}},
		)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to fetch companies: " + err.Error(),
			})
		}
		var companies []models.Company
		_ = companyCursor.All(context.Background(), &companies)
		companyCursor.Close(context.Background())

		spCursor, err := db.Collection("serviceProviders").Find(
			context.Background(),
			bson.M{"createdBy": bson.M{"$ne": nil}},
		)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to fetch service providers: " + err.Error(),
			})
		}
		var serviceProviders []models.ServiceProvider
		_ = spCursor.All(context.Background(), &serviceProviders)
		spCursor.Close(context.Background())

		whCursor, err := db.Collection("wholesalers").Find(
			context.Background(),
			bson.M{"createdBy": bson.M{"$ne": nil}},
		)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to fetch wholesalers: " + err.Error(),
			})
		}
		var wholesalers []models.Wholesaler
		_ = whCursor.All(context.Background(), &wholesalers)
		whCursor.Close(context.Background())

		// Collect all unique createdBy IDs
		idSet := make(map[string]struct{})
		for _, c := range companies {
			idSet[c.CreatedBy.Hex()] = struct{}{}
		}
		for _, s := range serviceProviders {
			idSet[s.CreatedBy.Hex()] = struct{}{}
		}
		for _, w := range wholesalers {
			idSet[w.CreatedBy.Hex()] = struct{}{}
		}
		var ids []primitive.ObjectID
		for id := range idSet {
			objID, err := primitive.ObjectIDFromHex(id)
			if err == nil {
				ids = append(ids, objID)
			}
		}

		// Query salespersons collection for all these IDs
		personCursor, err := db.Collection("salespersons").Find(
			context.Background(),
			bson.M{"_id": bson.M{"$in": ids}},
		)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to fetch salespersons: " + err.Error(),
			})
		}
		var salespersons []models.Salesperson
		_ = personCursor.All(context.Background(), &salespersons)
		personCursor.Close(context.Background())
		// Build map: salespersonID -> {fullName, email, logo}
		spMap := make(map[string]map[string]string)
		for _, sp := range salespersons {
			spMap[sp.ID.Hex()] = map[string]string{
				"fullName": sp.FullName,
				"email":    sp.Email,
				"logo":     sp.Image, // 'Image' field is used for logo/profile picture
			}
		}

		// Enrich companies
		enrichedCompanies := make([]map[string]interface{}, 0, len(companies))
		for _, c := range companies {
			spInfo := spMap[c.CreatedBy.Hex()]
			enrichedCompanies = append(enrichedCompanies, map[string]interface{}{
				"company":     c,
				"salesperson": spInfo,
			})
		}
		// Enrich service providers
		enrichedSPs := make([]map[string]interface{}, 0, len(serviceProviders))
		for _, s := range serviceProviders {
			spInfo := spMap[s.CreatedBy.Hex()]
			enrichedSPs = append(enrichedSPs, map[string]interface{}{
				"serviceProvider": s,
				"salesperson":     spInfo,
			})
		}
		// Enrich wholesalers
		enrichedWholesalers := make([]map[string]interface{}, 0, len(wholesalers))
		for _, w := range wholesalers {
			spInfo := spMap[w.CreatedBy.Hex()]
			enrichedWholesalers = append(enrichedWholesalers, map[string]interface{}{
				"wholesaler":  w,
				"salesperson": spInfo,
			})
		}

		return c.JSON(http.StatusOK, models.Response{
			Status:  http.StatusOK,
			Message: "All users created by salespersons retrieved successfully",
			Data: map[string]interface{}{
				"userName":         claims.Email,
				"companies":        enrichedCompanies,
				"serviceProviders": enrichedSPs,
				"wholesalers":      enrichedWholesalers,
			},
		})
	}

	salesPersonID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Invalid user ID: " + err.Error(),
		})
	}

	userType := claims.UserType
	var displayName string

	switch userType {
	case "salesperson":
		var salesperson models.Salesperson
		err = db.Collection("salespersons").FindOne(
			context.Background(),
			bson.M{"_id": salesPersonID},
		).Decode(&salesperson)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to fetch salesperson info: " + err.Error(),
			})
		}
		displayName = salesperson.FullName
	case "salesManager":
		var salesManager models.SalesManager
		err = db.Collection("salesmanagers").FindOne(
			context.Background(),
			bson.M{"_id": salesPersonID},
		).Decode(&salesManager)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to fetch sales manager info: " + err.Error(),
			})
		}
		displayName = salesManager.FullName
	case "manager":
		var manager models.Manager
		err = db.Collection("managers").FindOne(
			context.Background(),
			bson.M{"_id": salesPersonID},
		).Decode(&manager)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to fetch manager info: " + err.Error(),
			})
		}
		displayName = manager.FullName
	case "admin":
		var admin models.Admin
		err = db.Collection("admins").FindOne(
			context.Background(),
			bson.M{"_id": salesPersonID},
		).Decode(&admin)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to fetch admin info: " + err.Error(),
			})
		}
		displayName = admin.Email // No name field, use email
	default:
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "User type not allowed",
		})
	}

	// Fetch companies
	var companies []models.Company
	companyCursor, err := db.Collection("companies").Find(
		context.Background(),
		bson.M{"createdBy": salesPersonID},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch companies: " + err.Error(),
		})
	}
	_ = companyCursor.All(context.Background(), &companies)
	companyCursor.Close(context.Background())

	// Fetch service providers
	var serviceProviders []models.ServiceProvider
	spCursor, err := db.Collection("serviceProviders").Find(
		context.Background(),
		bson.M{"createdBy": salesPersonID},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch service providers: " + err.Error(),
		})
	}
	_ = spCursor.All(context.Background(), &serviceProviders)
	spCursor.Close(context.Background())

	// Fetch wholesalers
	var wholesalers []models.Wholesaler
	whCursor, err := db.Collection("wholesalers").Find(
		context.Background(),
		bson.M{"createdBy": salesPersonID},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch wholesalers: " + err.Error(),
		})
	}
	_ = whCursor.All(context.Background(), &wholesalers)
	whCursor.Close(context.Background())

	// Fetch user info for each entity (companies, serviceProviders, wholesalers)
	userIDs := make([]primitive.ObjectID, 0)
	for _, c := range companies {
		userIDs = append(userIDs, c.UserID)
	}
	for _, s := range serviceProviders {
		userIDs = append(userIDs, s.UserID)
	}
	for _, w := range wholesalers {
		userIDs = append(userIDs, w.UserID)
	}
	userMap := make(map[string]map[string]string)
	if len(userIDs) > 0 {
		userCursor, err := db.Collection("users").Find(
			context.Background(),
			bson.M{"_id": bson.M{"$in": userIDs}},
		)
		if err == nil {
			var users []models.User
			_ = userCursor.All(context.Background(), &users)
			userCursor.Close(context.Background())
			for _, u := range users {
				userMap[u.ID.Hex()] = map[string]string{
					"email": u.Email,
					"logo":  u.LogoPath, // or u.ProfilePic if that's the field
				}
			}
		}
	}
	// Enrich companies
	enrichedCompanies := make([]map[string]interface{}, 0, len(companies))
	for _, c := range companies {
		userInfo := userMap[c.UserID.Hex()]
		enrichedCompanies = append(enrichedCompanies, map[string]interface{}{
			"company": c,
			"user":    userInfo,
		})
	}
	// Enrich service providers
	enrichedSPs := make([]map[string]interface{}, 0, len(serviceProviders))
	for _, s := range serviceProviders {
		userInfo := userMap[s.UserID.Hex()]
		enrichedSPs = append(enrichedSPs, map[string]interface{}{
			"serviceProvider": s,
			"user":            userInfo,
		})
	}
	// Enrich wholesalers
	enrichedWholesalers := make([]map[string]interface{}, 0, len(wholesalers))
	for _, w := range wholesalers {
		userInfo := userMap[w.UserID.Hex()]
		enrichedWholesalers = append(enrichedWholesalers, map[string]interface{}{
			"wholesaler": w,
			"user":       userInfo,
		})
	}
	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Users created by user retrieved successfully",
		Data: map[string]interface{}{
			"userName":         displayName,
			"companies":        enrichedCompanies,
			"serviceProviders": enrichedSPs,
			"wholesalers":      enrichedWholesalers,
		},
	})
}

// GetSalespersonCreatedUsersWithCommission returns companies, wholesalers, and service providers created by the salesperson with businessName, email, contact info, logo, subscription plan, and calculated commission
func (spc *SalesPersonController) GetSalespersonCreatedUsersWithCommission(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "salesperson" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only salespersons can access this endpoint",
		})
	}
	db := spc.DB.Database("barrim")
	salesPersonID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Invalid user ID: " + err.Error(),
		})
	}

	// Get salesperson commission percent
	var salesperson models.Salesperson
	err = db.Collection("salespersons").FindOne(context.Background(), bson.M{"_id": salesPersonID}).Decode(&salesperson)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch salesperson info: " + err.Error(),
		})
	}

	// --- COMPANIES ---
	companyCursor, err := db.Collection("companies").Find(
		context.Background(),
		bson.M{"createdBy": salesPersonID},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch companies: " + err.Error(),
		})
	}
	var companies []models.Company
	_ = companyCursor.All(context.Background(), &companies)
	companyCursor.Close(context.Background())

	// --- WHOLESALERS ---
	wholesalerCursor, err := db.Collection("wholesalers").Find(
		context.Background(),
		bson.M{"createdBy": salesPersonID},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch wholesalers: " + err.Error(),
		})
	}
	var wholesalers []models.Wholesaler
	_ = wholesalerCursor.All(context.Background(), &wholesalers)
	wholesalerCursor.Close(context.Background())

	// --- SERVICE PROVIDERS ---
	spCursor, err := db.Collection("serviceProviders").Find(
		context.Background(),
		bson.M{"createdBy": salesPersonID},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch service providers: " + err.Error(),
		})
	}
	var serviceProviders []models.ServiceProvider
	_ = spCursor.All(context.Background(), &serviceProviders)
	spCursor.Close(context.Background())

	// --- USER INFO FOR ALL ---
	userIDs := make([]primitive.ObjectID, 0, len(companies)+len(wholesalers)+len(serviceProviders))
	for _, c := range companies {
		userIDs = append(userIDs, c.UserID)
	}
	for _, w := range wholesalers {
		userIDs = append(userIDs, w.UserID)
	}
	for _, s := range serviceProviders {
		userIDs = append(userIDs, s.UserID)
	}
	userMap := make(map[string]models.User)
	if len(userIDs) > 0 {
		userCursor, err := db.Collection("users").Find(
			context.Background(),
			bson.M{"_id": bson.M{"$in": userIDs}},
		)
		if err == nil {
			var users []models.User
			_ = userCursor.All(context.Background(), &users)
			userCursor.Close(context.Background())
			for _, u := range users {
				userMap[u.ID.Hex()] = u
			}
		}
	}

	// --- COMPANY SUBSCRIPTIONS ---
	companySubsCursor, err := db.Collection("company_subscriptions").Find(
		context.Background(),
		bson.M{"companyId": bson.M{"$in": userIDs}, "status": "active"},
	)
	var companySubs []struct {
		ID        primitive.ObjectID `bson:"_id"`
		CompanyID primitive.ObjectID `bson:"companyId"`
		PlanID    primitive.ObjectID `bson:"planId"`
	}
	if err == nil {
		_ = companySubsCursor.All(context.Background(), &companySubs)
		companySubsCursor.Close(context.Background())
	}
	companyPlanMap := make(map[string]primitive.ObjectID)
	for _, sub := range companySubs {
		companyPlanMap[sub.CompanyID.Hex()] = sub.PlanID
	}

	// --- WHOLESALER SUBSCRIPTIONS ---
	wholesalerSubsCursor, err := db.Collection("wholesaler_subscriptions").Find(
		context.Background(),
		bson.M{"wholesalerId": bson.M{"$in": userIDs}, "status": "active"},
	)
	var wholesalerSubs []struct {
		ID           primitive.ObjectID `bson:"_id"`
		WholesalerID primitive.ObjectID `bson:"wholesalerId"`
		PlanID       primitive.ObjectID `bson:"planId"`
	}
	if err == nil {
		_ = wholesalerSubsCursor.All(context.Background(), &wholesalerSubs)
		wholesalerSubsCursor.Close(context.Background())
	}
	wholesalerPlanMap := make(map[string]primitive.ObjectID)
	for _, sub := range wholesalerSubs {
		wholesalerPlanMap[sub.WholesalerID.Hex()] = sub.PlanID
	}

	// --- SERVICE PROVIDER SUBSCRIPTIONS ---
	spSubsCursor, err := db.Collection("service_provider_subscriptions").Find(
		context.Background(),
		bson.M{"serviceProviderId": bson.M{"$in": userIDs}, "status": "active"},
	)
	var spSubs []struct {
		ID                primitive.ObjectID `bson:"_id"`
		ServiceProviderID primitive.ObjectID `bson:"serviceProviderId"`
		PlanID            primitive.ObjectID `bson:"planId"`
	}
	if err == nil {
		_ = spSubsCursor.All(context.Background(), &spSubs)
		spSubsCursor.Close(context.Background())
	}
	spPlanMap := make(map[string]primitive.ObjectID)
	for _, sub := range spSubs {
		spPlanMap[sub.ServiceProviderID.Hex()] = sub.PlanID
	}

	// --- FETCH ALL PLANS ---
	planIDsSet := make(map[string]struct{})
	for _, pid := range companyPlanMap {
		planIDsSet[pid.Hex()] = struct{}{}
	}
	for _, pid := range wholesalerPlanMap {
		planIDsSet[pid.Hex()] = struct{}{}
	}
	for _, pid := range spPlanMap {
		planIDsSet[pid.Hex()] = struct{}{}
	}
	planIDs := make([]primitive.ObjectID, 0, len(planIDsSet))
	for pid := range planIDsSet {
		objID, err := primitive.ObjectIDFromHex(pid)
		if err == nil {
			planIDs = append(planIDs, objID)
		}
	}
	planMap := make(map[string]models.SubscriptionPlan)
	if len(planIDs) > 0 {
		planCursor, err := db.Collection("subscription_plans").Find(
			context.Background(),
			bson.M{"_id": bson.M{"$in": planIDs}},
		)
		if err == nil {
			var plans []models.SubscriptionPlan
			_ = planCursor.All(context.Background(), &plans)
			planCursor.Close(context.Background())
			for _, p := range plans {
				planMap[p.ID.Hex()] = p
			}
		}
	}

	// --- BUILD RESPONSE ---
	var companyResult []map[string]interface{}
	for _, c := range companies {
		user := userMap[c.UserID.Hex()]
		planID, hasPlan := companyPlanMap[c.UserID.Hex()]
		var planTitle string
		var planPrice float64
		var commission float64
		if hasPlan {
			plan := planMap[planID.Hex()]
			planTitle = plan.Title
			planPrice = plan.Price
			commission = planPrice * salesperson.CommissionPercent / 100.0
		}
		companyResult = append(companyResult, map[string]interface{}{
			"businessName":  c.BusinessName,
			"email":         user.Email,
			"contactPerson": c.ContactInfo.Phone,    // If you have a separate contactPerson field, use it
			"contactPhone":  c.ContactInfo.WhatsApp, // If you have a separate contactPhone field, use it
			"logo":          user.LogoPath,
			"planTitle":     planTitle,
			"planPrice":     planPrice,
			"commission":    commission,
		})
	}

	var wholesalerResult []map[string]interface{}
	for _, w := range wholesalers {
		user := userMap[w.UserID.Hex()]
		planID, hasPlan := wholesalerPlanMap[w.UserID.Hex()]
		var planTitle string
		var planPrice float64
		var commission float64
		if hasPlan {
			plan := planMap[planID.Hex()]
			planTitle = plan.Title
			planPrice = plan.Price
			commission = planPrice * salesperson.CommissionPercent / 100.0
		}
		wholesalerResult = append(wholesalerResult, map[string]interface{}{
			"businessName":  w.BusinessName,
			"email":         user.Email,
			"contactPerson": w.ContactInfo.Phone,
			"contactPhone":  w.ContactInfo.WhatsApp,
			"logo":          user.LogoPath,
			"planTitle":     planTitle,
			"planPrice":     planPrice,
			"commission":    commission,
		})
	}

	var spResult []map[string]interface{}
	for _, s := range serviceProviders {
		user := userMap[s.UserID.Hex()]
		planID, hasPlan := spPlanMap[s.UserID.Hex()]
		var planTitle string
		var planPrice float64
		var commission float64
		if hasPlan {
			plan := planMap[planID.Hex()]
			planTitle = plan.Title
			planPrice = plan.Price
			commission = planPrice * salesperson.CommissionPercent / 100.0
		}
		spResult = append(spResult, map[string]interface{}{
			"businessName":  s.BusinessName,
			"email":         user.Email,
			"contactPerson": s.ContactInfo.Phone,
			"contactPhone":  s.ContactInfo.WhatsApp,
			"logo":          user.LogoPath,
			"planTitle":     planTitle,
			"planPrice":     planPrice,
			"commission":    commission,
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Created users with commission info retrieved successfully",
		Data: map[string]interface{}{
			"companies":        companyResult,
			"wholesalers":      wholesalerResult,
			"serviceProviders": spResult,
		},
	})
}

// GetCommissionAndWithdrawalHistory retrieves all commission and withdrawal records for the authenticated user
func (spc *SalesPersonController) GetCommissionAndWithdrawalHistory(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Invalid user ID: " + err.Error(),
		})
	}

	db := spc.DB.Database("barrim")

	// Fetch commission records
	var commissionFilter bson.M
	switch claims.UserType {
	case "salesperson":
		// Try both field name formats to handle legacy data
		commissionFilter = bson.M{
			"$or": []bson.M{
				{"salespersonID": userID}, // New format (uppercase ID)
				{"salespersonId": userID}, // Legacy format (lowercase i)
			},
		}
	case "sales_manager":
		// Try both field name formats to handle legacy data
		commissionFilter = bson.M{
			"$or": []bson.M{
				{"salesManagerID": userID}, // New format (uppercase ID)
				{"salesManagerId": userID}, // Legacy format (lowercase i)
			},
		}
	default:
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "User type not allowed for commission history",
		})
	}
	commissionCursor, err := db.Collection("commissions").Find(
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

	// Fetch withdrawal records
	withdrawalFilter := bson.M{"userId": userID, "userType": claims.UserType}
	withdrawalCursor, err := db.Collection("withdrawals").Find(
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

	// For salespersons, also fetch referral balance and referral commissions
	var referralBalance float64
	var referralCommissions []models.ReferralCommission
	var referralCount int

	if claims.UserType == "salesperson" {
		// Get salesperson data to fetch referral balance
		var salesperson models.Salesperson
		err := db.Collection("salespersons").FindOne(
			context.Background(),
			bson.M{"_id": userID},
		).Decode(&salesperson)
		if err == nil {
			referralBalance = salesperson.ReferralBalance
			referralCount = len(salesperson.Referrals)
		}

		// Fetch referral commission records
		referralCommissionCursor, err := db.Collection("referral_commissions").Find(
			context.Background(),
			bson.M{"salespersonID": userID},
			&options.FindOptions{Sort: bson.M{"createdAt": -1}},
		)
		if err == nil {
			referralCommissionCursor.All(context.Background(), &referralCommissions)
			referralCommissionCursor.Close(context.Background())
		}
	}

	// Prepare response data
	responseData := map[string]interface{}{
		"commissions": commissions,
		"withdrawals": withdrawals,
	}

	// Add referral data for salespersons
	if claims.UserType == "salesperson" {
		responseData["referralBalance"] = referralBalance
		responseData["referralCount"] = referralCount
		responseData["referralCommissions"] = referralCommissions
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Commission and withdrawal history retrieved successfully",
		Data:    responseData,
	})
}

// GetServiceProviders retrieves all service providers created by the salesperson
func (spc *SalesPersonController) GetServiceProviders(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	salesPersonID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Invalid sales person ID",
		})
	}
	db := spc.DB.Database("barrim")
	spCursor, err := db.Collection("serviceProviders").Find(
		context.Background(),
		bson.M{"createdBy": salesPersonID},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch service providers",
		})
	}
	var serviceProviders []models.ServiceProvider
	_ = spCursor.All(context.Background(), &serviceProviders)
	spCursor.Close(context.Background())

	// Fetch user info for these service providers
	userIDs := make([]primitive.ObjectID, 0, len(serviceProviders))
	for _, s := range serviceProviders {
		userIDs = append(userIDs, s.UserID)
	}
	userMap := make(map[string]models.User)
	if len(userIDs) > 0 {
		userCursor, err := db.Collection("users").Find(
			context.Background(),
			bson.M{"_id": bson.M{"$in": userIDs}},
		)
		if err == nil {
			var users []models.User
			_ = userCursor.All(context.Background(), &users)
			userCursor.Close(context.Background())
			for _, u := range users {
				userMap[u.ID.Hex()] = u
			}
		}
	}

	// Enrich service providers with user info and subscription details
	var enrichedSPs []map[string]interface{}
	for _, s := range serviceProviders {
		user := userMap[s.UserID.Hex()]

		// Get active subscription for this service provider
		var planTitle string
		var planPrice float64
		var planDuration int
		var planType string
		var planBenefits interface{}
		var subscriptionStartDate time.Time
		var subscriptionEndDate time.Time
		var subscriptionLevel string
		var remainingDays int
		var hasActiveSubscription bool
		var totalActiveSubscriptions int
		var totalSubscriptionValue float64

		// Check for active service provider subscription
		var subscription models.ServiceProviderSubscription
		err := db.Collection("serviceProviderSubscriptions").FindOne(
			context.Background(),
			bson.M{
				"serviceProviderId": s.ID,
				"status":            "active",
				"endDate":           bson.M{"$gt": time.Now()},
			},
		).Decode(&subscription)

		// Count total active service provider subscriptions
		if err == nil {
			totalActiveSubscriptions++
		}

		if err == nil {
			// Get plan details
			var plan models.SubscriptionPlan
			err = db.Collection("subscription_plans").FindOne(
				context.Background(),
				bson.M{"_id": subscription.PlanID},
			).Decode(&plan)

			if err == nil {
				planTitle = plan.Title
				planPrice = plan.Price
				planDuration = plan.Duration
				planType = plan.Type
				planBenefits = plan.Benefits
				subscriptionStartDate = subscription.StartDate
				subscriptionEndDate = subscription.EndDate
				subscriptionLevel = "serviceProvider"
				hasActiveSubscription = true
				// Calculate remaining days
				remainingDays = int(time.Until(subscription.EndDate).Hours() / 24)
				// Add to total subscription value
				totalSubscriptionValue += planPrice
			}
		}

		enrichedSP := map[string]interface{}{
			"id":            s.ID,
			"businessName":  s.BusinessName,
			"category":      s.Category,
			"logoURL":       s.LogoURL,
			"contactPerson": s.ContactPerson,
			"contactPhone":  s.ContactPhone,
			"phone":         s.Phone,
			"email":         s.Email,
			"address": map[string]interface{}{
				"country":     s.Country,
				"governorate": s.Governorate,
				"district":    s.District,
				"city":        s.City,
			},
			"createdAt": s.CreatedAt,
			"status":    s.CreationRequest,
			"user":      user,
			"subscription": map[string]interface{}{
				"planTitle":                planTitle,
				"planPrice":                planPrice,
				"planDuration":             planDuration,
				"planType":                 planType,
				"planBenefits":             planBenefits,
				"startDate":                subscriptionStartDate,
				"endDate":                  subscriptionEndDate,
				"subscriptionLevel":        subscriptionLevel,
				"remainingDays":            remainingDays,
				"totalActiveSubscriptions": totalActiveSubscriptions,
				"totalSubscriptionValue":   totalSubscriptionValue,
				"hasActiveSubscription":    hasActiveSubscription,
			},
		}

		enrichedSPs = append(enrichedSPs, enrichedSP)
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Service providers retrieved successfully",
		Data:    enrichedSPs,
	})
}

// DeleteServiceProvider deletes a specific service provider by ID
func (spc *SalesPersonController) DeleteServiceProvider(c echo.Context) error {
	serviceProviderID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid service provider ID",
		})
	}
	db := spc.DB.Database("barrim")
	// Delete service provider from database
	result, err := db.Collection("serviceProviders").DeleteOne(
		context.Background(),
		bson.M{"_id": serviceProviderID},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to delete service provider",
		})
	}
	if result.DeletedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Service provider not found",
		})
	}
	// Delete associated user account
	_, err = db.Collection("users").DeleteOne(
		context.Background(),
		bson.M{"serviceProviderId": serviceProviderID},
	)
	if err != nil {
		log.Printf("Failed to delete associated user account: %v", err)
	}
	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Service provider deleted successfully",
	})
}

// UpdateServiceProvider updates a specific service provider
func (spc *SalesPersonController) UpdateServiceProvider(c echo.Context) error {
	serviceProviderID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid service provider ID",
		})
	}
	db := spc.DB.Database("barrim")
	// Parse form data
	form, err := c.MultipartForm()
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid form data",
		})
	}
	// Get form values
	updates := bson.M{
		"businessName": form.Value["businessName"][0],
		"category":     form.Value["category"][0],
		"contactInfo": bson.M{
			"phone":    form.Value["phone"][0],
			"whatsapp": form.Value["contactPhone"][0],
			"address": bson.M{
				"country":  form.Value["country"][0],
				"district": form.Value["district"][0],
				"city":     form.Value["city"][0],
				"street":   form.Value["street"][0],
			},
		},
		"updatedAt": time.Now(),
	}
	// Handle logo upload if provided
	if files, ok := form.File["logo"]; ok && len(files) > 0 {
		file := files[0]
		src, err := file.Open()
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to open uploaded file",
			})
		}
		defer src.Close()
		// Create uploads directory if it doesn't exist
		uploadDir := "uploads/logo"
		if err := os.MkdirAll(uploadDir, 0755); err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to create upload directory",
			})
		}
		filename := fmt.Sprintf("%d_%s", time.Now().UnixNano(), file.Filename)
		filepath := filepath.Join(uploadDir, filename)
		dst, err := os.Create(filepath)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to create destination file",
			})
		}
		defer dst.Close()
		if _, err = io.Copy(dst, src); err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to save file",
			})
		}
		updates["logoUrl"] = filepath
	}
	// Update service provider in database
	result, err := db.Collection("serviceProviders").UpdateOne(
		context.Background(),
		bson.M{"_id": serviceProviderID},
		bson.M{"$set": updates},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update service provider",
		})
	}
	if result.MatchedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Service provider not found",
		})
	}
	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Service provider updated successfully",
	})
}

// GetPendingRequests retrieves all pending requests created by the sales person
func (spc *SalesPersonController) GetPendingRequests(c echo.Context) error {
	// Get sales person ID from token
	claims := middleware.GetUserFromToken(c)
	salesPersonID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Invalid sales person ID",
		})
	}

	db := spc.DB.Database("barrim")

	// Get pending company requests
	var pendingCompanies []models.PendingCompanyRequest
	companyCursor, err := db.Collection("pending_company_requests").Find(
		context.Background(),
		bson.M{"salesPersonId": salesPersonID},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch pending company requests",
		})
	}
	defer companyCursor.Close(context.Background())
	companyCursor.All(context.Background(), &pendingCompanies)

	// Get pending wholesaler requests
	var pendingWholesalers []models.PendingWholesalerRequest
	wholesalerCursor, err := db.Collection("pending_wholesaler_requests").Find(
		context.Background(),
		bson.M{"salesPersonId": salesPersonID},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch pending wholesaler requests",
		})
	}
	defer wholesalerCursor.Close(context.Background())
	wholesalerCursor.All(context.Background(), &pendingWholesalers)

	// Get pending service provider requests
	var pendingServiceProviders []models.PendingServiceProviderRequest
	spCursor, err := db.Collection("pending_serviceProviders_requests").Find(
		context.Background(),
		bson.M{"salesPersonId": salesPersonID},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch pending service provider requests",
		})
	}
	defer spCursor.Close(context.Background())
	spCursor.All(context.Background(), &pendingServiceProviders)

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Pending requests retrieved successfully",
		Data: map[string]interface{}{
			"companies":        pendingCompanies,
			"wholesalers":      pendingWholesalers,
			"serviceProviders": pendingServiceProviders,
		},
	})
}

// GetPendingCompanyRequests retrieves pending company requests created by the sales person
func (spc *SalesPersonController) GetPendingCompanyRequests(c echo.Context) error {
	// Get sales person ID from token
	claims := middleware.GetUserFromToken(c)
	salesPersonID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Invalid sales person ID",
		})
	}

	var pendingCompanies []models.PendingCompanyRequest
	cursor, err := spc.DB.Database("barrim").Collection("pending_company_requests").Find(
		context.Background(),
		bson.M{"salesPersonId": salesPersonID},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch pending company requests",
		})
	}
	defer cursor.Close(context.Background())

	if err := cursor.All(context.Background(), &pendingCompanies); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode pending company requests",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Pending company requests retrieved successfully",
		Data:    pendingCompanies,
	})
}

// GetPendingWholesalerRequests retrieves pending wholesaler requests created by the sales person
func (spc *SalesPersonController) GetPendingWholesalerRequests(c echo.Context) error {
	// Get sales person ID from token
	claims := middleware.GetUserFromToken(c)
	salesPersonID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Invalid sales person ID",
		})
	}

	var pendingWholesalers []models.PendingWholesalerRequest
	cursor, err := spc.DB.Database("barrim").Collection("pending_wholesaler_requests").Find(
		context.Background(),
		bson.M{"salesPersonId": salesPersonID},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch pending wholesaler requests",
		})
	}
	defer cursor.Close(context.Background())

	if err := cursor.All(context.Background(), &pendingWholesalers); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode pending wholesaler requests",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Pending wholesaler requests retrieved successfully",
		Data:    pendingWholesalers,
	})
}

// GetPendingServiceProviderRequests retrieves pending service provider requests created by the sales person
func (spc *SalesPersonController) GetPendingServiceProviderRequests(c echo.Context) error {
	// Get sales person ID from token
	claims := middleware.GetUserFromToken(c)
	salesPersonID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Invalid sales person ID",
		})
	}

	var pendingServiceProviders []models.PendingServiceProviderRequest
	cursor, err := spc.DB.Database("barrim").Collection("pending_serviceProviders_requests").Find(
		context.Background(),
		bson.M{"salesPersonId": salesPersonID},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch pending service provider requests",
		})
	}
	defer cursor.Close(context.Background())

	if err := cursor.All(context.Background(), &pendingServiceProviders); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode pending service provider requests",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Pending service provider requests retrieved successfully",
		Data:    pendingServiceProviders,
	})
}

// RejectPendingCompanyRequest allows sales person to reject and delete their pending company request
func (spc *SalesPersonController) RejectPendingCompanyRequest(c echo.Context) error {
	// Get sales person ID from token
	claims := middleware.GetUserFromToken(c)
	salesPersonID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Invalid sales person ID",
		})
	}

	// Get request ID from URL parameter
	requestID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request ID",
		})
	}

	// Delete the pending request
	result, err := spc.DB.Database("barrim").Collection("pending_company_requests").DeleteOne(
		context.Background(),
		bson.M{
			"_id":           requestID,
			"salesPersonId": salesPersonID,
		},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to reject pending request",
		})
	}

	if result.DeletedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Pending request not found or you don't have permission to reject it",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Pending company request rejected and deleted successfully",
	})
}

// RejectPendingWholesalerRequest allows sales person to reject and delete their pending wholesaler request
func (spc *SalesPersonController) RejectPendingWholesalerRequest(c echo.Context) error {
	// Get sales person ID from token
	claims := middleware.GetUserFromToken(c)
	salesPersonID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Invalid sales person ID",
		})
	}

	// Get request ID from URL parameter
	requestID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request ID",
		})
	}

	// Delete the pending request
	result, err := spc.DB.Database("barrim").Collection("pending_wholesaler_requests").DeleteOne(
		context.Background(),
		bson.M{
			"_id":           requestID,
			"salesPersonId": salesPersonID,
		},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to reject pending request",
		})
	}

	if result.DeletedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Pending request not found or you don't have permission to reject it",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Pending wholesaler request rejected and deleted successfully",
	})
}

// RejectPendingServiceProviderRequest allows sales person to reject and delete their pending service provider request
func (spc *SalesPersonController) RejectPendingServiceProviderRequest(c echo.Context) error {
	// Get sales person ID from token
	claims := middleware.GetUserFromToken(c)
	salesPersonID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Invalid sales person ID",
		})
	}

	// Get request ID from URL parameter
	requestID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request ID",
		})
	}

	// Delete the pending request
	result, err := spc.DB.Database("barrim").Collection("pending_serviceProviders_requests").DeleteOne(
		context.Background(),
		bson.M{
			"_id":           requestID,
			"salesPersonId": salesPersonID,
		},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to reject pending request",
		})
	}

	if result.DeletedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Pending request not found or you don't have permission to reject it",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Pending service provider request rejected and deleted successfully",
	})
}

// GetPendingRequestsStatus retrieves a summary of pending requests status for the sales person
func (spc *SalesPersonController) GetPendingRequestsStatus(c echo.Context) error {
	// Get sales person ID from token
	claims := middleware.GetUserFromToken(c)
	salesPersonID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Invalid sales person ID",
		})
	}

	db := spc.DB.Database("barrim")

	// Count pending company requests
	pendingCompanyCount, err := db.Collection("pending_company_requests").CountDocuments(
		context.Background(),
		bson.M{"salesPersonId": salesPersonID, "status": "pending"},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to count pending company requests",
		})
	}

	// Count pending wholesaler requests
	pendingWholesalerCount, err := db.Collection("pending_wholesaler_requests").CountDocuments(
		context.Background(),
		bson.M{"salesPersonId": salesPersonID, "status": "pending"},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to count pending wholesaler requests",
		})
	}

	// Count pending service provider requests
	pendingSPCount, err := db.Collection("pending_serviceProviders_requests").CountDocuments(
		context.Background(),
		bson.M{"salesPersonId": salesPersonID, "status": "pending"},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to count pending service provider requests",
		})
	}

	// Count approved company requests
	approvedCompanyCount, err := db.Collection("pending_company_requests").CountDocuments(
		context.Background(),
		bson.M{"salesPersonId": salesPersonID, "status": "approved"},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to count approved company requests",
		})
	}

	// Count approved wholesaler requests
	approvedWholesalerCount, err := db.Collection("pending_wholesaler_requests").CountDocuments(
		context.Background(),
		bson.M{"salesPersonId": salesPersonID, "status": "approved"},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to count approved wholesaler requests",
		})
	}

	// Count approved service provider requests
	approvedSPCount, err := db.Collection("pending_serviceProviders_requests").CountDocuments(
		context.Background(),
		bson.M{"salesPersonId": salesPersonID, "status": "approved"},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to count approved service provider requests",
		})
	}

	// Count rejected company requests
	rejectedCompanyCount, err := db.Collection("pending_company_requests").CountDocuments(
		context.Background(),
		bson.M{"salesPersonId": salesPersonID, "status": "rejected"},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to count rejected company requests",
		})
	}

	// Count rejected wholesaler requests
	rejectedWholesalerCount, err := db.Collection("pending_wholesaler_requests").CountDocuments(
		context.Background(),
		bson.M{"salesPersonId": salesPersonID, "status": "rejected"},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to count rejected wholesaler requests",
		})
	}

	// Count rejected service provider requests
	rejectedSPCount, err := db.Collection("pending_serviceProviders_requests").CountDocuments(
		context.Background(),
		bson.M{"salesPersonId": salesPersonID, "status": "rejected"},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to count rejected service provider requests",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Pending requests status retrieved successfully",
		Data: map[string]interface{}{
			"pending": map[string]int64{
				"companies":        pendingCompanyCount,
				"wholesalers":      pendingWholesalerCount,
				"serviceProviders": pendingSPCount,
			},
			"approved": map[string]int64{
				"companies":        approvedCompanyCount,
				"wholesalers":      approvedWholesalerCount,
				"serviceProviders": approvedSPCount,
			},
			"rejected": map[string]int64{
				"companies":        rejectedCompanyCount,
				"wholesalers":      rejectedWholesalerCount,
				"serviceProviders": rejectedSPCount,
			},
		},
	})
}
