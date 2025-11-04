package routes

import (
	"github.com/HSouheill/barrim_backend/controllers"
	"github.com/HSouheill/barrim_backend/middleware"
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/mongo"
)

// RegisterServiceProviderCategoryRoutes sets up all service provider category-related routes
func RegisterServiceProviderCategoryRoutes(e *echo.Echo, db *mongo.Database) {
	serviceProviderCategoryController := controllers.NewServiceProviderCategoryController(db)

	// Service provider category routes group
	serviceProviderCategories := e.Group("/api/service-provider-categories")

	// Public routes (no auth required)
	serviceProviderCategories.GET("", serviceProviderCategoryController.GetAllServiceProviderCategories)
	serviceProviderCategories.GET("/:id", serviceProviderCategoryController.GetServiceProviderCategory)

	// Admin protected routes
	adminServiceProviderCategories := e.Group("/api/admin/service-provider-categories")
	adminServiceProviderCategories.Use(middleware.JWTMiddleware())
	adminServiceProviderCategories.Use(middleware.RequireUserType("admin", "super_admin"))

	// CRUD operations for service provider categories
	adminServiceProviderCategories.POST("", serviceProviderCategoryController.CreateServiceProviderCategory)
	adminServiceProviderCategories.PUT("/:id", serviceProviderCategoryController.UpdateServiceProviderCategory)
	adminServiceProviderCategories.DELETE("/:id", serviceProviderCategoryController.DeleteServiceProviderCategory)
}
