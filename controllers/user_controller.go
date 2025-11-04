// controllers/user_controller.go
package controllers

import (
	"context"
	"fmt"
	"io"
	"log"
	"math"
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

	"github.com/HSouheill/barrim_backend/config"
	"github.com/HSouheill/barrim_backend/middleware"
	"github.com/HSouheill/barrim_backend/models"
	"github.com/HSouheill/barrim_backend/repositories"
	"github.com/HSouheill/barrim_backend/utils"
)

// UserController contains user management logic
type UserController struct {
	DB         *mongo.Client
	collection *mongo.Collection
	userRepo   *repositories.UserRepository
}

// NewUserController creates a new user controller
func NewUserController(db *mongo.Client, userRepo *repositories.UserRepository) *UserController {
	return &UserController{DB: db, userRepo: userRepo}
}

// GetProfile handler gets the current user's profile
func (uc *UserController) GetProfile(c echo.Context) error {
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user collection
	collection := config.GetCollection(uc.DB, "users")

	// Get user information from token
	claims := middleware.GetUserFromToken(c)
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	// Find user by ID
	var user models.User
	err = collection.FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
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

	log.Printf("User profile data: %+v", user)

	// Remove password from response
	user.Password = ""

	// Make sure ProfilePic field is included for all user types
	// It's already in the User struct, so we just need to ensure it's included in the response

	// Return user profile
	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Profile retrieved successfully",
		Data:    user,
	})
}

// UpdateLocation handler updates a user's location
func (uc *UserController) UpdateLocation(c echo.Context) error {
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user collection
	collection := config.GetCollection(uc.DB, "users")

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
	var locReq models.UpdateLocationRequest
	if err := c.Bind(&locReq); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}

	// Validate location
	if locReq.Location == nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Location is required",
		})
	}

	// Update user location
	update := bson.M{
		"$set": bson.M{
			"location":  locReq.Location,
			"updatedAt": time.Now(),
		},
	}

	result, err := collection.UpdateOne(ctx, bson.M{"_id": userID}, update)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update location",
		})
	}

	if result.MatchedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "User not found",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Location updated successfully",
	})
}

// UpdateProfile handler updates a user's profile
// Add this method to user_controller.go
func (uc *UserController) UpdateProfile(c echo.Context) error {
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user collection
	collection := config.GetCollection(uc.DB, "users")

	// Get user information from token
	claims := middleware.GetUserFromToken(c)
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	// Find user by ID
	var user models.User
	err = collection.FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
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

	// Check if user is of userType "user"
	if user.UserType != "user" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Access denied. Only regular users can update profile using this endpoint",
		})
	}

	// Parse request body
	var updateReq struct {
		FullName        string           `json:"fullName"`
		Email           string           `json:"email"`
		Phone           string           `json:"phone"`
		DateOfBirth     string           `json:"dateOfBirth"`
		Gender          string           `json:"gender"`
		CurrentPassword string           `json:"currentPassword"`
		InterestedDeals []string         `json:"interestedDeals"`
		ProfilePic      string           `json:"profilePic"`
		Location        *models.Location `json:"location"`
	}
	if err := c.Bind(&updateReq); err != nil {
		log.Println("Error binding request:", err) // Debug log
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}

	log.Printf("Received update request: %+v\n", updateReq) // Debug log

	// Validate required fields
	if updateReq.CurrentPassword == "" {
		fmt.Println("Current password missing in request") // Debug log
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Current password is required",
		})
	}

	err = utils.CheckPassword(updateReq.CurrentPassword, user.Password)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Invalid password",
		})
	}

	// Build update
	update := bson.M{
		"$set": bson.M{
			"updatedAt": time.Now(),
		},
	}

	// Only update fields that have changed
	if updateReq.FullName != "" && updateReq.FullName != user.FullName {
		update["$set"].(bson.M)["fullName"] = updateReq.FullName
	}

	if updateReq.Email != "" && updateReq.Email != user.Email {
		// Check if email already exists
		var existingUser models.User
		err := collection.FindOne(ctx, bson.M{"email": updateReq.Email}).Decode(&existingUser)
		if err == nil && existingUser.ID != userID {
			return c.JSON(http.StatusBadRequest, models.Response{
				Status:  http.StatusBadRequest,
				Message: "Email already in use",
			})
		}
		update["$set"].(bson.M)["email"] = updateReq.Email
	}

	// Add additional user-specific fields
	if updateReq.Phone != "" && updateReq.Phone != user.Phone {
		update["$set"].(bson.M)["phone"] = updateReq.Phone
	}

	if updateReq.DateOfBirth != "" && updateReq.DateOfBirth != user.DateOfBirth {
		update["$set"].(bson.M)["dateOfBirth"] = updateReq.DateOfBirth
	}

	if updateReq.Gender != "" && updateReq.Gender != user.Gender {
		update["$set"].(bson.M)["gender"] = updateReq.Gender
	}

	if updateReq.ProfilePic != "" && updateReq.ProfilePic != user.ProfilePic {
		update["$set"].(bson.M)["profilePic"] = updateReq.ProfilePic
	}

	if updateReq.InterestedDeals != nil {
		update["$set"].(bson.M)["interestedDeals"] = updateReq.InterestedDeals
	}

	if updateReq.Location != nil {
		update["$set"].(bson.M)["location"] = updateReq.Location
	}

	// Perform update if there are changes
	if len(update["$set"].(bson.M)) > 1 { // 1 because updatedAt is always set
		_, err = collection.UpdateOne(ctx, bson.M{"_id": userID}, update)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to update profile",
			})
		}
	}

	// Get updated user data to return
	var updatedUser models.User
	err = collection.FindOne(ctx, bson.M{"_id": userID}).Decode(&updatedUser)
	if err != nil {
		return c.JSON(http.StatusOK, models.Response{
			Status:  http.StatusOK,
			Message: "Profile updated successfully",
		})
	}

	// Remove sensitive fields before returning
	updatedUser.Password = ""
	updatedUser.OTPInfo = nil
	updatedUser.ResetPasswordToken = ""
	updatedUser.ResetTokenExpiresAt = time.Time{}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Profile updated successfully",
		Data:    updatedUser,
	})
}

