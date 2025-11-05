package controllers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/HSouheill/barrim_backend/models"
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type SponsorshipSubscriptionController struct {
	DB *mongo.Database
}

func NewSponsorshipSubscriptionController(db *mongo.Database) *SponsorshipSubscriptionController {
	return &SponsorshipSubscriptionController{DB: db}
}

// CreateSponsorshipSubscriptionRequest creates a new sponsorship subscription request
func (ssc *SponsorshipSubscriptionController) CreateSponsorshipSubscriptionRequest(c echo.Context) error {
	var req models.SponsorshipSubscriptionRequestRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	// Validate request
	if err := c.Validate(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Validation failed",
			"error":   err.Error(),
		})
	}

	// Check if sponsorship exists and is valid
	sponsorshipCollection := ssc.DB.Collection("sponsorships")
	var sponsorship models.Sponsorship
	err := sponsorshipCollection.FindOne(context.Background(), bson.M{"_id": req.SponsorshipID}).Decode(&sponsorship)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, map[string]interface{}{
				"success": false,
				"message": "Sponsorship not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to retrieve sponsorship",
			"error":   err.Error(),
		})
	}

	// Check if sponsorship is still valid (not expired)
	if time.Now().After(sponsorship.EndDate) {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Sponsorship has expired",
		})
	}

	// Get entity name based on entity type
	entityName, err := ssc.getEntityName(req.EntityType, req.EntityID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Invalid entity",
			"error":   err.Error(),
		})
	}

	// Check if there's already a pending or active subscription for this entity and sponsorship
	subscriptionCollection := ssc.DB.Collection("sponsorship_subscriptions")
	var existingSubscription models.SponsorshipSubscription
	err = subscriptionCollection.FindOne(context.Background(), bson.M{
		"sponsorshipId": req.SponsorshipID,
		"entityId":      req.EntityID,
		"entityType":    req.EntityType,
		"status":        bson.M{"$in": []string{"active", "pending"}},
	}).Decode(&existingSubscription)
	if err == nil {
		return c.JSON(http.StatusConflict, map[string]interface{}{
			"success": false,
			"message": "Entity already has a pending or active subscription for this sponsorship",
		})
	}

	// Also check if there's already a pending request for this entity and sponsorship
	existingRequestCollection := ssc.DB.Collection("sponsorship_subscription_requests")
	var existingRequest models.SponsorshipSubscriptionRequest
	err = existingRequestCollection.FindOne(context.Background(), bson.M{
		"sponsorshipId": req.SponsorshipID,
		"entityId":      req.EntityID,
		"entityType":    req.EntityType,
		"status":        "pending",
	}).Decode(&existingRequest)
	if err == nil {
		return c.JSON(http.StatusConflict, map[string]interface{}{
			"success": false,
			"message": "Entity already has a pending subscription request for this sponsorship",
		})
	}

	// Create subscription request
	subscriptionRequest := models.SponsorshipSubscriptionRequest{
		SponsorshipID:   req.SponsorshipID,
		EntityType:      req.EntityType,
		EntityID:        req.EntityID,
		EntityName:      entityName,
		Status:          "pending",
		RequestedAt:     time.Now(),
		AdminApproved:   nil,
		ManagerApproved: nil,
	}

	// Insert into database
	requestCollection := ssc.DB.Collection("sponsorship_subscription_requests")
	result, err := requestCollection.InsertOne(context.Background(), subscriptionRequest)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to create subscription request",
			"error":   err.Error(),
		})
	}

	subscriptionRequest.ID = result.InsertedID.(primitive.ObjectID)

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"success": true,
		"message": "Sponsorship subscription request created successfully",
		"request": subscriptionRequest,
	})
}

