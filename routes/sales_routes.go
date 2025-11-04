package routes

import (
	"github.com/HSouheill/barrim_backend/controllers"
	"github.com/HSouheill/barrim_backend/middleware"
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/mongo"
)

// RegisterSalesRoutes sets up all sales manager and salesperson routes
func RegisterSalesRoutes(e *echo.Echo, db *mongo.Client) {
	salesManagerController := controllers.NewSalesManagerController(db.Database("barrim"))
	salesPersonController := controllers.NewSalesPersonController(db)
	salespersonReferralController := controllers.NewSalespersonReferralController(db.Database("barrim"))

	// Sales Manager routes
	salesManager := e.Group("/api/sales-manager")
	salesManager.Use(middleware.JWTMiddleware())
	salesManager.Use(middleware.RequireUserType("sales_manager", "admin", "salesperson", "manager"))

	// Salesperson management routes
	salesManager.POST("/salespersons", salesManagerController.CreateSalesperson)
	salesManager.GET("/salespersons", salesManagerController.GetAllSalespersons)
	salesManager.GET("/salespersons/:id", salesManagerController.GetSalesperson)
	salesManager.PUT("/salespersons/:id", salesManagerController.UpdateSalesperson)
	salesManager.DELETE("/salespersons/:id", salesManagerController.DeleteSalesperson)
	salesManager.GET("/salespersons/by-creator", salesManagerController.GetSalespersonsByCreator)

	// Pending entity creation approval routes
	salesManager.GET("/pending-companies", salesManagerController.GetPendingCompanyCreations)
	salesManager.POST("/pending-companies/:id/approve", salesManagerController.ApprovePendingCompany)
	salesManager.POST("/pending-companies/:id/reject", salesManagerController.RejectPendingCompany)

	// Created companies route (for viewing all created companies)
	salesManager.GET("/created-companies", salesManagerController.GetCreatedCompanies)

	salesManager.GET("/pending-wholesalers", salesManagerController.GetPendingWholesalerCreations)
	salesManager.POST("/pending-wholesalers/:id/approve", salesManagerController.ApprovePendingWholesaler)
	salesManager.POST("/pending-wholesalers/:id/reject", salesManagerController.RejectPendingWholesaler)

	salesManager.GET("/pending-service-providers", salesManagerController.GetPendingServiceProviderCreations)
	salesManager.POST("/pending-service-providers/:id/approve", salesManagerController.ApprovePendingServiceProvider)
	salesManager.POST("/pending-service-providers/:id/reject", salesManagerController.RejectPendingServiceProvider)

	// Subscription request processing routes for sales manager
	salesManager.GET("/subscription-requests/pending", salesManagerController.GetPendingSubscriptionRequests)
	salesManager.POST("/subscription-requests/:id/process", salesManagerController.ProcessSubscriptionRequest)
	salesManager.GET("/commission-withdrawal-history", salesManagerController.GetCommissionAndWithdrawalHistory)

	// Sales Person routes
	salesPerson := e.Group("/api/sales-person")
	salesPerson.Use(middleware.JWTMiddleware())
	salesPerson.Use(middleware.RequireUserType("salesperson", "salesManager", "admin", "manager"))

	// Company management
	salesPerson.POST("/companies", salesPersonController.CreateCompany)
	salesPerson.GET("/companies", salesPersonController.GetCompanies)
	salesPerson.GET("/companies/:id", salesPersonController.GetCompany)
	salesPerson.PUT("/companies/:id", salesPersonController.UpdateCompany)
	salesPerson.DELETE("/companies/:id", salesPersonController.DeleteCompany)

	// Wholesaler management
	salesPerson.POST("/wholesalers", salesPersonController.CreateWholesaler)
	salesPerson.GET("/wholesalers", salesPersonController.GetWholesalers)
	salesPerson.GET("/wholesalers/:id", salesPersonController.GetWholesaler)
	salesPerson.PUT("/wholesalers/:id", salesPersonController.UpdateWholesaler)
	salesPerson.DELETE("/wholesalers/:id", salesPersonController.DeleteWholesaler)

	// Service provider management
	salesPerson.POST("/service-providers", salesPersonController.CreateServiceProvider)
	salesPerson.GET("/service-providers", salesPersonController.GetServiceProviders)
	salesPerson.DELETE("/service-providers/:id", salesPersonController.DeleteServiceProvider)
	salesPerson.PUT("/service-providers/:id", salesPersonController.UpdateServiceProvider)

	// Commission routes
	salesPerson.GET("/commission-withdrawal-history", salesPersonController.GetCommissionAndWithdrawalHistory)
	salesPerson.GET("/created-users-details", salesPersonController.GetSalespersonCreatedUsersWithCommission)
	salesPerson.GET("/created-users", salesPersonController.GetAllCreatedUsers)

	// Salesperson referral routes
	salesPerson.POST("/referral/handle", salespersonReferralController.HandleReferral)
	salesPerson.GET("/referral/data", salespersonReferralController.GetSalespersonReferralData)
	salesPerson.GET("/referral/commissions", salespersonReferralController.GetReferralCommissions)
}

//when admin  CreateSalesManager by protected.POST("/sales-managers", adminController.CreateSalesManager)and add commission for him then the sales manager  CreateSalesperson by salesManager.POST("/salespersons", salesManagerController.CreateSalesperson) and a commission for him from his (salesmanager) commission. then thee sales person create a compant by salesPerson.POST("/companies", salesPersonController.CreateCompany) then when this company's branch subscribe in a subscription plan by companyGroup.POST("/subscription/:branchId/request", companySubscriptionController.CreateBranchSubscriptionRequest) and the admin approve it by protected.POST("/company/branch-subscription/requests/:id/process", subscriptionController.ProcessBranchSubscriptionRequest) then salesperson should get commission from the price of the subscription by salesPerson.GET("/commission-withdrawal-history", salesPersonController.GetCommissionAndWithdrawalHistory) and sales manager should get commission by salesManager.GET("/commission-withdrawal-history", salesManagerController.GetCommissionAndWithdrawalHistory) and the remaining of subscription price will added to the admin wallet by protected.GET("/wallet", adminController.GetAdminWallet)