// DeleteUser handler deletes the current user and associated company, wholesaler, or service provider
func (uc *UserController) DeleteUser(c echo.Context) error {
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user collection
	collection := config.GetCollection(uc.DB, "users")

	// Get user information from token
	claims := middleware.GetUserFromToken(c)
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	// Find the user to get related IDs
	var user models.User
	err = collection.FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
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

	// Delete associated company if exists
	if user.CompanyID != nil {
		companyCollection := config.GetCollection(uc.DB, "companies")
		_, _ = companyCollection.DeleteOne(ctx, bson.M{"_id": *user.CompanyID})
	}

	// Delete associated wholesaler if exists
	if user.WholesalerID != nil {
		wholesalerCollection := config.GetCollection(uc.DB, "wholesalers")
		_, _ = wholesalerCollection.DeleteOne(ctx, bson.M{"_id": *user.WholesalerID})
	}

	// Delete associated service provider if exists
	if user.ServiceProviderID != nil {
		// If service providers are stored in users collection with userType 'serviceProvider'
		_, _ = collection.DeleteOne(ctx, bson.M{"_id": *user.ServiceProviderID, "userType": "serviceProvider"})
		// If you have a separate collection for service providers, use that instead
	}

	// Delete user
	result, err := collection.DeleteOne(ctx, bson.M{"_id": userID})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to delete user",
		})
	}

	if result.DeletedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "User not found",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "User and associated records deleted successfully",
	})
}

// GetAllUsers handler returns all users with pagination
func (uc *UserController) GetAllUsers(c echo.Context) error {
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get pagination parameters
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit < 1 || limit > 100 {
		limit = 20 // default limit
	}
	skip := (page - 1) * limit

	// Get user collection
	collection := config.GetCollection(uc.DB, "users")

	// Set up options to exclude password field and apply pagination
	opts := options.Find().
		SetProjection(bson.M{"password": 0}).
		SetSkip(int64(skip)).
		SetLimit(int64(limit)).
		SetSort(bson.D{{Key: "createdAt", Value: -1}}) // Sort by creation date, newest first

	// Find all users
	cursor, err := collection.Find(ctx, bson.M{}, opts)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch users",
		})
	}
	defer cursor.Close(ctx)

	// Decode all users
	var users []models.User
	if err := cursor.All(ctx, &users); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode users",
		})
	}

	// Get total count for pagination info
	totalCount, err := collection.CountDocuments(ctx, bson.M{})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to count users",
		})
	}

	// Calculate pagination metadata
	totalPages := int(math.Ceil(float64(totalCount) / float64(limit)))

	// Return users with pagination info
	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Users retrieved successfully",
		Data: map[string]interface{}{
			"users": users,
			"pagination": map[string]interface{}{
				"totalCount": totalCount,
				"page":       page,
				"limit":      limit,
				"totalPages": totalPages,
			},
		},
	})
}

// UploadCompanyLogo handler uploads a company logo
func (uc *UserController) UploadCompanyLogo(c echo.Context) error {
	// Create a context with timeout
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

	// Get user collection
	collection := config.GetCollection(uc.DB, "users")

	// Check if user exists and is a company
	var user models.User
	err = collection.FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
	if err != nil {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "User not found",
		})
	}

	if user.UserType != "company" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Only company accounts can upload a logo",
		})
	}

	// Get file from form
	file, err := c.FormFile("logo")
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "No file uploaded or invalid file",
		})
	}

	// Validate file type (only images allowed)
	if !strings.HasPrefix(file.Header.Get("Content-Type"), "image/") {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Only image files are allowed",
		})
	}

	// Open the file
	src, err := file.Open()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to open uploaded file",
		})
	}
	defer src.Close()

	// Generate unique filename
	filename := userID.Hex() + "_" + time.Now().Format("20060102150405") + filepath.Ext(file.Filename)

	// Define storage path (you'll need to implement your own storage logic here)
	// This is just an example assuming you have a specific directory for file uploads
	storagePath := "./uploads/logos/" + filename

	// Create destination file
	dst, err := os.Create(storagePath)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to create destination file",
		})
	}
	defer dst.Close()

	// Copy the uploaded file to the destination file
	if _, err = io.Copy(dst, src); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to save uploaded file",
		})
	}

	// Update the user's companyInfo with the logo path
	logoURL := "/uploads/logos/" + filename // The URL from which the logo can be accessed

	// Update user in database
	update := bson.M{
		"$set": bson.M{
			"companyInfo.logo": logoURL,
			"updatedAt":        time.Now(),
		},
	}

	_, err = collection.UpdateOne(ctx, bson.M{"_id": userID}, update)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update company logo",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Company logo uploaded successfully",
		Data: map[string]string{
			"logoURL": logoURL,
		},
	})
}

