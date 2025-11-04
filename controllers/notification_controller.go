package controllers

import (
	"context"
	"log"
	"time"

	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"firebase.google.com/go/v4/messaging"
	"github.com/HSouheill/barrim_backend/config"
	"github.com/HSouheill/barrim_backend/middleware"
	"github.com/HSouheill/barrim_backend/models"
)

type NotificationController struct {
	db *mongo.Client
}

func NewNotificationController(db *mongo.Client) *NotificationController {
	return &NotificationController{db: db}
}

// SendToServiceProviderRequest represents the request body for sending notifications to service providers
type SendToServiceProviderRequest struct {
	ServiceProviderID string                 `json:"serviceProviderId" validate:"required"`
	Title             string                 `json:"title" validate:"required"`
	Message           string                 `json:"message" validate:"required"`
	Data              map[string]interface{} `json:"data,omitempty"`
}

// FCMTokenUpdateRequest represents the request body for updating FCM tokens
type FCMTokenUpdateRequest struct {
	FCMToken string `json:"fcmToken" validate:"required"`
}

// SendToServiceProvider sends a notification to a specific service provider
func (nc *NotificationController) SendToServiceProvider(c echo.Context) error {
	var req SendToServiceProviderRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(400, map[string]interface{}{
			"success": false,
			"message": "Invalid request body",
		})
	}

	// Validate required fields
	if req.ServiceProviderID == "" || req.Title == "" || req.Message == "" {
		return c.JSON(400, map[string]interface{}{
			"success": false,
			"message": "Missing required fields",
		})
	}

	// Convert string ID to ObjectID
	serviceProviderObjectID, err := primitive.ObjectIDFromHex(req.ServiceProviderID)
	if err != nil {
		return c.JSON(400, map[string]interface{}{
			"success": false,
			"message": "Invalid service provider ID",
		})
	}

	// Get service provider's FCM token from database
	collection := nc.db.Database("barrim").Collection("serviceProviders")
	var serviceProvider models.ServiceProvider
	err = collection.FindOne(context.Background(), bson.M{"_id": serviceProviderObjectID}).Decode(&serviceProvider)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(404, map[string]interface{}{
				"success": false,
				"message": "Service provider not found",
			})
		}
		log.Printf("Error finding service provider: %v", err)
		return c.JSON(500, map[string]interface{}{
			"success": false,
			"message": "Database error",
		})
	}

	if serviceProvider.FCMToken == "" {
		return c.JSON(404, map[string]interface{}{
			"success": false,
			"message": "Service provider has no FCM token",
		})
	}

	// Prepare FCM message
	message := &messaging.Message{
		Token: serviceProvider.FCMToken,
		Notification: &messaging.Notification{
			Title: req.Title,
			Body:  req.Message,
		},
		Data: nc.prepareNotificationData(req.Data),
		Android: &messaging.AndroidConfig{
			Priority: "high",
			Notification: &messaging.AndroidNotification{
				Sound:     "default",
				ChannelID: "barrim_fcm_channel",
			},
		},
		APNS: &messaging.APNSConfig{
			Payload: &messaging.APNSPayload{
				Aps: &messaging.Aps{
					Alert: &messaging.ApsAlert{
						Title: req.Title,
						Body:  req.Message,
					},
					Sound:    "default",
					Badge:    func() *int { v := 1; return &v }(),
					Category: "BOOKING_REQUEST",
				},
			},
		},
	}

	// Send FCM notification
	ctx := context.Background()

	// Check if Firebase app is initialized
	if config.FirebaseApp == nil {
		log.Printf("Firebase app is not initialized")
		return c.JSON(500, map[string]interface{}{
			"success": false,
			"message": "Firebase app not initialized",
		})
	}

	client, err := config.FirebaseApp.Messaging(ctx)
	if err != nil {
		log.Printf("Error getting messaging client: %v", err)
		return c.JSON(500, map[string]interface{}{
			"success": false,
			"message": "Failed to initialize messaging client",
			"error":   err.Error(),
		})
	}

	response, err := client.Send(ctx, message)
	if err != nil {
		log.Printf("Error sending FCM notification: %v", err)
		return c.JSON(500, map[string]interface{}{
			"success": false,
			"message": "Failed to send notification",
			"error":   err.Error(),
		})
	}

	log.Printf("Notification sent successfully: %s", response)

	return c.JSON(200, map[string]interface{}{
		"success":   true,
		"message":   "Notification sent successfully",
		"messageId": response,
	})
}

