// controllers/company_controller.go
package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/crypto/bcrypt"

	"github.com/HSouheill/barrim_backend/config"
	"github.com/HSouheill/barrim_backend/middleware"
	"github.com/HSouheill/barrim_backend/models"
	"github.com/HSouheill/barrim_backend/services"
	"github.com/HSouheill/barrim_backend/utils"
	"github.com/google/uuid"
)

// CompanyController handles company-related operations
type CompanyController struct {
	DB *mongo.Client
}

// CompanyFullData represents the complete company data including related entities
type CompanyFullData struct {
	Company             models.Company               `json:"company"`
	Branches            []models.Branch              `json:"branches,omitempty"`
	Subscriptions       []models.CompanySubscription `json:"subscriptions,omitempty"`
	BranchSubscriptions []models.BranchSubscription  `json:"branchSubscriptions,omitempty"`
	Comments            []models.BranchComment       `json:"comments,omitempty"`
}

// NewCompanyController creates a new company controller
func NewCompanyController(db *mongo.Client) *CompanyController {
	return &CompanyController{DB: db}
}

// GetAllCompanyData retrieves complete company data including branches, subscriptions, and comments
func (cc *CompanyController) GetAllCompanyData(c echo.Context) error {
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
	var result CompanyFullData

	// Get company data
	companyCollection := config.GetCollection(cc.DB, "companies")
	err = companyCollection.FindOne(ctx, bson.M{
		"$or": []bson.M{
			{"userId": userID},
			{"createdBy": userID},
		},
	}).Decode(&result.Company)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Company not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve company data",
		})
	}

	// Get company subscriptions
	subscriptionCollection := config.GetCollection(cc.DB, "company_subscriptions")
	cursor, err := subscriptionCollection.Find(ctx, bson.M{"companyId": result.Company.ID})
	if err != nil && err != mongo.ErrNoDocuments {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve company subscriptions",
		})
	}
	if err == nil {
		defer cursor.Close(ctx)
		if err = cursor.All(ctx, &result.Subscriptions); err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to decode company subscriptions",
			})
		}
	}

	// Get branch subscriptions if there are branches
	if len(result.Company.Branches) > 0 {
		branchIDs := make([]primitive.ObjectID, len(result.Company.Branches))
		for i, branch := range result.Company.Branches {
			branchIDs[i] = branch.ID
		}

		branchSubsCollection := config.GetCollection(cc.DB, "branch_subscriptions")
		cursor, err = branchSubsCollection.Find(ctx, bson.M{"branchId": bson.M{"$in": branchIDs}})
		if err != nil && err != mongo.ErrNoDocuments {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to retrieve branch subscriptions",
			})
		}
		if err == nil {
			defer cursor.Close(ctx)
			if err = cursor.All(ctx, &result.BranchSubscriptions); err != nil {
				return c.JSON(http.StatusInternalServerError, models.Response{
					Status:  http.StatusInternalServerError,
					Message: "Failed to decode branch subscriptions",
				})
			}
		}

		// Get branch comments
		commentsCollection := config.GetCollection(cc.DB, "branch_comments")
		cursor, err = commentsCollection.Find(ctx, bson.M{"branchId": bson.M{"$in": branchIDs}})
		if err != nil && err != mongo.ErrNoDocuments {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to retrieve branch comments",
			})
		}
		if err == nil {
			defer cursor.Close(ctx)
			if err = cursor.All(ctx, &result.Comments); err != nil {
				return c.JSON(http.StatusInternalServerError, models.Response{
					Status:  http.StatusInternalServerError,
					Message: "Failed to decode branch comments",
				})
			}
		}

		// Copy branches to the result
		result.Branches = result.Company.Branches
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Complete company data retrieved successfully",
		Data:    result,
	})
}

// GetCompanyData retrieves company data including category and subcategory
func (cc *CompanyController) GetCompanyData(c echo.Context) error {
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

	// Find company by user ID or by CreatedBy (for salesperson-created companies)
	var company models.Company
	companyCollection := config.GetCollection(cc.DB, "companies")

	// First try to find by userId (standard approach)
	err = companyCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&company)
	if err != nil {
		// If not found by userId, try to find by CreatedBy (salesperson-created companies)
		err = companyCollection.FindOne(ctx, bson.M{"createdBy": userID}).Decode(&company)
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
	}

	// Return company data with category and subcategory
	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Company data retrieved successfully",
		Data: map[string]interface{}{
			"companyInfo": map[string]interface{}{
				"name":        company.BusinessName,
				"category":    company.Category,
				"subCategory": company.SubCategory,
				"logo":        company.LogoURL,
				"contactInfo": map[string]interface{}{
					"phone":    company.ContactInfo.Phone,
					"whatsapp": company.ContactInfo.WhatsApp,
					"website":  company.ContactInfo.Website,
				},
				"socialMedia": map[string]interface{}{
					"facebook":  company.SocialMedia.Facebook,
					"instagram": company.SocialMedia.Instagram,
				},
			},
		},
	})
}

