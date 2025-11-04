package controllers

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"math"
	"math/big"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/HSouheill/barrim_backend/config"
	"github.com/HSouheill/barrim_backend/middleware"
	"github.com/HSouheill/barrim_backend/models"
	"github.com/HSouheill/barrim_backend/utils"
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/gomail.v2"
)

type AdminController struct {
	DB *mongo.Database
}

// OTPData stores OTP information
type OTPData struct {
	OTP       string
	ExpiresAt time.Time
}

// Store OTPs in memory (in production, use Redis or similar)
var otpStore = make(map[string]OTPData)

func NewAdminController(db *mongo.Database) *AdminController {
	return &AdminController{DB: db}
}

// generateOTP creates a 4-digit OTP
func rgenerateOTP() (string, error) {
	otp := ""
	for i := 0; i < 4; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err
		}
		otp += fmt.Sprintf("%d", num)
	}
	return otp, nil
}

// sendOTPEmail sends OTP to admin's email using SMTP2GO
func sendOTPEmail(email, otp string) error {
	// Check if SMTP environment variables are set
	smtpHost := os.Getenv("SMTP_HOST")
	smtpUser := os.Getenv("SMTP_USER")
	smtpPass := os.Getenv("SMTP_PASS")
	fromEmail := os.Getenv("FROM_EMAIL")

	// Use FROM_EMAIL as sender, if not available, fall back to SMTP_USER
	senderEmail := fromEmail
	if senderEmail == "" {
		senderEmail = smtpUser
	}

	// Set SMTP2GO defaults if not configured
	if smtpHost == "" {
		smtpHost = "mail.smtp2go.com"
	}
	if smtpUser == "" || smtpPass == "" {
		return fmt.Errorf("SMTP2GO configuration is incomplete: check SMTP_USER and SMTP_PASS")
	}

	// Get SMTP port from environment or default to 2525
	smtpPortStr := os.Getenv("SMTP_PORT")
	smtpPort := 2525 // Default port
	if smtpPortStr != "" {
		portNum, err := strconv.Atoi(smtpPortStr)
		if err == nil && portNum > 0 {
			smtpPort = portNum
		}
	}

	// Log SMTP2GO configuration (remove in production)
	log.Printf("Sending email via SMTP2GO: %s:%d using account %s", smtpHost, smtpPort, smtpUser)

	m := gomail.NewMessage()
	m.SetHeader("From", senderEmail)
	m.SetHeader("To", email)
	m.SetHeader("Subject", "Password Reset OTP")
	m.SetBody("text/plain", fmt.Sprintf("Your OTP for password reset is: %s\nThis OTP will expire in 10 minutes.", otp))

	d := gomail.NewDialer(
		smtpHost,
		smtpPort,
		smtpUser,
		smtpPass,
	)

	// Try to send the email
	err := d.DialAndSend(m)
	if err != nil {
		log.Printf("Failed to send email: %v", err)
		return err
	}

	return nil
}

// ForgotPassword handles the forgot password request
func (ac *AdminController) ForgotPassword(c echo.Context) error {
	adminEmail := os.Getenv("ADMIN_EMAIL")
	if adminEmail == "" {
		log.Println("Admin email not configured")
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Admin email not configured",
		})
	}

	// Generate OTP
	otp, err := rgenerateOTP()
	if err != nil {
		log.Printf("Failed to generate OTP: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to generate OTP",
		})
	}

	// Store OTP with expiration
	otpStore[adminEmail] = OTPData{
		OTP:       otp,
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}

	// Send OTP via email
	if err := sendOTPEmail(adminEmail, otp); err != nil {
		log.Printf("Failed to send OTP email: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to send OTP email: " + err.Error(),
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "OTP sent successfully",
	})
}

// VerifyOTPAndResetPassword handles OTP verification and password reset
func (ac *AdminController) VerifyOTPAndResetPassword(c echo.Context) error {
	var req struct {
		OTP         string `json:"otp"`
		NewPassword string `json:"newPassword"`
	}

	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}

	adminEmail := os.Getenv("ADMIN_EMAIL")
	otpData, exists := otpStore[adminEmail]
	if !exists {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "No OTP request found",
		})
	}

	// Check if OTP has expired
	if time.Now().After(otpData.ExpiresAt) {
		delete(otpStore, adminEmail)
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "OTP has expired",
		})
	}

	// Verify OTP
	if req.OTP != otpData.OTP {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid OTP",
		})
	}

	// Update password in environment variable
	// Note: In a production environment, you should use a more secure method
	// to update the password, such as a database or secure configuration service
	os.Setenv("ADMIN_PASSWORD", req.NewPassword)

	// Clear OTP after successful password reset
	delete(otpStore, adminEmail)

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Password reset successful",
	})
}

// UnifiedLogin handles login for admin, sales managers, and salespersons
func (ac *AdminController) UnifiedLogin(c echo.Context) error {
	// Parse request body
	var loginReq struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	if err := c.Bind(&loginReq); err != nil {
		log.Printf("Bind error: %v", err)
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}

	// Log the login attempt (remove in production)
	log.Printf("Login attempt for email: %s", loginReq.Email)

	// Validate input
	if loginReq.Email == "" || loginReq.Password == "" {
		log.Printf("Empty email or password")
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Email and password are required",
		})
	}

	// Super-admin login from .env
	superAdminEmail := os.Getenv("SUPER_ADMIN_EMAIL")
	superAdminPassword := os.Getenv("SUPER_ADMIN_PASSWORD")
	if superAdminEmail != "" && superAdminPassword != "" && loginReq.Email == superAdminEmail {
		if loginReq.Password != superAdminPassword {
			return c.JSON(http.StatusUnauthorized, models.Response{
				Status:  http.StatusUnauthorized,
				Message: "Invalid super-admin credentials",
			})
		}
		// Generate JWT token for super-admin
		token, refreshToken, err := middleware.GenerateJWT("super_admin", loginReq.Email, "super_admin")
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to generate token",
			})
		}
		cookie := new(http.Cookie)
		cookie.Name = "super_admin_token"
		cookie.Value = token
		cookie.Expires = time.Now().Add(24 * time.Hour)
		cookie.HttpOnly = true
		cookie.Secure = false // Set to true in production
		cookie.SameSite = http.SameSiteStrictMode
		c.SetCookie(cookie)
		return c.JSON(http.StatusOK, models.Response{
			Status:  http.StatusOK,
			Message: "Super-admin login successful",
			Data: map[string]interface{}{
				"token":        token,
				"refreshToken": refreshToken,
				"user": map[string]interface{}{
					"email": loginReq.Email,
					"role":  "super_admin",
					"type":  "super_admin",
				},
			},
		})
	}

	// First check if it's admin login
	adminEmail := os.Getenv("ADMIN_EMAIL")
	adminPassword := os.Getenv("ADMIN_PASSWORD")

	log.Printf("Admin email from env: %s", adminEmail)
	log.Printf("Admin password configured: %t", adminPassword != "")

	if adminEmail != "" && adminPassword != "" && loginReq.Email == adminEmail {
		log.Printf("Attempting admin login")
		// Admin login - plain text password comparison
		if loginReq.Password != adminPassword {
			log.Printf("Admin password mismatch")
			return c.JSON(http.StatusUnauthorized, models.Response{
				Status:  http.StatusUnauthorized,
				Message: "Invalid admin credentials",
			})
		}

		log.Printf("Admin login successful")
		// Generate JWT token for admin
		token, refreshToken, err := middleware.GenerateJWT("admin", loginReq.Email, "admin")
		if err != nil {
			log.Printf("Failed to generate admin token: %v", err)
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to generate token",
			})
		}

		// Set secure cookie
		cookie := new(http.Cookie)
		cookie.Name = "admin_token"
		cookie.Value = token
		cookie.Expires = time.Now().Add(24 * time.Hour)
		cookie.HttpOnly = true
		cookie.Secure = false // Set to false for development, true for production
		cookie.SameSite = http.SameSiteStrictMode
		c.SetCookie(cookie)

		return c.JSON(http.StatusOK, models.Response{
			Status:  http.StatusOK,
			Message: "Admin login successful",
			Data: map[string]interface{}{
				"token":        token,
				"refreshToken": refreshToken,
				"user": map[string]interface{}{
					"email": loginReq.Email,
					"role":  "admin",
					"type":  "admin",
				},
			},
		})
	}

	// Check for dynamic admin in the database
	var dbAdmin models.Admin
	err := ac.DB.Collection("admins").FindOne(context.Background(), bson.M{"email": loginReq.Email}).Decode(&dbAdmin)
	if err == nil {
		if err := bcrypt.CompareHashAndPassword([]byte(dbAdmin.Password), []byte(loginReq.Password)); err != nil {
			return c.JSON(http.StatusUnauthorized, models.Response{
				Status:  http.StatusUnauthorized,
				Message: "Invalid admin credentials",
			})
		}
		// Generate JWT token for admin
		token, refreshToken, err := middleware.GenerateJWT(dbAdmin.ID.Hex(), dbAdmin.Email, "admin")
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to generate token",
			})
		}
		cookie := new(http.Cookie)
		cookie.Name = "admin_token"
		cookie.Value = token
		cookie.Expires = time.Now().Add(24 * time.Hour)
		cookie.HttpOnly = true
		cookie.Secure = false // Set to true in production
		cookie.SameSite = http.SameSiteStrictMode
		c.SetCookie(cookie)
		return c.JSON(http.StatusOK, models.Response{
			Status:  http.StatusOK,
			Message: "Admin login successful",
			Data: map[string]interface{}{
				"token":        token,
				"refreshToken": refreshToken,
				"user": map[string]interface{}{
					"id":    dbAdmin.ID,
					"email": dbAdmin.Email,
					"role":  "admin",
					"type":  "admin",
				},
			},
		})
	}

	// Check sales manager login
	log.Printf("Checking sales manager login")
	var salesManager models.SalesManager
	err = ac.DB.Collection("salesManagers").FindOne(context.Background(), bson.M{"email": loginReq.Email}).Decode(&salesManager)
	if err == nil {
		log.Printf("Found sales manager: %s", salesManager.Email)
		// Found sales manager, verify password
		if err := bcrypt.CompareHashAndPassword([]byte(salesManager.Password), []byte(loginReq.Password)); err != nil {
			log.Printf("Sales manager password mismatch: %v", err)
			return c.JSON(http.StatusUnauthorized, models.Response{
				Status:  http.StatusUnauthorized,
				Message: "Invalid sales manager credentials",
			})
		}

		log.Printf("Sales manager login successful")
		// Generate JWT token for sales manager
		token, refreshToken, err := middleware.GenerateJWT(salesManager.ID.Hex(), loginReq.Email, "sales_manager")
		if err != nil {
			log.Printf("Failed to generate sales manager token: %v", err)
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to generate token",
			})
		}

		// Set secure cookie
		cookie := new(http.Cookie)
		cookie.Name = "sales_manager_token"
		cookie.Value = token
		cookie.Expires = time.Now().Add(24 * time.Hour)
		cookie.HttpOnly = true
		cookie.Secure = false // Set to false for development, true for production
		cookie.SameSite = http.SameSiteStrictMode
		c.SetCookie(cookie)

		return c.JSON(http.StatusOK, models.Response{
			Status:  http.StatusOK,
			Message: "Sales manager login successful",
			Data: map[string]interface{}{
				"token":        token,
				"refreshToken": refreshToken,
				"user": map[string]interface{}{
					"id":          salesManager.ID,
					"email":       salesManager.Email,
					"fullName":    salesManager.FullName,
					"phoneNumber": salesManager.PhoneNumber,
					"role":        "salesManager",
					"type":        "salesManager",
				},
			},
		})
	} else if err != mongo.ErrNoDocuments {
		log.Printf("Error querying sales manager: %v", err)
	}

	// Check salesperson login
	log.Printf("Checking salesperson login")
	var salesperson models.Salesperson
	err = ac.DB.Collection("salespersons").FindOne(context.Background(), bson.M{"email": loginReq.Email}).Decode(&salesperson)
	if err == nil {
		log.Printf("Found salesperson: %s", salesperson.Email)
		// Found salesperson, verify password
		if err := bcrypt.CompareHashAndPassword([]byte(salesperson.Password), []byte(loginReq.Password)); err != nil {
			log.Printf("Salesperson password mismatch: %v", err)
			return c.JSON(http.StatusUnauthorized, models.Response{
				Status:  http.StatusUnauthorized,
				Message: "Invalid salesperson credentials",
			})
		}

		log.Printf("Salesperson login successful")
		// Generate JWT token for salesperson
		token, refreshToken, err := middleware.GenerateJWT(salesperson.ID.Hex(), loginReq.Email, "salesperson")
		if err != nil {
			log.Printf("Failed to generate salesperson token: %v", err)
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to generate token",
			})
		}

		// Set secure cookie
		cookie := new(http.Cookie)
		cookie.Name = "salesperson_token"
		cookie.Value = token
		cookie.Expires = time.Now().Add(24 * time.Hour)
		cookie.HttpOnly = true
		cookie.Secure = false // Set to false for development, true for production
		cookie.SameSite = http.SameSiteStrictMode
		c.SetCookie(cookie)

		return c.JSON(http.StatusOK, models.Response{
			Status:  http.StatusOK,
			Message: "Salesperson login successful",
			Data: map[string]interface{}{
				"token":        token,
				"refreshToken": refreshToken,
				"user": map[string]interface{}{
					"id":             salesperson.ID,
					"email":          salesperson.Email,
					"fullName":       salesperson.FullName,
					"phoneNumber":    salesperson.PhoneNumber,
					"region":         salesperson.Region,
					"salesManagerID": salesperson.SalesManagerID,
					"role":           "salesperson",
					"type":           "salesperson",
				},
			},
		})
	} else if err != mongo.ErrNoDocuments {
		log.Printf("Error querying salesperson: %v", err)
	}

	// Check manager login
	log.Printf("Checking manager login")
	var manager models.Manager
	err = ac.DB.Collection("managers").FindOne(context.Background(), bson.M{"email": loginReq.Email}).Decode(&manager)
	if err == nil {
		log.Printf("Found manager: %s", manager.Email)
		// Found manager, verify password
		if err := bcrypt.CompareHashAndPassword([]byte(manager.Password), []byte(loginReq.Password)); err != nil {
			log.Printf("Manager password mismatch: %v", err)
			return c.JSON(http.StatusUnauthorized, models.Response{
				Status:  http.StatusUnauthorized,
				Message: "Invalid manager credentials",
			})
		}

		log.Printf("Manager login successful")
		// Generate JWT token for manager
		token, refreshToken, err := middleware.GenerateJWT(manager.ID.Hex(), loginReq.Email, "manager")
		if err != nil {
			log.Printf("Failed to generate manager token: %v", err)
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to generate token",
			})
		}

		// Set secure cookie
		cookie := new(http.Cookie)
		cookie.Name = "manager_token"
		cookie.Value = token
		cookie.Expires = time.Now().Add(24 * time.Hour)
		cookie.HttpOnly = true
		cookie.Secure = false // Set to false for development, true for production
		cookie.SameSite = http.SameSiteStrictMode
		c.SetCookie(cookie)

		return c.JSON(http.StatusOK, models.Response{
			Status:  http.StatusOK,
			Message: "Manager login successful",
			Data: map[string]interface{}{
				"token":        token,
				"refreshToken": refreshToken,
				"user": map[string]interface{}{
					"id":          manager.ID,
					"email":       manager.Email,
					"fullName":    manager.FullName,
					"rolesAccess": manager.RolesAccess,
					"role":        "manager",
					"type":        "manager",
				},
			},
		})
	} else if err != mongo.ErrNoDocuments {
		log.Printf("Error querying manager: %v", err)
	}

	// If no match found in any category
	log.Printf("No user found with email: %s", loginReq.Email)
	// Use constant time delay to prevent timing attacks
	time.Sleep(time.Millisecond * 100)
	return c.JSON(http.StatusUnauthorized, models.Response{
		Status:  http.StatusUnauthorized,
		Message: "Invalid credentials - user not found",
	})
}

// Legacy Login method - kept for backward compatibility
func (ac *AdminController) Login(c echo.Context) error {
	return ac.UnifiedLogin(c)
}

// GetActiveUsers retrieves all active users
func (ac *AdminController) GetActiveUsers(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	if claims.UserType == "super_admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Super-admins are not allowed to access this resource",
		})
	}
	collection := config.GetCollection(ac.DB.Client(), "users")
	ctx := context.Background()

	// Current time threshold for activity
	timeThreshold := time.Now().Add(-15 * time.Minute)
	log.Printf("Searching for users active since: %v", timeThreshold)

	// Query for users that are either:
	// 1. Marked as active AND have recent activity, OR
	// 2. Marked as active (even if lastActivityAt is not set)
	filter := bson.M{
		"$or": []bson.M{
			{
				"isActive":       true,
				"lastActivityAt": bson.M{"$gte": timeThreshold},
			},
			{
				"isActive":       true,
				"lastActivityAt": bson.M{"$exists": false},
			},
		},
	}

	cursor, err := collection.Find(ctx, filter)
	if err != nil {
		log.Printf("Error finding active users: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch active users",
		})
	}
	defer cursor.Close(ctx)

	var activeUsers []map[string]interface{}
	for cursor.Next(ctx) {
		var user models.User
		if err := cursor.Decode(&user); err != nil {
			log.Printf("Error decoding user: %v", err)
			continue // Skip users that can't be decoded
		}

		// Calculate time connected based on last activity
		var timeConnected string
		if !user.LastActivityAt.IsZero() {
			timeConnected = time.Since(user.LastActivityAt).String()
		} else {
			timeConnected = "unknown"
		}

		userData := map[string]interface{}{
			"id":            user.ID,
			"email":         user.Email,
			"fullName":      user.FullName,
			"userType":      user.UserType,
			"lastActivity":  user.LastActivityAt,
			"timeConnected": timeConnected,
			"isActive":      user.IsActive,
		}

		// Get additional information based on user type
		switch user.UserType {
		case "company":
			if user.CompanyID != nil {
				companyInfo, err := ac.getCompanyInfo(*user.CompanyID)
				if err == nil {
					userData["branchStatus"] = companyInfo.BranchStatus
					userData["salesManagerEmail"] = companyInfo.SalesManagerEmail
					userData["salespersonEmail"] = companyInfo.SalespersonEmail
				}
			}
		case "wholesaler":
			if user.WholesalerID != nil {
				wholesalerInfo, err := ac.getWholesalerInfo(*user.WholesalerID)
				if err == nil {
					userData["branchStatus"] = wholesalerInfo.BranchStatus
					userData["salesManagerEmail"] = wholesalerInfo.SalesManagerEmail
					userData["salespersonEmail"] = wholesalerInfo.SalespersonEmail
				}
			}
		case "service_provider":
			if user.ServiceProviderID != nil {
				serviceProviderInfo, err := ac.getServiceProviderInfo(*user.ServiceProviderID)
				if err == nil {
					userData["status"] = serviceProviderInfo.Status
					userData["salesManagerEmail"] = serviceProviderInfo.SalesManagerEmail
					userData["salespersonEmail"] = serviceProviderInfo.SalespersonEmail
				}
			}
		}

		activeUsers = append(activeUsers, userData)
	}

	log.Printf("Found %d active users", len(activeUsers))

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Active users retrieved successfully",
		Data: map[string]interface{}{
			"count": len(activeUsers),
			"users": activeUsers,
		},
	})
}

