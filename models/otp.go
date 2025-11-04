package models

import (
	"time"
)

// PhoneOTP represents the OTP verification data
type PhoneOTP struct {
	Phone      string         `bson:"phone"`
	OTP        string         `bson:"otp"`
	SignupData *SignupRequest `bson:"signupData,omitempty"`
	ExpiresAt  time.Time      `bson:"expiresAt"`
	Verified   bool           `bson:"verified"`
}