// CreateBranch handles the creation of a new branch with images
func (cc *CompanyController) CreateBranch(c echo.Context) error {
	// Add request logging at the start
	log.Printf("Starting branch creation request from IP: %s", c.RealIP())

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second) // Increased timeout
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
	var company models.Company
	companyCollection := config.GetCollection(cc.DB, "companies")
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
			Message: "Invalid branch data format",
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
			continue // Skip this file if there's an error
		}
		defer src.Close()

		// Create destination file
		dst, err := os.Create(uploadPath)
		if err != nil {
			log.Printf("Error creating destination file %s: %v", uploadPath, err)
			continue // Skip this file if there's an error
		}
		defer dst.Close()

		// Copy file
		if _, err = io.Copy(dst, src); err != nil {
			log.Printf("Error copying file data: %v", err)
			continue // Skip this file if there's an error
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
		Governorate: getString(branchData, "governorate", ""),
		District:    getString(branchData, "district", ""),
		City:        getString(branchData, "city", ""),
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

	// Get cost per customer from branch data
	costPerCustomer := 0.0
	if cost, ok := branchData["costPerCustomer"].(float64); ok {
		costPerCustomer = cost
	}

	// Create social media object for the branch
	socialMedia := models.SocialMedia{
		Facebook:  getString(branchData, "facebook", ""),
		Instagram: getString(branchData, "instagram", ""),
	}

	// Create branch object with safer type assertions
	branch := models.Branch{
		ID:              primitive.NewObjectID(),
		Name:            getString(branchData, "name", ""),
		Location:        address,
		Phone:           getString(branchData, "phone", ""),
		Category:        getString(branchData, "category", ""),
		SubCategory:     getString(branchData, "subCategory", ""),
		Description:     getString(branchData, "description", ""),
		Images:          imagePaths,
		Videos:          videoPaths,
		CostPerCustomer: costPerCustomer,
		Status:          "inactive", // Set status as inactive until branch subscribes to a plan
		SocialMedia:     socialMedia,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	// Add branch directly to company
	_, err = companyCollection.UpdateOne(
		ctx,
		bson.M{"_id": company.ID},
		bson.M{
			"$push": bson.M{
				"branches": branch,
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

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Branch created successfully.",
		Data: map[string]interface{}{
			"branchId": branch.ID.Hex(),
			"status":   "inactive",
			"branch":   branch,
		},
	})
}

// Helper function to safely get string values from a map
func getString(data map[string]interface{}, key, defaultValue string) string {
	if val, ok := data[key]; ok && val != nil {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return defaultValue
}

// GetBranches retrieves branches for a company
func (cc *CompanyController) GetBranches(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get company ID from query parameters
	companyIDParam := c.QueryParam("companyId")
	var companyID primitive.ObjectID
	var err error

	companyCollection := config.GetCollection(cc.DB, "companies")

	if companyIDParam != "" && companyIDParam != "null" {
		// If companyId is provided in query and not "null", use that
		log.Printf("Using companyId from query params: %s", companyIDParam)
		companyID, err = primitive.ObjectIDFromHex(companyIDParam)
		if err != nil {
			log.Printf("Invalid company ID format: %s", companyIDParam)
			return c.JSON(http.StatusBadRequest, models.Response{
				Status:  http.StatusBadRequest,
				Message: "Invalid company ID format",
			})
		}
	} else {
		// Otherwise use the authenticated user's ID to find their company
		claims := middleware.GetUserFromToken(c)
		userID, err := primitive.ObjectIDFromHex(claims.UserID)
		if err != nil {
			return c.JSON(http.StatusBadRequest, models.Response{
				Status:  http.StatusBadRequest,
				Message: "Invalid user ID",
			})
		}

		// Find company ID by user ID
		var company models.Company
		err = companyCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&company)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				return c.JSON(http.StatusNotFound, models.Response{
					Status:  http.StatusNotFound,
					Message: "Company not found for this user",
				})
			}
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to find company",
			})
		}
		companyID = company.ID
	}

	// Find the company by ID
	var company models.Company
	err = companyCollection.FindOne(ctx, bson.M{"_id": companyID}).Decode(&company)

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

	// Check if company has branches
	if len(company.Branches) == 0 {
		return c.JSON(http.StatusOK, models.Response{
			Status:  http.StatusOK,
			Message: "No branches found for this company",
			Data:    []models.Branch{}, // Return empty array
		})
	}

	log.Printf("Returning branches: %+v", company.Branches) // In GetBranches function

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Branches retrieved successfully",
		Data:    company.Branches, // Will include videos in each branch object
	})
}

// GetBranch retrieves a specific branch by ID
// GetBranch retrieves a specific branch by ID
func (cc *CompanyController) GetBranch(c echo.Context) error {
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

	// Find company by user ID
	companyCollection := config.GetCollection(cc.DB, "companies")
	var company models.Company
	err = companyCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&company)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Company not found for this user",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find company",
		})
	}

	// Find the branch with the matching ID
	var branch models.Branch
	found := false
	for _, b := range company.Branches {
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

	// Create a response with branch data and company social links
	responseData := map[string]interface{}{
		"branch": branch,
		"companySocialLinks": map[string]interface{}{
			"whatsapp":  company.ContactInfo.WhatsApp,
			"facebook":  company.SocialMedia.Facebook,
			"instagram": company.SocialMedia.Instagram,
			"website":   company.ContactInfo.Website,
		},
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Branch retrieved successfully",
		Data:    responseData,
	})
}

// DeleteBranch handles the deletion of a branch by ID
func (cc *CompanyController) DeleteBranch(c echo.Context) error {
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

	// Find company by user ID
	companyCollection := config.GetCollection(cc.DB, "companies")
	var company models.Company
	err = companyCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&company)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Company not found for this user",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find company",
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
	for _, branch := range company.Branches {
		if branch.ID == branchObjectID {
			imagesToDelete = branch.Images
			videosToDelete = branch.Videos
			break
		}
	}

	// Update company document to pull the branch
	update := bson.M{
		"$pull": bson.M{
			"branches": bson.M{"_id": branchObjectID},
		},
	}

	result, err := companyCollection.UpdateByID(ctx, company.ID, update)
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