// UploadProfilePhoto handler uploads a profile photo for service providers
func (uc *UserController) UploadProfilePhoto(c echo.Context) error {
	// Create a context with timeout
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

	// Get user collection
	collection := config.GetCollection(uc.DB, "users")

	// Check if user exists
	var user models.User
	err = collection.FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
	if err != nil {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "User not found",
		})
	}

	// Get file from form
	file, err := c.FormFile("photo")
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "No file uploaded or invalid file",
		})
	}

	// Validate file type (only images allowed)
	if !strings.HasPrefix(file.Header.Get("Content-Type"), "image/") {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Only image files are allowed",
		})
	}

	// Open the file
	src, err := file.Open()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to open uploaded file",
		})
	}
	defer src.Close()

	// Generate unique filename
	filename := userID.Hex() + "_" + time.Now().Format("20060102150405") + filepath.Ext(file.Filename)

	// Define storage path
	storagePath := "./uploads/profiles/" + filename

	// Create destination file
	dst, err := os.Create(storagePath)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to create destination file",
		})
	}
	defer dst.Close()

	// Copy the uploaded file to the destination file
	if _, err = io.Copy(dst, src); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to save uploaded file",
		})
	}

	// Update based on user type
	photoURL := "/uploads/profiles/" + filename
	update := bson.M{
		"$set": bson.M{
			"updatedAt": time.Now(),
		},
	}

	if user.UserType == "serviceProvider" {
		// For service providers, update the serviceProviderInfo.profilePhoto
		if user.ServiceProviderInfo == nil {
			update["$set"].(bson.M)["serviceProviderInfo"] = models.ServiceProviderInfo{
				ProfilePhoto: photoURL,
			}
		} else {
			update["$set"].(bson.M)["serviceProviderInfo.profilePhoto"] = photoURL
		}
	} else {
		// For regular users, update the profilePic field
		update["$set"].(bson.M)["profilePic"] = photoURL
	}

	_, err = collection.UpdateOne(ctx, bson.M{"_id": userID}, update)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update profile photo",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Profile photo uploaded successfully",
		Data: map[string]string{
			"photoURL": photoURL,
		},
	})
}

func (uc *UserController) UploadUserProfilePhoto(c echo.Context) error {
	// Create a context with timeout
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

	// Get user collection
	collection := config.GetCollection(uc.DB, "users")

	// Check if user exists and is a regular user
	var user models.User
	err = collection.FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
	if err != nil {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "User not found",
		})
	}

	if user.UserType != "user" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "This endpoint is only for regular users",
		})
	}

	// Get file from form
	file, err := c.FormFile("photo")
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "No file uploaded or invalid file",
		})
	}

	// Validate file type (only images allowed)
	if !strings.HasPrefix(file.Header.Get("Content-Type"), "image/") {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Only image files are allowed",
		})
	}

	// Open the file
	src, err := file.Open()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to open uploaded file",
		})
	}
	defer src.Close()

	// Generate unique filename
	filename := userID.Hex() + "_" + time.Now().Format("20060102150405") + filepath.Ext(file.Filename)

	// Define storage path
	storagePath := "./uploads/profiles/" + filename

	// Ensure directory exists
	os.MkdirAll("./uploads/profiles/", 0755)

	// Create destination file
	dst, err := os.Create(storagePath)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to create destination file",
		})
	}
	defer dst.Close()

	// Copy the uploaded file to the destination file
	if _, err = io.Copy(dst, src); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to save uploaded file",
		})
	}

	// Update the user's profilePic field
	photoURL := "/uploads/profiles/" + filename

	update := bson.M{
		"$set": bson.M{
			"profilePic": photoURL,
			"updatedAt":  time.Now(),
		},
	}

	_, err = collection.UpdateOne(ctx, bson.M{"_id": userID}, update)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update profile photo",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Profile photo uploaded successfully",
		Data: map[string]string{
			"photoURL": photoURL,
		},
	})
}

// UpdateAvailability updates a service provider's availability calendar
func (uc *UserController) UpdateAvailability(c echo.Context) error {
	// Create a context with timeout
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

	// Get user collection
	collection := config.GetCollection(uc.DB, "users")

	// Check if user exists and is a service provider
	var user models.User
	err = collection.FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
	if err != nil {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "User not found",
		})
	}

	if user.UserType != "serviceProvider" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Only service providers can update availability",
		})
	}

	// Parse request body
	var availabilityReq struct {
		AvailableDays  []string `json:"availableDays"`
		AvailableHours []string `json:"availableHours"`
	}

	if err := c.Bind(&availabilityReq); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}

	// Validate availability data
	if len(availabilityReq.AvailableDays) == 0 || len(availabilityReq.AvailableHours) == 0 {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Available days and hours are required",
		})
	}

	// Update the service provider's availability
	update := bson.M{
		"$set": bson.M{
			"updatedAt": time.Now(),
		},
	}

	if user.ServiceProviderInfo == nil {
		update["$set"].(bson.M)["serviceProviderInfo"] = models.ServiceProviderInfo{
			AvailableDays:  availabilityReq.AvailableDays,
			AvailableHours: availabilityReq.AvailableHours,
		}
	} else {
		update["$set"].(bson.M)["serviceProviderInfo.availableDays"] = availabilityReq.AvailableDays
		update["$set"].(bson.M)["serviceProviderInfo.availableHours"] = availabilityReq.AvailableHours
	}

	_, err = collection.UpdateOne(ctx, bson.M{"_id": userID}, update)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update availability",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Availability updated successfully",
	})
}

