// models/company.go
package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Company struct {
	ID               primitive.ObjectID   `json:"id,omitempty" bson:"_id,omitempty"`
	UserID           primitive.ObjectID   `json:"userId" bson:"userId"` // Reference to the user
	Email            string               `json:"email" bson:"email"`
	BusinessName     string               `json:"businessName" bson:"businessName"`
	Category         string               `json:"category" bson:"category"`
	SubCategory      string               `json:"subCategory,omitempty" bson:"subCategory,omitempty"`
	ReferralCode     string               `json:"referralCode,omitempty" bson:"referralCode,omitempty"`
	Referrals        []primitive.ObjectID `json:"referrals,omitempty" bson:"referrals,omitempty"` // Added: List of referred companies
	Points           int                  `json:"points" bson:"points"`
	ContactInfo      ContactInfo          `json:"contactInfo" bson:"contactInfo"`
	ContactPerson    string               `json:"contactPerson,omitempty" bson:"contactPerson,omitempty"`
	AdditionalPhones []string             `json:"additionalPhones,omitempty" bson:"additionalPhones,omitempty"`
	AdditionalEmails []string             `json:"additionalEmails,omitempty" bson:"additionalEmails,omitempty"`
	SocialMedia      SocialMedia          `json:"socialMedia,omitempty" bson:"socialMedia,omitempty"`
	LogoURL          string               `json:"logoUrl,omitempty" bson:"logoUrl,omitempty"`
	ProfilePicURL    string               `json:"profilePicUrl,omitempty" bson:"profilePicUrl,omitempty"`
	Balance          float64              `json:"balance" bson:"balance"`
	Branches         []Branch             `json:"branches,omitempty" bson:"branches,omitempty"`
	Sponsorship      bool                 `json:"sponsorship" bson:"sponsorship"` // Whether the company has active sponsorship
	CreatedBy        primitive.ObjectID   `json:"createdBy" bson:"createdBy"`
	CreatedAt        time.Time            `json:"createdAt" bson:"createdAt"`
	UpdatedAt        time.Time            `json:"updatedAt" bson:"updatedAt"`
	CreationRequest  string               `json:"CreationRequest,omitempty" bson:"CreationRequest,omitempty"` // "approved", "rejected", or ""
}

type ContactInfo struct {
	Phone    string  `json:"phone" bson:"phone"`
	WhatsApp string  `json:"whatsapp,omitempty" bson:"whatsapp,omitempty"`
	Website  string  `json:"website,omitempty" bson:"website,omitempty"`
	Address  Address `json:"address" bson:"address"`
}

type SocialMedia struct {
	Facebook  string `json:"facebook" bson:"facebook"`
	Instagram string `json:"instagram" bson:"instagram"`
}

type Address struct {
	Country     string  `json:"country" bson:"country"`
	Governorate string  `json:"governorate" bson:"governorate"`
	District    string  `json:"district" bson:"district"`
	City        string  `json:"city" bson:"city"`
	Lat         float64 `json:"lat" bson:"lat"`
	Lng         float64 `json:"lng" bson:"lng"`
}

type Branch struct {
	ID              primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	Name            string             `json:"name" bson:"name"`
	Location        Address            `json:"location" bson:"location"`
	Phone           string             `json:"phone" bson:"phone"`
	Category        string             `json:"category" bson:"category"`
	SubCategory     string             `json:"subCategory,omitempty" bson:"subCategory,omitempty"`
	Description     string             `json:"description,omitempty" bson:"description,omitempty"`
	Images          []string           `json:"images" bson:"images"`
	Videos          []string           `json:"videos,omitempty" bson:"videos"`
	CostPerCustomer float64            `json:"costPerCustomer,omitempty" bson:"costPerCustomer,omitempty"`
	Status          string             `json:"status" bson:"status"`           // "pending", "approved", "rejected"
	Sponsorship     bool               `json:"sponsorship" bson:"sponsorship"` // Whether the branch has active sponsorship
	SocialMedia     SocialMedia        `json:"socialMedia" bson:"socialMedia"` // Branch-specific social media links
	CreatedAt       time.Time          `json:"createdAt" bson:"createdAt"`
	UpdatedAt       time.Time          `json:"updatedAt" bson:"updatedAt"`
}

