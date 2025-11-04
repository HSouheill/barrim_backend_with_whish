package models

import (
	"encoding/json"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsontype"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Benefits represents the benefits of a subscription plan
type Benefits struct {
	Value interface{} `json:"-" bson:"value"`
}

// UnmarshalBSONValue implements the bson.ValueUnmarshaler interface
func (b *Benefits) UnmarshalBSONValue(t bsontype.Type, data []byte) error {
	var rawValue interface{}
	if err := bson.UnmarshalValue(t, data, &rawValue); err != nil {
		return err
	}

	switch v := rawValue.(type) {
	case string:
		b.Value = v
	case []interface{}:
		// Handle array case
		var benefits []map[string]string
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				benefit := make(map[string]string)
				for k, val := range m {
					if s, ok := val.(string); ok {
						benefit[k] = s
					}
				}
				benefits = append(benefits, benefit)
			}
		}
		b.Value = benefits
	case map[string]interface{}:
		// Handle single map case
		benefit := make(map[string]string)
		for k, val := range v {
			if s, ok := val.(string); ok {
				benefit[k] = s
			}
		}
		b.Value = benefit
	default:
		b.Value = rawValue
	}
	return nil
}

// MarshalBSONValue implements the bson.ValueMarshaler interface
func (b Benefits) MarshalBSONValue() (bsontype.Type, []byte, error) {
	return bson.MarshalValue(b.Value)
}

// MarshalJSON implements custom JSON marshaling to output benefits as a direct list
func (b Benefits) MarshalJSON() ([]byte, error) {
	return json.Marshal(b.Value)
}

// GetString returns the benefits as a string if it's a string type
func (b Benefits) GetString() string {
	if s, ok := b.Value.(string); ok {
		return s
	}
	return ""
}

// GetMaps returns the benefits as a slice of maps if possible
func (b Benefits) GetMaps() []map[string]string {
	if maps, ok := b.Value.([]map[string]string); ok {
		return maps
	}
	return nil
}

// SubscriptionPlan represents a subscription plan for companies, wholesalers, and service providers
type SubscriptionPlan struct {
	ID        primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	Title     string             `json:"title,omitempty" bson:"title,omitempty"`
	Price     float64            `json:"price,omitempty" bson:"price,omitempty"`
	Duration  int                `json:"duration,omitempty" bson:"duration,omitempty"`
	Type      string             `json:"type,omitempty" bson:"type,omitempty"`
	Benefits  Benefits           `json:"benefits,omitempty" bson:"benefits,omitempty"`
	CreatedAt time.Time          `json:"createdAt,omitempty" bson:"createdAt,omitempty"`
	UpdatedAt time.Time          `json:"updatedAt,omitempty" bson:"updatedAt,omitempty"`
	IsActive  bool               `json:"isActive,omitempty" bson:"isActive,omitempty"`
}

// SubscriptionPlanRequest represents the request body for creating/updating subscription plans
type SubscriptionPlanRequest struct {
	Title    string      `json:"title" validate:"required"`
	Price    float64     `json:"price" validate:"required,gt=0"`
	Duration int         `json:"duration" validate:"required,gt=0"`
	Type     string      `json:"type" validate:"required,oneof=company wholesaler serviceProvider"`
	Benefits interface{} `json:"benefits" validate:"required"`
	IsActive bool        `json:"isActive"`
}

// SubscriptionPlanResponse represents the response structure for subscription plan operations
type SubscriptionPlanResponse struct {
	Status  int              `json:"status"`
	Message string           `json:"message"`
	Data    SubscriptionPlan `json:"data,omitempty"`
}

// SubscriptionPlansResponse represents the response structure for multiple subscription plans
type SubscriptionPlansResponse struct {
	Status  int                `json:"status"`
	Message string             `json:"message"`
	Data    []SubscriptionPlan `json:"data,omitempty"`
}

// ActiveSubscription represents an active subscription for a service provider
type ActiveSubscription struct {
	ID                primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	ServiceProviderID primitive.ObjectID `json:"serviceProviderId" bson:"serviceProviderId"`
	PlanID            primitive.ObjectID `json:"planId" bson:"planId"`
	StartDate         time.Time          `json:"startDate" bson:"startDate"`
	EndDate           time.Time          `json:"endDate" bson:"endDate"`
	Status            string             `json:"status" bson:"status"` // active, cancelled, expired
	CancelledAt       *time.Time         `json:"cancelledAt,omitempty" bson:"cancelledAt,omitempty"`
	CreatedAt         time.Time          `json:"createdAt" bson:"createdAt"`
	UpdatedAt         time.Time          `json:"updatedAt" bson:"updatedAt"`
}
