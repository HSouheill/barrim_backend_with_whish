package security

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
)

// GenerateCSRFToken generates a secure random token for CSRF protection
func GenerateCSRFToken() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// ValidateContentType ensures the request has the correct content type
func ValidateContentType(contentType string) bool {
	validTypes := map[string]bool{
		"application/json":                  true,
		"application/x-www-form-urlencoded": true,
		"multipart/form-data":               true,
	}
	return validTypes[contentType]
}

// SanitizeHeaders removes sensitive headers
func SanitizeHeaders(headers http.Header) http.Header {
	sensitiveHeaders := []string{
		"Authorization",
		"Cookie",
		"Set-Cookie",
		"X-CSRF-Token",
	}

	for _, header := range sensitiveHeaders {
		headers.Del(header)
	}
	return headers
}
