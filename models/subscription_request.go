// models/subscription_request.go
package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type SubscriptionRequest struct {
	ID                primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	CompanyID         primitive.ObjectID `json:"companyId,omitempty" bson:"companyId,omitempty"`
	ServiceProviderID primitive.ObjectID `json:"serviceProviderId,omitempty" bson:"serviceProviderId,omitempty"`
	PlanID            primitive.ObjectID `json:"planId" bson:"planId"`
	Status            string             `json:"status" bson:"status"`
	AdminID           primitive.ObjectID `json:"adminId,omitempty" bson:"adminId,omitempty"`
	AdminNote         string             `json:"adminNote,omitempty" bson:"adminNote,omitempty"`
	RequestedAt       time.Time          `json:"requestedAt" bson:"requestedAt"`
	ProcessedAt       time.Time          `json:"processedAt,omitempty" bson:"processedAt,omitempty"`
	ImagePath         string             `json:"imagePath,omitempty" bson:"imagePath,omitempty"`

	// Approval workflow fields
	AdminApproved   *bool      `json:"adminApproved,omitempty" bson:"adminApproved,omitempty"`
	ManagerApproved *bool      `json:"managerApproved,omitempty" bson:"managerApproved,omitempty"`
	ApprovedBy      string     `json:"approvedBy,omitempty" bson:"approvedBy,omitempty"`
	RejectedBy      string     `json:"rejectedBy,omitempty" bson:"rejectedBy,omitempty"`
	ApprovedAt      *time.Time `json:"approvedAt,omitempty" bson:"approvedAt,omitempty"`
	RejectedAt      *time.Time `json:"rejectedAt,omitempty" bson:"rejectedAt,omitempty"`

	// Whish payment fields
	ExternalID    int64     `json:"externalId,omitempty" bson:"externalId,omitempty"`       // Whish payment external ID
	PaymentStatus string    `json:"paymentStatus,omitempty" bson:"paymentStatus,omitempty"` // "pending", "success", "failed"
	CollectURL    string    `json:"collectUrl,omitempty" bson:"collectUrl,omitempty"`       // Whish payment URL
	PaidAt        time.Time `json:"paidAt,omitempty" bson:"paidAt,omitempty"`
}