func (cc *CompanyController) UpdateBranch(c echo.Context) error {
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

	// Find company by user ID
	companyCollection := config.GetCollection(cc.DB, "companies")
	var company models.Company
	err = companyCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&company)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Company not found for this user",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find company",
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

	// Find the existing branch to get its current data
	var existingBranch models.Branch
	var existingImagePaths []string
	var existingVideoPaths []string
	branchFound := false
	for _, branch := range company.Branches {
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
			continue // Skip this file if there's an error
		}
		defer src.Close()

		// Create destination file
		dst, err := os.Create(uploadPath)
		if err != nil {
			log.Printf("Error creating destination file %s: %v", uploadPath, err)
			continue // Skip this file if there's an error
		}
		defer dst.Close()

		// Copy file
		if _, err = io.Copy(dst, src); err != nil {
			log.Printf("Error copying file data: %v", err)
			continue // Skip this file if there's an error
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
		Governorate: getString(branchData, "governorate", existingBranch.Location.Governorate),
		District:    getString(branchData, "district", existingBranch.Location.District),
		City:        getString(branchData, "city", existingBranch.Location.City),
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

	// Create social media object for the branch
	socialMedia := models.SocialMedia{
		Facebook:  getString(branchData, "facebook", existingBranch.SocialMedia.Facebook),
		Instagram: getString(branchData, "instagram", existingBranch.SocialMedia.Instagram),
	}

	// Create updated branch object
	updatedBranch := models.Branch{
		ID:          branchObjectID, // Keep the original ID
		Name:        getString(branchData, "name", existingBranch.Name),
		Location:    address,
		Phone:       getString(branchData, "phone", existingBranch.Phone),
		Category:    getString(branchData, "category", existingBranch.Category),
		SubCategory: getString(branchData, "subCategory", existingBranch.SubCategory),
		Description: getString(branchData, "description", existingBranch.Description),
		Images:      finalImagePaths,
		Videos:      finalVideoPaths, // Include videos
		CostPerCustomer: func() float64 {
			if val, ok := branchData["costPerCustomer"]; ok && val != nil {
				switch v := val.(type) {
				case float64:
					return v
				case int:
					return float64(v)
				case string:
					if f, err := strconv.ParseFloat(v, 64); err == nil {
						return f
					}
				}
			}
			return existingBranch.CostPerCustomer
		}(),
		Status:      getString(branchData, "status", existingBranch.Status), // Preserve or update status
		Sponsorship: existingBranch.Sponsorship,                             // Preserve sponsorship status
		SocialMedia: socialMedia,
		CreatedAt:   existingBranch.CreatedAt, // Keep original creation time
		UpdatedAt:   time.Now(),               // Update the update timestamp
	}

	log.Printf("Prepared updated branch object: %+v", updatedBranch)

	// Update the branch in the database
	// First, remove the old branch
	pull := bson.M{
		"$pull": bson.M{
			"branches": bson.M{"_id": branchObjectID},
		},
	}

	_, err = companyCollection.UpdateByID(ctx, company.ID, pull)
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

	result, err := companyCollection.UpdateByID(ctx, company.ID, push)
	if err != nil {
		log.Printf("Error updating branch: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update branch: " + err.Error(),
		})
	}

	log.Printf("Database update result: %+v", result)

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Branch updated successfully",
		Data: map[string]interface{}{
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
		},
	})
}

// UploadLogo handles uploading a company logo
func (cc *CompanyController) UploadLogo(c echo.Context) error {
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

	// Find company by user ID
	companyCollection := config.GetCollection(cc.DB, "companies")
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

	// Get file from request
	file, err := c.FormFile("logo")
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Failed to get logo file: " + err.Error(),
		})
	}

	// Check file size
	if file.Size > 5*1024*1024 { // 5MB limit
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Logo file is too large. Maximum size is 5MB.",
		})
	}

	// Check file type
	fileExt := filepath.Ext(file.Filename)
	if fileExt != ".jpg" && fileExt != ".jpeg" && fileExt != ".png" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid file type. Only JPG, JPEG, and PNG files are allowed.",
		})
	}

	// Generate unique filename
	filename := "logo_" + company.ID.Hex() + "_" + uuid.New().String() + fileExt
	uploadPath := filepath.Join("uploads", "logos", filename)

	// Ensure uploads directory exists
	os.MkdirAll(filepath.Join("uploads", "logos"), 0755)

	// Save file to uploads directory
	src, err := file.Open()
	if err != nil {
		log.Printf("Error opening file %s: %v", file.Filename, err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to process logo file: " + err.Error(),
		})
	}
	defer src.Close()

	// Create destination file
	dst, err := os.Create(uploadPath)
	if err != nil {
		log.Printf("Error creating destination file %s: %v", uploadPath, err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to save logo file: " + err.Error(),
		})
	}
	defer dst.Close()

	// Copy file
	if _, err = io.Copy(dst, src); err != nil {
		log.Printf("Error copying file data: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to save logo file: " + err.Error(),
		})
	}

	// Delete old logo if it exists
	if company.LogoURL != "" {
		if err := os.Remove(company.LogoURL); err != nil {
			log.Printf("Warning: Failed to delete old logo %s: %v", company.LogoURL, err)
			// Continue even if we can't delete the old file
		}
	}

	// Update company with new logo URL
	update := bson.M{
		"$set": bson.M{
			"logoUrl":   uploadPath,
			"updatedAt": time.Now(),
		},
	}

	result, err := companyCollection.UpdateByID(ctx, company.ID, update)
	if err != nil {
		log.Printf("Error updating company logo URL: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update company logo: " + err.Error(),
		})
	}

	if result.ModifiedCount == 0 {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update company logo",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Logo uploaded successfully",
		Data: map[string]interface{}{
			"logoUrl": uploadPath,
		},
	})
}

