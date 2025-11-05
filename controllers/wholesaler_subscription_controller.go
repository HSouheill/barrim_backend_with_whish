package controllers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"io"
	"mime/multipart"
	"path/filepath"
	"strconv"

	"github.com/HSouheill/barrim_backend/middleware"
	"github.com/HSouheill/barrim_backend/models"
	"github.com/HSouheill/barrim_backend/services"
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/gomail.v2"
)

// WholesalerSubscriptionController handles subscription-related operations for wholesalers
type WholesalerSubscriptionController struct {
	DB *mongo.Database
}

// NewWholesalerSubscriptionController creates a new wholesaler subscription controller
func NewWholesalerSubscriptionController(db *mongo.Database) *WholesalerSubscriptionController {
	return &WholesalerSubscriptionController{DB: db}
}

// GetAvailablePlans retrieves all available subscription plans for wholesalers
func (wsc *WholesalerSubscriptionController) GetAvailablePlans(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user information from token
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "wholesaler" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only wholesalers can access this endpoint",
		})
	}

	collection := wsc.DB.Collection("subscription_plans")
	cursor, err := collection.Find(ctx, bson.M{
		"type":     "wholesaler",
		"isActive": true,
	})
	if err != nil {
		log.Printf("Error finding subscription plans: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve subscription plans",
		})
	}
	defer cursor.Close(ctx)

	var plans []models.SubscriptionPlan
	if err = cursor.All(ctx, &plans); err != nil {
		log.Printf("Error decoding subscription plans: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve subscription plans",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Subscription plans retrieved successfully",
		Data:    plans,
	})
}

func (sc *SubscriptionController) CreateWholesalerSubscription(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user information from token
	claims := middleware.GetUserFromToken(c)
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	// Parse multipart form
	if err := c.Request().ParseMultipartForm(10 << 20); err != nil { // 10 MB max
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Failed to parse form data",
		})
	}

	// Get plan ID from form
	planID := c.FormValue("planId")
	if planID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Plan ID is required",
		})
	}

	// Convert plan ID to ObjectID
	planObjectID, err := primitive.ObjectIDFromHex(planID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid plan ID format",
		})
	}

	// Handle image upload (optional for wholesalers)
	var imagePath string
	if file, err := c.FormFile("image"); err == nil {
		// Validate file type
		contentType := file.Header.Get("Content-Type")
		if !strings.HasPrefix(contentType, "image/") {
			return c.JSON(http.StatusBadRequest, models.Response{
				Status:  http.StatusBadRequest,
				Message: "Only image files are allowed",
			})
		}

		// Save the uploaded file
		imagePath, err = sc.saveUploadedFile(file, "uploads/wholesaler_plans")
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to save uploaded image",
			})
		}
	}

	// Find wholesaler by user ID
	wholesalerCollection := sc.DB.Collection("wholesalers")
	var wholesaler models.Wholesaler
	err = wholesalerCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&wholesaler)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Wholesaler not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find wholesaler",
		})
	}

	// Check if there's already a pending subscription request
	wholesalerSubscriptionRequestsCollection := sc.DB.Collection("wholesaler_subscription_requests")
	var existingRequest models.WholesalerSubscriptionRequest
	err = wholesalerSubscriptionRequestsCollection.FindOne(ctx, bson.M{
		"wholesalerId": wholesaler.ID,
		"status":       "pending",
	}).Decode(&existingRequest)

	if err == nil {
		return c.JSON(http.StatusConflict, models.Response{
			Status:  http.StatusConflict,
			Message: "You already have a pending subscription request",
		})
	} else if err != mongo.ErrNoDocuments {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Error checking existing subscription requests",
		})
	}

	// Create wholesaler subscription request with dual approval fields
	subscriptionRequest := models.WholesalerSubscriptionRequest{
		ID:              primitive.NewObjectID(),
		WholesalerID:    wholesaler.ID,
		PlanID:          planObjectID,
		Status:          "pending",
		AdminApproved:   false,
		ManagerApproved: false,
		RequestedAt:     time.Now(),
	}

	// Save subscription request
	_, err = wholesalerSubscriptionRequestsCollection.InsertOne(ctx, subscriptionRequest)
	if err != nil {
		// If saving to database fails, delete the uploaded image
		if imagePath != "" {
			os.Remove(imagePath)
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to create subscription request",
		})
	}

	// Send notification to admin
	adminEmail := os.Getenv("ADMIN_EMAIL")
	if adminEmail != "" {
		subject := "New Wholesaler Subscription Request"
		body := fmt.Sprintf("A new subscription request has been submitted by wholesaler: %s\nPlan ID: %s\nRequested At: %s\n", wholesaler.BusinessName, planID, subscriptionRequest.RequestedAt.Format("2006-01-02 15:04:05"))
		if err := sc.sendNotificationEmail(adminEmail, subject, body); err != nil {
			log.Printf("Failed to send admin notification email: %v", err)
		}
	}

	// Optionally, send to manager as well if you have a manager email
	managerEmail := os.Getenv("MANAGER_EMAIL")
	if managerEmail != "" {
		subject := "New Wholesaler Subscription Request (Manager Notification)"
		body := fmt.Sprintf("A new subscription request has been submitted by wholesaler: %s\nPlan ID: %s\nRequested At: %s\n", wholesaler.BusinessName, planID, subscriptionRequest.RequestedAt.Format("2006-01-02 15:04:05"))
		if err := sc.sendNotificationEmail(managerEmail, subject, body); err != nil {
			log.Printf("Failed to send manager notification email: %v", err)
		}
	}

	// Send email notification to admin
	if err := sc.sendAdminNotificationEmail(
		"New Wholesaler Subscription Request",
		fmt.Sprintf("Wholesaler %s has requested a subscription. Please review the request.", wholesaler.BusinessName),
	); err != nil {
		log.Printf("Failed to send admin notification email: %v", err)
	}

	return c.JSON(http.StatusCreated, models.Response{
		Status:  http.StatusCreated,
		Message: "Subscription request created successfully. Waiting for admin and manager approval.",
		Data:    subscriptionRequest,
	})
}

// GetWholesalerSubscriptionPlans retrieves all available subscription plans for wholesalers
func (sc *SubscriptionController) GetWholesalerSubscriptionPlans(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	collection := sc.DB.Collection("subscription_plans")
	cursor, err := collection.Find(ctx, bson.M{
		"type":     "wholesaler",
		"isActive": true,
	})
	if err != nil {
		log.Printf("Error finding wholesaler subscription plans: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve subscription plans",
		})
	}
	defer cursor.Close(ctx)

	var plans []models.SubscriptionPlan
	if err = cursor.All(ctx, &plans); err != nil {
		log.Printf("Error decoding wholesaler subscription plans: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve subscription plans",
		})
	}

	// Sort plans by price in ascending order
	sort.Slice(plans, func(i, j int) bool {
		return plans[i].Price < plans[j].Price
	})

	if len(plans) == 0 {
		return c.JSON(http.StatusOK, models.Response{
			Status:  http.StatusOK,
			Message: "No subscription plans found for wholesalers",
			Data:    []models.SubscriptionPlan{},
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Wholesaler subscription plans retrieved successfully",
		Data:    plans,
	})
}

// GetCurrentWholesalerSubscription retrieves the current active subscription for a wholesaler
func (sc *SubscriptionController) GetCurrentWholesalerSubscription(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user information from token
	claims := middleware.GetUserFromToken(c)
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	// Find wholesaler by user ID
	wholesalerCollection := sc.DB.Collection("wholesalers")
	var wholesaler models.Wholesaler
	err = wholesalerCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&wholesaler)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Wholesaler not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find wholesaler",
		})
	}

	// Find active subscription
	subscriptionsCollection := sc.DB.Collection("wholesaler_subscriptions")
	var subscription models.WholesalerSubscription
	err = subscriptionsCollection.FindOne(ctx, bson.M{
		"wholesalerId": wholesaler.ID,
		"status":       "active",
		"endDate":      bson.M{"$gt": time.Now()},
	}).Decode(&subscription)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusOK, models.Response{
				Status:  http.StatusOK,
				Message: "No active subscription found",
				Data:    nil,
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find subscription",
		})
	}

	// Get plan details
	var plan models.SubscriptionPlan
	err = sc.DB.Collection("subscription_plans").FindOne(ctx, bson.M{"_id": subscription.PlanID}).Decode(&plan)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to get plan details",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Current wholesaler subscription retrieved successfully",
		Data: map[string]interface{}{
			"subscription": subscription,
			"plan":         plan,
		},
	})
}

// CancelWholesalerSubscription cancels an active subscription for a wholesaler
func (sc *SubscriptionController) CancelWholesalerSubscription(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user information from token
	claims := middleware.GetUserFromToken(c)
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	// Find wholesaler by user ID
	wholesalerCollection := sc.DB.Collection("wholesalers")
	var wholesaler models.Wholesaler
	err = wholesalerCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&wholesaler)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Wholesaler not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find wholesaler",
		})
	}

	// Find and update active subscription
	subscriptionsCollection := sc.DB.Collection("wholesaler_subscriptions")
	update := bson.M{
		"$set": bson.M{
			"status":    "cancelled",
			"autoRenew": false,
			"updatedAt": time.Now(),
		},
	}

	result, err := subscriptionsCollection.UpdateOne(ctx, bson.M{
		"wholesalerId": wholesaler.ID,
		"status":       "active",
	}, update)

	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to cancel subscription",
		})
	}

	if result.MatchedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "No active subscription found",
		})
	}

	// Send notification email to admin
	if err := sc.sendAdminNotificationEmail(
		"Wholesaler Subscription Cancelled",
		fmt.Sprintf("Wholesaler %s has cancelled their subscription.", wholesaler.BusinessName),
	); err != nil {
		log.Printf("Failed to send admin notification email: %v", err)
	}

	// Send notification email to wholesaler
	if err := sc.sendCompanyNotificationEmail(
		wholesaler.ContactInfo.Phone,
		"Subscription Cancelled",
		"Your subscription has been cancelled successfully. You can subscribe again at any time.",
	); err != nil {
		log.Printf("Failed to send wholesaler notification email: %v", err)
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Subscription cancelled successfully",
	})
}

