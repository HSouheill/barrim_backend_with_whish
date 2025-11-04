package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ReferralCommission represents a commission earned by a salesperson through referrals
type ReferralCommission struct {
	ID            primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	SalespersonID primitive.ObjectID `json:"salespersonId" bson:"salespersonId"`
	UserID        primitive.ObjectID `json:"userId" bson:"userId"`
	Amount        float64            `json:"amount" bson:"amount"`
	ReferralCode  string             `json:"referralCode" bson:"referralCode"`
	Status        string             `json:"status" bson:"status"` // "earned", "paid", "cancelled"
	CreatedAt     time.Time          `json:"createdAt" bson:"createdAt"`
	PaidAt        *time.Time         `json:"paidAt,omitempty" bson:"paidAt,omitempty"`
}