// company_controller.go
// UpdateCompanyData handles updating company profile information
func (cc *CompanyController) UpdateCompanyData(c echo.Context) error {
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

	// Parse request body
	var updateData map[string]interface{}
	if err := c.Bind(&updateData); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request data: " + err.Error(),
		})
	}

	// Find company by user ID
	companyCollection := config.GetCollection(cc.DB, "companies")
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

	// Prepare update fields
	updateFields := bson.M{
		"updatedAt": time.Now(),
	}

	// Update allowed fields
	if businessName, ok := updateData["businessName"].(string); ok && businessName != "" {
		updateFields["businessName"] = businessName
	}

	if category, ok := updateData["category"].(string); ok && category != "" {
		updateFields["category"] = category
	}

	if subCategory, ok := updateData["subCategory"].(string); ok {
		updateFields["subCategory"] = subCategory
	}

	// Handle contact info updates
	if contactInfo, ok := updateData["contactInfo"].(map[string]interface{}); ok {
		// Handle phone update
		if phone, ok := contactInfo["phone"].(string); ok && phone != "" {
			updateFields["contactInfo.phone"] = phone
		}

		// Handle whatsapp update
		if whatsapp, ok := contactInfo["whatsapp"].(string); ok {
			updateFields["contactInfo.whatsapp"] = whatsapp
		}

		// Handle website update
		if website, ok := contactInfo["website"].(string); ok {
			updateFields["contactInfo.website"] = website
		}

		// Handle address updates
		if address, ok := contactInfo["address"].(map[string]interface{}); ok {
			if country, ok := address["country"].(string); ok && country != "" {
				updateFields["contactInfo.address.country"] = country
			}

			if district, ok := address["district"].(string); ok && district != "" {
				updateFields["contactInfo.address.district"] = district
			}

			if city, ok := address["city"].(string); ok && city != "" {
				updateFields["contactInfo.address.city"] = city
			}

			if street, ok := address["street"].(string); ok && street != "" {
				updateFields["contactInfo.address.street"] = street
			}

			if postalCode, ok := address["postalCode"].(string); ok && postalCode != "" {
				updateFields["contactInfo.address.postalCode"] = postalCode
			}

			// Handle latitude
			if lat, ok := address["lat"]; ok && lat != nil {
				switch v := lat.(type) {
				case float64:
					updateFields["contactInfo.address.lat"] = v
				case int:
					updateFields["contactInfo.address.lat"] = float64(v)
				case string:
					if f, err := strconv.ParseFloat(v, 64); err == nil {
						updateFields["contactInfo.address.lat"] = f
					}
				}
			}

			// Handle longitude
			if lng, ok := address["lng"]; ok && lng != nil {
				switch v := lng.(type) {
				case float64:
					updateFields["contactInfo.address.lng"] = v
				case int:
					updateFields["contactInfo.address.lng"] = float64(v)
				case string:
					if f, err := strconv.ParseFloat(v, 64); err == nil {
						updateFields["contactInfo.address.lng"] = f
					}
				}
			}
		}
	}

	// Handle social media updates
	if socialMedia, ok := updateData["socialMedia"].(map[string]interface{}); ok {
		if facebook, ok := socialMedia["facebook"].(string); ok {
			updateFields["socialMedia.facebook"] = facebook
		}

		if instagram, ok := socialMedia["instagram"].(string); ok {
			updateFields["socialMedia.instagram"] = instagram
		}
	}

	// If there are no fields to update, return an error
	if len(updateFields) <= 1 { // Only contains updatedAt
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "No valid fields to update",
		})
	}

	// Build update operation
	update := bson.M{
		"$set": updateFields,
	}

	// Update company document
	result, err := companyCollection.UpdateByID(ctx, company.ID, update)
	if err != nil {
		log.Printf("Error updating company data: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update company data: " + err.Error(),
		})
	}

	if result.ModifiedCount == 0 {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "No changes applied to company data",
		})
	}

	// Get updated company data to return
	var updatedCompany models.Company
	err = companyCollection.FindOne(ctx, bson.M{"_id": company.ID}).Decode(&updatedCompany)
	if err != nil {
		log.Printf("Error retrieving updated company data: %v", err)
		return c.JSON(http.StatusOK, models.Response{
			Status:  http.StatusOK,
			Message: "Company updated successfully",
		})
	}

	// Return updated company data
	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Company updated successfully",
		Data: map[string]interface{}{
			"companyInfo": map[string]interface{}{
				"name":        updatedCompany.BusinessName,
				"category":    updatedCompany.Category,
				"subCategory": updatedCompany.SubCategory,
				"logo":        updatedCompany.LogoURL,
			},
			"contactInfo": updatedCompany.ContactInfo,
			"socialMedia": updatedCompany.SocialMedia,
		},
	})
}

// GetAllBranches retrieves all branches from all companies with branch status
func (cc *CompanyController) GetAllBranches(c echo.Context) error {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Get company collection
	companyCollection := config.GetCollection(cc.DB, "companies")

	// Find all companies with branches
	cursor, err := companyCollection.Find(ctx, bson.M{"branches": bson.M{"$exists": true, "$not": bson.M{"$size": 0}}})
	if err != nil {
		log.Printf("Error finding companies with branches: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve companies with branches",
		})
	}
	defer cursor.Close(ctx)

	var companies []models.Company
	if err = cursor.All(ctx, &companies); err != nil {
		log.Printf("Error decoding companies: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode companies",
		})
	}

	// Get user information for contact details (including email)
	db := cc.DB.Database("barrim")
	userIDs := make([]primitive.ObjectID, 0, len(companies))
	for _, company := range companies {
		userIDs = append(userIDs, company.UserID)
	}

	userMap := make(map[string]models.User)
	if len(userIDs) > 0 {
		userCursor, err := db.Collection("users").Find(
			ctx,
			bson.M{"_id": bson.M{"$in": userIDs}},
		)
		if err == nil {
			var users []models.User
			_ = userCursor.All(ctx, &users)
			userCursor.Close(ctx)
			for _, user := range users {
				userMap[user.ID.Hex()] = user
			}
		}
	}

	// Prepare response data - flatten all branches with company reference, status, and email
	var allBranches []map[string]interface{}
	for _, company := range companies {
		user := userMap[company.UserID.Hex()]
		for _, branch := range company.Branches {
			branchData := map[string]interface{}{
				"id":          branch.ID.Hex(),
				"name":        branch.Name,
				"location":    branch.Location,
				"phone":       branch.Phone,
				"category":    branch.Category,
				"subCategory": branch.SubCategory,
				"description": branch.Description,
				"images":      branch.Images,
				"videos":      branch.Videos,
				"status":      branch.Status, // Include branch status
				"socialMedia": branch.SocialMedia,
				"createdAt":   branch.CreatedAt,
				"updatedAt":   branch.UpdatedAt,
				"company": map[string]interface{}{
					"id":           company.ID.Hex(),
					"businessName": company.BusinessName,
					"email":        user.Email,
					"contactInfo": map[string]interface{}{
						"phone": company.ContactInfo.Phone,
					},
					"socialMedia": map[string]interface{}{
						"instagram": company.SocialMedia.Instagram,
						"facebook":  company.SocialMedia.Facebook,
					},
				},
			}
			allBranches = append(allBranches, branchData)
		}
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "All branches retrieved successfully",
		Data:    allBranches,
	})
}

