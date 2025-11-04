// controllers/serviceProviders_subscription_controller.go
package controllers

import (
	"context"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/HSouheill/barrim_backend/middleware"
	"github.com/HSouheill/barrim_backend/models"
	"github.com/HSouheill/barrim_backend/services"
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"gopkg.in/gomail.v2"
)

// ServiceProviderSubscriptionController handles service provider subscription operations
type ServiceProviderSubscriptionController struct {
	DB *mongo.Database
}

// NewServiceProviderSubscriptionController creates a new service provider subscription controller
func NewServiceProviderSubscriptionController(db *mongo.Database) *ServiceProviderSubscriptionController {
	return &ServiceProviderSubscriptionController{DB: db}
}

// GetServiceProviderSubscriptionPlans retrieves all available subscription plans for service providers
func (spc *ServiceProviderSubscriptionController) GetServiceProviderSubscriptionPlans(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Debug: Log the request
	log.Printf("DEBUG: GetServiceProviderSubscriptionPlans called")

	// Get user information from token to verify authentication
	claims := middleware.GetUserFromToken(c)
	if claims == nil {
		log.Printf("ERROR: No JWT claims found")
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Authentication required",
		})
	}

	log.Printf("DEBUG: JWT Claims - UserID: '%s', UserType: '%s'", claims.UserID, claims.UserType)

	// Verify user ID format
	if claims.UserID == "" {
		log.Printf("ERROR: Empty user ID in JWT claims")
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Empty user ID in token",
		})
	}

	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		log.Printf("ERROR: Invalid user ID format '%s': %v", claims.UserID, err)
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: fmt.Sprintf("Invalid user ID format: %s", claims.UserID),
		})
	}

	log.Printf("DEBUG: Successfully parsed userID: %s", userID.Hex())

	// Continue with the rest of your existing logic...
	// Just return the plans for now to test the basic auth flow
	collection := spc.DB.Collection("subscription_plans")
	cursor, err := collection.Find(ctx, bson.M{
		"type":     "serviceProvider",
		"isActive": true,
	})
	if err != nil {
		log.Printf("ERROR: Failed to query subscription plans: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve subscription plans",
		})
	}
	defer cursor.Close(ctx)

	var plans []models.SubscriptionPlan
	if err = cursor.All(ctx, &plans); err != nil {
		log.Printf("ERROR: Failed to decode subscription plans: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve subscription plans",
		})
	}

	log.Printf("DEBUG: Found %d subscription plans", len(plans))

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Service provider subscription plans retrieved successfully",
		Data:    plans,
	})
}

// findServiceProviderByUserID is a helper function to find service provider by user ID
func (spc *ServiceProviderSubscriptionController) findServiceProviderByUserID(ctx context.Context, userID primitive.ObjectID) (*models.ServiceProvider, error) {
	// First try to find in serviceProviders collection
	serviceProviderCollection := spc.DB.Collection("serviceProviders")
	var serviceProvider models.ServiceProvider
	err := serviceProviderCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&serviceProvider)
	if err == nil {
		return &serviceProvider, nil
	}

	if err != mongo.ErrNoDocuments {
		return nil, fmt.Errorf("error querying ServiceProvider collection: %w", err)
	}

	// If not found in serviceProviders collection, check users collection
	usersCollection := spc.DB.Collection("users")
	var user models.User
	err = usersCollection.FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("error querying users collection: %w", err)
	}

	// Check if user has ServiceProviderID field
	if user.ServiceProviderID != nil {
		err = serviceProviderCollection.FindOne(ctx, bson.M{"_id": *user.ServiceProviderID}).Decode(&serviceProvider)
		if err == nil {
			return &serviceProvider, nil
		}
	}

	// If still not found, create a service provider record
	// This is a fallback for users who are service providers but don't have a separate service provider record
	if user.UserType == "serviceProvider" {
		// Create a basic service provider record
		serviceProvider = models.ServiceProvider{
			ID:           primitive.NewObjectID(),
			UserID:       userID,
			BusinessName: user.FullName, // Use full name as business name fallback
			Category:     "General",     // Default category
			ContactInfo: models.ContactInfo{
				Phone: user.Phone,
			},
			CreatedBy: userID,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		// Insert the new service provider record
		_, err = serviceProviderCollection.InsertOne(ctx, serviceProvider)
		if err != nil {
			return nil, fmt.Errorf("failed to create service provider record: %w", err)
		}

		// Update user record to reference the service provider
		_, err = usersCollection.UpdateOne(ctx,
			bson.M{"_id": userID},
			bson.M{"$set": bson.M{"serviceProviderId": serviceProvider.ID}},
		)
		if err != nil {
			log.Printf("Warning: Failed to update user with service provider ID: %v", err)
		}

		return &serviceProvider, nil
	}

	return nil, fmt.Errorf("service provider not found for user ID: %s", userID.Hex())
}

