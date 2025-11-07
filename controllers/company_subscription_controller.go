// controllers/company_subscription_controller.go
package controllers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"io"
	"mime/multipart"
	"path/filepath"

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

// BranchSubscriptionController handles branch subscription operations
// (was CompanySubscriptionController)
type BranchSubscriptionController struct {
	DB *mongo.Database
}

// NewBranchSubscriptionController creates a new branch subscription controller
func NewBranchSubscriptionController(db *mongo.Database) *BranchSubscriptionController {
	return &BranchSubscriptionController{DB: db}
}

// GetCompanySubscriptionPlans retrieves all available subscription plans for companies
// GetCompanySubscriptionPlans retrieves all available subscription plans for companies
func (sc *SubscriptionController) GetCompanySubscriptionPlans(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	collection := sc.DB.Collection("subscription_plans")
	cursor, err := collection.Find(ctx, bson.M{
		"type":     "company",
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

	if len(plans) == 0 {
		return c.JSON(http.StatusOK, models.Response{
			Status:  http.StatusOK,
			Message: "No subscription plans found",
			Data:    []models.SubscriptionPlan{},
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Subscription plans retrieved successfully",
		Data:    plans,
	})
}

// CreateBranchSubscriptionRequest creates a new subscription request for a branch
func (sc *BranchSubscriptionController) CreateBranchSubscriptionRequest(c echo.Context) error {
	log.Printf("DEBUG: CreateBranchSubscriptionRequest called with path: %s", c.Request().URL.Path)
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

	// Get plan ID from form and branch ID from URL parameter
	planID := c.FormValue("planId")
	branchID := c.Param("branchId")
	if planID == "" || branchID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Plan ID and Branch ID are required",
		})
	}

	// Convert plan ID and branch ID to ObjectID
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

	// Find company by user ID and get the specific branch
	companyCollection := sc.DB.Collection("companies")
	var company models.Company
	err = companyCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&company)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Company not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find company",
		})
	}

	// Find the specific branch within the company
	var branch models.Branch
	branchFound := false
	for _, b := range company.Branches {
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
	subscriptionRequestsCollection := sc.DB.Collection("branch_subscription_requests")
	var existingRequest models.BranchSubscriptionRequest
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
	subscriptionsCollection := sc.DB.Collection("branch_subscriptions")
	var activeSubscription models.BranchSubscription
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

	// Create branch subscription request
	subscriptionRequest := models.BranchSubscriptionRequest{
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
	// This validates that your Whish merchant account is operational
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
		Invoice:            fmt.Sprintf("Branch Subscription - %s - Plan: %s", branch.Name, plan.Title),
		ExternalID:         &externalID,
		SuccessCallbackURL: fmt.Sprintf("%s/api/whish/payment/callback/success", baseURL),
		FailureCallbackURL: fmt.Sprintf("%s/api/whish/payment/callback/failure", baseURL),
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

	log.Printf("Whish payment created for branch subscription request %s: %s", subscriptionRequest.ID.Hex(), collectURL)

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

// HandleWhishPaymentSuccess handles Whish payment success callback
func (sc *BranchSubscriptionController) HandleWhishPaymentSuccess(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log.Printf("==========================================")
	log.Printf("💰 BRANCH SUBSCRIPTION PAYMENT CALLBACK RECEIVED")
	log.Printf("==========================================")

	// Get externalId from query parameters (Whish sends it as GET parameter)
	externalIDStr := c.QueryParam("externalId")
	if externalIDStr == "" {
		log.Printf("❌ PAYMENT CALLBACK FAILED: Missing externalId in Whish success callback")
		return c.String(http.StatusBadRequest, "Missing externalId parameter")
	}

	externalID, err := strconv.ParseInt(externalIDStr, 10, 64)
	if err != nil {
		log.Printf("❌ PAYMENT CALLBACK FAILED: Invalid externalId in callback: %v", err)
		return c.String(http.StatusBadRequest, "Invalid externalId")
	}

	log.Printf("📋 Processing payment callback for externalId: %d", externalID)

	// Find the subscription request by externalId
	subscriptionRequestsCollection := sc.DB.Collection("branch_subscription_requests")
	var subscriptionRequest models.BranchSubscriptionRequest
	err = subscriptionRequestsCollection.FindOne(ctx, bson.M{"externalId": externalID}).Decode(&subscriptionRequest)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			log.Printf("❌ PAYMENT CALLBACK FAILED: Subscription request not found for externalId: %d", externalID)
			return c.String(http.StatusNotFound, "Subscription request not found")
		}
		log.Printf("❌ PAYMENT CALLBACK FAILED: Error finding subscription request: %v", err)
		return c.String(http.StatusInternalServerError, "Database error")
	}

	log.Printf("✅ Found subscription request: %s (Branch: %s)", subscriptionRequest.ID.Hex(), subscriptionRequest.BranchID.Hex())

	// Check if already processed
	if subscriptionRequest.PaymentStatus == "success" || subscriptionRequest.Status == "active" {
		log.Printf("ℹ️  Payment already processed for request: %s", subscriptionRequest.ID.Hex())
		return c.String(http.StatusOK, "Payment already processed")
	}

	// Initialize Whish service and verify payment status
	whishService := services.NewWhishService()
	status, phoneNumber, err := whishService.GetPaymentStatus("USD", externalID)
	if err != nil {
		log.Printf("❌ PAYMENT CALLBACK FAILED: Failed to verify payment status: %v", err)
		return c.String(http.StatusInternalServerError, "Failed to verify payment")
	}

	log.Printf("📊 Payment status from Whish API: %s (Phone: %s)", status, phoneNumber)

	if status != "success" {
		log.Printf("❌ PAYMENT FAILED: Payment not successful, status: %s", status)
		log.Printf("   Request ID: %s", subscriptionRequest.ID.Hex())
		// Update request status to failed
		subscriptionRequestsCollection.UpdateOne(ctx,
			bson.M{"_id": subscriptionRequest.ID},
			bson.M{"$set": bson.M{
				"paymentStatus": "failed",
				"status":        "failed",
				"processedAt":   time.Now(),
			}})
		log.Printf("==========================================")
		return c.String(http.StatusBadRequest, "Payment not successful")
	}

	// Payment verified successfully - proceed to activate subscription
	log.Printf("🔄 Activating branch subscription...")
	err = sc.activateBranchSubscription(ctx, subscriptionRequest, phoneNumber)
	if err != nil {
		log.Printf("❌ PAYMENT CALLBACK FAILED: Failed to activate subscription: %v", err)
		return c.String(http.StatusInternalServerError, "Failed to activate subscription")
	}

	log.Printf("✅ PAYMENT SUCCESS: Branch subscription payment completed and activated")
	log.Printf("   Request ID: %s", subscriptionRequest.ID.Hex())
	log.Printf("   External ID: %d", externalID)
	log.Printf("   Branch ID: %s", subscriptionRequest.BranchID.Hex())
	log.Printf("   Phone: %s", phoneNumber)
	log.Printf("==========================================")
	return c.String(http.StatusOK, "Payment successful and subscription activated")
}