// GetPendingSponsorshipSubscriptionRequests retrieves all pending sponsorship subscription requests (admin only)
func (ssc *SponsorshipSubscriptionController) GetPendingSponsorshipSubscriptionRequests(c echo.Context) error {
	collection := ssc.DB.Collection("sponsorship_subscription_requests")

	// Parse query parameters
	page, _ := strconv.Atoi(c.QueryParam("page"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	entityType := c.QueryParam("entityType")

	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 10
	}

	// Build filter
	filter := bson.M{"status": "pending"}
	if entityType != "" {
		filter["entityType"] = entityType
	}

	// Count total documents
	total, err := collection.CountDocuments(context.Background(), filter)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to count subscription requests",
			"error":   err.Error(),
		})
	}

	// Set up pagination
	skip := (page - 1) * limit
	opts := options.Find().
		SetSkip(int64(skip)).
		SetLimit(int64(limit)).
		SetSort(bson.D{{Key: "requestedAt", Value: -1}})

	// Find requests
	cursor, err := collection.Find(context.Background(), filter, opts)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to retrieve subscription requests",
			"error":   err.Error(),
		})
	}
	defer cursor.Close(context.Background())

	var requests []models.SponsorshipSubscriptionRequest
	if err = cursor.All(context.Background(), &requests); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to decode subscription requests",
			"error":   err.Error(),
		})
	}

	// Enhance requests with sponsorship and entity details
	enhancedRequests := make([]map[string]interface{}, 0, len(requests))
	for _, request := range requests {
		enhancedRequest := map[string]interface{}{
			"request": request,
		}

		// Get sponsorship details
		sponsorshipCollection := ssc.DB.Collection("sponsorships")
		var sponsorship models.Sponsorship
		err := sponsorshipCollection.FindOne(context.Background(), bson.M{"_id": request.SponsorshipID}).Decode(&sponsorship)
		if err == nil {
			enhancedRequest["sponsorship"] = map[string]interface{}{
				"id":        sponsorship.ID,
				"title":     sponsorship.Title,
				"price":     sponsorship.Price,
				"duration":  sponsorship.Duration,
				"discount":  sponsorship.Discount,
				"startDate": sponsorship.StartDate,
				"endDate":   sponsorship.EndDate,
			}
		} else {
			enhancedRequest["sponsorship"] = nil
			log.Printf("Warning: Failed to retrieve sponsorship details for ID %s: %v", request.SponsorshipID.Hex(), err)
		}

		// Get entity details based on entity type
		entityDetails, err := ssc.getEnhancedEntityDetails(request.EntityType, request.EntityID)
		if err == nil {
			enhancedRequest["entityDetails"] = entityDetails
		} else {
			enhancedRequest["entityDetails"] = nil
			log.Printf("Warning: Failed to retrieve entity details for type %s, ID %s: %v", request.EntityType, request.EntityID.Hex(), err)
		}

		enhancedRequests = append(enhancedRequests, enhancedRequest)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"requests": enhancedRequests,
			"pagination": map[string]interface{}{
				"page":  page,
				"limit": limit,
				"total": total,
				"pages": (total + int64(limit) - 1) / int64(limit),
			},
		},
	})
}

// ProcessSponsorshipSubscriptionRequest processes a sponsorship subscription request (admin only)
func (ssc *SponsorshipSubscriptionController) ProcessSponsorshipSubscriptionRequest(c echo.Context) error {
	requestID := c.Param("id")
	if requestID == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Request ID is required",
		})
	}

	objectID, err := primitive.ObjectIDFromHex(requestID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Invalid request ID format",
		})
	}

	var req models.SponsorshipSubscriptionApprovalRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	// Validate status
	if req.Status != "approved" && req.Status != "rejected" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Status must be 'approved' or 'rejected'",
		})
	}

	// Get the subscription request
	requestCollection := ssc.DB.Collection("sponsorship_subscription_requests")
	var subscriptionRequest models.SponsorshipSubscriptionRequest
	err = requestCollection.FindOne(context.Background(), bson.M{"_id": objectID}).Decode(&subscriptionRequest)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, map[string]interface{}{
				"success": false,
				"message": "Subscription request not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to retrieve subscription request",
			"error":   err.Error(),
		})
	}

	// Check if request is already processed
	if subscriptionRequest.Status != "pending" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Request is already processed",
		})
	}

	// Get admin ID from context (set by middleware)
	adminID := c.Get("userId").(string)

	// Update the request
	update := bson.M{
		"status":      req.Status,
		"processedAt": time.Now(),
	}

	if req.Status == "approved" {
		update["adminApproved"] = true
		update["approvedBy"] = adminID
		update["approvedAt"] = time.Now()
		update["adminNote"] = req.AdminNote
	} else {
		update["adminApproved"] = false
		update["rejectedBy"] = adminID
		update["rejectedAt"] = time.Now()
		update["adminNote"] = req.AdminNote
	}

	_, err = requestCollection.UpdateOne(
		context.Background(),
		bson.M{"_id": objectID},
		bson.M{"$set": update},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to update subscription request",
			"error":   err.Error(),
		})
	}

	// If approved, create the actual subscription and update entity sponsorship status
	if req.Status == "approved" {
		err = ssc.createActiveSubscription(context.Background(), subscriptionRequest)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]interface{}{
				"success": false,
				"message": "Failed to create active subscription",
				"error":   err.Error(),
			})
		}

		// Update entity sponsorship status to true
		err = ssc.updateEntitySponsorshipStatus(context.Background(), subscriptionRequest.EntityType, subscriptionRequest.EntityID, true)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]interface{}{
				"success": false,
				"message": "Failed to update entity sponsorship status",
				"error":   err.Error(),
			})
		}

		// Add sponsorship price to admin wallet only if payment hasn't been processed yet
		// (Payment success callback already adds to wallet for paid requests)
		if subscriptionRequest.PaymentStatus != "success" {
			err = ssc.addSponsorshipToAdminWallet(subscriptionRequest.SponsorshipID)
			if err != nil {
				log.Printf("Warning: Failed to add sponsorship to admin wallet: %v", err)
				// Don't fail the approval if wallet update fails
			}
		} else {
			log.Printf("Skipping admin wallet addition - payment already processed and added to wallet")
		}
	} else {
		// If rejected, update entity sponsorship status to false
		err = ssc.updateEntitySponsorshipStatus(context.Background(), subscriptionRequest.EntityType, subscriptionRequest.EntityID, false)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]interface{}{
				"success": false,
				"message": "Failed to update entity sponsorship status",
				"error":   err.Error(),
			})
		}
	}

	statusText := "approved"
	if req.Status == "rejected" {
		statusText = "rejected"
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Sponsorship subscription request %s successfully", statusText),
	})
}

