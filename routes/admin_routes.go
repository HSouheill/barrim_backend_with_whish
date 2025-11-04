package routes

import (
	"github.com/HSouheill/barrim_backend/controllers"
	"github.com/HSouheill/barrim_backend/middleware"
	"github.com/HSouheill/barrim_backend/websocket"
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/mongo"
)

// RegisterAdminRoutes sets up all admin-related routes
func RegisterAdminRoutes(e *echo.Echo, db *mongo.Database, hub *websocket.Hub) {
	adminController := controllers.NewAdminController(db)
	subscriptionController := controllers.NewSubscriptionController(db)
	serviceProviderSubscriptionController := controllers.NewServiceProviderSubscriptionController(db)
	// Get the client from the database to initialize controllers that need it
	client := db.Client()
	serviceProviderController := controllers.NewServiceProviderController(client)
	adminBranchController := controllers.NewAdminBranchController(client)
	companyController := controllers.NewCompanyController(client)
	adsController := controllers.NewAdsController(db)

	// Admin routes group
	admin := e.Group("/api/admin")

	// Public routes (no auth required)
	admin.POST("/login", adminController.Login)
	admin.POST("/forgot-password", adminController.ForgotPassword)
	admin.POST("/verify-otp-reset", adminController.VerifyOTPAndResetPassword)

	// Super-admin protected routes
	superAdmin := admin.Group("")
	superAdmin.Use(middleware.JWTMiddleware())
	superAdmin.Use(middleware.RequireUserType("super_admin"))
	superAdmin.POST("/register", adminController.RegisterAdmin)

	// Protected routes (require admin authentication)
	protected := admin.Group("")
	protected.Use(middleware.JWTMiddleware())
	protected.Use(middleware.RequireUserType("admin"))

	// User management routes
	protected.GET("/users", adminController.GetActiveUsers)
	protected.GET("/users/all", adminController.GetAllUsers)
	protected.POST("/Create-managers", adminController.CreateUserManager)
	protected.POST("/users", adminController.CreateUser)
	protected.DELETE("/users/:id", adminController.DeleteUser)
	protected.GET("/companies", companyController.GetAllCompanies)

	// Admin wallet routes
	protected.GET("/wallet", adminController.GetAdminWallet)
	protected.GET("/wallet/transactions", adminController.GetAdminWalletTransactions)

	// All entities route
	protected.GET("/all-entities", adminController.GetAllEntities)

	// Sales manager routes
	protected.POST("/sales-managers", adminController.CreateSalesManager)
	protected.GET("/sales-managers", adminController.GetAllSalesManagers)
	protected.GET("/sales-managers/:id", adminController.GetSalesManager)
	protected.PUT("/sales-managers/:id", adminController.UpdateSalesManager)
	protected.DELETE("/sales-managers/:id", adminController.DeleteSalesManager)

	// Salesperson routes
	protected.POST("/salespersons", adminController.CreateSalesperson)
	protected.GET("/salespersons", adminController.GetAdminSalespersons)
	protected.GET("/salespersons/all", adminController.GetAllSalespersons)
	protected.GET("/salespersons/:id", adminController.GetSalesperson)
	protected.PUT("/salespersons/:id", adminController.UpdateSalesperson)
	protected.DELETE("/salespersons/:id", adminController.DeleteSalesperson)

	// Manager routes
	protected.GET("/managers", adminController.GetAllManagers)
	protected.GET("/managers/:id", adminController.GetManager)
	protected.PUT("/managers/:id", adminController.UpdateManager)
	protected.DELETE("/managers/:id", adminController.DeleteManager)

	// Subscription plan routes
	protected.POST("/subscription-plans", subscriptionController.CreateSubscriptionPlan)
	protected.GET("/subscription-plans", subscriptionController.GetSubscriptionPlans)
	protected.GET("/subscription-plans/:id", subscriptionController.GetSubscriptionPlan)
	protected.PUT("/subscription-plans/:id", subscriptionController.UpdateSubscriptionPlan)
	protected.DELETE("/subscription-plans/:id", subscriptionController.DeleteSubscriptionPlan)
	protected.GET("/subscription-plans/company", subscriptionController.GetCompanySubscriptionPlans)
	protected.GET("/subscription-plans/service-provider", subscriptionController.GetServiceProviderSubscriptionPlans)

	// Sponsorship routes
	sponsorshipController := controllers.NewSponsorshipController(db)
	protected.POST("/sponsorships", sponsorshipController.CreateSponsorship)
	protected.POST("/sponsorships/service-provider", sponsorshipController.CreateServiceProviderSponsorship)
	protected.POST("/sponsorships/company-wholesaler", sponsorshipController.CreateCompanyWholesalerSponsorship)
	protected.GET("/sponsorships", sponsorshipController.GetSponsorships)
	protected.GET("/sponsorships/:id", sponsorshipController.GetSponsorship)
	protected.PUT("/sponsorships/:id", sponsorshipController.UpdateSponsorship)
	protected.DELETE("/sponsorships/:id", sponsorshipController.DeleteSponsorship)

	// Duration management routes
	protected.GET("/sponsorships/duration/info", sponsorshipController.GetDurationInfo)
	protected.GET("/sponsorships/duration/validate", sponsorshipController.ValidateDuration)

	// Sponsorship subscription routes
	sponsorshipSubscriptionController := controllers.NewSponsorshipSubscriptionController(db)
	protected.GET("/sponsorship-subscriptions/requests/pending", sponsorshipSubscriptionController.GetPendingSponsorshipSubscriptionRequests)
	protected.POST("/sponsorship-subscriptions/requests/:id/process", sponsorshipSubscriptionController.ProcessSponsorshipSubscriptionRequest)
	protected.GET("/sponsorship-subscriptions/active", sponsorshipSubscriptionController.GetActiveSponsorshipSubscriptions)

	// Admin sponsorship subscription time remaining routes
	protected.GET("/sponsorship-subscriptions/company-branch/:branchId/time-remaining", sponsorshipSubscriptionController.GetTimeRemainingForCompanyBranch)
	protected.GET("/sponsorship-subscriptions/wholesaler-branch/:branchId/time-remaining", sponsorshipSubscriptionController.GetTimeRemainingForWholesalerBranch)
	protected.GET("/sponsorship-subscriptions/service-provider/:serviceProviderId/time-remaining", sponsorshipSubscriptionController.GetTimeRemainingForServiceProvider)
	protected.GET("/sponsorship-subscriptions/:entityType/:entityId/time-remaining", sponsorshipSubscriptionController.GetTimeRemainingForEntity)

	// Subscription request routes
	protected.GET("/subscription-requests/pending", subscriptionController.GetPendingSubscriptionRequests)
	protected.POST("/subscription-requests/:id/process", subscriptionController.ProcessSubscriptionRequest)

	// Get pending subscription requests (admin only)
	protected.GET("/wholesaler/subscription/pending", subscriptionController.GetPendingWholesalerSubscriptionRequests)

	// Get pending branch subscription requests only (admin only)
	protected.GET("/company/branch-subscription/requests/pending", subscriptionController.GetPendingBranchSubscriptionRequests)
	// Process branch subscription request (admin only)
	// DISABLED: Payment is now handled automatically via Whish payment callbacks
	// protected.POST("/company/branch-subscription/requests/:id/process", subscriptionController.ProcessBranchSubscriptionRequest)

	// Process subscription request (admin only)
	protected.POST("/wholesaler/subscription/process/:id", subscriptionController.ProcessWholesalerSubscriptionRequest)

	// Service Provider subscription request routes
	protected.GET("/service-provider/subscription/pending", serviceProviderSubscriptionController.GetPendingServiceProviderSubscriptionRequests)
	protected.POST("/service-provider/subscription/process/:id", serviceProviderSubscriptionController.ProcessServiceProviderSubscriptionRequest)

	// Service Provider status toggle route
	protected.PUT("/service-providers/:id/toggle-status", serviceProviderController.ToggleEntityStatus)

	// Branch request management routes
	protected.GET("/branch-requests/pending", adminBranchController.GetPendingBranchRequests)
	protected.GET("/branch-requests/:id", adminBranchController.GetBranchRequest)
	protected.POST("/branch-requests/:id/process", adminBranchController.ProcessBranchRequest)

	protected.GET("/access-roles", adminController.ListAccessRoles)

	// Manager approval routes
	manager := admin.Group("/manager")
	manager.Use(middleware.JWTMiddleware())
	manager.Use(middleware.RequireUserType("manager"))
	manager.POST("/approve/company/:id", adminController.ApproveCompanyByManager)
	manager.POST("/approve/serviceprovider/:id", adminController.ApproveServiceProviderByManager)
	manager.POST("/approve/wholesaler/:id", adminController.ApproveWholesalerByManager)
	manager.POST("/activate/company/:id", adminController.ActivateCompanyByManager)
	manager.POST("/activate/wholesaler/:id", adminController.ActivateWholesalerByManager)
	manager.POST("/activate/serviceprovider/:id", adminController.ActivateServiceProviderByManager)

	// Manager service provider status toggle route
	manager.PUT("/service-providers/:id/toggle-status", serviceProviderController.ToggleEntityStatus)

	// Wholesaler branch subscription approval/rejection (admin only)
	protected.POST("/wholesaler-branch-subscription/request/:id/approve", controllers.NewWholesalerBranchSubscriptionController(db).ApproveBranchSubscriptionRequest)
	protected.POST("/wholesaler-branch-subscription/request/:id/reject", controllers.NewWholesalerBranchSubscriptionController(db).RejectBranchSubscriptionRequest)

	// Get pending wholesaler branch subscription requests (admin only)
	protected.GET("/wholesaler/branch-subscription/requests/pending", controllers.NewWholesalerBranchSubscriptionController(db).GetPendingWholesalerBranchSubscriptionRequests)

	// Process wholesaler branch subscription requests (admin only)
	protected.POST("/wholesaler/branch-subscription/requests/:id/process", controllers.NewWholesalerBranchSubscriptionController(db).ProcessWholesalerBranchSubscriptionRequest)

	// Commission withdrawal approval/rejection routes (admin only)
	protected.GET("/withdrawals/pending", subscriptionController.GetPendingWithdrawalRequests)
	protected.POST("/withdrawals/:id/approve", subscriptionController.ApproveWithdrawalRequest)
	protected.POST("/withdrawals/:id/reject", subscriptionController.RejectWithdrawalRequest)

	// Toggle entity status (active/inactive) for company, wholesaler, serviceProvider
	protected.PUT("/toggle-status/:entityType/:id", adminController.ToggleEntityStatus)
	protected.PUT("/toggle-status/company/:companyId/branch/:branchId", adminController.ToggleCompanyBranchStatus)
	protected.PUT("/toggle-status/wholesaler/:wholesalerId/branch/:branchId", adminController.ToggleWholesalerBranchStatus)

	// Branch management routes
	protected.GET("/wholesaler/:wholesalerId/branches", adminController.GetWholesalerBranches)
	protected.GET("/wholesalers/branches", adminController.GetAllWholesalerBranches)

	// Delete branch routes
	protected.DELETE("/company/:companyId/branch/:branchId", adminController.DeleteCompanyBranch)
	protected.DELETE("/wholesaler/:wholesalerId/branch/:branchId", adminController.DeleteWholesalerBranch)

	// Booking management routes
	bookingController := controllers.NewBookingController(client, hub)
	protected.GET("/bookings", bookingController.GetAllBookingsForAdmin)
	protected.DELETE("/bookings/:id", bookingController.DeleteBookingForAdmin)

	// Review management routes
	reviewController := controllers.NewReviewController(client)
	protected.GET("/reviews", reviewController.GetAllReviewsAndRepliesForAdmin)
	protected.PUT("/reviews/:id/verify", reviewController.ToggleReviewVerification)
	protected.DELETE("/reviews/:id", reviewController.DeleteReview)

	// Delete entity by ID
	protected.DELETE("/entities/:entityType/:id", adminController.DeleteEntity)

	// Debug entity (for troubleshooting)
	protected.GET("/debug/:entityType/:id", adminController.DebugEntity)

	// Voucher management routes
	voucherController := controllers.NewVoucherController(db)
	protected.POST("/vouchers", voucherController.CreateVoucher)
	protected.POST("/vouchers/json", voucherController.CreateVoucherJSON)
	protected.POST("/vouchers/user-type", voucherController.CreateUserTypeVoucher)
	protected.GET("/vouchers", voucherController.GetAllVouchers)
	protected.PUT("/vouchers/:id", voucherController.UpdateVoucher)
	protected.DELETE("/vouchers/:id", voucherController.DeleteVoucher)
	protected.PUT("/vouchers/:id/toggle-status", voucherController.ToggleVoucherStatus)

	// Pending requests from admin-created salespersons
	protected.GET("/pending-requests", adminController.GetPendingRequestsFromAdminSalespersons)
	protected.POST("/pending-requests/process", adminController.ProcessPendingRequest)
	protected.POST("/pending-requests/approve", adminController.ApprovePendingRequest)
	protected.POST("/pending-requests/reject", adminController.RejectPendingRequest)

	// Also allow managers and sales managers with business_management role (controller checks role)
	manager.PUT("/toggle-status/:entityType/:id", adminController.ToggleEntityStatus)
	manager.DELETE("/entities/:entityType/:id", adminController.DeleteEntity)
	manager.GET("/debug/:entityType/:id", adminController.DebugEntity)

	manager.GET("/bookings", bookingController.GetAllBookingsForAdmin)
	manager.DELETE("/bookings/:id", bookingController.DeleteBookingForAdmin)

	salesManager := admin.Group("/salesmanager")
	salesManager.Use(middleware.JWTMiddleware())
	salesManager.Use(middleware.RequireUserType("sales_manager"))
	salesManager.PUT("/toggle-status/:entityType/:id", adminController.ToggleEntityStatus)
	salesManager.PUT("/service-providers/:id/toggle-status", serviceProviderController.ToggleEntityStatus)
	salesManager.DELETE("/entities/:entityType/:id", adminController.DeleteEntity)

	salesManager.GET("/bookings", bookingController.GetAllBookingsForAdmin)
	salesManager.DELETE("/bookings/:id", bookingController.DeleteBookingForAdmin)

	protected.POST("/ads", adsController.PostAd)
	protected.GET("/ads", adsController.GetAds)
	protected.DELETE("/ads/:id", adsController.DeleteAd)

}
