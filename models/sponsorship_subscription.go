package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// SponsorshipSubscription represents an active sponsorship subscription
type SponsorshipSubscription struct {
	ID              primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	SponsorshipID   primitive.ObjectID `json:"sponsorshipId" bson:"sponsorshipId"`     // Reference to the sponsorship
	EntityType      string             `json:"entityType" bson:"entityType"`             // "service_provider", "company_branch", "wholesaler_branch"
	EntityID        primitive.ObjectID `json:"entityId" bson:"entityId"`                 // ID of the service provider or branch
	StartDate       time.Time          `json:"startDate" bson:"startDate"`               // When the subscription becomes active
	EndDate         time.Time          `json:"endDate" bson:"endDate"`                   // When the subscription expires
	Status          string             `json:"status" bson:"status"`                     // "active", "expired", "cancelled"
	AutoRenew       bool               `json:"autoRenew" bson:"autoRenew"`               // Whether to auto-renew
	DiscountApplied float64            `json:"discountApplied" bson:"discountApplied"`   // Actual discount applied
	CreatedAt       time.Time          `json:"createdAt" bson:"createdAt"`
	UpdatedAt       time.Time          `json:"updatedAt" bson:"updatedAt"`
}

// SponsorshipSubscriptionRequest represents a pending subscription request that needs admin approval
type SponsorshipSubscriptionRequest struct {
	ID              primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	SponsorshipID   primitive.ObjectID `json:"sponsorshipId" bson:"sponsorshipId"`     // Reference to the sponsorship
	EntityType      string             `json:"entityType" bson:"entityType"`             // "service_provider", "company_branch", "wholesaler_branch"
	EntityID        primitive.ObjectID `json:"entityId" bson:"entityId"`                 // ID of the service provider or branch
	EntityName      string             `json:"entityName" bson:"entityName"`             // Name of the entity for display
	Status          string             `json:"status" bson:"status"`                     // "pending", "approved", "rejected"
	AdminNote       string             `json:"adminNote,omitempty" bson:"adminNote,omitempty"`
	ManagerNote     string             `json:"managerNote,omitempty" bson:"managerNote,omitempty"`
	AdminApproved   *bool              `json:"adminApproved,omitempty" bson:"adminApproved,omitempty"`
	ManagerApproved *bool              `json:"managerApproved,omitempty" bson:"managerApproved,omitempty"`
	ApprovedBy      string             `json:"approvedBy,omitempty" bson:"approvedBy,omitempty"`
	ApprovedAt      time.Time          `json:"approvedAt,omitempty" bson:"approvedAt,omitempty"`
	RejectedBy      string             `json:"rejectedBy,omitempty" bson:"rejectedBy,omitempty"`
	RejectedAt      time.Time          `json:"rejectedAt,omitempty" bson:"rejectedAt,omitempty"`
	RequestedAt     time.Time          `json:"requestedAt" bson:"requestedAt"`
	ProcessedAt     time.Time          `json:"processedAt,omitempty" bson:"processedAt,omitempty"`
}

// SponsorshipSubscriptionApprovalRequest represents the request body for approving/rejecting subscriptions
type SponsorshipSubscriptionApprovalRequest struct {
	Status      string `json:"status"` // "approved" or "rejected"
	AdminNote   string `json:"adminNote,omitempty"`
	ManagerNote string `json:"managerNote,omitempty"`
}

// SponsorshipSubscriptionRequestRequest represents the request body for creating a subscription request
type SponsorshipSubscriptionRequestRequest struct {
	SponsorshipID primitive.ObjectID `json:"sponsorshipId" validate:"required"`
	EntityType    string             `json:"entityType" validate:"required,oneof=service_provider company_branch wholesaler_branch"`
	EntityID      primitive.ObjectID `json:"entityId" validate:"required"`
}

// SponsorshipSubscriptionResponse represents the response format for sponsorship subscriptions
type SponsorshipSubscriptionResponse struct {
	ID              primitive.ObjectID `json:"id"`
	Sponsorship     Sponsorship        `json:"sponsorship"`
	EntityType      string             `json:"entityType"`
	EntityID        primitive.ObjectID `json:"entityId"`
	EntityName      string             `json:"entityName"`
	StartDate       time.Time          `json:"startDate"`
	EndDate         time.Time          `json:"endDate"`
	Status          string             `json:"status"`
	DiscountApplied float64            `json:"discountApplied"`
}