// Add to company_controller.go
func (cc *CompanyController) CreateBranchComment(c echo.Context) error {
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

	// Parse request body
	var commentRequest struct {
		Comment string `json:"comment"`
		Rating  int    `json:"rating,omitempty"`
	}
	if err := c.Bind(&commentRequest); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request data",
		})
	}

	// Validate comment
	if commentRequest.Comment == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Comment cannot be empty",
		})
	}

	// Validate rating if provided
	if commentRequest.Rating != 0 && (commentRequest.Rating < 1 || commentRequest.Rating > 5) {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Rating must be between 1 and 5",
		})
	}

	// Get user details
	usersCollection := config.GetCollection(cc.DB, "users")
	var user models.User
	err = usersCollection.FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to get user details",
		})
	}

	// Create comment object
	comment := models.BranchComment{
		ID:         primitive.NewObjectID(),
		BranchID:   branchObjectID,
		UserID:     userID,
		UserName:   user.FullName,
		UserAvatar: user.ProfilePic,
		Comment:    commentRequest.Comment,
		Rating:     commentRequest.Rating,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	// Insert comment into database
	commentsCollection := config.GetCollection(cc.DB, "branch_comments")
	_, err = commentsCollection.InsertOne(ctx, comment)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to save comment",
		})
	}

	return c.JSON(http.StatusCreated, models.Response{
		Status:  http.StatusCreated,
		Message: "Comment posted successfully",
		Data:    comment,
	})
}

// Add to company_controller.go
func (cc *CompanyController) GetBranchComments(c echo.Context) error {
	// Create context with timeout
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

	// Find comments for this branch
	commentsCollection := config.GetCollection(cc.DB, "branch_comments")
	filter := bson.M{"branchId": branchObjectID}
	opts := options.Find().SetSort(bson.M{"createdAt": -1}).SetSkip(int64(skip)).SetLimit(int64(limit))

	cursor, err := commentsCollection.Find(ctx, filter, opts)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve comments",
		})
	}
	defer cursor.Close(ctx)

	var comments []models.BranchComment
	if err = cursor.All(ctx, &comments); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode comments",
		})
	}

	// Get total count for pagination
	total, err := commentsCollection.CountDocuments(ctx, filter)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to count comments",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Comments retrieved successfully",
		Data: map[string]interface{}{
			"comments": comments,
			"total":    total,
			"page":     page,
			"limit":    limit,
		},
	})
}

// Add to company_controller.go
func (cc *CompanyController) ReplyToBranchComment(c echo.Context) error {
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

	// Get comment ID from URL parameter
	commentID := c.Param("commentId")
	if commentID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Comment ID is required",
		})
	}

	// Convert string comment ID to ObjectID
	commentObjectID, err := primitive.ObjectIDFromHex(commentID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid comment ID format",
		})
	}

	// Parse request body
	var replyRequest models.CommentReplyRequest
	if err := c.Bind(&replyRequest); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request data",
		})
	}

	// Validate reply
	if replyRequest.Reply == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Reply cannot be empty",
		})
	}

	// Get the company details of the logged-in user
	companiesCollection := config.GetCollection(cc.DB, "companies")
	var company models.Company
	err = companiesCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&company)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to get company details or user is not associated with a company",
		})
	}

	// Create reply object
	reply := models.CommentReply{
		ID:        primitive.NewObjectID(),
		CompanyID: company.ID,
		Reply:     replyRequest.Reply,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Add reply to the comment
	commentsCollection := config.GetCollection(cc.DB, "branch_comments")
	update := bson.M{
		"$push": bson.M{"replies": reply},
		"$set":  bson.M{"updatedAt": time.Now()},
	}

	result, err := commentsCollection.UpdateOne(
		ctx,
		bson.M{"_id": commentObjectID},
		update,
	)

	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to save reply",
		})
	}

	if result.MatchedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Comment not found",
		})
	}

	// Get the updated comment with the new reply
	var updatedComment models.BranchComment
	err = commentsCollection.FindOne(ctx, bson.M{"_id": commentObjectID}).Decode(&updatedComment)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to get updated comment",
		})
	}

	return c.JSON(http.StatusCreated, models.Response{
		Status:  http.StatusCreated,
		Message: "Reply posted successfully",
		Data:    updatedComment,
	})
}

type UpdateCompanyProfileRequest struct {
	BusinessName    string `json:"businessName"`
	Email           string `json:"email"`
	CurrentPassword string `json:"currentPassword,omitempty"`
	NewPassword     string `json:"newPassword,omitempty"`
	FullName        string `json:"fullName"`
	Phone           string `json:"phone,omitempty"`
	Website         string `json:"website,omitempty"`
	WhatsApp        string `json:"whatsapp,omitempty"`
	Facebook        string `json:"facebook,omitempty"`
	Instagram       string `json:"instagram,omitempty"`
}

// UpdateCompanyProfile handles updating company profile info including credentials
func (cc *CompanyController) UpdateCompanyProfile(c echo.Context) error {
	// Get user ID from token
	userID, err := utils.GetUserIDFromToken(c)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Invalid authentication token",
		})
	}

	// Parse multipart form for file and data
	form, err := c.MultipartForm()
	if err != nil {
		// If not multipart, try to bind JSON
		var req UpdateCompanyProfileRequest
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, models.Response{
				Status:  http.StatusBadRequest,
				Message: "Invalid request format",
			})
		}
		return cc.updateCompanyInfo(c, userID, req, nil)
	}

	// Handle form data
	files := form.File["logo"]
	var file *multipart.FileHeader
	if len(files) > 0 {
		file = files[0]
	}

	// Parse profile data from form
	var req UpdateCompanyProfileRequest
	req.BusinessName = c.FormValue("businessName")
	req.Email = c.FormValue("email")
	req.CurrentPassword = c.FormValue("currentPassword")
	req.NewPassword = c.FormValue("newPassword")
	req.FullName = c.FormValue("fullName")
	req.Phone = c.FormValue("phone")
	req.Website = c.FormValue("website")
	req.WhatsApp = c.FormValue("whatsapp")
	req.Facebook = c.FormValue("facebook")
	req.Instagram = c.FormValue("instagram")

	return cc.updateCompanyInfo(c, userID, req, file)
}

