// controllers/wholesaler_controller.go
package controllers

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/HSouheill/barrim_backend/config"
	"github.com/HSouheill/barrim_backend/middleware"
	"github.com/HSouheill/barrim_backend/models"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// WholesalerController handles wholesaler-related operations
type WholesalerController struct {
	DB *mongo.Client
}

// WholesalerFullData represents the complete wholesaler data including related entities
type WholesalerFullData struct {
	Wholesaler          models.Wholesaler                      `json:"wholesaler"`
	Branches            []models.WholesalerBranch              `json:"branches,omitempty"`
	Subscriptions       []models.WholesalerSubscription        `json:"subscriptions,omitempty"`
	BranchSubscriptions []models.WholesalerBranchSubscription  `json:"branchSubscriptions,omitempty"`
	SubscriptionReqs    []models.WholesalerSubscriptionRequest `json:"subscriptionRequests,omitempty"`
}

// NewWholesalerController creates a new wholesaler controller
func NewWholesalerController(db *mongo.Client) *WholesalerController {
	return &WholesalerController{DB: db}
}

// GetFullWholesalerData retrieves complete wholesaler data including branches, subscriptions, and requests
func (wc *WholesalerController) GetFullWholesalerData(c echo.Context) error {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
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

	// Initialize the response structure
	var result WholesalerFullData

	// Get wholesaler data
	wholesalerCollection := config.GetCollection(wc.DB, "wholesalers")
	err = wholesalerCollection.FindOne(ctx, bson.M{
		"$or": []bson.M{
			{"userId": userID},
			{"createdBy": userID},
		},
	}).Decode(&result.Wholesaler)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Wholesaler not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve wholesaler data",
		})
	}

	// Get wholesaler subscriptions
	subscriptionCollection := config.GetCollection(wc.DB, "wholesaler_subscriptions")
	cursor, err := subscriptionCollection.Find(ctx, bson.M{"wholesalerId": result.Wholesaler.ID})
	if err != nil && err != mongo.ErrNoDocuments {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve subscriptions",
		})
	}
	if err == nil {
		defer cursor.Close(ctx)
		if err = cursor.All(ctx, &result.Subscriptions); err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to decode subscriptions",
			})
		}
	}

	// Get subscription requests
	reqCursor, err := config.GetCollection(wc.DB, "wholesaler_subscription_requests").Find(ctx, bson.M{"wholesalerId": result.Wholesaler.ID})
	if err != nil && err != mongo.ErrNoDocuments {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve subscription requests",
		})
	}
	if err == nil {
		defer reqCursor.Close(ctx)
		if err = reqCursor.All(ctx, &result.SubscriptionReqs); err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to decode subscription requests",
			})
		}
	}

	// Get branches if they exist
	if len(result.Wholesaler.Branches) > 0 {
		// Convert Branch to WholesalerBranch
		for _, branch := range result.Wholesaler.Branches {
			wholesalerBranch := models.WholesalerBranch{
				ID:           branch.ID,
				WholesalerID: result.Wholesaler.ID,
				Name:         branch.Name,
				Location:     branch.Location,
				Phone:        branch.Phone,
				Category:     branch.Category,
				SubCategory:  branch.SubCategory,
				Description:  branch.Description,
				Images:       branch.Images,
				Videos:       branch.Videos,
				Status:       branch.Status,
				CreatedAt:    branch.CreatedAt,
				UpdatedAt:    branch.UpdatedAt,
			}
			result.Branches = append(result.Branches, wholesalerBranch)
		}

		// Get branch subscriptions if there are branches
		branchIDs := make([]primitive.ObjectID, len(result.Branches))
		for i, branch := range result.Branches {
			branchIDs[i] = branch.ID
		}

		// Get branch subscriptions
		branchSubsCollection := config.GetCollection(wc.DB, "wholesaler_branch_subscriptions")
		branchSubsCursor, err := branchSubsCollection.Find(ctx, bson.M{"branchId": bson.M{"$in": branchIDs}})
		if err != nil && err != mongo.ErrNoDocuments {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to retrieve branch subscriptions",
			})
		}
		if err == nil {
			defer branchSubsCursor.Close(ctx)
			if err = branchSubsCursor.All(ctx, &result.BranchSubscriptions); err != nil {
				return c.JSON(http.StatusInternalServerError, models.Response{
					Status:  http.StatusInternalServerError,
					Message: "Failed to decode branch subscriptions",
				})
			}
		}
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Complete wholesaler data retrieved successfully",
		Data:    result,
	})
}

