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
	"github.com/HSouheill/barrim_backend/utils"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// UploadLogo handles uploading a wholesaler logo
func (wc *WholesalerController) UploadLogo(c echo.Context) error {
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

	// Find wholesaler by user ID
	wholesalerCollection := config.GetCollection(wc.DB, "wholesalers")
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
	filename := "logo_" + wholesaler.ID.Hex() + "_" + uuid.New().String() + fileExt
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
	if wholesaler.LogoURL != "" {
		if err := os.Remove(wholesaler.LogoURL); err != nil {
			log.Printf("Warning: Failed to delete old logo %s: %v", wholesaler.LogoURL, err)
		}
	}

	// Update wholesaler with new logo URL
	update := bson.M{
		"$set": bson.M{
			"logoUrl":   uploadPath,
			"updatedAt": time.Now(),
		},
	}

	result, err := wholesalerCollection.UpdateByID(ctx, wholesaler.ID, update)
	if err != nil {
		log.Printf("Error updating wholesaler logo URL: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update wholesaler logo: " + err.Error(),
		})
	}

	if result.ModifiedCount == 0 {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update wholesaler logo",
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

// UpdateWholesalerData handles updating wholesaler profile information
func (wc *WholesalerController) UpdateWholesalerData(c echo.Context) error {
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

	// Find wholesaler by user ID
	wholesalerCollection := config.GetCollection(wc.DB, "wholesalers")
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

	// Update wholesaler document
	result, err := wholesalerCollection.UpdateByID(ctx, wholesaler.ID, update)
	if err != nil {
		log.Printf("Error updating wholesaler data: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update wholesaler data: " + err.Error(),
		})
	}

	if result.ModifiedCount == 0 {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "No changes applied to wholesaler data",
		})
	}

	// Get updated wholesaler data to return
	var updatedWholesaler models.Wholesaler
	err = wholesalerCollection.FindOne(ctx, bson.M{"_id": wholesaler.ID}).Decode(&updatedWholesaler)
	if err != nil {
		log.Printf("Error retrieving updated wholesaler data: %v", err)
		return c.JSON(http.StatusOK, models.Response{
			Status:  http.StatusOK,
			Message: "Wholesaler updated successfully",
		})
	}

	// Return updated wholesaler data
	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Wholesaler updated successfully",
		Data: map[string]interface{}{
			"wholesalerInfo": map[string]interface{}{
				"name":        updatedWholesaler.BusinessName,
				"category":    updatedWholesaler.Category,
				"subCategory": updatedWholesaler.SubCategory,
				"logo":        updatedWholesaler.LogoURL,
			},
			"contactInfo": updatedWholesaler.ContactInfo,
			"socialMedia": updatedWholesaler.SocialMedia,
		},
	})
}