// SearchServiceProviders allows searching for service providers by type and location
func (uc *UserController) SearchServiceProviders(c echo.Context) error {
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user collection
	collection := config.GetCollection(uc.DB, "users")

	// Get pagination parameters
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit < 1 || limit > 100 {
		limit = 20 // default limit
	}
	skip := (page - 1) * limit

	// Get filter parameters
	serviceType := c.QueryParam("serviceType")
	city := c.QueryParam("city")
	country := c.QueryParam("country")

	// Build filter
	filter := bson.M{"userType": "serviceProvider"}

	if serviceType != "" {
		filter["serviceProviderInfo.serviceType"] = serviceType
	}

	if city != "" {
		filter["location.city"] = city
	}

	if country != "" {
		filter["location.country"] = country
	}

	// Set up options to exclude password field and apply pagination
	opts := options.Find().
		SetProjection(bson.M{"password": 0}).
		SetSkip(int64(skip)).
		SetLimit(int64(limit)).
		SetSort(bson.D{{Key: "createdAt", Value: -1}})

	// Find service providers matching the criteria
	cursor, err := collection.Find(ctx, filter, opts)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch service providers",
		})
	}
	defer cursor.Close(ctx)

	// Decode all service providers
	var providers []models.User
	if err := cursor.All(ctx, &providers); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode service providers",
		})
	}

	// Get total count for pagination info
	totalCount, err := collection.CountDocuments(ctx, filter)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to count service providers",
		})
	}

	// Calculate pagination metadata
	totalPages := int(math.Ceil(float64(totalCount) / float64(limit)))

	// Return service providers with pagination info
	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Service providers retrieved successfully",
		Data: map[string]interface{}{
			"serviceProviders": providers,
			"pagination": map[string]interface{}{
				"totalCount": totalCount,
				"page":       page,
				"limit":      limit,
				"totalPages": totalPages,
			},
		},
	})
}

// GetCompaniesWithLocations handler returns all companies with location data and branch status
func (uc *UserController) GetCompaniesWithLocations(c echo.Context) error {
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get companies collection instead of users collection
	collection := config.GetCollection(uc.DB, "companies")

	// Set up filter for companies with location data
	filter := bson.M{
		"contactInfo.address": bson.M{"$exists": true, "$ne": nil},
	}

	// Set up options to include all necessary fields
	opts := options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}})

	// Find companies
	cursor, err := collection.Find(ctx, filter, opts)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch companies",
		})
	}
	defer cursor.Close(ctx)

	// Decode all companies
	var companies []models.Company
	if err := cursor.All(ctx, &companies); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode companies",
		})
	}

	// Get user information for contact details (including email)
	db := uc.DB.Database("barrim")
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

	// Return companies with email information
	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Companies retrieved successfully",
		Data:    enrichedCompanies,
	})
}

// ChangePassword handler changes a user's password
func (uc *UserController) ChangePassword(c echo.Context) error {
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user collection
	collection := config.GetCollection(uc.DB, "users")

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
	var changePassReq struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := c.Bind(&changePassReq); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}

	// Validate required fields
	if changePassReq.CurrentPassword == "" || changePassReq.NewPassword == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Current and new password are required",
		})
	}

	// Password validation
	if len(changePassReq.NewPassword) < 8 {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Password must be at least 8 characters long",
		})
	}

	// Find user by ID
	var user models.User
	err = collection.FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
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

	// Verify current password
	match, err := utils.CheckPasswordHash(changePassReq.CurrentPassword, user.Password)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to verify password",
		})
	}
	if !match {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Current password is incorrect",
		})
	}

	// Hash the new password
	hashedPassword, err := utils.HashPassword(changePassReq.NewPassword)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to hash password",
		})
	}

	// Update user's password
	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": user.ID},
		bson.M{
			"$set": bson.M{
				"password":  hashedPassword,
				"updatedAt": time.Now(),
			},
		},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update password",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Password changed successfully",
	})
}

// GetUserData fetches the current user's data (for userType = "user")
// This function retrieves ALL user profile data including DateOfBirth, gender, location, interestedDeals, etc.
func (uc *UserController) GetUserData(c echo.Context) error {
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user collection
	collection := config.GetCollection(uc.DB, "users")

	// Get user information from token
	claims := middleware.GetUserFromToken(c)
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	// Find user by ID and ensure userType is "user"
	filter := bson.M{
		"_id":      userID,
		"userType": "user",
	}

	// Define which fields to include in the response
	// Using projection with 0 means exclude these specific fields, all others are included by default
	// This ensures we get ALL user data including:
	// - DateOfBirth, gender, phone, contactPerson, contactPhone
	// - Location (city, country, district, street, postalCode, lat, lng)
	// - InterestedDeals, profilePic, logoPath
	// - FavoriteBranches, favoriteServiceProviders
	// - CompanyID, wholesalerID, serviceProviderID
	// - Points, referralCode, referrals
	// - All timestamps and metadata
	projection := bson.M{
		"password":            0, // Exclude for security
		"resetPasswordToken":  0, // Exclude for security
		"resetTokenExpiresAt": 0, // Exclude for security
		"otpInfo":             0, // Exclude for security
	}

	// Set up options with projection
	opts := options.FindOne().SetProjection(projection)

	// Find the user
	var user models.User
	err = collection.FindOne(ctx, filter, opts).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "User not found or user type is not 'user'",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find user data",
		})
	}

	// Log the retrieved user data for debugging (remove in production)
	log.Printf("Retrieved user data: ID=%s, FullName=%s, DateOfBirth=%s, Gender=%s, Phone=%s, Location=%+v, InterestedDeals=%v",
		user.ID.Hex(), user.FullName, user.DateOfBirth, user.Gender, user.Phone, user.Location, user.InterestedDeals)

	// Return user data
	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "User data retrieved successfully",
		Data:    user,
	})
}

