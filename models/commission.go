package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Commission struct {
	ID             primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	SubscriptionID primitive.ObjectID `bson:"subscriptionId" json:"subscriptionId"`
	CompanyID      primitive.ObjectID `bson:"companyId" json:"companyId"`
	PlanID         primitive.ObjectID `bson:"planId" json:"planId"`
	PlanPrice      float64            `bson:"planPrice" json:"planPrice"`

	// Admin commission fields
	AdminID                primitive.ObjectID `bson:"adminID" json:"adminId"`
	AdminCommission        float64            `bson:"adminCommission" json:"adminCommission"`
	AdminCommissionPercent float64            `bson:"adminCommissionPercent" json:"adminCommissionPercent"`

	// Salesperson commission fields
	SalespersonID                primitive.ObjectID `bson:"salespersonID" json:"salespersonId"`
	SalespersonCommission        float64            `bson:"salespersonCommission" json:"salespersonCommission"`
	SalespersonCommissionPercent float64            `bson:"salespersonCommissionPercent" json:"salespersonCommissionPercent"`

	// Legacy sales manager fields (keeping for backward compatibility)
	SalesManagerID                primitive.ObjectID `bson:"salesManagerID,omitempty" json:"salesManagerId,omitempty"`
	SalesManagerCommission        float64            `bson:"salesManagerCommission,omitempty" json:"salesManagerCommission,omitempty"`
	SalesManagerCommissionPercent float64            `bson:"salesManagerCommissionPercent,omitempty" json:"salesManagerCommissionPercent,omitempty"`

	CreatedAt time.Time  `bson:"createdAt" json:"createdAt"`
	Paid      bool       `bson:"paid" json:"paid"`
	PaidAt    *time.Time `bson:"paidAt,omitempty" json:"paidAt,omitempty"`
}
