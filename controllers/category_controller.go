package controllers

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/HSouheill/barrim_backend/models"
	"github.com/HSouheill/barrim_backend/utils"
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type CategoryController struct {
	DB *mongo.Database
}

func NewCategoryController(db *mongo.Database) *CategoryController {
	return &CategoryController{DB: db}
}

// CreateCategory creates a new category with optional logo upload and subcategories
func (cc *CategoryController) CreateCategory(c echo.Context) error {
	// Get the category name from form data
	categoryName := c.FormValue("name")

	if categoryName == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Category name is required",
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

	// Get color from form data
	color := c.FormValue("color")

	// Validate color format if provided
	if color != "" {
		// Basic validation: check if it's a valid hex color or CSS color name
		// This is a simple validation - you might want to add more sophisticated validation
		if !isValidColor(color) {
			return c.JSON(http.StatusBadRequest, models.Response{
				Status:  http.StatusBadRequest,
				Message: "Invalid color format. Please provide a valid hex color (e.g., #FF0000) or CSS color name",
			})
		}
	}

	// Check if category name already exists
	var existingCategory models.Category
	err := cc.DB.Collection("categories").FindOne(context.Background(), bson.M{"name": categoryName}).Decode(&existingCategory)
	if err == nil {
		return c.JSON(http.StatusConflict, models.Response{
			Status:  http.StatusConflict,
			Message: "Category with this name already exists",
		})
	}

	// Create the category document
	category := models.Category{
		Name:          categoryName,
		Color:         color,
		Subcategories: subcategoriesList,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	// Insert the category first
	result, err := cc.DB.Collection("categories").InsertOne(context.Background(), category)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to create category",
		})
	}

	category.ID = result.InsertedID.(primitive.ObjectID)

	// Handle logo upload if provided
	file, err := c.FormFile("logo")
	if err == nil && file != nil {
		// Logo was provided, process it

		// Validate file type
		if err := utils.ValidateFileType(file.Filename, "image"); err != nil {
			// If logo validation fails, still return success but with warning
			return c.JSON(http.StatusCreated, models.Response{
				Status:  http.StatusCreated,
				Message: "Category created successfully, but logo upload failed: " + err.Error(),
				Data:    category,
			})
		}

		// Read file data
		src, err := file.Open()
		if err != nil {
			return c.JSON(http.StatusCreated, models.Response{
				Status:  http.StatusCreated,
				Message: "Category created successfully, but logo upload failed: " + err.Error(),
				Data:    category,
			})
		}
		defer src.Close()

		// Generate unique filename
		filename := "category_" + category.ID.Hex() + "_" + time.Now().Format("20060102150405") + filepath.Ext(file.Filename)

		// Create category-specific directory path
		// Use the existing uploads directory structure that's already mounted
		categoryDir := "category"

		// Upload file using the utility with custom path
		fileData, err := io.ReadAll(src)
		if err != nil {
			return c.JSON(http.StatusCreated, models.Response{
				Status:  http.StatusCreated,
				Message: "Category created successfully, but logo upload failed: " + err.Error(),
				Data:    category,
			})
		}

		fileURL, err := utils.UploadFileToPath(fileData, filename, "image", categoryDir)
		if err != nil {
			return c.JSON(http.StatusCreated, models.Response{
				Status:  http.StatusCreated,
				Message: "Category created successfully, but logo upload failed: " + err.Error(),
				Data:    category,
			})
		}

		// Update category with logo URL
		_, err = cc.DB.Collection("categories").UpdateOne(
			context.Background(),
			bson.M{"_id": category.ID},
			bson.M{"$set": bson.M{
				"logo":      fileURL,
				"updatedAt": time.Now(),
			}},
		)
		if err != nil {
			return c.JSON(http.StatusCreated, models.Response{
				Status:  http.StatusCreated,
				Message: "Category created successfully, but logo update failed: " + err.Error(),
				Data:    category,
			})
		}

		// Update the category object for response
		category.Logo = fileURL
		category.UpdatedAt = time.Now()

		message := "Category created successfully with logo"
		if len(subcategoriesList) > 0 {
			message += " and subcategories"
		}
		if color != "" {
			message += " and color"
		}

		return c.JSON(http.StatusCreated, models.Response{
			Status:  http.StatusCreated,
			Message: message,
			Data:    category,
		})
	}

	// No logo provided, return success
	message := "Category created successfully"
	if len(subcategoriesList) > 0 {
		message += " with subcategories"
	}
	if color != "" {
		message += " and color"
	}

	return c.JSON(http.StatusCreated, models.Response{
		Status:  http.StatusCreated,
		Message: message,
		Data:    category,
	})
}

