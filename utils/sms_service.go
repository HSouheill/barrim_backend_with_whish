package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SMSService handles SMS sending using BestSMSBulk API
type SMSService struct {
	Username string
	Password string
	SenderID string
	APIPath  string
	Client   *http.Client
}

// SMSResponse represents the response from BestSMSBulk API
type SMSResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Data    struct {
		MessageID string `json:"message_id"`
		Cost      string `json:"cost"`
	} `json:"data"`
}

// NewSMSService creates a new SMS service instance
func NewSMSService() *SMSService {
	return &SMSService{
		Username: "barrim",
		Password: "9Z9ZBarrim@&$",
		SenderID: "Barrim",
		APIPath:  "https://www.bestsmsbulk.com/bestsmsbulkapi/common/sendSmsWpAPI.php",
		Client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SendOTP sends an OTP via SMS using BestSMSBulk API
func (s *SMSService) SendOTP(phoneNumber, otp string) error {
	// Prepare query parameters
	params := url.Values{}
	params.Set("username", s.Username)
	params.Set("password", s.Password)
	params.Set("senderid", s.SenderID)
	params.Set("destination", phoneNumber)
	params.Set("message", otp)
	params.Set("route", "wp") // wp = WhatsApp route
	params.Set("template", "otp")
	params.Set("variables", otp)

	// Build the full URL
	fullURL := fmt.Sprintf("%s?%s", s.APIPath, params.Encode())

	// Create HTTP request
	req, err := http.NewRequest("POST", fullURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "Barrim-OTP-Service/1.0")

	// Log the request for debugging
	fmt.Printf("📤 Sending OTP via WhatsApp to: %s | Route: wp | OTP: %s\n", phoneNumber, otp)
	fmt.Printf("🔗 API URL: %s\n", fullURL)

	// Send the request
	resp, err := s.Client.Do(req)
	if err != nil {
		fmt.Printf("❌ SMS Request Error: %v\n", err)
		return fmt.Errorf("failed to send SMS request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("❌ Failed to read response: %v\n", err)
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// Log the response for debugging
	fmt.Printf("📥 SMS API Response Status: %d\n", resp.StatusCode)
	fmt.Printf("📥 SMS API Response Body: %s\n", string(body))

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("SMS API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var smsResp SMSResponse
	if err := json.Unmarshal(body, &smsResp); err != nil {
		// If JSON parsing fails, check if it's a simple success response
		responseStr := strings.TrimSpace(string(body))
		if strings.Contains(strings.ToLower(responseStr), "success") ||
			strings.Contains(strings.ToLower(responseStr), "sent") ||
			resp.StatusCode == http.StatusOK {
			fmt.Printf("SMS sent successfully to %s (non-JSON response): %s\n", phoneNumber, responseStr)
			return nil
		}
		return fmt.Errorf("failed to parse SMS response: %w", err)
	}

	// Check if SMS was sent successfully
	if smsResp.Status == "success" || smsResp.Status == "sent" {
		fmt.Printf("SMS sent successfully to %s, Message ID: %s\n", phoneNumber, smsResp.Data.MessageID)
		return nil
	}

	return fmt.Errorf("SMS sending failed: %s", smsResp.Message)
}

// SendOTPViaSMS sends a 6-digit OTP via SMS using BestSMSBulk API
// This function maintains compatibility with the existing codebase
func SendOTPViaSMS(phone string, otp string) error {
	// Ensure phone number has proper format
	if !strings.HasPrefix(phone, "+") {
		phone = "+" + phone
	}

	// Create SMS service and send OTP
	smsService := NewSMSService()
	return smsService.SendOTP(phone, otp)
}

// SendOTPViaSMSWithMessage sends an OTP with a custom message
func SendOTPViaSMSWithMessage(phone string, otp string, customMessage string) error {
	// Ensure phone number has proper format
	if !strings.HasPrefix(phone, "+") {
		phone = "+" + phone
	}

	// Use custom message if provided, otherwise use default
	message := customMessage
	if message == "" {
		message = fmt.Sprintf("Your Barrim verification code is: %s. This code will expire in 10 minutes.", otp)
	}

	// Create SMS service and send OTP
	smsService := NewSMSService()
	return smsService.SendOTP(phone, message)
}