// GetActiveSponsorshipSubscriptions retrieves all active sponsorship subscriptions
func (ssc *SponsorshipSubscriptionController) GetActiveSponsorshipSubscriptions(c echo.Context) error {
	collection := ssc.DB.Collection("sponsorship_subscriptions")

	// Parse query parameters
	page, _ := strconv.Atoi(c.QueryParam("page"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	entityType := c.QueryParam("entityType")
	sponsorshipID := c.QueryParam("sponsorshipId")

	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 10
	}

	// Build filter
	filter := bson.M{"status": "active"}
	if entityType != "" {
		filter["entityType"] = entityType
	}
	if sponsorshipID != "" {
		sponsorshipObjectID, err := primitive.ObjectIDFromHex(sponsorshipID)
		if err == nil {
			filter["sponsorshipId"] = sponsorshipObjectID
		}
	}

	// Count total documents
	total, err := collection.CountDocuments(context.Background(), filter)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to count subscriptions",
			"error":   err.Error(),
		})
	}

	// Set up pagination
	skip := (page - 1) * limit
	opts := options.Find().
		SetSkip(int64(skip)).
		SetLimit(int64(limit)).
		SetSort(bson.D{{Key: "createdAt", Value: -1}})

	// Find subscriptions
	cursor, err := collection.Find(context.Background(), filter, opts)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to retrieve subscriptions",
			"error":   err.Error(),
		})
	}
	defer cursor.Close(context.Background())

	var subscriptions []models.SponsorshipSubscription
	if err = cursor.All(context.Background(), &subscriptions); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to decode subscriptions",
			"error":   err.Error(),
		})
	}

	// Enhance subscriptions with sponsorship and entity details
	enhancedSubscriptions := make([]map[string]interface{}, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		enhancedSubscription := map[string]interface{}{
			"subscription": subscription,
		}

		// Get sponsorship details
		sponsorshipCollection := ssc.DB.Collection("sponsorships")
		var sponsorship models.Sponsorship
		err := sponsorshipCollection.FindOne(context.Background(), bson.M{"_id": subscription.SponsorshipID}).Decode(&sponsorship)
		if err == nil {
			enhancedSubscription["sponsorship"] = map[string]interface{}{
				"id":        sponsorship.ID,
				"title":     sponsorship.Title,
				"price":     sponsorship.Price,
				"duration":  sponsorship.Duration,
				"discount":  sponsorship.Discount,
				"startDate": sponsorship.StartDate,
				"endDate":   sponsorship.EndDate,
			}
		} else {
			enhancedSubscription["sponsorship"] = nil
			log.Printf("Warning: Failed to retrieve sponsorship details for ID %s: %v", subscription.SponsorshipID.Hex(), err)
		}

		// Get entity details based on entity type
		entityDetails, err := ssc.getEnhancedEntityDetails(subscription.EntityType, subscription.EntityID)
		if err == nil {
			enhancedSubscription["entityDetails"] = entityDetails
		} else {
			enhancedSubscription["entityDetails"] = nil
			log.Printf("Warning: Failed to retrieve entity details for type %s, ID %s: %v", subscription.EntityType, subscription.EntityID.Hex(), err)
		}

		enhancedSubscriptions = append(enhancedSubscriptions, enhancedSubscription)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"subscriptions": enhancedSubscriptions,
			"pagination": map[string]interface{}{
				"page":  page,
				"limit": limit,
				"total": total,
				"pages": (total + int64(limit) - 1) / int64(limit),
			},
		},
	})
}

// GetTimeRemainingForCompanyBranch gets the time remaining for a company branch's sponsorship subscription
func (ssc *SponsorshipSubscriptionController) GetTimeRemainingForCompanyBranch(c echo.Context) error {
	branchID := c.Param("branchId")
	if branchID == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Branch ID is required",
		})
	}

	objectID, err := primitive.ObjectIDFromHex(branchID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Invalid branch ID format",
		})
	}

	// Find active subscription for this company branch
	collection := ssc.DB.Collection("sponsorship_subscriptions")
	var subscription models.SponsorshipSubscription
	err = collection.FindOne(context.Background(), bson.M{
		"entityType": "company_branch",
		"entityId":   objectID,
		"status":     "active",
	}).Decode(&subscription)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, map[string]interface{}{
				"success": false,
				"message": "No active sponsorship subscription found for this company branch",
			})
		}
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to retrieve subscription",
			"error":   err.Error(),
		})
	}

	// Calculate time remaining
	timeRemaining := subscription.EndDate.Sub(time.Now())

	// Check if subscription has expired
	if timeRemaining <= 0 {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": true,
			"data": map[string]interface{}{
				"hasActiveSubscription": false,
				"message":               "Subscription has expired",
				"subscription":          subscription,
			},
		})
	}

	// Get sponsorship details
	sponsorshipCollection := ssc.DB.Collection("sponsorships")
	var sponsorship models.Sponsorship
	err = sponsorshipCollection.FindOne(context.Background(), bson.M{"_id": subscription.SponsorshipID}).Decode(&sponsorship)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to retrieve sponsorship details",
			"error":   err.Error(),
		})
	}

	// Get company branch name
	var company models.Company
	err = ssc.DB.Collection("companies").FindOne(context.Background(), bson.M{"branches._id": objectID}).Decode(&company)
	var branchName string
	if err == nil {
		for _, branch := range company.Branches {
			if branch.ID == objectID {
				branchName = branch.Name
				break
			}
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"hasActiveSubscription": true,
			"timeRemaining": map[string]interface{}{
				"totalSeconds": int64(timeRemaining.Seconds()),
				"totalMinutes": int64(timeRemaining.Minutes()),
				"totalHours":   int64(timeRemaining.Hours()),
				"totalDays":    int64(timeRemaining.Hours() / 24),
				"formatted":    formatTimeRemaining(timeRemaining),
			},
			"subscription": subscription,
			"sponsorship":  sponsorship,
			"entityInfo": map[string]interface{}{
				"branchName":  branchName,
				"companyName": company.BusinessName,
			},
		},
	})
}