// GetWholesalerData retrieves wholesaler data including category, subcategory, and email information
func (wc *WholesalerController) GetWholesalerData(c echo.Context) error {
	// Create context with timeout
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

	log.Printf("User ID from token: %s", claims.UserID)

	// Find wholesaler by user ID or by CreatedBy (for salesperson-created wholesalers)
	var wholesaler models.Wholesaler
	wholesalerCollection := config.GetCollection(wc.DB, "wholesalers")

	// First try to find by userId (standard approach)
	err = wholesalerCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&wholesaler)
	if err != nil {
		// If not found by userId, try to find by CreatedBy (salesperson-created wholesalers)
		err = wholesalerCollection.FindOne(ctx, bson.M{"createdBy": userID}).Decode(&wholesaler)
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
	}

	// Get the main user email from the users collection
	var user models.User
	userCollection := config.GetCollection(wc.DB, "users")
	err = userCollection.FindOne(ctx, bson.M{"_id": wholesaler.UserID}).Decode(&user)
	if err != nil {
		log.Printf("Warning: Failed to retrieve user email: %v", err)
		// Continue without user email if there's an error
	}

	// Prepare email data
	emailData := map[string]interface{}{
		"email":            user.Email,
		"additionalEmails": wholesaler.AdditionalEmails,
		"allEmails":        []string{},
	}

	// Combine main email with additional emails
	if user.Email != "" {
		emailData["allEmails"] = append(emailData["allEmails"].([]string), user.Email)
	}
	if wholesaler.AdditionalEmails != nil {
		emailData["allEmails"] = append(emailData["allEmails"].([]string), wholesaler.AdditionalEmails...)
	}

	// Return wholesaler data with enhanced email information
	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Wholesaler data retrieved successfully",
		Data: map[string]interface{}{
			"wholesaler": wholesaler,
			"emails":     emailData,
		},
	})
}

