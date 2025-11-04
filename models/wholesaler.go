// models/wholesaler.go
package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Wholesaler struct {
	ID               primitive.ObjectID   `json:"id,omitempty" bson:"_id,omitempty"`
	UserID           primitive.ObjectID   `json:"userId" bson:"userId"` // Reference to the user account
	BusinessName     string               `json:"businessName" bson:"businessName"`
	Phone            string               `json:"phone" bson:"phone"`
	AdditionalPhones []string             `json:"additionalPhones,omitempty" bson:"additionalPhones,omitempty"`
	AdditionalEmails []string             `json:"additionalEmails,omitempty" bson:"additionalEmails,omitempty"`
	Category         string               `json:"category" bson:"category"`
	SubCategory      string               `json:"subCategory,omitempty" bson:"subCategory,omitempty"`
	ReferralCode     string               `json:"referralCode,omitempty" bson:"referralCode,omitempty"`
	Referrals        []primitive.ObjectID `json:"referrals,omitempty" bson:"referrals,omitempty"` // Added: List of referred wholesalers
	Points           int                  `json:"points" bson:"points"`
	ContactInfo      ContactInfo          `json:"contactInfo" bson:"contactInfo"`
	ContactPerson    string               `json:"contactPerson,omitempty" bson:"contactPerson,omitempty"`
	SocialMedia      SocialMedia          `json:"socialMedia,omitempty" bson:"socialMedia,omitempty"`
	LogoURL          string               `json:"logoUrl,omitempty" bson:"logoUrl,omitempty"`
	ProfilePicURL    string               `json:"profilePicUrl,omitempty" bson:"profilePicUrl,omitempty"`
	Balance          float64              `json:"balance" bson:"balance"`
	Branches         []Branch             `json:"branches,omitempty" bson:"branches,omitempty"` // Embedded branches (similar to Company model)
	Sponsorship      bool                 `json:"sponsorship" bson:"sponsorship"`               // Whether the wholesaler has active sponsorship
	CreatedBy        primitive.ObjectID   `json:"createdBy" bson:"createdBy"`
	CreatedAt        time.Time            `json:"createdAt" bson:"createdAt"`
	UpdatedAt        time.Time            `json:"updatedAt" bson:"updatedAt"`
	CreationRequest  string               `json:"CreationRequest,omitempty" bson:"CreationRequest,omitempty"` // "approved", "rejected", or ""
}

// WholesalerReferralData provides information about a wholesaler's referrals
type WholesalerReferralData struct {
	ReferralCode  string `json:"referralCode"`
	ReferralCount int    `json:"referralCount"`
	Points        int    `json:"points"`
	ReferralLink  string `json:"referralLink"`
}

type WholesalerReferralResponse struct {
	ReferrerID      primitive.ObjectID `json:"referrerId"`
	Referrer        Wholesaler         `json:"referrer"`
	NewWholesaler   Wholesaler         `json:"newWholesaler"`
	PointsAdded     int                `json:"pointsAdded"`
	NewReferralCode string             `json:"newReferralCode"`
}

type WholesalerSignupRequest struct {
	Email          string                `json:"email"`
	Password       string                `json:"password"`
	FullName       string                `json:"fullName"`
	UserType       string                `json:"userType"`
	Phone          string                `json:"phone"`
	WholesalerData *WholesalerSignupData `json:"wholesalerData"`
}

type WholesalerSignupWithLogoRequest struct {
	Email        string  `form:"email"`
	Password     string  `form:"password"`
	FullName     string  `form:"fullName"`
	BusinessName string  `form:"businessName"`
	Category     string  `form:"category"`
	SubCategory  string  `form:"subCategory"`
	Phone        string  `form:"phone"`
	ReferralCode string  `form:"referralCode"`
	Country      string  `form:"country"`
	Governorate  string  `form:"governorate"`
	District     string  `form:"district"`
	City         string  `form:"city"`
	Lat          float64 `form:"lat"`
	Lng          float64 `form:"lng"`
	Logo         string  `form:"logo"`
}

// Update the existing WholesalerSignupData to include more fields
// type WholesalerSignupData struct {
// 	BusinessName string       `json:"businessName"`
// 	Category     string       `json:"category"`
// 	SubCategory  string       `json:"subCategory,omitempty"`
// 	Logo         string       `json:"logo,omitempty"`
// 	Phone        string       `json:"phone"`
// 	Address      Address      `json:"address"`
// 	ReferralCode string       `json:"referralCode,omitempty"`
// 	SocialMedia  *SocialMedia `json:"socialMedia,omitempty"`
// 	ContactInfo  *ContactInfo `json:"contactInfo,omitempty"`
// }

type WholesalerSubscription struct {
	ID           primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	WholesalerID primitive.ObjectID `json:"wholesalerId" bson:"wholesalerId"` // Reference to the wholesaler
	PlanID       primitive.ObjectID `json:"planId" bson:"planId"`             // Reference to the subscribed plan (can reuse SubscriptionPlan from company model)
	StartDate    time.Time          `json:"startDate" bson:"startDate"`
	EndDate      time.Time          `json:"endDate" bson:"endDate"`
	Status       string             `json:"status" bson:"status"`       // e.g., "active", "paused", "expired"
	AutoRenew    bool               `json:"autoRenew" bson:"autoRenew"` // Whether the subscription should auto-renew
	CreatedAt    time.Time          `json:"createdAt" bson:"createdAt"`
	UpdatedAt    time.Time          `json:"updatedAt" bson:"updatedAt"`
}

