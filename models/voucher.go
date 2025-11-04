package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Voucher represents a voucher that can be purchased with points
type Voucher struct {
	ID          primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	Name        string             `json:"name" bson:"name"`
	Description string             `json:"description" bson:"description"`
	Image       string             `json:"image" bson:"image"`
	Points      int                `json:"points" bson:"points"` // Points required to purchase
	IsActive    bool               `json:"isActive" bson:"isActive"`
	CreatedBy   primitive.ObjectID `json:"createdBy" bson:"createdBy"` // Admin who created the voucher
	CreatedAt   time.Time          `json:"createdAt" bson:"createdAt"`
	UpdatedAt   time.Time          `json:"updatedAt" bson:"updatedAt"`
	// User-type specific voucher fields
	TargetUserType string `json:"targetUserType,omitempty" bson:"targetUserType,omitempty"` // "user", "company", "serviceProvider", "wholesaler"
}

// VoucherPurchase represents a user's purchase of a voucher
type VoucherPurchase struct {
	ID          primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	UserID      primitive.ObjectID `json:"userId" bson:"userId"`
	VoucherID   primitive.ObjectID `json:"voucherId" bson:"voucherId"`
	PointsUsed  int                `json:"pointsUsed" bson:"pointsUsed"`
	PurchasedAt time.Time          `json:"purchasedAt" bson:"purchasedAt"`
	IsUsed      bool               `json:"isUsed" bson:"isUsed"`
	UsedAt      time.Time          `json:"usedAt,omitempty" bson:"usedAt,omitempty"`
}

// VoucherRequest represents the request body for creating/updating vouchers
type VoucherRequest struct {
	Name        string `json:"name" validate:"required"`
	Description string `json:"description" validate:"required"`
	Image       string `json:"image" validate:"required"`
	Points      int    `json:"points" validate:"required,min=1"`
}

// UserTypeVoucherRequest represents the request body for creating user-type specific vouchers
// Note: This is now used for multipart form data, not JSON
type UserTypeVoucherRequest struct {
	Name           string `form:"name" validate:"required"`
	Description    string `form:"description" validate:"required"`
	Points         int    `form:"points" validate:"required,min=0"`
	TargetUserType string `form:"targetUserType" validate:"required,oneof=user company serviceProvider wholesaler"`
	// Image is handled as multipart file upload, not form field
}

// VoucherPurchaseRequest represents the request body for purchasing a voucher
type VoucherPurchaseRequest struct {
	VoucherID string `json:"voucherId" validate:"required"`
}

// VoucherResponse represents the response structure for voucher operations
type VoucherResponse struct {
	Status  int         `json:"status"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// UserVoucher represents a voucher with purchase information for a user
type UserVoucher struct {
	Voucher     Voucher         `json:"voucher"`
	Purchase    VoucherPurchase `json:"purchase"`
	CanPurchase bool            `json:"canPurchase"`
	UserPoints  int             `json:"userPoints"`
}

// CompanyVoucherPurchase represents a company's purchase of a voucher
type CompanyVoucherPurchase struct {
	ID          primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	CompanyID   primitive.ObjectID `json:"companyId" bson:"companyId"`
	VoucherID   primitive.ObjectID `json:"voucherId" bson:"voucherId"`
	PointsUsed  int                `json:"pointsUsed" bson:"pointsUsed"`
	PurchasedAt time.Time          `json:"purchasedAt" bson:"purchasedAt"`
	IsUsed      bool               `json:"isUsed" bson:"isUsed"`
	UsedAt      time.Time          `json:"usedAt,omitempty" bson:"usedAt,omitempty"`
}

// CompanyVoucher represents a voucher with purchase information for a company
type CompanyVoucher struct {
	Voucher       Voucher                `json:"voucher"`
	Purchase      CompanyVoucherPurchase `json:"purchase"`
	CanPurchase   bool                   `json:"canPurchase"`
	CompanyPoints int                    `json:"companyPoints"`
}

// ServiceProviderVoucherPurchase represents a service provider's purchase of a voucher
type ServiceProviderVoucherPurchase struct {
	ID                primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	ServiceProviderID primitive.ObjectID `json:"serviceProviderId" bson:"serviceProviderId"`
	VoucherID         primitive.ObjectID `json:"voucherId" bson:"voucherId"`
	PointsUsed        int                `json:"pointsUsed" bson:"pointsUsed"`
	PurchasedAt       time.Time          `json:"purchasedAt" bson:"purchasedAt"`
	IsUsed            bool               `json:"isUsed" bson:"isUsed"`
	UsedAt            time.Time          `json:"usedAt,omitempty" bson:"usedAt,omitempty"`
}

// ServiceProviderVoucher represents a voucher with purchase information for a service provider
type ServiceProviderVoucher struct {
	Voucher               Voucher                        `json:"voucher"`
	Purchase              ServiceProviderVoucherPurchase `json:"purchase"`
	CanPurchase           bool                           `json:"canPurchase"`
	ServiceProviderPoints int                            `json:"serviceProviderPoints"`
}

// WholesalerVoucherPurchase represents a wholesaler's purchase of a voucher
type WholesalerVoucherPurchase struct {
	ID           primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	WholesalerID primitive.ObjectID `json:"wholesalerId" bson:"wholesalerId"`
	VoucherID    primitive.ObjectID `json:"voucherId" bson:"voucherId"`
	PointsUsed   int                `json:"pointsUsed" bson:"pointsUsed"`
	PurchasedAt  time.Time          `json:"purchasedAt" bson:"purchasedAt"`
	IsUsed       bool               `json:"isUsed" bson:"isUsed"`
	UsedAt       time.Time          `json:"usedAt,omitempty" bson:"usedAt,omitempty"`
}

// WholesalerVoucher represents a voucher with purchase information for a wholesaler
type WholesalerVoucher struct {
	Voucher          Voucher                   `json:"voucher"`
	Purchase         WholesalerVoucherPurchase `json:"purchase"`
	CanPurchase      bool                      `json:"canPurchase"`
	WholesalerPoints int                       `json:"wholesalerPoints"`
}