// CreateServiceProviderSubscription creates a new subscription request for a service provider
func (spc *ServiceProviderSubscriptionController) CreateServiceProviderSubscription(c echo.Context) error {
	log.Printf("DEBUG: CreateServiceProviderSubscription called with path: %s", c.Request().URL.Path)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user information from token
	claims := middleware.GetUserFromToken(c)
	if claims == nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Authentication required",
		})
	}

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

	// Verify the plan exists and is active
	var plan models.SubscriptionPlan
	err = spc.DB.Collection("subscription_plans").FindOne(ctx, bson.M{
		"_id":      planObjectID,
		"isActive": true,
		"type":     "serviceProvider",
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

	// Find service provider by user ID
	serviceProvider, err := spc.findServiceProviderByUserID(ctx, userID)
	if err != nil {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: fmt.Sprintf("Service provider not found: %v", err),
		})
	}

	// Check if there's already a pending subscription request
	subscriptionRequestsCollection := spc.DB.Collection("subscription_requests")
	var existingRequest models.SubscriptionRequest
	err = subscriptionRequestsCollection.FindOne(ctx, bson.M{
		"serviceProviderId": serviceProvider.ID,
		"status":            bson.M{"$in": []string{"pending", "pending_payment"}},
	}).Decode(&existingRequest)

	if err == nil {
		return c.JSON(http.StatusConflict, models.Response{
			Status:  http.StatusConflict,
			Message: "You already have a pending subscription request. Please complete the payment first.",
		})
	} else if err != mongo.ErrNoDocuments {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Error checking existing subscription requests",
		})
	}

	// Check if service provider already has an active subscription
	subscriptionsCollection := spc.DB.Collection("serviceProviders_subscriptions")
	var activeSubscription models.ServiceProviderSubscription
	err = subscriptionsCollection.FindOne(ctx, bson.M{
		"serviceProviderId": serviceProvider.ID,
		"status":            "active",
		"endDate":           bson.M{"$gt": time.Now()},
	}).Decode(&activeSubscription)
	if err == nil {
		return c.JSON(http.StatusConflict, models.Response{
			Status:  http.StatusConflict,
			Message: "You already have an active subscription. Please wait for it to expire or cancel it first.",
		})
	} else if err != mongo.ErrNoDocuments {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Error checking active subscriptions",
		})
	}

	// Create subscription request with pending_payment status
	subscriptionRequest := models.SubscriptionRequest{
		ID:                primitive.NewObjectID(),
		ServiceProviderID: serviceProvider.ID,
		PlanID:            planObjectID,
		Status:            "pending_payment",
		RequestedAt:       time.Now(),
		PaymentStatus:     "pending",
	}

	// Generate externalId from ObjectID (use timestamp part as int64)
	externalID := int64(subscriptionRequest.ID.Timestamp().Unix())
	subscriptionRequest.ExternalID = externalID

	// Get base URL for callbacks
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "https://barrim.online" // Default fallback
	}

	// Initialize Whish service
	whishService := services.NewWhishService()

	// Check Whish merchant account balance to verify account is active
	whishBalance, err := whishService.GetBalance()
	if err != nil {
		log.Printf("Warning: Could not check Whish account balance: %v", err)
	} else {
		log.Printf("Whish merchant account balance: $%.2f", whishBalance)
		if whishBalance < 0 {
			log.Printf("Warning: Whish account has negative balance: $%.2f", whishBalance)
		}
	}

	// Create Whish payment request
	whishReq := models.WhishRequest{
		Amount:             &plan.Price,
		Currency:           "USD", // Use USD for subscription payments
		Invoice:            fmt.Sprintf("Service Provider Subscription - %s - Plan: %s", serviceProvider.BusinessName, plan.Title),
		ExternalID:         &externalID,
		SuccessCallbackURL: fmt.Sprintf("%s/api/whish/service-provider/payment/callback/success", baseURL),
		FailureCallbackURL: fmt.Sprintf("%s/api/whish/service-provider/payment/callback/failure", baseURL),
		SuccessRedirectURL: fmt.Sprintf("%s/payment-success?requestId=%s", baseURL, subscriptionRequest.ID.Hex()),
		FailureRedirectURL: fmt.Sprintf("%s/payment-failed?requestId=%s", baseURL, subscriptionRequest.ID.Hex()),
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

	log.Printf("Whish payment created for service provider subscription request %s: %s", subscriptionRequest.ID.Hex(), collectURL)

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

// HandleWhishPaymentSuccess handles Whish payment success callback for service provider subscriptions
func (spc *ServiceProviderSubscriptionController) HandleWhishPaymentSuccess(c echo.Context) error {
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
	subscriptionRequestsCollection := spc.DB.Collection("subscription_requests")
	var subscriptionRequest models.SubscriptionRequest
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
	err = spc.activateServiceProviderSubscription(ctx, subscriptionRequest, phoneNumber)
	if err != nil {
		log.Printf("Failed to activate subscription: %v", err)
		return c.String(http.StatusInternalServerError, "Failed to activate subscription")
	}

	log.Printf("Service provider subscription activated successfully for externalId: %d", externalID)
	return c.String(http.StatusOK, "Payment successful and subscription activated")
}

// HandleWhishPaymentFailure handles Whish payment failure callback for service provider subscriptions
func (spc *ServiceProviderSubscriptionController) HandleWhishPaymentFailure(c echo.Context) error {
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
	subscriptionRequestsCollection := spc.DB.Collection("subscription_requests")
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

// activateServiceProviderSubscription activates the subscription after successful payment
func (spc *ServiceProviderSubscriptionController) activateServiceProviderSubscription(ctx context.Context, subscriptionRequest models.SubscriptionRequest, payerPhone string) error {
	// Get plan details
	var plan models.SubscriptionPlan
	err := spc.DB.Collection("subscription_plans").FindOne(ctx, bson.M{"_id": subscriptionRequest.PlanID}).Decode(&plan)
	if err != nil {
		log.Printf("Failed to get plan: %v", err)
		return fmt.Errorf("failed to get plan details")
	}

	// Get service provider details
	var serviceProvider models.ServiceProvider
	err = spc.DB.Collection("serviceProviders").FindOne(ctx, bson.M{"_id": subscriptionRequest.ServiceProviderID}).Decode(&serviceProvider)
	if err != nil {
		log.Printf("Failed to get service provider: %v", err)
		return fmt.Errorf("failed to get service provider details")
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
	newSubscription := models.ServiceProviderSubscription{
		ID:                primitive.NewObjectID(),
		ServiceProviderID: subscriptionRequest.ServiceProviderID,
		PlanID:            subscriptionRequest.PlanID,
		StartDate:         startDate,
		EndDate:           endDate,
		Status:            "active",
		AutoRenew:         false,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	// Save subscription
	subscriptionsCollection := spc.DB.Collection("serviceProviders_subscriptions")
	_, err = subscriptionsCollection.InsertOne(ctx, newSubscription)
	if err != nil {
		log.Printf("Failed to create subscription: %v", err)
		return fmt.Errorf("failed to create subscription")
	}

	// Update service provider status to active
	spCollection := spc.DB.Collection("serviceProviders")
	_, err = spCollection.UpdateOne(ctx, bson.M{"_id": subscriptionRequest.ServiceProviderID}, bson.M{"$set": bson.M{"status": "active"}})
	if err != nil {
		log.Printf("Failed to update service provider status to active: %v", err)
	}

	// Handle commission and admin wallet (30% salesperson, 70% admin)
	planPrice := plan.Price
	if !serviceProvider.CreatedBy.IsZero() && serviceProvider.CreatedBy != serviceProvider.UserID {
		// Service provider was created by a salesperson - split commission
		var salesperson models.Salesperson
		err := spc.DB.Collection("salespersons").FindOne(ctx, bson.M{"_id": serviceProvider.CreatedBy}).Decode(&salesperson)
		if err == nil {
			// Calculate commissions: 30% salesperson, 70% admin
			salespersonPercent := 30.0
			adminPercent := 70.0

			salespersonCommission := planPrice * salespersonPercent / 100.0
			adminCommission := planPrice * adminPercent / 100.0

			// Add salesperson commission
			err = spc.addCommissionToSalespersonWallet(ctx, salesperson.ID, salespersonCommission, newSubscription.ID, "service_provider_subscription", serviceProvider.BusinessName)
			if err != nil {
				log.Printf("Failed to add salesperson commission: %v", err)
			} else {
				log.Printf("Added salesperson commission: $%.2f (30%% of $%.2f)", salespersonCommission, planPrice)
			}

			// Add admin commission
			err = spc.addSubscriptionIncomeToAdminWallet(ctx, adminCommission, newSubscription.ID, "service_provider_subscription_commission", serviceProvider.BusinessName, "")
			if err != nil {
				log.Printf("Failed to add admin commission: %v", err)
			} else {
				log.Printf("Added admin commission: $%.2f (70%% of $%.2f)", adminCommission, planPrice)
			}
		} else {
			log.Printf("Salesperson not found, adding full amount to admin wallet")
			// Salesperson not found, add full amount to admin
			err = spc.addSubscriptionIncomeToAdminWallet(ctx, planPrice, newSubscription.ID, "service_provider_subscription", serviceProvider.BusinessName, "")
			if err != nil {
				log.Printf("Failed to add subscription income to admin wallet: %v", err)
			}
		}
	} else {
		// Service provider created by itself - full amount to admin
		err = spc.addSubscriptionIncomeToAdminWallet(ctx, planPrice, newSubscription.ID, "service_provider_subscription", serviceProvider.BusinessName, "")
		if err != nil {
			log.Printf("Failed to add subscription income to admin wallet: %v", err)
		}
	}

	// Update subscription request status
	subscriptionRequestsCollection := spc.DB.Collection("subscription_requests")
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

	log.Printf("Service provider subscription activated successfully: ServiceProvider=%s, Plan=%s, Amount=$%.2f", serviceProvider.BusinessName, plan.Title, planPrice)
	return nil
}

// Helper methods for wallet management
func (spc *ServiceProviderSubscriptionController) addSubscriptionIncomeToAdminWallet(ctx context.Context, amount float64, entityID primitive.ObjectID, entityType, serviceProviderName, branchName string) error {
	adminWalletTransaction := models.AdminWallet{
		ID:          primitive.NewObjectID(),
		Type:        "subscription_income",
		Amount:      amount,
		Description: fmt.Sprintf("Subscription income from %s (%s)", serviceProviderName, entityType),
		EntityID:    entityID,
		EntityType:  entityType,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	_, err := spc.DB.Collection("admin_wallet").InsertOne(ctx, adminWalletTransaction)
	if err != nil {
		return fmt.Errorf("failed to insert admin wallet transaction: %w", err)
	}

	balanceCollection := spc.DB.Collection("admin_wallet_balance")
	var balance models.AdminWalletBalance
	err = balanceCollection.FindOne(ctx, bson.M{}).Decode(&balance)

	if err == mongo.ErrNoDocuments {
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

func (spc *ServiceProviderSubscriptionController) addCommissionToSalespersonWallet(ctx context.Context, salespersonID primitive.ObjectID, amount float64, subscriptionID primitive.ObjectID, entityType, serviceProviderName string) error {
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

	_, err := spc.DB.Collection("commission_records").InsertOne(ctx, commissionRecord)
	if err != nil {
		return fmt.Errorf("failed to insert salesperson commission record: %w", err)
	}

	_, err = spc.DB.Collection("salespersons").UpdateOne(
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
		amount, salespersonID.Hex(), entityType, serviceProviderName)
	return nil
}

// GetSubscriptionTimeRemaining returns the remaining time for the current subscription
func (spc *ServiceProviderSubscriptionController) GetSubscriptionTimeRemaining(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user information from token
	claims := middleware.GetUserFromToken(c)
	if claims == nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Authentication required",
		})
	}

	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	// Find service provider by user ID
	serviceProvider, err := spc.findServiceProviderByUserID(ctx, userID)
	if err != nil {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: fmt.Sprintf("Service provider not found: %v", err),
		})
	}

	// Find active subscription
	subscriptionsCollection := spc.DB.Collection("serviceProviders_subscriptions")
	var subscription models.ServiceProviderSubscription
	err = subscriptionsCollection.FindOne(ctx, bson.M{
		"serviceProviderId": serviceProvider.ID,
		"status":            "active",
		"endDate": bson.M{
			"$gt": time.Now(),
		},
	}).Decode(&subscription)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusOK, models.Response{
				Status:  http.StatusOK,
				Message: "No active subscription found",
				Data: map[string]interface{}{
					"remainingDays": 0,
					"isActive":      false,
				},
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve subscription information",
		})
	}

	// Calculate remaining time
	now := time.Now()
	remainingTime := subscription.EndDate.Sub(now)

	// Calculate days, hours, minutes, and seconds
	days := int(remainingTime.Hours() / 24)
	hours := int(remainingTime.Hours()) % 24
	minutes := int(remainingTime.Minutes()) % 60
	seconds := int(remainingTime.Seconds()) % 60

	// Format remaining time in a human-readable way
	remainingTimeFormatted := fmt.Sprintf("%dd %dh %dm %ds", days, hours, minutes, seconds)

	// Calculate percentage of subscription used
	totalDuration := subscription.EndDate.Sub(subscription.StartDate)
	usedDuration := now.Sub(subscription.StartDate)
	percentageUsed := (usedDuration.Seconds() / totalDuration.Seconds()) * 100

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Service provider subscription remaining time retrieved successfully",
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

// GetCurrentSubscription returns the current active subscription details
func (spc *ServiceProviderSubscriptionController) GetCurrentSubscription(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user information from token
	claims := middleware.GetUserFromToken(c)
	if claims == nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Authentication required",
		})
	}

	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	// Find service provider by user ID
	serviceProvider, err := spc.findServiceProviderByUserID(ctx, userID)
	if err != nil {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: fmt.Sprintf("Service provider not found: %v", err),
		})
	}

	// Find active subscription with plan details
	subscriptionsCollection := spc.DB.Collection("serviceProviders_subscriptions")
	var subscription models.ServiceProviderSubscription
	err = subscriptionsCollection.FindOne(ctx, bson.M{
		"serviceProviderId": serviceProvider.ID,
		"status":            "active",
		"endDate": bson.M{
			"$gt": time.Now(),
		},
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
			Message: "Failed to retrieve subscription information",
		})
	}

	// Get plan details
	subscriptionPlansCollection := spc.DB.Collection("subscription_plans")
	var plan models.SubscriptionPlan
	err = subscriptionPlansCollection.FindOne(ctx, bson.M{
		"_id": subscription.PlanID,
	}).Decode(&plan)

	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve plan details",
		})
	}

	// Calculate remaining days
	remainingTime := subscription.EndDate.Sub(time.Now())
	remainingDays := int(remainingTime.Hours() / 24)

	response := map[string]interface{}{
		"subscription":  subscription,
		"plan":          plan,
		"remainingDays": remainingDays,
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Current subscription retrieved successfully",
		Data:    response,
	})
}