// HandleWhishPaymentFailure handles Whish payment failure callback
func (sc *BranchSubscriptionController) HandleWhishPaymentFailure(c echo.Context) error {
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
	subscriptionRequestsCollection := sc.DB.Collection("branch_subscription_requests")
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

// activateBranchSubscription activates the subscription after successful payment
func (sc *BranchSubscriptionController) activateBranchSubscription(ctx context.Context, subscriptionRequest models.BranchSubscriptionRequest, payerPhone string) error {
	// Get plan details
	var plan models.SubscriptionPlan
	err := sc.DB.Collection("subscription_plans").FindOne(ctx, bson.M{"_id": subscriptionRequest.PlanID}).Decode(&plan)
	if err != nil {
		log.Printf("Failed to get plan: %v", err)
		return fmt.Errorf("failed to get plan details")
	}

	// Get company and branch details
	var company models.Company
	err = sc.DB.Collection("companies").FindOne(ctx, bson.M{"branches._id": subscriptionRequest.BranchID}).Decode(&company)
	if err != nil {
		log.Printf("Failed to get company: %v", err)
		return fmt.Errorf("failed to get company details")
	}

	// Find the specific branch
	var branch models.Branch
	branchFound := false
	for _, b := range company.Branches {
		if b.ID == subscriptionRequest.BranchID {
			branch = b
			branchFound = true
			break
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
	newSubscription := models.BranchSubscription{
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
	subscriptionsCollection := sc.DB.Collection("branch_subscriptions")
	_, err = subscriptionsCollection.InsertOne(ctx, newSubscription)
	if err != nil {
		log.Printf("Failed to create subscription: %v", err)
		return fmt.Errorf("failed to create subscription")
	}

	// Update branch status to active
	branchCollection := sc.DB.Collection("branches")
	_, err = branchCollection.UpdateOne(ctx, bson.M{"_id": subscriptionRequest.BranchID}, bson.M{"$set": bson.M{"status": "active"}})
	if err != nil {
		log.Printf("Failed to update branch status: %v", err)
	}

	// Update branch status in companies collection
	_, err = sc.DB.Collection("companies").UpdateOne(
		ctx,
		bson.M{
			"_id":          company.ID,
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
		log.Printf("Failed to update branch status in companies: %v", err)
	}

	// Update user status to active when subscription is activated
	usersCollection := sc.DB.Collection("users")
	_, err = usersCollection.UpdateOne(
		ctx,
		bson.M{"_id": company.UserID},
		bson.M{"$set": bson.M{"status": "active", "updatedAt": time.Now()}},
	)
	if err != nil {
		log.Printf("Failed to update user status to active: %v", err)
	}

	// Handle commission and admin wallet (30% salesperson, 70% admin)
	planPrice := plan.Price
	if company.CreatedBy != company.UserID && !company.CreatedBy.IsZero() {
		// Company was created by a salesperson - split commission
		var salesperson models.Salesperson
		err := sc.DB.Collection("salespersons").FindOne(ctx, bson.M{"_id": company.CreatedBy}).Decode(&salesperson)
		if err == nil {
			// Calculate commissions: 30% salesperson, 70% admin
			salespersonPercent := 30.0
			adminPercent := 70.0

			salespersonCommission := planPrice * salespersonPercent / 100.0
			adminCommission := planPrice * adminPercent / 100.0

			// Add salesperson commission (using SubscriptionController method through type assertion/conversion)
			// We need to access the method - let's create a helper or use direct implementation
			err = sc.addCommissionToSalespersonWalletDirect(ctx, salesperson.ID, salespersonCommission, newSubscription.ID, "branch_subscription", company.BusinessName)
			if err != nil {
				log.Printf("Failed to add salesperson commission: %v", err)
			} else {
				log.Printf("Added salesperson commission: $%.2f (30%% of $%.2f)", salespersonCommission, planPrice)
			}

			// Add admin commission
			err = sc.addSubscriptionIncomeToAdminWalletDirect(ctx, adminCommission, newSubscription.ID, "branch_subscription_commission", company.BusinessName, branch.Name)
			if err != nil {
				log.Printf("Failed to add admin commission: %v", err)
			} else {
				log.Printf("Added admin commission: $%.2f (70%% of $%.2f)", adminCommission, planPrice)
			}
		} else {
			log.Printf("Salesperson not found, adding full amount to admin wallet")
			// Salesperson not found, add full amount to admin
			err = sc.addSubscriptionIncomeToAdminWalletDirect(ctx, planPrice, newSubscription.ID, "branch_subscription", company.BusinessName, branch.Name)
			if err != nil {
				log.Printf("Failed to add subscription income to admin wallet: %v", err)
			}
		}
	} else {
		// Company created by itself - full amount to admin
		err = sc.addSubscriptionIncomeToAdminWalletDirect(ctx, planPrice, newSubscription.ID, "branch_subscription", company.BusinessName, branch.Name)
		if err != nil {
			log.Printf("Failed to add subscription income to admin wallet: %v", err)
		}
	}

	// Update subscription request status
	subscriptionRequestsCollection := sc.DB.Collection("branch_subscription_requests")
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

	log.Printf("Branch subscription activated successfully: Branch=%s, Plan=%s, Amount=$%.2f", branch.Name, plan.Title, planPrice)
	return nil
}

// Helper methods for wallet management (direct implementations for BranchSubscriptionController)
func (sc *BranchSubscriptionController) addSubscriptionIncomeToAdminWalletDirect(ctx context.Context, amount float64, entityID primitive.ObjectID, entityType, companyName, branchName string) error {
	adminWalletTransaction := models.AdminWallet{
		ID:          primitive.NewObjectID(),
		Type:        "subscription_income",
		Amount:      amount,
		Description: fmt.Sprintf("Subscription income from %s - %s (%s)", companyName, branchName, entityType),
		EntityID:    entityID,
		EntityType:  entityType,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	_, err := sc.DB.Collection("admin_wallet").InsertOne(ctx, adminWalletTransaction)
	if err != nil {
		return fmt.Errorf("failed to insert admin wallet transaction: %w", err)
	}

	balanceCollection := sc.DB.Collection("admin_wallet_balance")
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

func (sc *BranchSubscriptionController) addCommissionToSalespersonWalletDirect(ctx context.Context, salespersonID primitive.ObjectID, amount float64, subscriptionID primitive.ObjectID, entityType, companyName string) error {
	// Note: CommissionRecord model might need to be checked - using similar structure
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
		amount, salespersonID.Hex(), entityType, companyName)
	return nil
}

func (sc *SubscriptionController) CancelCompanySubscription(c echo.Context) error {
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

	// Find company by user ID
	companyCollection := sc.DB.Collection("companies")
	var company models.Company
	err = companyCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&company)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Company not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find company",
		})
	}

	// Get all branch IDs for this company
	branchIDs := make([]primitive.ObjectID, len(company.Branches))
	for i, branch := range company.Branches {
		branchIDs[i] = branch.ID
	}

	if len(branchIDs) == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "No branches found for this company",
		})
	}

	// Find and update active subscriptions for all branches
	subscriptionsCollection := sc.DB.Collection("branch_subscriptions")
	update := bson.M{
		"$set": bson.M{
			"status":    "cancelled",
			"autoRenew": false,
			"updatedAt": time.Now(),
		},
	}

	result, err := subscriptionsCollection.UpdateMany(ctx, bson.M{
		"branchId": bson.M{"$in": branchIDs},
		"status":   "active",
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

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Subscription cancelled successfully",
	})
}

// GetCompanySubscriptionRequestStatus gets the status of a company's subscription request
func (sc *SubscriptionController) GetCompanySubscriptionRequestStatus(c echo.Context) error {
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

	// Find company by user ID
	companyCollection := sc.DB.Collection("companies")
	var company models.Company
	err = companyCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&company)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Company not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find company",
		})
	}

	// Find the latest subscription request
	subscriptionRequestsCollection := sc.DB.Collection("subscription_requests")
	var subscriptionRequest models.SubscriptionRequest
	err = subscriptionRequestsCollection.FindOne(ctx,
		bson.M{"companyId": company.ID},
		options.FindOne().SetSort(bson.D{{"requestedAt", -1}}),
	).Decode(&subscriptionRequest)

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

	// Get plan details
	var plan models.SubscriptionPlan
	err = sc.DB.Collection("subscription_plans").FindOne(ctx, bson.M{"_id": subscriptionRequest.PlanID}).Decode(&plan)
	if err != nil {
		log.Printf("Failed to get plan details: %v", err)
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Subscription request status retrieved successfully",
		Data: map[string]interface{}{
			"hasRequest":  true,
			"request":     subscriptionRequest,
			"plan":        plan,
			"companyName": company.BusinessName,
		},
	})
}

// ApproveCompanySubscriptionRequest allows an admin to approve a company subscription request
func (sc *SubscriptionController) ApproveCompanySubscriptionRequest(c echo.Context) error {
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

	subscriptionRequestsCollection := sc.DB.Collection("subscription_requests")
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

	// Set company status to approved
	if !request.CompanyID.IsZero() {
		companyCollection := sc.DB.Collection("companies")
		_, err = companyCollection.UpdateOne(ctx, bson.M{"_id": request.CompanyID}, bson.M{"$set": bson.M{"status": "active"}})
		if err != nil {
			log.Printf("Failed to update company status: %v", err)
		}

		// Find branchId from branch_subscription_requests
		branchSubReqColl := sc.DB.Collection("branch_subscription_requests")
		var branchSubReq models.BranchSubscriptionRequest
		err = branchSubReqColl.FindOne(ctx, bson.M{"planId": request.PlanID, "status": "pending", "companyId": request.CompanyID}).Decode(&branchSubReq)
		if err == nil && !branchSubReq.BranchID.IsZero() {
			_, err = companyCollection.UpdateOne(
				ctx,
				bson.M{"_id": request.CompanyID, "branches._id": branchSubReq.BranchID},
				bson.M{"$set": bson.M{"branches.$.status": "active"}},
			)
			if err != nil {
				log.Printf("Failed to update branch status to active: %v", err)
			}
		}

		// --- Commission logic start ---
		// Only proceed if company was created by a salesperson
		if !request.CompanyID.IsZero() {
			// Get company details first
			var company models.Company
			err := sc.DB.Collection("companies").FindOne(ctx, bson.M{"_id": request.CompanyID}).Decode(&company)
			if err == nil && !company.CreatedBy.IsZero() {
				// Get salesperson
				var salesperson models.Salesperson
				err := sc.DB.Collection("salespersons").FindOne(ctx, bson.M{"_id": company.CreatedBy}).Decode(&salesperson)
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

							// Insert commission document using Commission model
							commission := models.Commission{
								ID:                           primitive.NewObjectID(),
								SubscriptionID:               request.ID,
								CompanyID:                    request.CompanyID,
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
							_, err := sc.DB.Collection("commissions").InsertOne(ctx, commission)
							if err != nil {
								log.Printf("Failed to insert commission: %v", err)
							} else {
								log.Printf("Commission inserted successfully - Plan Price: $%.2f, Admin Commission: $%.2f (%.1f%%), Salesperson Commission: $%.2f (%.1f%%)",
									planPrice, adminCommission, adminPercent, salespersonCommission, salespersonPercent)
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
		Message: "Subscription request approved and company status updated.",
	})
}

// RejectCompanySubscriptionRequest allows an admin to reject a company subscription request
func (sc *SubscriptionController) RejectCompanySubscriptionRequest(c echo.Context) error {
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

	subscriptionRequestsCollection := sc.DB.Collection("subscription_requests")
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

	// Find branchId from branch_subscription_requests for rejection
	if !request.CompanyID.IsZero() {
		companyCollection := sc.DB.Collection("companies")
		branchSubReqColl := sc.DB.Collection("branch_subscription_requests")
		var branchSubReq models.BranchSubscriptionRequest
		err = branchSubReqColl.FindOne(ctx, bson.M{"planId": request.PlanID, "status": "pending", "companyId": request.CompanyID}).Decode(&branchSubReq)
		if err == nil && !branchSubReq.BranchID.IsZero() {
			_, err = companyCollection.UpdateOne(
				ctx,
				bson.M{"_id": request.CompanyID, "branches._id": branchSubReq.BranchID},
				bson.M{"$set": bson.M{"branches.$.status": "inactive"}},
			)
			if err != nil {
				log.Printf("Failed to update branch status to inactive: %v", err)
			}
		}
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Subscription request rejected.",
	})
}

// ApproveCompanySubscriptionRequestByManager allows a manager to approve a company subscription request
func (sc *SubscriptionController) ApproveCompanySubscriptionRequestByManager(c echo.Context) error {
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

	subscriptionRequestsCollection := sc.DB.Collection("subscription_requests")
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

	// Set company status to approved
	if !request.CompanyID.IsZero() {
		companyCollection := sc.DB.Collection("companies")
		_, err = companyCollection.UpdateOne(ctx, bson.M{"_id": request.CompanyID}, bson.M{"$set": bson.M{"status": "approved"}})
		if err != nil {
			log.Printf("Failed to update company status: %v", err)
		}
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Subscription request approved by manager and company status updated.",
	})
}

// RejectCompanySubscriptionRequestByManager allows a manager to reject a company subscription request
func (sc *SubscriptionController) RejectCompanySubscriptionRequestByManager(c echo.Context) error {
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

	subscriptionRequestsCollection := sc.DB.Collection("subscription_requests")
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

// GetTotalCommissionBalance returns the total available commission balance for the authenticated user
func (sc *SubscriptionController) GetTotalCommissionBalance(c echo.Context) error {
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

	var match bson.M
	var sumField string

	switch claims.UserType {
	case "salesperson":
		// Try both field name formats to handle legacy data
		match = bson.M{
			"$or": []bson.M{
				{"salespersonID": userID}, // New format (uppercase ID)
				{"salespersonId": userID}, // Legacy format (lowercase i)
			},
		}
		sumField = "$salespersonCommission"
	case "sales_manager":
		// Try both field name formats to handle legacy data
		match = bson.M{
			"$or": []bson.M{
				{"salesManagerID": userID}, // New format (uppercase ID)
				{"salesManagerId": userID}, // Legacy format (lowercase i)
			},
		}
		sumField = "$salesManagerCommission"
	default:
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "User type not allowed for commission balance",
		})
	}

	// Get total commission earned
	pipeline := []bson.M{
		{"$match": match},
		{"$group": bson.M{"_id": nil, "total": bson.M{"$sum": sumField}}},
	}

	cursor, err := sc.DB.Collection("commissions").Aggregate(ctx, pipeline)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to aggregate commissions",
		})
	}
	defer cursor.Close(ctx)

	total := 0.0
	if cursor.Next(ctx) {
		var result struct {
			Total float64 `bson:"total"`
		}
		if err := cursor.Decode(&result); err == nil {
			total = result.Total
		}
	}

	// Calculate total withdrawn (approved withdrawals only)
	withdrawalMatch := bson.M{"userId": userID, "userType": claims.UserType, "status": "approved"}
	withdrawalCursor, err := sc.DB.Collection("withdrawals").Aggregate(ctx, []bson.M{
		{"$match": withdrawalMatch},
		{"$group": bson.M{"_id": nil, "totalWithdrawn": bson.M{"$sum": "$amount"}}},
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to aggregate withdrawals",
		})
	}
	defer withdrawalCursor.Close(ctx)

	totalWithdrawn := 0.0
	if withdrawalCursor.Next(ctx) {
		var result struct {
			TotalWithdrawn float64 `bson:"totalWithdrawn"`
		}
		if err := withdrawalCursor.Decode(&result); err == nil {
			totalWithdrawn = result.TotalWithdrawn
		}
	}

	// Calculate total pending withdrawals (these are reserved and not available)
	pendingMatch := bson.M{"userId": userID, "userType": claims.UserType, "status": "pending"}
	pendingCursor, err := sc.DB.Collection("withdrawals").Aggregate(ctx, []bson.M{
		{"$match": pendingMatch},
		{"$group": bson.M{"_id": nil, "totalPending": bson.M{"$sum": "$amount"}}},
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to aggregate pending withdrawals",
		})
	}
	defer pendingCursor.Close(ctx)

	totalPending := 0.0
	if pendingCursor.Next(ctx) {
		var result struct {
			TotalPending float64 `bson:"totalPending"`
		}
		if err := pendingCursor.Decode(&result); err == nil {
			totalPending = result.TotalPending
		}
	}

	// Available balance = Total earned - Total withdrawn - Total pending
	availableBalance := total - totalWithdrawn - totalPending

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Total commission balance retrieved successfully",
		Data: map[string]interface{}{
			"totalBalance":     total,
			"totalWithdrawn":   totalWithdrawn,
			"totalPending":     totalPending,
			"availableBalance": availableBalance,
		},
	})
}