// CreateBranch handles the creation of a new branch with images
func (wc *WholesalerController) CreateBranch(c echo.Context) error {
	// Add request logging at the start
	log.Printf("Starting branch creation request from IP: %s", c.RealIP())

	// Create context with timeout
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

	// Find wholesaler by user ID
	var wholesaler models.Wholesaler
	wholesalerCollection := config.GetCollection(wc.DB, "wholesalers")
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

	// Parse multipart form
	form, err := c.MultipartForm()
	if err != nil {
		log.Printf("Error parsing multipart form: %v", err)
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Failed to parse form data: " + err.Error(),
		})
	}

	// Get branch data from form
	data := form.Value["data"]
	if len(data) == 0 {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Branch data is required",
		})
	}

	// Log the raw data for debugging
	log.Printf("Raw branch data received: %s", data[0])

	// Parse branch data
	var branchData map[string]interface{}
	if err := json.Unmarshal([]byte(data[0]), &branchData); err != nil {
		log.Printf("Error unmarshaling branch data: %v", err)
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid branch data format: " + err.Error(),
		})
	}

	// Handle file uploads
	files := form.File["images"]
	var imagePaths []string

	for _, file := range files {
		// Generate unique filename
		filename := uuid.New().String() + filepath.Ext(file.Filename)
		uploadPath := filepath.Join("uploads", filename)

		// Ensure uploads directory exists
		os.MkdirAll("uploads", 0755)

		// Save file to uploads directory
		src, err := file.Open()
		if err != nil {
			log.Printf("Error opening file %s: %v", file.Filename, err)
			continue
		}
		defer src.Close()

		// Create destination file
		dst, err := os.Create(uploadPath)
		if err != nil {
			log.Printf("Error creating destination file %s: %v", uploadPath, err)
			continue
		}
		defer dst.Close()

		// Copy file
		if _, err = io.Copy(dst, src); err != nil {
			log.Printf("Error copying file data: %v", err)
			continue
		}

		imagePaths = append(imagePaths, uploadPath)
	}

	// Handle video uploads
	videoFiles := form.File["videos"]
	var videoPaths []string

	for _, file := range videoFiles {
		// Generate unique filename
		filename := "video_" + uuid.New().String() + filepath.Ext(file.Filename)
		uploadPath := filepath.Join("uploads", "videos", filename)

		// Ensure videos directory exists
		os.MkdirAll(filepath.Join("uploads", "videos"), 0755)

		// Save file to uploads directory
		src, err := file.Open()
		if err != nil {
			log.Printf("Error opening video file %s: %v", file.Filename, err)
			continue
		}
		defer src.Close()

		dst, err := os.Create(uploadPath)
		if err != nil {
			log.Printf("Error creating destination video file %s: %v", uploadPath, err)
			continue
		}
		defer dst.Close()

		if _, err = io.Copy(dst, src); err != nil {
			log.Printf("Error copying video file data: %v", err)
			continue
		}

		videoPaths = append(videoPaths, uploadPath)
	}

	// Create address for the branch
	address := models.Address{
		Country:     getString(branchData, "country", ""),
		District:    getString(branchData, "district", ""),
		City:        getString(branchData, "city", ""),
		Governorate: getString(branchData, "governorate", ""),
	}

	// Handle latitude and longitude
	if lat, ok := branchData["lat"]; ok && lat != nil {
		switch v := lat.(type) {
		case float64:
			address.Lat = v
		case int:
			address.Lat = float64(v)
		case string:
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				address.Lat = f
			}
		}
	}

	if lng, ok := branchData["lng"]; ok && lng != nil {
		switch v := lng.(type) {
		case float64:
			address.Lng = v
		case int:
			address.Lng = float64(v)
		case string:
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				address.Lng = f
			}
		}
	}

	// Create social media object for the branch
	socialMedia := models.SocialMedia{
		Facebook:  getString(branchData, "facebook", ""),
		Instagram: getString(branchData, "instagram", ""),
	}

	// Create branch object with safer type assertions
	branch := models.Branch{
		ID:          primitive.NewObjectID(),
		Name:        getString(branchData, "name", ""),
		Location:    address,
		Phone:       getString(branchData, "phone", ""),
		Category:    getString(branchData, "category", ""),
		SubCategory: getString(branchData, "subCategory", ""),
		Description: getString(branchData, "description", ""),
		Images:      imagePaths,
		Videos:      videoPaths,
		SocialMedia: socialMedia,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	log.Printf("Prepared branch object: %+v", branch)

	// Update wholesaler document to add the branch
	update := bson.M{
		"$push": bson.M{
			"branches": branch,
		},
	}

	result, err := wholesalerCollection.UpdateByID(ctx, wholesaler.ID, update)
	if err != nil {
		log.Printf("Error updating database: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to save branch: " + err.Error(),
		})
	}

	log.Printf("Database update result: %+v", result)

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Branch created successfully",
		Data: map[string]interface{}{
			"_id":         branch.ID.Hex(),
			"name":        branch.Name,
			"location":    branch.Location,
			"lat":         branch.Location.Lat,
			"lng":         branch.Location.Lng,
			"images":      branch.Images,
			"videos":      branch.Videos,
			"phone":       branch.Phone,
			"category":    branch.Category,
			"subCategory": branch.SubCategory,
			"description": branch.Description,
			"socialMedia": branch.SocialMedia,
		},
	})
}

