// controllers/password_controller.go
package controllers

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"gopkg.in/gomail.v2"

	"github.com/HSouheill/barrim_backend/config"
	"github.com/HSouheill/barrim_backend/models"
	"github.com/HSouheill/barrim_backend/utils"
)

// PasswordController handles password reset functionality
type PasswordController struct {
	DB *mongo.Client
}

// NewPasswordController creates a new password controller
func NewPasswordController(db *mongo.Client) *PasswordController {
	return &PasswordController{DB: db}
}

// ForgetPassword initiates the password reset process
func (pc *PasswordController) ForgetPassword(c echo.Context) error {
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Parse request body
	var forgetPassReq struct {
		Email string `json:"email"`
	}
	if err := c.Bind(&forgetPassReq); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}

	// Validate email
	if forgetPassReq.Email == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Email is required",
		})
	}

	// Get user collection
	collection := config.GetCollection(pc.DB, "users")

	// Check if user with email exists
	var user models.User
	err := collection.FindOne(ctx, bson.M{"email": forgetPassReq.Email}).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "No account associated with this email",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to check user",
		})
	}

	// Generate a 4-digit OTP
	otp, err := generateOTP(4)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to generate OTP",
		})
	}

	// Set OTP expiry time (15 minutes from now)
	expiryTime := time.Now().Add(15 * time.Minute)

	// Store OTP and expiry in database
	otpInfo := models.OTPInfo{
		OTP:       otp,
		ExpiresAt: expiryTime,
	}

	// Update user with OTP info
	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": user.ID},
		bson.M{"$set": bson.M{"otpInfo": otpInfo, "updatedAt": time.Now()}},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to save OTP information",
		})
	}

	// Send OTP via email
	err = sendOTPByEmail(user.Email, user.FullName, otp)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to send OTP email",
		})
	}

	// Return masked email for UI
	maskedEmail := maskEmail(user.Email)

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Password reset OTP sent successfully",
		Data: map[string]interface{}{
			"email":  maskedEmail,
			"userId": user.ID.Hex(),
		},
	})
}

// VerifyOTP verifies the OTP provided by the user
func (pc *PasswordController) VerifyOTP(c echo.Context) error {
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Parse request body
	var verifyOTPReq struct {
		UserID string `json:"userId"`
		OTP    string `json:"otp"`
	}
	if err := c.Bind(&verifyOTPReq); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}

	// Validate required fields
	if verifyOTPReq.UserID == "" || verifyOTPReq.OTP == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "User ID and OTP are required",
		})
	}

	// Convert user ID to ObjectID
	userID, err := primitive.ObjectIDFromHex(verifyOTPReq.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	// Get user collection
	collection := config.GetCollection(pc.DB, "users")

	// Find user with OTP
	var user models.User
	err = collection.FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "User not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve user",
		})
	}

	// Check if OTP info exists
	if user.OTPInfo == nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "No OTP request found. Please request a new OTP",
		})
	}

	// Check if OTP is expired
	if time.Now().After(user.OTPInfo.ExpiresAt) {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "OTP has expired. Please request a new OTP",
		})
	}

	// Verify OTP
	if user.OTPInfo.OTP != verifyOTPReq.OTP {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid OTP",
		})
	}

	// Generate a reset token
	resetToken := generateResetToken()
	tokenExpiry := time.Now().Add(1 * time.Hour)

	// Update user with reset token
	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": user.ID},
		bson.M{
			"$set": bson.M{
				"resetPasswordToken":  resetToken,
				"resetTokenExpiresAt": tokenExpiry,
				"updatedAt":           time.Now(),
			},
		},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update reset token",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "OTP verified successfully",
		Data: map[string]interface{}{
			"resetToken": resetToken,
			"userId":     user.ID.Hex(),
		},
	})
}