// Generic email notification helper
func (sc *SubscriptionController) sendwholesalerNotificationEmail(to, subject, body string) error {
	smtpHost := os.Getenv("SMTP_HOST")
	smtpUser := os.Getenv("SMTP_USER")
	smtpPass := os.Getenv("SMTP_PASS")
	fromEmail := os.Getenv("FROM_EMAIL")

	senderEmail := fromEmail
	if senderEmail == "" {
		senderEmail = smtpUser
	}

	if smtpHost == "" || smtpUser == "" || smtpPass == "" || senderEmail == "" {
		log.Println("SMTP configuration is incomplete for notifications")
		return fmt.Errorf("SMTP configuration is incomplete: check SMTP_HOST, SMTP_USER, SMTP_PASS, and FROM_EMAIL")
	}

	smtpPortStr := os.Getenv("SMTP_PORT")
	smtpPort := 2525 // Default port
	if smtpPortStr != "" {
		if portNum, err := strconv.Atoi(smtpPortStr); err == nil && portNum > 0 {
			smtpPort = portNum
		}
	}

	m := gomail.NewMessage()
	m.SetHeader("From", senderEmail)
	m.SetHeader("To", to)
	m.SetHeader("Subject", subject)
	m.SetBody("text/plain", body)

	d := gomail.NewDialer(smtpHost, smtpPort, smtpUser, smtpPass)
	return d.DialAndSend(m)
}

// Admin approval handler
func (sc *SubscriptionController) ApproveWholesalerSubscriptionRequest(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	requestID := c.Param("id")
	if requestID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Missing subscription request ID",
		})
	}

	objID, err := primitive.ObjectIDFromHex(requestID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid subscription request ID",
		})
	}

	wholesalerSubscriptionRequestsCollection := sc.DB.Collection("wholesaler_subscription_requests")
	var request models.WholesalerSubscriptionRequest
	err = wholesalerSubscriptionRequestsCollection.FindOne(ctx, bson.M{"_id": objID}).Decode(&request)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Subscription request not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find subscription request",
		})
	}

	// Update approval fields
	now := time.Now()
	approved := true
	update := bson.M{
		"$set": bson.M{
			"status":        "approved",
			"adminApproved": &approved,
			"approvedBy":    "admin",
			"approvedAt":    now,
			"processedAt":   now,
		},
	}
	_, err = wholesalerSubscriptionRequestsCollection.UpdateOne(ctx, bson.M{"_id": objID}, update)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update subscription request",
		})
	}

	// Set wholesaler status to approved
	if !request.WholesalerID.IsZero() {
		wholesalerCollection := sc.DB.Collection("wholesalers")
		_, err = wholesalerCollection.UpdateOne(ctx, bson.M{"_id": request.WholesalerID}, bson.M{"$set": bson.M{"status": "active"}})
		if err != nil {
			log.Printf("Failed to update wholesaler status: %v", err)
		}
	}

	// --- Commission logic start ---
	wholesalerCollection := sc.DB.Collection("wholesalers")
	// Get wholesaler details
	var wholesaler models.Wholesaler
	if err := wholesalerCollection.FindOne(ctx, bson.M{"_id": request.WholesalerID}).Decode(&wholesaler); err == nil {
		if !wholesaler.CreatedBy.IsZero() {
			// Get salesperson
			var salesperson models.Salesperson
			err := sc.DB.Collection("salespersons").FindOne(ctx, bson.M{"_id": wholesaler.CreatedBy}).Decode(&salesperson)
			if err == nil {
				// Get admin who created the salesperson
				var admin models.Admin
				err := sc.DB.Collection("admins").FindOne(ctx, bson.M{"_id": salesperson.CreatedBy}).Decode(&admin)
				if err == nil {
					// Get plan details
					var plan models.SubscriptionPlan
					err := sc.DB.Collection("subscription_plans").FindOne(ctx, bson.M{"_id": request.PlanID}).Decode(&plan)
					if err == nil {
						planPrice := plan.Price
						salespersonPercent := salesperson.CommissionPercent

						// Calculate admin commission (remaining percentage after salesperson commission)
						adminPercent := 100.0 - salespersonPercent

						// Calculate commissions correctly
						// Salesperson gets their percentage directly from the plan price
						salespersonCommission := planPrice * salespersonPercent / 100.0
						// Admin gets the remaining percentage
						adminCommission := planPrice * adminPercent / 100.0

						// Insert commission documents
						commissionCollection := sc.DB.Collection("commissions")
						commissionDocs := []interface{}{
							models.Commission{
								ID:                           primitive.NewObjectID(),
								SubscriptionID:               request.ID,
								CompanyID:                    primitive.NilObjectID,
								PlanID:                       plan.ID,
								PlanPrice:                    planPrice,
								AdminID:                      admin.ID,
								AdminCommission:              adminCommission,
								AdminCommissionPercent:       adminPercent,
								SalespersonID:                salesperson.ID,
								SalespersonCommission:        salespersonCommission,
								SalespersonCommissionPercent: salespersonPercent,
								CreatedAt:                    time.Now(),
								Paid:                         false,
								PaidAt:                       nil,
							},
						}
						_, err := commissionCollection.InsertMany(ctx, commissionDocs)
						if err != nil {
							log.Printf("Failed to insert commission docs: %v", err)
						} else {
							log.Printf("Commission inserted successfully - Plan Price: $%.2f, Admin Commission: $%.2f (%.1f%%), Salesperson Commission: $%.2f (%.1f%%)\n",
								planPrice, adminCommission, adminPercent, salespersonCommission, salespersonPercent)
						}
					}
				}
			}
		}
	}
	// --- Commission logic end ---

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Subscription request approved and wholesaler status updated.",
	})
}

// Admin reject handler
func (sc *SubscriptionController) RejectWholesalerSubscriptionRequest(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	requestID := c.Param("id")
	if requestID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Missing subscription request ID",
		})
	}

	objID, err := primitive.ObjectIDFromHex(requestID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid subscription request ID",
		})
	}

	wholesalerSubscriptionRequestsCollection := sc.DB.Collection("wholesaler_subscription_requests")
	var request models.WholesalerSubscriptionRequest
	err = wholesalerSubscriptionRequestsCollection.FindOne(ctx, bson.M{"_id": objID}).Decode(&request)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Subscription request not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find subscription request",
		})
	}

	now := time.Now()
	rejected := false
	update := bson.M{
		"$set": bson.M{
			"status":        "rejected",
			"adminApproved": &rejected,
			"rejectedBy":    "admin",
			"rejectedAt":    now,
			"processedAt":   now,
		},
	}
	_, err = wholesalerSubscriptionRequestsCollection.UpdateOne(ctx, bson.M{"_id": objID}, update)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update subscription request",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Subscription request rejected.",
	})
}

// Manager approval handler
func (sc *SubscriptionController) ApproveWholesalerSubscriptionRequestByManager(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	requestID := c.Param("id")
	if requestID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Missing subscription request ID",
		})
	}

	objID, err := primitive.ObjectIDFromHex(requestID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid subscription request ID",
		})
	}

	wholesalerSubscriptionRequestsCollection := sc.DB.Collection("wholesaler_subscription_requests")
	var request models.WholesalerSubscriptionRequest
	err = wholesalerSubscriptionRequestsCollection.FindOne(ctx, bson.M{"_id": objID}).Decode(&request)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Subscription request not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find subscription request",
		})
	}

	// Update approval fields
	now := time.Now()
	approved := true
	update := bson.M{
		"$set": bson.M{
			"status":          "approved",
			"managerApproved": &approved,
			"approvedBy":      "manager",
			"approvedAt":      now,
			"processedAt":     now,
		},
	}
	_, err = wholesalerSubscriptionRequestsCollection.UpdateOne(ctx, bson.M{"_id": objID}, update)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update subscription request",
		})
	}

	// Set wholesaler status to approved
	if !request.WholesalerID.IsZero() {
		wholesalerCollection := sc.DB.Collection("wholesalers")
		_, err = wholesalerCollection.UpdateOne(ctx, bson.M{"_id": request.WholesalerID}, bson.M{"$set": bson.M{"status": "active"}})
		if err != nil {
			log.Printf("Failed to update wholesaler status: %v", err)
		}
	}

	// --- Commission logic start ---
	wholesalerCollection := sc.DB.Collection("wholesalers")
	// Get wholesaler details
	var wholesaler models.Wholesaler
	if err := wholesalerCollection.FindOne(ctx, bson.M{"_id": request.WholesalerID}).Decode(&wholesaler); err == nil {
		if !wholesaler.CreatedBy.IsZero() {
			// Get salesperson
			var salesperson models.Salesperson
			err := sc.DB.Collection("salespersons").FindOne(ctx, bson.M{"_id": wholesaler.CreatedBy}).Decode(&salesperson)
			if err == nil {
				// Get admin who created the salesperson
				var admin models.Admin
				err := sc.DB.Collection("admins").FindOne(ctx, bson.M{"_id": salesperson.CreatedBy}).Decode(&admin)
				if err == nil {
					// Get plan details
					var plan models.SubscriptionPlan
					err := sc.DB.Collection("subscription_plans").FindOne(ctx, bson.M{"_id": request.PlanID}).Decode(&plan)
					if err == nil {
						planPrice := plan.Price
						salespersonPercent := salesperson.CommissionPercent

						// Calculate admin commission (remaining percentage after salesperson commission)
						adminPercent := 100.0 - salespersonPercent

						// Calculate commissions correctly
						// Salesperson gets their percentage directly from the plan price
						salespersonCommission := planPrice * salespersonPercent / 100.0
						// Admin gets the remaining percentage
						adminCommission := planPrice * adminPercent / 100.0

						// Insert commission documents
						commissionCollection := sc.DB.Collection("commissions")
						commissionDocs := []interface{}{
							models.Commission{
								ID:                           primitive.NewObjectID(),
								SubscriptionID:               request.ID,
								CompanyID:                    primitive.NilObjectID,
								PlanID:                       plan.ID,
								PlanPrice:                    planPrice,
								AdminID:                      admin.ID,
								AdminCommission:              adminCommission,
								AdminCommissionPercent:       adminPercent,
								SalespersonID:                salesperson.ID,
								SalespersonCommission:        salespersonCommission,
								SalespersonCommissionPercent: salespersonPercent,
								CreatedAt:                    time.Now(),
								Paid:                         false,
								PaidAt:                       nil,
							},
						}
						_, err := commissionCollection.InsertMany(ctx, commissionDocs)
						if err != nil {
							log.Printf("Failed to insert commission docs: %v", err)
						} else {
							log.Printf("Commission inserted successfully - Plan Price: $%.2f, Admin Commission: $%.2f (%.1f%%), Salesperson Commission: $%.2f (%.1f%%)\n",
								planPrice, adminCommission, adminPercent, salespersonCommission, salespersonPercent)
						}
					}
				}
			}
		}
	}
	// --- Commission logic end ---

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Subscription request approved by manager and wholesaler status updated.",
	})
}

