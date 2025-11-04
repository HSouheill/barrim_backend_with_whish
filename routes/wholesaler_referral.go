// routes/wholesaler_routes.go
package routes

import (
	"github.com/HSouheill/barrim_backend/controllers"
	"github.com/HSouheill/barrim_backend/middleware"
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/mongo"
)

// RegisterWholesalerReferralRoutes registers routes for wholesaler referral operations
func RegisterWholesalerReferralRoutes(e *echo.Echo, db *mongo.Client) {
	// Create a new wholesaler referral controller
	wholesalerReferralController := controllers.NewWholesalerReferralController(db)

	// Group routes with authentication middleware
	wholesalerGroup := e.Group("/api/wholesaler")
	wholesalerGroup.Use(middleware.JWTMiddleware())

	// Register routes
	wholesalerGroup.POST("/referral", wholesalerReferralController.HandleReferral)
	wholesalerGroup.GET("/referral", wholesalerReferralController.GetReferralData)
	wholesalerGroup.GET("/referral/qrcode", wholesalerReferralController.GetWholesalerReferralQRCode)
}
