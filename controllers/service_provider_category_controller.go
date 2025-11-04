package controllers

import (
	"context"
	"net/http"
	"path/filepath"
	"time"

	"io"

	"github.com/HSouheill/barrim_backend/models"
	"github.com/HSouheill/barrim_backend/utils"
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type ServiceProviderCategoryController struct {
	DB *mongo.Database
}

func NewServiceProviderCategoryController(db *mongo.Database) *ServiceProviderCategoryController {
	return &ServiceProviderCategoryController{DB: db}
}

// CreateServiceProviderCategory creates a new service provider category
func (spcc *ServiceProviderCategoryController) CreateServiceProviderCategory(c echo.Context) error {
	// Parse multipart form
	if err := c.Request().ParseMultipartForm(32 << 20); err != nil { // 32MB max
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Failed to parse form data",
		})
	}

	// Get form values
	name := c.FormValue("name")

	// Validate required fields
	if name == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Category name is required",
		})
	}

	// Create the category document
	categoryDoc := bson.M{
		"name":      name,
		"createdAt": time.Now(),
		"updatedAt": time.Now(),
	}

	// Check if category name already exists
	var existingCategory bson.M
	err := spcc.DB.Collection("serviceProviderCategories").FindOne(context.Background(), bson.M{"name": name}).Decode(&existingCategory)
	if err == nil {
		return c.JSON(http.StatusConflict, models.Response{
			Status:  http.StatusConflict,
			Message: "Service provider category with this name already exists",
		})
	}

	// Insert the category
	result, err := spcc.DB.Collection("serviceProviderCategories").InsertOne(context.Background(), categoryDoc)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to create service provider category",
		})
	}

	// Add the ID to the response
	categoryDoc["_id"] = result.InsertedID

	// Handle image upload if provided
	file, err := c.FormFile("image")
	if err == nil && file != nil {
		// Image was provided, process it

		// Validate file type
		if err := utils.ValidateFileType(file.Filename, "image"); err != nil {
			// If image validation fails, still return success but with warning
			return c.JSON(http.StatusCreated, models.Response{
				Status:  http.StatusCreated,
				Message: "Service provider category created successfully, but image upload failed: " + err.Error(),
				Data:    categoryDoc,
			})
		}

		// Read file data
		src, err := file.Open()
		if err != nil {
			return c.JSON(http.StatusCreated, models.Response{
				Status:  http.StatusCreated,
				Message: "Service provider category created successfully, but image upload failed: " + err.Error(),
				Data:    categoryDoc,
			})
		}
		defer src.Close()

		// Generate unique filename
		filename := "sp_category_" + result.InsertedID.(primitive.ObjectID).Hex() + "_" + time.Now().Format("20060102150405") + filepath.Ext(file.Filename)

		// Upload file using the utility with custom path
		fileData, err := io.ReadAll(src)
		if err != nil {
			return c.JSON(http.StatusCreated, models.Response{
				Status:  http.StatusCreated,
				Message: "Service provider category created successfully, but image upload failed: " + err.Error(),
				Data:    categoryDoc,
			})
		}

		fileURL, err := utils.UploadFileToPath(fileData, filename, "image", "category")
		if err != nil {
			return c.JSON(http.StatusCreated, models.Response{
				Status:  http.StatusCreated,
				Message: "Service provider category created successfully, but image upload failed: " + err.Error(),
				Data:    categoryDoc,
			})
		}

		// Update category with image URL
		_, err = spcc.DB.Collection("serviceProviderCategories").UpdateOne(
			context.Background(),
			bson.M{"_id": result.InsertedID},
			bson.M{"$set": bson.M{
				"logo":      fileURL,
				"updatedAt": time.Now(),
			}},
		)
		if err != nil {
			return c.JSON(http.StatusCreated, models.Response{
				Status:  http.StatusCreated,
				Message: "Service provider category created successfully, but image update failed: " + err.Error(),
				Data:    categoryDoc,
			})
		}

		// Update the category document for response
		categoryDoc["logo"] = fileURL
		categoryDoc["updatedAt"] = time.Now()

		return c.JSON(http.StatusCreated, models.Response{
			Status:  http.StatusCreated,
			Message: "Service provider category created successfully with image",
			Data:    categoryDoc,
		})
	}

	// No image provided, return success
	return c.JSON(http.StatusCreated, models.Response{
		Status:  http.StatusCreated,
		Message: "Service provider category created successfully",
		Data:    categoryDoc,
	})
}