// Manager reject handler
func (sc *SubscriptionController) RejectWholesalerSubscriptionRequestByManager(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	requestID := c.Param("id")
	if requestID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Missing subscription request ID",
		})
	}

	objID, err := primitive.ObjectIDFromHex(requestID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid subscription request ID",
		})
	}

	wholesalerSubscriptionRequestsCollection := sc.DB.Collection("wholesaler_subscription_requests")
	var request models.WholesalerSubscriptionRequest
	err = wholesalerSubscriptionRequestsCollection.FindOne(ctx, bson.M{"_id": objID}).Decode(&request)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Subscription request not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find subscription request",
		})
	}

	now := time.Now()
	rejected := false
	update := bson.M{
		"$set": bson.M{
			"status":          "rejected",
			"managerApproved": &rejected,
			"rejectedBy":      "manager",
			"rejectedAt":      now,
			"processedAt":     now,
		},
	}
	_, err = wholesalerSubscriptionRequestsCollection.UpdateOne(ctx, bson.M{"_id": objID}, update)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update subscription request",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Subscription request rejected by manager.",
	})
}

// WholesalerBranchSubscriptionController handles branch-level subscription operations for wholesalers
type WholesalerBranchSubscriptionController struct {
	DB *mongo.Database
}

// NewWholesalerBranchSubscriptionController creates a new controller
func NewWholesalerBranchSubscriptionController(db *mongo.Database) *WholesalerBranchSubscriptionController {
	return &WholesalerBranchSubscriptionController{DB: db}
}

// CreateBranchSubscriptionRequest creates a new subscription request for a wholesaler branch
func (sc *WholesalerBranchSubscriptionController) CreateBranchSubscriptionRequest(c echo.Context) error {
	log.Printf("DEBUG: CreateBranchSubscriptionRequest called for wholesaler with path: %s", c.Request().URL.Path)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	claims := middleware.GetUserFromToken(c)
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	if err := c.Request().ParseMultipartForm(10 << 20); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Failed to parse form data",
		})
	}

	// Get plan ID from form and branch ID from URL parameter
	planID := c.FormValue("planId")
	branchID := c.Param("branchId")
	if planID == "" || branchID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Plan ID and Branch ID are required",
		})
	}

	planObjectID, err := primitive.ObjectIDFromHex(planID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid plan ID format",
		})
	}
	branchObjectID, err := primitive.ObjectIDFromHex(branchID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid branch ID format",
		})
	}

	// Verify the plan exists and is active
	var plan models.SubscriptionPlan
	err = sc.DB.Collection("subscription_plans").FindOne(ctx, bson.M{
		"_id":      planObjectID,
		"isActive": true,
	}).Decode(&plan)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Subscription plan not found or inactive",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to verify subscription plan",
		})
	}

	// Find wholesaler by user ID and get the specific branch
	wholesalerCollection := sc.DB.Collection("wholesalers")
	var wholesaler models.Wholesaler
	err = wholesalerCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&wholesaler)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Wholesaler not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find wholesaler",
		})
	}

	// Find the specific branch within the wholesaler
	var branch models.Branch
	branchFound := false
	for _, b := range wholesaler.Branches {
		if b.ID == branchObjectID {
			branch = b
			branchFound = true
			break
		}
	}

	if !branchFound {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Branch not found or you do not have access.",
		})
	}

	// Check if there's already a pending subscription request for this branch
	subscriptionRequestsCollection := sc.DB.Collection("wholesaler_branch_subscription_requests")
	var existingRequest models.WholesalerBranchSubscriptionRequest
	err = subscriptionRequestsCollection.FindOne(ctx, bson.M{
		"branchId": branch.ID,
		"status":   bson.M{"$in": []string{"pending", "pending_payment"}},
	}).Decode(&existingRequest)
	if err == nil {
		return c.JSON(http.StatusConflict, models.Response{
			Status:  http.StatusConflict,
			Message: "You already have a pending subscription request for this branch. Please complete the payment first.",
		})
	} else if err != mongo.ErrNoDocuments {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Error checking existing subscription requests",
		})
	}

	// Check if branch already has an active subscription
	subscriptionsCollection := sc.DB.Collection("wholesaler_branch_subscriptions")
	var activeSubscription models.WholesalerBranchSubscription
	err = subscriptionsCollection.FindOne(ctx, bson.M{
		"branchId": branch.ID,
		"status":   "active",
		"endDate":  bson.M{"$gt": time.Now()},
	}).Decode(&activeSubscription)
	if err == nil {
		return c.JSON(http.StatusConflict, models.Response{
			Status:  http.StatusConflict,
			Message: "This branch already has an active subscription. Please wait for it to expire or cancel it first.",
		})
	} else if err != mongo.ErrNoDocuments {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Error checking active subscriptions",
		})
	}

	// Create wholesaler branch subscription request
	subscriptionRequest := models.WholesalerBranchSubscriptionRequest{
		ID:          primitive.NewObjectID(),
		BranchID:    branch.ID,
		PlanID:      planObjectID,
		RequestedAt: time.Now(),
	}

	// Generate externalId from ObjectID (use timestamp part as int64)
	externalID := int64(subscriptionRequest.ID.Timestamp().Unix())
	subscriptionRequest.ExternalID = externalID

	// Create subscription request with pending_payment status
	subscriptionRequest.Status = "pending_payment"
	subscriptionRequest.PaymentStatus = "pending"

	// Get base URL for callbacks
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "https://barrim.online" // Default fallback
	}

	// Get app URL for redirects
	appURL := os.Getenv("APP_URL")
	if appURL == "" {
		appURL = baseURL // Fallback to baseURL if APP_URL not set
	}

	// Initialize Whish service
	whishService := services.NewWhishService()

	// Check Whish merchant account balance to verify account is active
	whishBalance, err := whishService.GetBalance()
	if err != nil {
		log.Printf("Warning: Could not check Whish account balance: %v", err)
		// Continue anyway - balance check failure doesn't block payment creation
	} else {
		log.Printf("Whish merchant account balance: $%.2f", whishBalance)
		// Optional: Validate account is active (negative balance might indicate issues)
		if whishBalance < 0 {
			log.Printf("Warning: Whish account has negative balance: $%.2f", whishBalance)
		}
	}

	// Create Whish payment request
	whishReq := models.WhishRequest{
		Amount:             &plan.Price,
		Currency:           "USD", // Use USD for subscription payments
		Invoice:            fmt.Sprintf("Wholesaler Branch Subscription - %s - Plan: %s", branch.Name, plan.Title),
		ExternalID:         &externalID,
		SuccessCallbackURL: fmt.Sprintf("%s/api/whish/wholesaler-branch/payment/callback/success", baseURL),
		FailureCallbackURL: fmt.Sprintf("%s/api/whish/wholesaler-branch/payment/callback/failure", baseURL),
		SuccessRedirectURL: fmt.Sprintf("%s/payment-success?requestId=%s", appURL, subscriptionRequest.ID.Hex()),
		FailureRedirectURL: fmt.Sprintf("%s/payment-failed?requestId=%s", appURL, subscriptionRequest.ID.Hex()),
	}

	// Call Whish API to create payment
	collectURL, err := whishService.PostPayment(whishReq)
	if err != nil {
		log.Printf("Failed to create Whish payment: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: fmt.Sprintf("Failed to initiate payment: %v", err),
		})
	}

	// Save collectUrl to subscription request
	subscriptionRequest.CollectURL = collectURL

	// Save subscription request to database
	_, err = subscriptionRequestsCollection.InsertOne(ctx, subscriptionRequest)
	if err != nil {
		log.Printf("Failed to save subscription request: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to create subscription request",
		})
	}

	log.Printf("Whish payment created for wholesaler branch subscription request %s: %s", subscriptionRequest.ID.Hex(), collectURL)

	return c.JSON(http.StatusCreated, models.Response{
		Status:  http.StatusCreated,
		Message: "Payment initiated successfully. Please complete the payment to activate your subscription.",
		Data: map[string]interface{}{
			"requestId":     subscriptionRequest.ID,
			"plan":          plan,
			"status":        subscriptionRequest.Status,
			"submittedAt":   subscriptionRequest.RequestedAt,
			"paymentAmount": plan.Price,
			"collectUrl":    collectURL,
			"externalId":    externalID,
		},
	})
}

// HandleWhishPaymentSuccess handles Whish payment success callback for wholesaler branch subscriptions
func (sc *WholesalerBranchSubscriptionController) HandleWhishPaymentSuccess(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get externalId from query parameters (Whish sends it as GET parameter)
	externalIDStr := c.QueryParam("externalId")
	if externalIDStr == "" {
		log.Printf("Missing externalId in Whish success callback")
		return c.String(http.StatusBadRequest, "Missing externalId parameter")
	}

	externalID, err := strconv.ParseInt(externalIDStr, 10, 64)
	if err != nil {
		log.Printf("Invalid externalId in callback: %v", err)
		return c.String(http.StatusBadRequest, "Invalid externalId")
	}

	// Find the subscription request by externalId
	subscriptionRequestsCollection := sc.DB.Collection("wholesaler_branch_subscription_requests")
	var subscriptionRequest models.WholesalerBranchSubscriptionRequest
	err = subscriptionRequestsCollection.FindOne(ctx, bson.M{"externalId": externalID}).Decode(&subscriptionRequest)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			log.Printf("Subscription request not found for externalId: %d", externalID)
			return c.String(http.StatusNotFound, "Subscription request not found")
		}
		log.Printf("Error finding subscription request: %v", err)
		return c.String(http.StatusInternalServerError, "Database error")
	}

	// Check if already processed
	if subscriptionRequest.PaymentStatus == "success" || subscriptionRequest.Status == "active" {
		log.Printf("Payment already processed for request: %s", subscriptionRequest.ID.Hex())
		return c.String(http.StatusOK, "Payment already processed")
	}

	// Initialize Whish service and verify payment status
	whishService := services.NewWhishService()
	status, phoneNumber, err := whishService.GetPaymentStatus("USD", externalID)
	if err != nil {
		log.Printf("Failed to verify payment status: %v", err)
		return c.String(http.StatusInternalServerError, "Failed to verify payment")
	}

	if status != "success" {
		log.Printf("Payment not successful, status: %s", status)
		// Update request status to failed
		subscriptionRequestsCollection.UpdateOne(ctx,
			bson.M{"_id": subscriptionRequest.ID},
			bson.M{"$set": bson.M{
				"paymentStatus": "failed",
				"status":        "failed",
				"processedAt":   time.Now(),
			}})
		return c.String(http.StatusBadRequest, "Payment not successful")
	}

	// Payment verified successfully - proceed to activate subscription
	err = sc.activateWholesalerBranchSubscription(ctx, subscriptionRequest, phoneNumber)
	if err != nil {
		log.Printf("Failed to activate subscription: %v", err)
		return c.String(http.StatusInternalServerError, "Failed to activate subscription")
	}

	log.Printf("Wholesaler branch subscription activated successfully for externalId: %d", externalID)
	return c.String(http.StatusOK, "Payment successful and subscription activated")
}