// RequestCommissionWithdrawal allows a user to request a withdrawal from their commission balance
func (sc *SubscriptionController) RequestCommissionWithdrawal(c echo.Context) error {
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

	var req struct {
		Amount   float64 `json:"amount"`
		UserNote string  `json:"userNote,omitempty"`
	}
	if err := c.Bind(&req); err != nil || req.Amount <= 0 {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid amount",
		})
	}

	// Validate minimum withdrawal amount
	if req.Amount < 10.0 {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Minimum withdrawal amount is $10.00",
		})
	}

	// Check if user has sufficient available balance
	// Get current available balance using the same logic as GetTotalCommissionBalance
	var match bson.M
	var sumField string
	switch claims.UserType {
	case "salesperson":
		// Try both field name formats to handle legacy data
		match = bson.M{
			"$or": []bson.M{
				{"salespersonID": userID}, // New format (uppercase ID)
				{"salespersonId": userID}, // Legacy format (lowercase i)
			},
		}
		sumField = "$salespersonCommission"
	case "sales_manager":
		// Try both field name formats to handle legacy data
		match = bson.M{
			"$or": []bson.M{
				{"salesManagerID": userID}, // New format (uppercase ID)
				{"salesManagerId": userID}, // Legacy format (lowercase i)
			},
		}
		sumField = "$salesManagerCommission"
	default:
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "User type not allowed for commission withdrawal",
		})
	}

	// Get total commission earned
	pipeline := []bson.M{
		{"$match": match},
		{"$group": bson.M{"_id": nil, "total": bson.M{"$sum": sumField}}},
	}
	cursor, err := sc.DB.Collection("commissions").Aggregate(ctx, pipeline)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to aggregate commissions",
		})
	}
	defer cursor.Close(ctx)

	total := 0.0
	if cursor.Next(ctx) {
		var result struct {
			Total float64 `bson:"total"`
		}
		if err := cursor.Decode(&result); err == nil {
			total = result.Total
		}
	}

	// Calculate total withdrawn (approved withdrawals only)
	withdrawalMatch := bson.M{"userId": userID, "userType": claims.UserType, "status": "approved"}
	withdrawalCursor, err := sc.DB.Collection("withdrawals").Aggregate(ctx, []bson.M{
		{"$match": withdrawalMatch},
		{"$group": bson.M{"_id": nil, "totalWithdrawn": bson.M{"$sum": "$amount"}}},
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to aggregate withdrawals",
		})
	}
	defer withdrawalCursor.Close(ctx)

	totalWithdrawn := 0.0
	if withdrawalCursor.Next(ctx) {
		var result struct {
			TotalWithdrawn float64 `bson:"totalWithdrawn"`
		}
		if err := withdrawalCursor.Decode(&result); err == nil {
			totalWithdrawn = result.TotalWithdrawn
		}
	}

	// Calculate total pending withdrawals (these are reserved and not available)
	pendingMatch := bson.M{"userId": userID, "userType": claims.UserType, "status": "pending"}
	pendingCursor, err := sc.DB.Collection("withdrawals").Aggregate(ctx, []bson.M{
		{"$match": pendingMatch},
		{"$group": bson.M{"_id": nil, "totalPending": bson.M{"$sum": "$amount"}}},
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to aggregate pending withdrawals",
		})
	}
	defer pendingCursor.Close(ctx)

	totalPending := 0.0
	if pendingCursor.Next(ctx) {
		var result struct {
			TotalPending float64 `bson:"totalPending"`
		}
		if err := pendingCursor.Decode(&result); err == nil {
			totalPending = result.TotalPending
		}
	}

	// Available balance = Total earned - Total withdrawn - Total pending
	availableBalance := total - totalWithdrawn - totalPending

	// Check if requested amount exceeds available balance
	if req.Amount > availableBalance {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: fmt.Sprintf("Withdrawal amount $%.2f exceeds available balance $%.2f", req.Amount, availableBalance),
		})
	}

	// Record withdrawal request
	withdrawal := models.Withdrawal{
		ID:        primitive.NewObjectID(),
		UserID:    userID,
		UserType:  claims.UserType,
		Amount:    req.Amount,
		Status:    "pending",
		UserNote:  req.UserNote,
		CreatedAt: time.Now(),
	}
	_, err = sc.DB.Collection("withdrawals").InsertOne(ctx, withdrawal)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to create withdrawal request",
		})
	}

	// Send notification to admin
	adminEmail := os.Getenv("ADMIN_EMAIL")
	if adminEmail != "" {
		subject := "New Commission Withdrawal Request"
		userNoteText := ""
		if req.UserNote != "" {
			userNoteText = fmt.Sprintf("\nUser Note: %s", req.UserNote)
		}

		body := fmt.Sprintf("A new commission withdrawal request has been submitted.\n\nUser Type: %s\nUser ID: %s\nAmount: $%.2f\nRequested At: %s%s\n\nPlease review and approve or reject this request.",
			claims.UserType,
			userID.Hex(),
			req.Amount,
			withdrawal.CreatedAt.Format("2006-01-02 15:04:05"),
			userNoteText)

		if err := sc.companySendAdminNotificationEmail(subject, body); err != nil {
			log.Printf("Failed to send admin notification email for withdrawal request: %v", err)
		}
	}

	// Optionally, send to manager as well if you have a manager email
	managerEmail := os.Getenv("MANAGER_EMAIL")
	if managerEmail != "" {
		subject := "New Commission Withdrawal Request (Manager Notification)"
		userNoteText := ""
		if req.UserNote != "" {
			userNoteText = fmt.Sprintf("\nUser Note: %s", req.UserNote)
		}

		body := fmt.Sprintf("A new commission withdrawal request has been submitted.\n\nUser Type: %s\nUser ID: %s\nAmount: $%.2f\nRequested At: %s%s\n\nPlease review and approve or reject this request.",
			claims.UserType,
			userID.Hex(),
			req.Amount,
			withdrawal.CreatedAt.Format("2006-01-02 15:04:05"),
			userNoteText)

		if err := sc.sendNotificationEmail(managerEmail, subject, body); err != nil {
			log.Printf("Failed to send manager notification email for withdrawal request: %v", err)
		}
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Withdrawal request submitted successfully and sent to admin for approval. The requested amount has been reserved from your available balance and will be processed once approved.",
		Data: map[string]interface{}{
			"withdrawal":       withdrawal,
			"message":          "Request submitted - awaiting admin approval",
			"status":           "pending",
			"availableBalance": availableBalance - req.Amount,
			"reservedAmount":   req.Amount,
		},
	})
}