// AddBranch handles the creation of a new branch with images
func (wc *WholesalerController) AddBranch(c echo.Context) error {
	// Add request logging at the start
	log.Printf("Starting branch creation request from IP: %s", c.RealIP())

	// Create context with timeout
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

	// Find wholesaler by user ID
	var wholesaler models.Wholesaler
	wholesalerCollection := config.GetCollection(wc.DB, "wholesalers")
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

	// Parse multipart form
	form, err := c.MultipartForm()
	if err != nil {
		log.Printf("Error parsing multipart form: %v", err)
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Failed to parse form data: " + err.Error(),
		})
	}

	// Get branch data from form
	data := form.Value["data"]
	if len(data) == 0 {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Branch data is required",
		})
	}

	// Log the raw data for debugging
	log.Printf("Raw branch data received: %s", data[0])

	// Parse branch data
	var branchData map[string]interface{}
	if err := json.Unmarshal([]byte(data[0]), &branchData); err != nil {
		log.Printf("Error unmarshaling branch data: %v", err)
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid branch data format: " + err.Error(),
		})
	}

	// Handle file uploads
	files := form.File["images"]
	var imagePaths []string

	for _, file := range files {
		// Generate unique filename
		filename := uuid.New().String() + filepath.Ext(file.Filename)
		uploadPath := filepath.Join("uploads", filename)

		// Ensure uploads directory exists
		os.MkdirAll("uploads", 0755)

		// Save file to uploads directory
		src, err := file.Open()
		if err != nil {
			log.Printf("Error opening file %s: %v", file.Filename, err)
			continue
		}
		defer src.Close()

		// Create destination file
		dst, err := os.Create(uploadPath)
		if err != nil {
			log.Printf("Error creating destination file %s: %v", uploadPath, err)
			continue
		}
		defer dst.Close()

		// Copy file
		if _, err = io.Copy(dst, src); err != nil {
			log.Printf("Error copying file data: %v", err)
			continue
		}

		imagePaths = append(imagePaths, uploadPath)
	}

	// Handle video uploads
	videoFiles := form.File["videos"]
	var videoPaths []string

	for _, file := range videoFiles {
		// Generate unique filename
		filename := "video_" + uuid.New().String() + filepath.Ext(file.Filename)
		uploadPath := filepath.Join("uploads", "videos", filename)

		// Ensure videos directory exists
		os.MkdirAll(filepath.Join("uploads", "videos"), 0755)

		// Save file to uploads directory
		src, err := file.Open()
		if err != nil {
			log.Printf("Error opening video file %s: %v", file.Filename, err)
			continue
		}
		defer src.Close()

		dst, err := os.Create(uploadPath)
		if err != nil {
			log.Printf("Error creating destination video file %s: %v", uploadPath, err)
			continue
		}
		defer dst.Close()

		if _, err = io.Copy(dst, src); err != nil {
			log.Printf("Error copying video file data: %v", err)
			continue
		}

		videoPaths = append(videoPaths, uploadPath)
	}

	// Create address for the branch
	address := models.Address{
		Country:     getString(branchData, "country", ""),
		District:    getString(branchData, "district", ""),
		City:        getString(branchData, "city", ""),
		Governorate: getString(branchData, "governorate", ""),
	}

	// Handle latitude and longitude
	if lat, ok := branchData["lat"]; ok && lat != nil {
		switch v := lat.(type) {
		case float64:
			address.Lat = v
		case int:
			address.Lat = float64(v)
		case string:
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				address.Lat = f
			}
		}
	}

	if lng, ok := branchData["lng"]; ok && lng != nil {
		switch v := lng.(type) {
		case float64:
			address.Lng = v
		case int:
			address.Lng = float64(v)
		case string:
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				address.Lng = f
			}
		}
	}

	// Create social media object for the branch
	socialMedia := models.SocialMedia{
		Facebook:  getString(branchData, "facebook", ""),
		Instagram: getString(branchData, "instagram", ""),
	}

	// Create branch object with safer type assertions
	branch := models.Branch{
		ID:          primitive.NewObjectID(),
		Name:        getString(branchData, "name", ""),
		Location:    address,
		Phone:       getString(branchData, "phone", ""),
		Category:    getString(branchData, "category", ""),
		SubCategory: getString(branchData, "subCategory", ""),
		Description: getString(branchData, "description", ""),
		Images:      imagePaths,
		Videos:      videoPaths,
		SocialMedia: socialMedia,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	log.Printf("Prepared branch object: %+v", branch)

	// Update wholesaler document to add the branch
	update := bson.M{
		"$push": bson.M{
			"branches": branch,
		},
	}

	result, err := wholesalerCollection.UpdateByID(ctx, wholesaler.ID, update)
	if err != nil {
		log.Printf("Error updating database: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to save branch: " + err.Error(),
		})
	}

	log.Printf("Database update result: %+v", result)

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Branch created successfully",
		Data: map[string]interface{}{
			"_id":         branch.ID.Hex(),
			"name":        branch.Name,
			"location":    branch.Location,
			"lat":         branch.Location.Lat,
			"lng":         branch.Location.Lng,
			"images":      branch.Images,
			"videos":      branch.Videos,
			"phone":       branch.Phone,
			"category":    branch.Category,
			"subCategory": branch.SubCategory,
			"description": branch.Description,
			"socialMedia": branch.SocialMedia,
		},
	})
}