// HandleWhishPaymentFailure handles Whish payment failure callback for wholesaler branch subscriptions
func (sc *WholesalerBranchSubscriptionController) HandleWhishPaymentFailure(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	externalIDStr := c.QueryParam("externalId")
	if externalIDStr == "" {
		return c.String(http.StatusBadRequest, "Missing externalId parameter")
	}

	externalID, err := strconv.ParseInt(externalIDStr, 10, 64)
	if err != nil {
		return c.String(http.StatusBadRequest, "Invalid externalId")
	}

	// Update subscription request status to failed
	subscriptionRequestsCollection := sc.DB.Collection("wholesaler_branch_subscription_requests")
	_, err = subscriptionRequestsCollection.UpdateOne(ctx,
		bson.M{"externalId": externalID},
		bson.M{"$set": bson.M{
			"paymentStatus": "failed",
			"status":        "failed",
			"processedAt":   time.Now(),
		}})

	if err != nil {
		log.Printf("Failed to update subscription request on payment failure: %v", err)
		return c.String(http.StatusInternalServerError, "Failed to update status")
	}

	log.Printf("Payment failed for externalId: %d", externalID)
	return c.String(http.StatusOK, "Payment failure recorded")
}

// activateWholesalerBranchSubscription activates the subscription after successful payment
func (sc *WholesalerBranchSubscriptionController) activateWholesalerBranchSubscription(ctx context.Context, subscriptionRequest models.WholesalerBranchSubscriptionRequest, payerPhone string) error {
	// Get plan details
	var plan models.SubscriptionPlan
	err := sc.DB.Collection("subscription_plans").FindOne(ctx, bson.M{"_id": subscriptionRequest.PlanID}).Decode(&plan)
	if err != nil {
		log.Printf("Failed to get plan: %v", err)
		return fmt.Errorf("failed to get plan details")
	}

	// Get wholesaler and branch details using aggregation
	var wholesaler models.Wholesaler
	pipeline := []bson.M{
		{"$match": bson.M{"branches._id": subscriptionRequest.BranchID}},
		{"$limit": 1},
	}

	cursor, err := sc.DB.Collection("wholesalers").Aggregate(ctx, pipeline)
	if err != nil {
		log.Printf("Error aggregating wholesalers: %v", err)
		return fmt.Errorf("failed to get wholesaler details")
	}
	defer cursor.Close(ctx)

	var branch models.Branch
	branchFound := false
	if cursor.Next(ctx) {
		if err := cursor.Decode(&wholesaler); err != nil {
			log.Printf("Error decoding wholesaler: %v", err)
			return fmt.Errorf("failed to decode wholesaler details")
		}

		// Find the specific branch
		for _, b := range wholesaler.Branches {
			if b.ID == subscriptionRequest.BranchID {
				branch = b
				branchFound = true
				break
			}
		}
	}

	if !branchFound {
		return fmt.Errorf("branch not found")
	}

	// Calculate subscription dates
	startDate := time.Now()
	var endDate time.Time
	switch plan.Duration {
	case 1: // Monthly
		endDate = startDate.AddDate(0, 1, 0)
	case 3: // 3 Months
		endDate = startDate.AddDate(0, 3, 0)
	case 6: // 6 Months
		endDate = startDate.AddDate(0, 6, 0)
	case 12: // 1 Year
		endDate = startDate.AddDate(1, 0, 0)
	default:
		return fmt.Errorf("invalid plan duration")
	}

	// Create active subscription
	newSubscription := models.WholesalerBranchSubscription{
		ID:        primitive.NewObjectID(),
		BranchID:  subscriptionRequest.BranchID,
		PlanID:    subscriptionRequest.PlanID,
		StartDate: startDate,
		EndDate:   endDate,
		Status:    "active",
		AutoRenew: false,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Save subscription
	subscriptionsCollection := sc.DB.Collection("wholesaler_branch_subscriptions")
	_, err = subscriptionsCollection.InsertOne(ctx, newSubscription)
	if err != nil {
		log.Printf("Failed to create subscription: %v", err)
		return fmt.Errorf("failed to create subscription")
	}

	// Update branch status to active in the wholesaler's branches array
	wholesalerCollection := sc.DB.Collection("wholesalers")
	_, err = wholesalerCollection.UpdateOne(
		ctx,
		bson.M{
			"_id":          wholesaler.ID,
			"branches._id": subscriptionRequest.BranchID,
		},
		bson.M{
			"$set": bson.M{
				"branches.$.status":    "active",
				"branches.$.updatedAt": time.Now(),
			},
		},
	)
	if err != nil {
		log.Printf("Failed to update branch status: %v", err)
	}

	// Update user status to active when subscription is activated
	usersCollection := sc.DB.Collection("users")
	_, err = usersCollection.UpdateOne(
		ctx,
		bson.M{"_id": wholesaler.UserID},
		bson.M{"$set": bson.M{"status": "active", "updatedAt": time.Now()}},
	)
	if err != nil {
		log.Printf("Failed to update user status to active: %v", err)
	}

	// Handle commission and admin wallet (30% salesperson, 70% admin)
	planPrice := plan.Price
	if wholesaler.CreatedBy != wholesaler.UserID && !wholesaler.CreatedBy.IsZero() {
		// Wholesaler was created by a salesperson - split commission
		var salesperson models.Salesperson
		err := sc.DB.Collection("salespersons").FindOne(ctx, bson.M{"_id": wholesaler.CreatedBy}).Decode(&salesperson)
		if err == nil {
			// Calculate commissions: 30% salesperson, 70% admin
			salespersonPercent := 30.0
			adminPercent := 70.0

			salespersonCommission := planPrice * salespersonPercent / 100.0
			adminCommission := planPrice * adminPercent / 100.0

			// Add salesperson commission
			err = sc.addCommissionToSalespersonWallet(ctx, salesperson.ID, salespersonCommission, newSubscription.ID, "wholesaler_branch_subscription", wholesaler.BusinessName)
			if err != nil {
				log.Printf("Failed to add salesperson commission: %v", err)
			} else {
				log.Printf("Added salesperson commission: $%.2f (30%% of $%.2f)", salespersonCommission, planPrice)
			}

			// Add admin commission
			err = sc.addSubscriptionIncomeToAdminWallet(ctx, adminCommission, newSubscription.ID, "wholesaler_branch_subscription_commission", wholesaler.BusinessName, branch.Name)
			if err != nil {
				log.Printf("Failed to add admin commission: %v", err)
			} else {
				log.Printf("Added admin commission: $%.2f (70%% of $%.2f)", adminCommission, planPrice)
			}
		} else {
			log.Printf("Salesperson not found, adding full amount to admin wallet")
			// Salesperson not found, add full amount to admin
			err = sc.addSubscriptionIncomeToAdminWallet(ctx, planPrice, newSubscription.ID, "wholesaler_branch_subscription", wholesaler.BusinessName, branch.Name)
			if err != nil {
				log.Printf("Failed to add subscription income to admin wallet: %v", err)
			}
		}
	} else {
		// Wholesaler created by itself - full amount to admin
		err = sc.addSubscriptionIncomeToAdminWallet(ctx, planPrice, newSubscription.ID, "wholesaler_branch_subscription", wholesaler.BusinessName, branch.Name)
		if err != nil {
			log.Printf("Failed to add subscription income to admin wallet: %v", err)
		}
	}

	// Update subscription request status
	subscriptionRequestsCollection := sc.DB.Collection("wholesaler_branch_subscription_requests")
	_, err = subscriptionRequestsCollection.UpdateOne(ctx,
		bson.M{"_id": subscriptionRequest.ID},
		bson.M{"$set": bson.M{
			"paymentStatus": "success",
			"status":        "active",
			"paidAt":        time.Now(),
			"processedAt":   time.Now(),
		}})

	if err != nil {
		log.Printf("Failed to update subscription request status: %v", err)
	}

	log.Printf("Wholesaler branch subscription activated successfully: Branch=%s, Plan=%s, Amount=$%.2f", branch.Name, plan.Title, planPrice)
	return nil
}

// addCommissionToSalespersonWallet adds commission amount to salesperson's wallet
func (sc *WholesalerBranchSubscriptionController) addCommissionToSalespersonWallet(ctx context.Context, salespersonID primitive.ObjectID, amount float64, subscriptionID primitive.ObjectID, entityType, wholesalerName string) error {
	// CommissionRecord structure
	type CommissionRecord struct {
		ID             primitive.ObjectID `bson:"_id,omitempty"`
		SubscriptionID primitive.ObjectID `bson:"subscriptionId"`
		SalespersonID  primitive.ObjectID `bson:"salespersonId"`
		Amount         float64            `bson:"amount"`
		Role           string             `bson:"role"`
		Status         string             `bson:"status"`
		CreatedAt      time.Time          `bson:"createdAt"`
	}

	commissionRecord := CommissionRecord{
		ID:             primitive.NewObjectID(),
		SubscriptionID: subscriptionID,
		SalespersonID:  salespersonID,
		Amount:         amount,
		Role:           "salesperson",
		Status:         "pending",
		CreatedAt:      time.Now(),
	}

	_, err := sc.DB.Collection("commission_records").InsertOne(ctx, commissionRecord)
	if err != nil {
		return fmt.Errorf("failed to insert salesperson commission record: %w", err)
	}

	_, err = sc.DB.Collection("salespersons").UpdateOne(
		ctx,
		bson.M{"_id": salespersonID},
		bson.M{
			"$inc": bson.M{"commissionBalance": amount},
			"$set": bson.M{"updatedAt": time.Now()},
		},
	)
	if err != nil {
		return fmt.Errorf("failed to update salesperson commission balance: %w", err)
	}

	log.Printf("Commission $%.2f added to salesperson wallet (ID: %s) from %s - %s",
		amount, salespersonID.Hex(), entityType, wholesalerName)
	return nil
}

// saveUploadedFile saves an uploaded file to the specified directory
func (sc *WholesalerBranchSubscriptionController) saveUploadedFile(file *multipart.FileHeader, directory string) (string, error) {
	if err := os.MkdirAll(directory, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	ext := filepath.Ext(file.Filename)
	filename := primitive.NewObjectID().Hex() + ext
	filepath := filepath.Join(directory, filename)

	src, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("failed to open uploaded file: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(filepath)
	if err != nil {
		return "", fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dst.Close()

	if _, err = io.Copy(dst, src); err != nil {
		return "", fmt.Errorf("failed to copy file contents: %w", err)
	}

	return filepath, nil
}

// ApproveBranchSubscriptionRequest allows an admin or manager to approve a branch subscription request
func (sc *WholesalerBranchSubscriptionController) ApproveBranchSubscriptionRequest(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	requestID := c.Param("id")
	if requestID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Missing subscription request ID",
		})
	}
	objID, err := primitive.ObjectIDFromHex(requestID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid subscription request ID",
		})
	}

	subscriptionRequestsCollection := sc.DB.Collection("wholesaler_branch_subscription_requests")
	var request models.WholesalerBranchSubscriptionRequest
	err = subscriptionRequestsCollection.FindOne(ctx, bson.M{"_id": objID}).Decode(&request)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Subscription request not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find subscription request",
		})
	}

	// Update approval fields
	now := time.Now()
	approved := true
	update := bson.M{
		"$set": bson.M{
			"status":        "approved",
			"adminApproved": &approved,
			"approvedBy":    "admin",
			"approvedAt":    now,
			"processedAt":   now,
		},
	}
	_, err = subscriptionRequestsCollection.UpdateOne(ctx, bson.M{"_id": objID}, update)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update subscription request",
		})
	}

	// Create the branch subscription
	var plan models.SubscriptionPlan
	err = sc.DB.Collection("subscription_plans").FindOne(ctx, bson.M{"_id": request.PlanID}).Decode(&plan)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to get plan details",
		})
	}
	startDate := time.Now()
	var endDate time.Time
	switch plan.Duration {
	case 1:
		endDate = startDate.AddDate(0, 1, 0)
	case 6:
		endDate = startDate.AddDate(0, 6, 0)
	case 12:
		endDate = startDate.AddDate(1, 0, 0)
	default:
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Invalid plan duration",
		})
	}

	subscription := models.WholesalerBranchSubscription{
		ID:        primitive.NewObjectID(),
		BranchID:  request.BranchID,
		PlanID:    request.PlanID,
		StartDate: startDate,
		EndDate:   endDate,
		Status:    "active",
		AutoRenew: false,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	subscriptionsCollection := sc.DB.Collection("wholesaler_branch_subscriptions")
	_, err = subscriptionsCollection.InsertOne(ctx, subscription)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to create subscription",
		})
	}

	// After creating the branch subscription, set the branch status to active in the wholesaler's branches array
	if !request.BranchID.IsZero() {
		wholesalerCollection := sc.DB.Collection("wholesalers")
		_, err = wholesalerCollection.UpdateOne(
			ctx,
			bson.M{"branches._id": request.BranchID},
			bson.M{"$set": bson.M{"branches.$.status": "active"}},
		)
		if err != nil {
			log.Printf("Failed to update wholesaler branch status to active: %v", err)
		}
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Subscription request approved and branch subscription created.",
	})
}

