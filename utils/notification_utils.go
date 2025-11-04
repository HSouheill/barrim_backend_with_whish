package utils

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"firebase.google.com/go/v4/messaging"
	"github.com/HSouheill/barrim_backend/config"
	"github.com/HSouheill/barrim_backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"gopkg.in/gomail.v2"
)

// SaveNotification saves a notification to the database
func SaveNotification(db *mongo.Client, userID primitive.ObjectID, title, message, notifType string, data interface{}) error {
	collection := db.Database("barrim").Collection("notifications")

	notification := models.Notification{
		ID:        primitive.NewObjectID(),
		UserID:    userID,
		Title:     title,
		Message:   message,
		Type:      notifType,
		Data:      data,
		IsRead:    false,
		CreatedAt: time.Now(),
	}

	_, err := collection.InsertOne(context.Background(), notification)
	return err
}

// NotifySalesManagerOfRequest notifies the sales manager by email and in-app notification
func NotifySalesManagerOfRequest(db *mongo.Client, salesPersonID primitive.ObjectID, entityType, entityName string) error {
	// Find salesperson
	var salesperson models.Salesperson
	err := db.Database("barrim").Collection("salespersons").FindOne(context.Background(), bson.M{"_id": salesPersonID}).Decode(&salesperson)
	if err != nil {
		return fmt.Errorf("failed to find salesperson: %w", err)
	}
	// Find sales manager
	var salesManager models.SalesManager
	err = db.Database("barrim").Collection("sales_managers").FindOne(context.Background(), bson.M{"_id": salesperson.SalesManagerID}).Decode(&salesManager)
	if err != nil {
		return fmt.Errorf("failed to find sales manager: %w", err)
	}
	// Compose email
	subject := fmt.Sprintf("New %s Created", entityType)
	body := fmt.Sprintf("Dear %s,\n\nSalesperson %s has successfully created a new %s: %s.\nThe %s has been automatically approved and is now active in the system.\n\nBest regards,\nYour System", salesManager.FullName, salesperson.FullName, entityType, entityName, entityType)
	// Send email using gomail
	smtpHost := os.Getenv("SMTP_HOST")
	smtpUser := os.Getenv("SMTP_USER")
	smtpPass := os.Getenv("SMTP_PASS")
	smtpPort := 2525
	if portStr := os.Getenv("SMTP_PORT"); portStr != "" {
		fmt.Sscanf(portStr, "%d", &smtpPort)
	}
	m := gomail.NewMessage()
	m.SetHeader("From", smtpUser)
	m.SetHeader("To", salesManager.Email)
	m.SetHeader("Subject", subject)
	m.SetBody("text/plain", body)
	d := gomail.NewDialer(smtpHost, smtpPort, smtpUser, smtpPass)
	if err := d.DialAndSend(m); err != nil {
		log.Printf("Failed to send email to sales manager: %v", err)
	}
	// Save in-app notification
	notifTitle := fmt.Sprintf("New %s Created", entityType)
	notifMsg := fmt.Sprintf("Salesperson %s has created a new %s: %s.", salesperson.FullName, entityType, entityName)
	_ = SaveNotification(db, salesManager.ID, notifTitle, notifMsg, "entity_created", map[string]interface{}{
		"entityType":    entityType,
		"entityName":    entityName,
		"salesPersonId": salesPersonID.Hex(),
	})
	return nil
}

// NotifySalesManagerOfCreatedEntity notifies the sales manager about a newly created entity (no approval needed)
func NotifySalesManagerOfCreatedEntity(db *mongo.Client, salesPersonID primitive.ObjectID, entityType, entityName string) error {
	// Find salesperson
	var salesperson models.Salesperson
	err := db.Database("barrim").Collection("salespersons").FindOne(context.Background(), bson.M{"_id": salesPersonID}).Decode(&salesperson)
	if err != nil {
		return fmt.Errorf("failed to find salesperson: %w", err)
	}
	// Find sales manager
	var salesManager models.SalesManager
	err = db.Database("barrim").Collection("sales_managers").FindOne(context.Background(), bson.M{"_id": salesperson.SalesManagerID}).Decode(&salesManager)
	if err != nil {
		return fmt.Errorf("failed to find sales manager: %w", err)
	}
	// Compose email
	subject := fmt.Sprintf("New %s Created Successfully", entityType)
	body := fmt.Sprintf("Dear %s,\n\nSalesperson %s has successfully created a new %s: %s.\nThe %s has been automatically approved and is now active in the system.\n\nBest regards,\nYour System", salesManager.FullName, salesperson.FullName, entityType, entityName, entityType)
	// Send email using gomail
	smtpHost := os.Getenv("SMTP_HOST")
	smtpUser := os.Getenv("SMTP_USER")
	smtpPass := os.Getenv("SMTP_PASS")
	smtpPort := 2525
	if portStr := os.Getenv("SMTP_PORT"); portStr != "" {
		fmt.Sscanf(portStr, "%d", &smtpPort)
	}
	m := gomail.NewMessage()
	m.SetHeader("From", smtpUser)
	m.SetHeader("To", salesManager.Email)
	m.SetHeader("Subject", subject)
	m.SetBody("text/plain", body)
	d := gomail.NewDialer(smtpHost, smtpPort, smtpUser, smtpPass)
	if err := d.DialAndSend(m); err != nil {
		log.Printf("Failed to send email to sales manager: %v", err)
	}
	// Save in-app notification
	notifTitle := fmt.Sprintf("New %s Created Successfully", entityType)
	notifMsg := fmt.Sprintf("Salesperson %s has successfully created a new %s: %s.", salesperson.FullName, entityType, entityName)
	_ = SaveNotification(db, salesManager.ID, notifTitle, notifMsg, "entity_created", map[string]interface{}{
		"entityType":    entityType,
		"entityName":    entityName,
		"salesPersonId": salesPersonID.Hex(),
	})
	return nil
}

