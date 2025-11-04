package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Withdrawal struct {
	ID              primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	UserID          primitive.ObjectID  `bson:"userId" json:"userId"`
	UserType        string              `bson:"userType" json:"userType"`
	Amount          float64             `bson:"amount" json:"amount"`
	Status          string              `bson:"status" json:"status"` // e.g., "pending", "approved", "rejected"
	CreatedAt       time.Time           `bson:"createdAt" json:"createdAt"`
	ProcessedAt     *time.Time          `bson:"processedAt,omitempty" json:"processedAt,omitempty"`
	AdminID         *primitive.ObjectID `bson:"adminId,omitempty" json:"adminId,omitempty"`
	AdminNote       string              `bson:"adminNote,omitempty" json:"adminNote,omitempty"`
	UserNote        string              `bson:"userNote,omitempty" json:"userNote,omitempty"`
	RejectionReason string              `bson:"rejectionReason,omitempty" json:"rejectionReason,omitempty"`
}
