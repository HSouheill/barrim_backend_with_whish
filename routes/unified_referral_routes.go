// routes/unified_referral_routes.go
package routes

import (
	"github.com/HSouheill/barrim_backend/controllers"
	"github.com/HSouheill/barrim_backend/middleware"
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/mongo"
)

// RegisterUnifiedReferralRoutes registers routes for unified referral operations
func RegisterUnifiedReferralRoutes(e *echo.Echo, db *mongo.Client) {
	// Create a new unified referral controller
	unifiedReferralController := controllers.NewUnifiedReferralController(db)

	// Group routes with authentication middleware
	referralGroup := e.Group("/api/referrals")
	referralGroup.Use(middleware.JWTMiddleware())

	// Register unified referral routes
	referralGroup.POST("/apply", unifiedReferralController.HandleReferral)
	referralGroup.GET("/data", unifiedReferralController.GetReferralData)
	referralGroup.GET("/qrcode", unifiedReferralController.GetReferralQRCode)
}