// GetTotalWithdrawals returns the total withdrawn amount for the authenticated user
func (sc *SubscriptionController) GetTotalWithdrawals(c echo.Context) error {
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

	// Get total approved withdrawals (these are the actual withdrawals)
	approvedMatch := bson.M{"userId": userID, "userType": claims.UserType, "status": "approved"}
	approvedPipeline := []bson.M{
		{"$match": approvedMatch},
		{"$group": bson.M{"_id": nil, "totalWithdrawn": bson.M{"$sum": "$amount"}}},
	}

	// Get total pending withdrawals (these are reserved but not yet processed)
	pendingMatch := bson.M{"userId": userID, "userType": claims.UserType, "status": "pending"}
	pendingPipeline := []bson.M{
		{"$match": pendingMatch},
		{"$group": bson.M{"_id": nil, "totalPending": bson.M{"$sum": "$amount"}}},
	}

	// Get approved withdrawals
	cursor, err := sc.DB.Collection("withdrawals").Aggregate(ctx, approvedPipeline)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to aggregate approved withdrawals",
		})
	}
	defer cursor.Close(ctx)

	totalWithdrawn := 0.0
	if cursor.Next(ctx) {
		var result struct {
			TotalWithdrawn float64 `bson:"totalWithdrawn"`
		}
		if err := cursor.Decode(&result); err == nil {
			totalWithdrawn = result.TotalWithdrawn
		}
	}

	// Get pending withdrawals
	pendingCursor, err := sc.DB.Collection("withdrawals").Aggregate(ctx, pendingPipeline)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to aggregate pending withdrawals",
		})
	}
	defer pendingCursor.Close(ctx)

	totalPending := 0.0
	if pendingCursor.Next(ctx) {
		var result struct {
			TotalPending float64 `bson:"totalPending"`
		}
		if err := pendingCursor.Decode(&result); err == nil {
			totalPending = result.TotalPending
		}
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Total withdrawals retrieved successfully",
		Data: map[string]interface{}{
			"totalWithdrawn": totalWithdrawn,
			"totalPending":   totalPending,
		},
	})
}

// GetCommissionSummary returns the total commission, total withdrawals, and available balance for the authenticated user
func (sc *SubscriptionController) GetCommissionSummary(c echo.Context) error {
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

	log.Printf("DEBUG: GetCommissionSummary - UserID: %s, UserType: %s", userID.Hex(), claims.UserType)

	// Calculate total commission
	var match bson.M
	var sumField string
	switch claims.UserType {
	case "salesperson":
		// Try both field name formats to handle legacy data
		match = bson.M{
			"$or": []bson.M{
				{"salespersonID": userID}, // New format (uppercase ID)
				{"salespersonId": userID}, // Legacy format (lowercase i)
			},
		}
		sumField = "$salespersonCommission"
		log.Printf("DEBUG: Looking for commissions with salespersonID/salespersonId: %s", userID.Hex())
	case "admin":
		// Look for admin commissions
		match = bson.M{
			"adminID": userID,
		}
		sumField = "$adminCommission"
		log.Printf("DEBUG: Looking for commissions with adminID: %s", userID.Hex())
	case "sales_manager":
		// Try both field name formats to handle legacy data
		match = bson.M{
			"$or": []bson.M{
				{"salesManagerID": userID}, // New format (uppercase ID)
				{"salesManagerId": userID}, // Legacy format (lowercase i)
			},
		}
		sumField = "$salesManagerCommission"
		log.Printf("DEBUG: Looking for commissions with salesManagerID/salesManagerId: %s", userID.Hex())
	default:
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "User type not allowed for commission summary",
		})
	}

	// First, let's check if there are any commissions in the database at all
	totalCommissions, err := sc.DB.Collection("commissions").CountDocuments(ctx, bson.M{})
	if err != nil {
		log.Printf("DEBUG: Error counting total commissions: %v", err)
	} else {
		log.Printf("DEBUG: Total commissions in database: %d", totalCommissions)
	}

	// Check if there are any commissions for this user
	userCommissions, err := sc.DB.Collection("commissions").CountDocuments(ctx, match)
	if err != nil {
		log.Printf("DEBUG: Error counting user commissions: %v", err)
	} else {
		log.Printf("DEBUG: Commissions for user %s: %d", userID.Hex(), userCommissions)
	}

	// Let's also check what commissions exist (first 5)
	var sampleCommissions []bson.M
	sampleCursor, err := sc.DB.Collection("commissions").Find(ctx, bson.M{}, options.Find().SetLimit(5))
	if err == nil {
		log.Printf("DEBUG: Error fetching sample commissions: %v", err)
	} else {
		defer sampleCursor.Close(ctx)
		if err := sampleCursor.All(ctx, &sampleCommissions); err == nil {
			log.Printf("DEBUG: Sample commissions in database: %+v", sampleCommissions)
			// Log the salesperson IDs from existing commissions
			for i, comm := range sampleCommissions {
				if salespersonID, ok := comm["salespersonID"]; ok {
					log.Printf("DEBUG: Commission %d - salespersonID (new format): %v", i, salespersonID)
				}
				if salespersonId, ok := comm["salespersonId"]; ok {
					log.Printf("DEBUG: Commission %d - salespersonId (legacy format): %v", i, salespersonId)
				}
			}
		} else {
			log.Printf("DEBUG: Error decoding sample commissions: %v", err)
		}
	}
	commissionPipeline := []bson.M{
		{"$match": match},
		{"$group": bson.M{"_id": nil, "totalCommission": bson.M{"$sum": sumField}}},
	}
	log.Printf("DEBUG: Commission pipeline: %+v", commissionPipeline)
	commissionCursor, err := sc.DB.Collection("commissions").Aggregate(ctx, commissionPipeline)
	if err != nil {
		log.Printf("DEBUG: Error in commission aggregation: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to aggregate commissions",
		})
	}
	defer commissionCursor.Close(ctx)
	totalCommission := 0.0
	if commissionCursor.Next(ctx) {
		var result struct {
			TotalCommission float64 `bson:"totalCommission"`
		}
		if err := commissionCursor.Decode(&result); err == nil {
			totalCommission = result.TotalCommission
			log.Printf("DEBUG: Found total commission: %.2f", totalCommission)
		} else {
			log.Printf("DEBUG: Error decoding commission result: %v", err)
		}
	} else {
		log.Printf("DEBUG: No commission results found")
	}

	// Calculate total approved withdrawals (actual money withdrawn)
	approvedWithdrawalMatch := bson.M{"userId": userID, "userType": claims.UserType, "status": "approved"}
	approvedWithdrawalPipeline := []bson.M{
		{"$match": approvedWithdrawalMatch},
		{"$group": bson.M{"_id": nil, "totalApprovedWithdrawn": bson.M{"$sum": "$amount"}}},
	}
	approvedWithdrawalCursor, err := sc.DB.Collection("withdrawals").Aggregate(ctx, approvedWithdrawalPipeline)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to aggregate approved withdrawals",
		})
	}
	defer approvedWithdrawalCursor.Close(ctx)
	totalApprovedWithdrawn := 0.0
	if approvedWithdrawalCursor.Next(ctx) {
		var result struct {
			TotalApprovedWithdrawn float64 `bson:"totalApprovedWithdrawn"`
		}
		if err := approvedWithdrawalCursor.Decode(&result); err == nil {
			totalApprovedWithdrawn = result.TotalApprovedWithdrawn
		}
	}

	// Calculate total pending withdrawals (reserved amounts)
	pendingWithdrawalMatch := bson.M{"userId": userID, "userType": claims.UserType, "status": "pending"}
	pendingWithdrawalPipeline := []bson.M{
		{"$match": pendingWithdrawalMatch},
		{"$group": bson.M{"_id": nil, "totalPendingWithdrawn": bson.M{"$sum": "$amount"}}},
	}
	pendingWithdrawalCursor, err := sc.DB.Collection("withdrawals").Aggregate(ctx, pendingWithdrawalPipeline)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to aggregate pending withdrawals",
		})
	}
	defer pendingWithdrawalCursor.Close(ctx)
	totalPendingWithdrawn := 0.0
	if pendingWithdrawalCursor.Next(ctx) {
		var result struct {
			TotalPendingWithdrawn float64 `bson:"totalPendingWithdrawn"`
		}
		if err := pendingWithdrawalCursor.Decode(&result); err == nil {
			totalPendingWithdrawn = result.TotalPendingWithdrawn
		}
	}

	// For salespersons, also fetch referral balance
	var referralBalance float64
	var referralCount int
	if claims.UserType == "salesperson" {
		// Get salesperson data to fetch referral balance
		var salesperson models.Salesperson
		err := sc.DB.Collection("salespersons").FindOne(
			ctx,
			bson.M{"_id": userID},
		).Decode(&salesperson)
		if err == nil {
			referralBalance = salesperson.ReferralBalance
			referralCount = len(salesperson.Referrals)
		}
	}

	// Available balance = Total commission + Referral balance - Approved withdrawals - Pending withdrawals
	availableBalance := totalCommission + referralBalance - totalApprovedWithdrawn - totalPendingWithdrawn

	// Create user-specific message
	var userTypeDisplay string
	switch claims.UserType {
	case "salesperson":
		userTypeDisplay = "salesperson"
	case "admin":
		userTypeDisplay = "admin"
	case "sales_manager":
		userTypeDisplay = "sales manager"
	default:
		userTypeDisplay = claims.UserType
	}

	// Prepare response data
	responseData := map[string]interface{}{
		"totalCommission":        totalCommission,
		"totalApprovedWithdrawn": totalApprovedWithdrawn,
		"totalPendingWithdrawn":  totalPendingWithdrawn,
		"availableBalance":       availableBalance,
	}

	// Add referral data for salespersons
	if claims.UserType == "salesperson" {
		responseData["referralBalance"] = referralBalance
		responseData["referralCount"] = referralCount
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: fmt.Sprintf("Commission summary retrieved successfully for %s", userTypeDisplay),
		Data:    responseData,
	})
}

