package routes

import (
	"github.com/HSouheill/barrim_backend/controllers"
	"github.com/HSouheill/barrim_backend/middleware"
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/mongo"
)

// RegisterSubscriptionRoutes sets up all subscription and commission routes
func RegisterSubscriptionRoutes(e *echo.Echo, db *mongo.Client) {
	subscriptionController := controllers.NewSubscriptionController(db.Database("barrim"))
	serviceProviderSubscriptionController := controllers.NewServiceProviderSubscriptionController(db.Database("barrim"))

	// Financial dashboard routes
	financialDashboard := e.Group("/api/admin/financial_dashboard_revenue")
	financialDashboard.Use(middleware.JWTMiddleware())
	// Set DB in context for this group
	financialDashboard.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("db", db)
			return next(c)
		}
	})
	financialDashboard.Use(middleware.RequireFinancialDashboardAccess())

	financialDashboard.POST("/company-subscription/:id/approve", subscriptionController.ApproveCompanySubscriptionRequest)
	financialDashboard.POST("/company-subscription/:id/reject", subscriptionController.RejectCompanySubscriptionRequest)
	financialDashboard.POST("/service-provider-subscription/:id/approve", serviceProviderSubscriptionController.ApproveServiceProviderSubscriptionRequest)
	financialDashboard.POST("/service-provider-subscription/:id/reject", serviceProviderSubscriptionController.RejectServiceProviderSubscriptionRequest)
	financialDashboard.POST("/wholesaler-subscription/:id/approve", subscriptionController.ApproveWholesalerSubscriptionRequest)
	financialDashboard.POST("/wholesaler-subscription/:id/reject", subscriptionController.RejectWholesalerSubscriptionRequest)
	financialDashboard.GET("/service-provider-subscription/requests/pending", serviceProviderSubscriptionController.GetPendingServiceProviderSubscriptionRequests)
	financialDashboard.GET("/wholesaler-subscription/requests/pending", subscriptionController.GetPendingWholesalerSubscriptionRequests)

	// Manager approval endpoints
	manager := e.Group("/api/manager")
	manager.Use(middleware.JWTMiddleware())
	manager.Use(middleware.RequireUserType("manager", "admin"))

	// Manager approval endpoints for company subscriptions
	manager.POST("/company-subscription/:id/approve", subscriptionController.ApproveCompanySubscriptionRequestByManager)
	manager.POST("/company-subscription/:id/reject", subscriptionController.RejectCompanySubscriptionRequestByManager)

	// Manager approval endpoints for service provider subscriptions
	manager.POST("/service-provider-subscription/:id/approve", serviceProviderSubscriptionController.ApproveServiceProviderSubscriptionRequestByManager)
	manager.POST("/service-provider-subscription/:id/reject", serviceProviderSubscriptionController.RejectServiceProviderSubscriptionRequestByManager)

	// Manager approval endpoints for wholesaler subscriptions
	manager.POST("/wholesaler-subscription/:id/approve", subscriptionController.ApproveWholesalerSubscriptionRequestByManager)
	manager.POST("/wholesaler-subscription/:id/reject", subscriptionController.RejectWholesalerSubscriptionRequestByManager)

	// Manager endpoints to get all pending subscription requests
	manager.GET("/service-provider-subscription/requests/pending", serviceProviderSubscriptionController.GetPendingServiceProviderSubscriptionRequests)
	manager.GET("/wholesaler-subscription/requests/pending", subscriptionController.GetPendingWholesalerSubscriptionRequests)

	// Commission routes for authenticated users
	r := e.Group("/api")
	r.Use(middleware.JWTMiddleware())

	// Commission balance endpoint for authenticated users
	r.GET("/commission/balance", subscriptionController.GetTotalCommissionBalance)
	// Commission withdrawal endpoint for authenticated users
	r.POST("/commission/withdraw", subscriptionController.RequestCommissionWithdrawal)
	// Commission withdrawals total endpoint for authenticated users
	r.GET("/commission/withdrawals", subscriptionController.GetTotalWithdrawals)
	// Commission summary endpoint for authenticated users
	r.GET("/commission/summary", subscriptionController.GetCommissionSummary)
	// Commission details endpoint for authenticated users
	r.GET("/commission/details", subscriptionController.GetCommissions)

	// Commission routes for sales roles
	commission := r.Group("/commission")
	commission.Use(middleware.RequireUserType("sales_manager", "admin", "salesperson"))
	commission.GET("/balance", subscriptionController.GetTotalCommissionBalance)
	commission.POST("/withdraw", subscriptionController.RequestCommissionWithdrawal)
	commission.GET("/withdrawals", subscriptionController.GetTotalWithdrawals)
	commission.GET("/summary", subscriptionController.GetCommissionSummary)
	commission.GET("/details", subscriptionController.GetCommissions)
}