// GetTimeRemainingForWholesalerBranch gets the time remaining for a wholesaler branch's sponsorship subscription
func (ssc *SponsorshipSubscriptionController) GetTimeRemainingForWholesalerBranch(c echo.Context) error {
	branchID := c.Param("branchId")
	if branchID == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Branch ID is required",
		})
	}

	objectID, err := primitive.ObjectIDFromHex(branchID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Invalid branch ID format",
		})
	}

	// Find active subscription for this wholesaler branch
	collection := ssc.DB.Collection("sponsorship_subscriptions")
	var subscription models.SponsorshipSubscription
	err = collection.FindOne(context.Background(), bson.M{
		"entityType": "wholesaler_branch",
		"entityId":   objectID,
		"status":     "active",
	}).Decode(&subscription)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, map[string]interface{}{
				"success": false,
				"message": "No active sponsorship subscription found for this wholesaler branch",
			})
		}
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to retrieve subscription",
			"error":   err.Error(),
		})
	}

	// Calculate time remaining
	timeRemaining := subscription.EndDate.Sub(time.Now())

	// Check if subscription has expired
	if timeRemaining <= 0 {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": true,
			"data": map[string]interface{}{
				"hasActiveSubscription": false,
				"message":               "Subscription has expired",
				"subscription":          subscription,
			},
		})
	}

	// Get sponsorship details
	sponsorshipCollection := ssc.DB.Collection("sponsorships")
	var sponsorship models.Sponsorship
	err = sponsorshipCollection.FindOne(context.Background(), bson.M{"_id": subscription.SponsorshipID}).Decode(&sponsorship)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to retrieve sponsorship details",
			"error":   err.Error(),
		})
	}

	// Get wholesaler branch name
	var wholesaler models.Wholesaler
	err = ssc.DB.Collection("wholesalers").FindOne(context.Background(), bson.M{"branches._id": objectID}).Decode(&wholesaler)
	var branchName string
	if err == nil {
		for _, branch := range wholesaler.Branches {
			if branch.ID == objectID {
				branchName = branch.Name
				break
			}
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"hasActiveSubscription": true,
			"timeRemaining": map[string]interface{}{
				"totalSeconds": int64(timeRemaining.Seconds()),
				"totalMinutes": int64(timeRemaining.Minutes()),
				"totalHours":   int64(timeRemaining.Hours()),
				"totalDays":    int64(timeRemaining.Hours() / 24),
				"formatted":    formatTimeRemaining(timeRemaining),
			},
			"subscription": subscription,
			"sponsorship":  sponsorship,
			"entityInfo": map[string]interface{}{
				"branchName":     branchName,
				"wholesalerName": wholesaler.BusinessName,
			},
		},
	})
}

// GetTimeRemainingForServiceProvider gets the time remaining for a service provider's sponsorship subscription
func (ssc *SponsorshipSubscriptionController) GetTimeRemainingForServiceProvider(c echo.Context) error {
	serviceProviderID := c.Param("serviceProviderId")
	if serviceProviderID == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Service Provider ID is required",
		})
	}

	objectID, err := primitive.ObjectIDFromHex(serviceProviderID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Invalid service provider ID format",
		})
	}

	// Find active subscription for this service provider
	collection := ssc.DB.Collection("sponsorship_subscriptions")
	var subscription models.SponsorshipSubscription
	err = collection.FindOne(context.Background(), bson.M{
		"entityType": "service_provider",
		"entityId":   objectID,
		"status":     "active",
	}).Decode(&subscription)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, map[string]interface{}{
				"success": false,
				"message": "No active sponsorship subscription found for this service provider",
			})
		}
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to retrieve subscription",
			"error":   err.Error(),
		})
	}

	// Calculate time remaining
	timeRemaining := subscription.EndDate.Sub(time.Now())

	// Check if subscription has expired
	if timeRemaining <= 0 {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": true,
			"data": map[string]interface{}{
				"hasActiveSubscription": false,
				"message":               "Subscription has expired",
				"subscription":          subscription,
			},
		})
	}

	// Get sponsorship details
	sponsorshipCollection := ssc.DB.Collection("sponsorships")
	var sponsorship models.Sponsorship
	err = sponsorshipCollection.FindOne(context.Background(), bson.M{"_id": subscription.SponsorshipID}).Decode(&sponsorship)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to retrieve sponsorship details",
			"error":   err.Error(),
		})
	}

	// Get service provider details
	var serviceProvider models.ServiceProvider
	err = ssc.DB.Collection("serviceProviders").FindOne(context.Background(), bson.M{"_id": objectID}).Decode(&serviceProvider)
	if err != nil {
		// Try to find by userId field as fallback
		err = ssc.DB.Collection("serviceProviders").FindOne(context.Background(), bson.M{"userId": objectID}).Decode(&serviceProvider)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]interface{}{
				"success": false,
				"message": "Failed to retrieve service provider details",
				"error":   err.Error(),
			})
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"hasActiveSubscription": true,
			"timeRemaining": map[string]interface{}{
				"totalSeconds": int64(timeRemaining.Seconds()),
				"totalMinutes": int64(timeRemaining.Minutes()),
				"totalHours":   int64(timeRemaining.Hours()),
				"totalDays":    int64(timeRemaining.Hours() / 24),
				"formatted":    formatTimeRemaining(timeRemaining),
			},
			"subscription": subscription,
			"sponsorship":  sponsorship,
			"entityInfo": map[string]interface{}{
				"businessName": serviceProvider.BusinessName,
				"category":     serviceProvider.Category,
			},
		},
	})
}