// updateCompanyInfo handles the actual update logic
func (cc *CompanyController) updateCompanyInfo(c echo.Context, userID primitive.ObjectID, req UpdateCompanyProfileRequest, logoFile *multipart.FileHeader) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Initialize update operations
	userUpdate := bson.M{
		"updatedAt": time.Now(),
	}
	companyUpdate := bson.M{
		"updatedAt": time.Now(),
	}

	// Get current user data for validation
	usersCollection := cc.DB.Database("barrim").Collection("users")
	companiesCollection := cc.DB.Database("barrim").Collection("companies")

	var currentUser models.User
	err := usersCollection.FindOne(ctx, bson.M{"_id": userID}).Decode(&currentUser)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Error retrieving user data",
		})
	}

	// Find company associated with this user
	var company models.Company
	err = companiesCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&company)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Error retrieving company data",
		})
	}

	// Validate and update password if provided
	if req.CurrentPassword != "" && req.NewPassword != "" {
		// Verify current password
		err = bcrypt.CompareHashAndPassword([]byte(currentUser.Password), []byte(req.CurrentPassword))
		if err != nil {
			return c.JSON(http.StatusBadRequest, models.Response{
				Status:  http.StatusBadRequest,
				Message: "Current password is incorrect",
			})
		}

		// Hash new password
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Error processing new password",
			})
		}

		userUpdate["password"] = string(hashedPassword)
	}

	// Update email if provided and different from current
	if req.Email != "" && req.Email != currentUser.Email {
		// Check if email is already in use
		var existingUser models.User
		err = usersCollection.FindOne(ctx, bson.M{
			"email": req.Email,
			"_id":   bson.M{"$ne": userID},
		}).Decode(&existingUser)

		if err == nil {
			return c.JSON(http.StatusConflict, models.Response{
				Status:  http.StatusConflict,
				Message: "Email is already in use",
			})
		} else if err != mongo.ErrNoDocuments {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Error checking email availability",
			})
		}

		userUpdate["email"] = req.Email
	}

	// Update full name if provided
	if req.FullName != "" {
		userUpdate["fullName"] = req.FullName
	}

	// Update business name if provided
	if req.BusinessName != "" {
		companyUpdate["businessName"] = req.BusinessName
	}

	// Update contact info
	contactUpdates := bson.M{}

	if req.Phone != "" {
		contactUpdates["phone"] = req.Phone
	}

	if req.Website != "" {
		contactUpdates["website"] = req.Website
	}

	if req.WhatsApp != "" {
		contactUpdates["whatsapp"] = req.WhatsApp
	}

	// If any contact info updates, add to company update
	if len(contactUpdates) > 0 {
		companyUpdate["contactInfo"] = bson.M{
			"$set": contactUpdates,
		}
	}

	// Update social media
	socialMediaUpdates := bson.M{}

	if req.Facebook != "" {
		socialMediaUpdates["facebook"] = req.Facebook
	}

	if req.Instagram != "" {
		socialMediaUpdates["instagram"] = req.Instagram
	}

	// If any social media updates, add to company update
	if len(socialMediaUpdates) > 0 {
		companyUpdate["socialMedia"] = bson.M{
			"$set": socialMediaUpdates,
		}
	}

	// Handle logo upload if provided
	var logoURL string
	if logoFile != nil {
		logoURL, err = cc.uploadCompanyLogo(c, logoFile, company.ID.Hex())
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Error uploading logo: " + err.Error(),
			})
		}
		companyUpdate["logoUrl"] = logoURL
	}

	// Perform updates if there are any changes to make
	if len(userUpdate) > 1 {
		_, err = usersCollection.UpdateOne(
			ctx,
			bson.M{"_id": userID},
			bson.M{"$set": userUpdate},
		)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Error updating user information",
			})
		}
	}

	if len(companyUpdate) > 1 {
		_, err = companiesCollection.UpdateOne(
			ctx,
			bson.M{"_id": company.ID},
			bson.M{"$set": companyUpdate},
		)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Error updating company information",
			})
		}
	}

	// Get updated company data
	var updatedCompany models.Company
	err = companiesCollection.FindOne(ctx, bson.M{"_id": company.ID}).Decode(&updatedCompany)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Error retrieving updated company data",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Company profile updated successfully",
		Data:    updatedCompany,
	})
}

// uploadCompanyLogo handles the logo file upload
func (cc *CompanyController) uploadCompanyLogo(c echo.Context, file *multipart.FileHeader, companyID string) (string, error) {
	// Create upload directory if it doesn't exist
	uploadDir := "uploads/logo"
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return "", err
	}

	// Open the uploaded file
	src, err := file.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()

	// Generate a unique filename based on company ID and timestamp
	fileExt := filepath.Ext(file.Filename)
	allowedExts := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".gif": true}

	if !allowedExts[strings.ToLower(fileExt)] {
		return "", fmt.Errorf("unsupported file type, only .jpg, .jpeg, .png, and .gif are allowed")
	}

	filename := fmt.Sprintf("%s_%d%s", companyID, time.Now().Unix(), fileExt)
	filepath := filepath.Join(uploadDir, filename)

	// Create destination file
	dst, err := os.Create(filepath)
	if err != nil {
		return "", err
	}
	defer dst.Close()

	// Copy the uploaded file to the destination file
	if _, err = io.Copy(dst, src); err != nil {
		return "", err
	}

	// Return the relative path to the file
	return filepath, nil
}