// Helper function to send admin notification email
func (sc *SubscriptionController) companySendAdminNotificationEmail(subject, body string) error {
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

// sendNotificationEmail sends a notification email for SubscriptionController
func (sc *SubscriptionController) sendNotificationEmail(to, subject, body string) error {
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

// sendNotificationEmail sends a notification email (copied from SubscriptionController)
func (sc *BranchSubscriptionController) sendNotificationEmail(to, subject, body string) error {
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

// saveUploadedFile saves an uploaded file to the specified directory
func (sc *BranchSubscriptionController) saveUploadedFile(file *multipart.FileHeader, directory string) (string, error) {
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

// GetPendingWithdrawalRequests retrieves all pending withdrawal requests (admin only)
func (sc *SubscriptionController) GetPendingWithdrawalRequests(c echo.Context) error {
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

	// Find all pending withdrawal requests
	withdrawalsCollection := sc.DB.Collection("withdrawals")
	cursor, err := withdrawalsCollection.Find(ctx, bson.M{"status": "pending"})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve withdrawal requests",
		})
	}
	defer cursor.Close(ctx)

	var withdrawals []models.Withdrawal
	if err = cursor.All(ctx, &withdrawals); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode withdrawal requests",
		})
	}

	// Get user details for each withdrawal request
	var enrichedWithdrawals []map[string]interface{}
	for _, withdrawal := range withdrawals {
		var userDetails map[string]interface{}

		// Get user details based on user type
		switch withdrawal.UserType {
		case "salesperson":
			var salesperson models.Salesperson
			err = sc.DB.Collection("salespersons").FindOne(ctx, bson.M{"_id": withdrawal.UserID}).Decode(&salesperson)
			if err == nil {
				userDetails = map[string]interface{}{
					"id":       salesperson.ID,
					"fullName": salesperson.FullName,
					"email":    salesperson.Email,
					"phone":    salesperson.PhoneNumber,
					"userType": "salesperson",
				}
			}
		case "sales_manager":
			var salesManager models.SalesManager
			err = sc.DB.Collection("sales_managers").FindOne(ctx, bson.M{"_id": withdrawal.UserID}).Decode(&salesManager)
			if err == nil {
				userDetails = map[string]interface{}{
					"id":       salesManager.ID,
					"fullName": salesManager.FullName,
					"email":    salesManager.Email,
					"phone":    salesManager.PhoneNumber,
					"userType": "sales_manager",
				}
			}
		}

		enrichedWithdrawals = append(enrichedWithdrawals, map[string]interface{}{
			"withdrawal": withdrawal,
			"user":       userDetails,
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Pending withdrawal requests retrieved successfully",
		Data:    enrichedWithdrawals,
	})
}

// ApproveWithdrawalRequest handles the approval of a withdrawal request (admin only)
func (sc *SubscriptionController) ApproveWithdrawalRequest(c echo.Context) error {
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

	// Get withdrawal ID from URL parameter
	withdrawalID := c.Param("id")
	if withdrawalID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Withdrawal ID is required",
		})
	}

	// Convert withdrawal ID to ObjectID
	withdrawalObjectID, err := primitive.ObjectIDFromHex(withdrawalID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid withdrawal ID format",
		})
	}

	// Parse approval request body
	var approvalReq struct {
		AdminNote string `json:"adminNote,omitempty"`
	}
	if err := c.Bind(&approvalReq); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}

	// Get the withdrawal request
	withdrawalsCollection := sc.DB.Collection("withdrawals")
	var withdrawal models.Withdrawal
	err = withdrawalsCollection.FindOne(ctx, bson.M{"_id": withdrawalObjectID}).Decode(&withdrawal)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Withdrawal request not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find withdrawal request",
		})
	}

	// Check if request is already processed
	if withdrawal.Status != "pending" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Withdrawal request is already processed",
		})
	}

	// Convert adminID from string to ObjectID
	adminObjectID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid admin ID",
		})
	}

	// Update withdrawal request with approval
	update := bson.M{
		"$set": bson.M{
			"status":      "approved",
			"adminId":     adminObjectID,
			"adminNote":   approvalReq.AdminNote,
			"processedAt": time.Now(),
		},
	}

	_, err = withdrawalsCollection.UpdateOne(ctx, bson.M{"_id": withdrawalObjectID}, update)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update withdrawal request",
		})
	}

	// Send notification to user about approval
	var userEmail string
	var userName string

	switch withdrawal.UserType {
	case "salesperson":
		var salesperson models.Salesperson
		err = sc.DB.Collection("salespersons").FindOne(ctx, bson.M{"_id": withdrawal.UserID}).Decode(&salesperson)
		if err == nil {
			userEmail = salesperson.Email
			userName = salesperson.FullName
		}
	case "sales_manager":
		var salesManager models.SalesManager
		err = sc.DB.Collection("sales_managers").FindOne(ctx, bson.M{"_id": withdrawal.UserID}).Decode(&salesManager)
		if err == nil {
			userEmail = salesManager.Email
			userName = salesManager.FullName
		}
	}

	if userEmail != "" {
		subject := "Commission Withdrawal Approved! 🎉"
		body := fmt.Sprintf(`
			Dear %s,
			
			Great news! Your commission withdrawal request has been approved.
			
			Withdrawal Details:
			- Amount: $%.2f
			- Requested At: %s
			- Approved At: %s
			- Status: Approved
			
			Your withdrawal will be processed shortly. You will receive the funds according to your payment method.
			
			Thank you for your patience!
		`,
			userName,
			withdrawal.Amount,
			withdrawal.CreatedAt.Format("2006-01-02 15:04:05"),
			time.Now().Format("2006-01-02 15:04:05"),
		)

		if err := sc.sendNotificationEmail(userEmail, subject, body); err != nil {
			log.Printf("Failed to send approval notification email: %v", err)
		}
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Withdrawal request approved successfully",
		Data: map[string]interface{}{
			"withdrawalId": withdrawal.ID,
			"status":       "approved",
			"processedAt":  time.Now(),
		},
	})
}

// RejectWithdrawalRequest handles the rejection of a withdrawal request (admin only)
func (sc *SubscriptionController) RejectWithdrawalRequest(c echo.Context) error {
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

	// Get withdrawal ID from URL parameter
	withdrawalID := c.Param("id")
	if withdrawalID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Withdrawal ID is required",
		})
	}

	// Convert withdrawal ID to ObjectID
	withdrawalObjectID, err := primitive.ObjectIDFromHex(withdrawalID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid withdrawal ID format",
		})
	}

	// Parse rejection request body
	var rejectionReq struct {
		AdminNote string `json:"adminNote"`
	}
	if err := c.Bind(&rejectionReq); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}

	// Validate admin note is provided
	if rejectionReq.AdminNote == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Admin note is required for rejection",
		})
	}

	// Get the withdrawal request
	withdrawalsCollection := sc.DB.Collection("withdrawals")
	var withdrawal models.Withdrawal
	err = withdrawalsCollection.FindOne(ctx, bson.M{"_id": withdrawalObjectID}).Decode(&withdrawal)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Withdrawal request not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find withdrawal request",
		})
	}

	// Check if request is already processed
	if withdrawal.Status != "pending" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Withdrawal request is already processed",
		})
	}

	// Convert adminID from string to ObjectID
	adminObjectID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid admin ID",
		})
	}

	// Update withdrawal request with rejection
	update := bson.M{
		"$set": bson.M{
			"status":      "rejected",
			"adminId":     adminObjectID,
			"adminNote":   rejectionReq.AdminNote,
			"processedAt": time.Now(),
		},
	}

	_, err = withdrawalsCollection.UpdateOne(ctx, bson.M{"_id": withdrawalObjectID}, update)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update withdrawal request",
		})
	}

	// Send notification to user about rejection
	var userEmail string
	var userName string

	switch withdrawal.UserType {
	case "salesperson":
		var salesperson models.Salesperson
		err = sc.DB.Collection("salespersons").FindOne(ctx, bson.M{"_id": withdrawal.UserID}).Decode(&salesperson)
		if err == nil {
			userEmail = salesperson.Email
			userName = salesperson.FullName
		}
	case "sales_manager":
		var salesManager models.SalesManager
		err = sc.DB.Collection("sales_managers").FindOne(ctx, bson.M{"_id": withdrawal.UserID}).Decode(&salesManager)
		if err == nil {
			userEmail = salesManager.Email
			userName = salesManager.FullName
		}
	}

	if userEmail != "" {
		subject := "Commission Withdrawal Request Update"
		body := fmt.Sprintf(`
			Dear %s,
			
			Your commission withdrawal request has been reviewed.
			
			Withdrawal Details:
			- Amount: $%.2f
			- Requested At: %s
			- Status: Rejected
			
			Reason: %s
			
			If you have any questions, please contact our support team.
		`,
			userName,
			withdrawal.Amount,
			withdrawal.CreatedAt.Format("2006-01-02 15:04:05"),
			rejectionReq.AdminNote,
		)

		if err := sc.sendNotificationEmail(userEmail, subject, body); err != nil {
			log.Printf("Failed to send rejection notification email: %v", err)
		}
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Withdrawal request rejected successfully",
		Data: map[string]interface{}{
			"withdrawalId": withdrawal.ID,
			"status":       "rejected",
			"processedAt":  time.Now(),
		},
	})
}

