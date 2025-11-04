package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Ad struct {
	ID        primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	ImageURL  string             `json:"imageUrl" bson:"imageUrl"`
	CreatedBy primitive.ObjectID `json:"createdBy" bson:"createdBy"` // The admin who posted the ad
	CreatedAt time.Time          `json:"createdAt" bson:"createdAt"`
	IsActive  bool               `json:"isActive" bson:"isActive"`
}

type AdRequest struct {
	// Image to be sent via multipart/form-data, so we don't need fields
}

type AdsResponse struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
	Data    []Ad   `json:"data,omitempty"`
}