// UpdateUserPersonalInfo updates personal information for regular users
func (uc *UserController) UpdateUserPersonalInfo(c echo.Context) error {
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user collection
	collection := config.GetCollection(uc.DB, "users")

	// Get user ID from token
	userID, err := middleware.ExtractUserID(c)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Invalid token",
		})
	}

	// Parse request body
	var updateData struct {
		FullName    string           `json:"fullName"`
		Phone       string           `json:"phone"`
		DateOfBirth string           `json:"dateOfBirth"`
		Gender      string           `json:"gender"`
		Location    *models.Location `json:"location"`
	}

	if err := c.Bind(&updateData); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request data",
		})
	}

	// Convert string ID to ObjectID
	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	// Create update fields
	updateFields := bson.M{
		"fullName":    updateData.FullName,
		"phone":       updateData.Phone,
		"dateOfBirth": updateData.DateOfBirth,
		"gender":      updateData.Gender,
		"updatedAt":   time.Now(),
	}

	// Add location if provided
	if updateData.Location != nil {
		updateFields["location"] = updateData.Location
	}

	// Update user in database
	filter := bson.M{"_id": objID, "userType": "user"}
	update := bson.M{"$set": updateFields}

	result, err := collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update user",
		})
	}

	if result.MatchedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "User not found or not a regular user",
		})
	}

	// Get updated user data
	var updatedUser models.User
	err = collection.FindOne(ctx, bson.M{"_id": objID}).Decode(&updatedUser)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch updated user data",
		})
	}

	// Remove sensitive data before returning
	updatedUser.Password = ""
	updatedUser.OTPInfo = nil
	updatedUser.ResetPasswordToken = ""

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "User information updated successfully",
		Data:    updatedUser,
	})
}

// RegenerateAvailableDays updates a service provider's available days based on their weekday preferences
func (c *UserController) RegenerateAvailableDays(ctx echo.Context) error {
	userID := ctx.Param("id")

	// Validate ID
	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID format",
		})
	}

	// Get user from database
	collection := config.GetCollection(c.DB, "users")
	ctx2 := context.Background()

	var user models.User
	err = collection.FindOne(ctx2, bson.M{"_id": objID}).Decode(&user)
	if err != nil {
		return ctx.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "User not found",
		})
	}

	// Check if user is a service provider
	if user.UserType != "serviceProvider" || user.ServiceProviderInfo == nil {
		return ctx.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "User is not a service provider",
		})
	}

	// Check if the user has weekday availability set
	if !user.ServiceProviderInfo.ApplyToAllMonths || len(user.ServiceProviderInfo.AvailableWeekdays) == 0 {
		return ctx.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "User does not have weekday availability set",
		})
	}

	// Generate available days for the next 12 months
	startDate := time.Now()
	endDate := startDate.AddDate(1, 0, 0)
	newAvailableDays := user.ServiceProviderInfo.RegenerateAvailableDaysFromWeekdays(startDate, endDate)

	// Update the user in the database
	update := bson.M{
		"$set": bson.M{
			"serviceProviderInfo.availableDays": newAvailableDays,
			"updatedAt":                         time.Now(),
		},
	}

	_, err = collection.UpdateOne(ctx2, bson.M{"_id": objID}, update)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update available days: " + err.Error(),
		})
	}

	return ctx.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Available days updated successfully",
		Data: map[string]interface{}{
			"availableDays":     newAvailableDays,
			"availableWeekdays": user.ServiceProviderInfo.AvailableWeekdays,
		},
	})
}

func (uc *UserController) AddBranchToFavorites(c echo.Context) error {
	// Get the current user from the token
	user, err := utils.GetUserFromToken(c, uc.DB)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Unauthorized: " + err.Error(),
		})
	}

	// Parse the request to get branch ID
	var req struct {
		BranchID string `json:"branchId" validate:"required"`
	}

	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request format",
		})
	}

	// Validate required fields
	if req.BranchID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Branch ID is required",
		})
	}

	// Convert string ID to ObjectID
	branchID, err := primitive.ObjectIDFromHex(req.BranchID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid branch ID format",
		})
	}

	// Check if branch exists first
	companyCollection := uc.DB.Database("barrim").Collection("companies")
	var company models.Company
	filter := bson.M{"branches._id": branchID}
	err = companyCollection.FindOne(context.Background(), filter).Decode(&company)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Branch not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Error finding branch: " + err.Error(),
		})
	}

	// Check if branch is already in favorites
	usersCollection := uc.DB.Database("barrim").Collection("users")
	filter = bson.M{
		"_id":              user.ID,
		"favoriteBranches": branchID,
	}
	var existingUser models.User
	err = usersCollection.FindOne(context.Background(), filter).Decode(&existingUser)
	if err == nil {
		// Branch already in favorites
		return c.JSON(http.StatusOK, models.Response{
			Status:  http.StatusOK,
			Message: "Branch already in favorites",
		})
	}

	// Add branch to favorites
	update := bson.M{
		"$addToSet": bson.M{"favoriteBranches": branchID},
		"$set":      bson.M{"updatedAt": time.Now()},
	}
	_, err = usersCollection.UpdateOne(context.Background(), bson.M{"_id": user.ID}, update)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Error adding branch to favorites: " + err.Error(),
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Branch added to favorites successfully",
	})
}

// RemoveBranchFromFavorites removes a branch from user's favorites
func (uc *UserController) RemoveBranchFromFavorites(c echo.Context) error {
	// Get the current user from the token
	user, err := utils.GetUserFromToken(c, uc.DB)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Unauthorized: " + err.Error(),
		})
	}

	// Parse the request to get branch ID
	var req struct {
		BranchID string `json:"branchId" validate:"required"`
	}

	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request format",
		})
	}

	// Validate required fields
	if req.BranchID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Branch ID is required",
		})
	}

	// Convert string ID to ObjectID
	branchID, err := primitive.ObjectIDFromHex(req.BranchID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid branch ID format",
		})
	}

	// Remove branch from favorites
	usersCollection := uc.DB.Database("barrim").Collection("users")
	update := bson.M{
		"$pull": bson.M{"favoriteBranches": branchID},
		"$set":  bson.M{"updatedAt": time.Now()},
	}
	result, err := usersCollection.UpdateOne(context.Background(), bson.M{"_id": user.ID}, update)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Error removing branch from favorites: " + err.Error(),
		})
	}

	if result.ModifiedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Branch not found in favorites",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Branch removed from favorites successfully",
	})
}

