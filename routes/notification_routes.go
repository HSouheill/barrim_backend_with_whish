package routes

import (
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/HSouheill/barrim_backend/controllers"
	"github.com/HSouheill/barrim_backend/middleware"
)

// RegisterNotificationRoutes registers all notification-related routes
func RegisterNotificationRoutes(e *echo.Echo, db *mongo.Client) {
	// Initialize notification controller
	notificationController := controllers.NewNotificationController(db)

	// Create notification routes group
	notificationGroup := e.Group("/api/notifications")

	// Send notification to service provider (public endpoint - can be called by admin/system)
	notificationGroup.POST("/send-to-service-provider", notificationController.SendToServiceProvider)

	// Create authenticated routes group for FCM token updates
	authGroup := e.Group("/api")
	authGroup.Use(middleware.JWTMiddleware())

	// FCM token update endpoints (require authentication)
	authGroup.POST("/service-provider/fcm-token", notificationController.UpdateServiceProviderFCMToken)
	authGroup.POST("/users/fcm-token", notificationController.UpdateUserFCMToken)
}