// ProcessBranchSubscriptionRequest handles the approval or rejection of a branch subscription request (admin only)
func (sc *SubscriptionController) ProcessBranchSubscriptionRequest(c echo.Context) error {
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
	var approvalReq models.SubscriptionApprovalRequest
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

	// Get the branch subscription request
	branchSubscriptionRequestsCollection := sc.DB.Collection("branch_subscription_requests")
	var branchSubscriptionRequest models.BranchSubscriptionRequest
	err = branchSubscriptionRequestsCollection.FindOne(ctx, bson.M{"_id": requestObjectID}).Decode(&branchSubscriptionRequest)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Branch subscription request not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find branch subscription request",
		})
	}

	// Check if request is already processed
	if branchSubscriptionRequest.Status != "pending" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: fmt.Sprintf("Branch subscription request is already %s", branchSubscriptionRequest.Status),
		})
	}

	// Delete the branch subscription request from database after processing
	_, err = branchSubscriptionRequestsCollection.DeleteOne(ctx, bson.M{"_id": requestObjectID})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to delete branch subscription request",
		})
	}

	// Get company and branch details
	var company models.Company
	err = sc.DB.Collection("companies").FindOne(ctx, bson.M{"branches._id": branchSubscriptionRequest.BranchID}).Decode(&company)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to get company or branch details",
		})
	}

	// Find the specific branch in the company's branches array
	var branch models.Branch
	branchFound := false
	for _, b := range company.Branches {
		if b.ID == branchSubscriptionRequest.BranchID {
			branch = b
			branchFound = true
			break
		}
	}

	if !branchFound {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Branch not found in company",
		})
	}

	var plan models.SubscriptionPlan
	err = sc.DB.Collection("subscription_plans").FindOne(ctx, bson.M{"_id": branchSubscriptionRequest.PlanID}).Decode(&plan)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to get plan details",
		})
	}

	// If approved, create the subscription
	var subscription *models.BranchSubscription
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
		newSubscription := models.BranchSubscription{
			ID:        primitive.NewObjectID(),
			BranchID:  branchSubscriptionRequest.BranchID,
			PlanID:    branchSubscriptionRequest.PlanID,
			StartDate: startDate,
			EndDate:   endDate,
			Status:    "active",
			AutoRenew: false, // Default to false, can be changed later
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		// Save subscription
		subscriptionsCollection := sc.DB.Collection("branch_subscriptions")
		_, err = subscriptionsCollection.InsertOne(ctx, newSubscription)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to create subscription",
			})
		}

		// Update branch status to active in branches collection
		branchCollection := sc.DB.Collection("branches")
		_, err = branchCollection.UpdateOne(ctx, bson.M{"_id": branchSubscriptionRequest.BranchID}, bson.M{"$set": bson.M{"status": "active"}})
		if err != nil {
			log.Printf("Failed to update branch status to active in branches collection: %v", err)
		}

		// Also update the branch status in the companies.branches array
		// Use a more specific query to ensure the update works correctly
		_, err = sc.DB.Collection("companies").UpdateOne(
			ctx,
			bson.M{
				"_id":          company.ID,
				"branches._id": branchSubscriptionRequest.BranchID,
			},
			bson.M{
				"$set": bson.M{
					"branches.$.status":    "active",
					"branches.$.updatedAt": time.Now(),
				},
			},
		)
		if err != nil {
			log.Printf("Failed to update branch status to active in companies collection: %v", err)
			// Try alternative approach if the first one fails
			// Find the company again and update the specific branch
			var updatedCompany models.Company
			err2 := sc.DB.Collection("companies").FindOne(ctx, bson.M{"_id": company.ID}).Decode(&updatedCompany)
			if err2 == nil {
				// Update the branch status in the branches array
				for i, b := range updatedCompany.Branches {
					if b.ID == branchSubscriptionRequest.BranchID {
						updatedCompany.Branches[i].Status = "active"
						updatedCompany.Branches[i].UpdatedAt = time.Now()
						break
					}
				}

				// Update the entire company document
				_, err3 := sc.DB.Collection("companies").ReplaceOne(ctx, bson.M{"_id": company.ID}, updatedCompany)
				if err3 != nil {
					log.Printf("Failed to update branch status using alternative approach: %v", err3)
				} else {
					log.Printf("Successfully updated branch status using alternative approach")
				}
			} else {
				log.Printf("Failed to find company for alternative update approach: %v", err2)
			}
		} else {
			log.Printf("Successfully updated branch status to active in companies collection")
		}

		subscription = &newSubscription

		// --- Commission logic start ---
		// Check if company was created by a salesperson or by itself (user signup)
		log.Printf("DEBUG: Company CreatedBy field: %v (IsZero: %v)", company.CreatedBy, company.CreatedBy.IsZero())

		// Check if company was created by itself (user signup) - CreatedBy equals UserID
		if company.CreatedBy == company.UserID {
			log.Printf("DEBUG: Company was created by user signup, adding subscription price to admin wallet")
			// Add subscription price directly to admin wallet (no commission calculation needed)
			err := sc.addSubscriptionIncomeToAdminWallet(ctx, plan.Price, newSubscription.ID, "branch_subscription", company.BusinessName, branch.Name)
			if err != nil {
				log.Printf("Failed to add subscription income to admin wallet: %v", err)
			} else {
				log.Printf("Subscription income added to admin wallet: $%.2f from company '%s' (ID: %s) - User signup subscription",
					plan.Price, company.BusinessName, company.ID.Hex())
			}
		} else if !company.CreatedBy.IsZero() {
			log.Printf("DEBUG: Company was created by salesperson, proceeding with commission calculation")
			// Get salesperson
			var salesperson models.Salesperson
			err := sc.DB.Collection("salespersons").FindOne(ctx, bson.M{"_id": company.CreatedBy}).Decode(&salesperson)
			if err == nil {
				log.Printf("DEBUG: Found salesperson: %s (ID: %v)", salesperson.FullName, salesperson.ID)
				planPrice := plan.Price
				salespersonPercent := salesperson.CommissionPercent

				// Check if salesperson was created by admin (admin-created salesperson)
				var admin models.Admin
				err := sc.DB.Collection("admins").FindOne(ctx, bson.M{"_id": salesperson.CreatedBy}).Decode(&admin)
				if err == nil {
					// Admin-created salesperson: Admin gets remaining percentage after salesperson commission
					adminPercent := 100.0 - salespersonPercent

					// Calculate commissions correctly
					// Salesperson gets their percentage directly from the plan price
					salespersonCommission := planPrice * salespersonPercent / 100.0
					// Admin gets the remaining percentage
					adminCommission := planPrice * adminPercent / 100.0

					// Insert commission document using Commission model
					commission := models.Commission{
						ID:                           primitive.NewObjectID(),
						SubscriptionID:               newSubscription.ID,
						CompanyID:                    company.ID,
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
					_, err := sc.DB.Collection("commissions").InsertOne(ctx, commission)
					if err != nil {
						log.Printf("Failed to insert commission: %v", err)
					} else {
						log.Printf("Commission inserted successfully for branch subscription (admin-created salesperson) - Plan Price: $%.2f, Admin Commission: $%.2f (%.1f%%), Salesperson Commission: $%.2f (%.1f%%)",
							planPrice, adminCommission, adminPercent, salespersonCommission, salespersonPercent)

						// Add salesperson commission to their wallet
						err = sc.addCommissionToSalespersonWallet(ctx, salesperson.ID, salespersonCommission, newSubscription.ID, "branch_subscription", company.BusinessName)
						if err != nil {
							log.Printf("Failed to add commission to salesperson wallet: %v", err)
						}

						// Add admin commission to admin wallet
						err = sc.addSubscriptionIncomeToAdminWallet(ctx, adminCommission, newSubscription.ID, "branch_subscription_admin_commission", company.BusinessName, branch.Name)
						if err != nil {
							log.Printf("Failed to add admin commission to admin wallet: %v", err)
						}
					}
				} else {
					// Salesperson was created by sales manager: Handle sales manager commission
					var salesManager models.SalesManager
					err := sc.DB.Collection("sales_managers").FindOne(ctx, bson.M{"_id": salesperson.SalesManagerID}).Decode(&salesManager)
					if err != nil {
						// Try alternative collection name
						err = sc.DB.Collection("salesManagers").FindOne(ctx, bson.M{"_id": salesperson.SalesManagerID}).Decode(&salesManager)
					}
					if err == nil {
						log.Printf("DEBUG: Found sales manager: %s (ID: %v)", salesManager.FullName, salesManager.ID)
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
							CompanyID:                     company.ID,
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
							log.Printf("Commission inserted successfully for branch subscription (sales manager-created salesperson) - Plan Price: $%.2f, Sales Manager Commission: $%.2f, Salesperson Commission: $%.2f",
								planPrice, salesManagerCommission, salespersonCommission)

							// Add salesperson commission to their wallet
							err = sc.addCommissionToSalespersonWallet(ctx, salesperson.ID, salespersonCommission, newSubscription.ID, "branch_subscription", company.BusinessName)
							if err != nil {
								log.Printf("Failed to add commission to salesperson wallet: %v", err)
							}

							// Add sales manager commission to their wallet
							err = sc.addCommissionToSalesManagerWallet(ctx, salesManager.ID, salesManagerCommission, newSubscription.ID, "branch_subscription", company.BusinessName)
							if err != nil {
								log.Printf("Failed to add commission to sales manager wallet: %v", err)
							}

							// Calculate remaining amount for admin wallet
							totalCommissions := salesManagerCommission + salespersonCommission
							adminWalletAmount := planPrice - totalCommissions

							// Add remaining amount to admin wallet
							if adminWalletAmount > 0 {
								err = sc.addSubscriptionIncomeToAdminWallet(ctx, adminWalletAmount, newSubscription.ID, "branch_subscription_remaining", company.BusinessName, branch.Name)
								if err != nil {
									log.Printf("Failed to add remaining subscription income to admin wallet: %v", err)
								} else {
									log.Printf("Remaining subscription income added to admin wallet: $%.2f (Plan: $%.2f - Commissions: $%.2f) from company '%s'",
										adminWalletAmount, planPrice, totalCommissions, company.BusinessName)
								}
							}
						}
					} else {
						log.Printf("DEBUG: Failed to find sales manager for ID: %v, Error: %v", salesperson.SalesManagerID, err)
						// Fallback: Give all to admin if sales manager not found
						err = sc.addSubscriptionIncomeToAdminWallet(ctx, planPrice, newSubscription.ID, "branch_subscription_fallback", company.BusinessName, branch.Name)
						if err != nil {
							log.Printf("Failed to add fallback amount to admin wallet: %v", err)
						}
					}
				}
			} else {
				log.Printf("DEBUG: Failed to find salesperson for ID: %v, Error: %v", company.CreatedBy, err)
			}
		} else {
			log.Printf("DEBUG: Company was not created by a salesperson (CreatedBy is zero or not set)")
		}
		// --- Commission logic end ---

		// Send approval notification
		emailSubject := "Branch Subscription Request Approved! 🎉"
		log.Printf("Branch subscription approved notification: %s - %s", branch.Name, emailSubject)
	} else {
		// If rejected, update branch status to inactive and send rejection notification
		branchCollection := sc.DB.Collection("branches")
		_, err = branchCollection.UpdateOne(ctx, bson.M{"_id": branchSubscriptionRequest.BranchID}, bson.M{"$set": bson.M{"status": "inactive"}})
		if err != nil {
			log.Printf("Failed to update branch status to inactive: %v", err)
		}

		// Also update the branch status in the companies.branches array
		_, err = sc.DB.Collection("companies").UpdateOne(
			ctx,
			bson.M{
				"_id":          company.ID,
				"branches._id": branchSubscriptionRequest.BranchID,
			},
			bson.M{
				"$set": bson.M{
					"branches.$.status":    "inactive",
					"branches.$.updatedAt": time.Now(),
				},
			},
		)
		if err != nil {
			log.Printf("Failed to update branch status to inactive in companies collection: %v", err)
			// Try alternative approach if the first one fails
			var updatedCompany models.Company
			err2 := sc.DB.Collection("companies").FindOne(ctx, bson.M{"_id": company.ID}).Decode(&updatedCompany)
			if err2 == nil {
				// Update the branch status in the branches array
				for i, b := range updatedCompany.Branches {
					if b.ID == branchSubscriptionRequest.BranchID {
						updatedCompany.Branches[i].Status = "inactive"
						updatedCompany.Branches[i].UpdatedAt = time.Now()
						break
					}
				}

				// Update the entire company document
				_, err3 := sc.DB.Collection("companies").ReplaceOne(ctx, bson.M{"_id": company.ID}, updatedCompany)
				if err3 != nil {
					log.Printf("Failed to update branch status to inactive using alternative approach: %v", err3)
				} else {
					log.Printf("Successfully updated branch status to inactive using alternative approach")
				}
			} else {
				log.Printf("Failed to find company for alternative update approach (rejection): %v", err2)
			}
		} else {
			log.Printf("Successfully updated branch status to inactive in companies collection")
		}

		emailSubject := "Branch Subscription Request Update"
		log.Printf("Branch subscription rejected notification: %s - %s", branch.Name, emailSubject)
	}

	// Prepare response data
	responseData := map[string]interface{}{
		"requestId":    branchSubscriptionRequest.ID,
		"branchName":   branch.Name,
		"planName":     plan.Title,
		"status":       approvalReq.Status,
		"processedAt":  time.Now(),
		"adminNote":    approvalReq.AdminNote,
		"subscription": subscription,
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: fmt.Sprintf("Branch subscription request %s successfully", approvalReq.Status),
		Data:    responseData,
	})
}