// GetBranches retrieves branches for a wholesaler
func (wc *WholesalerController) GetBranches(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get wholesaler ID from query parameters
	wholesalerIDParam := c.QueryParam("wholesalerId")
	var wholesalerID primitive.ObjectID
	var err error

	wholesalerCollection := config.GetCollection(wc.DB, "wholesalers")

	if wholesalerIDParam != "" && wholesalerIDParam != "null" {
		// If wholesalerId is provided in query and not "null", use that
		log.Printf("Using wholesalerId from query params: %s", wholesalerIDParam)
		wholesalerID, err = primitive.ObjectIDFromHex(wholesalerIDParam)
		if err != nil {
			log.Printf("Invalid wholesaler ID format: %s", wholesalerIDParam)
			return c.JSON(http.StatusBadRequest, models.Response{
				Status:  http.StatusBadRequest,
				Message: "Invalid wholesaler ID format",
			})
		}
	} else {
		// Otherwise use the authenticated user's ID to find their wholesaler
		claims := middleware.GetUserFromToken(c)
		userID, err := primitive.ObjectIDFromHex(claims.UserID)
		if err != nil {
			return c.JSON(http.StatusBadRequest, models.Response{
				Status:  http.StatusBadRequest,
				Message: "Invalid user ID",
			})
		}

		// Find wholesaler ID by user ID
		var wholesaler models.Wholesaler
		err = wholesalerCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&wholesaler)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				return c.JSON(http.StatusNotFound, models.Response{
					Status:  http.StatusNotFound,
					Message: "Wholesaler not found for this user",
				})
			}
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to find wholesaler",
			})
		}
		wholesalerID = wholesaler.ID
	}

	// Find the wholesaler by ID
	var wholesaler models.Wholesaler
	err = wholesalerCollection.FindOne(ctx, bson.M{"_id": wholesalerID}).Decode(&wholesaler)

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

	// Check if wholesaler has branches
	if len(wholesaler.Branches) == 0 {
		return c.JSON(http.StatusOK, models.Response{
			Status:  http.StatusOK,
			Message: "No branches found for this wholesaler",
			Data:    []models.Branch{},
		})
	}

	log.Printf("Returning branches: %+v", wholesaler.Branches)

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Branches retrieved successfully",
		Data:    wholesaler.Branches,
	})
}

// GetBranch retrieves a specific branch by ID
func (wc *WholesalerController) GetBranch(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get branch ID from URL parameter
	branchID := c.Param("id")
	if branchID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Branch ID is required",
		})
	}

	// Convert string branch ID to ObjectID
	branchObjectID, err := primitive.ObjectIDFromHex(branchID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid branch ID format",
		})
	}

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
	wholesalerCollection := config.GetCollection(wc.DB, "wholesalers")
	var wholesaler models.Wholesaler
	err = wholesalerCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&wholesaler)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Wholesaler not found for this user",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find wholesaler",
		})
	}

	// Find the branch with the matching ID
	var branch models.Branch
	found := false
	for _, b := range wholesaler.Branches {
		if b.ID == branchObjectID {
			branch = b
			found = true
			break
		}
	}

	if !found {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Branch not found",
		})
	}

	log.Printf("Retrieved branch social media: %+v", branch.SocialMedia)
	log.Printf("Full retrieved branch: %+v", branch)

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Branch retrieved successfully",
		Data:    branch,
	})
}