// CancelSubscription cancels the current active subscription
func (spc *ServiceProviderSubscriptionController) CancelSubscription(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user information from token
	claims := middleware.GetUserFromToken(c)
	if claims == nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Authentication required",
		})
	}

	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	// Find service provider by user ID
	serviceProvider, err := spc.findServiceProviderByUserID(ctx, userID)
	if err != nil {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: fmt.Sprintf("Service provider not found: %v", err),
		})
	}

	// Find and update active subscription
	subscriptionsCollection := spc.DB.Collection("serviceProviders_subscriptions")
	now := time.Now()

	result, err := subscriptionsCollection.UpdateOne(
		ctx,
		bson.M{
			"serviceProviderId": serviceProvider.ID,
			"status":            "active",
			"endDate": bson.M{
				"$gt": now,
			},
		},
		bson.M{
			"$set": bson.M{
				"status":      "cancelled",
				"cancelledAt": now,
				"updatedAt":   now,
			},
		},
	)

	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to cancel subscription",
		})
	}

	if result.MatchedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "No active subscription found to cancel",
		})
	}

	// Send notification email to admin
	if err := spc.sendAdminNotificationEmail(
		"Service Provider Subscription Cancelled",
		fmt.Sprintf("Service Provider %s has cancelled their subscription.", serviceProvider.BusinessName),
	); err != nil {
		log.Printf("Failed to send admin notification email: %v", err)
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Subscription cancelled successfully",
	})
}

