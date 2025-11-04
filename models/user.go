// models/user.go
package models

import (
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// User model
type User struct {
	ID                       primitive.ObjectID   `json:"id,omitempty" bson:"_id,omitempty"`
	Email                    string               `json:"email" bson:"email"`
	Password                 string               `json:"password,omitempty" bson:"password"`
	FullName                 string               `json:"fullName" bson:"fullName"`
	UserType                 string               `json:"userType" bson:"userType"`
	IsActive                 bool                 `json:"isActive" bson:"isActive"`
	Status                   string               `json:"status,omitempty" bson:"status,omitempty"` // "pending", "approved", "rejected", "active", "inactive"
	LastActivityAt           time.Time            `json:"lastActivityAt" bson:"lastActivityAt"`
	DateOfBirth              string               `json:"dateOfBirth,omitempty" bson:"dateOfBirth,omitempty"`
	Gender                   string               `json:"gender,omitempty" bson:"gender,omitempty"`
	Phone                    string               `json:"phone,omitempty" bson:"phone,omitempty"`
	ContactPerson            string               `json:"contactPerson,omitempty" bson:"contactPerson,omitempty"`
	ContactPhone             string               `json:"contactPhone,omitempty" bson:"contactPhone,omitempty"`
	Points                   int                  `json:"points" bson:"points"`
	Referrals                []primitive.ObjectID `json:"referrals,omitempty" bson:"referrals,omitempty"`
	ReferralCode             string               `json:"referralCode,omitempty" bson:"referralCode,omitempty"`
	InterestedDeals          []string             `json:"interestedDeals,omitempty" bson:"interestedDeals,omitempty"`
	Location                 *Location            `json:"location,omitempty" bson:"location,omitempty"`
	ServiceProviderInfo      *ServiceProviderInfo `json:"serviceProviderInfo,omitempty" bson:"serviceProviderInfo,omitempty"`
	LogoPath                 string               `json:"logoPath,omitempty" bson:"logoPath,omitempty"`
	OTPInfo                  *OTPInfo             `json:"otpInfo,omitempty" bson:"otpInfo,omitempty"`
	ResetPasswordToken       string               `json:"resetPasswordToken,omitempty" bson:"resetPasswordToken,omitempty"`
	PhoneVerified            bool                 `json:"phoneVerified,omitempty" bson:"phoneVerified,omitempty"`
	ResetTokenExpiresAt      time.Time            `json:"resetTokenExpiresAt,omitempty" bson:"resetTokenExpiresAt,omitempty"`
	GoogleUID                string               `bson:"googleUID,omitempty" json:"googleUID,omitempty"`
	ProfilePic               string               `bson:"profilePic,omitempty" json:"profilePic,omitempty"`
	FavoriteBranches         []primitive.ObjectID `json:"favoriteBranches,omitempty" bson:"favoriteBranches,omitempty"`
	FavoriteServiceProviders []primitive.ObjectID `json:"favoriteServiceProviders,omitempty" bson:"favoriteServiceProviders,omitempty"`
	GoogleID                 string               `json:"googleId,omitempty" bson:"googleId,omitempty"`
	GoogleEmail              string               `json:"googleEmail,omitempty" bson:"googleEmail,omitempty"`
	CreatedAt                time.Time            `json:"createdAt" bson:"createdAt"`
	UpdatedAt                time.Time            `json:"updatedAt" bson:"updatedAt"`
	CompanyID                *primitive.ObjectID  `json:"companyId,omitempty" bson:"companyId,omitempty"`
	WholesalerID             *primitive.ObjectID  `json:"wholesalerId,omitempty" bson:"wholesalerId,omitempty"`
	ServiceProviderID        *primitive.ObjectID  `json:"serviceProviderId,omitempty" bson:"serviceProviderId,omitempty"`
	FirebaseUID              string               `json:"firebaseUID,omitempty" bson:"firebaseUID,omitempty"`
	AppleUserID              string               `bson:"appleUserID,omitempty" json:"appleUserID,omitempty"`
	FCMToken                 string               `json:"fcmToken,omitempty" bson:"fcmToken,omitempty"`
}

type ReferralRequest struct {
	ReferralCode string `json:"referralCode"`
}

type ReferralResponse struct {
	ReferrerID      primitive.ObjectID `json:"referrerId"`
	Referrer        User               `json:"referrer"`
	NewUser         User               `json:"newUser"`
	PointsAdded     int                `json:"pointsAdded"`
	NewReferralCode string             `json:"newReferralCode"`
}

type PointsUpdate struct {
	Points int `json:"points"`
}

type OTPInfo struct {
	OTP       string    `json:"otp" bson:"otp"`
	ExpiresAt time.Time `json:"expiresAt" bson:"expiresAt"`
}

// Location model
type Location struct {
	Country     string  `json:"country" bson:"country"`
	Governorate string  `json:"governorate" bson:"governorate"`
	District    string  `json:"district" bson:"district"`
	City        string  `json:"city" bson:"city"`
	Lat         float64 `json:"lat" bson:"lat"`
	Lng         float64 `json:"lng" bson:"lng"`
	Allowed     bool    `json:"allowed" bson:"allowed"`
}

type SocialLinks struct {
	Facebook  string `json:"facebook,omitempty" bson:"facebook,omitempty"`
	Instagram string `json:"instagram,omitempty" bson:"instagram,omitempty"`
	Twitter   string `json:"twitter,omitempty" bson:"twitter,omitempty"`
	LinkedIn  string `json:"linkedin,omitempty" bson:"linkedin,omitempty"`
	Website   string `json:"website,omitempty" bson:"website,omitempty"`
}

// Update ServiceProviderInfo to include SocialLinks
// Adding description field to ServiceProviderInfo struct
type ServiceProviderInfo struct {
	ServiceType              string               `json:"serviceType" bson:"serviceType"`
	CustomServiceType        string               `json:"customServiceType,omitempty" bson:"customServiceType,omitempty"`
	Description              string               `json:"description,omitempty" bson:"description,omitempty"`
	YearsExperience          interface{}          `json:"yearsExperience" bson:"yearsExperience"`
	ProfilePhoto             string               `json:"profilePhoto,omitempty" bson:"profilePhoto,omitempty"`
	CertificateImages        []string             `json:"certificateImages,omitempty" bson:"certificateImages,omitempty"`
	PortfolioImages          []string             `json:"portfolioImages,omitempty" bson:"portfolioImages,omitempty"`
	AvailableHours           []string             `json:"availableHours,omitempty" bson:"availableHours,omitempty"`
	AvailableDays            []string             `json:"availableDays,omitempty" bson:"availableDays,omitempty"`
	ApplyToAllMonths         bool                 `json:"applyToAllMonths,omitempty" bson:"applyToAllMonths,omitempty"`
	AvailableWeekdays        []string             `json:"availableWeekdays,omitempty" bson:"availableWeekdays,omitempty"`
	Rating                   float64              `json:"rating" bson:"rating"`
	ReferralCode             string               `json:"referralCode,omitempty" bson:"referralCode,omitempty"`
	Points                   int                  `json:"points" bson:"points"`
	Status                   string               `json:"status" bson:"status"` // "available" or "not_available"
	ReferredServiceProviders []primitive.ObjectID `json:"referredServiceProviders,omitempty" bson:"referredServiceProviders,omitempty"`
	SocialLinks              *SocialLinks         `json:"socialLinks,omitempty" bson:"socialLinks,omitempty"`
}

// Update the UpdateServiceProviderRequest to include description
type UpdateServiceProviderRequest struct {
	FullName          string      `json:"fullName,omitempty"`
	Email             string      `json:"email,omitempty"`
	Password          string      `json:"password,omitempty"`
	LogoPath          string      `json:"logoPath,omitempty"`
	Phone             string      `json:"phone,omitempty"`
	ServiceType       string      `json:"serviceType,omitempty"`
	Description       string      `json:"description,omitempty"`       // New field
	CertificateImages []string    `json:"certificateImages,omitempty"` // New field
	YearsExperience   interface{} `json:"yearsExperience,omitempty"`
	Location          *Location   `json:"location,omitempty"`
	AvailableDays     []string    `json:"availableDays,omitempty"`
	AvailableHours    []string    `json:"availableHours,omitempty"`
}

// Step 2: Create a request model for updating social links
type UpdateSocialLinksRequest struct {
	SocialLinks SocialLinks `json:"socialLinks"`
}

// AuthRequest models
type LoginRequest struct {
	Email      string `json:"email,omitempty"`
	Phone      string `json:"phone,omitempty"`
	Password   string `json:"password"`
	RememberMe bool   `json:"rememberMe,omitempty"`
}

type UpdateLocationRequest struct {
	Location *Location `json:"location"`
}

// Response model
type Response struct {
	Status  int         `json:"status"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func (s *ServiceProviderInfo) NormalizeYearsExperience() {
	if s == nil {
		return
	}

	// Convert string to int if needed
	if yearsStr, ok := s.YearsExperience.(string); ok {
		if years, err := strconv.Atoi(yearsStr); err == nil {
			s.YearsExperience = years
		} else {
			// Default to 1 year if conversion fails
			s.YearsExperience = 1
		}
	} else if s.YearsExperience == nil {
		// Default value if missing
		s.YearsExperience = 1
	}
}

type Review struct {
	ID                primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	ServiceProviderID primitive.ObjectID `json:"serviceProviderId" bson:"serviceProviderId"`
	UserID            primitive.ObjectID `json:"userId" bson:"userId"`
	Username          string             `json:"username" bson:"username"`
	UserProfilePic    string             `json:"userProfilePic" bson:"userProfilePic"`
	Rating            int                `json:"rating" bson:"rating"`
	Comment           string             `json:"comment" bson:"comment"`
	MediaType         string             `json:"mediaType,omitempty" bson:"mediaType,omitempty"` // "image" or "video"
	MediaURL          string             `json:"mediaUrl,omitempty" bson:"mediaUrl,omitempty"`
	ThumbnailURL      string             `json:"thumbnailUrl,omitempty" bson:"thumbnailUrl,omitempty"`
	IsVerified        bool               `json:"isVerified" bson:"isVerified"`
	CreatedAt         time.Time          `json:"createdAt" bson:"createdAt"`
	UpdatedAt         time.Time          `json:"updatedAt" bson:"updatedAt"`
	Reply             *ReviewReply       `json:"reply,omitempty" bson:"reply,omitempty"`
}

type ReviewReply struct {
	ServiceProviderID primitive.ObjectID `json:"serviceProviderId" bson:"serviceProviderId"`
	ReplyText         string             `json:"replyText" bson:"replyText"`
	CreatedAt         time.Time          `json:"createdAt" bson:"createdAt"`
}

// ReviewRequest is the model for creating a review (JSON version)
type ReviewRequest struct {
	ServiceProviderID string `json:"serviceProviderId"`
	Rating            int    `json:"rating"`
	Comment           string `json:"comment"`
	MediaType         string `json:"mediaType,omitempty"` // "image" or "video"
}

// ReviewMultipartRequest is the model for creating a review with media upload (multipart form)
type ReviewMultipartRequest struct {
	ServiceProviderID string `form:"serviceProviderId"`
	Rating            int    `form:"rating"`
	Comment           string `form:"comment"`
	MediaType         string `form:"mediaType,omitempty"` // "image" or "video"
}

// ReviewResponse is the model for review responses
type ReviewResponse struct {
	Status  int     `json:"status"`
	Message string  `json:"message"`
	Data    *Review `json:"data,omitempty"`
}

// ReviewsResponse is the model for multiple review responses
type ReviewsResponse struct {
	Status  int      `json:"status"`
	Message string   `json:"message"`
	Data    []Review `json:"data,omitempty"`
}

// / RegenerateAvailableDaysFromWeekdays generates available days for a date range based on weekday preferences
func (s *ServiceProviderInfo) RegenerateAvailableDaysFromWeekdays(startDate, endDate time.Time) []string {
	if s == nil || len(s.AvailableWeekdays) == 0 {
		return []string{}
	}

	allDays := make([]string, 0)

	// Create a set of selected weekdays for faster lookup
	selectedWeekdays := make(map[time.Weekday]bool)
	for _, weekdayStr := range s.AvailableWeekdays {
		switch weekdayStr {
		case "Monday":
			selectedWeekdays[time.Monday] = true
		case "Tuesday":
			selectedWeekdays[time.Tuesday] = true
		case "Wednesday":
			selectedWeekdays[time.Wednesday] = true
		case "Thursday":
			selectedWeekdays[time.Thursday] = true
		case "Friday":
			selectedWeekdays[time.Friday] = true
		case "Saturday":
			selectedWeekdays[time.Saturday] = true
		case "Sunday":
			selectedWeekdays[time.Sunday] = true
		}
	}

	// Iterate through each day in the date range
	for d := startDate; d.Before(endDate) || d.Equal(endDate); d = d.AddDate(0, 0, 1) {
		// Check if this day's weekday is in our selected weekdays
		if selectedWeekdays[d.Weekday()] {
			dateStr := d.Format("2006-01-02")
			allDays = append(allDays, dateStr)
		}
	}

	return allDays
}

// GoogleAuthRequest is the model for Google authentication
type GoogleAuthRequest struct {
	TokenID  string `json:"tokenId"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	PhotoURL string `json:"photoUrl"`
	GoogleID string `json:"googleId"`
}
