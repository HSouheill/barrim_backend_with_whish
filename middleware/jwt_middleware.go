// middleware/jwt_middleware.go
package middleware

import (
	"context"
	"errors"
	"log"
	"os"
	"time"

	"github.com/HSouheill/barrim_backend/config"
	"github.com/golang-jwt/jwt"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// JwtCustomClaims for JWT token
type JwtCustomClaims struct {
	UserID   string `json:"userId"`
	Email    string `json:"email"`
	UserType string `json:"userType"`
	jwt.StandardClaims
}

// Valid implements the Claims interface for backward compatibility with Echo's JWT middleware
func (c JwtCustomClaims) Valid() error {
	// Check if token is expired (skip check if ExpiresAt is 0)
	if c.ExpiresAt > 0 && time.Now().Unix() > c.ExpiresAt {
		return errors.New("token is expired")
	}

	// Check if token is used before valid time
	if c.NotBefore > 0 && time.Now().Unix() < c.NotBefore {
		return errors.New("token used before valid")
	}

	return nil
}

// Add token blacklist
var tokenBlacklist = make(map[string]time.Time)

// CleanupBlacklist periodically removes expired tokens from blacklist
func CleanupBlacklist() {
	for {
		time.Sleep(1 * time.Hour)
		now := time.Now()
		for token, expiry := range tokenBlacklist {
			if now.After(expiry) {
				delete(tokenBlacklist, token)
			}
		}
	}
}

// BlacklistToken adds a token to the blacklist
func BlacklistToken(token string, expiry time.Time) {
	tokenBlacklist[token] = expiry
}

// IsTokenBlacklisted checks if a token is blacklisted
func IsTokenBlacklisted(token string) bool {
	_, exists := tokenBlacklist[token]
	return exists
}

// GetJWTSecret returns the JWT secret from environment variables
func GetJWTSecret() string {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		panic("JWT_SECRET environment variable is required")
	}
	return secret
}

// GetJWTConfig returns JWT middleware configuration
func GetJWTConfig() middleware.JWTConfig {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		panic("JWT_SECRET environment variable is required")
	}

	return middleware.JWTConfig{
		Claims:     &JwtCustomClaims{},
		SigningKey: []byte(secret),
		SuccessHandler: func(c echo.Context) {
			user := c.Get("user").(*jwt.Token)
			tokenString := user.Raw

			// Check if token is blacklisted
			if IsTokenBlacklisted(tokenString) {
				c.Error(echo.NewHTTPError(echo.ErrUnauthorized.Code, "Token has been invalidated"))
				return
			}

			claims := user.Claims.(*JwtCustomClaims)

			// Additional security check: Verify user is still active in database
			// This prevents using tokens from deactivated accounts
			if !isUserActive(claims.UserID, c) {
				c.Error(echo.NewHTTPError(echo.ErrUnauthorized.Code, "User account is inactive"))
				return
			}

			c.Set("userId", claims.UserID)
			c.Set("userType", claims.UserType)
			c.Set("email", claims.Email)
		},
		ErrorHandler: func(err error) error {
			return echo.NewHTTPError(echo.ErrUnauthorized.Code, "Invalid or expired token")
		},
	}
}

// isUserActive checks if the user is still active in the database
func isUserActive(userID string, c echo.Context) bool {
	// Get database from context or use a global DB instance
	// This is a simplified check - in production, you might want to cache this
	db, ok := c.Get("db").(*mongo.Client)
	if !ok {
		// If no DB in context, we'll skip this check for now
		// In production, you should always have DB access
		return true
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Convert string ID to ObjectID
	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return false
	}

	// Check if user exists and is active
	var user struct {
		IsActive bool `bson:"isActive"`
	}

	err = db.Database("barrim").Collection("users").FindOne(ctx, bson.M{
		"_id": objID,
	}).Decode(&user)

	if err != nil {
		return false
	}

	return user.IsActive
}

// JWTMiddleware returns a configured JWT middleware
func JWTMiddleware() echo.MiddlewareFunc {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		log.Printf("Warning: JWT_SECRET environment variable is not set")
		return func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				return echo.NewHTTPError(echo.ErrUnauthorized.Code, "JWT configuration error")
			}
		}
	}

	return middleware.JWTWithConfig(middleware.JWTConfig{
		SigningKey: []byte(secret),
		Claims:     &JwtCustomClaims{},
		SuccessHandler: func(c echo.Context) {
			// Set user claims in context after successful validation
			user := c.Get("user").(*jwt.Token)
			claims := user.Claims.(*JwtCustomClaims)

			c.Logger().Infof("JWT middleware - Path: %s, UserID: %s, UserType: %s, Email: %s",
				c.Request().URL.Path, claims.UserID, claims.UserType, claims.Email)

			// Store claims in context for easy access
			c.Set("userId", claims.UserID)
			c.Set("userType", claims.UserType)
			c.Set("email", claims.Email)
		},
		ErrorHandler: func(err error) error {
			// Add more detailed error logging
			log.Printf("JWT middleware error: %v", err)
			if err.Error() == "token contains an invalid number of segments" {
				log.Printf("Token validation failed - Invalid token format. Expected format: header.payload.signature")
				return echo.NewHTTPError(echo.ErrUnauthorized.Code, "Invalid token format")
			}
			return echo.NewHTTPError(echo.ErrUnauthorized.Code, "Please provide valid credentials")
		},
	})
}

