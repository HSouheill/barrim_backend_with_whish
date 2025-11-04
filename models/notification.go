package models

import (
    "time"

    "go.mongodb.org/mongo-driver/bson/primitive"
)

// Notification model
type Notification struct {
    ID        primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
    UserID    primitive.ObjectID `json:"userId" bson:"userId"`       // The user who receives the notification
    Title     string             `json:"title" bson:"title"`         // Notification title
    Message   string             `json:"message" bson:"message"`     // Notification message
    Type      string             `json:"type" bson:"type"`           // Notification type (e.g., "booking_update")
    Data      interface{}        `json:"data,omitempty" bson:"data"` // Optional additional data
    IsRead    bool               `json:"isRead" bson:"isRead"`       // Whether the notification has been read
    CreatedAt time.Time          `json:"createdAt" bson:"createdAt"` // Timestamp of notification creation
}