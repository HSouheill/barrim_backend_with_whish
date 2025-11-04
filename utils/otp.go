// utils/otp.go
package utils

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"time"

	"github.com/go-redis/redis/v8"
)

func GenerateSecureOTP() (string, error) {
	// Generate 6 random bytes
	bytes := make([]byte, 6)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	// Convert to base32 string
	return base32.StdEncoding.EncodeToString(bytes)[:6], nil
}

func ValidateOTPAttempts(userID string, redis *redis.Client) error {
	key := "otp_attempts:" + userID
	attempts, err := redis.Incr(context.Background(), key).Result()
	if err != nil {
		return err
	}

	// Set expiry if first attempt
	if attempts == 1 {
		redis.Expire(context.Background(), key, 1*time.Hour)
	}

	// Limit to 5 attempts per hour
	if attempts > 5 {
		return errors.New("too many OTP attempts")
	}

	return nil
}

// SendOTPViaSMS function moved to utils/sms_service.go