// Helper function to get company information including branch status and creator details
func (ac *AdminController) getCompanyInfo(companyID primitive.ObjectID) (*CompanyInfo, error) {
	collection := config.GetCollection(ac.DB.Client(), "companies")
	ctx := context.Background()

	var company models.Company
	err := collection.FindOne(ctx, bson.M{"_id": companyID}).Decode(&company)
	if err != nil {
		return nil, err
	}

	companyInfo := &CompanyInfo{
		BranchStatus: "no_branches",
	}

	// Get branch status if branches exist
	if len(company.Branches) > 0 {
		// Check if any branch is active
		hasActiveBranch := false
		for _, branch := range company.Branches {
			if branch.Status == "approved" {
				hasActiveBranch = true
				break
			}
		}
		if hasActiveBranch {
			companyInfo.BranchStatus = "active"
		} else {
			companyInfo.BranchStatus = "pending"
		}
	}

	// Get sales manager and salesperson emails if created by them
	if !company.CreatedBy.IsZero() {
		// Check if created by sales manager
		salesManagerInfo, err := ac.getSalesManagerInfo(company.CreatedBy)
		if err == nil {
			companyInfo.SalesManagerEmail = salesManagerInfo.Email
		} else {
			// If not a sales manager, check if it's a salesperson
			salespersonInfo, err := ac.getSalespersonInfo(company.CreatedBy)
			if err == nil {
				companyInfo.SalespersonEmail = salespersonInfo.Email
				// Get the sales manager email for this salesperson
				if !salespersonInfo.SalesManagerID.IsZero() {
					salesManagerInfo, err := ac.getSalesManagerInfo(salespersonInfo.SalesManagerID)
					if err == nil {
						companyInfo.SalesManagerEmail = salesManagerInfo.Email
					}
				}
			}
		}
	}

	return companyInfo, nil
}

// Helper function to get wholesaler information including branch status and creator details
func (ac *AdminController) getWholesalerInfo(wholesalerID primitive.ObjectID) (*WholesalerInfo, error) {
	collection := config.GetCollection(ac.DB.Client(), "wholesalers")
	ctx := context.Background()

	var wholesaler models.Wholesaler
	err := collection.FindOne(ctx, bson.M{"_id": wholesalerID}).Decode(&wholesaler)
	if err != nil {
		return nil, err
	}

	wholesalerInfo := &WholesalerInfo{
		BranchStatus: "no_branches",
	}

	// Get branch status if branches exist
	if len(wholesaler.Branches) > 0 {
		// Check if any branch is active
		hasActiveBranch := false
		for _, branch := range wholesaler.Branches {
			if branch.Status == "approved" {
				hasActiveBranch = true
				break
			}
		}
		if hasActiveBranch {
			wholesalerInfo.BranchStatus = "active"
		} else {
			wholesalerInfo.BranchStatus = "pending"
		}
	}

	// Get sales manager and salesperson emails if created by them
	if !wholesaler.CreatedBy.IsZero() {
		// Check if created by sales manager
		salesManagerInfo, err := ac.getSalesManagerInfo(wholesaler.CreatedBy)
		if err == nil {
			wholesalerInfo.SalesManagerEmail = salesManagerInfo.Email
		} else {
			// If not a sales manager, check if it's a salesperson
			salespersonInfo, err := ac.getSalespersonInfo(wholesaler.CreatedBy)
			if err == nil {
				wholesalerInfo.SalespersonEmail = salespersonInfo.Email
				// Get the sales manager email for this salesperson
				if !salespersonInfo.SalesManagerID.IsZero() {
					salesManagerInfo, err := ac.getSalesManagerInfo(salespersonInfo.SalesManagerID)
					if err == nil {
						wholesalerInfo.SalesManagerEmail = salesManagerInfo.Email
					}
				}
			}
		}
	}

	return wholesalerInfo, nil
}

// Helper function to get service provider information including status and creator details
func (ac *AdminController) getServiceProviderInfo(serviceProviderID primitive.ObjectID) (*ServiceProviderAdditionalInfo, error) {
	collection := config.GetCollection(ac.DB.Client(), "service_providers")
	ctx := context.Background()

	var serviceProvider models.ServiceProvider
	err := collection.FindOne(ctx, bson.M{"_id": serviceProviderID}).Decode(&serviceProvider)
	if err != nil {
		return nil, err
	}

	serviceProviderInfo := &ServiceProviderAdditionalInfo{
		Status: serviceProvider.Status,
	}

	// Get sales manager and salesperson emails if created by them
	if !serviceProvider.CreatedBy.IsZero() {
		// Check if created by sales manager
		salesManagerInfo, err := ac.getSalesManagerInfo(serviceProvider.CreatedBy)
		if err == nil {
			serviceProviderInfo.SalesManagerEmail = salesManagerInfo.Email
		} else {
			// If not a sales manager, check if it's a salesperson
			salespersonInfo, err := ac.getSalespersonInfo(serviceProvider.CreatedBy)
			if err == nil {
				serviceProviderInfo.SalespersonEmail = salespersonInfo.Email
				// Get the sales manager email for this salesperson
				if !salespersonInfo.SalesManagerID.IsZero() {
					salesManagerInfo, err := ac.getSalesManagerInfo(salespersonInfo.SalesManagerID)
					if err == nil {
						serviceProviderInfo.SalesManagerEmail = salesManagerInfo.Email
					}
				}
			}
		}
	}

	return serviceProviderInfo, nil
}

// Helper function to get sales manager information
func (ac *AdminController) getSalesManagerInfo(salesManagerID primitive.ObjectID) (*SalesManagerInfo, error) {
	collection := config.GetCollection(ac.DB.Client(), "sales_managers")
	ctx := context.Background()

	var salesManager models.SalesManager
	err := collection.FindOne(ctx, bson.M{"_id": salesManagerID}).Decode(&salesManager)
	if err != nil {
		return nil, err
	}

	return &SalesManagerInfo{
		Email: salesManager.Email,
	}, nil
}

// Helper function to get salesperson information
func (ac *AdminController) getSalespersonInfo(salespersonID primitive.ObjectID) (*SalespersonInfo, error) {
	collection := config.GetCollection(ac.DB.Client(), "salespersons")
	ctx := context.Background()

	var salesperson models.Salesperson
	err := collection.FindOne(ctx, bson.M{"_id": salespersonID}).Decode(&salesperson)
	if err != nil {
		return nil, err
	}

	return &SalespersonInfo{
		Email:          salesperson.Email,
		SalesManagerID: salesperson.SalesManagerID,
	}, nil
}

// Helper structs for additional information
type CompanyInfo struct {
	BranchStatus      string `json:"branchStatus"`
	SalesManagerEmail string `json:"salesManagerEmail,omitempty"`
	SalespersonEmail  string `json:"salespersonEmail,omitempty"`
}

type WholesalerInfo struct {
	BranchStatus      string `json:"branchStatus"`
	SalesManagerEmail string `json:"salesManagerEmail,omitempty"`
	SalespersonEmail  string `json:"salespersonEmail,omitempty"`
}

type ServiceProviderAdditionalInfo struct {
	Status            string `json:"status"`
	SalesManagerEmail string `json:"salesManagerEmail,omitempty"`
	SalespersonEmail  string `json:"salespersonEmail,omitempty"`
}

type SalesManagerInfo struct {
	Email string `json:"email"`
}

type SalespersonInfo struct {
	Email          string             `json:"email"`
	SalesManagerID primitive.ObjectID `json:"salesManagerID"`
}

// GetAllUsers retrieves all users with userType "user"
func (ac *AdminController) GetAllUsers(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	if claims.UserType == "super_admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Super-admins are not allowed to access this resource",
		})
	}

	collection := config.GetCollection(ac.DB.Client(), "users")
	ctx := context.Background()

	// Query for users with userType "user"
	filter := bson.M{"userType": "user"}

	cursor, err := collection.Find(ctx, filter)
	if err != nil {
		log.Printf("Error finding users: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch users",
		})
	}
	defer cursor.Close(ctx)

	var users []models.User
	if err := cursor.All(ctx, &users); err != nil {
		log.Printf("Error decoding users: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode users",
		})
	}

	// Remove sensitive information from response
	for i := range users {
		users[i].Password = ""
		users[i].OTPInfo = nil
		users[i].ResetPasswordToken = ""
	}

	log.Printf("Found %d users with userType 'user'", len(users))

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Users retrieved successfully",
		Data: map[string]interface{}{
			"count": len(users),
			"users": users,
		},
	})
}

// GetUserStatus checks if a specific user is active
func (ac *AdminController) GetUserStatus(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	if claims.UserType == "super_admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Super-admins are not allowed to access this resource",
		})
	}
	userID := c.Param("id")

	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	collection := config.GetCollection(ac.DB.Client(), "users")
	ctx := context.Background()

	var user models.User
	err = collection.FindOne(ctx, bson.M{"_id": objID}).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "User not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch user",
		})
	}

	// Calculate if the user is active based on last activity time
	isRecentlyActive := false
	if !user.LastActivityAt.IsZero() {
		isRecentlyActive = time.Since(user.LastActivityAt) < 15*time.Minute
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "User status retrieved",
		Data: map[string]interface{}{
			"isActive":     user.IsActive && isRecentlyActive,
			"lastActivity": user.LastActivityAt,
		},
	})
}

// CreateSalesManager creates a new sales manager
func (ac *AdminController) CreateSalesManager(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	if claims.UserType == "super_admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Super-admins are not allowed to access this resource",
		})
	}
	var req struct {
		FullName          string  `json:"fullName"`
		Email             string  `json:"email"`
		Password          string  `json:"password"`
		PhoneNumber       string  `json:"phoneNumber"`
		CommissionPercent float64 `json:"commissionPercent"`
		Status            string  `json:"status"`
		CreatedBy         string  `json:"createdBy"`
		// RolesAccess       []string `json:"rolesAccess"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body: " + err.Error(),
		})
	}
	if req.FullName == "" || req.Email == "" || req.Password == "" || req.PhoneNumber == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Full name, email, password, phone number are required",
		})
	}
	if req.CommissionPercent <= 0 || req.CommissionPercent > 100 {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Commission percent must be between 0 and 100",
		})
	}
	// Validate rolesAccess
	// if len(req.RolesAccess) == 0 {
	// 	return c.JSON(http.StatusBadRequest, models.Response{
	// 		Status:  http.StatusBadRequest,
	// 		Message: "At least one access role must be selected",
	// 	})
	// }
	// for _, key := range req.RolesAccess {
	// 	count, err := ac.DB.Collection("access_roles").CountDocuments(context.Background(), bson.M{"key": key})
	// 	if err != nil || count == 0 {
	// 		return c.JSON(http.StatusBadRequest, models.Response{
	// 			Status:  http.StatusBadRequest,
	// 			Message: "Invalid access role: " + key,
	// 		})
	// 	}
	// }
	var existingManager models.SalesManager
	err := ac.DB.Collection("salesManagers").FindOne(context.Background(), bson.M{"email": req.Email}).Decode(&existingManager)
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
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to hash password",
		})
	}

	// Get admin ID from token
	adminID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid admin ID in token",
		})
	}

	salesManager := models.SalesManager{
		ID:                primitive.NewObjectID(),
		FullName:          req.FullName,
		Email:             req.Email,
		Password:          string(hashedPassword),
		PhoneNumber:       req.PhoneNumber,
		CommissionPercent: req.CommissionPercent,
		CreatedBy:         adminID, // Set to the admin who created this sales manager
		Salespersons:      []primitive.ObjectID{},
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
		// RolesAccess:       req.RolesAccess,
	}
	_, err = ac.DB.Collection("salesManagers").InsertOne(context.Background(), salesManager)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to create sales manager",
		})
	}
	salesManager.Password = ""
	return c.JSON(http.StatusCreated, models.Response{
		Status:  http.StatusCreated,
		Message: "Sales manager created successfully",
		Data:    salesManager,
	})
}

// GetAllSalesManagers retrieves all sales managers
func (ac *AdminController) GetAllSalesManagers(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	if claims.UserType == "super_admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Super-admins are not allowed to access this resource",
		})
	}
	var salesManagers []models.SalesManager
	cursor, err := ac.DB.Collection("salesManagers").Find(context.Background(), bson.M{})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch sales managers",
		})
	}
	defer cursor.Close(context.Background())

	if err := cursor.All(context.Background(), &salesManagers); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode sales managers",
		})
	}

	// Prepare response with rolesAccess included
	var result []map[string]interface{}
	for _, sm := range salesManagers {
		result = append(result, map[string]interface{}{
			"id":                sm.ID,
			"fullName":          sm.FullName,
			"email":             sm.Email,
			"phoneNumber":       sm.PhoneNumber,
			"commissionPercent": sm.CommissionPercent,
			"createdBy":         sm.CreatedBy,
			"createdAt":         sm.CreatedAt,
			"updatedAt":         sm.UpdatedAt,
			// "rolesAccess": sm.RolesAccess,
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Sales managers retrieved successfully",
		Data:    result,
	})
}

// GetSalesManager retrieves a specific sales manager by ID with their salespersons
func (ac *AdminController) GetSalesManager(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	if claims.UserType == "super_admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Super-admins are not allowed to access this resource",
		})
	}
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid ID format",
		})
	}

	var salesManager models.SalesManager
	err = ac.DB.Collection("salesManagers").FindOne(context.Background(), bson.M{"_id": id}).Decode(&salesManager)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Sales manager not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch sales manager",
		})
	}

	// Get all salespersons created by this sales manager
	var salespersons []models.Salesperson
	cursor, err := ac.DB.Collection("salespersons").Find(
		context.Background(),
		bson.M{"salesManagerId": id},
		&options.FindOptions{
			Sort: bson.M{"createdAt": -1}, // Sort by creation date, newest first
		},
	)
	if err != nil {
		log.Printf("Warning: Failed to fetch salespersons for sales manager %s: %v", id.Hex(), err)
		// Don't fail the request, just return empty salespersons array
		salespersons = []models.Salesperson{}
	} else {
		defer cursor.Close(context.Background())
		if err := cursor.All(context.Background(), &salespersons); err != nil {
			log.Printf("Warning: Failed to decode salespersons for sales manager %s: %v", id.Hex(), err)
			salespersons = []models.Salesperson{}
		}
	}

	// Get statistics for the sales manager
	var totalCompanies int64
	var totalWholesalers int64
	var totalServiceProviders int64

	if len(salespersons) > 0 {
		// Get salesperson IDs
		salespersonIDs := make([]primitive.ObjectID, 0, len(salespersons))
		for _, sp := range salespersons {
			salespersonIDs = append(salespersonIDs, sp.ID)
		}

		// Count companies created by these salespersons
		totalCompanies, _ = ac.DB.Collection("companies").CountDocuments(
			context.Background(),
			bson.M{"createdBy": bson.M{"$in": salespersonIDs}},
		)

		// Count wholesalers created by these salespersons
		totalWholesalers, _ = ac.DB.Collection("wholesalers").CountDocuments(
			context.Background(),
			bson.M{"createdBy": bson.M{"$in": salespersonIDs}},
		)

		// Count service providers created by these salespersons
		totalServiceProviders, _ = ac.DB.Collection("serviceProviders").CountDocuments(
			context.Background(),
			bson.M{"createdBy": bson.M{"$in": salespersonIDs}},
		)
	}

	// Create enriched response
	enrichedSalesManager := map[string]interface{}{
		"salesManager": salesManager,
		"salespersons": salespersons,
		"statistics": map[string]interface{}{
			"totalSalespersons":     len(salespersons),
			"totalCompanies":        totalCompanies,
			"totalWholesalers":      totalWholesalers,
			"totalServiceProviders": totalServiceProviders,
		},
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Sales manager retrieved successfully with salespersons",
		Data:    enrichedSalesManager,
	})
}

// UpdateSalesManager updates a specific sales manager
func (ac *AdminController) UpdateSalesManager(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	if claims.UserType == "super_admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Super-admins are not allowed to access this resource",
		})
	}
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid ID format",
		})
	}

	var updates models.SalesManager
	if err := c.Bind(&updates); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}

	result, err := ac.DB.Collection("salesManagers").UpdateOne(
		context.Background(),
		bson.M{"_id": id},
		bson.M{"$set": updates},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update sales manager",
		})
	}

	if result.MatchedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Sales manager not found",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Sales manager updated successfully",
	})
}

// DeleteSalesManager deletes a specific sales manager
func (ac *AdminController) DeleteSalesManager(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	if claims.UserType == "super_admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Super-admins are not allowed to access this resource",
		})
	}
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid ID format",
		})
	}

	// First, get the sales manager to find their email
	var salesManager models.SalesManager
	err = ac.DB.Collection("salesManagers").FindOne(context.Background(), bson.M{"_id": id}).Decode(&salesManager)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Sales manager not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch sales manager",
		})
	}

	// Delete the sales manager
	result, err := ac.DB.Collection("salesManagers").DeleteOne(context.Background(), bson.M{"_id": id})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to delete sales manager",
		})
	}

	if result.DeletedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Sales manager not found",
		})
	}

	// Delete associated user entry from users collection
	_, err = ac.DB.Collection("users").DeleteOne(context.Background(), bson.M{
		"$or": []bson.M{
			{"email": salesManager.Email},
			{"userType": "sales_manager", "email": salesManager.Email},
		},
	})
	if err != nil {
		log.Printf("Failed to delete associated user account for sales manager %s: %v", salesManager.Email, err)
	} else {
		log.Printf("Successfully deleted associated user account for sales manager %s", salesManager.Email)
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Sales manager deleted successfully",
	})
}

