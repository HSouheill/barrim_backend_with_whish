package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/HSouheill/barrim_backend/config"
	"github.com/HSouheill/barrim_backend/middleware"
	"github.com/HSouheill/barrim_backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// GoogleAuthService handles Google authentication
type GoogleAuthService struct {
	DB *mongo.Client
}

// GoogleUser represents Google user information
type GoogleUser struct {
	Email       string `json:"email"`
	DisplayName string `json:"name"`
	PhotoURL    string `json:"photoUrl"`
	GoogleID    string `json:"googleId"`
	IDToken     string `json:"idToken"`
	AccessToken string `json:"accessToken"`
}

// NewGoogleAuthService creates a new Google auth service
func NewGoogleAuthService(db *mongo.Client) *GoogleAuthService {
	return &GoogleAuthService{
		DB: db,
	}
}

// AuthenticateUser handles the Google authentication process
func (s *GoogleAuthService) AuthenticateUser(googleUser *GoogleUser) (map[string]interface{}, error) {
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Validate required fields
	if googleUser.Email == "" || googleUser.GoogleID == "" {
		return nil, errors.New("email and Google ID are required")
	}

	// Verify Google token with Google API (optional, but recommended)
	err := s.verifyGoogleToken(googleUser.IDToken)
	if err != nil {
		return nil, fmt.Errorf("failed to verify Google token: %w", err)
	}

	// Get user collection
	collection := config.GetCollection(s.DB, "users")

	// Check if user exists
	var user models.User
	err = collection.FindOne(ctx, bson.M{"email": googleUser.Email}).Decode(&user)

	// Initialize userData for response
	var userData map[string]interface{}

	if err != nil {
		if err == mongo.ErrNoDocuments {
			// User doesn't exist, create new user
			now := time.Now()
			newUser := models.User{
				Email:      googleUser.Email,
				FullName:   googleUser.DisplayName,
				UserType:   "user", // Default user type
				GoogleID:   googleUser.GoogleID,
				ProfilePic: googleUser.PhotoURL,
				Points:     0,
				CreatedAt:  now,
				UpdatedAt:  now,
			}

			// Insert user to database
			result, err := collection.InsertOne(ctx, newUser)
			if err != nil {
				return nil, fmt.Errorf("failed to create user: %w", err)
			}

			// Get inserted ID
			insertedID := result.InsertedID.(primitive.ObjectID)

			// Generate JWT token
			token, refreshToken, err := middleware.GenerateJWT(insertedID.Hex(), newUser.Email, newUser.UserType)
			if err != nil {
				return nil, fmt.Errorf("failed to generate token: %w", err)
			}

			// Set user data for response
			userData = map[string]interface{}{
				"token":        token,
				"refreshToken": refreshToken,
				"user": map[string]interface{}{
					"id":         insertedID,
					"email":      newUser.Email,
					"fullName":   newUser.FullName,
					"userType":   newUser.UserType,
					"points":     newUser.Points,
					"profilePic": newUser.ProfilePic,
				},
			}
		} else {
			return nil, fmt.Errorf("database error: %w", err)
		}
	} else {
		// User exists, update Google info
		update := bson.M{
			"$set": bson.M{
				"googleId":   googleUser.GoogleID,
				"profilePic": googleUser.PhotoURL,
				"updatedAt":  time.Now(),
			},
		}

		_, err = collection.UpdateOne(ctx, bson.M{"email": googleUser.Email}, update)
		if err != nil {
			return nil, fmt.Errorf("failed to update user: %w", err)
		}

		// Generate JWT token
		token, refreshToken, err := middleware.GenerateJWT(user.ID.Hex(), user.Email, user.UserType)
		if err != nil {
			return nil, fmt.Errorf("failed to generate token: %w", err)
		}

		// Set user data for response
		userData = map[string]interface{}{
			"token":        token,
			"refreshToken": refreshToken,
			"user": map[string]interface{}{
				"id":         user.ID,
				"email":      user.Email,
				"fullName":   user.FullName,
				"userType":   user.UserType,
				"points":     user.Points,
				"profilePic": user.ProfilePic,
				"location":   user.Location,
			},
		}
	}

	return userData, nil
}

// verifyGoogleToken verifies the Google ID token
// This is a simplified version. In production, you should use Google's official API
func (s *GoogleAuthService) verifyGoogleToken(idToken string) error {
	// In a real implementation, you would call Google's tokeninfo endpoint:
	// https://oauth2.googleapis.com/tokeninfo?id_token=XYZ123

	// Skip verification for now if token is empty (for testing)
	if idToken == "" {
		return nil
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// Create request to Google's tokeninfo endpoint
	req, err := http.NewRequest("GET", "https://oauth2.googleapis.com/tokeninfo", nil)
	if err != nil {
		return err
	}

	// Add token as query parameter
	q := req.URL.Query()
	q.Add("id_token", idToken)
	req.URL.RawQuery = q.Encode()

	// Send request
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("invalid token, Google API returned: %s", string(body))
	}

	// Parse response
	var tokenInfo map[string]interface{}
	err = json.Unmarshal(body, &tokenInfo)
	if err != nil {
		return err
	}

	// Verify that the token is not expired
	expiry, ok := tokenInfo["exp"].(string)
	if ok {
		expTime, err := time.Parse(time.RFC3339, expiry)
		if err == nil && expTime.Before(time.Now()) {
			return errors.New("token expired")
		}
	}

	return nil
}