// GetAllCategories retrieves all categories
func (cc *CategoryController) GetAllCategories(c echo.Context) error {
	cursor, err := cc.DB.Collection("categories").Find(context.Background(), bson.M{})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve categories",
		})
	}
	defer cursor.Close(context.Background())

	var categories []models.Category
	if err = cursor.All(context.Background(), &categories); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode categories",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Categories retrieved successfully",
		Data:    categories,
	})
}

// GetCategory retrieves a category by ID
func (cc *CategoryController) GetCategory(c echo.Context) error {
	id := c.Param("id")

	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid category ID",
		})
	}

	var category models.Category
	err = cc.DB.Collection("categories").FindOne(context.Background(), bson.M{"_id": objectID}).Decode(&category)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Category not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve category",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Category retrieved successfully",
		Data:    category,
	})
}

// UpdateCategory updates a category by ID
func (cc *CategoryController) UpdateCategory(c echo.Context) error {
	id := c.Param("id")

	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid category ID",
		})
	}

	// If multipart/form-data, handle fields and optional logo upload
	contentType := c.Request().Header.Get("Content-Type")
	if strings.HasPrefix(strings.ToLower(contentType), "multipart/form-data") {
		updateData := bson.M{}

		// Optional name with duplicate check
		if name := strings.TrimSpace(c.FormValue("name")); name != "" {
			var existingCategory models.Category
			err := cc.DB.Collection("categories").FindOne(
				context.Background(),
				bson.M{"name": name, "_id": bson.M{"$ne": objectID}},
			).Decode(&existingCategory)
			if err == nil {
				return c.JSON(http.StatusConflict, models.Response{
					Status:  http.StatusConflict,
					Message: "Category with this name already exists",
				})
			}
			updateData["name"] = name
		}

		// Optional color with validation
		if color := strings.TrimSpace(c.FormValue("color")); color != "" {
			if !isValidColor(color) {
				return c.JSON(http.StatusBadRequest, models.Response{
					Status:  http.StatusBadRequest,
					Message: "Invalid color format. Please provide a valid hex color (e.g., #FF0000) or CSS color name",
				})
			}
			updateData["color"] = color
		}

		// Optional subcategories (comma-separated)
		if subcategories := c.FormValue("subcategories"); strings.TrimSpace(subcategories) != "" {
			parts := strings.Split(subcategories, ",")
			var normalized []string
			for _, p := range parts {
				v := strings.TrimSpace(p)
				if v != "" {
					normalized = append(normalized, v)
				}
			}
			updateData["subcategories"] = normalized
		}

		// Optional logo file
		if file, err := c.FormFile("logo"); err == nil && file != nil {
			if err := utils.ValidateFileType(file.Filename, "image"); err != nil {
				return c.JSON(http.StatusBadRequest, models.Response{
					Status:  http.StatusBadRequest,
					Message: "Invalid logo file type",
				})
			}

			src, err := file.Open()
			if err != nil {
				return c.JSON(http.StatusInternalServerError, models.Response{
					Status:  http.StatusInternalServerError,
					Message: "Failed to read logo file",
				})
			}
			defer src.Close()

			fileData, err := io.ReadAll(src)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, models.Response{
					Status:  http.StatusInternalServerError,
					Message: "Failed to read logo data",
				})
			}

			filename := "category_" + id + "_" + time.Now().Format("20060102150405") + filepath.Ext(file.Filename)
			fileURL, err := utils.UploadFileToPath(fileData, filename, "image", "category")
			if err != nil {
				return c.JSON(http.StatusInternalServerError, models.Response{
					Status:  http.StatusInternalServerError,
					Message: "Failed to upload logo",
				})
			}
			updateData["logo"] = fileURL
		}

		if len(updateData) == 0 {
			return c.JSON(http.StatusBadRequest, models.Response{
				Status:  http.StatusBadRequest,
				Message: "No fields to update",
			})
		}

		updateData["updatedAt"] = time.Now()

		result, err := cc.DB.Collection("categories").UpdateOne(
			context.Background(),
			bson.M{"_id": objectID},
			bson.M{"$set": updateData},
		)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to update category",
			})
		}
		if result.MatchedCount == 0 {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Category not found",
			})
		}

		var updatedCategory models.Category
		err = cc.DB.Collection("categories").FindOne(context.Background(), bson.M{"_id": objectID}).Decode(&updatedCategory)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Category updated but failed to retrieve updated data",
			})
		}

		return c.JSON(http.StatusOK, models.Response{
			Status:  http.StatusOK,
			Message: "Category updated successfully",
			Data:    updatedCategory,
		})
	}

	// Fallback to existing JSON update
	updateData := make(map[string]interface{})
	if err := c.Bind(&updateData); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}

	delete(updateData, "_id")
	delete(updateData, "id")
	delete(updateData, "createdAt")
	updateData["updatedAt"] = time.Now()

	if name, exists := updateData["name"]; exists {
		var existingCategory models.Category
		err := cc.DB.Collection("categories").FindOne(
			context.Background(),
			bson.M{"name": name, "_id": bson.M{"$ne": objectID}},
		).Decode(&existingCategory)
		if err == nil {
			return c.JSON(http.StatusConflict, models.Response{
				Status:  http.StatusConflict,
				Message: "Category with this name already exists",
			})
		}
	}

	result, err := cc.DB.Collection("categories").UpdateOne(
		context.Background(),
		bson.M{"_id": objectID},
		bson.M{"$set": updateData},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update category",
		})
	}

	if result.MatchedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Category not found",
		})
	}

	var updatedCategory models.Category
	err = cc.DB.Collection("categories").FindOne(context.Background(), bson.M{"_id": objectID}).Decode(&updatedCategory)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Category updated but failed to retrieve updated data",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Category updated successfully",
		Data:    updatedCategory,
	})
}

