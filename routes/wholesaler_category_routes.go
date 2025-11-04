package routes

import (
	"github.com/HSouheill/barrim_backend/controllers"
	"github.com/HSouheill/barrim_backend/middleware"
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/mongo"
)

func RegisterWholesalerCategoryRoutes(e *echo.Echo, db *mongo.Database) {
	wholesalerCategoryController := controllers.NewWholesalerCategoryController(db)

	// Public routes (no authentication required)
	wholesalerCategories := e.Group("/api/wholesaler-categories")
	wholesalerCategories.GET("", wholesalerCategoryController.GetAllWholesalerCategories)
	wholesalerCategories.GET("/:id", wholesalerCategoryController.GetWholesalerCategory)

	// Admin routes (require authentication)
	adminWholesalerCategories := e.Group("/api/admin/wholesaler-categories")
	adminWholesalerCategories.Use(middleware.JWTMiddleware())
	adminWholesalerCategories.Use(middleware.RequireUserType("admin", "super_admin"))

	adminWholesalerCategories.POST("", wholesalerCategoryController.CreateWholesalerCategory)
	adminWholesalerCategories.PUT("/:id", wholesalerCategoryController.UpdateWholesalerCategory)
	adminWholesalerCategories.DELETE("/:id", wholesalerCategoryController.DeleteWholesalerCategory)
}