// RegisterAdmin allows a super-admin to create a new admin user securely
func (ac *AdminController) RegisterAdmin(c echo.Context) error {
	// Only allow if the requester is a super-admin (implement this check)
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "super_admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only super-admins can create admin users",
		})
	}

	var req struct {
		FullName string `json:"fullName"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}

	// Validate email format and password strength
	if !isValidEmail(req.Email) || !isStrongPassword(req.Password) {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid email or weak password",
		})
	}

	// Check for existing admin
	var existingAdmin models.Admin
	err := ac.DB.Collection("admins").FindOne(context.Background(), bson.M{"email": req.Email}).Decode(&existingAdmin)
	if err == nil {
		return c.JSON(http.StatusConflict, models.Response{
			Status:  http.StatusConflict,
			Message: "Email already exists",
		})
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to hash password",
		})
	}

	admin := models.Admin{
		ID:        primitive.NewObjectID(),
		Email:     req.Email,
		Password:  string(hashedPassword),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	_, err = ac.DB.Collection("admins").InsertOne(context.Background(), admin)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to create admin",
		})
	}

	// Log the creation event (for auditing)
	log.Printf("Admin created: %s by %s", admin.Email, claims.UserID)

	admin.Password = "" // Never return password
	return c.JSON(http.StatusCreated, models.Response{
		Status:  http.StatusCreated,
		Message: "Admin created successfully",
		Data:    admin,
	})
}

// isValidEmail validates the email format (simple regex)
func isValidEmail(email string) bool {
	// Basic regex for demonstration; use a more robust one in production
	re := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	return re.MatchString(email)
}

// isStrongPassword checks for minimum length and complexity
func isStrongPassword(password string) bool {
	if len(password) < 8 {
		return false
	}
	var hasUpper, hasLower, hasNumber, hasSpecial bool
	for _, c := range password {
		switch {
		case 'A' <= c && c <= 'Z':
			hasUpper = true
		case 'a' <= c && c <= 'z':
			hasLower = true
		case '0' <= c && c <= '9':
			hasNumber = true
		case c >= 33 && c <= 47:
			hasSpecial = true
		}
	}
	return hasUpper && hasLower && hasNumber && hasSpecial
}

// Add these functions to your subscription_controller.go

// CreateWholesalerSubscription creates a new subscription request for a wholesaler

// GetPendingWholesalerSubscriptionRequests retrieves all pending wholesaler subscription requests (admin only)
func (sc *SubscriptionController) GetPendingWholesalerSubscriptionRequests(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user information from token
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins can access this endpoint",
		})
	}

	// Find all pending wholesaler subscription requests
	wholesalerSubscriptionRequestsCollection := sc.DB.Collection("wholesaler_subscription_requests")
	cursor, err := wholesalerSubscriptionRequestsCollection.Find(ctx, bson.M{"status": "pending"})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve subscription requests",
		})
	}
	defer cursor.Close(ctx)

	var requests []models.WholesalerSubscriptionRequest
	if err = cursor.All(ctx, &requests); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode subscription requests",
		})
	}

	// Get wholesaler and plan details for each request
	var enrichedRequests []map[string]interface{}
	for _, req := range requests {
		// Get wholesaler details
		var wholesaler models.Wholesaler
		err = sc.DB.Collection("wholesalers").FindOne(ctx, bson.M{"_id": req.WholesalerID}).Decode(&wholesaler)
		if err != nil {
			log.Printf("Error getting wholesaler details: %v", err)
			continue
		}

		// Get plan details
		var plan models.SubscriptionPlan
		err = sc.DB.Collection("subscription_plans").FindOne(ctx, bson.M{"_id": req.PlanID}).Decode(&plan)
		if err != nil {
			log.Printf("Error getting plan details: %v", err)
			continue
		}

		enrichedRequests = append(enrichedRequests, map[string]interface{}{
			"request": req,
			"wholesaler": map[string]interface{}{
				"id":           wholesaler.ID,
				"businessName": wholesaler.BusinessName,
				"category":     wholesaler.Category,
				"phone":        wholesaler.Phone,
			},
			"plan": plan,
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Pending wholesaler subscription requests retrieved successfully",
		Data:    enrichedRequests,
	})
}

// ProcessWholesalerSubscriptionRequest handles the approval or rejection of a wholesaler subscription request (admin only)
func (sc *SubscriptionController) ProcessWholesalerSubscriptionRequest(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user information from token
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins can process subscription requests",
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
	var approvalReq models.WholesalerSubscriptionApprovalRequest
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

	// Get the wholesaler subscription request
	wholesalerSubscriptionRequestsCollection := sc.DB.Collection("wholesaler_subscription_requests")
	var subscriptionRequest models.WholesalerSubscriptionRequest
	err = wholesalerSubscriptionRequestsCollection.FindOne(ctx, bson.M{"_id": requestObjectID}).Decode(&subscriptionRequest)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Wholesaler subscription request not found",
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
			Message: fmt.Sprintf("Wholesaler subscription request is already %s", subscriptionRequest.Status),
		})
	}

	// Delete the subscription request from database after processing
	_, err = wholesalerSubscriptionRequestsCollection.DeleteOne(ctx, bson.M{"_id": requestObjectID})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to delete subscription request",
		})
	}

	// Get wholesaler details
	var wholesaler models.Wholesaler
	err = sc.DB.Collection("wholesalers").FindOne(ctx, bson.M{"_id": subscriptionRequest.WholesalerID}).Decode(&wholesaler)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to get wholesaler details",
		})
	}

	// Get plan details
	var plan models.SubscriptionPlan
	err = sc.DB.Collection("subscription_plans").FindOne(ctx, bson.M{"_id": subscriptionRequest.PlanID}).Decode(&plan)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to get plan details",
		})
	}

	// If approved, create the subscription
	var subscription *models.WholesalerSubscription
	if approvalReq.Status == "approved" {
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

		// Create subscription
		newSubscription := models.WholesalerSubscription{
			ID:           primitive.NewObjectID(),
			WholesalerID: subscriptionRequest.WholesalerID,
			PlanID:       subscriptionRequest.PlanID,
			StartDate:    startDate,
			EndDate:      endDate,
			Status:       "active",
			AutoRenew:    false, // Default to false, can be changed later
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}

		// Save subscription
		subscriptionsCollection := sc.DB.Collection("wholesaler_subscriptions")
		_, err = subscriptionsCollection.InsertOne(ctx, newSubscription)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to create subscription",
			})
		}

		// Update wholesaler status to active
		wholesalerCollection := sc.DB.Collection("wholesalers")
		_, err = wholesalerCollection.UpdateOne(ctx, bson.M{"_id": subscriptionRequest.WholesalerID}, bson.M{"$set": bson.M{"status": "active"}})
		if err != nil {
			log.Printf("Failed to update wholesaler status to active: %v", err)
		}

		// Update all branches status to active when wholesaler subscription is approved
		if len(wholesaler.Branches) > 0 {
			// Try to update all branches using positional operator
			for _, branch := range wholesaler.Branches {
				_, err = wholesalerCollection.UpdateOne(
					ctx,
					bson.M{
						"_id":          subscriptionRequest.WholesalerID,
						"branches._id": branch.ID,
					},
					bson.M{
						"$set": bson.M{
							"branches.$.status":    "active",
							"branches.$.updatedAt": time.Now(),
						},
					},
				)
				if err != nil {
					log.Printf("Failed to update branch status to active using positional operator: %v", err)
					// Try alternative approach if the first one fails
					var updatedWholesaler models.Wholesaler
					err2 := wholesalerCollection.FindOne(ctx, bson.M{"_id": subscriptionRequest.WholesalerID}).Decode(&updatedWholesaler)
					if err2 == nil {
						// Update the branch status in the branches array
						for i, b := range updatedWholesaler.Branches {
							if b.ID == branch.ID {
								updatedWholesaler.Branches[i].Status = "active"
								updatedWholesaler.Branches[i].UpdatedAt = time.Now()
								break
							}
						}

						// Update the entire wholesaler document
						_, err3 := wholesalerCollection.ReplaceOne(ctx, bson.M{"_id": subscriptionRequest.WholesalerID}, updatedWholesaler)
						if err3 != nil {
							log.Printf("Failed to update branch status using alternative approach: %v", err3)
						} else {
							log.Printf("Successfully updated branch status using alternative approach")
						}
					} else {
						log.Printf("Failed to find wholesaler for alternative update approach: %v", err2)
					}
				} else {
					log.Printf("Successfully updated branch status to active in wholesaler collection")
				}
			}
		}

		// Update user status to active when subscription is approved
		usersCollection := sc.DB.Collection("users")
		_, err = usersCollection.UpdateOne(
			ctx,
			bson.M{"wholesalerId": subscriptionRequest.WholesalerID},
			bson.M{"$set": bson.M{"status": "active", "updatedAt": time.Now()}},
		)
		if err != nil {
			log.Printf("Failed to update user status: %v", err)
		}

		subscription = &newSubscription

		// --- Commission logic start ---
		// Check if wholesaler was created by a salesperson or by itself (user signup)
		log.Printf("DEBUG: Wholesaler CreatedBy field: %v (IsZero: %v)", wholesaler.CreatedBy, wholesaler.CreatedBy.IsZero())

		// Check if wholesaler was created by itself (user signup) - CreatedBy equals UserID
		if wholesaler.CreatedBy == wholesaler.UserID {
			log.Printf("DEBUG: Wholesaler was created by user signup, adding subscription price to admin wallet")
			// Add subscription price directly to admin wallet (no commission calculation needed)
			log.Printf("Subscription income added to admin wallet: $%.2f from wholesaler '%s' (ID: %s) - User signup subscription",
				plan.Price, wholesaler.BusinessName, wholesaler.ID.Hex())
		} else if !wholesaler.CreatedBy.IsZero() {
			log.Printf("DEBUG: Wholesaler was created by salesperson, proceeding with commission calculation")
			// Get salesperson
			var salesperson models.Salesperson
			err := sc.DB.Collection("salespersons").FindOne(ctx, bson.M{"_id": wholesaler.CreatedBy}).Decode(&salesperson)
			if err == nil {
				log.Printf("DEBUG: Found salesperson: %s (ID: %v)", salesperson.FullName, salesperson.ID)
				// Get sales manager - try both collection names due to inconsistency
				var salesManager models.SalesManager
				err := sc.DB.Collection("sales_managers").FindOne(ctx, bson.M{"_id": salesperson.SalesManagerID}).Decode(&salesManager)
				if err != nil {
					// Try the alternative collection name
					log.Printf("DEBUG: Sales manager not found in sales_managers collection, trying salesManagers collection")
					err = sc.DB.Collection("salesManagers").FindOne(ctx, bson.M{"_id": salesperson.SalesManagerID}).Decode(&salesManager)
				}
				if err == nil {
					log.Printf("DEBUG: Found sales manager: %s (ID: %v)", salesManager.FullName, salesManager.ID)
					planPrice := plan.Price
					salespersonPercent := salesperson.CommissionPercent
					salesManagerPercent := salesManager.CommissionPercent

					log.Printf("DEBUG: Plan price: $%.2f, Salesperson commission percent: %.2f%%, Sales Manager commission percent: %.2f%%",
						planPrice, salespersonPercent, salesManagerPercent)

					// Calculate commissions correctly
					// Salesperson gets their percentage directly from the plan price
					salespersonCommission := planPrice * salespersonPercent / 100.0
					// Sales manager gets their percentage directly from the plan price
					salesManagerCommission := planPrice * salesManagerPercent / 100.0

					log.Printf("DEBUG: Calculated commissions - Sales Manager: $%.2f, Salesperson: $%.2f",
						salesManagerCommission, salespersonCommission)

					// Insert commission document using Commission model
					commission := models.Commission{
						ID:                            primitive.NewObjectID(),
						SubscriptionID:                newSubscription.ID,
						CompanyID:                     primitive.NilObjectID, // No company ID for wholesaler
						PlanID:                        plan.ID,
						PlanPrice:                     planPrice,
						SalespersonID:                 salesperson.ID,
						SalespersonCommission:         salespersonCommission,
						SalespersonCommissionPercent:  salespersonPercent,
						SalesManagerID:                salesManager.ID,
						SalesManagerCommission:        salesManagerCommission,
						SalesManagerCommissionPercent: salesManagerPercent,
						CreatedAt:                     time.Now(),
						Paid:                          false,
						PaidAt:                        nil,
					}
					_, err := sc.DB.Collection("commissions").InsertOne(ctx, commission)
					if err != nil {
						log.Printf("Failed to insert commission: %v", err)
					} else {
						log.Printf("Commission inserted successfully for wholesaler subscription - Plan Price: $%.2f, Sales Manager Commission: $%.2f, Salesperson Commission: $%.2f",
							planPrice, salesManagerCommission, salespersonCommission)
					}
				} else {
					log.Printf("DEBUG: Failed to find sales manager for ID: %v, Error: %v", salesperson.SalesManagerID, err)
				}
			} else {
				log.Printf("DEBUG: Failed to find salesperson for ID: %v, Error: %v", wholesaler.CreatedBy, err)
			}
		} else {
			log.Printf("DEBUG: Wholesaler was not created by a salesperson (CreatedBy is zero or not set)")
		}
		// --- Commission logic end ---

		// Send approval notification
		emailSubject := "Wholesaler Subscription Request Approved! 🎉"
		log.Printf("Wholesaler subscription approved notification: %s - %s", wholesaler.BusinessName, emailSubject)

		// Send final approval notification
		if err := sc.sendWholesalerNotificationEmail(
			wholesaler.Phone,
			"Subscription Approved",
			fmt.Sprintf("Your subscription request has been approved. Your subscription is active until %s.", endDate.Format("2006-01-02")),
		); err != nil {
			log.Printf("Failed to send wholesaler notification email: %v", err)
		}
	} else {
		// If rejected, update all branches status to inactive
		wholesalerCollection := sc.DB.Collection("wholesalers")
		// Try to update all branches using positional operator
		for _, branch := range wholesaler.Branches {
			_, err = wholesalerCollection.UpdateOne(
				ctx,
				bson.M{
					"_id":          subscriptionRequest.WholesalerID,
					"branches._id": branch.ID,
				},
				bson.M{
					"$set": bson.M{
						"branches.$.status":    "inactive",
						"branches.$.updatedAt": time.Now(),
					},
				},
			)
			if err != nil {
				log.Printf("Failed to update branch status to inactive using positional operator: %v", err)
				// Try alternative approach if the first one fails
				var updatedWholesaler models.Wholesaler
				err2 := wholesalerCollection.FindOne(ctx, bson.M{"_id": subscriptionRequest.WholesalerID}).Decode(&updatedWholesaler)
				if err2 == nil {
					// Update the branch status in the branches array
					for i, b := range updatedWholesaler.Branches {
						if b.ID == branch.ID {
							updatedWholesaler.Branches[i].Status = "inactive"
							updatedWholesaler.Branches[i].UpdatedAt = time.Now()
							break
						}
					}

					// Update the entire wholesaler document
					_, err3 := wholesalerCollection.ReplaceOne(ctx, bson.M{"_id": subscriptionRequest.WholesalerID}, updatedWholesaler)
					if err3 != nil {
						log.Printf("Failed to update branch status to inactive using alternative approach: %v", err3)
					} else {
						log.Printf("Successfully updated branch status to inactive using alternative approach")
					}
				} else {
					log.Printf("Failed to find wholesaler for alternative update approach (rejection): %v", err2)
				}
			} else {
				log.Printf("Successfully updated branch status to inactive in wholesaler collection")
			}
		}

		// Send rejection notification
		emailSubject := "Wholesaler Subscription Request Update"
		log.Printf("Wholesaler subscription rejected notification: %s - %s", wholesaler.BusinessName, emailSubject)

		// Send rejection notification to wholesaler
		if err := sc.sendWholesalerNotificationEmail(
			wholesaler.Phone,
			"Subscription Request Rejected",
			fmt.Sprintf("Your subscription request has been rejected. Reason: %s", approvalReq.AdminNote),
		); err != nil {
			log.Printf("Failed to send wholesaler notification email: %v", err)
		}
	}

	// Prepare response data
	responseData := map[string]interface{}{
		"requestId":      subscriptionRequest.ID,
		"wholesalerName": wholesaler.BusinessName,
		"planName":       plan.Title,
		"status":         approvalReq.Status,
		"processedAt":    time.Now(),
		"adminNote":      approvalReq.AdminNote,
		"subscription":   subscription,
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: fmt.Sprintf("Wholesaler subscription request %s successfully", approvalReq.Status),
		Data:    responseData,
	})
}

// sendWholesalerNotificationEmail sends a notification email to a wholesaler
func (sc *SubscriptionController) sendWholesalerNotificationEmail(phone, subject, body string) error {
	// TODO: Implement actual email sending to wholesaler
	// For now, just log the notification
	log.Printf("Wholesaler notification (to %s): %s - %s", phone, subject, body)
	return nil
}

// CreateUserManager allows an admin to create a user manager
func (ac *AdminController) CreateUserManager(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins can create user managers",
		})
	}

	var req struct {
		FullName string `json:"fullName"`
		Email    string `json:"email"`
		Password string `json:"password"`
		// RolesAccess []string `json:"rolesAccess"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}

	// Validate email and password
	if !isValidEmail(req.Email) || !isStrongPassword(req.Password) {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid email or weak password",
		})
	}

	// Validate rolesAccess
	// if len(req.RolesAccess) == 0 {
	// 	return c.JSON(http.StatusBadRequest, models.Response{
	// 		Status:  http.StatusBadRequest,
	// 		Message: "At least one access role must be selected",
	// 	})
	// }
	// // Check if all provided rolesAccess keys exist in AccessRole collection
	// for _, key := range req.RolesAccess {
	// 	count, err := ac.DB.Collection("access_roles").CountDocuments(context.Background(), bson.M{"key": key})
	// 	if err != nil || count == 0 {
	// 		return c.JSON(http.StatusBadRequest, models.Response{
	// 			Status:  http.StatusBadRequest,
	// 			Message: "Invalid access role: " + key,
	// 		})
	// 	}
	// }

	// Check for existing user manager
	var existingManager models.Manager
	err := ac.DB.Collection("managers").FindOne(context.Background(), bson.M{"email": req.Email}).Decode(&existingManager)
	if err == nil {
		return c.JSON(http.StatusConflict, models.Response{
			Status:  http.StatusConflict,
			Message: "Email already exists",
		})
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to hash password",
		})
	}

	manager := models.Manager{
		ID:        primitive.NewObjectID(),
		FullName:  req.FullName,
		Email:     req.Email,
		Password:  string(hashedPassword),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		// RolesAccess: req.RolesAccess,
	}

	_, err = ac.DB.Collection("managers").InsertOne(context.Background(), manager)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to create user manager",
		})
	}

	manager.Password = ""
	return c.JSON(http.StatusCreated, models.Response{
		Status:  http.StatusCreated,
		Message: "User manager created successfully",
		Data:    manager,
	})
}

// ListAccessRoles returns all available access roles
func (ac *AdminController) ListAccessRoles(c echo.Context) error {
	cursor, err := ac.DB.Collection("access_roles").Find(context.Background(), bson.M{})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch access roles",
		})
	}
	defer cursor.Close(context.Background())
	var roles []models.AccessRole
	if err := cursor.All(context.Background(), &roles); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode access roles",
		})
	}
	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Access roles retrieved successfully",
		Data:    roles,
	})
}