// RejectBranchSubscriptionRequest allows an admin or manager to reject a branch subscription request
func (sc *WholesalerBranchSubscriptionController) RejectBranchSubscriptionRequest(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	requestID := c.Param("id")
	if requestID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Missing subscription request ID",
		})
	}
	objID, err := primitive.ObjectIDFromHex(requestID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid subscription request ID",
		})
	}

	subscriptionRequestsCollection := sc.DB.Collection("wholesaler_branch_subscription_requests")
	var request models.WholesalerBranchSubscriptionRequest
	err = subscriptionRequestsCollection.FindOne(ctx, bson.M{"_id": objID}).Decode(&request)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Subscription request not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find subscription request",
		})
	}

	now := time.Now()
	rejected := false
	update := bson.M{
		"$set": bson.M{
			"status":        "rejected",
			"adminApproved": &rejected,
			"rejectedBy":    "admin",
			"rejectedAt":    now,
			"processedAt":   now,
		},
	}
	_, err = subscriptionRequestsCollection.UpdateOne(ctx, bson.M{"_id": objID}, update)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update subscription request",
		})
	}

	// After rejecting the branch subscription, set the branch status to inactive in the wholesaler's branches array
	if !request.BranchID.IsZero() {
		wholesalerCollection := sc.DB.Collection("wholesalers")
		_, err = wholesalerCollection.UpdateOne(
			ctx,
			bson.M{"branches._id": request.BranchID},
			bson.M{"$set": bson.M{"branches.$.status": "inactive"}},
		)
		if err != nil {
			log.Printf("Failed to update wholesaler branch status to inactive: %v", err)
		}
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Subscription request rejected.",
	})
}

// GetBranchSubscriptionRequestStatus gets the status of a branch's subscription request
func (sc *WholesalerBranchSubscriptionController) GetBranchSubscriptionRequestStatus(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	branchID := c.Param("branchId")
	if branchID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Missing branch ID",
		})
	}
	branchObjectID, err := primitive.ObjectIDFromHex(branchID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid branch ID",
		})
	}

	subscriptionRequestsCollection := sc.DB.Collection("wholesaler_branch_subscription_requests")
	var subscriptionRequest models.WholesalerBranchSubscriptionRequest
	err = subscriptionRequestsCollection.FindOne(ctx, bson.M{"branchId": branchObjectID}, options.FindOne().SetSort(bson.D{{"requestedAt", -1}})).Decode(&subscriptionRequest)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusOK, models.Response{
				Status:  http.StatusOK,
				Message: "No subscription request found",
				Data: map[string]interface{}{
					"hasRequest": false,
				},
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find subscription request",
		})
	}

	var plan models.SubscriptionPlan
	err = sc.DB.Collection("subscription_plans").FindOne(ctx, bson.M{"_id": subscriptionRequest.PlanID}).Decode(&plan)
	if err != nil {
		log.Printf("Failed to get plan details: %v", err)
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Subscription request status retrieved successfully",
		Data: map[string]interface{}{
			"hasRequest": true,
			"request":    subscriptionRequest,
			"plan":       plan,
		},
	})
}

// CancelBranchSubscription cancels an active subscription for a branch
func (sc *WholesalerBranchSubscriptionController) CancelBranchSubscription(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	branchID := c.Param("branchId")
	if branchID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Missing branch ID",
		})
	}
	branchObjectID, err := primitive.ObjectIDFromHex(branchID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid branch ID",
		})
	}

	subscriptionsCollection := sc.DB.Collection("wholesaler_branch_subscriptions")
	update := bson.M{
		"$set": bson.M{
			"status":    "cancelled",
			"autoRenew": false,
			"updatedAt": time.Now(),
		},
	}

	result, err := subscriptionsCollection.UpdateOne(ctx, bson.M{"branchId": branchObjectID, "status": "active"}, update)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to cancel subscription",
		})
	}

	if result.MatchedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "No active subscription found",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Subscription cancelled successfully",
	})
}

// GetBranchSubscriptionRemainingTime retrieves the remaining time for a branch's subscription
func (sc *WholesalerBranchSubscriptionController) GetBranchSubscriptionRemainingTime(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user information from token
	claims := middleware.GetUserFromToken(c)
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	branchID := c.Param("branchId")
	if branchID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Missing branch ID",
		})
	}
	branchObjectID, err := primitive.ObjectIDFromHex(branchID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid branch ID",
		})
	}

	// Find wholesaler by user ID to verify ownership
	wholesalerCollection := sc.DB.Collection("wholesalers")
	var wholesaler models.Wholesaler
	err = wholesalerCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&wholesaler)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Wholesaler not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find wholesaler",
		})
	}

	// Verify that the branch belongs to this wholesaler
	branchFound := false
	for _, branch := range wholesaler.Branches {
		if branch.ID == branchObjectID {
			branchFound = true
			break
		}
	}

	if !branchFound {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Branch does not belong to this wholesaler",
		})
	}

	subscriptionsCollection := sc.DB.Collection("wholesaler_branch_subscriptions")
	var subscription models.WholesalerBranchSubscription
	err = subscriptionsCollection.FindOne(ctx, bson.M{"branchId": branchObjectID, "status": "active", "endDate": bson.M{"$gt": time.Now()}}).Decode(&subscription)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusOK, models.Response{
				Status:  http.StatusOK,
				Message: "No active subscription found",
				Data: map[string]interface{}{
					"hasActiveSubscription": false,
					"remainingTime":         nil,
				},
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find subscription",
		})
	}

	now := time.Now()
	remainingTime := subscription.EndDate.Sub(now)
	days := int(remainingTime.Hours() / 24)
	hours := int(remainingTime.Hours()) % 24
	minutes := int(remainingTime.Minutes()) % 60
	seconds := int(remainingTime.Seconds()) % 60
	remainingTimeFormatted := fmt.Sprintf("%dd %dh %dm %ds", days, hours, minutes, seconds)
	totalDuration := subscription.EndDate.Sub(subscription.StartDate)
	usedDuration := now.Sub(subscription.StartDate)
	percentageUsed := (usedDuration.Seconds() / totalDuration.Seconds()) * 100

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Wholesaler branch subscription remaining time retrieved successfully",
		Data: map[string]interface{}{
			"hasActiveSubscription": true,
			"remainingTime": map[string]interface{}{
				"days":           days,
				"hours":          hours,
				"minutes":        minutes,
				"seconds":        seconds,
				"formatted":      remainingTimeFormatted,
				"percentageUsed": fmt.Sprintf("%.1f%%", percentageUsed),
				"startDate":      subscription.StartDate.Format(time.RFC3339),
				"endDate":        subscription.EndDate.Format(time.RFC3339),
			},
		},
	})
}

// GetPendingWholesalerBranchSubscriptionRequests retrieves all pending wholesaler branch subscription requests (admin only)
func (sc *WholesalerBranchSubscriptionController) GetPendingWholesalerBranchSubscriptionRequests(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user information from token
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "admin" && claims.UserType != "manager" && claims.UserType != "sales_manager" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins, managers, or sales managers can access this endpoint",
		})
	}

	// Find all pending wholesaler branch subscription requests
	subscriptionRequestsCollection := sc.DB.Collection("wholesaler_branch_subscription_requests")
	cursor, err := subscriptionRequestsCollection.Find(ctx, bson.M{"status": "pending"})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve subscription requests",
		})
	}
	defer cursor.Close(ctx)

	var requests []models.WholesalerBranchSubscriptionRequest
	if err = cursor.All(ctx, &requests); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode subscription requests",
		})
	}

	// Get branch, wholesaler, and plan details for each request
	var enrichedRequests []map[string]interface{}
	for _, req := range requests {
		log.Printf("Processing request ID: %v, Branch ID: %v", req.ID, req.BranchID)

		// Get all wholesalers and find the one containing this branch
		var wholesaler models.Wholesaler
		var branch models.Branch
		branchFound := false

		// Use aggregation to find wholesaler with specific branch
		pipeline := []bson.M{
			{"$match": bson.M{"branches._id": req.BranchID}},
			{"$limit": 1},
		}

		log.Printf("Aggregation pipeline: %+v", pipeline)
		cursor, err := sc.DB.Collection("wholesalers").Aggregate(ctx, pipeline)
		if err != nil {
			log.Printf("Error aggregating wholesalers: %v", err)
			continue
		}

		if cursor.Next(ctx) {
			if err := cursor.Decode(&wholesaler); err != nil {
				log.Printf("Error decoding wholesaler: %v", err)
				cursor.Close(ctx)
				continue
			}

			log.Printf("Found wholesaler: %v with %d branches", wholesaler.ID, len(wholesaler.Branches))

			// Find the specific branch
			for _, b := range wholesaler.Branches {
				if b.ID == req.BranchID {
					branch = b
					branchFound = true
					log.Printf("Found matching branch: %v", b.ID)
					break
				}
			}
		} else {
			log.Printf("No wholesaler found with branch ID: %v", req.BranchID)
		}
		cursor.Close(ctx)

		if !branchFound {
			log.Printf("Branch not found for request: %v", req.ID)
			continue
		}

		// Get plan details
		var plan models.SubscriptionPlan
		err = sc.DB.Collection("subscription_plans").FindOne(ctx, bson.M{"_id": req.PlanID}).Decode(&plan)
		if err != nil {
			log.Printf("Error getting plan details: %v", err)
			continue
		}

		enrichedRequests = append(enrichedRequests, map[string]interface{}{
			"request": req,
			"branch": map[string]interface{}{
				"id":       branch.ID,
				"name":     branch.Name,
				"location": branch.Location,
				"phone":    branch.Phone,
				"status":   branch.Status,
			},
			"wholesaler": map[string]interface{}{
				"id":           wholesaler.ID,
				"businessName": wholesaler.BusinessName,
				"category":     wholesaler.Category,
				"phone":        wholesaler.Phone,
			},
			"plan": plan,
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Pending wholesaler branch subscription requests retrieved successfully",
		Data:    enrichedRequests,
	})
}