// GetPendingBranchSubscriptionRequests retrieves only branch subscription requests (admin only)
func (sc *SubscriptionController) GetPendingBranchSubscriptionRequests(c echo.Context) error {
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

	// Get branch subscription requests only
	branchCollection := sc.DB.Collection("branch_subscription_requests")
	branchCursor, err := branchCollection.Find(ctx, bson.M{"status": "pending"})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve pending branch subscription requests",
		})
	}
	defer branchCursor.Close(ctx)

	var branchRequests []models.BranchSubscriptionRequest
	if err = branchCursor.All(ctx, &branchRequests); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode pending branch requests",
		})
	}

	// Collect all unique plan IDs
	planIDSet := make(map[primitive.ObjectID]struct{})
	for _, br := range branchRequests {
		planIDSet[br.PlanID] = struct{}{}
	}
	planIDs := make([]primitive.ObjectID, 0, len(planIDSet))
	for id := range planIDSet {
		planIDs = append(planIDs, id)
	}

	// Fetch all plans in one query
	planMap := make(map[primitive.ObjectID]models.SubscriptionPlan)
	if len(planIDs) > 0 {
		planCursor, err := sc.DB.Collection("subscription_plans").Find(ctx, bson.M{"_id": bson.M{"$in": planIDs}})
		if err == nil {
			var plans []models.SubscriptionPlan
			if err := planCursor.All(ctx, &plans); err == nil {
				for _, plan := range plans {
					planMap[plan.ID] = plan
				}
			}
			planCursor.Close(ctx)
		}
	}

	// Attach plan details and get branch/company details for each request
	var enrichedRequests []map[string]interface{}
	for _, br := range branchRequests {
		plan, _ := planMap[br.PlanID]

		// Get branch details by finding the company that contains this branch
		var company models.Company
		var branch models.Branch
		branchFound := false

		companyCursor, err := sc.DB.Collection("companies").Find(ctx, bson.M{})
		if err == nil {
			defer companyCursor.Close(ctx)
			for companyCursor.Next(ctx) {
				var comp models.Company
				if err := companyCursor.Decode(&comp); err == nil {
					for _, b := range comp.Branches {
						if b.ID == br.BranchID {
							company = comp
							branch = b
							branchFound = true
							break
						}
					}
					if branchFound {
						break
					}
				}
			}
		}

		enrichedRequests = append(enrichedRequests, map[string]interface{}{
			"request": br,
			"plan":    plan,
			"company": map[string]interface{}{
				"id":           company.ID,
				"businessName": company.BusinessName,
				"phone":        company.ContactInfo.Phone,
				"whatsapp":     company.ContactInfo.WhatsApp,
				"website":      company.ContactInfo.Website,
			},
			"branch": map[string]interface{}{
				"id":          branch.ID,
				"name":        branch.Name,
				"location":    branch.Location,
				"phone":       branch.Phone,
				"category":    branch.Category,
				"description": branch.Description,
				"status":      branch.Status,
			},
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Pending branch subscription requests retrieved successfully",
		Data: map[string]interface{}{
			"branchRequests":      enrichedRequests,
			"totalBranchRequests": len(branchRequests),
		},
	})
}

// GetCommissions retrieves detailed commission information for salesperson and sales manager
func (sc *SubscriptionController) GetCommissions(c echo.Context) error {
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

	// Check if user type is allowed
	if claims.UserType != "salesperson" && claims.UserType != "sales_manager" && claims.UserType != "admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "User type not allowed for commission access",
		})
	}

	// Build match condition based on user type
	var match bson.M
	switch claims.UserType {
	case "salesperson":
		// Try both field name formats to handle legacy data
		match = bson.M{
			"$or": []bson.M{
				{"salespersonID": userID}, // New format (uppercase ID)
				{"salespersonId": userID}, // Legacy format (lowercase i)
			},
		}
	case "admin":
		// Look for admin commissions
		match = bson.M{
			"adminID": userID,
		}
	case "sales_manager":
		// Try both field name formats to handle legacy data
		match = bson.M{
			"$or": []bson.M{
				{"salesManagerID": userID}, // New format (uppercase ID)
				{"salesManagerId": userID}, // Legacy format (lowercase i)
			},
		}
	}

	// Aggregate pipeline to get commission details with subscription and company information
	pipeline := []bson.M{
		{"$match": match},
		{
			"$lookup": bson.M{
				"from":         "subscriptions",
				"localField":   "subscriptionId",
				"foreignField": "_id",
				"as":           "subscription",
			},
		},
		{
			"$lookup": bson.M{
				"from":         "companies",
				"localField":   "companyId",
				"foreignField": "_id",
				"as":           "company",
			},
		},
		{
			"$lookup": bson.M{
				"from":         "subscription_plans",
				"localField":   "planId",
				"foreignField": "_id",
				"as":           "plan",
			},
		},
		{
			"$unwind": bson.M{
				"path":                       "$subscription",
				"preserveNullAndEmptyArrays": true,
			},
		},
		{
			"$unwind": bson.M{
				"path":                       "$company",
				"preserveNullAndEmptyArrays": true,
			},
		},
		{
			"$unwind": bson.M{
				"path":                       "$plan",
				"preserveNullAndEmptyArrays": true,
			},
		},
		{
			"$project": bson.M{
				"_id":                           1,
				"subscriptionId":                1,
				"companyId":                     1,
				"planId":                        1,
				"planPrice":                     1,
				"salespersonID":                 1,
				"salespersonCommission":         1,
				"salespersonCommissionPercent":  1,
				"salesManagerID":                1,
				"salesManagerCommission":        1,
				"salesManagerCommissionPercent": 1,
				"createdAt":                     1,
				"paid":                          1,
				"paidAt":                        1,
				"companyName":                   "$company.name",
				"planName":                      "$plan.name",
				"planDuration":                  "$plan.duration",
				"subscriptionType":              "$subscription.type",
				"subscriptionStatus":            "$subscription.status",
			},
		},
		{
			"$sort": bson.M{"createdAt": -1},
		},
	}

	cursor, err := sc.DB.Collection("commissions").Aggregate(ctx, pipeline)
	if err != nil {
		log.Printf("Error aggregating commissions: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve commission details",
		})
	}
	defer cursor.Close(ctx)

	var commissions []bson.M
	if err = cursor.All(ctx, &commissions); err != nil {
		log.Printf("Error decoding commissions: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode commission details",
		})
	}

	// Calculate summary statistics
	var totalCommission float64
	var totalPaidCommission float64
	var totalUnpaidCommission float64
	var commissionCount int

	for _, commission := range commissions {
		var commissionAmount float64
		if claims.UserType == "salesperson" {
			if amount, ok := commission["salespersonCommission"].(float64); ok {
				commissionAmount = amount
			}
		} else {
			if amount, ok := commission["salesManagerCommission"].(float64); ok {
				commissionAmount = amount
			}
		}

		totalCommission += commissionAmount
		commissionCount++

		if paid, ok := commission["paid"].(bool); ok && paid {
			totalPaidCommission += commissionAmount
		} else {
			totalUnpaidCommission += commissionAmount
		}
	}

	// Create user-specific message
	var userTypeDisplay string
	switch claims.UserType {
	case "salesperson":
		userTypeDisplay = "salesperson"
	case "admin":
		userTypeDisplay = "admin"
	case "sales_manager":
		userTypeDisplay = "sales manager"
	default:
		userTypeDisplay = claims.UserType
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: fmt.Sprintf("Commission details retrieved successfully for %s", userTypeDisplay),
		Data: map[string]interface{}{
			"commissions": commissions,
			"summary": map[string]interface{}{
				"totalCommission":       totalCommission,
				"totalPaidCommission":   totalPaidCommission,
				"totalUnpaidCommission": totalUnpaidCommission,
				"commissionCount":       commissionCount,
			},
		},
	})
}