// GetFavoriteBranches gets all favorite branches for a user
func (uc *UserController) GetFavoriteBranches(c echo.Context) error {
	// Get the current user from the token
	user, err := utils.GetUserFromToken(c, uc.DB)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Unauthorized: " + err.Error(),
		})
	}

	// If user has no favorites, return empty array
	if len(user.FavoriteBranches) == 0 {
		return c.JSON(http.StatusOK, models.Response{
			Status:  http.StatusOK,
			Message: "No favorite branches found",
			Data:    []interface{}{},
		})
	}

	// Find all branches that match the IDs in user.FavoriteBranches
	companyCollection := uc.DB.Database("barrim").Collection("companies")

	pipeline := []bson.M{
		{
			"$match": bson.M{"branches._id": bson.M{"$in": user.FavoriteBranches}},
		},
		{
			"$unwind": "$branches",
		},
		{
			"$match": bson.M{"branches._id": bson.M{"$in": user.FavoriteBranches}},
		},
		{
			"$project": bson.M{
				"_id":                0,
				"branch._id":         "$branches._id",
				"branch.name":        "$branches.name",
				"branch.location":    "$branches.location",
				"branch.phone":       "$branches.phone",
				"branch.category":    "$branches.category",
				"branch.subCategory": "$branches.subCategory",
				"branch.description": "$branches.description",
				"branch.images":      "$branches.images",
				"branch.videos":      "$branches.videos",
				"branch.createdAt":   "$branches.createdAt",
				"branch.updatedAt":   "$branches.updatedAt",
				"companyName":        "$businessName",
				"companyId":          "$_id",
				"logoUrl":            "$logoUrl",
			},
		},
		{
			"$replaceRoot": bson.M{"newRoot": bson.M{
				"branch":      "$branch",
				"companyName": "$companyName",
				"companyId":   "$companyId",
				"logoUrl":     "$logoUrl",
			}},
		},
	}

	cursor, err := companyCollection.Aggregate(context.Background(), pipeline)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Error fetching favorite branches: " + err.Error(),
		})
	}
	defer cursor.Close(context.Background())

	// Create a list to store the results
	var favoriteBranches []map[string]interface{}
	if err = cursor.All(context.Background(), &favoriteBranches); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Error parsing favorite branches: " + err.Error(),
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Favorite branches retrieved successfully",
		Data:    favoriteBranches,
	})
}

// AddServiceProviderToFavorites adds a service provider to user's favorites
func (uc *UserController) AddServiceProviderToFavorites(c echo.Context) error {
	// Get the current user from the token
	user, err := utils.GetUserFromToken(c, uc.DB)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Unauthorized: " + err.Error(),
		})
	}

	// Parse the request to get service provider ID
	var req struct {
		ServiceProviderID string `json:"serviceProviderId" validate:"required"`
	}

	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request format",
		})
	}

	// Validate required fields
	if req.ServiceProviderID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Service Provider ID is required",
		})
	}

	// Convert string ID to ObjectID
	serviceProviderID, err := primitive.ObjectIDFromHex(req.ServiceProviderID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid service provider ID format",
		})
	}

	// Check if service provider exists in serviceProviders collection first (primary location)
	serviceProvidersCollection := uc.DB.Database("barrim").Collection("serviceProviders")
	usersCollection := uc.DB.Database("barrim").Collection("users")
	var serviceProviderDoc models.ServiceProvider
	filter := bson.M{"_id": serviceProviderID}
	err = serviceProvidersCollection.FindOne(context.Background(), filter).Decode(&serviceProviderDoc)

	// If not found in serviceProviders collection, check users collection as fallback
	if err != nil {
		if err == mongo.ErrNoDocuments {
			// Check in users collection as fallback
			var serviceProviderUser models.User
			filter = bson.M{
				"_id":      serviceProviderID,
				"userType": "serviceProvider",
			}
			err = usersCollection.FindOne(context.Background(), filter).Decode(&serviceProviderUser)
			if err != nil {
				if err == mongo.ErrNoDocuments {
					return c.JSON(http.StatusNotFound, models.Response{
						Status:  http.StatusNotFound,
						Message: "Service provider not found in either serviceProviders or users collection",
					})
				}
				return c.JSON(http.StatusInternalServerError, models.Response{
					Status:  http.StatusInternalServerError,
					Message: "Error finding service provider: " + err.Error(),
				})
			}
			// Service provider found in users collection
			// We can proceed with adding to favorites
		} else {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Error finding service provider: " + err.Error(),
			})
		}
	}

	// Create a new field for favorite service providers if it doesn't exist in User model
	// Check if service provider is already in favorites
	filter = bson.M{
		"_id":                      user.ID,
		"favoriteServiceProviders": serviceProviderID,
	}
	var existingUser models.User
	err = usersCollection.FindOne(context.Background(), filter).Decode(&existingUser)
	if err == nil {
		// Service provider already in favorites
		return c.JSON(http.StatusOK, models.Response{
			Status:  http.StatusOK,
			Message: "Service provider already in favorites",
		})
	}

	// Add service provider to favorites
	update := bson.M{
		"$addToSet": bson.M{"favoriteServiceProviders": serviceProviderID},
		"$set":      bson.M{"updatedAt": time.Now()},
	}
	_, err = usersCollection.UpdateOne(context.Background(), bson.M{"_id": user.ID}, update)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Error adding service provider to favorites: " + err.Error(),
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Service provider added to favorites successfully",
	})
}