// UpdateServiceProviderFCMToken updates the FCM token for a service provider
func (nc *NotificationController) UpdateServiceProviderFCMToken(c echo.Context) error {
	var req FCMTokenUpdateRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(400, map[string]interface{}{
			"success": false,
			"message": "Invalid request body",
		})
	}

	// Get user from JWT token
	claims := middleware.GetUserFromToken(c)
	if claims == nil {
		return c.JSON(401, map[string]interface{}{
			"success": false,
			"message": "Unauthorized",
		})
	}

	// Get user ID from claims
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(400, map[string]interface{}{
			"success": false,
			"message": "Invalid user ID",
		})
	}

	// Find the user's service provider ID
	var user models.User
	err = nc.db.Database("barrim").Collection("users").FindOne(context.Background(), bson.M{"_id": userID}).Decode(&user)
	if err != nil {
		return c.JSON(401, map[string]interface{}{
			"success": false,
			"message": "User not found",
		})
	}

	// Get service provider ID (check if user is a service provider)
	serviceProviderID := userID
	if user.ServiceProviderID != nil {
		serviceProviderID = *user.ServiceProviderID
	}

	// Update FCM token in both users and serviceProviders collections
	usersCollection := nc.db.Database("barrim").Collection("users")
	serviceProvidersCollection := nc.db.Database("barrim").Collection("serviceProviders")

	// Update in users collection
	_, err = usersCollection.UpdateOne(
		context.Background(),
		bson.M{"_id": userID},
		bson.M{"$set": bson.M{"fcmToken": req.FCMToken}},
	)
	if err != nil {
		log.Printf("Error updating user FCM token: %v", err)
	}

	// Update in serviceProviders collection
	_, err = serviceProvidersCollection.UpdateOne(
		context.Background(),
		bson.M{"_id": serviceProviderID},
		bson.M{"$set": bson.M{"fcmToken": req.FCMToken}},
	)
	if err != nil {
		log.Printf("Error updating service provider FCM token: %v", err)
		return c.JSON(500, map[string]interface{}{
			"success": false,
			"message": "Failed to update FCM token",
		})
	}

	return c.JSON(200, map[string]interface{}{
		"success": true,
		"message": "FCM token updated",
	})
}

// UpdateUserFCMToken updates the FCM token for a user
func (nc *NotificationController) UpdateUserFCMToken(c echo.Context) error {
	var req FCMTokenUpdateRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(400, map[string]interface{}{
			"success": false,
			"message": "Invalid request body",
		})
	}

	// Get user from JWT token
	claims := middleware.GetUserFromToken(c)
	if claims == nil {
		return c.JSON(401, map[string]interface{}{
			"success": false,
			"message": "Unauthorized",
		})
	}

	// Get user ID from claims
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(400, map[string]interface{}{
			"success": false,
			"message": "Invalid user ID",
		})
	}

	// Update FCM token in database
	collection := nc.db.Database("barrim").Collection("users")
	_, err = collection.UpdateOne(
		context.Background(),
		bson.M{"_id": userID},
		bson.M{"$set": bson.M{"fcmToken": req.FCMToken}},
	)
	if err != nil {
		log.Printf("Error updating user FCM token: %v", err)
		return c.JSON(500, map[string]interface{}{
			"success": false,
			"message": "Failed to update FCM token",
		})
	}

	return c.JSON(200, map[string]interface{}{
		"success": true,
		"message": "FCM token updated",
	})
}

// prepareNotificationData prepares the notification data with default values
func (nc *NotificationController) prepareNotificationData(data map[string]interface{}) map[string]string {
	result := map[string]string{
		"type":              "booking_request",
		"bookingId":         "",
		"serviceProviderId": "",
		"customerName":      "",
		"serviceType":       "",
		"bookingDate":       "",
		"timeSlot":          "",
		"isEmergency":       "false",
		"timestamp":         time.Now().Format(time.RFC3339),
	}

	// Override with provided data
	if data != nil {
		for key, value := range data {
			if str, ok := value.(string); ok {
				result[key] = str
			} else {
				result[key] = ""
			}
		}
	}

	return result
}
