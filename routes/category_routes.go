package routes

import (
	"github.com/HSouheill/barrim_backend/controllers"
	"github.com/HSouheill/barrim_backend/middleware"
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/mongo"
)

// RegisterCategoryRoutes sets up all category-related routes
func RegisterCategoryRoutes(e *echo.Echo, db *mongo.Database) {
	categoryController := controllers.NewCategoryController(db)

	// Category routes group
	categories := e.Group("/api/categories")

	// Public routes (no auth required)
	categories.GET("", categoryController.GetAllCategories)
	categories.GET("/:id", categoryController.GetCategory)

	// Admin protected routes
	adminCategories := e.Group("/api/admin/categories")
	adminCategories.Use(middleware.JWTMiddleware())
	adminCategories.Use(middleware.RequireUserType("admin", "super_admin"))

	// CRUD operations for categories
	adminCategories.POST("", categoryController.CreateCategory) // Now accepts multipart form with name + optional logo
	adminCategories.PUT("/:id", categoryController.UpdateCategory)
	adminCategories.DELETE("/:id", categoryController.DeleteCategory)
	adminCategories.POST("/:id/logo", categoryController.UploadCategoryLogo) // Still available for updating existing categories
}
