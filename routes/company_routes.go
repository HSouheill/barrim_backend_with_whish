// routes/company_routes.go
package routes

import (
	"github.com/HSouheill/barrim_backend/controllers"
	customMiddleware "github.com/HSouheill/barrim_backend/middleware"
	"github.com/labstack/echo/v4"
)

func RegisterCompanyRoutes(e *echo.Echo, companyController *controllers.CompanyController, companyReferralController *controllers.CompanyReferralController, subscriptionController *controllers.SubscriptionController, companySubscriptionController *controllers.BranchSubscriptionController, companyVoucherController *controllers.CompanyVoucherController) {
	// Company-specific routes (restricted to company user type)
	companyGroup := e.Group("/api/companies")
	companyGroup.Use(customMiddleware.JWTMiddleware())
	companyGroup.Use(customMiddleware.RequireUserType("company", "user"))

	// Subscription routes
	companyGroup.GET("/subscription-plans", subscriptionController.GetCompanySubscriptionPlans)
	companyGroup.POST("/subscription/:branchId/request", companySubscriptionController.CreateBranchSubscriptionRequest)
	companyGroup.GET("/subscription/request/:branchId/status", companySubscriptionController.GetBranchSubscriptionRequestStatus)
	companyGroup.POST("/subscription/:branchId/verify-activate", companySubscriptionController.VerifyAndActivateBranchSubscription)
	companyGroup.POST("/subscription/:branchId/cancel", subscriptionController.CancelCompanySubscription)
	companyGroup.GET("/subscription/:branchId/remaining-time", companySubscriptionController.GetBranchSubscriptionRemainingTime)

	// Whish payment callback routes (public - no auth required for Whish callbacks)
	e.GET("/api/whish/payment/callback/success", companySubscriptionController.HandleWhishPaymentSuccess)
	e.GET("/api/whish/payment/callback/failure", companySubscriptionController.HandleWhishPaymentFailure)

	// Sponsorship routes for company branches
	companyGroup.POST("/sponsorship/:branchId/request", companyController.CreateCompanyBranchSponsorshipRequest)
	companyGroup.GET("/sponsorship/:branchId/remaining-time", companyController.GetCompanyBranchSponsorshipRemainingTime)

	// Whish payment callback routes for sponsorship (public - no auth required for Whish callbacks)
	e.GET("/api/whish/company-branch/sponsorship/payment/callback/success", func(c echo.Context) error {
		return companyController.HandleWhishSponsorshipPaymentSuccess(c)
	})
	e.GET("/api/whish/company-branch/sponsorship/payment/callback/failure", func(c echo.Context) error {
		return companyController.HandleWhishSponsorshipPaymentFailure(c)
	})

	// Test route to check if company routes are working
	companyGroup.GET("/test", func(c echo.Context) error {
		return c.JSON(200, map[string]string{
			"message": "Company routes are working",
		})
	})

	// Company profile routes
	companyGroup.GET("/data", companyController.GetCompanyData)
	companyGroup.GET("/full-data", companyController.GetAllCompanyData)
	companyGroup.PUT("/data", companyController.UpdateCompanyData)
	companyGroup.POST("/logo", companyController.UploadLogo)

	// New comprehensive profile update route
	companyGroup.PUT("/profile", companyController.UpdateCompanyProfile)

	// Branch routes
	companyGroup.POST("/branches", companyController.CreateBranch)
	companyGroup.GET("/branches", companyController.GetBranches)
	companyGroup.GET("/branches/:id", companyController.GetBranch)
	companyGroup.PUT("/branches/:id", companyController.UpdateBranch)
	companyGroup.DELETE("/branches/:id", companyController.DeleteBranch)

	// Public routes - no authentication required
	e.GET("/api/all-branches", companyController.GetAllBranches)

	// Public routes - no authentication required for viewing comments
	e.GET("/api/branches/:id/comments", companyController.GetBranchComments)
	e.GET("/api/branches/:id/rating", companyController.GetBranchRating)

	// Protected routes - authentication required for posting comments
	companyGroup.POST("/branches/:id/comments", companyController.CreateBranchComment)

	// Comment replies - only companies can reply to comments
	companyGroup.POST("/comments/:commentId/reply", companyController.ReplyToBranchComment)

	// ============= Referral Routes =============

	// Authenticated referral routes (works for both company and regular users)
	authGroup := e.Group("/api/referrals")
	authGroup.Use(customMiddleware.JWTMiddleware())

	// Handle referral submission
	authGroup.POST("/apply", companyReferralController.HandleReferral)

	// Get referral data
	authGroup.GET("/data", companyReferralController.GetReferralData)

	// Get QR code for referral
	authGroup.GET("/qrcode", companyReferralController.GetCompanyReferralQRCode)

	// Company-specific referral routes
	companyReferralGroup := companyGroup.Group("/referrals")
	companyReferralGroup.Use(customMiddleware.RequireUserType("company"))

	// Additional company-specific referral routes can be added here if needed in the future
	// companyReferralGroup.GET("/stats", companyReferralController.GetCompanyReferralStats)
	// companyReferralGroup.GET("/history", companyReferralController.GetCompanyReferralHistory)

	// ============= Voucher Routes =============

	// Company voucher routes
	companyGroup.GET("/vouchers/available", companyVoucherController.GetAvailableVouchersForCompany)
	companyGroup.POST("/vouchers/purchase", companyVoucherController.PurchaseVoucherForCompany)
	companyGroup.GET("/vouchers/purchased", companyVoucherController.GetCompanyVouchers)
	companyGroup.PUT("/vouchers/:id/use", companyVoucherController.UseVoucherForCompany)

	// Example for wholesaler branch subscription routes (to be added in wholesaler_routes.go):
	// wholesalerGroup.POST("/subscription/:branchId/request", wholesalerBranchSubscriptionController.CreateBranchSubscriptionRequest)
	// wholesalerGroup.GET("/subscription/request/:branchId/status", wholesalerBranchSubscriptionController.GetBranchSubscriptionRequestStatus)
	// wholesalerGroup.POST("/subscription/:branchId/cancel", wholesalerBranchSubscriptionController.CancelBranchSubscription)
	// wholesalerGroup.GET("/subscription/:branchId/remaining-time", wholesalerBranchSubscriptionController.GetBranchSubscriptionRemainingTime)
	// wholesalerGroup.POST("/subscription/request/:id/approve", wholesalerBranchSubscriptionController.ApproveBranchSubscriptionRequest)
	// wholesalerGroup.POST("/subscription/request/:id/reject", wholesalerBranchSubscriptionController.RejectBranchSubscriptionRequest)
}
