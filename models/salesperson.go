package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Salesperson struct {
	ID                primitive.ObjectID   `json:"id,omitempty" bson:"_id,omitempty"`
	FullName          string               `json:"fullName" bson:"fullName"`
	Email             string               `json:"email" bson:"email"`
	PhoneNumber       string               `json:"phoneNumber" bson:"phoneNumber"`
	Password          string               `json:"password" bson:"password"`
	Image             string               `json:"Image" bson:"Image"`
	SalesManagerID    primitive.ObjectID   `json:"salesManagerId" bson:"salesManagerId"`
	Region            string               `json:"region" bson:"region"`
	ReferralCode      string               `json:"referralCode,omitempty" bson:"referralCode,omitempty"`
	Referrals         []primitive.ObjectID `json:"referrals,omitempty" bson:"referrals,omitempty"`
	ReferralBalance   float64              `json:"referralBalance" bson:"referralBalance"`
	CreatedAt         time.Time            `json:"createdAt" bson:"createdAt"`
	UpdatedAt         time.Time            `json:"updatedAt" bson:"updatedAt"`
	CreatedBy         primitive.ObjectID   `json:"createdBy" bson:"createdBy"`
	CompanyID         primitive.ObjectID   `json:"companyId" bson:"companyId"`
	Commissions       []Commission         `json:"commissions,omitempty" bson:"commissions,omitempty"`
	CommissionPercent float64              `json:"commissionPercent" bson:"commissionPercent"`
}

// type Commission struct {
// 	Amount  float64            `json:"amount" bson:"amount"`
// 	Date    time.Time          `json:"date" bson:"date"`
// 	OrderID primitive.ObjectID `json:"orderId" bson:"orderId"`
// 	Status  string             `json:"status" bson:"status"` // pending, paid
// }

// CommissionRecord tracks commissions for both salesperson and sales manager
// Role: "salesperson" or "sales_manager"
type CommissionRecord struct {
	ID             primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	SubscriptionID primitive.ObjectID `bson:"subscriptionId" json:"subscriptionId"`
	SalespersonID  primitive.ObjectID `bson:"salespersonId" json:"salespersonId"`
	SalesManagerID primitive.ObjectID `bson:"salesManagerId" json:"salesManagerId"`
	Amount         float64            `bson:"amount" json:"amount"`
	Role           string             `bson:"role" json:"role"`
	Status         string             `bson:"status" json:"status"` // pending, paid
	CreatedAt      time.Time          `bson:"createdAt" json:"createdAt"`
}
