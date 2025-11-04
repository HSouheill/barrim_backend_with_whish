package utils

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/go-redis/redis/v8"
)

// RememberedCredentials represents the stored credentials for "Remember Me"
type RememberedCredentials struct {
	Email      string    `json:"email"`
	Phone      string    `json:"phone"`
	UserType   string    `json:"userType"`
	UserID     string    `json:"userId"`
	ExpiresAt  time.Time `json:"expiresAt"`
	DeviceInfo string    `json:"deviceInfo"`
}

// RememberMeToken represents the token used to retrieve remembered credentials
type RememberMeToken struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// GenerateRememberMeToken generates a secure token for "Remember Me"
func GenerateRememberMeToken() (string, error) {
	// Generate 32 random bytes
	bytes := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// EncryptCredentials encrypts the credentials before storing in Redis
func EncryptCredentials(credentials RememberedCredentials) (string, error) {
	// Get encryption key from environment variable
	key := os.Getenv("REMEMBER_ME_ENCRYPTION_KEY")
	if key == "" {
		// Fallback to a default key (not recommended for production)
		key = "default-encryption-key-32-bytes-long"
	}

	// Ensure key is exactly 32 bytes
	if len(key) < 32 {
		key = key + "00000000000000000000000000000000"
	}
	key = key[:32]

	// Convert credentials to JSON
	jsonData, err := json.Marshal(credentials)
	if err != nil {
		return "", err
	}

	// Create cipher block
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return "", err
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	// Create nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	// Encrypt
	ciphertext := gcm.Seal(nonce, nonce, jsonData, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptCredentials decrypts the credentials from Redis
func DecryptCredentials(encryptedData string) (*RememberedCredentials, error) {
	// Get encryption key from environment variable
	key := os.Getenv("REMEMBER_ME_ENCRYPTION_KEY")
	if key == "" {
		// Fallback to a default key (not recommended for production)
		key = "default-encryption-key-32-bytes-long"
	}

	// Ensure key is exactly 32 bytes
	if len(key) < 32 {
		key = key + "00000000000000000000000000000000"
	}
	key = key[:32]

	// Decode base64
	ciphertext, err := base64.StdEncoding.DecodeString(encryptedData)
	if err != nil {
		return nil, err
	}

	// Create cipher block
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return nil, err
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// Extract nonce
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	// Unmarshal JSON
	var credentials RememberedCredentials
	if err := json.Unmarshal(plaintext, &credentials); err != nil {
		return nil, err
	}

	return &credentials, nil
}

// StoreRememberedCredentials stores encrypted credentials in Redis
func StoreRememberedCredentials(redisClient *redis.Client, token string, credentials RememberedCredentials, expiration time.Duration) error {
	if redisClient == nil {
		return fmt.Errorf("Redis client not available")
	}

	ctx := context.Background()

	// Encrypt credentials
	encryptedData, err := EncryptCredentials(credentials)
	if err != nil {
		return fmt.Errorf("failed to encrypt credentials: %w", err)
	}

	// Store in Redis with expiration
	key := fmt.Sprintf("remember_me:%s", token)
	err = redisClient.Set(ctx, key, encryptedData, expiration).Err()
	if err != nil {
		return fmt.Errorf("failed to store in Redis: %w", err)
	}

	return nil
}

// RetrieveRememberedCredentials retrieves and decrypts credentials from Redis
func RetrieveRememberedCredentials(redisClient *redis.Client, token string) (*RememberedCredentials, error) {
	if redisClient == nil {
		return nil, fmt.Errorf("Redis client not available")
	}

	ctx := context.Background()

	// Get from Redis
	key := fmt.Sprintf("remember_me:%s", token)
	encryptedData, err := redisClient.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("remember me token not found or expired")
		}
		return nil, fmt.Errorf("Redis error: %w", err)
	}

	// Decrypt credentials
	credentials, err := DecryptCredentials(encryptedData)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt credentials: %w", err)
	}

	// Check if expired
	if time.Now().After(credentials.ExpiresAt) {
		// Remove expired token
		redisClient.Del(ctx, key)
		return nil, fmt.Errorf("remember me token expired")
	}

	return credentials, nil
}

// RemoveRememberedCredentials removes the remembered credentials from Redis
func RemoveRememberedCredentials(redisClient *redis.Client, token string) error {
	if redisClient == nil {
		return fmt.Errorf("Redis client not available")
	}

	ctx := context.Background()
	key := fmt.Sprintf("remember_me:%s", token)

	err := redisClient.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("failed to remove from Redis: %w", err)
	}

	return nil
}

// CleanupExpiredRememberMeTokens removes all expired remember me tokens
func CleanupExpiredRememberMeTokens(redisClient *redis.Client) error {
	if redisClient == nil {
		return fmt.Errorf("Redis client not available")
	}

	ctx := context.Background()

	// Get all remember me keys
	pattern := "remember_me:*"
	keys, err := redisClient.Keys(ctx, pattern).Result()
	if err != nil {
		return fmt.Errorf("failed to get keys: %w", err)
	}

	// Check each key for expiration
	for _, key := range keys {
		encryptedData, err := redisClient.Get(ctx, key).Result()
		if err != nil {
			continue
		}

		// Try to decrypt and check expiration
		credentials, err := DecryptCredentials(encryptedData)
		if err != nil {
			// Remove invalid data
			redisClient.Del(ctx, key)
			continue
		}

		// Remove expired tokens
		if time.Now().After(credentials.ExpiresAt) {
			redisClient.Del(ctx, key)
		}
	}

	return nil
}