// GetBranchRating retrieves the average rating for a branch
// CalculateBranchRating calculates the average rating for a branch
func (cc *CompanyController) CalculateBranchRating(branchID primitive.ObjectID) (float64, int, error) {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Find all comments for this branch that have ratings
	commentsCollection := config.GetCollection(cc.DB, "branch_comments")
	filter := bson.M{
		"branchId": branchID,
		"rating": bson.M{
			"$exists": true,
			"$ne":     nil,
		},
	}

	// Find all comments with ratings
	cursor, err := commentsCollection.Find(ctx, filter)
	if err != nil {
		return 0, 0, err
	}
	defer cursor.Close(ctx)

	var comments []models.BranchComment
	if err = cursor.All(ctx, &comments); err != nil {
		return 0, 0, err
	}

	// If no ratings found, return 0
	if len(comments) == 0 {
		return 0, 0, nil
	}

	// Calculate the sum of all ratings
	var totalRating int
	for _, comment := range comments {
		totalRating += comment.Rating
	}

	// Calculate the average rating
	averageRating := float64(totalRating) / float64(len(comments))

	// Return the average rating and the count of ratings
	return averageRating, len(comments), nil
}

// GetBranchRating retrieves the average rating for a branch
func (cc *CompanyController) GetBranchRating(c echo.Context) error {
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

	// Calculate the average rating
	averageRating, ratingCount, err := cc.CalculateBranchRating(branchObjectID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to calculate branch rating",
		})
	}

	// Fetch the branch to include in response
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	branchesCollection := config.GetCollection(cc.DB, "companies")

	// Find the branch using the aggregation pipeline to filter nested documents
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"branches._id": branchObjectID,
		}}},
		{{Key: "$project", Value: bson.M{
			"branch": bson.M{
				"$filter": bson.M{
					"input": "$branches",
					"as":    "branch",
					"cond":  bson.M{"$eq": []interface{}{"$branch._id", branchObjectID}},
				},
			},
		}}},
		{{Key: "$project", Value: bson.M{
			"branch": bson.M{"$arrayElemAt": []interface{}{"$branch", 0}},
		}}},
	}

	cursor, err := branchesCollection.Aggregate(ctx, pipeline)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve branch",
		})
	}
	defer cursor.Close(ctx)

	var results []bson.M
	if err = cursor.All(ctx, &results); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode branch",
		})
	}

	if len(results) == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Branch not found",
		})
	}

	// Return the average rating data
	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Branch rating retrieved successfully",
		Data: map[string]interface{}{
			"branchId":      branchObjectID,
			"averageRating": averageRating,
			"ratingCount":   ratingCount,
			"branch":        results[0]["branch"],
		},
	})
}

// GetAllCompanies returns all companies (admin, or manager/sales_manager with business_management role)
func (cc *CompanyController) GetAllCompanies(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	if claims.UserType == "admin" {
		// allow
	} else if claims.UserType == "manager" {
		var manager models.Manager
		err := cc.DB.Database("barrim").Collection("managers").FindOne(context.Background(), bson.M{"_id": claims.UserID}).Decode(&manager)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to fetch manager",
			})
		}
		if !hasRole(manager.RolesAccess, "business_management") {
			return c.JSON(http.StatusForbidden, models.Response{
				Status:  http.StatusForbidden,
				Message: "Manager does not have business_management role",
			})
		}
	} else if claims.UserType == "sales_manager" {
		var salesManager models.SalesManager
		err := cc.DB.Database("barrim").Collection("sales_managers").FindOne(context.Background(), bson.M{"_id": claims.UserID}).Decode(&salesManager)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to fetch sales manager",
			})
		}
		if !hasRole(salesManager.RolesAccess, "business_management") {
			return c.JSON(http.StatusForbidden, models.Response{
				Status:  http.StatusForbidden,
				Message: "Sales manager does not have business_management role",
			})
		}
	} else {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins, managers, or sales managers can view all companies",
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	companyCollection := config.GetCollection(cc.DB, "companies")
	cursor, err := companyCollection.Find(ctx, bson.M{})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch companies",
		})
	}
	defer cursor.Close(ctx)

	var companies []models.Company
	if err := cursor.All(ctx, &companies); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode companies",
		})
	}

	// Get user information for contact details (including email)
	db := cc.DB.Database("barrim")
	userIDs := make([]primitive.ObjectID, 0, len(companies))
	for _, company := range companies {
		userIDs = append(userIDs, company.UserID)
	}

	userMap := make(map[string]models.User)
	if len(userIDs) > 0 {
		userCursor, err := db.Collection("users").Find(
			ctx,
			bson.M{"_id": bson.M{"$in": userIDs}},
		)
		if err == nil {
			var users []models.User
			_ = userCursor.All(ctx, &users)
			userCursor.Close(ctx)
			for _, user := range users {
				userMap[user.ID.Hex()] = user
			}
		}
	}

	// Enrich companies with user email information
	var enrichedCompanies []map[string]interface{}
	for _, company := range companies {
		user := userMap[company.UserID.Hex()]

		enrichedCompany := map[string]interface{}{
			"id":           company.ID,
			"userId":       company.UserID,
			"email":        user.Email,
			"businessName": company.BusinessName,
			"category":     company.Category,
			"subCategory":  company.SubCategory,
			"referralCode": company.ReferralCode,
			"points":       company.Points,
			"contactInfo":  company.ContactInfo,
			"socialMedia":  company.SocialMedia,
			"balance":      company.Balance,
			"branches":     company.Branches,
			"createdBy":    company.CreatedBy,
			"createdAt":    company.CreatedAt,
			"updatedAt":    company.UpdatedAt,
			"logoUrl":      company.LogoURL,
			"sponsorship":  company.Sponsorship,
		}

		enrichedCompanies = append(enrichedCompanies, enrichedCompany)
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Companies retrieved successfully",
		Data:    enrichedCompanies,
	})
}