// GetTimeRemainingForEntity gets the time remaining for any entity's sponsorship subscription
func (ssc *SponsorshipSubscriptionController) GetTimeRemainingForEntity(c echo.Context) error {
	entityType := c.Param("entityType")
	entityID := c.Param("entityId")

	if entityType == "" || entityID == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Entity type and entity ID are required",
		})
	}

	// Validate entity type (accept both camelCase and snake_case)
	validEntityTypes := map[string]bool{
		"service_provider":  true,
		"company_branch":    true,
		"wholesaler_branch": true,
		"serviceProvider":   true,
		"companyBranch":     true,
		"wholesalerBranch":  true,
	}

	if !validEntityTypes[entityType] {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Invalid entity type. Must be one of: service_provider/serviceProvider, company_branch/companyBranch, wholesaler_branch/wholesalerBranch",
		})
	}

	objectID, err := primitive.ObjectIDFromHex(entityID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Invalid entity ID format",
		})
	}

	// Find active subscription for this entity
	collection := ssc.DB.Collection("sponsorship_subscriptions")
	var subscription models.SponsorshipSubscription
	err = collection.FindOne(context.Background(), bson.M{
		"entityType": entityType,
		"entityId":   objectID,
		"status":     "active",
	}).Decode(&subscription)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, map[string]interface{}{
				"success": false,
				"message": fmt.Sprintf("No active sponsorship subscription found for this %s", entityType),
			})
		}
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to retrieve subscription",
			"error":   err.Error(),
		})
	}

	// Calculate time remaining
	timeRemaining := subscription.EndDate.Sub(time.Now())

	// Check if subscription has expired
	if timeRemaining <= 0 {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": true,
			"data": map[string]interface{}{
				"hasActiveSubscription": false,
				"message":               "Subscription has expired",
				"subscription":          subscription,
				"entityType":            entityType,
			},
		})
	}

	// Get sponsorship details
	sponsorshipCollection := ssc.DB.Collection("sponsorships")
	var sponsorship models.Sponsorship
	err = sponsorshipCollection.FindOne(context.Background(), bson.M{"_id": subscription.SponsorshipID}).Decode(&sponsorship)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to retrieve sponsorship details",
			"error":   err.Error(),
		})
	}

	// Get entity details based on type
	entityInfo, err := ssc.getEntityDetails(entityType, objectID)
	if err != nil {
		// Don't fail the request if we can't get entity details
		log.Printf("Warning: Failed to get entity details: %v", err)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"hasActiveSubscription": true,
			"timeRemaining": map[string]interface{}{
				"totalSeconds": int64(timeRemaining.Seconds()),
				"totalMinutes": int64(timeRemaining.Minutes()),
				"totalHours":   int64(timeRemaining.Hours()),
				"totalDays":    int64(timeRemaining.Hours() / 24),
				"formatted":    formatTimeRemaining(timeRemaining),
			},
			"subscription": subscription,
			"sponsorship":  sponsorship,
			"entityType":   entityType,
			"entityInfo":   entityInfo,
		},
	})
}

