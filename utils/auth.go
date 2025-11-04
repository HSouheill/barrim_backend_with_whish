// utils/auth.go
package utils

import (
	"context"
	"errors"
	"time"

	"github.com/HSouheill/barrim_backend/middleware"
	customMiddleware "github.com/HSouheill/barrim_backend/middleware"
	"github.com/HSouheill/barrim_backend/models"
	"github.com/golang-jwt/jwt"
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// ValidateTokenResponse represents the response for token validation
type ValidateTokenResponse struct {
	Valid     bool         `json:"valid"`
	User      *models.User `json:"user,omitempty"`
	Message   string       `json:"message,omitempty"`
	ExpiresAt *time.Time   `json:"expiresAt,omitempty"`
}

// ValidateToken validates a JWT token and returns user information if valid
// This function can be used by the frontend to check session validity
func ValidateToken(tokenString string, db *mongo.Client) (*ValidateTokenResponse, error) {
	if tokenString == "" {
		return &ValidateTokenResponse{
			Valid:   false,
			Message: "No token provided",
		}, nil
	}

	// Parse and validate the token
	token, err := jwt.ParseWithClaims(tokenString, &customMiddleware.JwtCustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Validate the signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(middleware.GetJWTSecret()), nil
	})

	if err != nil {
		return &ValidateTokenResponse{
			Valid:   false,
			Message: "Invalid token: " + err.Error(),
		}, nil
	}

	if !token.Valid {
		return &ValidateTokenResponse{
			Valid:   false,
			Message: "Token is not valid",
		}, nil
	}

	// Extract claims
	claims, ok := token.Claims.(*customMiddleware.JwtCustomClaims)
	if !ok {
		return &ValidateTokenResponse{
			Valid:   false,
			Message: "Invalid token claims",
		}, nil
	}

	// Check if token is expired (ExpiresAt is Unix timestamp)
	if claims.ExpiresAt > 0 && time.Now().Unix() > claims.ExpiresAt {
		return &ValidateTokenResponse{
			Valid:   false,
			Message: "Token has expired",
		}, nil
	}

	// Convert string ID to ObjectID
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return &ValidateTokenResponse{
			Valid:   false,
			Message: "Invalid user ID format",
		}, nil
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Find user in database
	usersCollection := db.Database("barrim").Collection("users")
	var user models.User
	err = usersCollection.FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return &ValidateTokenResponse{
				Valid:   false,
				Message: "User not found",
			}, nil
		}
		return &ValidateTokenResponse{
			Valid:   false,
			Message: "Error retrieving user: " + err.Error(),
		}, nil
	}

	// Check if user is active
	if !user.IsActive {
		return &ValidateTokenResponse{
			Valid:   false,
			Message: "User account is inactive",
		}, nil
	}

	// Don't return password in response
	user.Password = ""

	// Calculate token expiration time from Unix timestamp
	var expiresAt *time.Time
	if claims.ExpiresAt > 0 {
		expTime := time.Unix(claims.ExpiresAt, 0)
		expiresAt = &expTime
	}

	return &ValidateTokenResponse{
		Valid:     true,
		User:      &user,
		Message:   "Token is valid",
		ExpiresAt: expiresAt,
	}, nil
}

// ValidateTokenFromHeader extracts token from Authorization header and validates it
func ValidateTokenFromHeader(authHeader string, db *mongo.Client) (*ValidateTokenResponse, error) {
	if authHeader == "" {
		return &ValidateTokenResponse{
			Valid:   false,
			Message: "No authorization header provided",
		}, nil
	}

	// Extract token from "Bearer <token>" format
	if len(authHeader) < 7 || authHeader[:7] != "Bearer " {
		return &ValidateTokenResponse{
			Valid:   false,
			Message: "Invalid authorization header format",
		}, nil
	}

	tokenString := authHeader[7:] // Remove "Bearer " prefix
	return ValidateToken(tokenString, db)
}

// GetUserFromToken extracts the user from the JWT token and retrieves the full user object from the database
func GetUserFromToken(c echo.Context, db *mongo.Client) (*models.User, error) {
	// Get user claims from the token
	userToken := c.Get("user")
	if userToken == nil {
		return nil, errors.New("no token found")
	}

	token, ok := userToken.(*jwt.Token)
	if !ok {
		return nil, errors.New("invalid token type")
	}

	claims, ok := token.Claims.(*middleware.JwtCustomClaims)
	if !ok {
		return nil, errors.New("invalid claims type")
	}

	// Convert string ID to ObjectID
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return nil, errors.New("invalid user ID format")
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Find user in database
	usersCollection := db.Database("barrim").Collection("users")
	var user models.User
	err = usersCollection.FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, errors.New("user not found")
		}
		return nil, errors.New("error retrieving user")
	}

	// Don't return password in response
	user.Password = ""

	return &user, nil
}

// GetUserIDFromToken extracts the user ID from the JWT token
func GetUserIDFromToken(c echo.Context) (primitive.ObjectID, error) {
	user := c.Get("user").(*jwt.Token)

	// Try to cast to custom claims first
	if claims, ok := user.Claims.(*customMiddleware.JwtCustomClaims); ok {
		return primitive.ObjectIDFromHex(claims.UserID)
	}

	// Fallback to standard map claims if needed
	if claims, ok := user.Claims.(jwt.MapClaims); ok {
		idStr, ok := claims["id"].(string)
		if !ok {
			return primitive.ObjectID{}, echo.ErrUnauthorized
		}
		return primitive.ObjectIDFromHex(idStr)
	}

	return primitive.ObjectID{}, echo.ErrUnauthorized
}
