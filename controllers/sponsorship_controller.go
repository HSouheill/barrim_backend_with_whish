package controllers

import (
	"context"
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

type SponsorshipController struct {
	DB *mongo.Database
}

func NewSponsorshipController(db *mongo.Database) *SponsorshipController {
	return &SponsorshipController{DB: db}
}

// CreateSponsorship creates a new sponsorship card (admin only) - Generic method
func (sc *SponsorshipController) CreateSponsorship(c echo.Context) error {
	var req models.SponsorshipRequest
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

	collection := sc.DB.Collection("sponsorships")

	// Get admin ID from context (set by middleware)
	adminID := c.Get("userId").(string)
	adminObjectID, err := primitive.ObjectIDFromHex(adminID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Invalid admin ID",
			"error":   err.Error(),
		})
	}

	// Enhanced duration validation
	if err := req.ValidateDuration(); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Duration validation failed",
			"error":   err.Error(),
		})
	}

	// Validate dates
	if req.StartDate.After(req.EndDate) {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Start date cannot be after end date",
		})
	}

	// Calculate duration info for better user experience
	durationInfo := models.CalculateDurationInfo(req.Duration)

	// Create sponsorship with enhanced duration handling
	sponsorship := models.Sponsorship{
		Title:        req.Title,
		Price:        req.Price,
		Duration:     req.Duration,
		DurationInfo: durationInfo,
		Discount:     req.Discount,
		UsedCount:    0,
		StartDate:    req.StartDate,
		EndDate:      req.EndDate,
		CreatedBy:    adminObjectID,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	// Insert into database
	result, err := collection.InsertOne(context.Background(), sponsorship)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to create sponsorship",
			"error":   err.Error(),
		})
	}

	sponsorship.ID = result.InsertedID.(primitive.ObjectID)

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"success":     true,
		"message":     "Sponsorship created successfully",
		"sponsorship": sponsorship,
	})
}

// CreateServiceProviderSponsorship creates a new sponsorship specifically for service providers
func (sc *SponsorshipController) CreateServiceProviderSponsorship(c echo.Context) error {
	var req models.SponsorshipRequest
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

	collection := sc.DB.Collection("sponsorships")

	// Get admin ID from context (set by middleware)
	adminID := c.Get("userId").(string)
	adminObjectID, err := primitive.ObjectIDFromHex(adminID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Invalid admin ID",
			"error":   err.Error(),
		})
	}

	// Enhanced duration validation
	if err := req.ValidateDuration(); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Duration validation failed",
			"error":   err.Error(),
		})
	}

	// Validate dates
	if req.StartDate.After(req.EndDate) {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Start date cannot be after end date",
		})
	}

	// Calculate duration info for better user experience
	durationInfo := models.CalculateDurationInfo(req.Duration)

	// Create sponsorship with enhanced duration handling and service provider specific title
	sponsorship := models.Sponsorship{
		Title:        "Service Provider: " + req.Title,
		Price:        req.Price,
		Duration:     req.Duration,
		DurationInfo: durationInfo,
		Discount:     req.Discount,
		UsedCount:    0,
		StartDate:    req.StartDate,
		EndDate:      req.EndDate,
		CreatedBy:    adminObjectID,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	// Insert into database
	result, err := collection.InsertOne(context.Background(), sponsorship)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to create service provider sponsorship",
			"error":   err.Error(),
		})
	}

	sponsorship.ID = result.InsertedID.(primitive.ObjectID)

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"success":     true,
		"message":     "Service provider sponsorship created successfully",
		"sponsorship": sponsorship,
	})
}

