package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type WholesalerCategory struct {
	ID            primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	Name          string             `json:"name" bson:"name"`
	Subcategories []string           `json:"subcategories" bson:"subcategories"`
	CreatedAt     time.Time          `json:"createdAt" bson:"createdAt"`
	UpdatedAt     time.Time          `json:"updatedAt" bson:"updatedAt"`
}
