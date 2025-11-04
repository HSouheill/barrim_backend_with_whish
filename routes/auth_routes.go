package routes

import (
	"github.com/HSouheill/barrim_backend/controllers"
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/mongo"
)

// RegisterAuthRoutes sets up all authentication and public routes
func RegisterAuthRoutes(e *echo.Echo, db *mongo.Client, authController *controllers.AuthController, userController *controllers.UserController) {
	passwordController := controllers.NewPasswordController(db)
	serviceProviderController := controllers.NewServiceProviderReferralController(db)
	reviewController := controllers.NewReviewController(db)
	wholesalerController := controllers.NewWholesalerController(db)

	// Public authentication routes
	e.POST("/api/auth/signup", authController.Signup)
	e.POST("/api/auth/check-exists", authController.CheckEmailOrPhoneExists)
	// Admin login is handled in admin_routes.go
	e.POST("/api/auth/login", authController.Login)
	e.POST("/api/auth/logout", authController.Logout)
	e.POST("/api/auth/force-logout", authController.ForceLogout)
	e.GET("/api/auth/logout-history", authController.GetLogoutHistory)
	e.POST("api/auth/google", authController.GoogleLogin)
	e.POST("api/auth/google-cloud-signin", authController.GoogleCloudSignIn)
	e.POST("api/auth/signup-service-provider-with-logo", authController.SignupServiceProviderWithLogo)
	e.POST("api/auth/signup-wholesaler-with-logo", authController.SignupWholesalerWithLogo)
	e.POST("api/auth/sms-verify-otp", authController.VerifyOTP)
	e.POST("api/auth/resend-otp", authController.ResendOTP)
	e.POST("/api/auth/apple-login", authController.AppleSignin)
	e.POST("/api/auth/google-auth-without-firebase", authController.GoogleAuthWithoutFirebase)
	e.GET("/api/auth/validate-token", authController.ValidateToken)
	e.POST("/api/auth/refresh-token", authController.RefreshToken)
	e.POST("/api/auth/remember-me/get", authController.GetRememberedCredentials)
	e.POST("/api/auth/remember-me/remove", authController.RemoveRememberedCredentials)
	e.POST("/api/auth/forget-password", passwordController.ForgetPassword)
	e.POST("/api/auth/verify-otp", passwordController.VerifyOTP)
	e.POST("/api/auth/reset-password", passwordController.ResetPassword)
	e.POST("/api/auth/signup-with-logo", authController.SignupWithLogo)

	// Public service provider routes
	e.GET("/api/service-providers", userController.SearchServiceProviders)
	e.GET("/api/service-providers/all", serviceProviderController.GetAllServiceProviders)
	e.GET("/api/service-providers/:userid/reviews", reviewController.GetReviewsByProviderID)
	e.GET("/api/service-providers/:id/logo", serviceProviderController.GetServiceProviderLogo)
	e.GET("/api/service-providers/:id", serviceProviderController.GetServiceProviderByID)
	e.GET("/api/qrcode/referral/:code", serviceProviderController.GenerateReferralQRCode)
	e.GET("/api/qrcode/referral/:code/base64", serviceProviderController.GenerateReferralQRCodeAsBase64)

	// Public wholesaler routes
	e.GET("/api/wholesalers", wholesalerController.GetAllWholesalers)

	// Public booking routes
	e.GET("/api/bookings/available-slots/:id", func(c echo.Context) error {
		bookingController := controllers.NewBookingController(db, nil)
		return bookingController.GetAvailableTimeSlots(c)
	})

	// Public company and wholesaler filter
	e.GET("/filter/companies-wholesalers", userController.FilterCompaniesAndWholesalers)

	// Public sponsorship routes
	e.GET("/api/sponsorships", func(c echo.Context) error {
		sponsorshipController := controllers.NewSponsorshipController(db.Database("barrim"))
		return sponsorshipController.GetSponsorships(c)
	})
	e.GET("/api/sponsorships/service-provider", func(c echo.Context) error {
		sponsorshipController := controllers.NewSponsorshipController(db.Database("barrim"))
		return sponsorshipController.GetServiceProviderSponsorships(c)
	})
	e.GET("/api/sponsorships/company-wholesaler", func(c echo.Context) error {
		sponsorshipController := controllers.NewSponsorshipController(db.Database("barrim"))
		return sponsorshipController.GetCompanyWholesalerSponsorships(c)
	})
	e.GET("/api/sponsorships/:id", func(c echo.Context) error {
		sponsorshipController := controllers.NewSponsorshipController(db.Database("barrim"))
		return sponsorshipController.GetSponsorship(c)
	})

	// Public sponsorship subscription routes
	e.POST("/api/sponsorship-subscriptions/request", func(c echo.Context) error {
		sponsorshipSubscriptionController := controllers.NewSponsorshipSubscriptionController(db.Database("barrim"))
		return sponsorshipSubscriptionController.CreateSponsorshipSubscriptionRequest(c)
	})

	// Public sponsorship subscription time remaining routes
	e.GET("/api/sponsorship-subscriptions/company-branch/:branchId/time-remaining", func(c echo.Context) error {
		sponsorshipSubscriptionController := controllers.NewSponsorshipSubscriptionController(db.Database("barrim"))
		return sponsorshipSubscriptionController.GetTimeRemainingForCompanyBranch(c)
	})
	e.GET("/api/sponsorship-subscriptions/wholesaler-branch/:branchId/time-remaining", func(c echo.Context) error {
		sponsorshipSubscriptionController := controllers.NewSponsorshipSubscriptionController(db.Database("barrim"))
		return sponsorshipSubscriptionController.GetTimeRemainingForWholesalerBranch(c)
	})
	e.GET("/api/sponsorship-subscriptions/service-provider/:serviceProviderId/time-remaining", func(c echo.Context) error {
		sponsorshipSubscriptionController := controllers.NewSponsorshipSubscriptionController(db.Database("barrim"))
		return sponsorshipSubscriptionController.GetTimeRemainingForServiceProvider(c)
	})

	// General entity time remaining route
	e.GET("/api/sponsorship-subscriptions/:entityType/:entityId/time-remaining", func(c echo.Context) error {
		sponsorshipSubscriptionController := controllers.NewSponsorshipSubscriptionController(db.Database("barrim"))
		return sponsorshipSubscriptionController.GetTimeRemainingForEntity(c)
	})

	// Public service provider status update
	e.PUT("/api/service-provider/status", serviceProviderController.UpdateServiceProviderStatus)

	// Public salesperson referral route (for users to use salesperson referral codes)
	salespersonReferralController := controllers.NewSalespersonReferralController(db.Database("barrim"))
	e.POST("/api/salesperson-referral/handle", salespersonReferralController.HandleReferral)
}