// DeleteBranch handles the deletion of a branch by ID
func (wc *WholesalerController) DeleteBranch(c echo.Context) error {
	// Create context with timeout
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
	wholesalerCollection := config.GetCollection(wc.DB, "wholesalers")
	var wholesaler models.Wholesaler
	err = wholesalerCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&wholesaler)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Wholesaler not found for this user",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find wholesaler",
		})
	}

	// Get branch ID from URL parameter
	branchID := c.Param("id")
	if branchID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Branch ID is required",
		})
	}

	// Convert string branch ID to ObjectID
	branchObjectID, err := primitive.ObjectIDFromHex(branchID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid branch ID format",
		})
	}

	// Find the branch with the matching ID to get its images and videos
	var imagesToDelete []string
	var videosToDelete []string
	for _, branch := range wholesaler.Branches {
		if branch.ID == branchObjectID {
			imagesToDelete = branch.Images
			videosToDelete = branch.Videos
			break
		}
	}

	// Update wholesaler document to pull the branch
	update := bson.M{
		"$pull": bson.M{
			"branches": bson.M{"_id": branchObjectID},
		},
	}

	result, err := wholesalerCollection.UpdateByID(ctx, wholesaler.ID, update)
	if err != nil {
		log.Printf("Error deleting branch: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to delete branch: " + err.Error(),
		})
	}

	if result.ModifiedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Branch not found or already deleted",
		})
	}

	// Delete image files from filesystem
	var deletionErrors []string
	for _, imagePath := range imagesToDelete {
		if err := os.Remove(imagePath); err != nil {
			log.Printf("Error deleting image file %s: %v", imagePath, err)
			deletionErrors = append(deletionErrors, imagePath)
		} else {
			log.Printf("Successfully deleted image file: %s", imagePath)
		}
	}

	// Delete video files from filesystem
	for _, videoPath := range videosToDelete {
		if err := os.Remove(videoPath); err != nil {
			log.Printf("Error deleting video file %s: %v", videoPath, err)
			deletionErrors = append(deletionErrors, videoPath)
		} else {
			log.Printf("Successfully deleted video file: %s", videoPath)
		}
	}

	// Log deletion errors but don't fail the request
	if len(deletionErrors) > 0 {
		log.Printf("Some media files could not be deleted: %v", deletionErrors)
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Branch deleted successfully",
	})
}

