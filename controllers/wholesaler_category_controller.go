package controllers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/HSouheill/barrim_backend/models"
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type WholesalerCategoryController struct {
	DB *mongo.Database
}

func NewWholesalerCategoryController(db *mongo.Database) *WholesalerCategoryController {
	return &WholesalerCategoryController{DB: db}
}

// CreateWholesalerCategory creates a new wholesaler category with subcategories
func (wcc *WholesalerCategoryController) CreateWholesalerCategory(c echo.Context) error {
	// Get the category name from form data
	categoryName := c.FormValue("name")

	if categoryName == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Wholesaler category name is required",
		})
	}

	// Get subcategories from form data
	subcategories := c.FormValue("subcategories")
	var subcategoriesList []string

	// Parse subcategories if provided (comma-separated string)
	if subcategories != "" {
		// Split by comma and trim whitespace
		subcategoriesList = strings.Split(subcategories, ",")
		for i, sub := range subcategoriesList {
			subcategoriesList[i] = strings.TrimSpace(sub)
		}
		// Remove empty strings
		var filteredSubcategories []string
		for _, sub := range subcategoriesList {
			if sub != "" {
				filteredSubcategories = append(filteredSubcategories, sub)
			}
		}
		subcategoriesList = filteredSubcategories
	}

	// Check if category name already exists
	var existingCategory models.WholesalerCategory
	err := wcc.DB.Collection("wholesaler_categories").FindOne(context.Background(), bson.M{"name": categoryName}).Decode(&existingCategory)
	if err == nil {
		return c.JSON(http.StatusConflict, models.Response{
			Status:  http.StatusConflict,
			Message: "Wholesaler category with this name already exists",
		})
	}

	// Create the category document
	category := models.WholesalerCategory{
		Name:          categoryName,
		Subcategories: subcategoriesList,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	// Insert the category
	result, err := wcc.DB.Collection("wholesaler_categories").InsertOne(context.Background(), category)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to create wholesaler category",
		})
	}

	category.ID = result.InsertedID.(primitive.ObjectID)

	// Return success
	message := "Wholesaler category created successfully"
	if len(subcategoriesList) > 0 {
		message += " with subcategories"
	}

	return c.JSON(http.StatusCreated, models.Response{
		Status:  http.StatusCreated,
		Message: message,
		Data:    category,
	})
}

// GetAllWholesalerCategories retrieves all wholesaler categories
func (wcc *WholesalerCategoryController) GetAllWholesalerCategories(c echo.Context) error {
	cursor, err := wcc.DB.Collection("wholesaler_categories").Find(context.Background(), bson.M{})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve wholesaler categories",
		})
	}
	defer cursor.Close(context.Background())

	var categories []models.WholesalerCategory
	if err = cursor.All(context.Background(), &categories); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode wholesaler categories",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Wholesaler categories retrieved successfully",
		Data:    categories,
	})
}

// GetWholesalerCategory retrieves a wholesaler category by ID
func (wcc *WholesalerCategoryController) GetWholesalerCategory(c echo.Context) error {
	id := c.Param("id")

	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid wholesaler category ID",
		})
	}

	var category models.WholesalerCategory
	err = wcc.DB.Collection("wholesaler_categories").FindOne(context.Background(), bson.M{"_id": objectID}).Decode(&category)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Wholesaler category not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve wholesaler category",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Wholesaler category retrieved successfully",
		Data:    category,
	})
}

// UpdateWholesalerCategory updates a wholesaler category by ID
func (wcc *WholesalerCategoryController) UpdateWholesalerCategory(c echo.Context) error {
	id := c.Param("id")

	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid wholesaler category ID",
		})
	}

	updateData := make(map[string]interface{})
	if err := c.Bind(&updateData); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}

	// Remove ID and timestamps from update data
	delete(updateData, "_id")
	delete(updateData, "id")
	delete(updateData, "createdAt")

	// Add updated timestamp
	updateData["updatedAt"] = time.Now()

	// Check if category name already exists (if name is being updated)
	if name, exists := updateData["name"]; exists {
		var existingCategory models.WholesalerCategory
		err := wcc.DB.Collection("wholesaler_categories").FindOne(
			context.Background(),
			bson.M{"name": name, "_id": bson.M{"$ne": objectID}},
		).Decode(&existingCategory)
		if err == nil {
			return c.JSON(http.StatusConflict, models.Response{
				Status:  http.StatusConflict,
				Message: "Wholesaler category with this name already exists",
			})
		}
	}

	// Update the category
	result, err := wcc.DB.Collection("wholesaler_categories").UpdateOne(
		context.Background(),
		bson.M{"_id": objectID},
		bson.M{"$set": updateData},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update wholesaler category",
		})
	}

	if result.MatchedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Wholesaler category not found",
		})
	}

	// Retrieve the updated category
	var updatedCategory models.WholesalerCategory
	err = wcc.DB.Collection("wholesaler_categories").FindOne(context.Background(), bson.M{"_id": objectID}).Decode(&updatedCategory)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Wholesaler category updated but failed to retrieve updated data",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Wholesaler category updated successfully",
		Data:    updatedCategory,
	})
}

// DeleteWholesalerCategory deletes a wholesaler category by ID
func (wcc *WholesalerCategoryController) DeleteWholesalerCategory(c echo.Context) error {
	id := c.Param("id")

	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid wholesaler category ID",
		})
	}

	// Check if category is being used by any wholesalers
	count, err := wcc.DB.Collection("wholesalers").CountDocuments(context.Background(), bson.M{"category": objectID})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to check wholesaler category usage",
		})
	}

	if count > 0 {
		return c.JSON(http.StatusConflict, models.Response{
			Status:  http.StatusConflict,
			Message: "Cannot delete wholesaler category that is being used by wholesalers",
		})
	}

	// Delete the category
	result, err := wcc.DB.Collection("wholesaler_categories").DeleteOne(context.Background(), bson.M{"_id": objectID})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to delete wholesaler category",
		})
	}

	if result.DeletedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Wholesaler category not found",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Wholesaler category deleted successfully",
	})
}