// ProcessWholesalerBranchSubscriptionRequest handles the approval or rejection of a wholesaler branch subscription request (admin only)
func (sc *WholesalerBranchSubscriptionController) ProcessWholesalerBranchSubscriptionRequest(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user information from token
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "admin" && claims.UserType != "manager" && claims.UserType != "sales_manager" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins, managers, or sales managers can process subscription requests",
		})
	}

	// Get request ID from URL parameter
	requestID := c.Param("id")
	if requestID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Request ID is required",
		})
	}

	// Convert request ID to ObjectID
	requestObjectID, err := primitive.ObjectIDFromHex(requestID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request ID format",
		})
	}

	// Parse approval request body
	var approvalReq struct {
		Status    string `json:"status"` // "approved" or "rejected"
		AdminNote string `json:"adminNote,omitempty"`
	}
	if err := c.Bind(&approvalReq); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}

	// Validate status
	if approvalReq.Status != "approved" && approvalReq.Status != "rejected" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid status. Must be 'approved' or 'rejected'",
		})
	}

	// Get the wholesaler branch subscription request
	subscriptionRequestsCollection := sc.DB.Collection("wholesaler_branch_subscription_requests")
	var subscriptionRequest models.WholesalerBranchSubscriptionRequest
	err = subscriptionRequestsCollection.FindOne(ctx, bson.M{"_id": requestObjectID}).Decode(&subscriptionRequest)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Wholesaler branch subscription request not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find subscription request",
		})
	}

	// Check if request is already processed
	if subscriptionRequest.Status != "pending" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: fmt.Sprintf("Wholesaler branch subscription request is already %s", subscriptionRequest.Status),
		})
	}

	// Get wholesaler details using aggregation
	var wholesaler models.Wholesaler
	var branch models.Branch
	branchFound := false

	log.Printf("Looking for wholesaler with branch ID: %v", subscriptionRequest.BranchID)

	// Use aggregation to find wholesaler with specific branch
	pipeline := []bson.M{
		{"$match": bson.M{"branches._id": subscriptionRequest.BranchID}},
		{"$limit": 1},
	}

	log.Printf("Aggregation pipeline: %+v", pipeline)
	cursor, err := sc.DB.Collection("wholesalers").Aggregate(ctx, pipeline)
	if err != nil {
		log.Printf("Error aggregating wholesalers: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to get wholesaler details",
		})
	}
	defer cursor.Close(ctx)

	if cursor.Next(ctx) {
		if err := cursor.Decode(&wholesaler); err != nil {
			log.Printf("Error decoding wholesaler: %v", err)
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to decode wholesaler details",
			})
		}

		log.Printf("Found wholesaler: %v with %d branches", wholesaler.ID, len(wholesaler.Branches))

		// Find the specific branch
		for _, b := range wholesaler.Branches {
			if b.ID == subscriptionRequest.BranchID {
				branch = b
				branchFound = true
				log.Printf("Found matching branch: %v", b.ID)
				break
			}
		}
	} else {
		log.Printf("No wholesaler found with branch ID: %v", subscriptionRequest.BranchID)
	}

	if !branchFound {
		log.Printf("Branch not found for request: %v", subscriptionRequest.ID)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Branch not found in wholesaler",
		})
	}

	// Get plan details
	var plan models.SubscriptionPlan
	err = sc.DB.Collection("subscription_plans").FindOne(ctx, bson.M{"_id": subscriptionRequest.PlanID}).Decode(&plan)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to get plan details",
		})
	}

	// If approved, create the subscription
	var subscription *models.WholesalerBranchSubscription
	if approvalReq.Status == "approved" {
		// Calculate end date based on plan duration
		startDate := time.Now()
		var endDate time.Time
		switch plan.Duration {
		case 1: // Monthly
			endDate = startDate.AddDate(0, 1, 0)
		case 6: // 6 Months
			endDate = startDate.AddDate(0, 6, 0)
		case 12: // 1 Year
			endDate = startDate.AddDate(1, 0, 0)
		default:
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Invalid plan duration",
			})
		}

		// Create subscription
		newSubscription := models.WholesalerBranchSubscription{
			ID:        primitive.NewObjectID(),
			BranchID:  subscriptionRequest.BranchID,
			PlanID:    subscriptionRequest.PlanID,
			StartDate: startDate,
			EndDate:   endDate,
			Status:    "active",
			AutoRenew: false, // Default to false, can be changed later
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		// Save subscription
		subscriptionsCollection := sc.DB.Collection("wholesaler_branch_subscriptions")
		_, err = subscriptionsCollection.InsertOne(ctx, newSubscription)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to create subscription",
			})
		}

		// Update branch status to active in the wholesaler's branches array
		wholesalerCollection := sc.DB.Collection("wholesalers")
		_, err = wholesalerCollection.UpdateOne(
			ctx,
			bson.M{
				"_id":          wholesaler.ID,
				"branches._id": subscriptionRequest.BranchID,
			},
			bson.M{
				"$set": bson.M{
					"branches.$.status":    "active",
					"branches.$.updatedAt": time.Now(),
				},
			},
		)
		if err != nil {
			log.Printf("Failed to update branch status to active: %v", err)
		}

		subscription = &newSubscription

		// --- Commission logic start ---
		// Check if wholesaler was created by a salesperson or by itself (user signup)
		log.Printf("DEBUG: Wholesaler CreatedBy field: %v (IsZero: %v)", wholesaler.CreatedBy, wholesaler.CreatedBy.IsZero())

		// Check if wholesaler was created by itself (user signup) - CreatedBy equals UserID
		if wholesaler.CreatedBy == wholesaler.UserID {
			log.Printf("DEBUG: Wholesaler was created by user signup, adding subscription price to admin wallet")
			// Add subscription price directly to admin wallet (no commission calculation needed)
			err := sc.addSubscriptionIncomeToAdminWallet(ctx, plan.Price, newSubscription.ID, "wholesaler_branch_subscription", wholesaler.BusinessName, branch.Name)
			if err != nil {
				log.Printf("Failed to add subscription income to admin wallet: %v", err)
			} else {
				log.Printf("Subscription income added to admin wallet: $%.2f from wholesaler '%s' (ID: %s) - User signup subscription",
					plan.Price, wholesaler.BusinessName, wholesaler.ID.Hex())
			}
		} else if !wholesaler.CreatedBy.IsZero() {
			log.Printf("DEBUG: Wholesaler was created by salesperson, proceeding with commission calculation")
			// Get salesperson
			var salesperson models.Salesperson
			err := sc.DB.Collection("salespersons").FindOne(ctx, bson.M{"_id": wholesaler.CreatedBy}).Decode(&salesperson)
			if err == nil {
				log.Printf("DEBUG: Found salesperson: %s (ID: %v)", salesperson.FullName, salesperson.ID)
				// Get sales manager - try both collection names due to inconsistency
				var salesManager models.SalesManager
				err := sc.DB.Collection("sales_managers").FindOne(ctx, bson.M{"_id": salesperson.SalesManagerID}).Decode(&salesManager)
				if err != nil {
					// Try the alternative collection name
					log.Printf("DEBUG: Sales manager not found in sales_managers collection, trying salesManagers collection")
					err = sc.DB.Collection("salesManagers").FindOne(ctx, bson.M{"_id": salesperson.SalesManagerID}).Decode(&salesManager)
				}
				if err == nil {
					log.Printf("DEBUG: Found sales manager: %s (ID: %v)", salesManager.FullName, salesManager.ID)
					planPrice := plan.Price
					salespersonPercent := salesperson.CommissionPercent
					salesManagerPercent := salesManager.CommissionPercent

					log.Printf("DEBUG: Plan price: $%.2f, Salesperson commission percent: %.2f%%, Sales Manager commission percent: %.2f%%",
						planPrice, salespersonPercent, salesManagerPercent)

					// Calculate commissions correctly
					// Salesperson gets their percentage directly from the plan price
					salespersonCommission := planPrice * salespersonPercent / 100.0
					// Sales manager gets their percentage directly from the plan price
					salesManagerCommission := planPrice * salesManagerPercent / 100.0

					log.Printf("DEBUG: Calculated commissions - Sales Manager: $%.2f, Salesperson: $%.2f",
						salesManagerCommission, salespersonCommission)

					// Insert commission document using Commission model
					commission := models.Commission{
						ID:                            primitive.NewObjectID(),
						SubscriptionID:                newSubscription.ID,
						CompanyID:                     wholesaler.ID, // Using wholesaler ID as the entity ID
						PlanID:                        plan.ID,
						PlanPrice:                     planPrice,
						SalespersonID:                 salesperson.ID,
						SalespersonCommission:         salespersonCommission,
						SalespersonCommissionPercent:  salespersonPercent,
						SalesManagerID:                salesManager.ID,
						SalesManagerCommission:        salesManagerCommission,
						SalesManagerCommissionPercent: salesManagerPercent,
						CreatedAt:                     time.Now(),
						Paid:                          false,
						PaidAt:                        nil,
					}
					_, err := sc.DB.Collection("commissions").InsertOne(ctx, commission)
					if err != nil {
						log.Printf("Failed to insert commission: %v", err)
					} else {
						log.Printf("Commission inserted successfully for wholesaler branch subscription - Plan Price: $%.2f, Sales Manager Commission: $%.2f, Salesperson Commission: $%.2f",
							planPrice, salesManagerCommission, salespersonCommission)
					}
				} else {
					log.Printf("DEBUG: Failed to find sales manager for ID: %v, Error: %v", salesperson.SalesManagerID, err)
				}
			} else {
				log.Printf("DEBUG: Failed to find salesperson for ID: %v, Error: %v", wholesaler.CreatedBy, err)
			}
		} else {
			log.Printf("DEBUG: Wholesaler was not created by a salesperson (CreatedBy is zero or not set)")
		}
		// --- Commission logic end ---

		// Send approval notification to wholesaler
		if err := sc.sendWholesalerNotificationEmail(
			wholesaler.Phone,
			"Branch Subscription Approved",
			fmt.Sprintf("Your branch subscription request for '%s' has been approved. Your subscription is active until %s.", branch.Name, endDate.Format("2006-01-02")),
		); err != nil {
			log.Printf("Failed to send wholesaler notification email: %v", err)
		}
	} else {
		// If rejected, update branch status to inactive
		wholesalerCollection := sc.DB.Collection("wholesalers")
		_, err = wholesalerCollection.UpdateOne(
			ctx,
			bson.M{
				"_id":          wholesaler.ID,
				"branches._id": subscriptionRequest.BranchID,
			},
			bson.M{
				"$set": bson.M{
					"branches.$.status":    "inactive",
					"branches.$.updatedAt": time.Now(),
				},
			},
		)
		if err != nil {
			log.Printf("Failed to update branch status to inactive: %v", err)
		}

		// Send rejection notification to wholesaler
		if err := sc.sendWholesalerNotificationEmail(
			wholesaler.Phone,
			"Branch Subscription Request Rejected",
			fmt.Sprintf("Your branch subscription request for '%s' has been rejected. Reason: %s", branch.Name, approvalReq.AdminNote),
		); err != nil {
			log.Printf("Failed to send wholesaler notification email: %v", err)
		}
	}

	// Update the request status
	now := time.Now()
	update := bson.M{
		"$set": bson.M{
			"status":      approvalReq.Status,
			"processedAt": now,
			"adminNote":   approvalReq.AdminNote,
		},
	}

	if approvalReq.Status == "approved" {
		update["$set"].(bson.M)["approvedAt"] = now
		update["$set"].(bson.M)["approvedBy"] = claims.UserType
	} else {
		update["$set"].(bson.M)["rejectedAt"] = now
		update["$set"].(bson.M)["rejectedBy"] = claims.UserType
	}

	_, err = subscriptionRequestsCollection.UpdateOne(ctx, bson.M{"_id": requestObjectID}, update)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update subscription request",
		})
	}

	// Prepare response data
	responseData := map[string]interface{}{
		"requestId":      subscriptionRequest.ID,
		"branchName":     branch.Name,
		"wholesalerName": wholesaler.BusinessName,
		"planName":       plan.Title,
		"status":         approvalReq.Status,
		"processedAt":    now,
		"adminNote":      approvalReq.AdminNote,
		"subscription":   subscription,
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: fmt.Sprintf("Wholesaler branch subscription request %s successfully", approvalReq.Status),
		Data:    responseData,
	})
}