// Approve a company by manager
func (ac *AdminController) ApproveCompanyByManager(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	if claims.UserType == "admin" {
		// allow
	} else if claims.UserType == "manager" {
		// fetch manager, check hasRole(manager.RolesAccess, ...)
		var manager models.Manager
		err := ac.DB.Collection("managers").FindOne(context.Background(), bson.M{"_id": claims.UserID}).Decode(&manager)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to fetch manager",
			})
		}
		if !hasRole(manager.RolesAccess, "financial_dashboard_revenue") {
			return c.JSON(http.StatusForbidden, models.Response{
				Status:  http.StatusForbidden,
				Message: "Manager does not have financial_dashboard_revenue role",
			})
		}
	} else if claims.UserType == "sales_manager" {
		var salesManager models.SalesManager
		err := ac.DB.Collection("sales_managers").FindOne(context.Background(), bson.M{"_id": claims.UserID}).Decode(&salesManager)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to fetch sales manager",
			})
		}
		if !hasRole(salesManager.RolesAccess, "financial_dashboard_revenue") {
			return c.JSON(http.StatusForbidden, models.Response{
				Status:  http.StatusForbidden,
				Message: "Sales manager does not have financial_dashboard_revenue role",
			})
		}
	} else {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins, managers, or sales managers can approve companies",
		})
	}
	companyID := c.Param("id")
	objID, err := primitive.ObjectIDFromHex(companyID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid company ID",
		})
	}
	collection := ac.DB.Collection("companies")
	ctx := context.Background()
	update := bson.M{"$set": bson.M{"status": "approved", "updatedAt": time.Now()}}
	result, err := collection.UpdateOne(ctx, bson.M{"_id": objID}, update)
	if err != nil || result.MatchedCount == 0 {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to approve company",
		})
	}

	// Handle user account creation or update
	usersCollection := ac.DB.Collection("users")

	// First, try to update existing user status
	result, err = usersCollection.UpdateOne(
		ctx,
		bson.M{"companyId": objID},
		bson.M{"$set": bson.M{"isActive": true, "updatedAt": time.Now()}},
	)
	if err != nil {
		log.Printf("Failed to update user status: %v", err)
	}

	// If no user was found, create one
	if result.MatchedCount == 0 {
		// Get the company details to create user account
		var company bson.M
		err = collection.FindOne(ctx, bson.M{"_id": objID}).Decode(&company)
		if err == nil {
			// Create user account if email exists
			if email, exists := company["email"]; exists && email != "" {
				emailStr := email.(string)

				// Check if user already exists by email
				var existingUser bson.M
				err = usersCollection.FindOne(ctx, bson.M{"email": emailStr}).Decode(&existingUser)
				if err == mongo.ErrNoDocuments {
					// User doesn't exist, create one
					fullName := ""
					phone := ""
					contactPerson := ""
					contactPhone := ""

					// Get business name as full name
					if v, ok := company["businessName"].(string); ok {
						fullName = v
					}

					// Get phone and contact info from contactInfo
					if contactInfo, ok := company["contactInfo"].(bson.M); ok {
						if v, ok := contactInfo["phone"].(string); ok {
							phone = v
						}
						if v, ok := contactInfo["whatsapp"].(string); ok {
							contactPhone = v
						}
					}

					if v, ok := company["contactPerson"].(string); ok {
						contactPerson = v
					}

					// Get password (it should exist in the company record)
					password := ""
					if v, ok := company["password"].(string); ok {
						password = v
					}

					// Create user document
					userDoc := bson.M{
						"email":         emailStr,
						"password":      password,
						"fullName":      fullName,
						"userType":      "company",
						"phone":         phone,
						"contactPerson": contactPerson,
						"contactPhone":  contactPhone,
						"companyId":     objID,
						"isActive":      true,
						"createdAt":     time.Now(),
						"updatedAt":     time.Now(),
					}

					insertRes, err := usersCollection.InsertOne(ctx, userDoc)
					if err != nil {
						log.Printf("Error creating user account for company: %v", err)
						// Don't fail the approval if user creation fails
					} else {
						// Update the company with the user ID for easy lookup
						if userOID, ok := insertRes.InsertedID.(primitive.ObjectID); ok {
							_, updErr := collection.UpdateOne(ctx, bson.M{"_id": objID}, bson.M{"$set": bson.M{"userId": userOID}})
							if updErr != nil {
								log.Printf("Warning: failed to set userId on company document: %v", updErr)
							}
						}
						log.Printf("User account created successfully for company: %s", emailStr)
					}
				} else if err != nil {
					log.Printf("Error checking existing user: %v", err)
				} else {
					log.Printf("User account already exists for company: %s", emailStr)
				}
			}
		}
	}
	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Company approved successfully",
	})
}

// Approve a service provider by manager
func (ac *AdminController) ApproveServiceProviderByManager(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	if claims.UserType == "admin" {
		// allow
	} else if claims.UserType == "manager" {
		var manager models.Manager
		err := ac.DB.Collection("managers").FindOne(context.Background(), bson.M{"_id": claims.UserID}).Decode(&manager)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to fetch manager",
			})
		}
		if !hasRole(manager.RolesAccess, "financial_dashboard_revenue") {
			return c.JSON(http.StatusForbidden, models.Response{
				Status:  http.StatusForbidden,
				Message: "Manager does not have financial_dashboard_revenue role",
			})
		}
	} else if claims.UserType == "sales_manager" {
		var salesManager models.SalesManager
		err := ac.DB.Collection("sales_managers").FindOne(context.Background(), bson.M{"_id": claims.UserID}).Decode(&salesManager)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to fetch sales manager",
			})
		}
		if !hasRole(salesManager.RolesAccess, "financial_dashboard_revenue") {
			return c.JSON(http.StatusForbidden, models.Response{
				Status:  http.StatusForbidden,
				Message: "Sales manager does not have financial_dashboard_revenue role",
			})
		}
	} else {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins, managers, or sales managers can approve service providers",
		})
	}
	spID := c.Param("id")
	objID, err := primitive.ObjectIDFromHex(spID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid service provider ID",
		})
	}
	collection := ac.DB.Collection("serviceProviders")
	ctx := context.Background()

	// Get the service provider details first
	var serviceProvider bson.M
	err = collection.FindOne(ctx, bson.M{"_id": objID}).Decode(&serviceProvider)
	if err != nil {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Service provider not found",
		})
	}

	// Update service provider status
	update := bson.M{"$set": bson.M{"status": "approved", "updatedAt": time.Now()}}
	result, err := collection.UpdateOne(ctx, bson.M{"_id": objID}, update)
	if err != nil || result.MatchedCount == 0 {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to approve service provider",
		})
	}

	// Create user account if email exists and user doesn't already exist
	if email, exists := serviceProvider["email"]; exists && email != "" {
		emailStr := email.(string)

		// Check if user already exists
		usersCollection := ac.DB.Collection("users")
		var existingUser bson.M
		err = usersCollection.FindOne(ctx, bson.M{"email": emailStr}).Decode(&existingUser)
		if err == mongo.ErrNoDocuments {
			// User doesn't exist, create one
			fullName := ""
			phone := ""
			contactPerson := ""
			contactPhone := ""

			// Get business name as full name
			if v, ok := serviceProvider["businessName"].(string); ok {
				fullName = v
			}

			// Get phone and contact info
			if v, ok := serviceProvider["phone"].(string); ok {
				phone = v
			}
			if v, ok := serviceProvider["contactPerson"].(string); ok {
				contactPerson = v
			}
			if v, ok := serviceProvider["contactPhone"].(string); ok {
				contactPhone = v
			}

			// Get password (it should exist in the service provider record)
			password := ""
			if v, ok := serviceProvider["password"].(string); ok {
				password = v
			}

			// Create user document
			userDoc := bson.M{
				"email":             emailStr,
				"password":          password,
				"fullName":          fullName,
				"userType":          "serviceProvider",
				"phone":             phone,
				"contactPerson":     contactPerson,
				"contactPhone":      contactPhone,
				"serviceProviderId": objID,
				"isActive":          true,
				"createdAt":         time.Now(),
				"updatedAt":         time.Now(),
			}

			insertRes, err := usersCollection.InsertOne(ctx, userDoc)
			if err != nil {
				log.Printf("Error creating user account for service provider: %v", err)
				// Don't fail the approval if user creation fails
			} else {
				// Update the service provider with the user ID for easy lookup
				if userOID, ok := insertRes.InsertedID.(primitive.ObjectID); ok {
					_, updErr := collection.UpdateOne(ctx, bson.M{"_id": objID}, bson.M{"$set": bson.M{"userId": userOID}})
					if updErr != nil {
						log.Printf("Warning: failed to set userId on service provider document: %v", updErr)
					}
				}
				log.Printf("User account created successfully for service provider: %s", emailStr)
			}
		} else if err != nil {
			log.Printf("Error checking existing user: %v", err)
		} else {
			log.Printf("User account already exists for service provider: %s", emailStr)
		}
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Service provider approved successfully",
	})
}

// Approve a wholesaler by manager
func (ac *AdminController) ApproveWholesalerByManager(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	if claims.UserType == "admin" {
		// allow
	} else if claims.UserType == "manager" {
		var manager models.Manager
		err := ac.DB.Collection("managers").FindOne(context.Background(), bson.M{"_id": claims.UserID}).Decode(&manager)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to fetch manager",
			})
		}
		if !hasRole(manager.RolesAccess, "financial_dashboard_revenue") {
			return c.JSON(http.StatusForbidden, models.Response{
				Status:  http.StatusForbidden,
				Message: "Manager does not have financial_dashboard_revenue role",
			})
		}
	} else if claims.UserType == "sales_manager" {
		var salesManager models.SalesManager
		err := ac.DB.Collection("sales_managers").FindOne(context.Background(), bson.M{"_id": claims.UserID}).Decode(&salesManager)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to fetch sales manager",
			})
		}
		if !hasRole(salesManager.RolesAccess, "financial_dashboard_revenue") {
			return c.JSON(http.StatusForbidden, models.Response{
				Status:  http.StatusForbidden,
				Message: "Sales manager does not have financial_dashboard_revenue role",
			})
		}
	} else {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins, managers, or sales managers can approve wholesalers",
		})
	}
	wholesalerID := c.Param("id")
	objID, err := primitive.ObjectIDFromHex(wholesalerID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid wholesaler ID",
		})
	}
	collection := ac.DB.Collection("wholesalers")
	ctx := context.Background()

	// Get the wholesaler details first
	var wholesaler bson.M
	err = collection.FindOne(ctx, bson.M{"_id": objID}).Decode(&wholesaler)
	if err != nil {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Wholesaler not found",
		})
	}

	// Update wholesaler status
	update := bson.M{"$set": bson.M{"status": "approved", "updatedAt": time.Now()}}
	result, err := collection.UpdateOne(ctx, bson.M{"_id": objID}, update)
	if err != nil || result.MatchedCount == 0 {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to approve wholesaler",
		})
	}

	// Create user account if email exists and user doesn't already exist
	if email, exists := wholesaler["email"]; exists && email != "" {
		emailStr := email.(string)

		// Check if user already exists
		usersCollection := ac.DB.Collection("users")
		var existingUser bson.M
		err = usersCollection.FindOne(ctx, bson.M{"email": emailStr}).Decode(&existingUser)
		if err == mongo.ErrNoDocuments {
			// User doesn't exist, create one
			fullName := ""
			phone := ""
			contactPerson := ""
			contactPhone := ""

			// Get business name as full name
			if v, ok := wholesaler["businessName"].(string); ok {
				fullName = v
			}

			// Get phone and contact info from contactInfo or direct fields
			if contactInfo, ok := wholesaler["contactInfo"].(bson.M); ok {
				if v, ok := contactInfo["phone"].(string); ok {
					phone = v
				}
				if v, ok := contactInfo["whatsapp"].(string); ok {
					contactPhone = v
				}
			}
			// Also check direct phone field for wholesalers
			if phone == "" {
				if v, ok := wholesaler["phone"].(string); ok {
					phone = v
				}
			}

			if v, ok := wholesaler["contactPerson"].(string); ok {
				contactPerson = v
			}

			// Get password (it should exist in the wholesaler record)
			password := ""
			if v, ok := wholesaler["password"].(string); ok {
				password = v
			}

			// Create user document
			userDoc := bson.M{
				"email":         emailStr,
				"password":      password,
				"fullName":      fullName,
				"userType":      "wholesaler",
				"phone":         phone,
				"contactPerson": contactPerson,
				"contactPhone":  contactPhone,
				"wholesalerId":  objID,
				"isActive":      true,
				"createdAt":     time.Now(),
				"updatedAt":     time.Now(),
			}

			insertRes, err := usersCollection.InsertOne(ctx, userDoc)
			if err != nil {
				log.Printf("Error creating user account for wholesaler: %v", err)
				// Don't fail the approval if user creation fails
			} else {
				// Update the wholesaler with the user ID for easy lookup
				if userOID, ok := insertRes.InsertedID.(primitive.ObjectID); ok {
					_, updErr := collection.UpdateOne(ctx, bson.M{"_id": objID}, bson.M{"$set": bson.M{"userId": userOID}})
					if updErr != nil {
						log.Printf("Warning: failed to set userId on wholesaler document: %v", updErr)
					}
				}
				log.Printf("User account created successfully for wholesaler: %s", emailStr)
			}
		} else if err != nil {
			log.Printf("Error checking existing user: %v", err)
		} else {
			log.Printf("User account already exists for wholesaler: %s", emailStr)
		}
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Wholesaler approved successfully",
	})
}

// GetManager retrieves a specific manager by ID
// Note: This function retrieves a Manager entity (higher-level admin role)
// To get salespersons created by a SalesManager, use GetSalesManager instead
func (ac *AdminController) GetManager(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins can access this resource",
		})
	}
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid ID format",
		})
	}

	var manager models.Manager
	err = ac.DB.Collection("managers").FindOne(context.Background(), bson.M{"_id": id}).Decode(&manager)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Manager not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch manager",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Manager retrieved successfully (Note: Use GetSalesManager to get salespersons)",
		Data:    manager,
	})
}

// UpdateManager updates a specific manager by ID
func (ac *AdminController) UpdateManager(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins can update managers",
		})
	}
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid ID format",
		})
	}

	var updates models.Manager
	if err := c.Bind(&updates); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}

	result, err := ac.DB.Collection("managers").UpdateOne(
		context.Background(),
		bson.M{"_id": id},
		bson.M{"$set": updates},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update manager",
		})
	}

	if result.MatchedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Manager not found",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Manager updated successfully",
	})
}

// DeleteManager deletes a specific manager by ID
func (ac *AdminController) DeleteManager(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins can delete managers",
		})
	}
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid ID format",
		})
	}

	// First, get the manager to find their email
	var manager models.Manager
	err = ac.DB.Collection("managers").FindOne(context.Background(), bson.M{"_id": id}).Decode(&manager)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Manager not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch manager",
		})
	}

	// Delete the manager
	result, err := ac.DB.Collection("managers").DeleteOne(context.Background(), bson.M{"_id": id})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to delete manager",
		})
	}

	if result.DeletedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Manager not found",
		})
	}

	// Delete associated user entry from users collection
	_, err = ac.DB.Collection("users").DeleteOne(context.Background(), bson.M{
		"$or": []bson.M{
			{"email": manager.Email},
			{"userType": "manager", "email": manager.Email},
		},
	})
	if err != nil {
		log.Printf("Failed to delete associated user account for manager %s: %v", manager.Email, err)
	} else {
		log.Printf("Successfully deleted associated user account for manager %s", manager.Email)
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Manager deleted successfully",
	})
}

// GetAllManagers retrieves all managers
func (ac *AdminController) GetAllManagers(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins can access this resource",
		})
	}
	var managers []models.Manager
	cursor, err := ac.DB.Collection("managers").Find(context.Background(), bson.M{})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch managers",
		})
	}
	defer cursor.Close(context.Background())

	if err := cursor.All(context.Background(), &managers); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode managers",
		})
	}

	// Remove passwords from response
	for i := range managers {
		managers[i].Password = ""
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Managers retrieved successfully",
		Data:    managers,
	})
}

// hasRole checks if the manager has the required role
func hasRole(roles []string, required string) bool {
	for _, r := range roles {
		if r == required {
			return true
		}
	}
	return false
}

