package controllers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/HSouheill/barrim_backend/middleware"
	"github.com/HSouheill/barrim_backend/models"
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// ApprovalController handles approval requests for companies, service providers, and wholesalers
type ApprovalController struct {
	DB *mongo.Database
}

// NewApprovalController creates a new approval controller
func NewApprovalController(db *mongo.Database) *ApprovalController {
	return &ApprovalController{DB: db}
}

// GetPendingApprovalRequests retrieves all pending approval requests
func (ac *ApprovalController) GetPendingApprovalRequests(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user information from token
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "admin" && claims.UserType != "manager" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins and managers can view approval requests",
		})
	}

	// Get pending approval requests
	collection := ac.DB.Collection("approval_requests")
	cursor, err := collection.Find(ctx, bson.M{"status": "pending"})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve approval requests",
		})
	}
	defer cursor.Close(ctx)

	var approvalRequests []models.ApprovalRequest
	if err = cursor.All(ctx, &approvalRequests); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode approval requests",
		})
	}

	// Enrich requests with entity details
	var enrichedRequests []map[string]interface{}
	for _, req := range approvalRequests {
		enrichedRequest := map[string]interface{}{
			"approvalRequest": req,
		}

		// Get entity details based on entity type
		switch req.EntityType {
		case "company":
			var company models.Company
			err = ac.DB.Collection("companies").FindOne(ctx, bson.M{"_id": req.EntityID}).Decode(&company)
			if err == nil {
				enrichedRequest["entity"] = map[string]interface{}{
					"businessName": company.BusinessName,
					"category":     company.Category,
					"phone":        company.ContactInfo.Phone,
					"email":        "", // Companies don't have direct email in contact info
				}
			}
		case "wholesaler":
			var wholesaler models.Wholesaler
			err = ac.DB.Collection("wholesalers").FindOne(ctx, bson.M{"_id": req.EntityID}).Decode(&wholesaler)
			if err == nil {
				enrichedRequest["entity"] = map[string]interface{}{
					"businessName": wholesaler.BusinessName,
					"category":     wholesaler.Category,
					"phone":        wholesaler.Phone,
					"email":        "", // Wholesalers don't have direct email in contact info
				}
			}
		case "serviceProvider":
			var serviceProvider models.ServiceProvider
			err = ac.DB.Collection("serviceProviders").FindOne(ctx, bson.M{"_id": req.EntityID}).Decode(&serviceProvider)
			if err == nil {
				enrichedRequest["entity"] = map[string]interface{}{
					"businessName": serviceProvider.BusinessName,
					"category":     serviceProvider.Category,
					"phone":        serviceProvider.ContactInfo.Phone,
					"email":        "", // Service providers don't have direct email in contact info
				}
			}
		}

		enrichedRequests = append(enrichedRequests, enrichedRequest)
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Pending approval requests retrieved successfully",
		Data:    enrichedRequests,
	})
}

// ProcessApprovalRequest handles the approval or rejection of an approval request
func (ac *ApprovalController) ProcessApprovalRequest(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user information from token
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "admin" && claims.UserType != "manager" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins and managers can process approval requests",
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
	var approvalReq models.ApprovalResponse
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

	// Get the approval request
	approvalRequestsCollection := ac.DB.Collection("approval_requests")
	var approvalRequest models.ApprovalRequest
	err = approvalRequestsCollection.FindOne(ctx, bson.M{"_id": requestObjectID}).Decode(&approvalRequest)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Approval request not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find approval request",
		})
	}

	// Check if request is already processed
	if approvalRequest.Status != "pending" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Approval request is already processed",
		})
	}

	// Convert user ID from string to ObjectID
	userObjectID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	// Update approval request based on user type
	update := bson.M{}
	if claims.UserType == "admin" {
		update = bson.M{
			"$set": bson.M{
				"adminId":       userObjectID,
				"adminNote":     approvalReq.Note,
				"adminApproved": approvalReq.Status == "approved",
				"processedAt":   time.Now(),
			},
		}
	} else if claims.UserType == "manager" {
		update = bson.M{
			"$set": bson.M{
				"managerId":       userObjectID,
				"managerNote":     approvalReq.Note,
				"managerApproved": approvalReq.Status == "approved",
				"processedAt":     time.Now(),
			},
		}
	}

	_, err = approvalRequestsCollection.UpdateOne(ctx, bson.M{"_id": requestObjectID}, update)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update approval request",
		})
	}

	// Get updated approval request to check if both admin and manager have approved
	var updatedRequest models.ApprovalRequest
	err = approvalRequestsCollection.FindOne(ctx, bson.M{"_id": requestObjectID}).Decode(&updatedRequest)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to get updated request",
		})
	}

	// Determine final status based on approvals
	finalStatus := "pending"
	if updatedRequest.AdminApproved && updatedRequest.ManagerApproved {
		finalStatus = "approved"
	} else if !updatedRequest.AdminApproved || !updatedRequest.ManagerApproved {
		if (updatedRequest.AdminApproved && !updatedRequest.ManagerApproved) || (!updatedRequest.AdminApproved && updatedRequest.ManagerApproved) {
			// One approved, one rejected
			finalStatus = "rejected"
		}
	}

	// Update the entity status if both have made their decision
	if finalStatus != "pending" {
		// Update entity status based on entity type
		var collectionName string
		switch updatedRequest.EntityType {
		case "company":
			collectionName = "companies"
		case "wholesaler":
			collectionName = "wholesalers"
		case "serviceProvider":
			collectionName = "serviceProviders"
		default:
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Invalid entity type",
			})
		}

		// Update entity status
		entityCollection := ac.DB.Collection(collectionName)
		entityUpdate := bson.M{
			"$set": bson.M{
				"status":    finalStatus,
				"updatedAt": time.Now(),
			},
		}

		_, err = entityCollection.UpdateOne(ctx, bson.M{"_id": updatedRequest.EntityID}, entityUpdate)
		if err != nil {
			log.Printf("Failed to update entity status: %v", err)
		}

		// Update approval request status
		_, err = approvalRequestsCollection.UpdateOne(ctx, bson.M{"_id": requestObjectID}, bson.M{
			"$set": bson.M{"status": finalStatus},
		})
		if err != nil {
			log.Printf("Failed to update approval request status: %v", err)
		}
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: fmt.Sprintf("Approval request %s successfully", approvalReq.Status),
		Data: map[string]interface{}{
			"requestId":    requestObjectID,
			"entityType":   updatedRequest.EntityType,
			"entityId":     updatedRequest.EntityID,
			"finalStatus":  finalStatus,
			"processedAt":  time.Now(),
			"approverNote": approvalReq.Note,
		},
	})
}

// GetApprovalRequestStatus gets the status of an approval request for a user
func (ac *ApprovalController) GetApprovalRequestStatus(c echo.Context) error {
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

	// Get approval request for this user
	collection := ac.DB.Collection("approval_requests")
	var approvalRequest models.ApprovalRequest
	singleResult := collection.FindOne(ctx, bson.M{"userId": userID})
	err = singleResult.Decode(&approvalRequest)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusOK, models.Response{
				Status:  http.StatusOK,
				Message: "No approval request found",
				Data: map[string]interface{}{
					"hasRequest": false,
				},
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find approval request",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Approval request status retrieved successfully",
		Data: map[string]interface{}{
			"hasRequest": true,
			"request":    approvalRequest,
		},
	})
}
