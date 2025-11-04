// filepath: /my-go-app/my-go-app/src/models/category.go
package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Category struct {
	ID            primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	Name          string             `json:"name" bson:"name"`
	Logo          string             `json:"logo" bson:"logo"`
	Color         string             `json:"color" bson:"color"`
	Subcategories []string           `json:"subcategories" bson:"subcategories"`
	CreatedAt     time.Time          `json:"createdAt" bson:"createdAt"`
	UpdatedAt     time.Time          `json:"updatedAt" bson:"updatedAt"`
}

type AccessRole struct {
	ID   primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	Name string             `json:"name" bson:"name"`
	Key  string             `json:"key" bson:"key"` // e.g., "user_management"
}