// Helper function to get entity name based on entity type and ID
func (ssc *SponsorshipSubscriptionController) getEntityName(entityType string, entityID primitive.ObjectID) (string, error) {
	// Normalize entity type to handle both camelCase and snake_case formats
	normalizedEntityType := ssc.normalizeEntityType(entityType)

	switch normalizedEntityType {
	case "service_provider":
		log.Printf("Looking for service provider with ID: %s", entityID.Hex())
		var serviceProvider models.ServiceProvider
		err := ssc.DB.Collection("serviceProviders").FindOne(context.Background(), bson.M{"_id": entityID}).Decode(&serviceProvider)
		if err != nil {
			log.Printf("Error finding service provider: %v", err)
			// Also try to find by userId field
			err2 := ssc.DB.Collection("serviceProviders").FindOne(context.Background(), bson.M{"userId": entityID}).Decode(&serviceProvider)
			if err2 != nil {
				log.Printf("Also not found by userId: %v", err2)
				return "", fmt.Errorf("service provider not found by _id or userId")
			}
			log.Printf("Found service provider by userId: %s", serviceProvider.BusinessName)
		} else {
			log.Printf("Found service provider by _id: %s", serviceProvider.BusinessName)
		}
		return serviceProvider.BusinessName, nil

	case "company_branch":
		var company models.Company
		err := ssc.DB.Collection("companies").FindOne(context.Background(), bson.M{"branches._id": entityID}).Decode(&company)
		if err != nil {
			return "", fmt.Errorf("company branch not found")
		}
		// Find the specific branch
		for _, branch := range company.Branches {
			if branch.ID == entityID {
				return fmt.Sprintf("%s - %s", company.BusinessName, branch.Name), nil
			}
		}
		return "", fmt.Errorf("company branch not found")

	case "wholesaler_branch":
		var wholesaler models.Wholesaler
		err := ssc.DB.Collection("wholesalers").FindOne(context.Background(), bson.M{"branches._id": entityID}).Decode(&wholesaler)
		if err != nil {
			return "", fmt.Errorf("wholesaler branch not found")
		}
		// Find the specific branch
		for _, branch := range wholesaler.Branches {
			if branch.ID == entityID {
				return fmt.Sprintf("%s - %s", wholesaler.BusinessName, branch.Name), nil
			}
		}
		return "", fmt.Errorf("wholesaler branch not found")

	default:
		return "", fmt.Errorf("invalid entity type")
	}
}

// Helper function to get entity details based on entity type and ID
func (ssc *SponsorshipSubscriptionController) getEntityDetails(entityType string, entityID primitive.ObjectID) (map[string]interface{}, error) {
	// Normalize entity type to handle both camelCase and snake_case formats
	normalizedEntityType := ssc.normalizeEntityType(entityType)

	switch normalizedEntityType {
	case "service_provider":
		var serviceProvider models.ServiceProvider
		err := ssc.DB.Collection("serviceProviders").FindOne(context.Background(), bson.M{"_id": entityID}).Decode(&serviceProvider)
		if err != nil {
			// Try to find by userId field as fallback
			err = ssc.DB.Collection("serviceProviders").FindOne(context.Background(), bson.M{"userId": entityID}).Decode(&serviceProvider)
			if err != nil {
				return nil, err
			}
		}
		return map[string]interface{}{
			"businessName": serviceProvider.BusinessName,
			"category":     serviceProvider.Category,
		}, nil

	case "company_branch":
		var company models.Company
		err := ssc.DB.Collection("companies").FindOne(context.Background(), bson.M{"branches._id": entityID}).Decode(&company)
		if err != nil {
			return nil, err
		}
		// Find the specific branch
		for _, branch := range company.Branches {
			if branch.ID == entityID {
				return map[string]interface{}{
					"branchName":  branch.Name,
					"companyName": company.BusinessName,
				}, nil
			}
		}
		return nil, fmt.Errorf("company branch not found")

	case "wholesaler_branch":
		var wholesaler models.Wholesaler
		err := ssc.DB.Collection("wholesalers").FindOne(context.Background(), bson.M{"branches._id": entityID}).Decode(&wholesaler)
		if err != nil {
			return nil, err
		}
		// Find the specific branch
		for _, branch := range wholesaler.Branches {
			if branch.ID == entityID {
				return map[string]interface{}{
					"branchName":     branch.Name,
					"wholesalerName": wholesaler.BusinessName,
				}, nil
			}
		}
		return nil, fmt.Errorf("wholesaler branch not found")

	default:
		return nil, fmt.Errorf("invalid entity type")
	}
}