// DeleteCategory deletes a category by ID
func (cc *CategoryController) DeleteCategory(c echo.Context) error {
	id := c.Param("id")

	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid category ID",
		})
	}

	// Check if category is being used by any companies
	count, err := cc.DB.Collection("companies").CountDocuments(context.Background(), bson.M{"categoryId": objectID})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to check category usage",
		})
	}

	if count > 0 {
		return c.JSON(http.StatusConflict, models.Response{
			Status:  http.StatusConflict,
			Message: "Cannot delete category: it is being used by companies",
		})
	}

	// Delete the category
	result, err := cc.DB.Collection("categories").DeleteOne(context.Background(), bson.M{"_id": objectID})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to delete category",
		})
	}

	if result.DeletedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Category not found",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Category deleted successfully",
	})
}

// UploadCategoryLogo handles logo upload for categories
func (cc *CategoryController) UploadCategoryLogo(c echo.Context) error {
	// Get the category ID from the URL parameter
	categoryID := c.Param("id")

	objectID, err := primitive.ObjectIDFromHex(categoryID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid category ID",
		})
	}

	// Check if category exists
	var category models.Category
	err = cc.DB.Collection("categories").FindOne(context.Background(), bson.M{"_id": objectID}).Decode(&category)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Category not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve category",
		})
	}

	// Get the uploaded file
	file, err := c.FormFile("logo")
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Logo file is required",
		})
	}

	// Validate file type
	if err := utils.ValidateFileType(file.Filename, "image"); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid file type",
		})
	}

	// Read file data
	src, err := file.Open()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to read file",
		})
	}
	defer src.Close()

	// Generate unique filename
	filename := "category_" + categoryID + "_" + time.Now().Format("20060102150405") + filepath.Ext(file.Filename)

	// Create category-specific directory path
	categoryDir := "category"

	// Upload file using the existing utility with custom path
	fileData, err := io.ReadAll(src)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to read file data",
		})
	}

	fileURL, err := utils.UploadFileToPath(fileData, filename, "image", categoryDir)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to upload file",
		})
	}

	// Update category with new logo URL
	_, err = cc.DB.Collection("categories").UpdateOne(
		context.Background(),
		bson.M{"_id": objectID},
		bson.M{"$set": bson.M{
			"logo":      fileURL,
			"updatedAt": time.Now(),
		}},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update category logo",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Category logo uploaded successfully",
		Data:    map[string]string{"logo": fileURL},
	})
}