// GetPendingServiceProviderSubscriptionRequests retrieves all pending service provider subscription requests (admin only)
func (spc *ServiceProviderSubscriptionController) GetPendingServiceProviderSubscriptionRequests(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user information from token
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins can access this endpoint",
		})
	}

	// Find all pending service provider subscription requests from subscription_requests collection
	subscriptionRequestsCollection := spc.DB.Collection("subscription_requests")
	cursor, err := subscriptionRequestsCollection.Find(ctx, bson.M{
		"status":            "pending",
		"serviceProviderId": bson.M{"$exists": true, "$ne": nil},
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve subscription requests",
		})
	}
	defer cursor.Close(ctx)

	var requests []models.SubscriptionRequest
	if err = cursor.All(ctx, &requests); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode subscription requests",
		})
	}

	// Get service provider and plan details for each request
	var enrichedRequests []map[string]interface{}
	for _, req := range requests {
		// Get service provider details
		var serviceProvider models.ServiceProvider
		err = spc.DB.Collection("serviceProviders").FindOne(ctx, bson.M{"_id": req.ServiceProviderID}).Decode(&serviceProvider)
		if err != nil {
			log.Printf("Error getting service provider details: %v", err)
			continue
		}

		// Get plan details
		var plan models.SubscriptionPlan
		err = spc.DB.Collection("subscription_plans").FindOne(ctx, bson.M{"_id": req.PlanID}).Decode(&plan)
		if err != nil {
			log.Printf("Error getting plan details: %v", err)
			continue
		}

		enrichedRequests = append(enrichedRequests, map[string]interface{}{
			"request": req,
			"serviceProvider": map[string]interface{}{
				"id":           serviceProvider.ID,
				"businessName": serviceProvider.BusinessName,
				"category":     serviceProvider.Category,
				"contactInfo":  serviceProvider.ContactInfo,
			},
			"plan": plan,
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Pending service provider subscription requests retrieved successfully",
		Data:    enrichedRequests,
	})
}