// CreateUser allows admin or manager/sales_manager (with user_management role) to create a user
func (ac *AdminController) CreateUser(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	if claims.UserType == "admin" {
		// allow
	} else if claims.UserType == "manager" {
		// Fetch manager from DB
		var manager models.Manager
		err := ac.DB.Collection("managers").FindOne(context.Background(), bson.M{"_id": claims.UserID}).Decode(&manager)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to fetch manager",
			})
		}
		if !hasRole(manager.RolesAccess, "user_management") {
			return c.JSON(http.StatusForbidden, models.Response{
				Status:  http.StatusForbidden,
				Message: "Manager does not have user_management role",
			})
		}
	} else if claims.UserType == "sales_manager" {
		// Fetch sales manager from DB
		var salesManager models.SalesManager
		err := ac.DB.Collection("sales_managers").FindOne(context.Background(), bson.M{"_id": claims.UserID}).Decode(&salesManager)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to fetch sales manager",
			})
		}
		if !hasRole(salesManager.RolesAccess, "user_management") {
			return c.JSON(http.StatusForbidden, models.Response{
				Status:  http.StatusForbidden,
				Message: "Sales manager does not have user_management role",
			})
		}
	} else {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins, managers, or sales managers can create users",
		})
	}

	var req struct {
		FullName    string           `json:"fullName"`
		Age         int              `json:"age"`
		Gender      string           `json:"gender"`
		Email       string           `json:"email"`
		Password    string           `json:"password"`
		UserType    string           `json:"userType"`
		Phone       string           `json:"phone"`
		Location    *models.Location `json:"location"`
		DateOfBirth string           `json:"dateOfBirth"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}

	// Validate required fields
	if req.FullName == "" || req.Email == "" || req.Password == "" || req.Phone == "" || req.DateOfBirth == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Missing required fields",
		})
	}
	if req.UserType == "" {
		req.UserType = "user"
	}

	// Check for existing user
	var existingUser models.User
	err := ac.DB.Collection("users").FindOne(context.Background(), bson.M{"email": req.Email}).Decode(&existingUser)
	if err == nil {
		return c.JSON(http.StatusConflict, models.Response{
			Status:  http.StatusConflict,
			Message: "Email already exists",
		})
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to hash password",
		})
	}

	user := models.User{
		ID:          primitive.NewObjectID(),
		FullName:    req.FullName,
		Email:       req.Email,
		Password:    string(hashedPassword),
		UserType:    req.UserType,
		Phone:       req.Phone,
		Gender:      req.Gender,
		DateOfBirth: req.DateOfBirth,
		IsActive:    true,
		Status:      "active", // Admin-created users are active by default
		Location:    req.Location,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	_, err = ac.DB.Collection("users").InsertOne(context.Background(), user)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to create user",
		})
	}

	user.Password = "" // Never return password
	return c.JSON(http.StatusCreated, models.Response{
		Status:  http.StatusCreated,
		Message: "User created successfully",
		Data:    user,
	})
}

// DeleteUser allows admin or manager/sales_manager (with user_management role) to delete a user by ID
func (ac *AdminController) DeleteUser(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	if claims.UserType == "admin" {
		// allow
	} else if claims.UserType == "manager" {
		// Fetch manager from DB
		var manager models.Manager
		err := ac.DB.Collection("managers").FindOne(context.Background(), bson.M{"_id": claims.UserID}).Decode(&manager)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to fetch manager",
			})
		}
		if !hasRole(manager.RolesAccess, "user_management") {
			return c.JSON(http.StatusForbidden, models.Response{
				Status:  http.StatusForbidden,
				Message: "Manager does not have user_management role",
			})
		}
	} else if claims.UserType == "sales_manager" {
		// Fetch sales manager from DB
		var salesManager models.SalesManager
		err := ac.DB.Collection("sales_managers").FindOne(context.Background(), bson.M{"_id": claims.UserID}).Decode(&salesManager)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to fetch sales manager",
			})
		}
		if !hasRole(salesManager.RolesAccess, "user_management") {
			return c.JSON(http.StatusForbidden, models.Response{
				Status:  http.StatusForbidden,
				Message: "Sales manager does not have user_management role",
			})
		}
	} else {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins, managers, or sales managers can delete users",
		})
	}

	userID := c.Param("id")
	if userID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "User ID is required",
		})
	}
	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID format",
		})
	}

	result, err := ac.DB.Collection("users").DeleteOne(context.Background(), bson.M{"_id": objID})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to delete user",
		})
	}
	if result.DeletedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "User not found",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "User deleted successfully",
	})
}

// ActivateCompanyByManager allows manager/sales_manager with business_management role to activate a company
func (ac *AdminController) ActivateCompanyByManager(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	if claims.UserType == "admin" {
		// allow
	} else if claims.UserType == "manager" {
		var manager models.Manager
		err := ac.DB.Collection("managers").FindOne(context.Background(), bson.M{"_id": claims.UserID}).Decode(&manager)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to fetch manager",
			})
		}
		if !hasRole(manager.RolesAccess, "business_management") {
			return c.JSON(http.StatusForbidden, models.Response{
				Status:  http.StatusForbidden,
				Message: "Manager does not have business_management role",
			})
		}
	} else if claims.UserType == "sales_manager" {
		var salesManager models.SalesManager
		err := ac.DB.Collection("sales_managers").FindOne(context.Background(), bson.M{"_id": claims.UserID}).Decode(&salesManager)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to fetch sales manager",
			})
		}
		if !hasRole(salesManager.RolesAccess, "business_management") {
			return c.JSON(http.StatusForbidden, models.Response{
				Status:  http.StatusForbidden,
				Message: "Sales manager does not have business_management role",
			})
		}
	} else {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins, managers, or sales managers can activate companies",
		})
	}

	// Get company ID from URL parameter
	companyID := c.Param("id")
	if companyID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Company ID is required",
		})
	}

	// Update company status to active
	result, err := ac.DB.Collection("companies").UpdateOne(
		context.Background(),
		bson.M{"_id": companyID},
		bson.M{"$set": bson.M{"status": "active"}},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to activate company",
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
		Message: "Company activated successfully",
	})
}

// ActivateWholesalerByManager allows manager/sales_manager with business_management role to activate a wholesaler
func (ac *AdminController) ActivateWholesalerByManager(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	if claims.UserType == "admin" {
		// allow
	} else if claims.UserType == "manager" {
		var manager models.Manager
		err := ac.DB.Collection("managers").FindOne(context.Background(), bson.M{"_id": claims.UserID}).Decode(&manager)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to fetch manager",
			})
		}
		if !hasRole(manager.RolesAccess, "business_management") {
			return c.JSON(http.StatusForbidden, models.Response{
				Status:  http.StatusForbidden,
				Message: "Manager does not have business_management role",
			})
		}
	} else if claims.UserType == "sales_manager" {
		var salesManager models.SalesManager
		err := ac.DB.Collection("sales_managers").FindOne(context.Background(), bson.M{"_id": claims.UserID}).Decode(&salesManager)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to fetch sales manager",
			})
		}
		if !hasRole(salesManager.RolesAccess, "business_management") {
			return c.JSON(http.StatusForbidden, models.Response{
				Status:  http.StatusForbidden,
				Message: "Sales manager does not have business_management role",
			})
		}
	} else {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins, managers, or sales managers can activate wholesalers",
		})
	}

	// Get wholesaler ID from URL parameter
	wholesalerID := c.Param("id")
	if wholesalerID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Wholesaler ID is required",
		})
	}

	// Update wholesaler status to active
	result, err := ac.DB.Collection("wholesalers").UpdateOne(
		context.Background(),
		bson.M{"_id": wholesalerID},
		bson.M{"$set": bson.M{"status": "active"}},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to activate wholesaler",
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
		Message: "Wholesaler activated successfully",
	})
}

// ActivateServiceProviderByManager allows manager/sales_manager with business_management role to activate a service provider
func (ac *AdminController) ActivateServiceProviderByManager(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	if claims.UserType == "admin" {
		// allow
	} else if claims.UserType == "manager" {
		var manager models.Manager
		err := ac.DB.Collection("managers").FindOne(context.Background(), bson.M{"_id": claims.UserID}).Decode(&manager)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to fetch manager",
			})
		}
		if !hasRole(manager.RolesAccess, "business_management") {
			return c.JSON(http.StatusForbidden, models.Response{
				Status:  http.StatusForbidden,
				Message: "Manager does not have business_management role",
			})
		}
	} else if claims.UserType == "sales_manager" {
		var salesManager models.SalesManager
		err := ac.DB.Collection("sales_managers").FindOne(context.Background(), bson.M{"_id": claims.UserID}).Decode(&salesManager)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to fetch sales manager",
			})
		}
		if !hasRole(salesManager.RolesAccess, "business_management") {
			return c.JSON(http.StatusForbidden, models.Response{
				Status:  http.StatusForbidden,
				Message: "Sales manager does not have business_management role",
			})
		}
	} else {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins, managers, or sales managers can activate service providers",
		})
	}

	// Get service provider ID from URL parameter
	serviceProviderID := c.Param("id")
	if serviceProviderID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Service provider ID is required",
		})
	}

	// Update service provider status to active
	result, err := ac.DB.Collection("serviceProviders").UpdateOne(
		context.Background(),
		bson.M{"_id": serviceProviderID},
		bson.M{"$set": bson.M{"status": "active"}},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to activate service provider",
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
		Message: "Service provider activated successfully",
	})
}

// GetAllEntities returns all users, companies, service providers, and wholesalers in one response
func (ac *AdminController) GetAllEntities(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "admin" && claims.UserType != "manager" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins and managers can access this resource",
		})
	}

	db := ac.DB
	ctx := c.Request().Context()

	// Users
	userOpts := options.Find().SetProjection(bson.M{"password": 0})
	userCursor, err := db.Collection("users").Find(ctx, bson.M{}, userOpts)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{Status: http.StatusInternalServerError, Message: "Failed to fetch users"})
	}
	var users []models.User
	if err := userCursor.All(ctx, &users); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{Status: http.StatusInternalServerError, Message: "Failed to decode users"})
	}
	for i := range users {
		users[i].OTPInfo = nil
		users[i].ResetPasswordToken = ""
	}

	// Companies
	companyCursor, err := db.Collection("companies").Find(ctx, bson.M{})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{Status: http.StatusInternalServerError, Message: "Failed to fetch companies"})
	}
	var companies []models.Company
	if err := companyCursor.All(ctx, &companies); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{Status: http.StatusInternalServerError, Message: "Failed to decode companies"})
	}

	// Service Providers (from users collection with userType)
	spOpts := options.Find().SetProjection(bson.M{"password": 0})
	spCursor, err := db.Collection("users").Find(ctx, bson.M{"userType": "serviceProvider"}, spOpts)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{Status: http.StatusInternalServerError, Message: "Failed to fetch service providers"})
	}
	var serviceProviders []models.User
	if err := spCursor.All(ctx, &serviceProviders); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{Status: http.StatusInternalServerError, Message: "Failed to decode service providers"})
	}
	for i := range serviceProviders {
		serviceProviders[i].OTPInfo = nil
		serviceProviders[i].ResetPasswordToken = ""
	}

	// Wholesalers
	wholesalerCursor, err := db.Collection("wholesalers").Find(ctx, bson.M{})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{Status: http.StatusInternalServerError, Message: "Failed to fetch wholesalers"})
	}
	var wholesalers []models.Wholesaler
	if err := wholesalerCursor.All(ctx, &wholesalers); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{Status: http.StatusInternalServerError, Message: "Failed to decode wholesalers"})
	}

	// Extract all branches from companies
	var allBranches []models.Branch
	for _, company := range companies {
		allBranches = append(allBranches, company.Branches...)
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "All entities retrieved successfully",
		Data: map[string]interface{}{
			"users":            users,
			"companies":        companies,
			"serviceProviders": serviceProviders,
			"wholesalers":      wholesalers,
			"branches":         allBranches,
		},
	})
}

// DeleteEntity allows admin or manager/sales_manager (with user_management role) to delete entities by ID
func (ac *AdminController) DeleteEntity(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	if claims.UserType == "admin" {
		// allow
	} else if claims.UserType == "manager" {
		var manager models.Manager
		err := ac.DB.Collection("managers").FindOne(context.Background(), bson.M{"_id": claims.UserID}).Decode(&manager)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to fetch manager",
			})
		}
		if !hasRole(manager.RolesAccess, "user_management") {
			return c.JSON(http.StatusForbidden, models.Response{
				Status:  http.StatusForbidden,
				Message: "Manager does not have user_management role",
			})
		}
	} else if claims.UserType == "sales_manager" {
		var salesManager models.SalesManager
		err := ac.DB.Collection("sales_managers").FindOne(context.Background(), bson.M{"_id": claims.UserID}).Decode(&salesManager)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to fetch sales manager",
			})
		}
		if !hasRole(salesManager.RolesAccess, "user_management") {
			return c.JSON(http.StatusForbidden, models.Response{
				Status:  http.StatusForbidden,
				Message: "Sales manager does not have user_management role",
			})
		}
	} else {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins, managers, or sales managers can delete entities",
		})
	}

	entityType := c.Param("entityType") // "user", "company", "wholesaler", "serviceProvider"
	entityID := c.Param("id")
	if entityType == "" || entityID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Entity type and ID are required",
		})
	}

	objID, err := primitive.ObjectIDFromHex(entityID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid entity ID format",
		})
	}

	var collectionName string
	switch entityType {
	case "user":
		collectionName = "users"
	case "company":
		collectionName = "companies"
	case "wholesaler":
		collectionName = "wholesalers"
	case "serviceProvider", "service_provider":
		collectionName = "serviceProviders"
	default:
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid entity type",
		})
	}

	collection := ac.DB.Collection(collectionName)

	// Add debugging information
	log.Printf("Attempting to delete %s with ID: %s from collection: %s", entityType, entityID, collectionName)

	// Check if entity exists before deletion
	var existingEntity bson.M
	err = collection.FindOne(context.Background(), bson.M{"_id": objID}).Decode(&existingEntity)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			log.Printf("Entity not found: %s with ID: %s in collection: %s", entityType, entityID, collectionName)
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: fmt.Sprintf("%s not found with ID: %s", entityType, entityID),
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to check if entity exists",
		})
	}

	log.Printf("Entity found, proceeding with deletion")

	// Delete the entity
	result, err := collection.DeleteOne(context.Background(), bson.M{"_id": objID})
	if err != nil {
		log.Printf("Failed to delete entity: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to delete entity",
		})
	}
	if result.DeletedCount == 0 {
		log.Printf("No entity was deleted despite being found")
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Entity not found",
		})
	}

	log.Printf("Successfully deleted %s with ID: %s", entityType, entityID)

	// Delete associated branches for companies and wholesalers
	if entityType == "company" || entityType == "wholesaler" {
		branchesCollection := ac.DB.Collection("branches")
		branchFilter := bson.M{}
		if entityType == "company" {
			branchFilter = bson.M{"companyId": objID}
		} else if entityType == "wholesaler" {
			branchFilter = bson.M{"wholesalerId": objID}
		}

		// Delete branches from separate branches collection
		branchResult, err := branchesCollection.DeleteMany(context.Background(), branchFilter)
		if err != nil {
			log.Printf("Failed to delete branches: %v", err)
		} else {
			log.Printf("Deleted %d branches for %s", branchResult.DeletedCount, entityType)
		}
	}

	// Delete associated user account for companies and wholesalers
	if entityType == "company" || entityType == "wholesaler" {
		usersCollection := ac.DB.Collection("users")
		userFilter := bson.M{}
		if entityType == "company" {
			userFilter = bson.M{"companyId": objID}
		} else if entityType == "wholesaler" {
			userFilter = bson.M{"wholesalerId": objID}
		}

		// Delete associated user account
		userResult, err := usersCollection.DeleteOne(context.Background(), userFilter)
		if err != nil {
			log.Printf("Failed to delete associated user account: %v", err)
		} else if userResult.DeletedCount > 0 {
			log.Printf("Deleted associated user account for %s", entityType)
		}
	}

	// Delete associated user account for service providers
	if entityType == "serviceProvider" || entityType == "service_provider" {
		usersCollection := ac.DB.Collection("users")
		userFilter := bson.M{"serviceProviderId": objID}

		userResult, err := usersCollection.DeleteOne(context.Background(), userFilter)
		if err != nil {
			log.Printf("Failed to delete associated user account for service provider: %v", err)
		} else if userResult.DeletedCount > 0 {
			log.Printf("Deleted associated user account for service provider")
		}
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: fmt.Sprintf("%s deleted successfully", entityType),
	})
}

// DebugEntity helps debug entity deletion issues by checking if an entity exists
func (ac *AdminController) DebugEntity(c echo.Context) error {
	entityType := c.Param("entityType")
	entityID := c.Param("id")

	if entityType == "" || entityID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Entity type and ID are required",
		})
	}

	objID, err := primitive.ObjectIDFromHex(entityID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid entity ID format",
		})
	}

	var collectionName string
	switch entityType {
	case "user":
		collectionName = "users"
	case "company":
		collectionName = "companies"
	case "wholesaler":
		collectionName = "wholesalers"
	case "serviceProvider", "service_provider":
		collectionName = "serviceProviders"
	default:
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid entity type",
		})
	}

	collection := ac.DB.Collection(collectionName)

	// Check if entity exists
	var entity bson.M
	err = collection.FindOne(context.Background(), bson.M{"_id": objID}).Decode(&entity)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: fmt.Sprintf("%s not found with ID: %s in collection: %s", entityType, entityID, collectionName),
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to check if entity exists",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: fmt.Sprintf("%s found with ID: %s in collection: %s", entityType, entityID, collectionName),
		Data:    entity,
	})
}

// ToggleEntityStatus allows admin, or manager/sales_manager with business_management role, to set status (active/inactive) for company, wholesaler, service provider, or user
func (ac *AdminController) ToggleEntityStatus(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	if claims.UserType == "admin" {
		// allow
	} else if claims.UserType == "manager" {
		var manager models.Manager
		err := ac.DB.Collection("managers").FindOne(context.Background(), bson.M{"_id": claims.UserID}).Decode(&manager)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to fetch manager",
			})
		}
		if !hasRole(manager.RolesAccess, "business_management") {
			return c.JSON(http.StatusForbidden, models.Response{
				Status:  http.StatusForbidden,
				Message: "Manager does not have business_management role",
			})
		}
	} else if claims.UserType == "sales_manager" {
		var salesManager models.SalesManager
		err := ac.DB.Collection("sales_managers").FindOne(context.Background(), bson.M{"_id": claims.UserID}).Decode(&salesManager)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to fetch sales manager",
			})
		}
		if !hasRole(salesManager.RolesAccess, "business_management") {
			return c.JSON(http.StatusForbidden, models.Response{
				Status:  http.StatusForbidden,
				Message: "Sales manager does not have business_management role",
			})
		}
	} else {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins, managers, or sales managers can update entity status",
		})
	}

	entityType := c.Param("entityType") // "company", "wholesaler", "serviceProvider"
	entityID := c.Param("id")
	if entityType == "" || entityID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Entity type and ID are required",
		})
	}

	var req struct {
		Status string `json:"status"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}
	if req.Status != "active" && req.Status != "inactive" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Status must be 'active' or 'inactive'",
		})
	}

	objID, err := primitive.ObjectIDFromHex(entityID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid entity ID format",
		})
	}

	var collectionName string
	switch entityType {
	case "company":
		collectionName = "companies"
	case "wholesaler":
		collectionName = "wholesalers"
	case "serviceProvider", "service_provider":
		collectionName = "serviceProviders"
	case "user":
		collectionName = "users"
	default:
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid entity type",
		})
	}

	collection := ac.DB.Collection(collectionName)
	update := bson.M{"$set": bson.M{"status": req.Status, "updatedAt": time.Now()}}
	result, err := collection.UpdateOne(context.Background(), bson.M{"_id": objID}, update)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update entity status",
		})
	}
	if result.MatchedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Entity not found",
		})
	}

	// Update branch statuses for companies and wholesalers
	if entityType == "company" || entityType == "wholesaler" {
		// Update embedded branches within the entity document
		branchUpdate := bson.M{"$set": bson.M{"branches.$[].status": req.Status}}
		_, err = collection.UpdateOne(context.Background(), bson.M{"_id": objID}, branchUpdate)
		if err != nil {
			log.Printf("Failed to update embedded branch statuses: %v", err)
		}

		// Also update branches in the separate branches collection
		branchesCollection := ac.DB.Collection("branches")
		branchFilter := bson.M{}
		if entityType == "company" {
			branchFilter = bson.M{"companyId": objID}
		} else if entityType == "wholesaler" {
			branchFilter = bson.M{"wholesalerId": objID}
		}

		branchStatusUpdate := bson.M{"$set": bson.M{"status": req.Status, "updatedAt": time.Now()}}
		_, err = branchesCollection.UpdateMany(context.Background(), branchFilter, branchStatusUpdate)
		if err != nil {
			log.Printf("Failed to update branch statuses in branches collection: %v", err)
		}
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Entity status and associated branch statuses updated successfully",
	})
}

// ToggleCompanyBranchStatus updates the status of a specific branch of a company
func (ac *AdminController) ToggleCompanyBranchStatus(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins can update branch status",
		})
	}

	companyID := c.Param("companyId")
	branchID := c.Param("branchId")
	if companyID == "" || branchID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Company ID and Branch ID are required",
		})
	}

	var req struct {
		Status string `json:"status"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}
	if req.Status != "active" && req.Status != "inactive" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Status must be 'active' or 'inactive'",
		})
	}

	companyObjID, err := primitive.ObjectIDFromHex(companyID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid company ID format",
		})
	}
	branchObjID, err := primitive.ObjectIDFromHex(branchID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid branch ID format",
		})
	}

	collection := ac.DB.Collection("companies")
	filter := bson.M{
		"_id":          companyObjID,
		"branches._id": branchObjID,
	}
	update := bson.M{
		"$set": bson.M{
			"branches.$.status":    req.Status,
			"branches.$.updatedAt": time.Now(),
		},
	}
	result, err := collection.UpdateOne(context.Background(), filter, update)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update branch status",
		})
	}
	if result.MatchedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Branch not found in company",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Branch status updated successfully",
	})
}

// ToggleWholesalerBranchStatus updates the status of a specific branch of a wholesaler
func (ac *AdminController) ToggleWholesalerBranchStatus(c echo.Context) error {
	// Debug log for received path parameters
	wholesalerID := c.Param("wholesalerId")
	branchID := c.Param("branchId")
	log.Printf("[DEBUG] Received wholesalerId: '%s', branchId: '%s'", wholesalerID, branchID)

	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins can update wholesaler branch status",
		})
	}

	if wholesalerID == "" || branchID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Wholesaler ID and Branch ID are required",
		})
	}

	var req struct {
		Status string `json:"status"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}
	if req.Status != "active" && req.Status != "inactive" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Status must be 'active' or 'inactive'",
		})
	}

	wholesalerObjID, err := primitive.ObjectIDFromHex(wholesalerID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid wholesaler ID format",
		})
	}
	branchObjID, err := primitive.ObjectIDFromHex(branchID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid branch ID format",
		})
	}

	collection := ac.DB.Collection("wholesalers")
	filter := bson.M{
		"_id":          wholesalerObjID,
		"branches._id": branchObjID,
	}
	update := bson.M{
		"$set": bson.M{
			"branches.$.status":    req.Status,
			"branches.$.updatedAt": time.Now(),
		},
	}
	result, err := collection.UpdateOne(context.Background(), filter, update)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update branch status",
		})
	}
	if result.MatchedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Branch not found in wholesaler",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Wholesaler branch status updated successfully",
	})
}