// UpdateBranch handles updating an existing branch
func (wc *WholesalerController) UpdateBranch(c echo.Context) error {
	// Add request logging at the start
	log.Printf("Starting branch update request from IP: %s", c.RealIP())

	// Create context with timeout
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

	// Find wholesaler by user ID
	wholesalerCollection := config.GetCollection(wc.DB, "wholesalers")
	var wholesaler models.Wholesaler
	err = wholesalerCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&wholesaler)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Wholesaler not found for this user",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find wholesaler",
		})
	}

	// Get branch ID from URL parameter
	branchID := c.Param("id")
	if branchID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Branch ID is required",
		})
	}

	// Convert string branch ID to ObjectID
	branchObjectID, err := primitive.ObjectIDFromHex(branchID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid branch ID format",
		})
	}

	// Parse multipart form
	form, err := c.MultipartForm()
	if err != nil {
		log.Printf("Error parsing multipart form: %v", err)
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Failed to parse form data: " + err.Error(),
		})
	}

	// Get branch data from form
	data := form.Value["data"]
	if len(data) == 0 {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Branch data is required",
		})
	}

	// Log the raw data for debugging
	log.Printf("Raw branch update data received: %s", data[0])

	// Parse branch data
	var branchData map[string]interface{}
	if err := json.Unmarshal([]byte(data[0]), &branchData); err != nil {
		log.Printf("Error unmarshaling branch data: %v", err)
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid branch data format: " + err.Error(),
		})
	}

	log.Printf("Parsed branch data: %+v", branchData)

	// Find the existing branch to get its current data
	var existingBranch models.Branch
	var existingImagePaths []string
	var existingVideoPaths []string
	branchFound := false
	for _, branch := range wholesaler.Branches {
		if branch.ID == branchObjectID {
			existingBranch = branch
			existingImagePaths = branch.Images
			existingVideoPaths = branch.Videos
			branchFound = true
			break
		}
	}

	if !branchFound {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Branch not found",
		})
	}

	// Handle new image uploads
	files := form.File["images"]
	var newImagePaths []string

	for _, file := range files {
		// Generate unique filename
		filename := uuid.New().String() + filepath.Ext(file.Filename)
		uploadPath := filepath.Join("uploads", filename)

		// Ensure uploads directory exists
		os.MkdirAll("uploads", 0755)

		// Save file to uploads directory
		src, err := file.Open()
		if err != nil {
			log.Printf("Error opening file %s: %v", file.Filename, err)
			continue
		}
		defer src.Close()

		// Create destination file
		dst, err := os.Create(uploadPath)
		if err != nil {
			log.Printf("Error creating destination file %s: %v", uploadPath, err)
			continue
		}
		defer dst.Close()

		// Copy file
		if _, err = io.Copy(dst, src); err != nil {
			log.Printf("Error copying file data: %v", err)
			continue
		}

		newImagePaths = append(newImagePaths, uploadPath)
	}

	// Handle new video uploads
	videoFiles := form.File["videos"]
	var newVideoPaths []string

	for _, file := range videoFiles {
		// Generate unique filename
		filename := "video_" + uuid.New().String() + filepath.Ext(file.Filename)
		uploadPath := filepath.Join("uploads", "videos", filename)

		// Ensure videos directory exists
		os.MkdirAll(filepath.Join("uploads", "videos"), 0755)

		// Save file to uploads directory
		src, err := file.Open()
		if err != nil {
			log.Printf("Error opening video file %s: %v", file.Filename, err)
			continue
		}
		defer src.Close()

		dst, err := os.Create(uploadPath)
		if err != nil {
			log.Printf("Error creating destination video file %s: %v", uploadPath, err)
			continue
		}
		defer dst.Close()

		if _, err = io.Copy(dst, src); err != nil {
			log.Printf("Error copying video file data: %v", err)
			continue
		}

		newVideoPaths = append(newVideoPaths, uploadPath)
	}

	// Determine final image paths (keep existing unless new ones are provided)
	var finalImagePaths []string
	if len(newImagePaths) > 0 {
		finalImagePaths = newImagePaths
		// Delete old images if new ones are uploaded
		for _, oldPath := range existingImagePaths {
			if err := os.Remove(oldPath); err != nil {
				log.Printf("Warning: Failed to delete old image %s: %v", oldPath, err)
			}
		}
	} else {
		finalImagePaths = existingImagePaths
	}

	// Determine final video paths (keep existing unless new ones are provided)
	var finalVideoPaths []string
	if len(newVideoPaths) > 0 {
		finalVideoPaths = newVideoPaths
		// Delete old videos if new ones are uploaded
		for _, oldPath := range existingVideoPaths {
			if err := os.Remove(oldPath); err != nil {
				log.Printf("Warning: Failed to delete old video %s: %v", oldPath, err)
			}
		}
	} else {
		finalVideoPaths = existingVideoPaths
	}

	// Update address for the branch
	address := models.Address{
		Country:     getString(branchData, "country", existingBranch.Location.Country),
		District:    getString(branchData, "district", existingBranch.Location.District),
		City:        getString(branchData, "city", existingBranch.Location.City),
		Governorate: getString(branchData, "governorate", existingBranch.Location.Governorate),
		Lat:         existingBranch.Location.Lat,
		Lng:         existingBranch.Location.Lng,
	}

	// Handle latitude and longitude
	if lat, ok := branchData["lat"]; ok && lat != nil {
		switch v := lat.(type) {
		case float64:
			address.Lat = v
		case int:
			address.Lat = float64(v)
		case string:
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				address.Lat = f
			}
		}
	}

	if lng, ok := branchData["lng"]; ok && lng != nil {
		switch v := lng.(type) {
		case float64:
			address.Lng = v
		case int:
			address.Lng = float64(v)
		case string:
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				address.Lng = f
			}
		}
	}

	// Create social media object for the branch - robust extraction
	var facebookValue, instagramValue string

	// Extract Facebook value - try multiple approaches
	if fb, exists := branchData["facebook"]; exists && fb != nil {
		if fbStr, ok := fb.(string); ok && fbStr != "" {
			facebookValue = fbStr
		} else {
			facebookValue = existingBranch.SocialMedia.Facebook
		}
	} else if fbForm := form.Value["facebook"]; len(fbForm) > 0 && fbForm[0] != "" {
		// Check if facebook was sent as a separate form field
		facebookValue = fbForm[0]
	} else {
		facebookValue = existingBranch.SocialMedia.Facebook
	}

	// Extract Instagram value - try multiple approaches
	if ig, exists := branchData["instagram"]; exists && ig != nil {
		if igStr, ok := ig.(string); ok && igStr != "" {
			instagramValue = igStr
		} else {
			instagramValue = existingBranch.SocialMedia.Instagram
		}
	} else if igForm := form.Value["instagram"]; len(igForm) > 0 && igForm[0] != "" {
		// Check if instagram was sent as a separate form field
		instagramValue = igForm[0]
	} else {
		instagramValue = existingBranch.SocialMedia.Instagram
	}

	socialMedia := models.SocialMedia{
		Facebook:  facebookValue,
		Instagram: instagramValue,
	}

	// Create updated branch object
	updatedBranch := models.Branch{
		ID:          branchObjectID,
		Name:        getString(branchData, "name", existingBranch.Name),
		Location:    address,
		Phone:       getString(branchData, "phone", existingBranch.Phone),
		Category:    getString(branchData, "category", existingBranch.Category),
		SubCategory: getString(branchData, "subCategory", existingBranch.SubCategory),
		Description: getString(branchData, "description", existingBranch.Description),
		Images:      finalImagePaths,
		Videos:      finalVideoPaths,
		SocialMedia: socialMedia,
		CreatedAt:   existingBranch.CreatedAt,
		UpdatedAt:   time.Now(),
	}

	log.Printf("Prepared updated branch object with social media: %+v", updatedBranch.SocialMedia)

	// Update the branch in the database
	// First, remove the old branch
	pull := bson.M{
		"$pull": bson.M{
			"branches": bson.M{"_id": branchObjectID},
		},
	}

	_, err = wholesalerCollection.UpdateByID(ctx, wholesaler.ID, pull)
	if err != nil {
		log.Printf("Error removing old branch: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update branch: " + err.Error(),
		})
	}

	// Then, add the updated branch
	push := bson.M{
		"$push": bson.M{
			"branches": updatedBranch,
		},
	}

	result, err := wholesalerCollection.UpdateByID(ctx, wholesaler.ID, push)
	if err != nil {
		log.Printf("Error updating branch: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update branch: " + err.Error(),
		})
	}

	log.Printf("Database update result: %+v", result)
	log.Printf("Updated branch social media: %+v", updatedBranch.SocialMedia)

	responseData := map[string]interface{}{
		"_id":         updatedBranch.ID.Hex(),
		"name":        updatedBranch.Name,
		"location":    updatedBranch.Location,
		"lat":         updatedBranch.Location.Lat,
		"lng":         updatedBranch.Location.Lng,
		"images":      updatedBranch.Images,
		"videos":      updatedBranch.Videos,
		"phone":       updatedBranch.Phone,
		"category":    updatedBranch.Category,
		"subCategory": updatedBranch.SubCategory,
		"description": updatedBranch.Description,
		"socialMedia": updatedBranch.SocialMedia,
	}

	// Debug: Log response data
	log.Printf("Response social media: %+v", responseData["socialMedia"])

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Branch updated successfully",
		Data:    responseData,
	})
}