// ProcessServiceProviderSubscriptionRequest handles the approval or rejection of a service provider subscription request (admin only)
func (spc *ServiceProviderSubscriptionController) ProcessServiceProviderSubscriptionRequest(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user information from token
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins can process subscription requests",
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
	var approvalReq models.ServiceProviderSubscriptionApprovalRequest
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

	// Get the subscription request from the subscription_requests collection
	subscriptionRequestsCollection := spc.DB.Collection("subscription_requests")
	var subscriptionRequest models.SubscriptionRequest
	err = subscriptionRequestsCollection.FindOne(ctx, bson.M{"_id": requestObjectID}).Decode(&subscriptionRequest)
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

	// Check if request is already processed
	if subscriptionRequest.Status != "pending" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: fmt.Sprintf("Subscription request is already %s", subscriptionRequest.Status),
		})
	}

	// Delete the subscription request from database after processing
	_, err = subscriptionRequestsCollection.DeleteOne(ctx, bson.M{"_id": requestObjectID})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to delete subscription request",
		})
	}

	// Get service provider and plan details for response and notifications
	var serviceProvider models.ServiceProvider
	err = spc.DB.Collection("serviceProviders").FindOne(ctx, bson.M{"_id": subscriptionRequest.ServiceProviderID}).Decode(&serviceProvider)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to get service provider details",
		})
	}

	var plan models.SubscriptionPlan
	err = spc.DB.Collection("subscription_plans").FindOne(ctx, bson.M{"_id": subscriptionRequest.PlanID}).Decode(&plan)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to get plan details",
		})
	}

	// If approved, create the subscription
	var subscription *models.ServiceProviderSubscription
	if approvalReq.Status == "approved" {
		// Calculate end date based on plan duration
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
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Invalid plan duration",
			})
		}

		// Create subscription
		newSubscription := models.ServiceProviderSubscription{
			ID:                primitive.NewObjectID(),
			ServiceProviderID: subscriptionRequest.ServiceProviderID,
			PlanID:            subscriptionRequest.PlanID,
			StartDate:         startDate,
			EndDate:           endDate,
			Status:            "active",
			AutoRenew:         false, // Default to false, can be changed later
			CreatedAt:         time.Now(),
			UpdatedAt:         time.Now(),
		}

		// Save subscription
		subscriptionsCollection := spc.DB.Collection("serviceProviders_subscriptions")
		_, err = subscriptionsCollection.InsertOne(ctx, newSubscription)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to create subscription",
			})
		}

		// Update service provider status to active
		serviceProviderCollection := spc.DB.Collection("serviceProviders")
		_, err = serviceProviderCollection.UpdateOne(ctx, bson.M{"_id": subscriptionRequest.ServiceProviderID}, bson.M{"$set": bson.M{"status": "active"}})
		if err != nil {
			log.Printf("Failed to update service provider status to active: %v", err)
		}

		// Update user status to active when subscription is approved
		usersCollection := spc.DB.Collection("users")
		_, err = usersCollection.UpdateOne(
			ctx,
			bson.M{"serviceProviderId": subscriptionRequest.ServiceProviderID},
			bson.M{"$set": bson.M{"status": "active", "updatedAt": time.Now()}},
		)
		if err != nil {
			log.Printf("Failed to update user status: %v", err)
		}

		subscription = &newSubscription

		// --- Commission logic start ---
		// Only proceed if service provider was created by a salesperson
		log.Printf("DEBUG: Service Provider CreatedBy field: %v (IsZero: %v)", serviceProvider.CreatedBy, serviceProvider.CreatedBy.IsZero())
		if !serviceProvider.CreatedBy.IsZero() {
			log.Printf("DEBUG: Service provider was created by salesperson, proceeding with commission calculation")
			// Get salesperson
			var salesperson models.Salesperson
			err := spc.DB.Collection("salespersons").FindOne(ctx, bson.M{"_id": serviceProvider.CreatedBy}).Decode(&salesperson)
			if err == nil {
				log.Printf("DEBUG: Found salesperson: %s (ID: %v)", salesperson.FullName, salesperson.ID)
				// Get admin who created the salesperson
				var admin models.Admin
				err := spc.DB.Collection("admins").FindOne(ctx, bson.M{"_id": salesperson.CreatedBy}).Decode(&admin)
				if err == nil {
					log.Printf("DEBUG: Found admin: %s (ID: %v)", admin.Email, admin.ID)
					planPrice := plan.Price
					salespersonPercent := salesperson.CommissionPercent

					// Calculate admin commission (remaining percentage after salesperson commission)
					adminPercent := 100.0 - salespersonPercent

					log.Printf("DEBUG: Plan price: $%.2f, Salesperson commission percent: %.2f%%, Admin commission percent: %.2f%%",
						planPrice, salespersonPercent, adminPercent)

					// Calculate commissions correctly
					// Salesperson gets their percentage directly from the plan price
					salespersonCommission := planPrice * salespersonPercent / 100.0
					// Admin gets the remaining percentage
					adminCommission := planPrice * adminPercent / 100.0

					log.Printf("DEBUG: Calculated commissions - Admin: $%.2f, Salesperson: $%.2f",
						adminCommission, salespersonCommission)

					// Insert commission document using Commission model
					commission := models.Commission{
						ID:                           primitive.NewObjectID(),
						SubscriptionID:               newSubscription.ID,
						CompanyID:                    primitive.NilObjectID, // No company ID for service provider
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
					}
					_, err := spc.DB.Collection("commissions").InsertOne(ctx, commission)
					if err != nil {
						log.Printf("Failed to insert commission: %v", err)
					} else {
						log.Printf("Commission inserted successfully for service provider subscription - Plan Price: $%.2f, Admin Commission: $%.2f (%.1f%%), Salesperson Commission: $%.2f (%.1f%%)",
							planPrice, adminCommission, adminPercent, salespersonCommission, salespersonPercent)
					}
				} else {
					log.Printf("DEBUG: Failed to find admin for ID: %v, Error: %v", salesperson.CreatedBy, err)
				}
			} else {
				log.Printf("DEBUG: Failed to find salesperson for ID: %v, Error: %v", serviceProvider.CreatedBy, err)
			}
		} else {
			log.Printf("DEBUG: Service provider was not created by a salesperson (CreatedBy is zero or not set)")
		}
		// --- Commission logic end ---

		// Send approval notification
		emailSubject := "Service Provider Subscription Request Approved! 🎉"
		log.Printf("Service provider subscription approved notification: %s - %s", serviceProvider.BusinessName, emailSubject)

		emailBody := fmt.Sprintf(`
			Great news! Your subscription request has been approved.
			
			Subscription Details:
			- Plan: %s
			- Duration: %d months
			- Price: $%.2f
			- Start Date: %s
			- End Date: %s
			- Status: Active
			
			Your subscription is now active and you can enjoy all the benefits of your chosen plan.
			
			Thank you for choosing our services!
		`,
			plan.Title,
			plan.Duration,
			plan.Price,
			startDate.Format("2006-01-02"),
			endDate.Format("2006-01-02"),
		)

		if err := spc.sendServiceProviderNotificationEmail(serviceProvider.ContactInfo.Phone, emailSubject, emailBody); err != nil {
			log.Printf("Failed to send approval notification email: %v", err)
		}
	} else {
		// If rejected, send rejection notification
		emailSubject := "Service Provider Subscription Request Update"
		log.Printf("Service provider subscription rejected notification: %s - %s", serviceProvider.BusinessName, emailSubject)

		emailBody := fmt.Sprintf(`
			Your subscription request has been reviewed.
			
			Status: Rejected
			Plan: %s ($%.2f for %d months)
			
			Reason: %s
			
			If you have any questions, please contact our support team.
		`,
			plan.Title,
			plan.Price,
			plan.Duration,
			approvalReq.AdminNote,
		)

		// Update user status to inactive when subscription is rejected
		usersCollection := spc.DB.Collection("users")
		_, err = usersCollection.UpdateOne(
			ctx,
			bson.M{"serviceProviderId": subscriptionRequest.ServiceProviderID},
			bson.M{"$set": bson.M{"status": "inactive", "updatedAt": time.Now()}},
		)
		if err != nil {
			log.Printf("Failed to update user status: %v", err)
		}

		if err := spc.sendServiceProviderNotificationEmail(serviceProvider.ContactInfo.Phone, emailSubject, emailBody); err != nil {
			log.Printf("Failed to send rejection notification email: %v", err)
		}
	}

	// Prepare response data
	responseData := map[string]interface{}{
		"requestId":           subscriptionRequest.ID,
		"serviceProviderName": serviceProvider.BusinessName,
		"planName":            plan.Title,
		"status":              approvalReq.Status,
		"processedAt":         time.Now(),
		"adminNote":           approvalReq.AdminNote,
		"subscription":        subscription,
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: fmt.Sprintf("Service provider subscription request %s successfully", approvalReq.Status),
		Data:    responseData,
	})
}