// DeleteCompanyBranch allows admin to delete a specific branch of a company
func (ac *AdminController) DeleteCompanyBranch(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Check if user is admin
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins can delete company branches",
		})
	}

	companyID := c.Param("companyId")
	branchID := c.Param("branchId")
	if companyID == "" || branchID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Company ID and Branch ID are required",
		})
	}

	companyObjID, err := primitive.ObjectIDFromHex(companyID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid company ID format",
		})
	}

	branchObjID, err := primitive.ObjectIDFromHex(branchID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid branch ID format",
		})
	}

	// Find company and get branch data for file deletion
	companyCollection := ac.DB.Collection("companies")
	var company models.Company
	err = companyCollection.FindOne(ctx, bson.M{"_id": companyObjID}).Decode(&company)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Company not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find company",
		})
	}

	// Find the branch to get images and videos for deletion
	var imagesToDelete []string
	var videosToDelete []string
	branchFound := false
	for _, branch := range company.Branches {
		if branch.ID == branchObjID {
			imagesToDelete = branch.Images
			videosToDelete = branch.Videos
			branchFound = true
			break
		}
	}

	if !branchFound {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Branch not found in company",
		})
	}

	// Delete the branch from the company
	update := bson.M{
		"$pull": bson.M{
			"branches": bson.M{"_id": branchObjID},
		},
	}

	result, err := companyCollection.UpdateByID(ctx, companyObjID, update)
	if err != nil {
		log.Printf("Error deleting branch: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to delete branch: " + err.Error(),
		})
	}

	if result.ModifiedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Branch not found or already deleted",
		})
	}

	// Delete image files from filesystem
	var deletionErrors []string
	for _, imagePath := range imagesToDelete {
		if err := os.Remove(imagePath); err != nil {
			log.Printf("Error deleting image file %s: %v", imagePath, err)
			deletionErrors = append(deletionErrors, imagePath)
		} else {
			log.Printf("Successfully deleted image file: %s", imagePath)
		}
	}

	// Delete video files from filesystem
	for _, videoPath := range videosToDelete {
		if err := os.Remove(videoPath); err != nil {
			log.Printf("Error deleting video file %s: %v", videoPath, err)
			deletionErrors = append(deletionErrors, videoPath)
		} else {
			log.Printf("Successfully deleted video file: %s", videoPath)
		}
	}

	// Log deletion errors but don't fail the request
	if len(deletionErrors) > 0 {
		log.Printf("Some media files could not be deleted: %v", deletionErrors)
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Branch deleted successfully",
	})
}

// GetWholesalerBranches allows admin to get all branches for a specific wholesaler
func (ac *AdminController) GetWholesalerBranches(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Check if user is admin
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins can access wholesaler branches",
		})
	}

	// Get wholesaler ID from URL parameter
	wholesalerID := c.Param("wholesalerId")
	if wholesalerID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Wholesaler ID is required",
		})
	}

	// Convert string ID to ObjectID
	wholesalerObjID, err := primitive.ObjectIDFromHex(wholesalerID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid wholesaler ID format",
		})
	}

	// Find the wholesaler
	wholesalerCollection := ac.DB.Collection("wholesalers")
	var wholesaler models.Wholesaler
	err = wholesalerCollection.FindOne(ctx, bson.M{"_id": wholesalerObjID}).Decode(&wholesaler)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Wholesaler not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find wholesaler",
		})
	}

	// Check if wholesaler has branches
	if len(wholesaler.Branches) == 0 {
		return c.JSON(http.StatusOK, models.Response{
			Status:  http.StatusOK,
			Message: "No branches found for this wholesaler",
			Data: map[string]interface{}{
				"wholesalerId": wholesaler.ID,
				"businessName": wholesaler.BusinessName,
				"branches":     []models.Branch{},
				"branchCount":  0,
			},
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Wholesaler branches retrieved successfully",
		Data: map[string]interface{}{
			"wholesalerId": wholesaler.ID,
			"businessName": wholesaler.BusinessName,
			"branches":     wholesaler.Branches,
			"branchCount":  len(wholesaler.Branches),
		},
	})
}

// GetAllWholesalerBranches allows admin to get all branches from all wholesalers
func (ac *AdminController) GetAllWholesalerBranches(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Check if user is admin
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins can access all wholesaler branches",
		})
	}

	// Find all wholesalers with branches
	wholesalerCollection := ac.DB.Collection("wholesalers")
	cursor, err := wholesalerCollection.Find(ctx, bson.M{"branches": bson.M{"$exists": true, "$ne": []interface{}{}}})
	if err != nil {
		log.Printf("Error finding wholesalers with branches: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve wholesalers with branches",
		})
	}
	defer cursor.Close(ctx)

	var wholesalers []models.Wholesaler
	if err = cursor.All(ctx, &wholesalers); err != nil {
		log.Printf("Error decoding wholesalers: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode wholesalers",
		})
	}

	// Prepare response data - flatten all branches with wholesaler reference
	var allBranches []map[string]interface{}
	totalBranches := 0

	for _, wholesaler := range wholesalers {
		for _, branch := range wholesaler.Branches {
			branchData := map[string]interface{}{
				"branchId":        branch.ID,
				"branchName":      branch.Name,
				"location":        branch.Location,
				"phone":           branch.Phone,
				"category":        branch.Category,
				"subCategory":     branch.SubCategory,
				"description":     branch.Description,
				"images":          branch.Images,
				"videos":          branch.Videos,
				"status":          branch.Status,
				"sponsorship":     branch.Sponsorship,
				"socialMedia":     branch.SocialMedia,
				"createdAt":       branch.CreatedAt,
				"updatedAt":       branch.UpdatedAt,
				"wholesalerId":    wholesaler.ID,
				"wholesalerName":  wholesaler.BusinessName,
				"wholesalerPhone": wholesaler.Phone,
			}
			allBranches = append(allBranches, branchData)
			totalBranches++
		}
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "All wholesaler branches retrieved successfully",
		Data: map[string]interface{}{
			"branches":                     allBranches,
			"totalBranches":                totalBranches,
			"totalWholesalersWithBranches": len(wholesalers),
		},
	})
}

// DeleteWholesalerBranch allows admin to delete a specific branch of a wholesaler
func (ac *AdminController) DeleteWholesalerBranch(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Check if user is admin
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins can delete wholesaler branches",
		})
	}

	wholesalerID := c.Param("wholesalerId")
	branchID := c.Param("branchId")
	if wholesalerID == "" || branchID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Wholesaler ID and Branch ID are required",
		})
	}

	wholesalerObjID, err := primitive.ObjectIDFromHex(wholesalerID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid wholesaler ID format",
		})
	}

	branchObjID, err := primitive.ObjectIDFromHex(branchID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid branch ID format",
		})
	}

	// Find wholesaler and get branch data for file deletion
	wholesalerCollection := ac.DB.Collection("wholesalers")
	var wholesaler models.Wholesaler
	err = wholesalerCollection.FindOne(ctx, bson.M{"_id": wholesalerObjID}).Decode(&wholesaler)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Wholesaler not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find wholesaler",
		})
	}

	// Find the branch to get images and videos for deletion
	var imagesToDelete []string
	var videosToDelete []string
	branchFound := false
	for _, branch := range wholesaler.Branches {
		if branch.ID == branchObjID {
			imagesToDelete = branch.Images
			videosToDelete = branch.Videos
			branchFound = true
			break
		}
	}

	if !branchFound {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Branch not found in wholesaler",
		})
	}

	// Delete the branch from the wholesaler
	update := bson.M{
		"$pull": bson.M{
			"branches": bson.M{"_id": branchObjID},
		},
	}

	result, err := wholesalerCollection.UpdateByID(ctx, wholesalerObjID, update)
	if err != nil {
		log.Printf("Error deleting branch: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to delete branch: " + err.Error(),
		})
	}

	if result.ModifiedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Branch not found or already deleted",
		})
	}

	// Delete image files from filesystem
	var deletionErrors []string
	for _, imagePath := range imagesToDelete {
		if err := os.Remove(imagePath); err != nil {
			log.Printf("Error deleting image file %s: %v", imagePath, err)
			deletionErrors = append(deletionErrors, imagePath)
		} else {
			log.Printf("Successfully deleted image file: %s", imagePath)
		}
	}

	// Delete video files from filesystem
	for _, videoPath := range videosToDelete {
		if err := os.Remove(videoPath); err != nil {
			log.Printf("Error deleting video file %s: %v", videoPath, err)
			deletionErrors = append(deletionErrors, videoPath)
		} else {
			log.Printf("Successfully deleted video file: %s", videoPath)
		}
	}

	// Log deletion errors but don't fail the request
	if len(deletionErrors) > 0 {
		log.Printf("Some media files could not be deleted: %v", deletionErrors)
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Branch deleted successfully",
	})
}

// GetAdminWallet retrieves the admin wallet information including total income from subscriptions
// and total commissions paid to salespersons and sales managers
func (ac *AdminController) GetAdminWallet(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Check if user is admin
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins can access admin wallet information",
		})
	}

	// Get total income from all active subscriptions
	totalIncome, err := ac.calculateTotalSubscriptionIncome(ctx)
	if err != nil {
		log.Printf("Error calculating total subscription income: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to calculate total subscription income",
		})
	}

	// Get total commissions paid to salespersons and sales managers
	totalCommissions, err := ac.calculateTotalCommissions(ctx)
	if err != nil {
		log.Printf("Error calculating total commissions: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to calculate total commissions",
		})
	}

	// Calculate net profit (income - commissions)
	netProfit := totalIncome - totalCommissions

	// Get detailed breakdown by subscription type
	incomeBreakdown, err := ac.getIncomeBreakdownByType(ctx)
	if err != nil {
		log.Printf("Error getting income breakdown: %v", err)
		// Continue without breakdown rather than failing completely
	}

	// Get commission breakdown
	commissionBreakdown, err := ac.getCommissionBreakdown(ctx)
	if err != nil {
		log.Printf("Error getting commission breakdown: %v", err)
		// Continue without breakdown rather than failing completely
	}

	// Get income from admin wallet collection transactions
	var adminWalletIncome float64
	var withdrawalIncome float64
	var allAdminWalletIncome float64 // Total income from all admin wallet transactions
	cursor, err := ac.DB.Collection("admin_wallet").Find(ctx, bson.M{})
	if err == nil {
		defer cursor.Close(ctx)
		for cursor.Next(ctx) {
			var transaction models.AdminWallet
			if err := cursor.Decode(&transaction); err == nil {
				// Add all income types to total
				allAdminWalletIncome += transaction.Amount

				// Categorize by type
				if transaction.Type == "subscription_income" {
					adminWalletIncome += transaction.Amount
				} else if transaction.Type == "withdrawal_income" {
					withdrawalIncome += transaction.Amount
				}
			}
		}
	}

	// Calculate total admin wallet balance including admin wallet transactions and withdrawal income
	totalAdminWallet := totalIncome + adminWalletIncome + withdrawalIncome - totalCommissions

	walletData := map[string]interface{}{
		"totalIncome":          totalIncome,
		"adminWalletIncome":    adminWalletIncome,
		"allAdminWalletIncome": allAdminWalletIncome,
		"totalCommissions":     totalCommissions,
		"withdrawalIncome":     withdrawalIncome,
		"totalAdminWallet":     totalAdminWallet,
		"netProfit":            netProfit,
		"incomeBreakdown":      incomeBreakdown,
		"commissionBreakdown":  commissionBreakdown,
		"lastUpdated":          time.Now(),
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Admin wallet information retrieved successfully",
		Data:    walletData,
	})
}

// calculateTotalSubscriptionIncome calculates the total income from all active subscriptions
func (ac *AdminController) calculateTotalSubscriptionIncome(ctx context.Context) (float64, error) {
	var totalIncome float64

	// 1. Get income from company subscriptions
	companyIncome, err := ac.getCompanySubscriptionIncome(ctx)
	if err != nil {
		log.Printf("Error getting company subscription income: %v", err)
	} else {
		totalIncome += companyIncome
	}

	// 2. Get income from wholesaler subscriptions
	wholesalerIncome, err := ac.getWholesalerSubscriptionIncome(ctx)
	if err != nil {
		log.Printf("Error getting wholesaler subscription income: %v", err)
	} else {
		totalIncome += wholesalerIncome
	}

	// 3. Get income from service provider subscriptions
	serviceProviderIncome, err := ac.getServiceProviderSubscriptionIncome(ctx)
	if err != nil {
		log.Printf("Error getting service provider subscription income: %v", err)
	} else {
		totalIncome += serviceProviderIncome
	}

	// 4. Get income from sponsorship subscriptions
	sponsorshipIncome, err := ac.getSponsorshipSubscriptionIncome(ctx)
	if err != nil {
		log.Printf("Error getting sponsorship subscription income: %v", err)
	} else {
		totalIncome += sponsorshipIncome
	}

	return totalIncome, nil
}

// getCompanySubscriptionIncome calculates income from company subscriptions
func (ac *AdminController) getCompanySubscriptionIncome(ctx context.Context) (float64, error) {
	collection := config.GetCollection(ac.DB.Client(), "company_subscriptions")

	pipeline := []bson.M{
		{"$match": bson.M{"status": "active"}},
		{"$lookup": bson.M{
			"from":         "subscription_plans",
			"localField":   "planId",
			"foreignField": "_id",
			"as":           "plan",
		}},
		{"$unwind": "$plan"},
		{"$group": bson.M{
			"_id":   nil,
			"total": bson.M{"$sum": "$plan.price"},
		}},
	}

	cursor, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, err
	}
	defer cursor.Close(ctx)

	var result struct {
		Total float64 `bson:"total"`
	}
	if cursor.Next(ctx) {
		if err := cursor.Decode(&result); err != nil {
			return 0, err
		}
	}

	return result.Total, nil
}

// getWholesalerSubscriptionIncome calculates income from wholesaler subscriptions
func (ac *AdminController) getWholesalerSubscriptionIncome(ctx context.Context) (float64, error) {
	collection := config.GetCollection(ac.DB.Client(), "wholesaler_subscriptions")

	pipeline := []bson.M{
		{"$match": bson.M{"status": "active"}},
		{"$lookup": bson.M{
			"from":         "subscription_plans",
			"localField":   "planId",
			"foreignField": "_id",
			"as":           "plan",
		}},
		{"$unwind": "$plan"},
		{"$group": bson.M{
			"_id":   nil,
			"total": bson.M{"$sum": "$plan.price"},
		}},
	}

	cursor, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, err
	}
	defer cursor.Close(ctx)

	var result struct {
		Total float64 `bson:"total"`
	}
	if cursor.Next(ctx) {
		if err := cursor.Decode(&result); err != nil {
			return 0, err
		}
	}

	return result.Total, nil
}

// getServiceProviderSubscriptionIncome calculates income from service provider subscriptions
func (ac *AdminController) getServiceProviderSubscriptionIncome(ctx context.Context) (float64, error) {
	collection := config.GetCollection(ac.DB.Client(), "service_provider_subscriptions")

	pipeline := []bson.M{
		{"$match": bson.M{"status": "active"}},
		{"$lookup": bson.M{
			"from":         "subscription_plans",
			"localField":   "planId",
			"foreignField": "_id",
			"as":           "plan",
		}},
		{"$unwind": "$plan"},
		{"$group": bson.M{
			"_id":   nil,
			"total": bson.M{"$sum": "$plan.price"},
		}},
	}

	cursor, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, err
	}
	defer cursor.Close(ctx)

	var result struct {
		Total float64 `bson:"total"`
	}
	if cursor.Next(ctx) {
		if err := cursor.Decode(&result); err != nil {
			return 0, err
		}
	}

	return result.Total, nil
}

// getSponsorshipSubscriptionIncome calculates income from sponsorship subscriptions
func (ac *AdminController) getSponsorshipSubscriptionIncome(ctx context.Context) (float64, error) {
	collection := config.GetCollection(ac.DB.Client(), "sponsorship_subscriptions")

	pipeline := []bson.M{
		{"$match": bson.M{"status": "active"}},
		{"$lookup": bson.M{
			"from":         "sponsorships",
			"localField":   "sponsorshipId",
			"foreignField": "_id",
			"as":           "sponsorship",
		}},
		{"$unwind": "$sponsorship"},
		{"$group": bson.M{
			"_id":   nil,
			"total": bson.M{"$sum": "$sponsorship.price"},
		}},
	}

	cursor, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, err
	}
	defer cursor.Close(ctx)

	var result struct {
		Total float64 `bson:"total"`
	}
	if cursor.Next(ctx) {
		if err := cursor.Decode(&result); err != nil {
			return 0, err
		}
	}

	return result.Total, nil
}

// calculateTotalCommissions calculates the total commissions paid to salespersons and sales managers
func (ac *AdminController) calculateTotalCommissions(ctx context.Context) (float64, error) {
	collection := config.GetCollection(ac.DB.Client(), "commissions")

	pipeline := []bson.M{
		{"$group": bson.M{
			"_id":                         nil,
			"totalSalespersonCommission":  bson.M{"$sum": "$salespersonCommission"},
			"totalSalesManagerCommission": bson.M{"$sum": "$salesManagerCommission"},
		}},
	}

	cursor, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, err
	}
	defer cursor.Close(ctx)

	var result struct {
		TotalSalespersonCommission  float64 `bson:"totalSalespersonCommission"`
		TotalSalesManagerCommission float64 `bson:"totalSalesManagerCommission"`
	}
	if cursor.Next(ctx) {
		if err := cursor.Decode(&result); err != nil {
			return 0, err
		}
	}

	totalCommissions := result.TotalSalespersonCommission + result.TotalSalesManagerCommission
	return totalCommissions, nil
}

// getIncomeBreakdownByType provides detailed breakdown of income by subscription type
func (ac *AdminController) getIncomeBreakdownByType(ctx context.Context) (map[string]interface{}, error) {
	breakdown := make(map[string]interface{})

	// Company subscriptions (main company subscriptions)
	companyIncome, err := ac.getCompanySubscriptionIncome(ctx)
	if err != nil {
		breakdown["company"] = map[string]interface{}{
			"income": 0,
			"error":  err.Error(),
		}
	} else {
		breakdown["company"] = map[string]interface{}{
			"income": companyIncome,
			"error":  nil,
		}
	}

	// Add company branch subscription income from admin_wallet
	companyBranchIncome, err := ac.getCompanyBranchSubscriptionIncome(ctx)
	if err != nil {
		log.Printf("Error getting company branch subscription income: %v", err)
	} else {
		// Add branch income to company income
		if currentCompany, ok := breakdown["company"].(map[string]interface{}); ok {
			if currentIncome, ok := currentCompany["income"].(float64); ok {
				currentCompany["income"] = currentIncome + companyBranchIncome
			}
		}
	}

	// Wholesaler subscriptions (main wholesaler subscriptions)
	wholesalerIncome, err := ac.getWholesalerSubscriptionIncome(ctx)
	if err != nil {
		breakdown["wholesaler"] = map[string]interface{}{
			"income": 0,
			"error":  err.Error(),
		}
	} else {
		breakdown["wholesaler"] = map[string]interface{}{
			"income": wholesalerIncome,
			"error":  nil,
		}
	}

	// Add wholesaler branch subscription income from admin_wallet
	wholesalerBranchIncome, err := ac.getWholesalerBranchSubscriptionIncome(ctx)
	if err != nil {
		log.Printf("Error getting wholesaler branch subscription income: %v", err)
	} else {
		// Add branch income to wholesaler income
		if currentWholesaler, ok := breakdown["wholesaler"].(map[string]interface{}); ok {
			if currentIncome, ok := currentWholesaler["income"].(float64); ok {
				currentWholesaler["income"] = currentIncome + wholesalerBranchIncome
			}
		}
	}

	// Service provider subscriptions (main service provider subscriptions)
	serviceProviderIncome, err := ac.getServiceProviderSubscriptionIncome(ctx)
	if err != nil {
		breakdown["serviceProvider"] = map[string]interface{}{
			"income": 0,
			"error":  err.Error(),
		}
	} else {
		breakdown["serviceProvider"] = map[string]interface{}{
			"income": serviceProviderIncome,
			"error":  nil,
		}
	}

	// Add service provider branch subscription income from admin_wallet
	serviceProviderBranchIncome, err := ac.getServiceProviderBranchSubscriptionIncome(ctx)
	if err != nil {
		log.Printf("Error getting service provider branch subscription income: %v", err)
	} else {
		// Add branch income to service provider income
		if currentServiceProvider, ok := breakdown["serviceProvider"].(map[string]interface{}); ok {
			if currentIncome, ok := currentServiceProvider["income"].(float64); ok {
				currentServiceProvider["income"] = currentIncome + serviceProviderBranchIncome
			}
		}
	}

	// Sponsorship subscriptions
	sponsorshipIncome, err := ac.getSponsorshipSubscriptionIncome(ctx)
	if err != nil {
		breakdown["sponsorship"] = map[string]interface{}{
			"income": 0,
			"error":  err.Error(),
		}
	} else {
		breakdown["sponsorship"] = map[string]interface{}{
			"income": sponsorshipIncome,
			"error":  nil,
		}
	}

	return breakdown, nil
}