// BranchComment represents a comment on a company branch
type BranchComment struct {
	ID         primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	BranchID   primitive.ObjectID `json:"branchId" bson:"branchId"`
	UserID     primitive.ObjectID `json:"userId" bson:"userId"`
	UserName   string             `json:"userName" bson:"userName"`
	UserAvatar string             `json:"userAvatar,omitempty" bson:"userAvatar,omitempty"`
	Comment    string             `json:"comment" bson:"comment"`
	Rating     int                `json:"rating,omitempty" bson:"rating,omitempty"`
	Replies    []CommentReply     `json:"replies,omitempty" bson:"replies,omitempty"`
	CreatedAt  time.Time          `json:"createdAt" bson:"createdAt"`
	UpdatedAt  time.Time          `json:"updatedAt" bson:"updatedAt"`
}

// CommentReply represents a company's reply to a user comment
type CommentReply struct {
	ID        primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	CompanyID primitive.ObjectID `json:"companyId" bson:"companyId"`
	Reply     string             `json:"reply" bson:"reply"`
	CreatedAt time.Time          `json:"createdAt" bson:"createdAt"`
	UpdatedAt time.Time          `json:"updatedAt" bson:"updatedAt"`
}

// CommentReplyRequest is used when a company replies to a comment
type CommentReplyRequest struct {
	Reply string `json:"reply"`
}

// CompanyReferralRequest is used when a company provides a referral code
type CompanyReferralRequest struct {
	ReferralCode string `json:"referralCode"`
}

// CompanyReferralResponse is the response format for company referrals
type CompanyReferralResponse struct {
	ReferrerID      primitive.ObjectID `json:"referrerId"`
	Referrer        Company            `json:"referrer"`
	NewCompany      Company            `json:"newCompany"`
	PointsAdded     int                `json:"pointsAdded"`
	NewReferralCode string             `json:"newReferralCode"`
}

// CompanyReferralData provides information about a company's referrals
type CompanyReferralData struct {
	ReferralCode  string `json:"referralCode"`
	ReferralCount int    `json:"referralCount"`
	Points        int    `json:"points"`
	ReferralLink  string `json:"referralLink"`
}

// CompanySubscription represents a company's subscription
type CompanySubscription struct {
	ID        primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	CompanyID primitive.ObjectID `json:"companyId" bson:"companyId"` // Reference to the company
	PlanID    primitive.ObjectID `json:"planId" bson:"planId"`       // Reference to the subscribed plan
	StartDate time.Time          `json:"startDate" bson:"startDate"`
	EndDate   time.Time          `json:"endDate" bson:"endDate"`
	Status    string             `json:"status" bson:"status"`       // e.g., "active", "paused", "expired"
	AutoRenew bool               `json:"autoRenew" bson:"autoRenew"` // Whether the subscription should auto-renew
	CreatedAt time.Time          `json:"createdAt" bson:"createdAt"`
	UpdatedAt time.Time          `json:"updatedAt" bson:"updatedAt"`
}

// BranchSubscription represents a branch's subscription
// Similar to CompanySubscription but for branches
type BranchSubscription struct {
	ID        primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	BranchID  primitive.ObjectID `json:"branchId" bson:"branchId"` // Reference to the branch
	PlanID    primitive.ObjectID `json:"planId" bson:"planId"`     // Reference to the subscribed plan
	StartDate time.Time          `json:"startDate" bson:"startDate"`
	EndDate   time.Time          `json:"endDate" bson:"endDate"`
	Status    string             `json:"status" bson:"status"`       // e.g., "active", "paused", "expired"
	AutoRenew bool               `json:"autoRenew" bson:"autoRenew"` // Whether the subscription should auto-renew
	CreatedAt time.Time          `json:"createdAt" bson:"createdAt"`
	UpdatedAt time.Time          `json:"updatedAt" bson:"updatedAt"`
}

// SubscriptionApprovalRequest represents the request body for approving/rejecting subscriptions
type SubscriptionApprovalRequest struct {
	Status    string `json:"status"` // "approved" or "rejected"
	AdminNote string `json:"adminNote,omitempty"`
}

// Update SubscriptionRequest to reference branchId instead of companyId
// (Assuming SubscriptionRequest is defined elsewhere, but if not, define here)

type BranchSubscriptionRequest struct {
	ID              primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	BranchID        primitive.ObjectID `json:"branchId" bson:"branchId"` // Reference to the branch
	PlanID          primitive.ObjectID `json:"planId" bson:"planId"`
	Status          string             `json:"status" bson:"status"` // "pending_payment", "paid", "failed", "active"
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