// ResetPassword resets the user's password
func (pc *PasswordController) ResetPassword(c echo.Context) error {
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Parse request body
	var resetPassReq struct {
		UserID      string `json:"userId"`
		ResetToken  string `json:"resetToken"`
		NewPassword string `json:"newPassword"`
	}
	if err := c.Bind(&resetPassReq); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}

	// Validate required fields
	if resetPassReq.UserID == "" || resetPassReq.ResetToken == "" || resetPassReq.NewPassword == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "User ID, reset token, and new password are required",
		})
	}

	// Password validation
	if len(resetPassReq.NewPassword) < 8 {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Password must be at least 8 characters long",
		})
	}

	// Convert user ID to ObjectID
	userID, err := primitive.ObjectIDFromHex(resetPassReq.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	// Get user collection
	collection := config.GetCollection(pc.DB, "users")

	// Find user with token
	var user models.User
	err = collection.FindOne(ctx, bson.M{
		"_id":                userID,
		"resetPasswordToken": resetPassReq.ResetToken,
	}).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusBadRequest, models.Response{
				Status:  http.StatusBadRequest,
				Message: "Invalid or expired reset token",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve user",
		})
	}

	// Check if token is expired
	if user.ResetTokenExpiresAt.Before(time.Now()) {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Reset token has expired. Please request a new password reset",
		})
	}

	// Hash the new password
	hashedPassword, err := utils.HashPassword(resetPassReq.NewPassword)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to hash password",
		})
	}

	// Update user's password and clear reset token fields
	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": user.ID},
		bson.M{
			"$set": bson.M{
				"password":  hashedPassword,
				"updatedAt": time.Now(),
			},
			"$unset": bson.M{
				"resetPasswordToken":  "",
				"resetTokenExpiresAt": "",
				"otpInfo":             "",
			},
		},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update password",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Password reset successfully",
	})
}

// Helper functions

// generateOTP generates a random OTP of the specified length
func generateOTP(length int) (string, error) {
	const digits = "0123456789"
	result := make([]byte, length)
	for i := 0; i < length; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		if err != nil {
			return "", err
		}
		result[i] = digits[num.Int64()]
	}
	return string(result), nil
}

// sendOTPByEmail sends the OTP to the user's email using SMTP2GO
func sendOTPByEmail(email, name, otp string) error {
	// Get email configuration from environment variables with proper error checking
	smtpHost := os.Getenv("SMTP_HOST")
	smtpPortStr := os.Getenv("SMTP_PORT")
	smtpUser := os.Getenv("SMTP_USER")
	smtpPass := os.Getenv("SMTP_PASS")
	fromEmail := os.Getenv("FROM_EMAIL")

	// Validate required configuration
	if smtpHost == "" || smtpPortStr == "" || smtpUser == "" || smtpPass == "" || fromEmail == "" {
		return fmt.Errorf("missing SMTP configuration")
	}

	// Convert port to integer
	smtpPort, err := strconv.Atoi(smtpPortStr)
	if err != nil {
		return fmt.Errorf("invalid SMTP port: %v", err)
	}

	// Email content
	subject := "Password Reset OTP"
	body := fmt.Sprintf(`
		<html>
		<body>
			<h2>Reset Your Password</h2>
			<p>Hello %s,</p>
			<p>You have requested to reset your password. Please use the following OTP code to verify your request:</p>
			<h3 style="background-color: #f0f0f0; padding: 10px; font-size: 24px; letter-spacing: 5px; text-align: center;">%s</h3>
			<p>This code will expire in 15 minutes.</p>
			<p>If you did not request a password reset, please ignore this email or contact support if you have concerns.</p>
			<p>Thank you,<br>The Barrim Team</p>
		</body>
		</html>
	`, name, otp)

	// Set up the gomail message
	m := gomail.NewMessage()
	m.SetHeader("From", fromEmail)
	m.SetHeader("To", email)
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", body)

	// Create dialer with environment variables
	d := gomail.NewDialer(smtpHost, smtpPort, smtpUser, smtpPass)

	// For debugging - remove in production
	fmt.Printf("Using SMTP config: Host=%s, Port=%d, User=%s, From=%s\n",
		smtpHost, smtpPort, smtpUser, fromEmail)

	// Send email
	if err := d.DialAndSend(m); err != nil {
		return fmt.Errorf("failed to send email: %v", err)
	}

	return nil
}

// maskEmail partially masks an email address for privacy
func maskEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return email // Return original if not a valid email
	}

	name := parts[0]
	domain := parts[1]

	if len(name) <= 2 {
		return name[:1] + "***@" + domain
	}

	return name[:2] + strings.Repeat("*", len(name)-2) + "@" + domain
}

// generateResetToken generates a random token for password reset
func generateResetToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}