// sendServiceProviderNotificationEmail sends a notification email to a service provider
func (spc *ServiceProviderSubscriptionController) sendServiceProviderNotificationEmail(phone, subject, body string) error {
	// TODO: Implement actual email sending to service provider
	// For now, just log the notification
	log.Printf("Service provider notification (to %s): %s - %s", phone, subject, body)
	return nil
}

// Helper function to save uploaded files
func (spc *ServiceProviderSubscriptionController) saveUploadedFile(file *multipart.FileHeader, directory string) (string, error) {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(directory, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	// Generate unique filename
	ext := filepath.Ext(file.Filename)
	filename := primitive.NewObjectID().Hex() + ext
	filepath := filepath.Join(directory, filename)

	// Open source file
	src, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("failed to open uploaded file: %w", err)
	}
	defer src.Close()

	// Create destination file
	dst, err := os.Create(filepath)
	if err != nil {
		return "", fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dst.Close()

	// Copy file contents
	if _, err = io.Copy(dst, src); err != nil {
		return "", fmt.Errorf("failed to copy file contents: %w", err)
	}

	return filepath, nil
}

// Helper function to send admin notification email
func (spc *ServiceProviderSubscriptionController) sendAdminNotificationEmail(subject, body string) error {
	adminEmail := os.Getenv("ADMIN_EMAIL")
	if adminEmail == "" {
		log.Println("Admin email not configured for notifications")
		return fmt.Errorf("admin email not configured")
	}

	// Check if SMTP environment variables are set
	smtpHost := os.Getenv("SMTP_HOST")
	smtpUser := os.Getenv("SMTP_USER")
	smtpPass := os.Getenv("SMTP_PASS")
	fromEmail := os.Getenv("FROM_EMAIL")

	// Use FROM_EMAIL as sender, if not available, fall back to SMTP_USER
	senderEmail := fromEmail
	if senderEmail == "" {
		senderEmail = smtpUser
	}

	if smtpHost == "" || smtpUser == "" || smtpPass == "" || senderEmail == "" {
		log.Println("SMTP configuration is incomplete for notifications")
		return fmt.Errorf("SMTP configuration is incomplete: check SMTP_HOST, SMTP_USER, SMTP_PASS, and FROM_EMAIL")
	}

	// Get SMTP port from environment or default to 2525
	smtpPortStr := os.Getenv("SMTP_PORT")
	smtpPort := 2525 // Default port
	if smtpPortStr != "" {
		portNum, err := strconv.Atoi(smtpPortStr)
		if err == nil && portNum > 0 {
			smtpPort = portNum
		}
	}

	m := gomail.NewMessage()
	m.SetHeader("From", senderEmail)
	m.SetHeader("To", adminEmail)
	m.SetHeader("Subject", subject)
	m.SetBody("text/plain", body)

	d := gomail.NewDialer(
		smtpHost,
		smtpPort,
		smtpUser,
		smtpPass,
	)

	// Try to send the email
	err := d.DialAndSend(m)
	if err != nil {
		log.Printf("Failed to send notification email: %v", err)
		return fmt.Errorf("failed to send email: %w", err)
	}

	log.Printf("Admin notification email sent successfully for subject: %s", subject)
	return nil
}

// Generic email notification helper
func (spc *ServiceProviderSubscriptionController) sendNotificationEmail(to, subject, body string) error {
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
func (spc *ServiceProviderSubscriptionController) ApproveServiceProviderSubscriptionRequest(c echo.Context) error {
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

	subscriptionRequestsCollection := spc.DB.Collection("subscription_requests")
	var request models.SubscriptionRequest
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

	// Set service provider status to approved
	if !request.ServiceProviderID.IsZero() {
		spCollection := spc.DB.Collection("serviceProviders")
		_, err = spCollection.UpdateOne(ctx, bson.M{"_id": request.ServiceProviderID}, bson.M{"$set": bson.M{"status": "active"}})
		if err != nil {
			log.Printf("Failed to update service provider status to active: %v", err)
		}

		// --- Commission logic start ---
		// Get service provider details
		var serviceProvider models.ServiceProvider
		if err := spCollection.FindOne(ctx, bson.M{"_id": request.ServiceProviderID}).Decode(&serviceProvider); err == nil {
			if !serviceProvider.CreatedBy.IsZero() {
				// Get salesperson
				var salesperson models.Salesperson
				err := spc.DB.Collection("salespersons").FindOne(ctx, bson.M{"_id": serviceProvider.CreatedBy}).Decode(&salesperson)
				if err == nil {
					// Get sales manager
					var salesManager models.SalesManager
					err := spc.DB.Collection("sales_managers").FindOne(ctx, bson.M{"_id": salesperson.SalesManagerID}).Decode(&salesManager)
					if err == nil {
						// Get plan details
						var plan models.SubscriptionPlan
						err := spc.DB.Collection("subscription_plans").FindOne(ctx, bson.M{"_id": request.PlanID}).Decode(&plan)
						if err == nil {
							planPrice := plan.Price
							salespersonPercent := salesperson.CommissionPercent
							salesManagerPercent := salesManager.CommissionPercent
							// Calculate commissions correctly
							// Salesperson gets their percentage directly from the plan price
							salespersonCommission := planPrice * salespersonPercent / 100.0
							// Sales manager gets their percentage directly from the plan price
							salesManagerCommission := planPrice * salesManagerPercent / 100.0
							// Insert commission documents
							commissionCollection := spc.DB.Collection("commissions")
							commissionDocs := []interface{}{
								models.Commission{
									ID:                            primitive.NewObjectID(),
									SubscriptionID:                request.ID,
									CompanyID:                     primitive.NilObjectID,
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
								},
							}
							_, err := commissionCollection.InsertMany(ctx, commissionDocs)
							if err != nil {
								log.Printf("Failed to insert commission docs: %v", err)
							}
						}
					}
				}
			}
		}
		// --- Commission logic end ---
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Subscription request approved and service provider status updated.",
	})
}

// Admin reject handler
func (spc *ServiceProviderSubscriptionController) RejectServiceProviderSubscriptionRequest(c echo.Context) error {
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

	subscriptionRequestsCollection := spc.DB.Collection("subscription_requests")
	var request models.SubscriptionRequest
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

	// Set service provider status to inactive
	if !request.ServiceProviderID.IsZero() {
		spCollection := spc.DB.Collection("serviceProviders")
		_, err = spCollection.UpdateOne(ctx, bson.M{"_id": request.ServiceProviderID}, bson.M{"$set": bson.M{"status": "inactive"}})
		if err != nil {
			log.Printf("Failed to update service provider status to inactive: %v", err)
		}
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Subscription request rejected.",
	})
}