// WholesalerSubscriptionRequest represents a pending subscription request that needs admin approval
type WholesalerSubscriptionRequest struct {
	ID              primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	WholesalerID    primitive.ObjectID `json:"wholesalerId" bson:"wholesalerId"`
	PlanID          primitive.ObjectID `json:"planId" bson:"planId"`
	Status          string             `json:"status" bson:"status"` // "pending", "approved", "rejected"
	AdminID         primitive.ObjectID `json:"adminId,omitempty" bson:"adminId,omitempty"`
	ManagerID       primitive.ObjectID `json:"managerId,omitempty" bson:"managerId,omitempty"`
	AdminNote       string             `json:"adminNote,omitempty" bson:"adminNote,omitempty"`
	ManagerNote     string             `json:"managerNote,omitempty" bson:"managerNote,omitempty"`
	AdminApproved   bool               `json:"adminApproved" bson:"adminApproved"`
	ManagerApproved bool               `json:"managerApproved" bson:"managerApproved"`
	RequestedAt     time.Time          `json:"requestedAt" bson:"requestedAt"`
	ProcessedAt     time.Time          `json:"processedAt,omitempty" bson:"processedAt,omitempty"`
}

// WholesalerSubscriptionApprovalRequest represents the request body for approving/rejecting subscriptions
type WholesalerSubscriptionApprovalRequest struct {
	Status      string `json:"status"` // "approved" or "rejected"
	AdminNote   string `json:"adminNote,omitempty"`
	ManagerNote string `json:"managerNote,omitempty"`
}

// WholesalerBranch represents a branch of a wholesaler
// Similar to Branch in company.go
type WholesalerBranch struct {
	ID           primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	WholesalerID primitive.ObjectID `json:"wholesalerId" bson:"wholesalerId"`
	Name         string             `json:"name" bson:"name"`
	Location     Address            `json:"location" bson:"location"`
	Phone        string             `json:"phone" bson:"phone"`
	Category     string             `json:"category" bson:"category"`
	SubCategory  string             `json:"subCategory,omitempty" bson:"subCategory,omitempty"`
	Description  string             `json:"description,omitempty" bson:"description,omitempty"`
	Images       []string           `json:"images" bson:"images"`
	Videos       []string           `json:"videos,omitempty" bson:"videos"`
	Status       string             `json:"status" bson:"status"`
	Sponsorship  bool               `json:"sponsorship" bson:"sponsorship"` // Whether the branch has active sponsorship
	SocialMedia  SocialMedia        `json:"socialMedia" bson:"socialMedia"` // Branch-specific social media links
	CreatedAt    time.Time          `json:"createdAt" bson:"createdAt"`
	UpdatedAt    time.Time          `json:"updatedAt" bson:"updatedAt"`
}

// WholesalerBranchSubscription represents a branch's subscription
// Similar to BranchSubscription in company.go
type WholesalerBranchSubscription struct {
	ID        primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	BranchID  primitive.ObjectID `json:"branchId" bson:"branchId"`
	PlanID    primitive.ObjectID `json:"planId" bson:"planId"`
	StartDate time.Time          `json:"startDate" bson:"startDate"`
	EndDate   time.Time          `json:"endDate" bson:"endDate"`
	Status    string             `json:"status" bson:"status"`
	AutoRenew bool               `json:"autoRenew" bson:"autoRenew"`
	CreatedAt time.Time          `json:"createdAt" bson:"createdAt"`
	UpdatedAt time.Time          `json:"updatedAt" bson:"updatedAt"`
}

// WholesalerBranchSubscriptionRequest represents a subscription request for a wholesaler branch
type WholesalerBranchSubscriptionRequest struct {
	ID              primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	BranchID        primitive.ObjectID `json:"branchId" bson:"branchId"`
	PlanID          primitive.ObjectID `json:"planId" bson:"planId"`
	Status          string             `json:"status" bson:"status"`
	RequestedAt     time.Time          `json:"requestedAt" bson:"requestedAt"`
	ImagePath       string             `json:"imagePath,omitempty" bson:"imagePath,omitempty"`
	AdminNote       string             `json:"adminNote,omitempty" bson:"adminNote,omitempty"`
	AdminApproved   *bool              `json:"adminApproved,omitempty" bson:"adminApproved,omitempty"`
	ManagerApproved *bool              `json:"managerApproved,omitempty" bson:"managerApproved,omitempty"`
	ApprovedBy      string             `json:"approvedBy,omitempty" bson:"approvedBy,omitempty"`
	ApprovedAt      time.Time          `json:"approvedAt,omitempty" bson:"approvedAt,omitempty"`
	RejectedBy      string             `json:"rejectedBy,omitempty" bson:"rejectedBy,omitempty"`
	RejectedAt      time.Time          `json:"rejectedAt,omitempty" bson:"rejectedAt,omitempty"`
	ProcessedAt     time.Time          `json:"processedAt,omitempty" bson:"processedAt,omitempty"`

	// Whish payment fields
	ExternalID    int64     `json:"externalId,omitempty" bson:"externalId,omitempty"`       // Whish payment external ID
	PaymentStatus string    `json:"paymentStatus,omitempty" bson:"paymentStatus,omitempty"` // "pending", "success", "failed"
	CollectURL    string    `json:"collectUrl,omitempty" bson:"collectUrl,omitempty"`       // Whish payment URL
	PaidAt        time.Time `json:"paidAt,omitempty" bson:"paidAt,omitempty"`
}