// getCommissionBreakdown provides detailed breakdown of commissions
func (ac *AdminController) getCommissionBreakdown(ctx context.Context) (map[string]interface{}, error) {
	collection := config.GetCollection(ac.DB.Client(), "commissions")

	pipeline := []bson.M{
		{"$group": bson.M{
			"_id":                         nil,
			"totalSalespersonCommission":  bson.M{"$sum": "$salespersonCommission"},
			"totalSalesManagerCommission": bson.M{"$sum": "$salesManagerCommission"},
			"totalCommissions":            bson.M{"$sum": bson.M{"$add": []string{"$salespersonCommission", "$salesManagerCommission"}}},
		}},
	}

	cursor, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var result struct {
		TotalSalespersonCommission  float64 `bson:"totalSalespersonCommission"`
		TotalSalesManagerCommission float64 `bson:"totalSalesManagerCommission"`
		TotalCommissions            float64 `bson:"totalCommissions"`
	}
	if cursor.Next(ctx) {
		if err := cursor.Decode(&result); err != nil {
			return nil, err
		}
	}

	breakdown := map[string]interface{}{
		"salesperson": map[string]interface{}{
			"commission": result.TotalSalespersonCommission,
			"percentage": 0, // Will calculate if total > 0
		},
		"salesManager": map[string]interface{}{
			"commission": result.TotalSalesManagerCommission,
			"percentage": 0, // Will calculate if total > 0
		},
		"total": result.TotalCommissions,
	}

	// Calculate percentages if there are commissions
	if result.TotalCommissions > 0 {
		if salespersonBreakdown, ok := breakdown["salesperson"].(map[string]interface{}); ok {
			salespersonBreakdown["percentage"] = (result.TotalSalespersonCommission / result.TotalCommissions) * 100
		}
		if salesManagerBreakdown, ok := breakdown["salesManager"].(map[string]interface{}); ok {
			salesManagerBreakdown["percentage"] = (result.TotalSalesManagerCommission / result.TotalCommissions) * 100
		}
	}

	return breakdown, nil
}

// CreateSalesperson allows admin to create a new salesperson
func (ac *AdminController) CreateSalesperson(c echo.Context) error {
	// Check if user is admin
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins can create salespersons",
		})
	}

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

	// Validate required fields
	if req.FullName == "" || req.Email == "" || req.Password == "" || req.PhoneNumber == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Full name, email, password, and phone number are required",
		})
	}

	// Validate commission percent
	if req.CommissionPercent < 0 || req.CommissionPercent > 100 {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Commission percent must be between 0 and 100",
		})
	}

	// Check if email already exists
	var existingSalesperson models.Salesperson
	err := ac.DB.Collection("salespersons").FindOne(
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

	// Hash the password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to hash password",
		})
	}

	// Get admin ID from token for CreatedBy field
	adminID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Invalid admin ID in token",
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
		CreatedBy:         adminID,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	result, err := ac.DB.Collection("salespersons").InsertOne(context.Background(), salesperson)
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

	_, err = ac.DB.Collection("users").InsertOne(context.Background(), user)
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

// GetAllSalespersons allows admin to retrieve all salespersons
func (ac *AdminController) GetAdminSalespersons(c echo.Context) error {
	// Check if user is admin
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins can view all salespersons",
		})
	}

	// Get admin ID from token
	adminID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid admin ID",
		})
	}

	var salespersons []models.Salesperson
	cursor, err := ac.DB.Collection("salespersons").Find(
		context.Background(),
		bson.M{"createdBy": adminID}, // Only get salespersons created by this admin
		&options.FindOptions{Sort: bson.M{"createdAt": -1}},
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
		Message: "Salespersons created by admin retrieved successfully",
		Data:    salespersons,
	})
}

// GetSalesperson allows admin to retrieve a specific salesperson by ID
func (ac *AdminController) GetSalesperson(c echo.Context) error {
	// Check if user is admin
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins can view salesperson details",
		})
	}

	// Get admin ID from token
	adminID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid admin ID",
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
	err = ac.DB.Collection("salespersons").FindOne(
		context.Background(),
		bson.M{
			"_id":       salespersonID,
			"createdBy": adminID, // Only allow admin to view salespersons they created
		},
	).Decode(&salesperson)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Salesperson not found or you don't have permission to view it",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch salesperson",
		})
	}

	// Remove password from response
	salesperson.Password = ""

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Salesperson retrieved successfully",
		Data:    salesperson,
	})
}

// UpdateSalesperson allows admin to update a specific salesperson
func (ac *AdminController) UpdateSalesperson(c echo.Context) error {
	// Check if user is admin
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins can update salesperson information",
		})
	}

	salespersonID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid salesperson ID",
		})
	}

	// First verify that the salesperson exists
	var existingSalesperson models.Salesperson
	err = ac.DB.Collection("salespersons").FindOne(
		context.Background(),
		bson.M{"_id": salespersonID},
	).Decode(&existingSalesperson)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Salesperson not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to verify salesperson",
		})
	}

	// Parse request body
	updateRequest := make(map[string]interface{})
	if err := c.Bind(&updateRequest); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}

	// If email is being updated, check for uniqueness
	if email, exists := updateRequest["email"].(string); exists && email != "" && email != existingSalesperson.Email {
		var emailCheck models.Salesperson
		err := ac.DB.Collection("salespersons").FindOne(
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
		"Image":             "Image",
		"image":             "Image",
		"commissionPercent": "commissionPercent",
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
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to hash password",
			})
		}
		updateData["password"] = string(hashedPassword)
	}

	// Update the salesperson
	result, err := ac.DB.Collection("salespersons").UpdateOne(
		context.Background(),
		bson.M{"_id": salespersonID},
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
	err = ac.DB.Collection("salespersons").FindOne(
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

// DeleteSalesperson allows admin to delete a specific salesperson
func (ac *AdminController) DeleteSalesperson(c echo.Context) error {
	// Check if user is admin
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins can delete salespersons",
		})
	}

	salespersonID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid salesperson ID",
		})
	}

	// First verify that the salesperson exists and get their details
	var salesperson models.Salesperson
	err = ac.DB.Collection("salespersons").FindOne(
		context.Background(),
		bson.M{"_id": salespersonID},
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
			Message: "Failed to verify salesperson",
		})
	}

	// Delete the salesperson
	result, err := ac.DB.Collection("salespersons").DeleteOne(
		context.Background(),
		bson.M{"_id": salespersonID},
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
	_, err = ac.DB.Collection("users").DeleteOne(context.Background(), bson.M{
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

// GetPendingRequestsFromAdminSalespersons retrieves all pending requests from salespersons created by the admin
func (ac *AdminController) GetPendingRequestsFromAdminSalespersons(c echo.Context) error {
	// Get admin ID from token
	claims := middleware.GetUserFromToken(c)
	adminID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Invalid admin ID",
		})
	}

	ctx := context.Background()

	// First, get all salespersons created by this admin
	var salespersonIDs []primitive.ObjectID
	salespersonCursor, err := ac.DB.Collection("salespersons").Find(ctx, bson.M{"createdBy": adminID})
	if err != nil {
		log.Printf("Error finding salespersons: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch salespersons",
		})
	}
	defer salespersonCursor.Close(ctx)

	for salespersonCursor.Next(ctx) {
		var salesperson models.Salesperson
		if err := salespersonCursor.Decode(&salesperson); err != nil {
			log.Printf("Error decoding salesperson: %v", err)
			continue
		}
		salespersonIDs = append(salespersonIDs, salesperson.ID)
	}

	if len(salespersonIDs) == 0 {
		return c.JSON(http.StatusOK, models.Response{
			Status:  http.StatusOK,
			Message: "No salespersons found for this admin",
			Data:    map[string]interface{}{"pendingRequests": []interface{}{}},
		})
	}

	// Get pending company requests
	var pendingCompanyRequests []models.PendingCompanyRequest
	companyCursor, err := ac.DB.Collection("pending_company_requests").Find(ctx, bson.M{"salesPersonId": bson.M{"$in": salespersonIDs}})
	if err != nil {
		log.Printf("Error finding pending company requests: %v", err)
	} else {
		defer companyCursor.Close(ctx)
		for companyCursor.Next(ctx) {
			var request models.PendingCompanyRequest
			if err := companyCursor.Decode(&request); err != nil {
				log.Printf("Error decoding company request: %v", err)
				continue
			}
			pendingCompanyRequests = append(pendingCompanyRequests, request)
		}
	}

	// Get pending wholesaler requests
	var pendingWholesalerRequests []models.PendingWholesalerRequest
	wholesalerCursor, err := ac.DB.Collection("pending_wholesaler_requests").Find(ctx, bson.M{"salesPersonId": bson.M{"$in": salespersonIDs}})
	if err != nil {
		log.Printf("Error finding pending wholesaler requests: %v", err)
	} else {
		defer wholesalerCursor.Close(ctx)
		for wholesalerCursor.Next(ctx) {
			var request models.PendingWholesalerRequest
			if err := wholesalerCursor.Decode(&request); err != nil {
				log.Printf("Error decoding wholesaler request: %v", err)
				continue
			}
			pendingWholesalerRequests = append(pendingWholesalerRequests, request)
		}
	}

	// Get pending service provider requests
	var pendingServiceProviderRequests []models.PendingServiceProviderRequest
	serviceProviderCursor, err := ac.DB.Collection("pending_serviceProviders_requests").Find(ctx, bson.M{"salesPersonId": bson.M{"$in": salespersonIDs}})
	if err != nil {
		log.Printf("Error finding pending service provider requests: %v", err)
	} else {
		defer serviceProviderCursor.Close(ctx)
		for serviceProviderCursor.Next(ctx) {
			var request models.PendingServiceProviderRequest
			if err := serviceProviderCursor.Decode(&request); err != nil {
				log.Printf("Error decoding service provider request: %v", err)
				continue
			}
			pendingServiceProviderRequests = append(pendingServiceProviderRequests, request)
		}
	}

	// Prepare response data
	responseData := map[string]interface{}{
		"pendingCompanyRequests":         pendingCompanyRequests,
		"pendingWholesalerRequests":      pendingWholesalerRequests,
		"pendingServiceProviderRequests": pendingServiceProviderRequests,
		"totalPendingRequests":           len(pendingCompanyRequests) + len(pendingWholesalerRequests) + len(pendingServiceProviderRequests),
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Pending requests retrieved successfully",
		Data:    responseData,
	})
}

// ProcessPendingRequest approves or rejects a pending request from a salesperson created by the admin
func (ac *AdminController) ProcessPendingRequest(c echo.Context) error {
	// Get admin ID from token
	claims := middleware.GetUserFromToken(c)
	adminID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Invalid admin ID",
		})
	}

	// Parse request body
	var requestBody struct {
		RequestType string `json:"requestType"` // "company", "wholesaler", "serviceprovider"
		RequestID   string `json:"requestId"`
		Action      string `json:"action"` // "approve" or "reject"
	}

	if err := c.Bind(&requestBody); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}

	if requestBody.RequestType == "" || requestBody.RequestID == "" || requestBody.Action == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Missing required fields: requestType, requestId, action",
		})
	}

	if requestBody.Action != "approve" && requestBody.Action != "reject" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Action must be either 'approve' or 'reject'",
		})
	}

	requestObjID, err := primitive.ObjectIDFromHex(requestBody.RequestID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request ID format",
		})
	}

	ctx := context.Background()

	// Verify that the request is from a salesperson created by this admin
	var salespersonID primitive.ObjectID
	var collectionName string
	var entityCollection string

	switch requestBody.RequestType {
	case "company":
		collectionName = "pending_company_requests"
		entityCollection = "companies"
	case "wholesaler":
		collectionName = "pending_wholesaler_requests"
		entityCollection = "wholesalers"
	case "serviceprovider":
		collectionName = "pending_serviceProviders_requests"
		entityCollection = "serviceProviders"
	default:
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request type. Must be 'company', 'wholesaler', or 'serviceprovider'",
		})
	}

	// Get the pending request to verify salesperson
	var pendingRequest bson.M
	err = ac.DB.Collection(collectionName).FindOne(ctx, bson.M{"_id": requestObjID}).Decode(&pendingRequest)
	if err != nil {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Pending request not found",
		})
	}

	// Extract salesperson ID from the pending request
	if salesPersonIDInterface, exists := pendingRequest["salesPersonId"]; exists {
		if salesPersonIDStr, ok := salesPersonIDInterface.(primitive.ObjectID); ok {
			salespersonID = salesPersonIDStr
		} else {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Invalid salesperson ID in pending request",
			})
		}
	} else {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Salesperson ID not found in pending request",
		})
	}

	// Verify that this salesperson was created by the current admin
	var salesperson models.Salesperson
	err = ac.DB.Collection("salespersons").FindOne(ctx, bson.M{"_id": salespersonID, "createdBy": adminID}).Decode(&salesperson)
	if err != nil {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "You can only process requests from salespersons you created",
		})
	}

	if requestBody.Action == "approve" {
		// Handle approval
		return ac.approvePendingRequest(c, requestBody.RequestType, requestObjID, collectionName, entityCollection)
	} else {
		// Handle rejection
		return ac.rejectPendingRequest(c, requestBody.RequestType, requestObjID, collectionName)
	}
}

// ApprovePendingRequest approves a pending request (public wrapper)
func (ac *AdminController) ApprovePendingRequest(c echo.Context) error {
	// Get admin ID from token
	claims := middleware.GetUserFromToken(c)
	adminID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Invalid admin ID",
		})
	}

	// Parse request body
	var requestBody struct {
		RequestType string `json:"requestType"` // "company", "wholesaler", "serviceprovider"
		RequestID   string `json:"requestId"`
	}

	if err := c.Bind(&requestBody); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}

	if requestBody.RequestType == "" || requestBody.RequestID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Missing required fields: requestType, requestId",
		})
	}

	requestObjID, err := primitive.ObjectIDFromHex(requestBody.RequestID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request ID format",
		})
	}

	ctx := context.Background()

	// Verify that the request is from a salesperson created by this admin
	var salespersonID primitive.ObjectID
	var collectionName string
	var entityCollection string

	switch requestBody.RequestType {
	case "company":
		collectionName = "pending_company_requests"
		entityCollection = "companies"
	case "wholesaler":
		collectionName = "pending_wholesaler_requests"
		entityCollection = "wholesalers"
	case "serviceprovider":
		collectionName = "pending_serviceProviders_requests"
		entityCollection = "serviceProviders"
	default:
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request type. Must be 'company', 'wholesaler', or 'serviceprovider'",
		})
	}

	// Get the pending request to verify salesperson
	var pendingRequest bson.M
	err = ac.DB.Collection(collectionName).FindOne(ctx, bson.M{"_id": requestObjID}).Decode(&pendingRequest)
	if err != nil {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Pending request not found",
		})
	}

	// Extract salesperson ID from the pending request
	if salesPersonIDInterface, exists := pendingRequest["salesPersonId"]; exists {
		if salesPersonIDStr, ok := salesPersonIDInterface.(primitive.ObjectID); ok {
			salespersonID = salesPersonIDStr
		} else {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Invalid salesperson ID in pending request",
			})
		}
	} else {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Salesperson ID not found in pending request",
		})
	}

	// Verify that this salesperson was created by the current admin
	var salesperson models.Salesperson
	err = ac.DB.Collection("salespersons").FindOne(ctx, bson.M{"_id": salespersonID, "createdBy": adminID}).Decode(&salesperson)
	if err != nil {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "You can only process requests from salespersons you created",
		})
	}

	// Handle approval
	return ac.approvePendingRequest(c, requestBody.RequestType, requestObjID, collectionName, entityCollection)
}

// RejectPendingRequest rejects a pending request (public wrapper)
func (ac *AdminController) RejectPendingRequest(c echo.Context) error {
	// Get admin ID from token
	claims := middleware.GetUserFromToken(c)
	adminID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Invalid admin ID",
		})
	}

	// Parse request body
	var requestBody struct {
		RequestType string `json:"requestType"` // "company", "wholesaler", "serviceprovider"
		RequestID   string `json:"requestId"`
	}

	if err := c.Bind(&requestBody); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}

	if requestBody.RequestType == "" || requestBody.RequestID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Missing required fields: requestType, requestId",
		})
	}

	requestObjID, err := primitive.ObjectIDFromHex(requestBody.RequestID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request ID format",
		})
	}

	ctx := context.Background()

	// Verify that the request is from a salesperson created by this admin
	var salespersonID primitive.ObjectID
	var collectionName string

	switch requestBody.RequestType {
	case "company":
		collectionName = "pending_company_requests"
	case "wholesaler":
		collectionName = "pending_wholesaler_requests"
	case "serviceprovider":
		collectionName = "pending_serviceProviders_requests"
	default:
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request type. Must be 'company', 'wholesaler', or 'serviceprovider'",
		})
	}

	// Get the pending request to verify salesperson
	var pendingRequest bson.M
	err = ac.DB.Collection(collectionName).FindOne(ctx, bson.M{"_id": requestObjID}).Decode(&pendingRequest)
	if err != nil {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Pending request not found",
		})
	}

	// Extract salesperson ID from the pending request
	if salesPersonIDInterface, exists := pendingRequest["salesPersonId"]; exists {
		if salesPersonIDStr, ok := salesPersonIDInterface.(primitive.ObjectID); ok {
			salespersonID = salesPersonIDStr
		} else {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Invalid salesperson ID in pending request",
			})
		}
	} else {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Salesperson ID not found in pending request",
		})
	}

	// Verify that this salesperson was created by the current admin
	var salesperson models.Salesperson
	err = ac.DB.Collection("salespersons").FindOne(ctx, bson.M{"_id": salespersonID, "createdBy": adminID}).Decode(&salesperson)
	if err != nil {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "You can only process requests from salespersons you created",
		})
	}

	// Handle rejection
	return ac.rejectPendingRequest(c, requestBody.RequestType, requestObjID, collectionName)
}

