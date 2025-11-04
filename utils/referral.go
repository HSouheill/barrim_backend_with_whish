package utils

import (
	"crypto/rand"
	"encoding/base32"
	"strings"
)

// ReferralType represents the type of entity for which a referral code is being generated
type ReferralType string

const (
	UserType            ReferralType = "USR"
	CompanyType         ReferralType = "COM"
	ServiceProviderType ReferralType = "SP"
	WholesalerType      ReferralType = "WS"
	SalespersonType     ReferralType = "SPR"
)

// GenerateReferralCode generates a unique referral code for the specified entity type
// Format: {TYPE}-{RANDOM} where RANDOM is 6 alphanumeric characters
// Example: COM-ABC123, SP-XYZ789, WS-DEF456
func GenerateReferralCode(entityType ReferralType) (string, error) {
	// Generate 4 random bytes (will give us 6 characters in base32)
	randomBytes := make([]byte, 4)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return "", err
	}

	// Convert to base32 and take first 6 characters
	randomStr := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(randomBytes)
	randomStr = randomStr[:6]

	// Convert to uppercase and remove any non-alphanumeric characters
	randomStr = strings.ToUpper(randomStr)
	randomStr = strings.Map(func(r rune) rune {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return -1
	}, randomStr)

	// Ensure we have exactly 6 characters
	if len(randomStr) < 6 {
		// If we somehow got less than 6 characters, pad with zeros
		randomStr = randomStr + strings.Repeat("0", 6-len(randomStr))
	}

	// Combine with entity type
	return string(entityType) + "-" + randomStr, nil
}

// GenerateCompanyReferralCode generates a referral code for a company
func GenerateCompanyReferralCode() (string, error) {
	return GenerateReferralCode(CompanyType)
}

// GenerateServiceProviderReferralCode generates a referral code for a service provider
func GenerateServiceProviderReferralCode() (string, error) {
	return GenerateReferralCode(ServiceProviderType)
}

// GenerateWholesalerReferralCode generates a referral code for a wholesaler
func GenerateWholesalerReferralCode() (string, error) {
	return GenerateReferralCode(WholesalerType)
}

// GenerateUserReferralCode generates a referral code for a regular user
func GenerateUserReferralCode() (string, error) {
	return GenerateReferralCode(UserType)
}

// GenerateSalespersonReferralCode generates a referral code for a salesperson
func GenerateSalespersonReferralCode() (string, error) {
	return GenerateReferralCode(SalespersonType)
}