// Helper function to create active subscription
func (ssc *SponsorshipSubscriptionController) createActiveSubscription(ctx context.Context, request models.SponsorshipSubscriptionRequest) error {
	// Get sponsorship details
	sponsorshipCollection := ssc.DB.Collection("sponsorships")
	var sponsorship models.Sponsorship
	err := sponsorshipCollection.FindOne(ctx, bson.M{"_id": request.SponsorshipID}).Decode(&sponsorship)
	if err != nil {
		return err
	}

	// Calculate start and end dates
	startDate := time.Now()
	endDate := startDate.AddDate(0, 0, sponsorship.Duration)

	// Create active subscription
	subscription := models.SponsorshipSubscription{
		SponsorshipID:   request.SponsorshipID,
		EntityType:      ssc.normalizeEntityType(request.EntityType), // Normalize to snake_case for consistency
		EntityID:        request.EntityID,
		StartDate:       startDate,
		EndDate:         endDate,
		Status:          "active",
		AutoRenew:       false,
		DiscountApplied: sponsorship.Discount,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	// Insert into database
	subscriptionCollection := ssc.DB.Collection("sponsorship_subscriptions")
	_, err = subscriptionCollection.InsertOne(ctx, subscription)
	if err != nil {
		return err
	}

	// Update sponsorship used count
	_, err = sponsorshipCollection.UpdateOne(
		ctx,
		bson.M{"_id": request.SponsorshipID},
		bson.M{"$inc": bson.M{"usedCount": 1}},
	)

	return err
}

// Helper function to normalize entity type (handle both camelCase and snake_case)
func (ssc *SponsorshipSubscriptionController) normalizeEntityType(entityType string) string {
	switch entityType {
	case "serviceProvider":
		return "service_provider"
	case "companyBranch":
		return "company_branch"
	case "wholesalerBranch":
		return "wholesaler_branch"
	default:
		return entityType
	}
}

// Helper function to update entity sponsorship status
func (ssc *SponsorshipSubscriptionController) updateEntitySponsorshipStatus(ctx context.Context, entityType string, entityID primitive.ObjectID, hasSponsorship bool) error {
	// Normalize entity type to handle both camelCase and snake_case formats
	normalizedEntityType := ssc.normalizeEntityType(entityType)

	switch normalizedEntityType {
	case "service_provider":
		// Update service provider sponsorship status
		_, err := ssc.DB.Collection("serviceProviders").UpdateOne(
			ctx,
			bson.M{"_id": entityID},
			bson.M{"$set": bson.M{
				"sponsorship": hasSponsorship,
				"updatedAt":   time.Now(),
			}},
		)
		return err

	case "company_branch":
		// Update company branch sponsorship status
		_, err := ssc.DB.Collection("companies").UpdateOne(
			ctx,
			bson.M{"branches._id": entityID},
			bson.M{"$set": bson.M{
				"branches.$.sponsorship": hasSponsorship,
				"branches.$.updatedAt":   time.Now(),
			}},
		)
		return err

	case "wholesaler_branch":
		// Update wholesaler branch sponsorship status
		_, err := ssc.DB.Collection("wholesalers").UpdateOne(
			ctx,
			bson.M{"branches._id": entityID},
			bson.M{"$set": bson.M{
				"branches.$.sponsorship": hasSponsorship,
				"branches.$.updatedAt":   time.Now(),
			}},
		)
		return err

	default:
		return fmt.Errorf("invalid entity type: %s", entityType)
	}
}

// addSponsorshipToAdminWallet adds the sponsorship price to the admin wallet
// This method creates a record of the sponsorship income for the admin wallet calculation
func (ssc *SponsorshipSubscriptionController) addSponsorshipToAdminWallet(sponsorshipID primitive.ObjectID) error {
	ctx := context.Background()
	// Get sponsorship details to get the price
	sponsorshipCollection := ssc.DB.Collection("sponsorships")
	var sponsorship models.Sponsorship
	err := sponsorshipCollection.FindOne(ctx, bson.M{"_id": sponsorshipID}).Decode(&sponsorship)
	if err != nil {
		return fmt.Errorf("failed to get sponsorship details: %v", err)
	}

	return ssc.addSponsorshipIncomeToAdminWallet(ctx, sponsorship.Price, sponsorshipID, sponsorship.Title)
}

// addSponsorshipIncomeToAdminWallet adds sponsorship income to the admin wallet
func (ssc *SponsorshipSubscriptionController) addSponsorshipIncomeToAdminWallet(ctx context.Context, amount float64, entityID primitive.ObjectID, description string) error {
	// Create admin wallet transaction
	adminWalletTransaction := models.AdminWallet{
		ID:          primitive.NewObjectID(),
		Type:        "subscription_income", // Use subscription_income type for sponsorship income
		Amount:      amount,
		Description: fmt.Sprintf("Sponsorship income: %s", description),
		EntityID:    entityID,
		EntityType:  "sponsorship",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Insert the transaction
	_, err := ssc.DB.Collection("admin_wallet").InsertOne(ctx, adminWalletTransaction)
	if err != nil {
		return fmt.Errorf("failed to insert admin wallet transaction: %w", err)
	}

	// Update or create admin wallet balance
	balanceCollection := ssc.DB.Collection("admin_wallet_balance")

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

	log.Printf("Sponsorship income added to admin wallet: $%.2f - %s", amount, description)
	return nil
}

// Helper function to format time remaining in a human-readable format
func formatTimeRemaining(duration time.Duration) string {
	if duration <= 0 {
		return "Expired"
	}

	days := int(duration.Hours() / 24)
	hours := int(duration.Hours()) % 24
	minutes := int(duration.Minutes()) % 60
	seconds := int(duration.Seconds()) % 60

	if days > 0 {
		if days == 1 {
			return fmt.Sprintf("1 day, %d hours", hours)
		}
		return fmt.Sprintf("%d days, %d hours", days, hours)
	} else if hours > 0 {
		if hours == 1 {
			return fmt.Sprintf("1 hour, %d minutes", minutes)
		}
		return fmt.Sprintf("%d hours, %d minutes", hours, minutes)
	} else if minutes > 0 {
		if minutes == 1 {
			return fmt.Sprintf("1 minute, %d seconds", seconds)
		}
		return fmt.Sprintf("%d minutes, %d seconds", minutes, seconds)
	} else {
		return fmt.Sprintf("%d seconds", seconds)
	}
}

// Helper function to get enhanced entity details including user information
func (ssc *SponsorshipSubscriptionController) getEnhancedEntityDetails(entityType string, entityID primitive.ObjectID) (map[string]interface{}, error) {
	// Normalize entity type to handle both camelCase and snake_case formats
	normalizedEntityType := ssc.normalizeEntityType(entityType)

	switch normalizedEntityType {
	case "service_provider":
		var serviceProvider models.ServiceProvider
		err := ssc.DB.Collection("serviceProviders").FindOne(context.Background(), bson.M{"_id": entityID}).Decode(&serviceProvider)
		if err != nil {
			// Try to find by userId field as fallback
			err = ssc.DB.Collection("serviceProviders").FindOne(context.Background(), bson.M{"userId": entityID}).Decode(&serviceProvider)
			if err != nil {
				return nil, err
			}
		}

		// Get user details if available
		var userDetails map[string]interface{}
		if !serviceProvider.UserID.IsZero() {
			var user models.User
			err := ssc.DB.Collection("users").FindOne(context.Background(), bson.M{"_id": serviceProvider.UserID}).Decode(&user)
			if err == nil {
				userDetails = map[string]interface{}{
					"id":        user.ID,
					"email":     user.Email,
					"fullName":  user.FullName,
					"phone":     user.Phone,
					"userType":  user.UserType,
					"isActive":  user.IsActive,
					"status":    user.Status,
					"createdAt": user.CreatedAt,
				}
			}
		}

		return map[string]interface{}{
			"entityType":    "service_provider",
			"id":            serviceProvider.ID,
			"businessName":  serviceProvider.BusinessName,
			"category":      serviceProvider.Category,
			"email":         serviceProvider.Email,
			"phone":         serviceProvider.Phone,
			"contactPerson": serviceProvider.ContactPerson,
			"contactPhone":  serviceProvider.ContactPhone,
			"country":       serviceProvider.Country,
			"governorate":   serviceProvider.Governorate,
			"district":      serviceProvider.District,
			"city":          serviceProvider.City,
			"logoURL":       serviceProvider.LogoURL,
			"status":        serviceProvider.Status,
			"sponsorship":   serviceProvider.Sponsorship,
			"createdAt":     serviceProvider.CreatedAt,
			"userDetails":   userDetails,
		}, nil

	case "company_branch":
		var company models.Company
		err := ssc.DB.Collection("companies").FindOne(context.Background(), bson.M{"branches._id": entityID}).Decode(&company)
		if err != nil {
			return nil, err
		}

		// Find the specific branch
		var branch models.Branch
		for _, b := range company.Branches {
			if b.ID == entityID {
				branch = b
				break
			}
		}

		// Get user details if available
		var userDetails map[string]interface{}
		if !company.UserID.IsZero() {
			var user models.User
			err := ssc.DB.Collection("users").FindOne(context.Background(), bson.M{"_id": company.UserID}).Decode(&user)
			if err == nil {
				userDetails = map[string]interface{}{
					"id":        user.ID,
					"email":     user.Email,
					"fullName":  user.FullName,
					"phone":     user.Phone,
					"userType":  user.UserType,
					"isActive":  user.IsActive,
					"status":    user.Status,
					"createdAt": user.CreatedAt,
				}
			}
		}

		return map[string]interface{}{
			"entityType":  "company_branch",
			"id":          branch.ID,
			"name":        branch.Name,
			"category":    branch.Category,
			"subCategory": branch.SubCategory,
			"description": branch.Description,
			"phone":       branch.Phone,
			"status":      branch.Status,
			"sponsorship": branch.Sponsorship,
			"createdAt":   branch.CreatedAt,
			"companyInfo": map[string]interface{}{
				"id":            company.ID,
				"businessName":  company.BusinessName,
				"category":      company.Category,
				"email":         company.Email,
				"contactPerson": company.ContactPerson,
				"logoURL":       company.LogoURL,
				"sponsorship":   company.Sponsorship,
			},
			"userDetails": userDetails,
		}, nil

	case "wholesaler_branch":
		var wholesaler models.Wholesaler
		err := ssc.DB.Collection("wholesalers").FindOne(context.Background(), bson.M{"branches._id": entityID}).Decode(&wholesaler)
		if err != nil {
			return nil, err
		}

		// Find the specific branch
		var branch models.Branch
		for _, b := range wholesaler.Branches {
			if b.ID == entityID {
				branch = b
				break
			}
		}

		// Get user details if available
		var userDetails map[string]interface{}
		if !wholesaler.UserID.IsZero() {
			var user models.User
			err := ssc.DB.Collection("users").FindOne(context.Background(), bson.M{"_id": wholesaler.UserID}).Decode(&user)
			if err == nil {
				userDetails = map[string]interface{}{
					"id":        user.ID,
					"email":     user.Email,
					"fullName":  user.FullName,
					"phone":     user.Phone,
					"userType":  user.UserType,
					"isActive":  user.IsActive,
					"status":    user.Status,
					"createdAt": user.CreatedAt,
				}
			}
		}

		return map[string]interface{}{
			"entityType":  "wholesaler_branch",
			"id":          branch.ID,
			"name":        branch.Name,
			"category":    branch.Category,
			"subCategory": branch.SubCategory,
			"description": branch.Description,
			"phone":       branch.Phone,
			"status":      branch.Status,
			"sponsorship": branch.Sponsorship,
			"createdAt":   branch.CreatedAt,
			"wholesalerInfo": map[string]interface{}{
				"id":           wholesaler.ID,
				"businessName": wholesaler.BusinessName,
				"category":     wholesaler.Category,
				"phone":        wholesaler.Phone,
				"logoURL":      wholesaler.LogoURL,
				"sponsorship":  wholesaler.Sponsorship,
			},
			"userDetails": userDetails,
		}, nil

	default:
		return nil, fmt.Errorf("invalid entity type: %s", entityType)
	}
}
