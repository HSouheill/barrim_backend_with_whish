package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/HSouheill/barrim_backend/models"
)

// WhishService handles interactions with the Whish API
type WhishService struct {
	baseURL    string
	channel    string
	secret     string
	websiteURL string
	isTesting  bool
}

// NewWhishService creates a new Whish service instance
func NewWhishService() *WhishService {
	// Determine environment (testing or production)
	// Default to production unless WHISH_ENV is set to "testing"
	whishEnv := os.Getenv("WHISH_ENV")
	isTesting := whishEnv == "testing"

	var baseURL string

	baseURL = "https://api.sandbox.whish.money/itel-service/api/"

	// Get credentials from environment variables
	channelStr := os.Getenv("WHISH_CHANNEL")
	secret := os.Getenv("WHISH_SECRET")
	websiteUrl := os.Getenv("WHISH_WEBSITE_URL")

	// Validate required credentials
	if channelStr == "" || secret == "" || websiteUrl == "" {
		log.Printf("WARNING: Whish credentials not fully configured:")
		if channelStr == "" {
			log.Printf("  - WHISH_CHANNEL is missing")
		}
		if secret == "" {
			log.Printf("  - WHISH_SECRET is missing")
		}
		if websiteUrl == "" {
			log.Printf("  - WHISH_WEBSITE_URL is missing")
		}
		log.Printf("Please set these environment variables for Whish payment service to work")
		log.Printf("Set WHISH_ENV=testing to use sandbox, or leave unset for production")
	} else {
		log.Printf("Whish Service Configuration:")
		log.Printf("  Environment: %s", map[bool]string{true: "testing", false: "production"}[isTesting])
		log.Printf("  Base URL: %s", baseURL)
		log.Printf("  Channel: %s", channelStr)
		log.Printf("  Website URL: %s", websiteUrl)
		log.Printf("  Secret: [CONFIGURED]")
	}

	return &WhishService{
		baseURL:    baseURL,
		channel:    channelStr,
		secret:     secret,
		websiteURL: websiteUrl,
		isTesting:  isTesting,
	}
}

// getHeaders returns the standard headers required for Whish API requests
func (s *WhishService) getHeaders() map[string]string {
	return map[string]string{
		"Content-Type": "application/json",
		"channel":      s.channel,
		"secret":       s.secret,
		"websiteurl":   s.websiteURL,
	}
}

// makeRequest performs an HTTP request to the Whish API
func (s *WhishService) makeRequest(method, endpoint string, payload interface{}) (*models.WhishResponse, error) {
	url := s.baseURL + endpoint

	// Create request body
	var body io.Reader
	if payload != nil {
		jsonData, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}
		body = bytes.NewBuffer(jsonData)
	}

	// Create request
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Validate credentials
	if s.channel == "" || s.secret == "" || s.websiteURL == "" {
		return nil, fmt.Errorf("missing Whish credentials. Please set WHISH_CHANNEL, WHISH_SECRET, and WHISH_WEBSITE_URL environment variables")
	}

	// Add headers
	headers := s.getHeaders()

	// Log request details (only in testing or with debug enabled)
	if s.isTesting || os.Getenv("WHISH_DEBUG") == "true" {
		log.Printf("Whish API Request:")
		log.Printf("  URL: %s", url)
		log.Printf("  Method: %s", method)
		for key, value := range headers {
			if key == "secret" {
				log.Printf("  %s: [HIDDEN]", key)
			} else {
				log.Printf("  %s: %s", key, value)
			}
		}
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Send request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Log the raw response for debugging (only in testing or with debug enabled)
	if s.isTesting || os.Getenv("WHISH_DEBUG") == "true" {
		log.Printf("Whish API Response: %s", string(respBody))
	}

	// Parse response
	var whishResp models.WhishResponse
	if err := json.Unmarshal(respBody, &whishResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w\nResponse body: %s", err, string(respBody))
	}

	// Check if the request was successful
	if !whishResp.Status {
		code := "unknown"
		if whishResp.Code != nil {
			if codeStr, ok := whishResp.Code.(string); ok {
				code = codeStr
			} else {
				code = fmt.Sprintf("%v", whishResp.Code)
			}
		}

		// Extract dialog message for better error reporting
		var errorMsg string
		if whishResp.Dialog != nil {
			if dialogMap, ok := whishResp.Dialog.(map[string]interface{}); ok {
				if msg, ok := dialogMap["message"].(string); ok {
					errorMsg = fmt.Sprintf("whish API error: %s - %s", code, msg)
				}
			}
		}

		if errorMsg == "" {
			errorMsg = fmt.Sprintf("whish API error: %s", code)
		}

		log.Printf("Whish API Error Details: Code=%s, Dialog=%v", code, whishResp.Dialog)

		return &whishResp, fmt.Errorf(errorMsg)
	}

	return &whishResp, nil
}

// GetBalance retrieves the real balance of the account
func (s *WhishService) GetBalance() (float64, error) {
	resp, err := s.makeRequest("GET", "payment/account/balance", nil)
	if err != nil {
		return 0, err
	}

	// Extract balance from response
	if balanceDetails, ok := resp.Data["balanceDetails"].(map[string]interface{}); ok {
		if balance, ok := balanceDetails["balance"].(float64); ok {
			return balance, nil
		}
	}

	return 0, fmt.Errorf("failed to parse balance from response")
}

// GetRate returns the current rate/fees that will be deducted from the invoice amount
func (s *WhishService) GetRate(amount float64, currency string) (float64, error) {
	payload := models.WhishRequest{
		Amount:   &amount,
		Currency: currency,
	}

	resp, err := s.makeRequest("POST", "payment/whish/rate", payload)
	if err != nil {
		return 0, err
	}

	// Extract rate from response
	if rate, ok := resp.Data["rate"].(float64); ok {
		return rate, nil
	}

	return 0, fmt.Errorf("failed to parse rate from response")
}

// PostPayment creates a payment and returns the collect URL
func (s *WhishService) PostPayment(req models.WhishRequest) (string, error) {
	resp, err := s.makeRequest("POST", "payment/whish", req)
	if err != nil {
		return "", err
	}

	// Extract collect URL from response
	if collectURL, ok := resp.Data["collectUrl"].(string); ok {
		return collectURL, nil
	}

	return "", fmt.Errorf("failed to parse collect URL from response")
}

// GetPaymentStatus returns the status of a payment transaction
func (s *WhishService) GetPaymentStatus(currency string, externalID int64) (string, string, error) {
	payload := models.WhishRequest{
		Currency:   currency,
		ExternalID: &externalID,
	}

	resp, err := s.makeRequest("POST", "payment/collect/status", payload)
	if err != nil {
		return "", "", err
	}

	// Extract status and phone number from response
	var status, phoneNumber string

	if s, ok := resp.Data["collectStatus"].(string); ok {
		status = s
	}

	if pn, ok := resp.Data["payerPhoneNumber"].(string); ok {
		phoneNumber = pn
	}

	return status, phoneNumber, nil
}