// GetAllServiceProviderCategories retrieves all service provider categories
func (spcc *ServiceProviderCategoryController) GetAllServiceProviderCategories(c echo.Context) error {
	cursor, err := spcc.DB.Collection("serviceProviderCategories").Find(context.Background(), bson.M{})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve service provider categories",
		})
	}
	defer cursor.Close(context.Background())

	var categories []bson.M
	if err = cursor.All(context.Background(), &categories); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode service provider categories",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Service provider categories retrieved successfully",
		Data:    categories,
	})
}

// GetServiceProviderCategory retrieves a service provider category by ID
func (spcc *ServiceProviderCategoryController) GetServiceProviderCategory(c echo.Context) error {
	id := c.Param("id")

	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid category ID",
		})
	}

	var category bson.M
	err = spcc.DB.Collection("serviceProviderCategories").FindOne(context.Background(), bson.M{"_id": objectID}).Decode(&category)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Service provider category not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve service provider category",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Service provider category retrieved successfully",
		Data:    category,
	})
}

// UpdateServiceProviderCategory updates a service provider category by ID
func (spcc *ServiceProviderCategoryController) UpdateServiceProviderCategory(c echo.Context) error {
	id := c.Param("id")

	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid category ID",
		})
	}

	// Parse multipart form
	if err := c.Request().ParseMultipartForm(32 << 20); err != nil { // 32MB max
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Failed to parse form data",
		})
	}

	// Get form values
	name := c.FormValue("name")

	// Prepare update data
	updateData := make(map[string]interface{})
	if name != "" {
		updateData["name"] = name
	}

	// Handle image upload
	file, err := c.FormFile("image")
	if err == nil && file != nil {
		// Image was provided, process it

		// Validate file type
		if err := utils.ValidateFileType(file.Filename, "image"); err != nil {
			return c.JSON(http.StatusBadRequest, models.Response{
				Status:  http.StatusBadRequest,
				Message: "Invalid image format: " + err.Error(),
			})
		}

		// Read file data
		src, err := file.Open()
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to read uploaded file",
			})
		}
		defer src.Close()

		// Generate unique filename
		filename := "sp_category_" + objectID.Hex() + "_" + time.Now().Format("20060102150405") + filepath.Ext(file.Filename)

		// Upload file using the utility with custom path
		fileData, err := io.ReadAll(src)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to read uploaded file",
			})
		}

		fileURL, err := utils.UploadFileToPath(fileData, filename, "image", "category")
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to upload image: " + err.Error(),
			})
		}

		updateData["logo"] = fileURL
	}

	// Remove ID and timestamps from update data
	delete(updateData, "_id")
	delete(updateData, "id")
	delete(updateData, "createdAt")

	// Add updated timestamp
	updateData["updatedAt"] = time.Now()

	// Check if category name already exists (if name is being updated)
	if name != "" {
		var existingCategory bson.M
		err := spcc.DB.Collection("serviceProviderCategories").FindOne(
			context.Background(),
			bson.M{"name": name, "_id": bson.M{"$ne": objectID}},
		).Decode(&existingCategory)
		if err == nil {
			return c.JSON(http.StatusConflict, models.Response{
				Status:  http.StatusConflict,
				Message: "Service provider category with this name already exists",
			})
		}
	}

	// Update the category
	result, err := spcc.DB.Collection("serviceProviderCategories").UpdateOne(
		context.Background(),
		bson.M{"_id": objectID},
		bson.M{"$set": updateData},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update service provider category",
		})
	}

	if result.MatchedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Service provider category not found",
		})
	}

	// Retrieve the updated category
	var updatedCategory bson.M
	err = spcc.DB.Collection("serviceProviderCategories").FindOne(context.Background(), bson.M{"_id": objectID}).Decode(&updatedCategory)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Service provider category updated but failed to retrieve updated data",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Service provider category updated successfully",
		Data:    updatedCategory,
	})
}

// DeleteServiceProviderCategory deletes a service provider category by ID
func (spcc *ServiceProviderCategoryController) DeleteServiceProviderCategory(c echo.Context) error {
	id := c.Param("id")

	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid category ID",
		})
	}

	// Check if category is being used by any service providers
	count, err := spcc.DB.Collection("serviceProviders").CountDocuments(context.Background(), bson.M{"category": objectID.Hex()})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to check category usage",
		})
	}

	if count > 0 {
		return c.JSON(http.StatusConflict, models.Response{
			Status:  http.StatusConflict,
			Message: "Cannot delete category: it is being used by service providers",
		})
	}

	// Delete the category
	result, err := spcc.DB.Collection("serviceProviderCategories").DeleteOne(context.Background(), bson.M{"_id": objectID})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to delete service provider category",
		})
	}

	if result.DeletedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Service provider category not found",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Service provider category deleted successfully",
	})
}