// GenerateJWT generates new JWT token with refresh token
func GenerateJWT(userID, email, userType string) (string, string, error) {
	// Set custom claims (no expiration - tokens never expire)
	claims := &JwtCustomClaims{
		UserID:   userID,
		Email:    email,
		UserType: userType,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: 0, // 0 means never expires
			IssuedAt:  time.Now().Unix(),
		},
	}

	// Create token with claims
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Generate refresh token (also never expires)
	refreshClaims := &JwtCustomClaims{
		UserID:   userID,
		Email:    email,
		UserType: userType,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: 0, // 0 means never expires
			IssuedAt:  time.Now().Unix(),
		},
	}
	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)

	// Generate encoded tokens
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		return "", "", errors.New("JWT_SECRET environment variable is required")
	}

	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", "", err
	}

	refreshTokenString, err := refreshToken.SignedString([]byte(secret))
	if err != nil {
		return "", "", err
	}

	return tokenString, refreshTokenString, nil
}

// GetUserFromToken extracts user information from JWT token
func GetUserFromToken(c echo.Context) *JwtCustomClaims {
	user := c.Get("user")
	if user == nil {
		return nil
	}

	token, ok := user.(*jwt.Token)
	if !ok {
		return nil
	}

	claims, ok := token.Claims.(*JwtCustomClaims)
	if !ok {
		return nil
	}

	return claims
}

func ExtractUserID(c echo.Context) (string, error) {
	user := c.Get("user")
	if user == nil {
		return "", errors.New("invalid token")
	}

	token, ok := user.(*jwt.Token)
	if !ok {
		return "", errors.New("invalid token type")
	}

	// First try to get claims as JwtCustomClaims
	if claims, ok := token.Claims.(*JwtCustomClaims); ok {
		return claims.UserID, nil
	}

	// Fallback to MapClaims if needed
	if mapClaims, ok := token.Claims.(jwt.MapClaims); ok {
		if userID, ok := mapClaims["userId"].(string); ok {
			return userID, nil
		}
		if userID, ok := mapClaims["id"].(string); ok {
			return userID, nil
		}
	}

	return "", errors.New("invalid user ID in token")
}

// ExtractUserType safely extracts the user type from the context
func ExtractUserType(c echo.Context) string {
	// First try to get from context keys
	if userType, ok := c.Get("userType").(string); ok && userType != "" {
		return userType
	}

	// If not found, try from token claims
	claims := GetUserFromToken(c)
	if claims != nil {
		return claims.UserType
	}

	return ""
}

func GetUserIDFromToken(c echo.Context) string {
	// First try to get from context keys if already extracted
	if userID, ok := c.Get("userId").(string); ok && userID != "" {
		return userID
	}

	// If not found in context, try extracting from token claims
	claims := GetUserFromToken(c)
	if claims != nil {
		return claims.UserID
	}

	// If token doesn't exist or is invalid, return empty string
	return ""
}

// ActivityTracker middleware updates user's last activity timestamp
func ActivityTracker(db *mongo.Client) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Skip for unauthenticated routes
			userID := GetUserIDFromToken(c)
			if userID == "" {
				return next(c)
			}

			// Convert string ID to ObjectID
			objID, err := primitive.ObjectIDFromHex(userID)
			if err != nil {
				// Just log and continue, don't block the request
				// Consider configuring your logger
				return next(c)
			}

			// Update lastActivityAt and isActive in background
			go func() {
				collection := config.GetCollection(db, "users")
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				now := time.Now()
				filter := bson.M{"_id": objID}
				update := bson.M{"$set": bson.M{
					"lastActivityAt": now,
					"isActive":       true,
					"updatedAt":      now,
				}}

				_, err := collection.UpdateOne(ctx, filter, update)
				if err != nil {
					// Log error but don't fail the request
					// Consider configuring your logger
				}
			}()

			return next(c)
		}
	}
}

// Add a function to automatically mark users as inactive after a certain period
func MarkInactiveUsers(db *mongo.Client, inactiveThreshold time.Duration) {
	collection := config.GetCollection(db, "users")
	ctx := context.Background()

	// Find all active users who haven't had activity in the threshold time
	cutoffTime := time.Now().Add(-inactiveThreshold)
	filter := bson.M{
		"isActive":       true,
		"lastActivityAt": bson.M{"$lt": cutoffTime},
	}

	update := bson.M{"$set": bson.M{"isActive": false}}

	_, err := collection.UpdateMany(ctx, filter, update)
	if err != nil {
		// Log the error
	}
}