// CreateCompanyWholesalerSponsorship creates a new sponsorship specifically for companies and wholesalers
func (sc *SponsorshipController) CreateCompanyWholesalerSponsorship(c echo.Context) error {
	var req models.SponsorshipRequest
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

	collection := sc.DB.Collection("sponsorships")

	// Get admin ID from context (set by middleware)
	adminID := c.Get("userId").(string)
	adminObjectID, err := primitive.ObjectIDFromHex(adminID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Invalid admin ID",
			"error":   err.Error(),
		})
	}

	// Enhanced duration validation
	if err := req.ValidateDuration(); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Duration validation failed",
			"error":   err.Error(),
		})
	}

	// Validate dates
	if req.StartDate.After(req.EndDate) {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Start date cannot be after end date",
		})
	}

	// Calculate duration info for better user experience
	durationInfo := models.CalculateDurationInfo(req.Duration)

	// Create sponsorship with enhanced duration handling and company/wholesaler specific title
	sponsorship := models.Sponsorship{
		Title:        "Company/Wholesaler: " + req.Title,
		Price:        req.Price,
		Duration:     req.Duration,
		DurationInfo: durationInfo,
		Discount:     req.Discount,
		UsedCount:    0,
		StartDate:    req.StartDate,
		EndDate:      req.EndDate,
		CreatedBy:    adminObjectID,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	// Insert into database
	result, err := collection.InsertOne(context.Background(), sponsorship)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to create company/wholesaler sponsorship",
			"error":   err.Error(),
		})
	}

	sponsorship.ID = result.InsertedID.(primitive.ObjectID)

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"success":     true,
		"message":     "Company/Wholesaler sponsorship created successfully",
		"sponsorship": sponsorship,
	})
}

// GetSponsorships retrieves all sponsorships with optional filtering
func (sc *SponsorshipController) GetSponsorships(c echo.Context) error {
	collection := sc.DB.Collection("sponsorships")

	// Parse query parameters
	page, _ := strconv.Atoi(c.QueryParam("page"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))

	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 10
	}

	// Build filter
	filter := bson.M{}

	// Count total documents
	total, err := collection.CountDocuments(context.Background(), filter)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to count sponsorships",
			"error":   err.Error(),
		})
	}

	// Set up pagination
	skip := (page - 1) * limit
	opts := options.Find().
		SetSkip(int64(skip)).
		SetLimit(int64(limit)).
		SetSort(bson.D{{Key: "createdAt", Value: -1}})

	// Find sponsorships
	cursor, err := collection.Find(context.Background(), filter, opts)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to retrieve sponsorships",
			"error":   err.Error(),
		})
	}
	defer cursor.Close(context.Background())

	var sponsorships []models.Sponsorship
	if err = cursor.All(context.Background(), &sponsorships); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to decode sponsorships",
			"error":   err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"sponsorships": sponsorships,
			"pagination": map[string]interface{}{
				"page":  page,
				"limit": limit,
				"total": total,
				"pages": (total + int64(limit) - 1) / int64(limit),
			},
		},
	})
}

// GetSponsorship retrieves a specific sponsorship by ID
func (sc *SponsorshipController) GetSponsorship(c echo.Context) error {
	sponsorshipID := c.Param("id")
	if sponsorshipID == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Sponsorship ID is required",
		})
	}

	objectID, err := primitive.ObjectIDFromHex(sponsorshipID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Invalid sponsorship ID format",
		})
	}

	collection := sc.DB.Collection("sponsorships")
	var sponsorship models.Sponsorship
	err = collection.FindOne(context.Background(), bson.M{"_id": objectID}).Decode(&sponsorship)
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

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success":     true,
		"sponsorship": sponsorship,
	})
}

