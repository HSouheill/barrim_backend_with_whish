package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ApprovalRequest represents a generic approval request
type ApprovalRequest struct {
	ID              primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	EntityType      string             `json:"entityType" bson:"entityType"` // "company", "serviceProvider", "wholesaler"
	EntityID        primitive.ObjectID `json:"entityId" bson:"entityId"`
	UserID          primitive.ObjectID `json:"userId" bson:"userId"`
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

// ApprovalResponse represents the response for approval operations
type ApprovalResponse struct {
	Status   string `json:"status"` // "approved", "rejected", "pending"
	Note     string `json:"note,omitempty"`
	UserID   string `json:"userId"`
	UserType string `json:"userType"`
}

// PendingApprovalRequest represents a pending approval request with entity details
type PendingApprovalRequest struct {
	ID           primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	EntityType   string             `json:"entityType" bson:"entityType"`
	EntityID     primitive.ObjectID `json:"entityId" bson:"entityId"`
	UserID       primitive.ObjectID `json:"userId" bson:"userId"`
	BusinessName string             `json:"businessName" bson:"businessName"`
	Category     string             `json:"category" bson:"category"`
	Phone        string             `json:"phone" bson:"phone"`
	Email        string             `json:"email" bson:"email"`
	Status       string             `json:"status" bson:"status"`
	RequestedAt  time.Time          `json:"requestedAt" bson:"requestedAt"`
}
