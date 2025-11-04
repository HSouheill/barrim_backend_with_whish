package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Booking model
type Booking struct {
	ID                primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	UserID            primitive.ObjectID `json:"userId" bson:"userId"`
	ServiceProviderID primitive.ObjectID `json:"serviceProviderId" bson:"serviceProviderId"`
	BookingDate       time.Time          `json:"bookingDate" bson:"bookingDate"`
	TimeSlot          string             `json:"timeSlot" bson:"timeSlot"`
	PhoneNumber       string             `json:"phoneNumber" bson:"phoneNumber"`
	Details           string             `json:"details" bson:"details"`
	IsEmergency       bool               `json:"isEmergency" bson:"isEmergency"`
	Status            string             `json:"status" bson:"status"`                                         // "pending", "accepted", "rejected", "confirmed", "completed", "cancelled"
	ProviderResponse  string             `json:"providerResponse,omitempty" bson:"providerResponse,omitempty"` // Optional message from service provider
	MediaTypes        []string           `json:"mediaTypes,omitempty" bson:"mediaTypes,omitempty"`             // Array of "image" or "video"
	MediaURLs         []string           `json:"mediaUrls,omitempty" bson:"mediaUrls,omitempty"`               // Array of URLs to the uploaded media
	ThumbnailURLs     []string           `json:"thumbnailUrls,omitempty" bson:"thumbnailUrls,omitempty"`       // Array of URLs to the thumbnails (for videos)
	CreatedAt         time.Time          `json:"createdAt" bson:"createdAt"`
	UpdatedAt         time.Time          `json:"updatedAt" bson:"updatedAt"`
}

// BookingRequest model
type BookingRequest struct {
	ServiceProviderID string    `json:"serviceProviderId"`
	BookingDate       time.Time `json:"bookingDate"`
	TimeSlot          string    `json:"timeSlot"`
	PhoneNumber       string    `json:"phoneNumber"`
	Details           string    `json:"details"`
	IsEmergency       bool      `json:"isEmergency"`
	MediaTypes        []string  `json:"mediaTypes,omitempty"`     // Array of "image" or "video"
	MediaFiles        []string  `json:"mediaFiles,omitempty"`     // Array of Base64 encoded media files
	MediaFileNames    []string  `json:"mediaFileNames,omitempty"` // Array of original filenames of the media
}

// BookingStatusUpdateRequest model for updating booking status
type BookingStatusUpdateRequest struct {
	Status           string `json:"status"`
	ProviderResponse string `json:"providerResponse,omitempty"`
}

// BookingResponse model
type BookingResponse struct {
	Status  int      `json:"status"`
	Message string   `json:"message"`
	Data    *Booking `json:"data,omitempty"`
}

// BookingsResponse model for multiple bookings
type BookingsResponse struct {
	Status  int       `json:"status"`
	Message string    `json:"message"`
	Data    []Booking `json:"data,omitempty"`
}
