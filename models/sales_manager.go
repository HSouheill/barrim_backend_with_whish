package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type SalesManager struct {
	ID                primitive.ObjectID   `json:"id,omitempty" bson:"_id,omitempty"`
	FullName          string               `json:"fullName" bson:"fullName"`
	Email             string               `json:"email" bson:"email"`
	Password          string               `json:"password,omitempty" bson:"password"`
	PhoneNumber       string               `json:"phoneNumber" bson:"phoneNumber"`
	Image             string               `json:"image,omitempty" bson:"image,omitempty"`
	CreatedBy         primitive.ObjectID   `json:"createdBy" bson:"createdBy"` // Admin ID
	Salespersons      []primitive.ObjectID `json:"salespersons" bson:"salespersons"`
	RolesAccess       []string             `json:"rolesAccess" bson:"rolesAccess"`
	CommissionPercent float64              `json:"commissionPercent" bson:"commissionPercent"`
	CreatedAt         time.Time            `json:"createdAt" bson:"createdAt"`
	UpdatedAt         time.Time            `json:"updatedAt" bson:"updatedAt"`
}
