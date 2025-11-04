package utils

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/HSouheill/barrim_backend/models"
)

// GetServiceProviderUserID returns the user ID for a service provider
// This handles both new unified ID approach and legacy service provider ID approach
func GetServiceProviderUserID(serviceProviderID primitive.ObjectID, db *mongo.Client) (primitive.ObjectID, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// First, try to find a user with this ID (new unified approach)
	usersCollection := db.Database("barrim").Collection("users")
	var user models.User
	err := usersCollection.FindOne(ctx, bson.M{"_id": serviceProviderID, "userType": "serviceProvider"}).Decode(&user)
	if err == nil {
		return serviceProviderID, nil
	}

	// If not found, try to find the service provider record and get its user ID (legacy approach)
	serviceProvidersCollection := db.Database("barrim").Collection("serviceProviders")
	var serviceProvider models.ServiceProvider
	err = serviceProvidersCollection.FindOne(ctx, bson.M{"_id": serviceProviderID}).Decode(&serviceProvider)
	if err != nil {
		return primitive.ObjectID{}, errors.New("service provider not found")
	}

	return serviceProvider.UserID, nil
}

// IsServiceProviderOwner checks if a user owns a service provider (handles both unified and legacy IDs)
func IsServiceProviderOwner(userID, serviceProviderID primitive.ObjectID, db *mongo.Client) (bool, error) {
	// Direct match (unified ID approach)
	if userID == serviceProviderID {
		return true, nil
	}

	// Check if user has a service provider ID that matches (legacy approach)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	usersCollection := db.Database("barrim").Collection("users")
	var user models.User
	err := usersCollection.FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
	if err != nil {
		return false, err
	}

	// Check if user's service provider ID matches
	if user.ServiceProviderID != nil && *user.ServiceProviderID == serviceProviderID {
		return true, nil
	}

	return false, nil
}
