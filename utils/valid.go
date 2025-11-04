// utils/validation.go
package utils

import (
	"errors"
	"html"
	"mime/multipart"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// IsValidImageFile checks if the uploaded file is a valid image
func IsValidImageFile(file *multipart.FileHeader) bool {
	// List of allowed image extensions
	allowedExtensions := []string{".jpg", ".jpeg", ".png", ".gif", ".svg"}

	// Get the file extension
	filename := strings.ToLower(file.Filename)

	// Check if the file has an allowed extension
	for _, ext := range allowedExtensions {
		if strings.HasSuffix(filename, ext) {
			return true
		}
	}

	return false
}

// SanitizeInput sanitizes user input to prevent XSS and injection attacks
func SanitizeInput(input string) string {
	// Trim spaces
	input = strings.TrimSpace(input)

	// HTML escape
	input = html.EscapeString(input)

	// Remove control characters
	input = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, input)

	// Remove any potential script tags
	scriptRegex := regexp.MustCompile(`<script[^>]*>.*?</script>`)
	input = scriptRegex.ReplaceAllString(input, "")

	return input
}

// SanitizeEmail sanitizes and validates an email address
func SanitizeEmail(email string) (string, error) {
	email = strings.TrimSpace(email)
	email = strings.ToLower(email)

	// Basic email validation regex
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	if !emailRegex.MatchString(email) {
		return "", errors.New("invalid email format")
	}

	return email, nil
}

// SanitizePhone sanitizes and validates a phone number
func SanitizePhone(phone string) (string, error) {
	// If phone is empty, return empty string (phone is optional)
	if strings.TrimSpace(phone) == "" {
		return "", nil
	}

	// Remove all non-numeric characters except +
	phone = regexp.MustCompile(`[^\d+]`).ReplaceAllString(phone, "")

	// Ensure phone number starts with +
	if !strings.HasPrefix(phone, "+") {
		phone = "+" + phone
	}

	// Basic validation for international phone number
	if len(phone) < 8 || len(phone) > 15 {
		return "", errors.New("invalid phone number length")
	}

	return phone, nil
}

// ValidateFile validates file size and type
func ValidateFile(filename string, size int64) error {
	// Check file size (e.g., 5MB limit)
	if size > 5*1024*1024 {
		return errors.New("file too large")
	}

	// Check file extension
	ext := strings.ToLower(filepath.Ext(filename))
	allowedExts := map[string]bool{
		".jpg":  true,
		".jpeg": true,
		".png":  true,
		".gif":  true,
		".svg":  true,
	}

	if !allowedExts[ext] {
		return errors.New("invalid file type")
	}

	return nil
}

// SanitizeStringArray sanitizes an array of strings
func SanitizeStringArray(inputs []string) []string {
	sanitized := make([]string, len(inputs))
	for i, input := range inputs {
		sanitized[i] = SanitizeInput(input)
	}
	return sanitized
}

// SanitizeMap sanitizes all string values in a map
func SanitizeMap(input map[string]string) map[string]string {
	sanitized := make(map[string]string)
	for k, v := range input {
		sanitized[k] = SanitizeInput(v)
	}
	return sanitized
}
