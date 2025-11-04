package controllers

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/HSouheill/barrim_backend/config"
	"github.com/HSouheill/barrim_backend/middleware"
	"github.com/HSouheill/barrim_backend/models"
)

// AdminBranchController handles branch-related admin operations
type AdminBranchController struct {
	DB *mongo.Client
}

// NewAdminBranchController creates a new admin branch controller
func NewAdminBranchController(db *mongo.Client) *AdminBranchController {
	return &AdminBranchController{DB: db}
}

// GetPendingBranchRequests retrieves all pending branch requests
func (abc *AdminBranchController) GetPendingBranchRequests(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get query parameters for pagination
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page <= 0 {
		page = 1
	}
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	skip := (page - 1) * limit

	// Find pending branch requests
	branchRequestsCollection := config.GetCollection(abc.DB, "branch_requests")
	filter := bson.M{"status": "pending"}
	opts := options.Find().
		SetSort(bson.M{"submittedAt": -1}).
		SetSkip(int64(skip)).
		SetLimit(int64(limit))

	cursor, err := branchRequestsCollection.Find(ctx, filter, opts)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve branch requests",
		})
	}
	defer cursor.Close(ctx)

	var requests []models.BranchRequest
	if err = cursor.All(ctx, &requests); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode branch requests",
		})
	}

	// Get total count for pagination
	total, err := branchRequestsCollection.CountDocuments(ctx, filter)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to count branch requests",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Branch requests retrieved successfully",
		Data: map[string]interface{}{
			"requests": requests,
			"total":    total,
			"page":     page,
			"limit":    limit,
		},
	})
}

// ProcessBranchRequest handles approving or rejecting a branch request
func (abc *AdminBranchController) ProcessBranchRequest(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Get admin information from token
	claims := middleware.GetUserFromToken(c)
	adminID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid admin ID",
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

	// Convert string request ID to ObjectID
	requestObjectID, err := primitive.ObjectIDFromHex(requestID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request ID format",
		})
	}

	// Parse request body
	var approvalRequest models.BranchApprovalRequest
	if err := c.Bind(&approvalRequest); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request data",
		})
	}

	// Validate status
	if approvalRequest.Status != "approved" && approvalRequest.Status != "rejected" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid status. Must be 'approved' or 'rejected'",
		})
	}

	// Get branch request
	branchRequestsCollection := config.GetCollection(abc.DB, "branch_requests")
	var branchRequest models.BranchRequest
	err = branchRequestsCollection.FindOne(ctx, bson.M{"_id": requestObjectID}).Decode(&branchRequest)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Branch request not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find branch request",
		})
	}

	// Check if request is already processed
	if branchRequest.Status != "pending" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Branch request is already processed",
		})
	}

	// Update branch request status
	update := bson.M{
		"$set": bson.M{
			"status":      approvalRequest.Status,
			"adminId":     adminID,
			"adminNote":   approvalRequest.AdminNote,
			"processedAt": time.Now(),
		},
	}

	_, err = branchRequestsCollection.UpdateOne(
		ctx,
		bson.M{"_id": requestObjectID},
		update,
	)
	if err != nil {
		log.Printf("Error updating branch request: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update branch request: " + err.Error(),
		})
	}

	// If approved, add the branch to the company
	if approvalRequest.Status == "approved" {
		// Update branch status
		branchRequest.BranchData.Status = "approved"

		// Add branch to company
		companyCollection := config.GetCollection(abc.DB, "companies")
		_, err = companyCollection.UpdateOne(
			ctx,
			bson.M{"_id": branchRequest.CompanyID},
			bson.M{
				"$push": bson.M{
					"branches": branchRequest.BranchData,
				},
			},
		)
		if err != nil {
			log.Printf("Error adding branch to company: %v", err)
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to add branch to company: " + err.Error(),
			})
		}
	}

	// Get updated branch request
	var updatedRequest models.BranchRequest
	err = branchRequestsCollection.FindOne(ctx, bson.M{"_id": requestObjectID}).Decode(&updatedRequest)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve updated request",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Branch request processed successfully",
		Data:    updatedRequest,
	})
}

// GetBranchRequest retrieves a specific branch request
func (abc *AdminBranchController) GetBranchRequest(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get request ID from URL parameter
	requestID := c.Param("id")
	if requestID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Request ID is required",
		})
	}

	// Convert string request ID to ObjectID
	requestObjectID, err := primitive.ObjectIDFromHex(requestID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request ID format",
		})
	}

	// Get branch request
	branchRequestsCollection := config.GetCollection(abc.DB, "branch_requests")
	var branchRequest models.BranchRequest
	err = branchRequestsCollection.FindOne(ctx, bson.M{"_id": requestObjectID}).Decode(&branchRequest)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Branch request not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find branch request",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Branch request retrieved successfully",
		Data:    branchRequest,
	})
}
