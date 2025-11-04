package services

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/HSouheill/barrim_backend/config"
	"github.com/HSouheill/barrim_backend/middleware"
	"github.com/HSouheill/barrim_backend/models"
	"github.com/golang-jwt/jwt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// GoogleCloudAuthService handles Google Cloud Platform authentication
type GoogleCloudAuthService struct {
	DB *mongo.Client
}

// GoogleCloudUser represents Google Cloud user information
type GoogleCloudUser struct {
	IDToken     string `json:"idToken"`
	AccessToken string `json:"accessToken,omitempty"`
}

// GoogleTokenInfo represents the response from Google's tokeninfo endpoint
type GoogleTokenInfo struct {
	Iss           string `json:"iss"`
	Sub           string `json:"sub"`
	Aud           string `json:"aud"`
	Exp           string `json:"exp"`
	Iat           string `json:"iat"`
	Email         string `json:"email"`
	EmailVerified string `json:"email_verified"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Locale        string `json:"locale"`
}

// GoogleJWK represents a Google JSON Web Key
type GoogleJWK struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// GoogleJWKS represents the Google JSON Web Key Set
type GoogleJWKS struct {
	Keys []GoogleJWK `json:"keys"`
}

// NewGoogleCloudAuthService creates a new Google Cloud auth service
func NewGoogleCloudAuthService(db *mongo.Client) *GoogleCloudAuthService {
	return &GoogleCloudAuthService{
		DB: db,
	}
}

// AuthenticateWithGoogleCloud handles Google Cloud Platform authentication
func (s *GoogleCloudAuthService) AuthenticateWithGoogleCloud(googleUser *GoogleCloudUser) (map[string]interface{}, error) {
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Validate required fields
	if googleUser.IDToken == "" {
		return nil, fmt.Errorf("ID token is required")
	}

	// Verify and parse the Google ID token
	tokenInfo, err := s.verifyGoogleIDToken(googleUser.IDToken)
	if err != nil {
		return nil, fmt.Errorf("failed to verify Google ID token: %w", err)
	}

	// Validate email verification
	// Google can return email_verified as boolean true or string "true"
	if tokenInfo.EmailVerified != "true" {
		// Log the actual value for debugging
		fmt.Printf("Email verification status: '%s'\n", tokenInfo.EmailVerified)
		// For development, we'll be more lenient and only require email to be present
		// In production, you might want to be more strict and require verified emails
		if tokenInfo.Email == "" {
			return nil, fmt.Errorf("email is required")
		}
		fmt.Printf("Warning: Email not verified by Google, but proceeding with authentication\n")
	}

	// Ensure we have required fields
	if tokenInfo.Email == "" {
		return nil, fmt.Errorf("email is required")
	}
	if tokenInfo.Sub == "" {
		return nil, fmt.Errorf("Google user ID is required")
	}

	// Get user collection
	collection := config.GetCollection(s.DB, "users")

	// Check if user exists by Google ID first, then by email
	var user models.User
	err = collection.FindOne(ctx, bson.M{"googleID": tokenInfo.Sub}).Decode(&user)

	if err != nil && err == mongo.ErrNoDocuments {
		// User not found by Google ID, check by email
		err = collection.FindOne(ctx, bson.M{"email": tokenInfo.Email}).Decode(&user)
	}

	var userData map[string]interface{}

	if err != nil {
		if err == mongo.ErrNoDocuments {
			// User doesn't exist, create new user
			now := time.Now()
			newUser := models.User{
				ID:         primitive.NewObjectID(),
				Email:      tokenInfo.Email,
				FullName:   tokenInfo.Name,
				UserType:   "user", // Default user type
				GoogleID:   tokenInfo.Sub,
				ProfilePic: tokenInfo.Picture,
				Points:     0,
				CreatedAt:  now,
				UpdatedAt:  now,
			}

			// Insert user to database
			_, err = collection.InsertOne(ctx, newUser)
			if err != nil {
				return nil, fmt.Errorf("failed to create user: %w", err)
			}

			// Generate JWT token
			token, refreshToken, err := middleware.GenerateJWT(newUser.ID.Hex(), newUser.Email, newUser.UserType)
			if err != nil {
				return nil, fmt.Errorf("failed to generate token: %w", err)
			}

			// Set user data for response
			userData = map[string]interface{}{
				"token":        token,
				"refreshToken": refreshToken,
				"user": map[string]interface{}{
					"id":         newUser.ID,
					"email":      newUser.Email,
					"fullName":   newUser.FullName,
					"userType":   newUser.UserType,
					"points":     newUser.Points,
					"profilePic": newUser.ProfilePic,
					"googleID":   newUser.GoogleID,
				},
			}
		} else {
			return nil, fmt.Errorf("database error: %w", err)
		}
	} else {
		// User exists, update Google info if needed
		update := bson.M{
			"$set": bson.M{
				"googleID":   tokenInfo.Sub,
				"profilePic": tokenInfo.Picture,
				"updatedAt":  time.Now(),
			},
		}

		// Only update if Google ID is not already set
		if user.GoogleID == "" {
			_, err = collection.UpdateOne(ctx, bson.M{"_id": user.ID}, update)
			if err != nil {
				return nil, fmt.Errorf("failed to update user: %w", err)
			}
			user.GoogleID = tokenInfo.Sub
			user.ProfilePic = tokenInfo.Picture
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
				"googleID":   user.GoogleID,
				"location":   user.Location,
			},
		}
	}

	return userData, nil
}

// verifyGoogleIDToken verifies the Google ID token using Google's public keys
func (s *GoogleCloudAuthService) verifyGoogleIDToken(idToken string) (*GoogleTokenInfo, error) {
	// Parse the JWT header to get the key ID
	parts := strings.Split(idToken, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid token format")
	}

	// Decode header
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("failed to decode JWT header: %w", err)
	}

	var header struct {
		Kid string `json:"kid"`
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, fmt.Errorf("failed to parse JWT header: %w", err)
	}

	// Fetch Google's public keys
	jwks, err := s.fetchGoogleJWKS()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Google JWKS: %w", err)
	}

	// Find the matching key
	var matchingKey *GoogleJWK
	for _, key := range jwks.Keys {
		if key.Kid == header.Kid {
			matchingKey = &key
			break
		}
	}

	if matchingKey == nil {
		return nil, fmt.Errorf("no matching key found for kid: %s", header.Kid)
	}

	// Convert JWK to RSA public key
	publicKey, err := s.jwkToRSAPublicKey(matchingKey)
	if err != nil {
		return nil, fmt.Errorf("failed to convert JWK to RSA public key: %w", err)
	}

	// Parse and verify the JWT
	token, err := jwt.Parse(idToken, func(token *jwt.Token) (interface{}, error) {
		if token.Method.Alg() != header.Alg {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return publicKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse JWT: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	// Extract claims
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("failed to extract claims")
	}

	// Convert claims to GoogleTokenInfo
	tokenInfo := &GoogleTokenInfo{
		Iss:           getStringFromClaims(claims, "iss"),
		Sub:           getStringFromClaims(claims, "sub"),
		Aud:           getStringFromClaims(claims, "aud"),
		Exp:           getStringFromClaims(claims, "exp"),
		Iat:           getStringFromClaims(claims, "iat"),
		Email:         getStringFromClaims(claims, "email"),
		EmailVerified: getEmailVerifiedFromClaims(claims),
		Name:          getStringFromClaims(claims, "name"),
		Picture:       getStringFromClaims(claims, "picture"),
		GivenName:     getStringFromClaims(claims, "given_name"),
		FamilyName:    getStringFromClaims(claims, "family_name"),
		Locale:        getStringFromClaims(claims, "locale"),
	}

	// Debug logging
	fmt.Printf("Token info - Email: %s, EmailVerified: %s, Name: %s, Sub: %s\n",
		tokenInfo.Email, tokenInfo.EmailVerified, tokenInfo.Name, tokenInfo.Sub)

	// Validate token issuer
	if tokenInfo.Iss != "https://accounts.google.com" && tokenInfo.Iss != "accounts.google.com" {
		return nil, fmt.Errorf("invalid token issuer: %s", tokenInfo.Iss)
	}

	// Validate audience (your Google OAuth client ID)
	// You should replace this with your actual Google OAuth client ID
	// For now, we'll skip this validation, but in production you should validate it
	// if tokenInfo.Aud != "your-google-oauth-client-id" {
	//     return nil, fmt.Errorf("invalid token audience: %s", tokenInfo.Aud)
	// }

	// Check token expiration
	if tokenInfo.Exp != "" {
		expTime, err := time.Parse(time.RFC3339, tokenInfo.Exp)
		if err == nil && expTime.Before(time.Now()) {
			return nil, fmt.Errorf("token expired")
		}
	}

	return tokenInfo, nil
}

// fetchGoogleJWKS fetches Google's JSON Web Key Set
func (s *GoogleCloudAuthService) fetchGoogleJWKS() (*GoogleJWKS, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get("https://www.googleapis.com/oauth2/v3/certs")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch JWKS: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var jwks GoogleJWKS
	if err := json.Unmarshal(body, &jwks); err != nil {
		return nil, err
	}

	return &jwks, nil
}

// jwkToRSAPublicKey converts a Google JWK to an RSA public key
func (s *GoogleCloudAuthService) jwkToRSAPublicKey(jwk *GoogleJWK) (*rsa.PublicKey, error) {
	// Decode the modulus (n)
	nBytes, err := base64.RawURLEncoding.DecodeString(jwk.N)
	if err != nil {
		return nil, fmt.Errorf("failed to decode modulus: %w", err)
	}

	// Decode the exponent (e)
	eBytes, err := base64.RawURLEncoding.DecodeString(jwk.E)
	if err != nil {
		return nil, fmt.Errorf("failed to decode exponent: %w", err)
	}

	// Convert bytes to big integers
	n := new(big.Int).SetBytes(nBytes)
	e := new(big.Int).SetBytes(eBytes)

	// Create RSA public key
	publicKey := &rsa.PublicKey{
		N: n,
		E: int(e.Int64()),
	}

	return publicKey, nil
}

// getStringFromClaims safely extracts string values from JWT claims
func getStringFromClaims(claims jwt.MapClaims, key string) string {
	if val, ok := claims[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// getEmailVerifiedFromClaims safely extracts email_verified from JWT claims
// Google can return this as either a boolean or string
func getEmailVerifiedFromClaims(claims jwt.MapClaims) string {
	if val, ok := claims["email_verified"]; ok {
		switch v := val.(type) {
		case bool:
			if v {
				return "true"
			}
			return "false"
		case string:
			return v
		}
	}
	return "false"
}