// ChangeWholesalerDetails allows a wholesaler to update their password, logo, and email
// ChangeWholesalerDetails allows a wholesaler to update their password, logo, and email
// after verifying their current password
func (wc *WholesalerController) ChangeWholesalerDetails(c echo.Context) error {
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

	// Parse form data
	form, err := c.MultipartForm()
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Failed to parse form data: " + err.Error(),
		})
	}

	// Get user collection to verify password
	userCollection := config.GetCollection(wc.DB, "users")
	var user models.User
	err = userCollection.FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "User not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find user",
		})
	}

	// Get wholesaler collection
	wholesalerCollection := config.GetCollection(wc.DB, "wholesalers")
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

	// Verify current password - required for any change
	currentPassword := ""
	if currentPasswords := form.Value["currentPassword"]; len(currentPasswords) > 0 {
		currentPassword = currentPasswords[0]
	}

	if currentPassword == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Current password is required to make changes",
		})
	}

	// Verify the current password
	err = utils.CheckPassword(currentPassword, user.Password)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Current password is incorrect",
		})
	}

	updateFields := bson.M{
		"updatedAt": time.Now(),
	}

	userUpdateFields := bson.M{
		"updatedAt": time.Now(),
	}

	changesRequested := false
	userChangesRequested := false

	// Handle password change
	if passwords := form.Value["newPassword"]; len(passwords) > 0 && passwords[0] != "" {
		hashedPassword, err := utils.HashPassword(passwords[0])
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to hash password",
			})
		}
		userUpdateFields["password"] = hashedPassword
		userChangesRequested = true
		changesRequested = true
	}

	// Handle email change
	if emails := form.Value["email"]; len(emails) > 0 && emails[0] != "" && emails[0] != user.Email {
		// Check if email is already in use
		var existingUser models.User
		err = userCollection.FindOne(ctx, bson.M{"email": emails[0]}).Decode(&existingUser)
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

		userUpdateFields["email"] = emails[0]
		userChangesRequested = true
		changesRequested = true
	}

	// Handle logo change
	if logoFiles := form.File["logo"]; len(logoFiles) > 0 {
		file := logoFiles[0]

		// Check file size
		if file.Size > 5*1024*1024 { // 5MB limit
			return c.JSON(http.StatusBadRequest, models.Response{
				Status:  http.StatusBadRequest,
				Message: "Logo file is too large. Maximum size is 5MB.",
			})
		}

		fileExt := filepath.Ext(file.Filename)
		if fileExt != ".jpg" && fileExt != ".jpeg" && fileExt != ".png" {
			return c.JSON(http.StatusBadRequest, models.Response{
				Status:  http.StatusBadRequest,
				Message: "Invalid file type. Only JPG, JPEG, and PNG files are allowed.",
			})
		}

		// Generate unique filename
		filename := "logo_" + wholesaler.ID.Hex() + "_" + uuid.New().String() + fileExt
		uploadPath := filepath.Join("uploads", "logos", filename)

		// Ensure uploads directory exists
		os.MkdirAll(filepath.Join("uploads", "logos"), 0755)

		// Save file to uploads directory
		src, err := file.Open()
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to process logo file: " + err.Error(),
			})
		}
		defer src.Close()

		dst, err := os.Create(uploadPath)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to save logo file: " + err.Error(),
			})
		}
		defer dst.Close()

		if _, err = io.Copy(dst, src); err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to save logo file: " + err.Error(),
			})
		}

		// Delete old logo if it exists
		if wholesaler.LogoURL != "" {
			if err := os.Remove(wholesaler.LogoURL); err != nil {
				log.Printf("Warning: Failed to delete old logo %s: %v", wholesaler.LogoURL, err)
			}
		}

		updateFields["logoUrl"] = uploadPath
		changesRequested = true
	}

	if !changesRequested && !userChangesRequested {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "No changes requested",
		})
	}

	// Update user document if needed
	if userChangesRequested {
		userUpdate := bson.M{
			"$set": userUpdateFields,
		}

		userResult, err := userCollection.UpdateByID(ctx, userID, userUpdate)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to update user details: " + err.Error(),
			})
		}

		if userResult.ModifiedCount == 0 {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "No changes applied to user details",
			})
		}
	}

	// Update wholesaler document if needed
	if len(updateFields) > 1 { // More than just updatedAt
		update := bson.M{
			"$set": updateFields,
		}

		result, err := wholesalerCollection.UpdateByID(ctx, wholesaler.ID, update)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to update wholesaler details: " + err.Error(),
			})
		}

		if result.ModifiedCount == 0 && !userChangesRequested {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "No changes applied to wholesaler details",
			})
		}
	}

	// Get updated data to return to client
	var updatedWholesaler models.Wholesaler
	err = wholesalerCollection.FindOne(ctx, bson.M{"_id": wholesaler.ID}).Decode(&updatedWholesaler)

	responseData := map[string]interface{}{
		"message": "Details updated successfully",
	}

	if len(updateFields) > 1 {
		responseData["logoUrl"] = updatedWholesaler.LogoURL
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Wholesaler details updated successfully",
		Data:    responseData,
	})
}

// GetAllBranches retrieves all branches from all wholesalers with branch status
func (wc *WholesalerController) GetAllBranches(c echo.Context) error {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Get wholesaler collection
	wholesalerCollection := config.GetCollection(wc.DB, "wholesalers")

	// Find all wholesalers with branches
	cursor, err := wholesalerCollection.Find(ctx, bson.M{"branches": bson.M{"$exists": true, "$not": bson.M{"$size": 0}}})
	if err != nil {
		log.Printf("Error finding wholesalers with branches: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve wholesalers with branches",
		})
	}
	defer cursor.Close(ctx)

	var wholesalers []models.Wholesaler
	if err = cursor.All(ctx, &wholesalers); err != nil {
		log.Printf("Error decoding wholesalers: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode wholesalers",
		})
	}

	// Prepare response data - flatten all branches with wholesaler reference and status
	var allBranches []map[string]interface{}
	for _, wholesaler := range wholesalers {
		for _, branch := range wholesaler.Branches {
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
				"createdAt":   branch.CreatedAt,
				"updatedAt":   branch.UpdatedAt,
				"wholesaler": map[string]interface{}{
					"id":           wholesaler.ID.Hex(),
					"businessName": wholesaler.BusinessName,
					"contactInfo": map[string]interface{}{
						"phone": wholesaler.ContactInfo.Phone,
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

// EditBranch handles updating an existing branch
func (wc *WholesalerController) EditBranch(c echo.Context) error {
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
		CreatedAt:   existingBranch.CreatedAt,
		UpdatedAt:   time.Now(),
	}

	log.Printf("Prepared updated branch object: %+v", updatedBranch)

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
		},
	})
}