// approvePendingRequest handles the approval of a pending request
func (ac *AdminController) approvePendingRequest(c echo.Context, requestType string, requestID primitive.ObjectID, collectionName, entityCollection string) error {
	ctx := context.Background()

	// Get the pending request
	var pendingRequest bson.M
	err := ac.DB.Collection(collectionName).FindOne(ctx, bson.M{"_id": requestID}).Decode(&pendingRequest)
	if err != nil {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Pending request not found",
		})
	}

	// Extract entity data and create the actual entity
	var entityData bson.M
	switch requestType {
	case "company":
		if companyData, exists := pendingRequest["company"]; exists {
			entityData = companyData.(bson.M)
		}
	case "wholesaler":
		if wholesalerData, exists := pendingRequest["wholesaler"]; exists {
			entityData = wholesalerData.(bson.M)
		}
	case "serviceprovider":
		if serviceProviderData, exists := pendingRequest["serviceProvider"]; exists {
			entityData = serviceProviderData.(bson.M)
		}
	}

	if entityData == nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Entity data not found in pending request",
		})
	}

	// Update entity status to approved
	entityData["status"] = "approved"
	entityData["creationRequest"] = "approved"
	entityData["updatedAt"] = time.Now()

	// Get the entity ID
	entityID := entityData["_id"].(primitive.ObjectID)

	// For service providers, update existing entity instead of inserting new one
	if requestType == "serviceprovider" {
		// Update the existing service provider
		_, err = ac.DB.Collection(entityCollection).UpdateOne(ctx,
			bson.M{"_id": entityID},
			bson.M{"$set": entityData})
		if err != nil {
			log.Printf("Error updating approved service provider: %v", err)
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to approve service provider request",
			})
		}
	} else {
		// For companies and wholesalers, insert new entity (original behavior)
		_, err = ac.DB.Collection(entityCollection).InsertOne(ctx, entityData)
		if err != nil {
			log.Printf("Error inserting approved entity: %v", err)
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to approve request",
			})
		}
	}

	// Handle user account creation/update based on entity type
	if email, exists := pendingRequest["email"]; exists && email != "" {
		if password, exists := pendingRequest["password"]; exists && password != "" {
			// Extract additional fields from entity data
			fullName := ""
			phone := ""
			contactPerson := ""
			contactPhone := ""

			// Get business name as full name
			if v, ok := entityData["businessName"].(string); ok {
				fullName = v
			}

			// Get phone and contact info based on entity type
			switch requestType {
			case "company", "wholesaler":
				if contactInfo, ok := entityData["contactInfo"].(bson.M); ok {
					if v, ok := contactInfo["phone"].(string); ok {
						phone = v
					}
					if v, ok := contactInfo["whatsapp"].(string); ok {
						contactPhone = v
					}
				}
				// For wholesalers, also check direct phone field
				if phone == "" && requestType == "wholesaler" {
					if v, ok := entityData["phone"].(string); ok {
						phone = v
					}
				}
				if v, ok := entityData["contactPerson"].(string); ok {
					contactPerson = v
				}
			case "serviceprovider":
				if v, ok := entityData["phone"].(string); ok {
					phone = v
				}
				if v, ok := entityData["contactPerson"].(string); ok {
					contactPerson = v
				}
				if v, ok := entityData["contactPhone"].(string); ok {
					contactPhone = v
				}
			}

			entityID := entityData["_id"].(primitive.ObjectID)

			if requestType == "serviceprovider" {
				// For service providers, check if user exists and update/create accordingly
				// Map requestType to correct userType
				var userType string
				switch requestType {
				case "serviceprovider":
					userType = "serviceProvider"
				case "company":
					userType = "company"
				case "wholesaler":
					userType = "wholesaler"
				default:
					userType = requestType
				}

				userUpdateDoc := bson.M{
					"email":         email.(string),
					"password":      password.(string),
					"fullName":      fullName,
					"userType":      userType,
					"phone":         phone,
					"contactPerson": contactPerson,
					"contactPhone":  contactPhone,
					"isActive":      true,
					"updatedAt":     time.Now(),
				}

				// Try to update existing user first
				updateResult, err := ac.DB.Collection("users").UpdateOne(ctx,
					bson.M{"serviceProviderId": entityID},
					bson.M{"$set": userUpdateDoc})

				if err != nil {
					log.Printf("Error updating user account: %v", err)
				} else if updateResult.MatchedCount == 0 {
					// No existing user found, create a new one
					userDoc := bson.M{
						"email":             email.(string),
						"password":          password.(string),
						"fullName":          fullName,
						"userType":          userType,
						"phone":             phone,
						"contactPerson":     contactPerson,
						"contactPhone":      contactPhone,
						"serviceProviderId": entityID,
						"isActive":          true,
						"createdAt":         time.Now(),
						"updatedAt":         time.Now(),
					}

					_, err := ac.DB.Collection("users").InsertOne(ctx, userDoc)
					if err != nil {
						log.Printf("Error creating user account during approval: %v", err)
					} else {
						log.Printf("Created user account for service provider during approval")
					}
				} else {
					log.Printf("Updated existing user account for service provider during approval")
				}
			} else {
				// For companies and wholesalers, create new user (original behavior)
				// Map requestType to correct userType
				var userType string
				switch requestType {
				case "serviceprovider":
					userType = "serviceProvider"
				case "company":
					userType = "company"
				case "wholesaler":
					userType = "wholesaler"
				default:
					userType = requestType
				}

				userDoc := bson.M{
					"email":         email.(string),
					"password":      password.(string),
					"fullName":      fullName,
					"userType":      userType,
					"phone":         phone,
					"contactPerson": contactPerson,
					"contactPhone":  contactPhone,
					"isActive":      true,
					"createdAt":     time.Now(),
					"updatedAt":     time.Now(),
				}

				// Set the appropriate ID field based on entity type
				switch requestType {
				case "company":
					userDoc["companyId"] = entityID
				case "wholesaler":
					userDoc["wholesalerId"] = entityID
				}

				insertRes, err := ac.DB.Collection("users").InsertOne(ctx, userDoc)
				if err != nil {
					log.Printf("Error creating user account: %v", err)
					// Don't fail the approval if user creation fails
				} else {
					// Update the entity with the user ID for easy lookup
					if userOID, ok := insertRes.InsertedID.(primitive.ObjectID); ok {
						_, updErr := ac.DB.Collection(entityCollection).UpdateOne(ctx, bson.M{"_id": entityID}, bson.M{"$set": bson.M{"userId": userOID}})
						if updErr != nil {
							log.Printf("Warning: failed to set userId on %s document: %v", requestType, updErr)
						}
					}
				}
			}
		}
	}

	// Update branch status if entity has branches
	if branches, exists := entityData["branches"]; exists {
		if branchArray, ok := branches.([]interface{}); ok {
			for _, branch := range branchArray {
				if branchMap, ok := branch.(bson.M); ok {
					branchMap["status"] = "active"
					branchMap["updatedAt"] = time.Now()
				}
			}
		}
	}

	// Remove the pending request
	_, err = ac.DB.Collection(collectionName).DeleteOne(ctx, bson.M{"_id": requestID})
	if err != nil {
		log.Printf("Error removing pending request: %v", err)
		// Don't fail the approval if cleanup fails
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: fmt.Sprintf("%s request approved successfully", requestType),
		Data:    map[string]interface{}{"entityId": entityData["_id"]},
	})
}

// rejectPendingRequest handles the rejection of a pending request
func (ac *AdminController) rejectPendingRequest(c echo.Context, requestType string, requestID primitive.ObjectID, collectionName string) error {
	ctx := context.Background()

	// For service providers, we need to get the entity ID before deleting the pending request
	// so we can clean up the created service provider and user
	var entityID primitive.ObjectID
	if requestType == "serviceprovider" {
		var pendingRequest bson.M
		err := ac.DB.Collection(collectionName).FindOne(ctx, bson.M{"_id": requestID}).Decode(&pendingRequest)
		if err == nil {
			if serviceProviderData, exists := pendingRequest["serviceProvider"]; exists {
				if serviceProviderMap, ok := serviceProviderData.(bson.M); ok {
					if id, ok := serviceProviderMap["_id"].(primitive.ObjectID); ok {
						entityID = id
					}
				}
			}
		}
	}

	// Delete the pending request from the database
	result, err := ac.DB.Collection(collectionName).DeleteOne(ctx, bson.M{"_id": requestID})
	if err != nil {
		log.Printf("Error deleting rejected request: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to reject request",
		})
	}

	if result.DeletedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Pending request not found",
		})
	}

	// For service providers, clean up the created service provider and user
	if requestType == "serviceprovider" && !entityID.IsZero() {
		// Delete the service provider
		_, err = ac.DB.Collection("serviceProviders").DeleteOne(ctx, bson.M{"_id": entityID})
		if err != nil {
			log.Printf("Error deleting service provider during rejection: %v", err)
		}

		// Delete the associated user
		_, err = ac.DB.Collection("users").DeleteOne(ctx, bson.M{"serviceProviderId": entityID})
		if err != nil {
			log.Printf("Error deleting user during service provider rejection: %v", err)
		}
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: fmt.Sprintf("%s request rejected successfully", requestType),
	})
}

// GetAllSalespersonsInDatabase retrieves all salespersons in the database regardless of who created them
func (ac *AdminController) GetAllSalespersons(c echo.Context) error {
	// Check if user is admin
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins can view all salespersons in the database",
		})
	}

	var salespersons []models.Salesperson
	cursor, err := ac.DB.Collection("salespersons").Find(
		context.Background(),
		bson.M{}, // No filter - get all salespersons
		&options.FindOptions{Sort: bson.M{"createdAt": -1}},
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

	// Get additional information about who created each salesperson
	var enrichedSalespersons []map[string]interface{}
	for _, salesperson := range salespersons {
		salespersonData := map[string]interface{}{
			"id":                salesperson.ID,
			"fullName":          salesperson.FullName,
			"email":             salesperson.Email,
			"phoneNumber":       salesperson.PhoneNumber,
			"region":            salesperson.Region,
			"commissionPercent": salesperson.CommissionPercent,
			"image":             salesperson.Image,
			"createdAt":         salesperson.CreatedAt,
			"updatedAt":         salesperson.UpdatedAt,
			"createdBy":         salesperson.CreatedBy,
		}

		// Get information about who created this salesperson
		if !salesperson.CreatedBy.IsZero() {
			// Check if created by admin
			var admin models.Admin
			err := ac.DB.Collection("admins").FindOne(context.Background(), bson.M{"_id": salesperson.CreatedBy}).Decode(&admin)
			if err == nil {
				salespersonData["createdByType"] = "admin"
				salespersonData["createdByName"] = admin.Email
			} else {
				// Check if created by sales manager
				var salesManager models.SalesManager
				err = ac.DB.Collection("salesManagers").FindOne(context.Background(), bson.M{"_id": salesperson.CreatedBy}).Decode(&salesManager)
				if err == nil {
					salespersonData["createdByType"] = "sales_manager"
					salespersonData["createdByName"] = salesManager.FullName
				} else {
					salespersonData["createdByType"] = "unknown"
					salespersonData["createdByName"] = "Unknown"
				}
			}
		} else {
			salespersonData["createdByType"] = "none"
			salespersonData["createdByName"] = "None"
		}

		// Get sales manager information if assigned
		if !salesperson.SalesManagerID.IsZero() {
			var salesManager models.SalesManager
			err := ac.DB.Collection("salesManagers").FindOne(context.Background(), bson.M{"_id": salesperson.SalesManagerID}).Decode(&salesManager)
			if err == nil {
				salespersonData["assignedSalesManager"] = map[string]interface{}{
					"id":       salesManager.ID,
					"fullName": salesManager.FullName,
					"email":    salesManager.Email,
				}
			}
		}

		enrichedSalespersons = append(enrichedSalespersons, salespersonData)
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "All salespersons in database retrieved successfully",
		Data: map[string]interface{}{
			"count":        len(enrichedSalespersons),
			"salespersons": enrichedSalespersons,
		},
	})
}

// GetAdminWalletTransactions retrieves detailed transactions from the admin wallet
func (ac *AdminController) GetAdminWalletTransactions(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Check if user is admin
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins can access admin wallet transactions",
		})
	}

	// Get pagination parameters
	page, _ := strconv.Atoi(c.QueryParam("page"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if page <= 0 {
		page = 1
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	skip := (page - 1) * limit

	// Get transactions from admin_wallet collection
	cursor, err := ac.DB.Collection("admin_wallet").Find(
		ctx,
		bson.M{},
		options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetSkip(int64(skip)).SetLimit(int64(limit)),
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve admin wallet transactions",
		})
	}
	defer cursor.Close(ctx)

	var transactions []models.AdminWallet
	if err = cursor.All(ctx, &transactions); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode admin wallet transactions",
		})
	}

	// Get total count for pagination
	totalCount, err := ac.DB.Collection("admin_wallet").CountDocuments(ctx, bson.M{})
	if err != nil {
		log.Printf("Error counting admin wallet transactions: %v", err)
		totalCount = 0
	}

	// Calculate summary statistics
	var totalIncome, totalWithdrawalIncome, totalCommissionsPaid float64
	for _, transaction := range transactions {
		switch transaction.Type {
		case "subscription_income":
			totalIncome += transaction.Amount
		case "withdrawal_income":
			totalWithdrawalIncome += transaction.Amount
		case "commission_paid":
			totalCommissionsPaid += transaction.Amount
		}
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Admin wallet transactions retrieved successfully",
		Data: map[string]interface{}{
			"transactions": transactions,
			"pagination": map[string]interface{}{
				"page":       page,
				"limit":      limit,
				"totalCount": totalCount,
				"totalPages": int(math.Ceil(float64(totalCount) / float64(limit))),
			},
			"summary": map[string]interface{}{
				"totalIncome":           totalIncome,
				"totalWithdrawalIncome": totalWithdrawalIncome,
				"totalCommissionsPaid":  totalCommissionsPaid,
			},
		},
	})
}

// Post https://barrim.online/api/wholesaler/branches
// output:
// {
//     "status": 200,
//     "message": "Branch created successfully",
//     "data": {
//         "_id": "68c9928669d5f086c3a49df7",
//         "category": "Wholesale",
//          "description": "Updated branch description",
//         "images": null,
//         "lat": 30.0444,
//         "lng": 31.2357,
//         "location": {
//             "country": "Egypt",
//             "governorate": "Cairo",
//             "district": "Nasr City",
//             "city": "Cairo",
//             "lat": 30.0444,
//             "lng": 31.2357
//              },
//         "name": "Updated Branch Name",
//         "phone": "1234567890",
//         "socialMedia": {
//             "facebook": "https://facebook.com/updated-branch",
//             "instagram": "https://instagram.com/updated-branch"
//         },
//         "subCategory": "Electronics",
//         "videos": null
//     }
// }
// Put https://barrim.online/api/wholesaler/branches/68c9928669d5f086c3a49df7
// body: form-data: key: data, value: {
//   "name": "Updated Branch Name",
//   "phone": "1234567890",
//   "category": "Wholesale",
//   "subCategory": "Electronics",
//   "description": "Updated branch description",
//   "country": "Egypt",
//   "governorate": "Cairo",
//   "district": "Nasr City",
//   "city": "Cairo",
//   "lat": 30.0444,
//   "lng": 31.2357,
//   "facebook": "https://facebook.com/updated-branch",
//   "instagram": "https://instagram.com/updated-branch"
// }
// output:
// {
//     "status": 200,
//     "message": "Branch updated successfully",
//     "data": {
//         "_id": "68c9928669d5f086c3a49df7",
//         "category": "Wholesale",
//         "description": "Updated branch description",
//         "images": null,
//         "lat": 30.0444,
//         "lng": 31.2357,
//          "location": {
//             "country": "Egypt",
//             "governorate": "Cairo",
//             "district": "Nasr City",
//             "city": "Cairo",
//             "lat": 30.0444,
//             "lng": 31.2357
//         },
//         "name": "Updated Branch Name",
//         "phone": "1234567890",
//         "subCategory": "Electronics",
//         "videos": null
//           }
// }
// GET @ https://barrim.online/api/wholesaler/branches/68c9928669d5f086c3a49df7
// {
//     "status": 200,
//     "message": "Branch retrieved successfully",
//     "data": {
//         "id": "68c9928669d5f086c3a49df7",
//         "name": "Updated Branch Name",
//         "location": {
//              "country": "Egypt",
//             "governorate": "Cairo",
//             "district": "Nasr City",
//             "city": "Cairo",
//             "lat": 30.0444,
//             "lng": 31.2357
//         },
//         "phone": "1234567890",
//         "category": "Wholesale",
//         "subCategory": "Electronics",
//         "description": "Updated branch description",
//          "images": null,
//         "status": "",
//         "sponsorship": false,
//         "socialMedia": {
//             "facebook": "",
//             "instagram": ""
//         },
//         "createdAt": "2025-09-16T16:38:30.856Z",
//         "updatedAt": "2025-09-16T16:39:10.696Z"
//     }
//     }

// as you see creating a wholesaler branch is creating social links but when i update branch the social links removed, and get branch is not retrieving the social links

// getCompanyBranchSubscriptionIncome gets income from company branch subscriptions in admin_wallet
func (ac *AdminController) getCompanyBranchSubscriptionIncome(ctx context.Context) (float64, error) {
	var totalIncome float64
	cursor, err := ac.DB.Collection("admin_wallet").Find(ctx, bson.M{
		"type":       "subscription_income",
		"entityType": "branch_subscription",
	})
	if err != nil {
		return 0, err
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var transaction models.AdminWallet
		if err := cursor.Decode(&transaction); err != nil {
			continue
		}
		totalIncome += transaction.Amount
	}

	return totalIncome, nil
}

// getWholesalerBranchSubscriptionIncome gets income from wholesaler branch subscriptions in admin_wallet
func (ac *AdminController) getWholesalerBranchSubscriptionIncome(ctx context.Context) (float64, error) {
	var totalIncome float64
	cursor, err := ac.DB.Collection("admin_wallet").Find(ctx, bson.M{
		"type":       "subscription_income",
		"entityType": "wholesaler_branch_subscription",
	})
	if err != nil {
		return 0, err
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var transaction models.AdminWallet
		if err := cursor.Decode(&transaction); err != nil {
			continue
		}
		totalIncome += transaction.Amount
	}

	return totalIncome, nil
}

// getServiceProviderBranchSubscriptionIncome gets income from service provider branch subscriptions in admin_wallet
func (ac *AdminController) getServiceProviderBranchSubscriptionIncome(ctx context.Context) (float64, error) {
	var totalIncome float64
	cursor, err := ac.DB.Collection("admin_wallet").Find(ctx, bson.M{
		"type":       "subscription_income",
		"entityType": "service_provider_branch_subscription",
	})
	if err != nil {
		return 0, err
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var transaction models.AdminWallet
		if err := cursor.Decode(&transaction); err != nil {
			continue
		}
		totalIncome += transaction.Amount
	}

	return totalIncome, nil
}
