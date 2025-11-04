package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Manager struct {
	ID          primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	FullName    string             `json:"fullName" bson:"fullName"`
	Email       string             `json:"email" bson:"email"`
	Password    string             `json:"password,omitempty" bson:"password"`
	CreatedAt   time.Time          `json:"createdAt" bson:"createdAt"`
	UpdatedAt   time.Time          `json:"updatedAt" bson:"updatedAt"`
	RolesAccess []string           `json:"rolesAccess" bson:"rolesAccess"`
}
