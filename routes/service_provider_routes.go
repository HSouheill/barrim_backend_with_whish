// routes/serviceProviders_routes.go
package routes

import (
	"log"

	"github.com/HSouheill/barrim_backend/controllers"
	"github.com/HSouheill/barrim_backend/middleware"
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/mongo"
)

// RegisterServiceProviderRoutes sets up all service provider-related routes
func RegisterServiceProviderRoutes(e *echo.Echo, db *mongo.Database, serviceProviderVoucherController *controllers.ServiceProviderVoucherController) {
	log.Println("Registering service provider routes...")

	serviceProviderSubscriptionController := controllers.NewServiceProviderSubscriptionController(db)
	salesPersonController := controllers.NewSalesPersonController(db.Client())
	serviceProviderController := controllers.NewServiceProviderController(db.Client())

	// Service provider routes group
	serviceProvider := e.Group("/api/service-providers")

	// Public route to get service provider data by ID
	serviceProvider.GET("/:id/full-data", serviceProviderController.GetFullServiceProviderData)
	log.Println("Created service provider group at /api/service-providers")

	// Protected routes (require service provider authentication)
	protected := serviceProvider.Group("")
	protected.Use(middleware.JWTMiddleware())
	protected.Use(middleware.RequireUserType("serviceProvider"))
	log.Println("Added middleware to protected group")

	// Subscription routes with debug logging
	protected.GET("/subscription-plans", func(c echo.Context) error {
		log.Printf("Received request for subscription plans from %s", c.Request().RemoteAddr)
		return serviceProviderSubscriptionController.GetServiceProviderSubscriptionPlans(c)
	})
	log.Println("Registered /subscription-plans endpoint")

	protected.GET("/subscription/current", func(c echo.Context) error {
		log.Printf("Received request for current subscription from %s", c.Request().RemoteAddr)
		return serviceProviderSubscriptionController.GetCurrentSubscription(c)
	})
	log.Println("Registered /subscription/current endpoint")

	protected.GET("/subscription/remaining-time", func(c echo.Context) error {
		log.Printf("Received request for subscription remaining time from %s", c.Request().RemoteAddr)
		return serviceProviderSubscriptionController.GetSubscriptionTimeRemaining(c)
	})
	log.Println("Registered /subscription/remaining-time endpoint")

	protected.POST("/subscription/cancel", func(c echo.Context) error {
		log.Printf("Received request to cancel subscription from %s", c.Request().RemoteAddr)
		return serviceProviderSubscriptionController.CancelSubscription(c)
	})
	log.Println("Registered /subscription/cancel endpoint")

	protected.POST("/subscription-requests", func(c echo.Context) error {
		log.Printf("Received subscription request from %s", c.Request().RemoteAddr)
		return serviceProviderSubscriptionController.CreateServiceProviderSubscription(c)
	})
	log.Println("Registered /subscription-requests endpoint")

	// Sponsorship routes for service providers
	protected.POST("/sponsorship/request", func(c echo.Context) error {
		log.Printf("Received sponsorship request from %s", c.Request().RemoteAddr)
		return serviceProviderSubscriptionController.CreateServiceProviderSponsorshipRequest(c)
	})
	log.Println("Registered /sponsorship/request endpoint")

	protected.GET("/sponsorship/remaining-time", func(c echo.Context) error {
		log.Printf("Received request for sponsorship remaining time from %s", c.Request().RemoteAddr)
		return serviceProviderSubscriptionController.GetServiceProviderSponsorshipRemainingTime(c)
	})
	log.Println("Registered /sponsorship/remaining-time endpoint")

	// Service provider profile routes
	protected.GET("/profile", func(c echo.Context) error {
		log.Printf("Received request for service provider profile from %s", c.Request().RemoteAddr)
		// TODO: Implement GetServiceProviderProfile
		return c.JSON(501, map[string]string{"message": "Not implemented yet"})
	})
	log.Println("Registered /profile endpoint")

	protected.PUT("/profile", func(c echo.Context) error {
		log.Printf("Received request to update service provider profile from %s", c.Request().RemoteAddr)
		// TODO: Implement UpdateServiceProviderProfile
		return c.JSON(501, map[string]string{"message": "Not implemented yet"})
	})
	log.Println("Registered /profile update endpoint")

	// Toggle status route - service providers can toggle their own status
	protected.PUT("/toggle-status/:id", func(c echo.Context) error {
		log.Printf("Received request to toggle service provider status from %s", c.Request().RemoteAddr)
		return serviceProviderController.ToggleEntityStatus(c)
	})
	log.Println("Registered /toggle-status/:id endpoint")

	// Update description route - service providers can update their description
	protected.PUT("/description", func(c echo.Context) error {
		log.Printf("Received request to update service provider description from %s", c.Request().RemoteAddr)
		return serviceProviderController.UpdateServiceProviderDescription(c)
	})
	log.Println("Registered /description endpoint")

	// Portfolio image routes - service providers can manage portfolio images
	protected.POST("/portfolio/upload", func(c echo.Context) error {
		log.Printf("Received portfolio image upload request from %s", c.Request().RemoteAddr)
		return serviceProviderController.UploadPortfolioImage(c)
	})
	log.Println("Registered /portfolio/upload endpoint")

	protected.GET("/portfolio", func(c echo.Context) error {
		log.Printf("Received request for portfolio images from %s", c.Request().RemoteAddr)
		return serviceProviderController.GetPortfolioImages(c)
	})
	log.Println("Registered /portfolio endpoint")

	protected.PUT("/portfolio", func(c echo.Context) error {
		log.Printf("Received portfolio image update request from %s", c.Request().RemoteAddr)
		return serviceProviderController.UpdatePortfolioImage(c)
	})
	log.Println("Registered /portfolio update endpoint")

	protected.DELETE("/portfolio", func(c echo.Context) error {
		log.Printf("Received portfolio image delete request from %s", c.Request().RemoteAddr)
		return serviceProviderController.DeletePortfolioImage(c)
	})
	log.Println("Registered /portfolio delete endpoint")

	// Public routes (no authentication required)
	public := serviceProvider.Group("")

	// Registration route (handled by sales person)
	public.POST("/register", func(c echo.Context) error {
		log.Printf("Received service provider registration request from %s", c.Request().RemoteAddr)
		return salesPersonController.CreateServiceProvider(c)
	})
	log.Println("Registered /register endpoint")

	// Whish payment callback routes (public - no auth required for Whish callbacks, registered directly on main Echo instance)
	e.GET("/api/whish/service-provider/payment/callback/success", func(c echo.Context) error {
		log.Printf("Received Whish payment success callback for service provider from %s", c.Request().RemoteAddr)
		return serviceProviderSubscriptionController.HandleWhishPaymentSuccess(c)
	})
	log.Println("Registered /api/whish/service-provider/payment/callback/success endpoint")

	e.GET("/api/whish/service-provider/payment/callback/failure", func(c echo.Context) error {
		log.Printf("Received Whish payment failure callback for service provider from %s", c.Request().RemoteAddr)
		return serviceProviderSubscriptionController.HandleWhishPaymentFailure(c)
	})
	log.Println("Registered /api/whish/service-provider/payment/callback/failure endpoint")

	// Admin/Manager routes for toggling any service provider status
	adminGroup := serviceProvider.Group("/admin")
	adminGroup.Use(middleware.JWTMiddleware())
	adminGroup.Use(middleware.RequireUserType("admin", "manager", "sales_manager"))

	adminGroup.PUT("/toggle-status/:id", func(c echo.Context) error {
		log.Printf("Received admin request to toggle service provider status from %s", c.Request().RemoteAddr)
		return serviceProviderController.ToggleEntityStatus(c)
	})
	log.Println("Registered admin /toggle-status/:id endpoint")

	// ============= Voucher Routes =============

	// Service provider voucher routes
	protected.GET("/vouchers/available", func(c echo.Context) error {
		log.Printf("Received request for available vouchers from %s", c.Request().RemoteAddr)
		return serviceProviderVoucherController.GetAvailableVouchersForServiceProvider(c)
	})
	log.Println("Registered /vouchers/available endpoint")

	protected.POST("/vouchers/purchase", func(c echo.Context) error {
		log.Printf("Received voucher purchase request from %s", c.Request().RemoteAddr)
		return serviceProviderVoucherController.PurchaseVoucherForServiceProvider(c)
	})
	log.Println("Registered /vouchers/purchase endpoint")

	protected.GET("/vouchers/purchased", func(c echo.Context) error {
		log.Printf("Received request for purchased vouchers from %s", c.Request().RemoteAddr)
		return serviceProviderVoucherController.GetServiceProviderVouchers(c)
	})
	log.Println("Registered /vouchers/purchased endpoint")

	protected.PUT("/vouchers/:id/use", func(c echo.Context) error {
		log.Printf("Received voucher use request from %s", c.Request().RemoteAddr)
		return serviceProviderVoucherController.UseVoucherForServiceProvider(c)
	})
	log.Println("Registered /vouchers/:id/use endpoint")

	log.Println("Finished registering all service provider routes")
}