// Manager approval handler
func (spc *ServiceProviderSubscriptionController) ApproveServiceProviderSubscriptionRequestByManager(c echo.Context) error {
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

	subscriptionRequestsCollection := spc.DB.Collection("subscription_requests")
	var request models.SubscriptionRequest
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
			"status":          "approved",
			"managerApproved": &approved,
			"approvedBy":      "manager",
			"approvedAt":      now,
			"processedAt":     now,
		},
	}
	_, err = subscriptionRequestsCollection.UpdateOne(ctx, bson.M{"_id": objID}, update)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update subscription request",
		})
	}

	// Set service provider status to approved
	if !request.ServiceProviderID.IsZero() {
		spCollection := spc.DB.Collection("serviceProviders")
		_, err = spCollection.UpdateOne(ctx, bson.M{"_id": request.ServiceProviderID}, bson.M{"$set": bson.M{"status": "approved"}})
		if err != nil {
			log.Printf("Failed to update service provider status: %v", err)
		}

		// --- Commission logic start ---
		// Get service provider details
		var serviceProvider models.ServiceProvider
		if err := spCollection.FindOne(ctx, bson.M{"_id": request.ServiceProviderID}).Decode(&serviceProvider); err == nil {
			// Check if service provider was created by itself (user signup) - CreatedBy equals UserID
			if serviceProvider.CreatedBy == serviceProvider.UserID {
				log.Printf("DEBUG: Service provider was created by user signup, adding subscription price to admin wallet")
				// Get plan details to get the price
				var plan models.SubscriptionPlan
				err := spc.DB.Collection("subscription_plans").FindOne(ctx, bson.M{"_id": request.PlanID}).Decode(&plan)
				if err == nil {
					log.Printf("Subscription income added to admin wallet: $%.2f from service provider '%s' (ID: %s) - User signup subscription",
						plan.Price, serviceProvider.BusinessName, serviceProvider.ID.Hex())
				}
			} else if !serviceProvider.CreatedBy.IsZero() {
				// Get salesperson
				var salesperson models.Salesperson
				err := spc.DB.Collection("salespersons").FindOne(ctx, bson.M{"_id": serviceProvider.CreatedBy}).Decode(&salesperson)
				if err == nil {
					// Get sales manager
					var salesManager models.SalesManager
					err := spc.DB.Collection("sales_managers").FindOne(ctx, bson.M{"_id": salesperson.SalesManagerID}).Decode(&salesManager)
					if err == nil {
						// Get plan details
						var plan models.SubscriptionPlan
						err := spc.DB.Collection("subscription_plans").FindOne(ctx, bson.M{"_id": request.PlanID}).Decode(&plan)
						if err == nil {
							planPrice := plan.Price
							salespersonPercent := salesperson.CommissionPercent
							salesManagerPercent := salesManager.CommissionPercent
							// Calculate commissions correctly
							// Salesperson gets their percentage directly from the plan price
							salespersonCommission := planPrice * salespersonPercent / 100.0
							// Sales manager gets their percentage directly from the plan price
							salesManagerCommission := planPrice * salesManagerPercent / 100.0
							// Insert commission documents
							commissionCollection := spc.DB.Collection("commissions")
							commissionDocs := []interface{}{
								models.Commission{
									ID:                            primitive.NewObjectID(),
									SubscriptionID:                request.ID,
									CompanyID:                     primitive.NilObjectID,
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
								},
							}
							_, err := commissionCollection.InsertMany(ctx, commissionDocs)
							if err != nil {
								log.Printf("Failed to insert commission docs: %v", err)
							}
						}
					}
				}
			}
		}
		// --- Commission logic end ---
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Subscription request approved by manager and service provider status updated.",
	})
}

// Manager reject handler
func (spc *ServiceProviderSubscriptionController) RejectServiceProviderSubscriptionRequestByManager(c echo.Context) error {
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

	subscriptionRequestsCollection := spc.DB.Collection("subscription_requests")
	var request models.SubscriptionRequest
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
			"status":          "rejected",
			"managerApproved": &rejected,
			"rejectedBy":      "manager",
			"rejectedAt":      now,
			"processedAt":     now,
		},
	}
	_, err = subscriptionRequestsCollection.UpdateOne(ctx, bson.M{"_id": objID}, update)
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