// UpdateSponsorship updates an existing sponsorship (admin only)
func (sc *SponsorshipController) UpdateSponsorship(c echo.Context) error {
	sponsorshipID := c.Param("id")
	if sponsorshipID == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Sponsorship ID is required",
		})
	}

	objectID, err := primitive.ObjectIDFromHex(sponsorshipID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Invalid sponsorship ID format",
		})
	}

	var req models.SponsorshipUpdateRequest
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

	// Check if sponsorship exists
	collection := sc.DB.Collection("sponsorships")
	var existingSponsorship models.Sponsorship
	err = collection.FindOne(context.Background(), bson.M{"_id": objectID}).Decode(&existingSponsorship)
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

	// Enhanced duration validation for updates
	if err := req.ValidateDurationUpdate(); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Duration validation failed",
			"error":   err.Error(),
		})
	}

	// Build update document
	update := bson.M{}
	if req.Title != nil {
		update["title"] = *req.Title
	}
	if req.Price != nil {
		update["price"] = *req.Price
	}
	if req.Duration != nil {
		update["duration"] = *req.Duration
		// Recalculate duration info when duration changes
		durationInfo := models.CalculateDurationInfo(*req.Duration)
		update["durationInfo"] = durationInfo
	}
	if req.Discount != nil {
		update["discount"] = *req.Discount
	}
	if req.StartDate != nil {
		update["startDate"] = *req.StartDate
	}
	if req.EndDate != nil {
		update["endDate"] = *req.EndDate
	}

	// Validate dates if both are provided
	if req.StartDate != nil && req.EndDate != nil {
		if req.StartDate.After(*req.EndDate) {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{
				"success": false,
				"message": "Start date cannot be after end date",
			})
		}
	}

	update["updatedAt"] = time.Now()

	// Update sponsorship
	_, err = collection.UpdateOne(
		context.Background(),
		bson.M{"_id": objectID},
		bson.M{"$set": update},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to update sponsorship",
			"error":   err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Sponsorship updated successfully",
	})
}

// DeleteSponsorship deletes a sponsorship (admin only)
func (sc *SponsorshipController) DeleteSponsorship(c echo.Context) error {
	sponsorshipID := c.Param("id")
	if sponsorshipID == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Sponsorship ID is required",
		})
	}

	objectID, err := primitive.ObjectIDFromHex(sponsorshipID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Invalid sponsorship ID format",
		})
	}

	collection := sc.DB.Collection("sponsorships")
	result, err := collection.DeleteOne(context.Background(), bson.M{"_id": objectID})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to delete sponsorship",
			"error":   err.Error(),
		})
	}

	if result.DeletedCount == 0 {
		return c.JSON(http.StatusNotFound, map[string]interface{}{
			"success": false,
			"message": "Sponsorship not found",
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Sponsorship deleted successfully",
	})
}

// GetDurationInfo retrieves duration information and recommendations
func (sc *SponsorshipController) GetDurationInfo(c echo.Context) error {
	// Get recommended durations
	recommendedDurations := models.GetRecommendedDurations()
	
	// Create duration info for each recommended duration
	var durationInfos []map[string]interface{}
	for _, days := range recommendedDurations {
		info := models.CalculateDurationInfo(days)
		durationInfos = append(durationInfos, map[string]interface{}{
			"days":        info.Days,
			"weeks":       info.Weeks,
			"months":      info.Months,
			"years":       info.Years,
			"unit":        info.Unit,
			"description": info.GetDurationDescription(),
			"isValid":    info.IsValid,
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"recommendedDurations": durationInfos,
			"limits": map[string]interface{}{
				"minDays": models.MinDurationDays,
				"maxDays": models.MaxDurationDays,
				"default": models.DefaultDuration,
			},
			"presets": map[string]interface{}{
				"week":      models.DurationWeek,
				"month":     models.DurationMonth,
				"quarter":   models.DurationQuarter,
				"halfYear":  models.DurationHalfYear,
				"year":      models.DurationYear,
			},
		},
	})
}

