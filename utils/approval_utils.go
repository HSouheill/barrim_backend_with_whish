package utils

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// ApprovePendingRequestByManager approves a pending request and inserts the entity into the main collection
func ApprovePendingRequestByManager(db *mongo.Client, requestID primitive.ObjectID, entityType string) error {
	ctx := context.Background()
	var pendingCollectionName, mainCollectionName, requestField string

	switch entityType {
	case "company":
		pendingCollectionName = "pending_company_requests"
		mainCollectionName = "companies"
		requestField = "company"
	case "wholesaler":
		pendingCollectionName = "pending_wholesaler_requests"
		mainCollectionName = "wholesalers"
		requestField = "wholesaler"
	case "serviceProvider":
		pendingCollectionName = "pending_serviceProviders_requests"
		mainCollectionName = "serviceProviders"
		requestField = "serviceProvider"
	default:
		return fmt.Errorf("invalid entity type")
	}

	pendingColl := db.Database("barrim").Collection(pendingCollectionName)
	mainColl := db.Database("barrim").Collection(mainCollectionName)

	// Get the pending request
	var pendingDoc bson.M
	err := pendingColl.FindOne(ctx, bson.M{"_id": requestID}).Decode(&pendingDoc)
	if err != nil {
		return fmt.Errorf("pending request not found: %w", err)
	}

	// Debug: Print the pending document structure
	fmt.Printf("DEBUG: Pending document keys: %v\n", getKeys(pendingDoc))
	fmt.Printf("DEBUG: Pending document email field: %v\n", pendingDoc["email"])

	// Extract the entity object from the request field
	entityDoc, ok := pendingDoc[requestField].(bson.M)
	if !ok {
		return fmt.Errorf("invalid pending request structure: missing %s", requestField)
	}

	// Get the entity ID if it exists
	var entityID primitive.ObjectID
	if id, exists := entityDoc["_id"]; exists {
		if objID, ok := id.(primitive.ObjectID); ok {
			entityID = objID
		}
	}

	// Ensure the entity status is set to "inactive" before inserting to main collection
	// This ensures entities can login after sales manager approval but only become active after subscription approval
	entityDoc["status"] = "inactive"
	entityDoc["CreationRequest"] = "approved"

	// Set the CreatedBy field from the salesPersonId in the pending request
	if salesPersonID, exists := pendingDoc["salesPersonId"]; exists {
		entityDoc["createdBy"] = salesPersonID
	}

	// If entity already has an ID, use UpdateOne with upsert
	// Otherwise, use InsertOne
	var insertResult *mongo.InsertOneResult
	if !entityID.IsZero() {
		// Remove _id from entityDoc to avoid duplicate key error
		delete(entityDoc, "_id")

		// Insert into main collection
		insertResult, err = mainColl.InsertOne(ctx, entityDoc)
		if err != nil {
			return fmt.Errorf("failed to insert into main collection: %w", err)
		}

		// Get the new entity's ID
		newEntityID, ok := insertResult.InsertedID.(primitive.ObjectID)
		if !ok {
			return fmt.Errorf("failed to get inserted entity ID")
		}
		entityID = newEntityID
	} else {
		// Insert into main collection
		insertResult, err = mainColl.InsertOne(ctx, entityDoc)
		if err != nil {
			return fmt.Errorf("failed to insert into main collection: %w", err)
		}

		// Get the new entity's ID
		entityID, ok = insertResult.InsertedID.(primitive.ObjectID)
		if !ok {
			return fmt.Errorf("failed to get inserted entity ID")
		}
	}

	// Prepare user document
	usersColl := db.Database("barrim").Collection("users")
	var userDoc bson.M

	switch entityType {
	case "company":
		// Extract credentials from pending request (not from entityDoc)
		email := ""
		phone := ""
		fullName := ""
		password := ""
		contactPerson := ""
		contactPhone := ""

		// Get email from pending request
		if v, ok := pendingDoc["email"].(string); ok {
			email = v
			fmt.Printf("DEBUG: Extracted email for company: %s\n", email)
		} else {
			fmt.Printf("DEBUG: Failed to extract email for company. Type: %T, Value: %v\n", pendingDoc["email"], pendingDoc["email"])
		}

		// Get password from pending request
		if v, ok := pendingDoc["password"].(string); ok {
			password = v
		}

		// Get phone from entityDoc contactInfo
		if contactInfo, ok := entityDoc["contactInfo"].(bson.M); ok {
			if v, ok := contactInfo["phone"].(string); ok {
				phone = v
			}
			if v, ok := contactInfo["whatsapp"].(string); ok {
				contactPhone = v
			}
		}

		// Get contactPerson from entityDoc
		if v, ok := entityDoc["contactPerson"].(string); ok {
			contactPerson = v
		}

		// Get business name as full name
		if v, ok := entityDoc["businessName"].(string); ok {
			fullName = v
		}

		// Set up user document
		userDoc = bson.M{
			"email":         email,
			"password":      password,
			"fullName":      fullName,
			"userType":      "company",
			"phone":         phone,
			"contactPerson": contactPerson,
			"contactPhone":  contactPhone,
			"companyId":     entityID,
			"isActive":      true,
			"createdAt":     entityDoc["createdAt"],
			"updatedAt":     entityDoc["updatedAt"],
		}

		fmt.Printf("DEBUG: User document for company: %+v\n", userDoc)

	case "wholesaler":
		// Extract credentials from pending request (not from entityDoc)
		email := ""
		phone := ""
		fullName := ""
		password := ""
		contactPerson := ""
		contactPhone := ""

		// Get email from pending request
		if v, ok := pendingDoc["email"].(string); ok {
			email = v
			fmt.Printf("DEBUG: Extracted email for wholesaler: %s\n", email)
		} else {
			fmt.Printf("DEBUG: Failed to extract email for wholesaler. Type: %T, Value: %v\n", pendingDoc["email"], pendingDoc["email"])
		}

		// Get password from pending request
		if v, ok := pendingDoc["password"].(string); ok {
			password = v
		}

		// Get phone from entityDoc contactInfo or direct field
		if contactInfo, ok := entityDoc["contactInfo"].(bson.M); ok {
			if v, ok := contactInfo["phone"].(string); ok {
				phone = v
			}
			if v, ok := contactInfo["whatsapp"].(string); ok {
				contactPhone = v
			}
		}
		if phone == "" {
			if v, ok := entityDoc["phone"].(string); ok {
				phone = v
			}
		}

		// Get contactPerson from entityDoc
		if v, ok := entityDoc["contactPerson"].(string); ok {
			contactPerson = v
		}

		// Get business name as full name
		if v, ok := entityDoc["businessName"].(string); ok {
			fullName = v
		}

		userDoc = bson.M{
			"email":         email,
			"password":      password,
			"fullName":      fullName,
			"userType":      "wholesaler",
			"phone":         phone,
			"contactPerson": contactPerson,
			"contactPhone":  contactPhone,
			"wholesalerId":  entityID,
			"isActive":      true,
			"createdAt":     entityDoc["createdAt"],
			"updatedAt":     entityDoc["updatedAt"],
		}

	case "serviceProvider":
		// Extract credentials from pending request (not from entityDoc)
		email := ""
		phone := ""
		fullName := ""
		password := ""
		contactPerson := ""
		contactPhone := ""

		// Get email from pending request
		if v, ok := pendingDoc["email"].(string); ok {
			email = v
			fmt.Printf("DEBUG: Extracted email for serviceProvider: %s\n", email)
		} else {
			fmt.Printf("DEBUG: Failed to extract email for serviceProvider. Type: %T, Value: %v\n", pendingDoc["email"], pendingDoc["email"])
		}

		// Get password from pending request
		if v, ok := pendingDoc["password"].(string); ok {
			password = v
		}

		// Get phone from entityDoc
		if v, ok := entityDoc["phone"].(string); ok {
			phone = v
		}

		// Get contactPerson and contactPhone from entityDoc
		if v, ok := entityDoc["contactPerson"].(string); ok {
			contactPerson = v
		}
		if v, ok := entityDoc["contactPhone"].(string); ok {
			contactPhone = v
		}

		// Get business name as full name
		if v, ok := entityDoc["businessName"].(string); ok {
			fullName = v
		}

		userDoc = bson.M{
			"email":             email,
			"password":          password,
			"fullName":          fullName,
			"userType":          "serviceProvider",
			"phone":             phone,
			"contactPerson":     contactPerson,
			"contactPhone":      contactPhone,
			"serviceProviderId": entityID,
			"isActive":          true,
			"createdAt":         entityDoc["createdAt"],
			"updatedAt":         entityDoc["updatedAt"],
		}
	}

	// Only insert user if we have the minimum required fields
	if userDoc != nil && userDoc["email"] != "" && userDoc["password"] != "" {
		insertRes, err := usersColl.InsertOne(ctx, userDoc)
		if err != nil {
			return fmt.Errorf("failed to create user account: %w", err)
		}

		// Update the newly inserted entity with the user's ID for easy lookup later
		if userOID, ok := insertRes.InsertedID.(primitive.ObjectID); ok {
			_, updErr := mainColl.UpdateOne(ctx, bson.M{"_id": entityID}, bson.M{"$set": bson.M{"userId": userOID}})
			if updErr != nil {
				fmt.Printf("warning: failed to set userId on %s document: %v\n", entityType, updErr)
			}
		}
	} else {
		// Log that user creation was skipped due to missing credentials
		fmt.Printf("User creation skipped for %s (ID: %s) due to missing email or password. Email: '%v', Password: '%v'", entityType, entityID.Hex(), userDoc["email"], userDoc["password"])
	}

	// Update the pending request status to approved
	_, err = pendingColl.UpdateOne(ctx, bson.M{"_id": requestID}, bson.M{"$set": bson.M{"status": "approved", "requestStatus": "approved"}})
	if err != nil {
		return fmt.Errorf("failed to update pending request status: %w", err)
	}

	return nil
}

// RejectPendingRequestByManager rejects a pending request and sets its status to rejected
func RejectPendingRequestByManager(db *mongo.Client, requestID primitive.ObjectID, entityType string) error {
	ctx := context.Background()
	var pendingCollectionName string

	switch entityType {
	case "company":
		pendingCollectionName = "pending_company_requests"
	case "wholesaler":
		pendingCollectionName = "pending_wholesaler_requests"
	case "serviceProvider":
		pendingCollectionName = "pending_serviceProviders_requests"
	default:
		return fmt.Errorf("invalid entity type")
	}

	pendingColl := db.Database("barrim").Collection(pendingCollectionName)

	// Update the pending request status to rejected
	_, err := pendingColl.UpdateOne(ctx, bson.M{"_id": requestID}, bson.M{"$set": bson.M{"status": "rejected"}})
	if err != nil {
		return fmt.Errorf("failed to update pending request status: %w", err)
	}

	return nil
}

// Helper function to get keys from a bson.M
func getKeys(m bson.M) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