// sendWholesalerNotificationEmail sends a notification email to a wholesaler
func (sc *WholesalerBranchSubscriptionController) sendWholesalerNotificationEmail(phone, subject, body string) error {
	// TODO: Implement actual email sending to wholesaler
	// For now, just log the notification
	log.Printf("Wholesaler notification (to %s): %s - %s", phone, subject, body)
	return nil
}

// CreateWholesalerBranchSponsorshipRequest allows wholesalers to create sponsorship requests for their branches
func (sc *WholesalerBranchSubscriptionController) CreateWholesalerBranchSponsorshipRequest(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user information from token
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "wholesaler" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only wholesalers can create sponsorship requests for their branches",
		})
	}

	// Get branch ID from URL parameter
	branchID := c.Param("branchId")
	if branchID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Branch ID is required",
		})
	}

	// Convert branch ID to ObjectID
	branchObjectID, err := primitive.ObjectIDFromHex(branchID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid branch ID format",
		})
	}

	// Parse request body
	var req struct {
		SponsorshipID primitive.ObjectID `json:"sponsorshipId" validate:"required"`
		AdminNote     string             `json:"adminNote,omitempty"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}

	// Validate sponsorship ID
	if req.SponsorshipID.IsZero() {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Sponsorship ID is required",
		})
	}

	// Get wholesaler by user ID
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	wholesalerCollection := sc.DB.Collection("wholesalers")
	var wholesaler models.Wholesaler
	err = wholesalerCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&wholesaler)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Wholesaler not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find wholesaler",
		})
	}

	// Verify that the branch belongs to this wholesaler
	var branch models.Branch
	branchFound := false
	for _, b := range wholesaler.Branches {
		if b.ID == branchObjectID {
			branch = b
			branchFound = true
			break
		}
	}

	if !branchFound {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Branch not found or you don't have access to it",
		})
	}

	// Check if sponsorship exists and is valid
	sponsorshipCollection := sc.DB.Collection("sponsorships")
	var sponsorship models.Sponsorship
	err = sponsorshipCollection.FindOne(ctx, bson.M{"_id": req.SponsorshipID}).Decode(&sponsorship)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Sponsorship not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve sponsorship",
		})
	}

	// Check if sponsorship is still valid (not expired)
	if time.Now().After(sponsorship.EndDate) {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Sponsorship has expired",
		})
	}

	// Check if sponsorship is still valid (not started yet)
	if time.Now().Before(sponsorship.StartDate) {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Sponsorship has not started yet",
		})
	}

	// Check if there's already a pending or active subscription for this branch and sponsorship
	subscriptionCollection := sc.DB.Collection("sponsorship_subscriptions")
	var existingSubscription models.SponsorshipSubscription
	err = subscriptionCollection.FindOne(ctx, bson.M{
		"sponsorshipId": req.SponsorshipID,
		"entityId":      branchObjectID,
		"entityType":    "wholesaler_branch",
		"status":        bson.M{"$in": []string{"active", "pending"}},
	}).Decode(&existingSubscription)
	if err == nil {
		return c.JSON(http.StatusConflict, models.Response{
			Status:  http.StatusConflict,
			Message: "This branch already has a pending or active subscription for this sponsorship",
		})
	}

	// Check if there's already a pending request for this branch and sponsorship
	existingRequestCollection := sc.DB.Collection("sponsorship_subscription_requests")
	var existingRequest models.SponsorshipSubscriptionRequest
	err = existingRequestCollection.FindOne(ctx, bson.M{
		"sponsorshipId": req.SponsorshipID,
		"entityId":      branchObjectID,
		"entityType":    "wholesaler_branch",
		"status":        bson.M{"$in": []string{"pending", "pending_payment"}},
	}).Decode(&existingRequest)
	if err == nil {
		return c.JSON(http.StatusConflict, models.Response{
			Status:  http.StatusConflict,
			Message: "This branch already has a pending subscription request for this sponsorship",
		})
	}

	// Generate external ID for Whish payment (using timestamp-based unique ID)
	externalID := time.Now().UnixNano() / int64(time.Millisecond)

	// Create sponsorship subscription request
	subscriptionRequest := models.SponsorshipSubscriptionRequest{
		ID:            primitive.NewObjectID(),
		SponsorshipID: req.SponsorshipID,
		EntityType:    "wholesaler_branch",
		EntityID:      branchObjectID,
		EntityName:    fmt.Sprintf("%s - %s", wholesaler.BusinessName, branch.Name),
		Status:        "pending_payment",
		RequestedAt:   time.Now(),
		AdminNote:     req.AdminNote,
		ExternalID:    externalID,
		PaymentStatus: "pending",
	}

	// Get base URL for callback URLs (backend API)
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "https://barrim.online" // Default fallback
	}

	// Get app URL for user redirects (frontend/mobile app)
	appURL := os.Getenv("APP_URL")
	if appURL == "" {
		appURL = "barrim://payment" // Fallback to baseURL if APP_URL not set
	}

	// Initialize Whish service
	whishService := services.NewWhishService()

	// Check Whish merchant account balance to verify account is active
	whishBalance, err := whishService.GetBalance()
	if err != nil {
		log.Printf("Warning: Could not check Whish account balance: %v", err)
		// Continue anyway - balance check failure doesn't block payment creation
	} else {
		log.Printf("Whish merchant account balance: $%.2f", whishBalance)
		if whishBalance < 0 {
			log.Printf("Warning: Whish account has negative balance: $%.2f", whishBalance)
		}
	}

	// Create Whish payment request
	whishReq := models.WhishRequest{
		Amount:             &sponsorship.Price,
		Currency:           "USD", // Use USD for sponsorship payments
		Invoice:            fmt.Sprintf("Wholesaler Branch Sponsorship - %s - %s - Sponsorship: %s", wholesaler.BusinessName, branch.Name, sponsorship.Title),
		ExternalID:         &externalID,
		SuccessCallbackURL: fmt.Sprintf("%s/api/whish/wholesaler-branch/sponsorship/payment/callback/success", baseURL),
		FailureCallbackURL: fmt.Sprintf("%s/api/whish/wholesaler-branch/sponsorship/payment/callback/failure", baseURL),
		SuccessRedirectURL: fmt.Sprintf("%s/payment-success?requestId=%s", appURL, subscriptionRequest.ID.Hex()),
		FailureRedirectURL: fmt.Sprintf("%s/payment-failed?requestId=%s", appURL, subscriptionRequest.ID.Hex()),
	}

	// Call Whish API to create payment
	collectURL, err := whishService.PostPayment(whishReq)
	if err != nil {
		log.Printf("Failed to create Whish payment: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: fmt.Sprintf("Failed to initiate payment: %v", err),
		})
	}

	// Save collectUrl to subscription request
	subscriptionRequest.CollectURL = collectURL

	// Save the sponsorship subscription request to database
	_, err = existingRequestCollection.InsertOne(ctx, subscriptionRequest)
	if err != nil {
		log.Printf("Failed to save sponsorship subscription request: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to create sponsorship subscription request",
		})
	}

	log.Printf("Whish payment created for wholesaler branch sponsorship request %s: %s", subscriptionRequest.ID.Hex(), collectURL)

	// Send notification to admin (optional)
	adminEmail := os.Getenv("ADMIN_EMAIL")
	if adminEmail != "" {
		subject := "New Wholesaler Branch Sponsorship Request"
		body := fmt.Sprintf("A new sponsorship request has been submitted by wholesaler: %s\nBranch: %s\nSponsorship: %s\nPrice: $%.2f\nRequested At: %s\n",
			wholesaler.BusinessName, branch.Name, sponsorship.Title, sponsorship.Price, subscriptionRequest.RequestedAt.Format("2006-01-02 15:04:05"))

		if err := sc.sendWholesalerNotificationEmail(adminEmail, subject, body); err != nil {
			log.Printf("Failed to send admin notification email: %v", err)
		}
	}

	return c.JSON(http.StatusCreated, models.Response{
		Status:  http.StatusCreated,
		Message: "Sponsorship subscription request created successfully. Please complete the payment.",
		Data: map[string]interface{}{
			"requestId":   subscriptionRequest.ID,
			"sponsorship": sponsorship,
			"branch":      branch,
			"wholesaler":  wholesaler.BusinessName,
			"status":      subscriptionRequest.Status,
			"submittedAt": subscriptionRequest.RequestedAt,
			"adminNote":   subscriptionRequest.AdminNote,
			"paymentUrl":  collectURL,
			"price":       sponsorship.Price,
		},
	})
}

// addSubscriptionIncomeToAdminWallet adds subscription income to the admin wallet
func (sc *WholesalerBranchSubscriptionController) addSubscriptionIncomeToAdminWallet(ctx context.Context, amount float64, entityID primitive.ObjectID, entityType, wholesalerName, branchName string) error {
	// Create admin wallet transaction
	adminWalletTransaction := models.AdminWallet{
		ID:          primitive.NewObjectID(),
		Type:        "subscription_income",
		Amount:      amount,
		Description: fmt.Sprintf("Subscription income from %s - %s (%s)", wholesalerName, branchName, entityType),
		EntityID:    entityID,
		EntityType:  entityType,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Insert the transaction
	_, err := sc.DB.Collection("admin_wallet").InsertOne(ctx, adminWalletTransaction)
	if err != nil {
		return fmt.Errorf("failed to insert admin wallet transaction: %w", err)
	}

	// Update or create admin wallet balance
	balanceCollection := sc.DB.Collection("admin_wallet_balance")

	// Try to find existing balance record
	var balance models.AdminWalletBalance
	err = balanceCollection.FindOne(ctx, bson.M{}).Decode(&balance)

	if err == mongo.ErrNoDocuments {
		// Create new balance record
		balance = models.AdminWalletBalance{
			ID:                    primitive.NewObjectID(),
			TotalIncome:           amount,
			TotalWithdrawalIncome: 0,
			TotalCommissionsPaid:  0,
			NetBalance:            amount,
			LastUpdated:           time.Now(),
		}
		_, err = balanceCollection.InsertOne(ctx, balance)
		if err != nil {
			return fmt.Errorf("failed to create admin wallet balance: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to find admin wallet balance: %w", err)
	} else {
		// Update existing balance record
		update := bson.M{
			"$inc": bson.M{
				"totalIncome": amount,
				"netBalance":  amount,
			},
			"$set": bson.M{
				"lastUpdated": time.Now(),
			},
		}
		_, err = balanceCollection.UpdateOne(ctx, bson.M{"_id": balance.ID}, update)
		if err != nil {
			return fmt.Errorf("failed to update admin wallet balance: %w", err)
		}
	}

	return nil
}

// HandleWhishSponsorshipPaymentSuccess handles Whish payment success callback for wholesaler branch sponsorship
func (sc *WholesalerBranchSubscriptionController) HandleWhishSponsorshipPaymentSuccess(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log.Printf("==========================================")
	log.Printf("💰 WHOLESALER SPONSORSHIP PAYMENT CALLBACK RECEIVED")
	log.Printf("==========================================")

	// Get externalId from query parameters (Whish sends it as GET parameter)
	externalIDStr := c.QueryParam("externalId")
	if externalIDStr == "" {
		log.Printf("❌ PAYMENT FAILED: Missing externalId in Whish sponsorship success callback")
		return c.String(http.StatusBadRequest, "Missing externalId parameter")
	}

	externalID, err := strconv.ParseInt(externalIDStr, 10, 64)
	if err != nil {
		log.Printf("❌ PAYMENT FAILED: Invalid externalId in callback: %v", err)
		return c.String(http.StatusBadRequest, "Invalid externalId")
	}

	log.Printf("📋 Processing payment for externalId: %d", externalID)

	// Find the sponsorship subscription request by externalId
	requestCollection := sc.DB.Collection("sponsorship_subscription_requests")
	var subscriptionRequest models.SponsorshipSubscriptionRequest
	err = requestCollection.FindOne(ctx, bson.M{"externalId": externalID}).Decode(&subscriptionRequest)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			log.Printf("Sponsorship subscription request not found for externalId: %d", externalID)
			return c.String(http.StatusNotFound, "Sponsorship subscription request not found")
		}
		log.Printf("Error finding sponsorship subscription request: %v", err)
		return c.String(http.StatusInternalServerError, "Database error")
	}

	// Check if already processed
	if subscriptionRequest.PaymentStatus == "success" || subscriptionRequest.Status == "approved" || subscriptionRequest.Status == "active" {
		log.Printf("Payment already processed for sponsorship request: %s", subscriptionRequest.ID.Hex())
		return c.String(http.StatusOK, "Payment already processed")
	}

	// Initialize Whish service and verify payment status
	whishService := services.NewWhishService()
	status, phoneNumber, err := whishService.GetPaymentStatus("USD", externalID)
	if err != nil {
		log.Printf("Failed to verify payment status: %v", err)
		return c.String(http.StatusInternalServerError, "Failed to verify payment")
	}

	if status != "success" {
		log.Printf("❌ PAYMENT FAILED: Payment not successful, status: %s", status)
		log.Printf("   Request ID: %s", subscriptionRequest.ID.Hex())
		log.Printf("   Entity: %s (%s)", subscriptionRequest.EntityName, subscriptionRequest.EntityType)
		// Update request status to failed
		requestCollection.UpdateOne(ctx,
			bson.M{"_id": subscriptionRequest.ID},
			bson.M{"$set": bson.M{
				"paymentStatus": "failed",
				"status":        "failed",
				"processedAt":   time.Now(),
			}})
		log.Printf("==========================================")
		return c.String(http.StatusBadRequest, "Payment not successful")
	}

	// Payment verified successfully - activate subscription immediately
	// Get sponsorship details
	sponsorshipCollection := sc.DB.Collection("sponsorships")
	var sponsorship models.Sponsorship
	err = sponsorshipCollection.FindOne(ctx, bson.M{"_id": subscriptionRequest.SponsorshipID}).Decode(&sponsorship)
	if err != nil {
		log.Printf("Failed to get sponsorship details: %v", err)
		return c.String(http.StatusInternalServerError, "Failed to get sponsorship details")
	}

	// Add sponsorship income to admin wallet using sponsorship subscription controller
	sponsorshipSubscriptionController := NewSponsorshipSubscriptionController(sc.DB)
	err = sponsorshipSubscriptionController.addSponsorshipIncomeToAdminWallet(
		ctx,
		sponsorship.Price,
		subscriptionRequest.SponsorshipID,
		fmt.Sprintf("%s - %s", sponsorship.Title, subscriptionRequest.EntityName),
	)
	if err != nil {
		log.Printf("Failed to add sponsorship income to admin wallet: %v", err)
		// Don't fail the payment if wallet update fails, but log it
	}

	// Create active subscription immediately after payment
	err = sponsorshipSubscriptionController.createActiveSubscription(ctx, subscriptionRequest)
	if err != nil {
		log.Printf("Failed to create active subscription: %v", err)
		return c.String(http.StatusInternalServerError, "Failed to activate subscription")
	}

	// Update entity sponsorship status to active
	err = sponsorshipSubscriptionController.updateEntitySponsorshipStatus(ctx, subscriptionRequest.EntityType, subscriptionRequest.EntityID, true)
	if err != nil {
		log.Printf("Failed to update entity sponsorship status: %v", err)
		// Don't fail the process if status update fails, but log it
	}

	// Update request status - mark as approved/active after payment
	update := bson.M{
		"$set": bson.M{
			"paymentStatus": "success",
			"status":        "approved",
			"adminApproved": true,
			"approvedAt":    time.Now(),
			"paidAt":        time.Now(),
			"processedAt":   time.Now(),
		},
	}

	_, err = requestCollection.UpdateOne(ctx, bson.M{"_id": subscriptionRequest.ID}, update)
	if err != nil {
		log.Printf("Failed to update sponsorship subscription request status: %v", err)
		return c.String(http.StatusInternalServerError, "Failed to update request status")
	}

	log.Printf("✅ PAYMENT SUCCESS: Wholesaler branch sponsorship payment completed and activated")
	log.Printf("   Request ID: %s", subscriptionRequest.ID.Hex())
	log.Printf("   External ID: %d", externalID)
	log.Printf("   Entity: %s (%s)", subscriptionRequest.EntityName, subscriptionRequest.EntityType)
	log.Printf("   Amount: $%.2f", sponsorship.Price)
	log.Printf("   Phone: %s", phoneNumber)
	log.Printf("   Sponsorship: %s", sponsorship.Title)
	log.Printf("   Status: Activated")
	log.Printf("==========================================")
	return c.String(http.StatusOK, "Payment successful and sponsorship activated.")
}

// HandleWhishSponsorshipPaymentFailure handles Whish payment failure callback for wholesaler branch sponsorship
func (sc *WholesalerBranchSubscriptionController) HandleWhishSponsorshipPaymentFailure(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log.Printf("==========================================")
	log.Printf("❌ WHOLESALER SPONSORSHIP PAYMENT FAILURE CALLBACK RECEIVED")
	log.Printf("==========================================")

	externalIDStr := c.QueryParam("externalId")
	if externalIDStr == "" {
		log.Printf("❌ PAYMENT FAILED: Missing externalId parameter in failure callback")
		return c.String(http.StatusBadRequest, "Missing externalId parameter")
	}

	externalID, err := strconv.ParseInt(externalIDStr, 10, 64)
	if err != nil {
		log.Printf("❌ PAYMENT FAILED: Invalid externalId: %v", err)
		return c.String(http.StatusBadRequest, "Invalid externalId")
	}

	log.Printf("📋 Processing payment failure for externalId: %d", externalID)

	// Update sponsorship subscription request status to failed
	requestCollection := sc.DB.Collection("sponsorship_subscription_requests")
	var subscriptionRequest models.SponsorshipSubscriptionRequest
	err = requestCollection.FindOne(ctx, bson.M{"externalId": externalID}).Decode(&subscriptionRequest)
	if err == nil {
		log.Printf("   Request ID: %s", subscriptionRequest.ID.Hex())
		log.Printf("   Entity: %s (%s)", subscriptionRequest.EntityName, subscriptionRequest.EntityType)
	}

	_, err = requestCollection.UpdateOne(ctx,
		bson.M{"externalId": externalID},
		bson.M{"$set": bson.M{
			"paymentStatus": "failed",
			"status":        "failed",
			"processedAt":   time.Now(),
		}})

	if err != nil {
		log.Printf("❌ PAYMENT FAILED: Error updating request status: %v", err)
		return c.String(http.StatusInternalServerError, "Failed to update status")
	}

	log.Printf("❌ PAYMENT FAILED: Wholesaler sponsorship payment failed")
	log.Printf("   External ID: %d", externalID)
	log.Printf("   Status: Marked as failed in database")
	log.Printf("==========================================")
	return c.String(http.StatusOK, "Payment failure recorded")
}