// ValidateDuration validates a specific duration value
func (sc *SponsorshipController) ValidateDuration(c echo.Context) error {
	durationStr := c.QueryParam("duration")
	if durationStr == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Duration parameter is required",
		})
	}

	duration, err := strconv.Atoi(durationStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Invalid duration format",
			"error":   "Duration must be a valid integer",
		})
	}

	// Validate duration
	if !models.IsDurationValid(duration) {
		var errorMsg string
		if duration < models.MinDurationDays {
			errorMsg = models.ErrDurationTooShort.Error()
		} else {
			errorMsg = models.ErrDurationTooLong.Error()
		}

		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Duration validation failed",
			"error":   errorMsg,
			"limits": map[string]interface{}{
				"minDays": models.MinDurationDays,
				"maxDays": models.MaxDurationDays,
			},
		})
	}

	// Calculate and return duration info
	durationInfo := models.CalculateDurationInfo(duration)
	
	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"duration": duration,
			"isValid": true,
			"info": map[string]interface{}{
				"days":        durationInfo.Days,
				"weeks":       durationInfo.Weeks,
				"months":      durationInfo.Months,
				"years":       durationInfo.Years,
				"unit":        durationInfo.Unit,
				"description": durationInfo.GetDurationDescription(),
			},
		},
	})
}

// GetServiceProviderSponsorships retrieves all service provider sponsorships
func (sc *SponsorshipController) GetServiceProviderSponsorships(c echo.Context) error {
	collection := sc.DB.Collection("sponsorships")

	// Parse query parameters
	page, _ := strconv.Atoi(c.QueryParam("page"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))

	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 10
	}

	// Build filter for service provider sponsorships
	filter := bson.M{
		"title": bson.M{
			"$regex":   "^Service Provider:",
			"$options": "i", // Case insensitive
		},
	}

	// Count total documents
	total, err := collection.CountDocuments(context.Background(), filter)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to count service provider sponsorships",
			"error":   err.Error(),
		})
	}

	// Set up pagination
	skip := (page - 1) * limit
	opts := options.Find().
		SetSkip(int64(skip)).
		SetLimit(int64(limit)).
		SetSort(bson.D{{Key: "createdAt", Value: -1}})

	// Find service provider sponsorships
	cursor, err := collection.Find(context.Background(), filter, opts)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to retrieve service provider sponsorships",
			"error":   err.Error(),
		})
	}
	defer cursor.Close(context.Background())

	var sponsorships []models.Sponsorship
	if err = cursor.All(context.Background(), &sponsorships); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to decode service provider sponsorships",
			"error":   err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"sponsorships": sponsorships,
			"pagination": map[string]interface{}{
				"page":  page,
				"limit": limit,
				"total": total,
				"pages": (total + int64(limit) - 1) / int64(limit),
			},
			"entityType": "service_provider",
		},
	})
}

// GetCompanyWholesalerSponsorships retrieves all company and wholesaler sponsorships
func (sc *SponsorshipController) GetCompanyWholesalerSponsorships(c echo.Context) error {
	collection := sc.DB.Collection("sponsorships")

	// Parse query parameters
	page, _ := strconv.Atoi(c.QueryParam("page"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))

	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 10
	}

	// Build filter for company/wholesaler sponsorships
	filter := bson.M{
		"title": bson.M{
			"$regex":   "^Company/Wholesaler:",
			"$options": "i", // Case insensitive
		},
	}

	// Count total documents
	total, err := collection.CountDocuments(context.Background(), filter)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to count company/wholesaler sponsorships",
			"error":   err.Error(),
		})
	}

	// Set up pagination
	skip := (page - 1) * limit
	opts := options.Find().
		SetSkip(int64(skip)).
		SetLimit(int64(limit)).
		SetSort(bson.D{{Key: "createdAt", Value: -1}})

	// Find company/wholesaler sponsorships
	cursor, err := collection.Find(context.Background(), filter, opts)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to retrieve company/wholesaler sponsorships",
			"error":   err.Error(),
		})
	}
	defer cursor.Close(context.Background())

	var sponsorships []models.Sponsorship
	if err = cursor.All(context.Background(), &sponsorships); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to decode company/wholesaler sponsorships",
			"error":   err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"sponsorships": sponsorships,
			"pagination": map[string]interface{}{
				"page":  page,
				"limit": limit,
				"total": total,
				"pages": (total + int64(limit) - 1) / int64(limit),
			},
			"entityType": "company_wholesaler",
		},
	})
}