// GetAllWholesalers retrieves all wholesalers from the database with branch status
func (wc *WholesalerController) GetAllWholesalers(c echo.Context) error {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get wholesaler collection
	wholesalerCollection := config.GetCollection(wc.DB, "wholesalers")

	// Set up filter for wholesalers with location data
	filter := bson.M{
		"contactInfo.address": bson.M{"$exists": true, "$ne": nil},
	}

	// Create options to exclude sensitive data and sort by creation date
	opts := options.Find().
		SetProjection(bson.M{
			"password": 0,
			"userId":   0,
		}).
		SetSort(bson.D{{Key: "createdAt", Value: -1}})

	// Find all wholesalers with location data
	cursor, err := wholesalerCollection.Find(ctx, filter, opts)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve wholesalers",
		})
	}
	defer cursor.Close(ctx)

	// Decode wholesalers
	var wholesalers []models.Wholesaler
	if err := cursor.All(ctx, &wholesalers); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode wholesalers data",
		})
	}

	// Check if any wholesalers were found
	if len(wholesalers) == 0 {
		return c.JSON(http.StatusOK, models.Response{
			Status:  http.StatusOK,
			Message: "No wholesalers found",
			Data:    []models.Wholesaler{},
		})
	}

	// Return success response with wholesalers including branch status
	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Wholesalers retrieved successfully",
		Data:    wholesalers,
	})
}