// SendFCMNotificationToServiceProvider sends a Firebase Cloud Messaging notification to a service provider
func SendFCMNotificationToServiceProvider(db *mongo.Client, serviceProviderID primitive.ObjectID, title, message string, data map[string]interface{}) error {
	// Get service provider's FCM token from database
	collection := db.Database("barrim").Collection("serviceProviders")
	var serviceProvider models.ServiceProvider
	err := collection.FindOne(context.Background(), bson.M{"_id": serviceProviderID}).Decode(&serviceProvider)
	if err != nil {
		return fmt.Errorf("failed to find service provider: %w", err)
	}

	if serviceProvider.FCMToken == "" {
		log.Printf("Service provider %s has no FCM token", serviceProviderID.Hex())
		return fmt.Errorf("service provider has no FCM token")
	}

	// Check if Firebase app is initialized
	if config.FirebaseApp == nil {
		log.Printf("Firebase app is not initialized")
		return fmt.Errorf("firebase app not initialized")
	}

	// Get Firebase messaging client
	ctx := context.Background()
	client, err := config.FirebaseApp.Messaging(ctx)
	if err != nil {
		log.Printf("Error getting messaging client: %v", err)
		return fmt.Errorf("failed to initialize messaging client: %w", err)
	}

	// Prepare notification data with default values
	notificationData := map[string]string{
		"type":              "booking_request",
		"bookingId":         "",
		"serviceProviderId": serviceProviderID.Hex(),
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
				notificationData[key] = str
			} else {
				notificationData[key] = ""
			}
		}
	}

	// Prepare FCM message
	fcmMessage := &messaging.Message{
		Token: serviceProvider.FCMToken,
		Notification: &messaging.Notification{
			Title: title,
			Body:  message,
		},
		Data: notificationData,
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
						Title: title,
						Body:  message,
					},
					Sound:    "default",
					Badge:    func() *int { v := 1; return &v }(),
					Category: "BOOKING_REQUEST",
				},
			},
		},
	}

	// Send FCM notification
	response, err := client.Send(ctx, fcmMessage)
	if err != nil {
		log.Printf("Error sending FCM notification: %v", err)
		return fmt.Errorf("failed to send FCM notification: %w", err)
	}

	log.Printf("FCM notification sent successfully to service provider %s: %s", serviceProviderID.Hex(), response)
	return nil
}

// SendFCMNotificationToUser sends a Firebase Cloud Messaging notification to a user
func SendFCMNotificationToUser(db *mongo.Client, userID primitive.ObjectID, title, message string, data map[string]interface{}) error {
	// Get user's FCM token from database
	collection := db.Database("barrim").Collection("users")
	var user models.User
	err := collection.FindOne(context.Background(), bson.M{"_id": userID}).Decode(&user)
	if err != nil {
		return fmt.Errorf("failed to find user: %w", err)
	}

	if user.FCMToken == "" {
		log.Printf("User %s has no FCM token", userID.Hex())
		return fmt.Errorf("user has no FCM token")
	}

	if config.FirebaseApp == nil {
		log.Printf("Firebase app is not initialized")
		return fmt.Errorf("firebase app not initialized")
	}

	ctx := context.Background()
	client, err := config.FirebaseApp.Messaging(ctx)
	if err != nil {
		log.Printf("Error getting messaging client: %v", err)
		return fmt.Errorf("failed to initialize messaging client: %w", err)
	}

	notificationData := map[string]string{
		"type":      "review_reply",
		"timestamp": time.Now().Format(time.RFC3339),
	}
	// Override/merge with provided data
	if data != nil {
		for key, value := range data {
			if str, ok := value.(string); ok {
				notificationData[key] = str
			} else {
				notificationData[key] = ""
			}
		}
	}

	fcmMessage := &messaging.Message{
		Token: user.FCMToken,
		Notification: &messaging.Notification{
			Title: title,
			Body:  message,
		},
		Data: notificationData,
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
						Title: title,
						Body:  message,
					},
					Sound:    "default",
					Badge:    func() *int { v := 1; return &v }(),
					Category: "REVIEW_REPLY",
				},
			},
		},
	}

	response, err := client.Send(ctx, fcmMessage)
	if err != nil {
		log.Printf("Error sending FCM notification to user: %v", err)
		return fmt.Errorf("failed to send FCM notification: %w", err)
	}

	log.Printf("FCM notification sent successfully to user %s: %s", userID.Hex(), response)
	return nil
}
