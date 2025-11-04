package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ServiceProviderSubscription represents a service provider's subscription
type ServiceProviderSubscription struct {
	ID                primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	ServiceProviderID primitive.ObjectID `json:"serviceProviderId" bson:"serviceProviderId"` // Reference to the service provider
	PlanID            primitive.ObjectID `json:"planId" bson:"planId"`                       // Reference to the subscribed plan
	StartDate         time.Time          `json:"startDate" bson:"startDate"`
	EndDate           time.Time          `json:"endDate" bson:"endDate"`
	Status            string             `json:"status" bson:"status"`       // e.g., "active", "paused", "expired"
	AutoRenew         bool               `json:"autoRenew" bson:"autoRenew"` // Whether the subscription should auto-renew
	CreatedAt         time.Time          `json:"createdAt" bson:"createdAt"`
	UpdatedAt         time.Time          `json:"updatedAt" bson:"updatedAt"`
}

// ServiceProviderSubscriptionRequest represents a pending subscription request that needs admin approval
type ServiceProviderSubscriptionRequest struct {
	ID                primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	ServiceProviderID primitive.ObjectID `json:"serviceProviderId" bson:"serviceProviderId"`
	PlanID            primitive.ObjectID `json:"planId" bson:"planId"`
	Status            string             `json:"status" bson:"status"` // "pending", "approved", "rejected"
	AdminID           primitive.ObjectID `json:"adminId,omitempty" bson:"adminId,omitempty"`
	ManagerID         primitive.ObjectID `json:"managerId,omitempty" bson:"managerId,omitempty"`
	AdminNote         string             `json:"adminNote,omitempty" bson:"adminNote,omitempty"`
	ManagerNote       string             `json:"managerNote,omitempty" bson:"managerNote,omitempty"`
	AdminApproved     bool               `json:"adminApproved" bson:"adminApproved"`
	ManagerApproved   bool               `json:"managerApproved" bson:"managerApproved"`
	RequestedAt       time.Time          `json:"requestedAt" bson:"requestedAt"`
	ProcessedAt       time.Time          `json:"processedAt,omitempty" bson:"processedAt,omitempty"`
}

// ServiceProviderSubscriptionApprovalRequest represents the request body for approving/rejecting subscriptions
type ServiceProviderSubscriptionApprovalRequest struct {
	Status      string `json:"status"` // "approved" or "rejected"
	AdminNote   string `json:"adminNote,omitempty"`
	ManagerNote string `json:"managerNote,omitempty"`
}

type ServiceProvider struct {
	ID           primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	UserID       primitive.ObjectID `json:"userId,omitempty" bson:"userId,omitempty"`
	BusinessName string             `json:"businessName,omitempty" bson:"businessName,omitempty"`
	Category     string             `json:"category,omitempty" bson:"category,omitempty"`
	// Flat fields for salesperson-created service providers
	Email            string   `json:"email,omitempty" bson:"email,omitempty"`
	Phone            string   `json:"phone,omitempty" bson:"phone,omitempty"`
	Password         string   `json:"password,omitempty" bson:"password,omitempty"`
	ContactPerson    string   `json:"contactPerson,omitempty" bson:"contactPerson,omitempty"`
	ContactPhone     string   `json:"contactPhone,omitempty" bson:"contactPhone,omitempty"`
	Country          string   `json:"country,omitempty" bson:"country,omitempty"`
	Governorate      string   `json:"governorate,omitempty" bson:"governorate,omitempty"`
	District         string   `json:"district,omitempty" bson:"district,omitempty"`
	City             string   `json:"city,omitempty" bson:"city,omitempty"`
	LogoURL          string   `json:"logo,omitempty" bson:"logo,omitempty"`
	ProfilePicURL    string   `json:"profilePicUrl,omitempty" bson:"profilePicUrl,omitempty"`
	AdditionalPhones []string `json:"additionalPhones,omitempty" bson:"additionalPhones,omitempty"`
	AdditionalEmails []string `json:"additionalEmails,omitempty" bson:"additionalEmails,omitempty"`
	// Nested fields for backward compatibility
	ContactInfo       ContactInfo          `json:"contactInfo,omitempty" bson:"contactInfo,omitempty"`
	ReferralCode      string               `json:"referralCode,omitempty" bson:"referralCode,omitempty"`
	Referrals         []primitive.ObjectID `json:"referrals,omitempty" bson:"referrals,omitempty"` // List of referred entities
	Points            int                  `json:"points" bson:"points"`
	CommissionPercent float64              `bson:"commissionPercent,omitempty" json:"commissionPercent,omitempty"`
	Sponsorship       bool                 `json:"sponsorship,omitempty" bson:"sponsorship,omitempty"` // Whether the service provider has active sponsorship
	CreatedBy         primitive.ObjectID   `json:"createdBy,omitempty" bson:"createdBy,omitempty"`
	CreatedAt         time.Time            `json:"createdAt,omitempty" bson:"createdAt,omitempty"`
	UpdatedAt         time.Time            `json:"updatedAt,omitempty" bson:"updatedAt,omitempty"`
	Status            string               `json:"status,omitempty" bson:"status,omitempty"`
	CreationRequest   string               `json:"CreationRequest,omitempty" bson:"CreationRequest,omitempty"` // "approved", "rejected", or ""
	FCMToken          string               `json:"fcmToken,omitempty" bson:"fcmToken,omitempty"`
	// Service provider specific information
	ServiceProviderInfo *ServiceProviderInfo `json:"serviceProviderInfo,omitempty" bson:"serviceProviderInfo,omitempty"`
}
