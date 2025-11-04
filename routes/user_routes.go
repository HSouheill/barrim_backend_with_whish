package routes

import (
	"net/http"

	"github.com/HSouheill/barrim_backend/controllers"
	"github.com/HSouheill/barrim_backend/middleware"
	"github.com/HSouheill/barrim_backend/models"
	"github.com/HSouheill/barrim_backend/utils"
	"github.com/HSouheill/barrim_backend/websocket"
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/mongo"
)

// RegisterUserRoutes sets up all user-related protected routes
func RegisterUserRoutes(e *echo.Echo, db *mongo.Client, userController *controllers.UserController, hub *websocket.Hub) {
	referralController := controllers.NewReferralController(db)
	reviewController := controllers.NewReviewController(db)
	bookingController := controllers.NewBookingController(db, hub)

	// Protected routes group
	r := e.Group("/api")
	r.Use(middleware.JWTMiddleware())

	// User profile and management routes
	r.GET("/users", userController.GetAllUsers)
	r.GET("/users/profile", userController.GetProfile)
	r.PUT("/users/profile", userController.UpdateProfile)
	r.PUT("/users/location", userController.UpdateLocation)
	r.DELETE("/users", userController.DeleteUser)
	r.POST("/upload-logo", userController.UploadCompanyLogo)
	r.POST("/upload-profile-photo", userController.UploadProfilePhoto)
	r.POST("/upload-user-profile-photo", userController.UploadUserProfilePhoto)
	r.GET("/users/get-user-data", userController.GetUserData)
	r.POST("/update-availability", userController.UpdateAvailability)
	r.GET("/user/companies", userController.GetCompaniesWithLocations)
	r.POST("/save-locations", userController.UpdateLocation)
	r.POST("/users/change-password", userController.ChangePassword)
	r.PUT("/users/personal-info", userController.UpdateUserPersonalInfo)
	r.GET("/users/notifications", userController.GetNotifications)

	// Referral routes
	r.POST("/users/handle-referral", referralController.HandleReferral)
	r.GET("/users/referral-data", referralController.GetReferralData)

	// Voucher routes
	voucherController := controllers.NewVoucherController(db.Database("barrim"))
	r.GET("/vouchers", voucherController.GetAvailableVouchers)
	r.POST("/vouchers/purchase", voucherController.PurchaseVoucher)
	r.GET("/vouchers/my-vouchers", voucherController.GetUserVouchers)
	r.PUT("/vouchers/:id/use", voucherController.UseVoucher)

	// Review routes
	r.POST("/reviews", reviewController.CreateReview)
	r.POST("/reviews/:id/reply", reviewController.PostReviewReply)
	r.GET("/reviews/:id/reply", reviewController.GetReviewReply)

	// Booking routes
	r.POST("/bookings", bookingController.CreateBooking)
	r.GET("/bookings/user", bookingController.GetUserBookings)
	r.PUT("/bookings/:id/status", bookingController.UpdateBookingStatus)
	r.PUT("/bookings/:id/cancel", bookingController.CancelBooking)

	// Favorites routes
	r.POST("/users/favorites", userController.AddBranchToFavorites)
	r.DELETE("/users/favorites", userController.RemoveBranchFromFavorites)
	r.GET("/users/favorites", userController.GetFavoriteBranches)
	r.POST("/users/favorite-providers", userController.AddServiceProviderToFavorites)
	r.DELETE("/users/favorite-providers", userController.RemoveServiceProviderFromFavorites)
	r.GET("/users/favorite-providers", userController.GetFavoriteServiceProviders)

	// WebSocket route
	r.GET("/ws", func(c echo.Context) error {
		// Authenticate the user using the token
		user, err := utils.GetUserFromToken(c, db)
		if err != nil {
			return c.JSON(http.StatusUnauthorized, models.Response{
				Status:  http.StatusUnauthorized,
				Message: "Unauthorized",
			})
		}

		// Handle the WebSocket connection
		return websocket.HandleWebSocket(c, hub, user.ID)
	})

	// Company-specific routes
	company := r.Group("/company")
	company.Use(middleware.RequireUserType("company", "user"))
	company.POST("/logo", userController.UploadCompanyLogo)
	company.POST("/handle-referral", referralController.HandleCompanyReferral)
	company.GET("/referral-data", referralController.GetCompanyReferralData)

	// Service provider specific routes
	serviceProvider := r.Group("/service-provider")
	serviceProvider.Use(middleware.RequireUserType("serviceProvider"))
	serviceProvider.POST("/availability", userController.UpdateAvailability)
	serviceProvider.POST("/photo", userController.UploadProfilePhoto)
	serviceProvider.POST("/certificate", func(c echo.Context) error {
		serviceProviderController := controllers.NewServiceProviderReferralController(db)
		return serviceProviderController.UploadCertificateImage(c)
	})
	serviceProvider.GET("/certificates", func(c echo.Context) error {
		serviceProviderController := controllers.NewServiceProviderReferralController(db)
		return serviceProviderController.GetCertificates(c)
	})
	serviceProvider.DELETE("/certificate", func(c echo.Context) error {
		serviceProviderController := controllers.NewServiceProviderReferralController(db)
		return serviceProviderController.DeleteCertificate(c)
	})
	serviceProvider.GET("/certificate/details", func(c echo.Context) error {
		serviceProviderController := controllers.NewServiceProviderReferralController(db)
		return serviceProviderController.GetCertificateDetails(c)
	})
	serviceProvider.GET("/details", func(c echo.Context) error {
		serviceProviderController := controllers.NewServiceProviderReferralController(db)
		return serviceProviderController.GetServiceProviderDetails(c)
	})
	serviceProvider.GET("/bookings", bookingController.GetProviderBookings)
	serviceProvider.GET("/bookings/pending", bookingController.GetPendingBookings)
	serviceProvider.PUT("/bookings/:id/respond", bookingController.AcceptBooking)
	serviceProvider.POST("/referral", func(c echo.Context) error {
		serviceProviderController := controllers.NewServiceProviderReferralController(db)
		return serviceProviderController.HandleServiceProviderReferral(c)
	})
	serviceProvider.GET("/referral-data", func(c echo.Context) error {
		serviceProviderController := controllers.NewServiceProviderReferralController(db)
		return serviceProviderController.GetServiceProviderReferralData(c)
	})
	serviceProvider.PUT("/social-links", func(c echo.Context) error {
		serviceProviderController := controllers.NewServiceProviderReferralController(db)
		return serviceProviderController.UpdateServiceProviderSocialLinks(c)
	})
	serviceProvider.PUT("/update", func(c echo.Context) error {
		serviceProviderController := controllers.NewServiceProviderReferralController(db)
		return serviceProviderController.UpdateServiceProviderData(c)
	})

	adsController := controllers.NewAdsController(db.Database("barrim"))
	r.GET("/ads", adsController.GetAds)
}