// CreateServiceProviderSponsorshipRequest allows service providers to create sponsorship requests
func (spc *ServiceProviderSubscriptionController) CreateServiceProviderSponsorshipRequest(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user information from token
	claims := middleware.GetUserFromToken(c)
	if claims == nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Authentication required",
		})
	}

	if claims.UserType != "serviceProvider" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only service providers can create sponsorship requests",
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

	// Get service provider by user ID
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	serviceProvider, err := spc.findServiceProviderByUserID(ctx, userID)
	if err != nil {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: fmt.Sprintf("Service provider not found: %v", err),
		})
	}

	// Check if sponsorship exists and is valid
	sponsorshipCollection := spc.DB.Collection("sponsorships")
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

	// Check if there's already a pending or active subscription for this service provider and sponsorship
	subscriptionCollection := spc.DB.Collection("sponsorship_subscriptions")
	var existingSubscription models.SponsorshipSubscription
	err = subscriptionCollection.FindOne(ctx, bson.M{
		"sponsorshipId": req.SponsorshipID,
		"entityId":      serviceProvider.ID,
		"entityType":    "serviceProvider",
		"status":        bson.M{"$in": []string{"active", "pending"}},
	}).Decode(&existingSubscription)
	if err == nil {
		return c.JSON(http.StatusConflict, models.Response{
			Status:  http.StatusConflict,
			Message: "This service provider already has a pending or active subscription for this sponsorship",
		})
	}

	// Check if there's already a pending request for this service provider and sponsorship
	existingRequestCollection := spc.DB.Collection("sponsorship_subscription_requests")
	var existingRequest models.SponsorshipSubscriptionRequest
	err = existingRequestCollection.FindOne(ctx, bson.M{
		"sponsorshipId": req.SponsorshipID,
		"entityId":      serviceProvider.ID,
		"entityType":    "serviceProvider",
		"status":        "pending",
	}).Decode(&existingRequest)
	if err == nil {
		return c.JSON(http.StatusConflict, models.Response{
			Status:  http.StatusConflict,
			Message: "This service provider already has a pending subscription request for this sponsorship",
		})
	}

	// Create sponsorship subscription request
	subscriptionRequest := models.SponsorshipSubscriptionRequest{
		ID:            primitive.NewObjectID(),
		SponsorshipID: req.SponsorshipID,
		EntityType:    "serviceProvider",
		EntityID:      serviceProvider.ID,
		EntityName:    serviceProvider.BusinessName,
		Status:        "pending",
		RequestedAt:   time.Now(),
		AdminNote:     req.AdminNote,
	}

	// Save the sponsorship subscription request
	_, err = existingRequestCollection.InsertOne(ctx, subscriptionRequest)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to create sponsorship subscription request",
		})
	}

	// Send notification to admin (optional)
	adminEmail := os.Getenv("ADMIN_EMAIL")
	if adminEmail != "" {
		subject := "New Service Provider Sponsorship Request"
		body := fmt.Sprintf("A new sponsorship request has been submitted by service provider: %s\nSponsorship: %s\nRequested At: %s\n",
			serviceProvider.BusinessName, sponsorship.Title, subscriptionRequest.RequestedAt.Format("2006-01-02 15:04:05"))

		// For now, just log the notification since we don't have email functionality in service provider controller
		log.Printf("Admin notification (to %s): %s - %s", adminEmail, subject, body)
	}

	return c.JSON(http.StatusCreated, models.Response{
		Status:  http.StatusCreated,
		Message: "Sponsorship subscription request created successfully. Waiting for admin approval.",
		Data: map[string]interface{}{
			"requestId":   subscriptionRequest.ID,
			"sponsorship": sponsorship,
			"serviceProvider": map[string]interface{}{
				"id":           serviceProvider.ID,
				"businessName": serviceProvider.BusinessName,
				"category":     serviceProvider.Category,
			},
			"status":      subscriptionRequest.Status,
			"submittedAt": subscriptionRequest.RequestedAt,
			"adminNote":   subscriptionRequest.AdminNote,
		},
	})
}

// GetServiceProviderSponsorshipRemainingTime gets the remaining time for a service provider's sponsorship subscription
func (spc *ServiceProviderSubscriptionController) GetServiceProviderSponsorshipRemainingTime(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user information from token
	claims := middleware.GetUserFromToken(c)
	if claims == nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Authentication required",
		})
	}

	if claims.UserType != "serviceProvider" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only service providers can access this endpoint",
		})
	}

	// Get service provider by user ID to verify ownership
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	serviceProvider, err := spc.findServiceProviderByUserID(ctx, userID)
	if err != nil {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: fmt.Sprintf("Service provider not found: %v", err),
		})
	}

	// Find active sponsorship subscription for this service provider
	collection := spc.DB.Collection("sponsorship_subscriptions")
	var subscription models.SponsorshipSubscription
	err = collection.FindOne(ctx, bson.M{
		"entityType": "serviceProvider",
		"entityId":   serviceProvider.ID,
		"status":     "active",
	}).Decode(&subscription)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusOK, models.Response{
				Status:  http.StatusOK,
				Message: "No active sponsorship subscription found for this service provider",
				Data: map[string]interface{}{
					"hasActiveSubscription": false,
					"message":               "No active sponsorship subscription found",
				},
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve subscription",
		})
	}

	// Calculate time remaining
	timeRemaining := subscription.EndDate.Sub(time.Now())

	// Check if subscription has expired
	if timeRemaining <= 0 {
		return c.JSON(http.StatusOK, models.Response{
			Status:  http.StatusOK,
			Message: "Sponsorship subscription has expired",
			Data: map[string]interface{}{
				"hasActiveSubscription": false,
				"message":               "Subscription has expired",
				"subscription":          subscription,
			},
		})
	}

	// Get sponsorship details
	sponsorshipCollection := spc.DB.Collection("sponsorships")
	var sponsorship models.Sponsorship
	err = sponsorshipCollection.FindOne(ctx, bson.M{"_id": subscription.SponsorshipID}).Decode(&sponsorship)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve sponsorship details",
		})
	}

	// Format time remaining
	days := int(timeRemaining.Hours() / 24)
	hours := int(timeRemaining.Hours()) % 24
	minutes := int(timeRemaining.Minutes()) % 60
	seconds := int(timeRemaining.Seconds()) % 60
	formattedTime := fmt.Sprintf("%dd %dh %dm %ds", days, hours, minutes, seconds)

	// Calculate percentage used
	totalDuration := subscription.EndDate.Sub(subscription.StartDate)
	usedDuration := time.Now().Sub(subscription.StartDate)
	percentageUsed := (usedDuration.Seconds() / totalDuration.Seconds()) * 100

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Service provider sponsorship subscription remaining time retrieved successfully",
		Data: map[string]interface{}{
			"hasActiveSubscription": true,
			"timeRemaining": map[string]interface{}{
				"days":           days,
				"hours":          hours,
				"minutes":        minutes,
				"seconds":        seconds,
				"formatted":      formattedTime,
				"percentageUsed": fmt.Sprintf("%.1f%%", percentageUsed),
				"startDate":      subscription.StartDate.Format(time.RFC3339),
				"endDate":        subscription.EndDate.Format(time.RFC3339),
			},
			"subscription": subscription,
			"sponsorship":  sponsorship,
			"entityInfo": map[string]interface{}{
				"serviceProviderName": serviceProvider.BusinessName,
				"category":            serviceProvider.Category,
			},
		},
	})
}