// isValidColor checks if the provided color string is valid
func isValidColor(color string) bool {
	// Remove any leading/trailing whitespace
	color = strings.TrimSpace(color)

	// Check if it's a hex color
	if strings.HasPrefix(color, "#") {
		// Hex colors should be 3, 4, 6, or 8 characters long (including #)
		hexLen := len(color) - 1
		return hexLen == 3 || hexLen == 4 || hexLen == 6 || hexLen == 8
	}

	// Check if it's a valid CSS color name (basic check)
	validCSSColors := map[string]bool{
		// Basic colors
		"red": true, "green": true, "blue": true, "yellow": true, "black": true, "white": true,
		"gray": true, "grey": true, "orange": true, "purple": true, "pink": true, "brown": true,
		"cyan": true, "magenta": true, "lime": true, "navy": true, "teal": true, "silver": true,
		"gold": true, "indigo": true, "violet": true, "coral": true, "salmon": true, "turquoise": true,
		"transparent": true, "currentcolor": true,

		// Extended color palette
		"aliceblue": true, "antiquewhite": true, "aqua": true, "aquamarine": true, "azure": true,
		"beige": true, "bisque": true, "blanchedalmond": true, "blueviolet": true, "burlywood": true,
		"cadetblue": true, "chartreuse": true, "chocolate": true, "cornflowerblue": true, "cornsilk": true,
		"crimson": true, "darkblue": true, "darkcyan": true, "darkgoldenrod": true, "darkgray": true,
		"darkgreen": true, "darkgrey": true, "darkkhaki": true, "darkmagenta": true, "darkolivegreen": true,
		"darkorange": true, "darkorchid": true, "darkred": true, "darksalmon": true, "darkseagreen": true,
		"darkslateblue": true, "darkslategray": true, "darkslategrey": true, "darkturquoise": true, "darkviolet": true,
		"deeppink": true, "deepskyblue": true, "dimgray": true, "dimgrey": true, "dodgerblue": true,
		"firebrick": true, "floralwhite": true, "forestgreen": true, "fuchsia": true, "gainsboro": true,
		"ghostwhite": true, "goldenrod": true, "greenyellow": true, "honeydew": true, "hotpink": true,
		"indianred": true, "ivory": true, "khaki": true, "lavender": true, "lavenderblush": true,
		"lawngreen": true, "lemonchiffon": true, "lightblue": true, "lightcoral": true, "lightcyan": true,
		"lightgoldenrodyellow": true, "lightgray": true, "lightgreen": true, "lightgrey": true, "lightpink": true,
		"lightsalmon": true, "lightseagreen": true, "lightskyblue": true, "lightslategray": true, "lightslategrey": true,
		"lightsteelblue": true, "lightyellow": true, "limegreen": true, "linen": true, "mediumaquamarine": true,
		"mediumblue": true, "mediumorchid": true, "mediumpurple": true, "mediumseagreen": true, "mediumslateblue": true,
		"mediumspringgreen": true, "mediumturquoise": true, "mediumvioletred": true, "midnightblue": true, "mintcream": true,
		"mistyrose": true, "moccasin": true, "navajowhite": true, "oldlace": true, "olive": true,
		"olivedrab": true, "orangered": true, "orchid": true, "palegoldenrod": true, "palegreen": true,
		"paleturquoise": true, "palevioletred": true, "papayawhip": true, "peachpuff": true, "peru": true,
		"plum": true, "powderblue": true, "rosybrown": true, "royalblue": true, "saddlebrown": true,
		"sandybrown": true, "seagreen": true, "seashell": true, "sienna": true, "skyblue": true,
		"slateblue": true, "slategray": true, "slategrey": true, "snow": true, "springgreen": true,
		"steelblue": true, "tan": true, "thistle": true, "tomato": true, "wheat": true,
		"whitesmoke": true, "yellowgreen": true,

		// Additional modern colors
		"rebeccapurple": true,

		// Material Design inspired colors
		"materialred": true, "materialpink": true, "materialpurple": true, "materialdeeppurple": true,
		"materialindigo": true, "materialblue": true, "materiallightblue": true, "materialcyan": true,
		"materialteal": true, "materialgreen": true, "materiallightgreen": true, "materiallime": true,
		"materialyellow": true, "materialamber": true, "materialorange": true, "materialdeeporange": true,
		"materialbrown": true, "materialgray": true, "materialbluegray": true,

		// Web-safe colors
		"maroon": true,

		// Additional descriptive colors
		"cream": true, "mint": true, "peach": true, "rose": true, "sage": true, "sky": true,
		"stone": true, "amber": true, "emerald": true, "ruby": true, "sapphire": true, "jade": true,
	}

	return validCSSColors[strings.ToLower(color)]
}