// GetBranchSubscriptionRequestStatus gets the status of a branch's subscription request
func (sc *BranchSubscriptionController) GetBranchSubscriptionRequestStatus(c echo.Context) error {
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

	// Get branch ID from URL parameter
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

	// Verify the branch belongs to the company
	companyCollection := sc.DB.Collection("companies")
	var company models.Company
	err = companyCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&company)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Company not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find company",
		})
	}

	// Verify branch belongs to this company
	branchFound := false
	for _, b := range company.Branches {
		if b.ID == branchObjectID {
			branchFound = true
			break
		}
	}

	if !branchFound {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Branch does not belong to your company",
		})
	}

	// Find the latest subscription request for this branch
	subscriptionRequestsCollection := sc.DB.Collection("branch_subscription_requests")
	var subscriptionRequest models.BranchSubscriptionRequest
	err = subscriptionRequestsCollection.FindOne(ctx,
		bson.M{"branchId": branchObjectID},
		options.FindOne().SetSort(bson.D{{"requestedAt", -1}}),
	).Decode(&subscriptionRequest)

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

	// If payment status is pending, automatically verify payment and activate if successful
	if subscriptionRequest.PaymentStatus == "pending" && subscriptionRequest.ExternalID != 0 {
		log.Printf("🔄 Auto-verifying payment for subscription request: %s (externalId: %d)", subscriptionRequest.ID.Hex(), subscriptionRequest.ExternalID)
		
		// Initialize Whish service and verify payment status
		whishService := services.NewWhishService()
		status, phoneNumber, err := whishService.GetPaymentStatus("USD", subscriptionRequest.ExternalID)
		if err != nil {
			log.Printf("⚠️  Failed to auto-verify payment status: %v", err)
			// Continue to return the current status even if verification fails
		} else {
			log.Printf("📊 Auto-verification result: status=%s, phone=%s", status, phoneNumber)
			
			// If payment is successful, activate the subscription
			if status == "success" {
				log.Printf("✅ Payment verified as successful, activating subscription...")
				
				// Check if subscription already exists
				subscriptionsCollection := sc.DB.Collection("branch_subscriptions")
				var existingSubscription models.BranchSubscription
				err = subscriptionsCollection.FindOne(ctx, bson.M{
					"branchId": branchObjectID,
					"status":   "active",
				}).Decode(&existingSubscription)
				
				if err != nil {
					// No active subscription exists, activate it
					err = sc.activateBranchSubscription(ctx, subscriptionRequest, phoneNumber)
					if err != nil {
						log.Printf("❌ Failed to auto-activate subscription: %v", err)
					} else {
						log.Printf("✅ Subscription auto-activated successfully")
						// Reload the subscription request to get updated status
						err = subscriptionRequestsCollection.FindOne(ctx, bson.M{"_id": subscriptionRequest.ID}).Decode(&subscriptionRequest)
						if err != nil {
							log.Printf("Warning: Failed to reload subscription request after activation: %v", err)
						}
					}
				} else {
					log.Printf("ℹ️  Subscription already active, updating request status")
					// Update request status even if subscription already exists
					subscriptionRequestsCollection.UpdateOne(ctx,
						bson.M{"_id": subscriptionRequest.ID},
						bson.M{"$set": bson.M{
							"paymentStatus": "success",
							"status":        "active",
							"paidAt":        time.Now(),
							"processedAt":   time.Now(),
						}})
					// Reload the subscription request
					err = subscriptionRequestsCollection.FindOne(ctx, bson.M{"_id": subscriptionRequest.ID}).Decode(&subscriptionRequest)
					if err != nil {
						log.Printf("Warning: Failed to reload subscription request: %v", err)
					}
				}
			} else if status == "failed" {
				// Update request status to failed
				subscriptionRequestsCollection.UpdateOne(ctx,
					bson.M{"_id": subscriptionRequest.ID},
					bson.M{"$set": bson.M{
						"paymentStatus": "failed",
						"status":        "failed",
						"processedAt":   time.Now(),
					}})
				// Reload the subscription request
				err = subscriptionRequestsCollection.FindOne(ctx, bson.M{"_id": subscriptionRequest.ID}).Decode(&subscriptionRequest)
				if err != nil {
					log.Printf("Warning: Failed to reload subscription request: %v", err)
				}
			}
		}
	}

	// Get plan details
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

// GetBranchSubscriptionRemainingTime retrieves the remaining time of the current active branch subscription
func (sc *BranchSubscriptionController) GetBranchSubscriptionRemainingTime(c echo.Context) error {
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

	// Find company by user ID to verify ownership
	companyCollection := sc.DB.Collection("companies")
	var company models.Company
	err = companyCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&company)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Company not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find company",
		})
	}

	// Verify that the branch belongs to this company
	branchFound := false
	for _, branch := range company.Branches {
		if branch.ID == branchObjectID {
			branchFound = true
			break
		}
	}

	if !branchFound {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Branch does not belong to this company",
		})
	}

	// Find active branch subscription
	subscriptionsCollection := sc.DB.Collection("branch_subscriptions")
	var subscription models.BranchSubscription
	err = subscriptionsCollection.FindOne(ctx, bson.M{
		"branchId": branchObjectID,
		"status":   "active",
		"endDate":  bson.M{"$gt": time.Now()},
	}).Decode(&subscription)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusOK, models.Response{
				Status:  http.StatusOK,
				Message: "No active branch subscription found",
				Data: map[string]interface{}{
					"hasActiveSubscription": false,
					"remainingTime":         nil,
				},
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find branch subscription",
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
		Message: "Branch subscription remaining time retrieved successfully",
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

// VerifyAndActivateBranchSubscription manually verifies payment status and activates subscription if payment was successful
// This is useful when the payment callback wasn't called by Whish
func (sc *BranchSubscriptionController) VerifyAndActivateBranchSubscription(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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

	// Get branch ID from URL parameter
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

	// Verify the branch belongs to the company
	companyCollection := sc.DB.Collection("companies")
	var company models.Company
	err = companyCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&company)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Company not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find company",
		})
	}

	// Verify branch belongs to this company
	branchFound := false
	for _, b := range company.Branches {
		if b.ID == branchObjectID {
			branchFound = true
			break
		}
	}

	if !branchFound {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Branch does not belong to your company",
		})
	}

	// Find the latest subscription request for this branch
	subscriptionRequestsCollection := sc.DB.Collection("branch_subscription_requests")
	var subscriptionRequest models.BranchSubscriptionRequest
	err = subscriptionRequestsCollection.FindOne(ctx,
		bson.M{"branchId": branchObjectID},
		options.FindOne().SetSort(bson.D{{"requestedAt", -1}}),
	).Decode(&subscriptionRequest)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "No subscription request found for this branch",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find subscription request",
		})
	}

	// Check if already activated
	if subscriptionRequest.PaymentStatus == "success" && subscriptionRequest.Status == "active" {
		// Check if subscription exists
		subscriptionsCollection := sc.DB.Collection("branch_subscriptions")
		var existingSubscription models.BranchSubscription
		err = subscriptionsCollection.FindOne(ctx, bson.M{
			"branchId": branchObjectID,
			"status":   "active",
		}).Decode(&existingSubscription)

		if err == nil {
			return c.JSON(http.StatusOK, models.Response{
				Status:  http.StatusOK,
				Message: "Subscription is already active",
				Data: map[string]interface{}{
					"subscription": existingSubscription,
				},
			})
		}
	}

	// Check if externalId exists
	if subscriptionRequest.ExternalID == 0 {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "No payment external ID found for this subscription request",
		})
	}

	log.Printf("🔄 Verifying payment status for externalId: %d", subscriptionRequest.ExternalID)

	// Initialize Whish service and verify payment status
	whishService := services.NewWhishService()
	status, phoneNumber, err := whishService.GetPaymentStatus("USD", subscriptionRequest.ExternalID)
	if err != nil {
		log.Printf("Failed to verify payment status: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: fmt.Sprintf("Failed to verify payment: %v", err),
		})
	}

	log.Printf("📊 Payment status from Whish API: %s (Phone: %s)", status, phoneNumber)

	if status != "success" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: fmt.Sprintf("Payment not successful. Status: %s", status),
			Data: map[string]interface{}{
				"paymentStatus": status,
				"phoneNumber":   phoneNumber,
			},
		})
	}

	// Payment verified successfully - proceed to activate subscription
	log.Printf("🔄 Activating branch subscription...")
	err = sc.activateBranchSubscription(ctx, subscriptionRequest, phoneNumber)
	if err != nil {
		log.Printf("Failed to activate subscription: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: fmt.Sprintf("Failed to activate subscription: %v", err),
		})
	}

	log.Printf("✅ Subscription activated successfully for branch: %s", branchID)

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Payment verified and subscription activated successfully",
		Data: map[string]interface{}{
			"branchId":    branchID,
			"externalId":  subscriptionRequest.ExternalID,
			"phoneNumber": phoneNumber,
		},
	})
}

// addSubscriptionIncomeToAdminWallet adds subscription income to the admin wallet
func (sc *SubscriptionController) addSubscriptionIncomeToAdminWallet(ctx context.Context, amount float64, entityID primitive.ObjectID, entityType, companyName, branchName string) error {
	// Create admin wallet transaction
	adminWalletTransaction := models.AdminWallet{
		ID:          primitive.NewObjectID(),
		Type:        "subscription_income",
		Amount:      amount,
		Description: fmt.Sprintf("Subscription income from %s - %s (%s)", companyName, branchName, entityType),
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

// addCommissionToSalespersonWallet adds commission amount to salesperson's wallet
func (sc *SubscriptionController) addCommissionToSalespersonWallet(ctx context.Context, salespersonID primitive.ObjectID, amount float64, subscriptionID primitive.ObjectID, entityType, companyName string) error {
	// Create commission record for salesperson wallet
	commissionRecord := models.CommissionRecord{
		ID:             primitive.NewObjectID(),
		SubscriptionID: subscriptionID,
		SalespersonID:  salespersonID,
		Amount:         amount,
		Role:           "salesperson",
		Status:         "pending", // Will be marked as paid when processed
		CreatedAt:      time.Now(),
	}

	// Insert the commission record
	_, err := sc.DB.Collection("commission_records").InsertOne(ctx, commissionRecord)
	if err != nil {
		return fmt.Errorf("failed to insert salesperson commission record: %w", err)
	}

	// Update salesperson's commission balance
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
		amount, salespersonID.Hex(), entityType, companyName)
	return nil
}

// addCommissionToSalesManagerWallet adds commission amount to sales manager's wallet
func (sc *SubscriptionController) addCommissionToSalesManagerWallet(ctx context.Context, salesManagerID primitive.ObjectID, amount float64, subscriptionID primitive.ObjectID, entityType, companyName string) error {
	// Create commission record for sales manager wallet
	commissionRecord := models.CommissionRecord{
		ID:             primitive.NewObjectID(),
		SubscriptionID: subscriptionID,
		SalesManagerID: salesManagerID,
		Amount:         amount,
		Role:           "sales_manager",
		Status:         "pending", // Will be marked as paid when processed
		CreatedAt:      time.Now(),
	}

	// Insert the commission record
	_, err := sc.DB.Collection("commission_records").InsertOne(ctx, commissionRecord)
	if err != nil {
		return fmt.Errorf("failed to insert sales manager commission record: %w", err)
	}

	// Update sales manager's commission balance
	_, err = sc.DB.Collection("sales_managers").UpdateOne(
		ctx,
		bson.M{"_id": salesManagerID},
		bson.M{
			"$inc": bson.M{"commissionBalance": amount},
			"$set": bson.M{"updatedAt": time.Now()},
		},
	)
	if err != nil {
		return fmt.Errorf("failed to update sales manager commission balance: %w", err)
	}

	log.Printf("Commission $%.2f added to sales manager wallet (ID: %s) from %s - %s",
		amount, salesManagerID.Hex(), entityType, companyName)
	return nil
}