// RemoveServiceProviderFromFavorites removes a service provider from user's favorites
func (uc *UserController) RemoveServiceProviderFromFavorites(c echo.Context) error {
	// Get the current user from the token
	user, err := utils.GetUserFromToken(c, uc.DB)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Unauthorized: " + err.Error(),
		})
	}

	// Parse the request to get service provider ID
	var req struct {
		ServiceProviderID string `json:"serviceProviderId" validate:"required"`
	}

	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request format",
		})
	}

	// Validate required fields
	if req.ServiceProviderID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Service Provider ID is required",
		})
	}

	// Convert string ID to ObjectID
	serviceProviderID, err := primitive.ObjectIDFromHex(req.ServiceProviderID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid service provider ID format",
		})
	}

	// Remove service provider from favorites
	usersCollection := uc.DB.Database("barrim").Collection("users")
	update := bson.M{
		"$pull": bson.M{"favoriteServiceProviders": serviceProviderID},
		"$set":  bson.M{"updatedAt": time.Now()},
	}
	result, err := usersCollection.UpdateOne(context.Background(), bson.M{"_id": user.ID}, update)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Error removing service provider from favorites: " + err.Error(),
		})
	}

	if result.ModifiedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Service provider not found in favorites",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Service provider removed from favorites successfully",
	})
}

// GetFavoriteServiceProviders gets all favorite service providers for a user
func (uc *UserController) GetFavoriteServiceProviders(c echo.Context) error {
	// Get the current user from the token
	user, err := utils.GetUserFromToken(c, uc.DB)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Unauthorized: " + err.Error(),
		})
	}

	usersCollection := uc.DB.Database("barrim").Collection("users")
	var userData map[string]interface{}
	err = usersCollection.FindOne(context.Background(), bson.M{"_id": user.ID}).Decode(&userData)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Error fetching user data: " + err.Error(),
		})
	}

	favoriteServiceProviders, ok := userData["favoriteServiceProviders"].(primitive.A)
	if !ok || len(favoriteServiceProviders) == 0 {
		return c.JSON(http.StatusOK, models.Response{
			Status:  http.StatusOK,
			Message: "No favorite service providers found",
			Data:    []interface{}{},
		})
	}

	// Convert primitive.A to []primitive.ObjectID
	favoriteIds := make([]primitive.ObjectID, len(favoriteServiceProviders))
	for i, id := range favoriteServiceProviders {
		oid, ok := id.(primitive.ObjectID)
		if !ok {
			oidStr, ok := id.(string)
			if !ok {
				continue
			}
			oid, err = primitive.ObjectIDFromHex(oidStr)
			if err != nil {
				continue
			}
		}
		favoriteIds[i] = oid
	}

	// Find all service providers that match the IDs from both collections
	var allFavoriteProviders []map[string]interface{}

	// First, try to find in serviceProviders collection (primary)
	serviceProvidersCollection := uc.DB.Database("barrim").Collection("serviceProviders")
	serviceProviderPipeline := []bson.M{
		{
			"$match": bson.M{
				"_id": bson.M{"$in": favoriteIds},
			},
		},
		{
			"$project": bson.M{
				"_id":                 1,
				"businessName":        1,
				"email":               1,
				"phone":               1,
				"logo":                1,
				"category":            1,
				"country":             1,
				"district":            1,
				"city":                1,
				"street":              1,
				"postalCode":          1,
				"serviceProviderInfo": 1,
				"contactInfo":         1,
				"createdAt":           1,
				"updatedAt":           1,
				"source":              "serviceProviders",
			},
		},
	}

	serviceProviderCursor, err := serviceProvidersCollection.Aggregate(context.Background(), serviceProviderPipeline)
	if err == nil {
		var serviceProviderDocs []map[string]interface{}
		if err = serviceProviderCursor.All(context.Background(), &serviceProviderDocs); err == nil {
			allFavoriteProviders = append(allFavoriteProviders, serviceProviderDocs...)
		}
		serviceProviderCursor.Close(context.Background())
	}

	// Then, try to find in users collection as fallback
	pipeline := []bson.M{
		{
			"$match": bson.M{
				"_id":      bson.M{"$in": favoriteIds},
				"userType": "serviceProvider",
			},
		},
		{
			"$project": bson.M{
				"_id":                 1,
				"fullName":            1,
				"logoPath":            1,
				"email":               1,
				"phone":               1,
				"serviceProviderInfo": 1,
				"location":            1,
				"createdAt":           1,
				"updatedAt":           1,
				"source":              "users",
			},
		},
	}

	cursor, err := usersCollection.Aggregate(context.Background(), pipeline)
	if err == nil {
		var userProviders []map[string]interface{}
		if err = cursor.All(context.Background(), &userProviders); err == nil {
			allFavoriteProviders = append(allFavoriteProviders, userProviders...)
		}
		cursor.Close(context.Background())
	}
	if err == nil {
		var serviceProviderDocs []map[string]interface{}
		if err = serviceProviderCursor.All(context.Background(), &serviceProviderDocs); err == nil {
			allFavoriteProviders = append(allFavoriteProviders, serviceProviderDocs...)
		}
		serviceProviderCursor.Close(context.Background())
	}

	// Process and format the results
	for i, provider := range allFavoriteProviders {
		// Handle profile photos and logos based on source
		if source, ok := provider["source"].(string); ok {
			if source == "users" {
				// Handle user collection data
				if serviceProviderInfo, exists := provider["serviceProviderInfo"].(map[string]interface{}); exists {
					if profilePhoto, exists := serviceProviderInfo["profilePhoto"].(string); exists && profilePhoto != "" {
						allFavoriteProviders[i]["profilePhoto"] = "/uploads/serviceprovider/" + profilePhoto
					}
				}
				if logoPath, exists := provider["logoPath"].(string); exists && logoPath != "" {
					allFavoriteProviders[i]["logoPath"] = "/" + logoPath
				}
			} else if source == "serviceProviders" {
				// Handle serviceProviders collection data
				if logo, exists := provider["logo"].(string); exists && logo != "" {
					allFavoriteProviders[i]["logo"] = "/" + logo
				}
				// Handle ServiceProviderInfo from serviceProviders collection
				if serviceProviderInfo, exists := provider["serviceProviderInfo"].(map[string]interface{}); exists {
					if profilePhoto, exists := serviceProviderInfo["profilePhoto"].(string); exists && profilePhoto != "" {
						allFavoriteProviders[i]["profilePhoto"] = "/uploads/serviceprovider/" + profilePhoto
					}
				}
			}
		}
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Favorite service providers retrieved successfully",
		Data:    allFavoriteProviders,
	})
}