// CreateCompanyBranchSponsorshipRequest allows companies to create sponsorship requests for their branches
func (cc *CompanyController) CreateCompanyBranchSponsorshipRequest(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user information from token
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "company" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only companies can create sponsorship requests for their branches",
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

	// Get company by user ID
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	companyCollection := config.GetCollection(cc.DB, "companies")
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
			Message: "Branch not found or you don't have access to it",
		})
	}

	// Check if sponsorship exists and is valid
	sponsorshipCollection := config.GetCollection(cc.DB, "sponsorships")
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
	subscriptionCollection := config.GetCollection(cc.DB, "sponsorship_subscriptions")
	var existingSubscription models.SponsorshipSubscription
	err = subscriptionCollection.FindOne(ctx, bson.M{
		"sponsorshipId": req.SponsorshipID,
		"entityId":      branchObjectID,
		"entityType":    "company_branch",
		"status":        bson.M{"$in": []string{"active", "pending"}},
	}).Decode(&existingSubscription)
	if err == nil {
		return c.JSON(http.StatusConflict, models.Response{
			Status:  http.StatusConflict,
			Message: "This branch already has a pending or active subscription for this sponsorship",
		})
	}

	// Check if there's already a pending request for this branch and sponsorship
	existingRequestCollection := config.GetCollection(cc.DB, "sponsorship_subscription_requests")
	var existingRequest models.SponsorshipSubscriptionRequest
	err = existingRequestCollection.FindOne(ctx, bson.M{
		"sponsorshipId": req.SponsorshipID,
		"entityId":      branchObjectID,
		"entityType":    "company_branch",
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
		EntityType:    "company_branch",
		EntityID:      branchObjectID,
		EntityName:    fmt.Sprintf("%s - %s", company.BusinessName, branch.Name),
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
		appURL = "barrim://payment"
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
		Invoice:            fmt.Sprintf("Company Branch Sponsorship - %s - %s - Sponsorship: %s", company.BusinessName, branch.Name, sponsorship.Title),
		ExternalID:         &externalID,
		SuccessCallbackURL: fmt.Sprintf("%s/api/whish/company-branch/sponsorship/payment/callback/success", baseURL),
		FailureCallbackURL: fmt.Sprintf("%s/api/whish/company-branch/sponsorship/payment/callback/failure", baseURL),
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

	log.Printf("Whish payment created for company branch sponsorship request %s: %s", subscriptionRequest.ID.Hex(), collectURL)

	// Send notification to admin (optional)
	adminEmail := os.Getenv("ADMIN_EMAIL")
	if adminEmail != "" {
		subject := "New Company Branch Sponsorship Request"
		body := fmt.Sprintf("A new sponsorship request has been submitted by company: %s\nBranch: %s\nSponsorship: %s\nPrice: $%.2f\nRequested At: %s\n",
			company.BusinessName, branch.Name, sponsorship.Title, sponsorship.Price, subscriptionRequest.RequestedAt.Format("2006-01-02 15:04:05"))

		// For now, just log the notification since we don't have email functionality in company controller
		log.Printf("Admin notification (to %s): %s - %s", adminEmail, subject, body)
	}

	return c.JSON(http.StatusCreated, models.Response{
		Status:  http.StatusCreated,
		Message: "Sponsorship subscription request created successfully. Please complete the payment.",
		Data: map[string]interface{}{
			"requestId":   subscriptionRequest.ID,
			"sponsorship": sponsorship,
			"branch":      branch,
			"company":     company.BusinessName,
			"status":      subscriptionRequest.Status,
			"submittedAt": subscriptionRequest.RequestedAt,
			"adminNote":   subscriptionRequest.AdminNote,
			"paymentUrl":  collectURL,
			"price":       sponsorship.Price,
		},
	})
}

// HandleWhishSponsorshipPaymentSuccess handles Whish payment success callback for company branch sponsorship
func (cc *CompanyController) HandleWhishSponsorshipPaymentSuccess(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log.Printf("==========================================")
	log.Printf("💰 COMPANY SPONSORSHIP PAYMENT CALLBACK RECEIVED")
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
	db := cc.DB.Database("barrim")
	requestCollection := db.Collection("sponsorship_subscription_requests")
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
	sponsorshipCollection := db.Collection("sponsorships")
	var sponsorship models.Sponsorship
	err = sponsorshipCollection.FindOne(ctx, bson.M{"_id": subscriptionRequest.SponsorshipID}).Decode(&sponsorship)
	if err != nil {
		log.Printf("Failed to get sponsorship details: %v", err)
		return c.String(http.StatusInternalServerError, "Failed to get sponsorship details")
	}

	// Add sponsorship income to admin wallet using sponsorship subscription controller
	sponsorshipSubscriptionController := NewSponsorshipSubscriptionController(db)
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

	log.Printf("✅ PAYMENT SUCCESS: Company branch sponsorship payment completed and activated")
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

// HandleWhishSponsorshipPaymentFailure handles Whish payment failure callback for company branch sponsorship
func (cc *CompanyController) HandleWhishSponsorshipPaymentFailure(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log.Printf("==========================================")
	log.Printf("❌ COMPANY SPONSORSHIP PAYMENT FAILURE CALLBACK RECEIVED")
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
	db := cc.DB.Database("barrim")
	requestCollection := db.Collection("sponsorship_subscription_requests")
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

	log.Printf("❌ PAYMENT FAILED: Company sponsorship payment failed")
	log.Printf("   External ID: %d", externalID)
	log.Printf("   Status: Marked as failed in database")
	log.Printf("==========================================")
	return c.String(http.StatusOK, "Payment failure recorded")
}

// GetCompanyBranchSponsorshipRemainingTime gets the remaining time for a company branch's sponsorship subscription
func (cc *CompanyController) GetCompanyBranchSponsorshipRemainingTime(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user information from token
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "company" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only companies can access this endpoint",
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

	// Get company by user ID to verify ownership
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	companyCollection := config.GetCollection(cc.DB, "companies")
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
	var branch models.Branch
	for _, b := range company.Branches {
		if b.ID == branchObjectID {
			branch = b
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

	// Find active sponsorship subscription for this company branch
	collection := config.GetCollection(cc.DB, "sponsorship_subscriptions")
	var subscription models.SponsorshipSubscription
	err = collection.FindOne(ctx, bson.M{
		"entityType": "company_branch",
		"entityId":   branchObjectID,
		"status":     "active",
	}).Decode(&subscription)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusOK, models.Response{
				Status:  http.StatusOK,
				Message: "No active sponsorship subscription found for this company branch",
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
	sponsorshipCollection := config.GetCollection(cc.DB, "sponsorships")
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
		Message: "Company branch sponsorship subscription remaining time retrieved successfully",
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
				"branchName":  branch.Name,
				"companyName": company.BusinessName,
			},
		},
	})
}