// UpdateFCMToken updates the user's FCM token
func (c *UserController) UpdateFCMToken(ctx echo.Context) error {
	// Get user from token
	user, err := utils.GetUserFromToken(ctx, c.DB)
	if err != nil {
		return ctx.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Unauthorized",
		})
	}

	// Parse request body
	var request struct {
		FCMToken string `json:"fcmToken"`
	}
	if err := ctx.Bind(&request); err != nil {
		return ctx.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request",
		})
	}

	// Update FCM token in database
	collection := c.DB.Database("barrim").Collection("users")
	_, err = collection.UpdateOne(
		context.Background(),
		bson.M{"_id": user.ID},
		bson.M{"$set": bson.M{"fcmToken": request.FCMToken}},
	)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update FCM token",
		})
	}

	return ctx.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "FCM token updated successfully",
	})
}

// GetNotifications retrieves notifications for the current user
func (uc *UserController) GetNotifications(ctx echo.Context) error {
	// Get user from token
	user, err := utils.GetUserFromToken(ctx, uc.DB)
	if err != nil {
		return ctx.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Unauthorized",
		})
	}

	// Retrieve notifications from the database
	collection := uc.DB.Database("barrim").Collection("notifications")
	cursor, err := collection.Find(context.Background(), bson.M{"userId": user.ID})
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Error retrieving notifications",
		})
	}
	defer cursor.Close(context.Background())

	// Decode notifications
	var notifications []models.Notification
	if err := cursor.All(context.Background(), &notifications); err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Error decoding notifications",
		})
	}

	return ctx.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Notifications retrieved successfully",
		Data:    notifications,
	})
}

func (uc *UserController) UploadProfilePicture(c echo.Context) error {
	// Get user ID from context
	userID := c.Get("user_id").(string)

	// Get file from request
	file, err := c.FormFile("profile_picture")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"message": "No file uploaded",
		})
	}

	// Validate file size (max 5MB)
	if file.Size > 5*1024*1024 {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"message": "File size exceeds 5MB limit",
		})
	}

	// Validate file type
	src, err := file.Open()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Error reading file",
		})
	}
	defer src.Close()

	// Read first 512 bytes for mime type detection
	buffer := make([]byte, 512)
	_, err = src.Read(buffer)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Error reading file",
		})
	}

	// Detect mime type
	mimeType := http.DetectContentType(buffer)
	allowedMimeTypes := map[string]bool{
		"image/jpeg": true,
		"image/png":  true,
		"image/gif":  true,
	}

	if !allowedMimeTypes[mimeType] {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"message": "Invalid file type. Only JPEG, PNG and GIF images are allowed",
		})
	}

	// Generate secure filename
	ext := filepath.Ext(file.Filename)
	secureFilename := fmt.Sprintf("%s_%d%s", userID, time.Now().UnixNano(), ext)

	// Create uploads directory if it doesn't exist
	uploadDir := "uploads/profile_pictures"
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Error creating upload directory",
		})
	}

	// Create destination file
	dst, err := os.Create(filepath.Join(uploadDir, secureFilename))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Error creating file",
		})
	}
	defer dst.Close()

	// Reset file reader to beginning
	src.Seek(0, 0)

	// Copy file content
	if _, err = io.Copy(dst, src); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Error saving file",
		})
	}

	// Update user profile picture URL in database
	profileURL := fmt.Sprintf("/uploads/profile_pictures/%s", secureFilename)
	if err := uc.userRepo.UpdateProfilePicture(userID, profileURL); err != nil {
		// Clean up uploaded file if database update fails
		os.Remove(filepath.Join(uploadDir, secureFilename))
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Error updating profile picture",
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Profile picture uploaded successfully",
		"url":     profileURL,
	})
}

func (uc *UserController) FilterCompaniesAndWholesalers(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Parse query params
	category := c.QueryParam("category")
	latStr := c.QueryParam("lat")
	lngStr := c.QueryParam("lng")
	distanceStr := c.QueryParam("distance") // in meters

	if category == "" || latStr == "" || lngStr == "" || distanceStr == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "category, lat, lng, and distance are required",
		})
	}

	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid latitude",
		})
	}
	lng, err := strconv.ParseFloat(lngStr, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid longitude",
		})
	}
	distance, err := strconv.ParseFloat(distanceStr, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid distance",
		})
	}

	collection := config.GetCollection(uc.DB, "users")

	// Build filter
	filter := bson.M{
		"userType": bson.M{"$in": []string{"company", "wholesaler"}},
		"location": bson.M{
			"$near": bson.M{
				"$geometry": bson.M{
					"type":        "Point",
					"coordinates": []float64{lng, lat},
				},
				"$maxDistance": distance,
			},
		},
		"category": category, // or use "$in": []string{category} if it's an array
	}

	// Exclude sensitive fields
	opts := options.Find().SetProjection(bson.M{
		"password":           0,
		"resetPasswordToken": 0,
		"otpInfo":            0,
	})

	cursor, err := collection.Find(ctx, filter, opts)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch results",
		})
	}
	defer cursor.Close(ctx)

	var results []models.User
	if err := cursor.All(ctx, &results); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode results",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Filtered results retrieved successfully",
		Data:    results,
	})
}
