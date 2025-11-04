package controllers

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/HSouheill/barrim_backend/middleware"
	"github.com/HSouheill/barrim_backend/models"
	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type CompanyVoucherController struct {
	DB *mongo.Database
}

func NewCompanyVoucherController(db *mongo.Database) *CompanyVoucherController {
	return &CompanyVoucherController{DB: db}
}

// GetAvailableVouchersForCompany retrieves all active vouchers for companies
func (cvc *CompanyVoucherController) GetAvailableVouchersForCompany(c echo.Context) error {
	collection := cvc.DB.Collection("vouchers")
	ctx := context.Background()

	// Get company info to check their points
	claims := middleware.GetUserFromToken(c)
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		log.Printf("Invalid user ID from token: %s, error: %v", claims.UserID, err)
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	log.Printf("Looking for company with user ID: %s", userID.Hex())

	// Get company's current points - find company by userId field
	companiesCollection := cvc.DB.Collection("companies")
	var company models.Company
	err = companiesCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&company)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			log.Printf("Company not found with user ID: %s", userID.Hex())
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Company not found",
				Data:    "No company document found with the provided user ID",
			})
		}
		log.Printf("Error retrieving company: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve company information",
			Data:    err.Error(),
		})
	}

	// Get vouchers available for companies
	cursor, err := collection.Find(ctx, bson.M{
		"isActive":       true,
		"targetUserType": "company",
	})
	if err != nil {
		log.Printf("Error retrieving vouchers: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve vouchers",
			Data:    err.Error(),
		})
	}
	defer cursor.Close(ctx)

	var vouchers []models.Voucher
	if err = cursor.All(ctx, &vouchers); err != nil {
		log.Printf("Error decoding vouchers: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode vouchers",
			Data:    err.Error(),
		})
	}

	// Create company vouchers with purchase capability info
	var companyVouchers []models.CompanyVoucher
	for _, voucher := range vouchers {
		canPurchase := company.Points >= voucher.Points
		companyVouchers = append(companyVouchers, models.CompanyVoucher{
			Voucher:       voucher,
			CanPurchase:   canPurchase,
			CompanyPoints: company.Points,
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Available vouchers retrieved successfully",
		Data: map[string]interface{}{
			"count":         len(companyVouchers),
			"vouchers":      companyVouchers,
			"companyPoints": company.Points,
		},
	})
}

// PurchaseVoucherForCompany allows a company to purchase a voucher with points
func (cvc *CompanyVoucherController) PurchaseVoucherForCompany(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	var req models.VoucherPurchaseRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
			Data:    err.Error(),
		})
	}

	// Validate request
	validate := validator.New()
	if err := validate.Struct(req); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Validation failed",
			Data:    err.Error(),
		})
	}

	voucherID, err := primitive.ObjectIDFromHex(req.VoucherID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid voucher ID",
		})
	}

	ctx := context.Background()

	// Get the voucher
	vouchersCollection := cvc.DB.Collection("vouchers")
	var voucher models.Voucher
	err = vouchersCollection.FindOne(ctx, bson.M{"_id": voucherID, "isActive": true}).Decode(&voucher)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Voucher not found or inactive",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve voucher",
		})
	}

	// Get company's current points
	companiesCollection := cvc.DB.Collection("companies")
	var company models.Company
	err = companiesCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&company)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Company not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve company information",
		})
	}

	// Check if company has enough points
	if company.Points < voucher.Points {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Insufficient points",
		})
	}

	// Check if company already purchased this voucher
	purchasesCollection := cvc.DB.Collection("company_voucher_purchases")
	var existingPurchase models.CompanyVoucherPurchase
	err = purchasesCollection.FindOne(ctx, bson.M{
		"companyId": company.ID, // Use the actual company ID from the database
		"voucherId": voucherID,
	}).Decode(&existingPurchase)
	if err == nil {
		return c.JSON(http.StatusConflict, models.Response{
			Status:  http.StatusConflict,
			Message: "You have already purchased this voucher",
		})
	}

	// Create purchase record and automatically use the voucher
	// When a company purchases a voucher, it's immediately used (no separate usage step required)
	purchase := models.CompanyVoucherPurchase{
		ID:          primitive.NewObjectID(),
		CompanyID:   company.ID, // Use the actual company ID from the database
		VoucherID:   voucherID,
		PointsUsed:  voucher.Points,
		PurchasedAt: time.Now(),
		IsUsed:      true,       // Automatically mark as used upon purchase
		UsedAt:      time.Now(), // Set usage timestamp to purchase time
	}

	_, err = purchasesCollection.InsertOne(ctx, purchase)
	if err != nil {
		log.Printf("Error creating purchase record: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to create purchase record",
		})
	}

	// Deduct points from company
	_, err = companiesCollection.UpdateByID(ctx, company.ID, bson.M{
		"$inc": bson.M{"points": -voucher.Points},
	})
	if err != nil {
		log.Printf("Error deducting points: %v", err)
		// Note: In a production environment, you might want to implement compensation logic
		// to reverse the purchase if point deduction fails
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to deduct points",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Voucher purchased and used successfully",
	})
}

// GetCompanyVouchers retrieves all vouchers purchased by the current company
func (cvc *CompanyVoucherController) GetCompanyVouchers(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	ctx := context.Background()

	// First get the company to get the actual company ID
	companiesCollection := cvc.DB.Collection("companies")
	var company models.Company
	err = companiesCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&company)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Company not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve company information",
		})
	}

	// Get company's purchased vouchers
	purchasesCollection := cvc.DB.Collection("company_voucher_purchases")
	cursor, err := purchasesCollection.Find(ctx, bson.M{"companyId": company.ID})
	if err != nil {
		log.Printf("Error retrieving company vouchers: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve company vouchers",
			Data:    err.Error(),
		})
	}
	defer cursor.Close(ctx)

	var purchases []models.CompanyVoucherPurchase
	if err = cursor.All(ctx, &purchases); err != nil {
		log.Printf("Error decoding purchases: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode purchases",
			Data:    err.Error(),
		})
	}

	// Get voucher details for each purchase
	var companyVouchers []models.CompanyVoucher
	vouchersCollection := cvc.DB.Collection("vouchers")

	for _, purchase := range purchases {
		var voucher models.Voucher
		err := vouchersCollection.FindOne(ctx, bson.M{"_id": purchase.VoucherID}).Decode(&voucher)
		if err != nil {
			log.Printf("Error retrieving voucher %s: %v", purchase.VoucherID.Hex(), err)
			continue
		}

		companyVouchers = append(companyVouchers, models.CompanyVoucher{
			Voucher:  voucher,
			Purchase: purchase,
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Company vouchers retrieved successfully",
		Data: map[string]interface{}{
			"count":    len(companyVouchers),
			"vouchers": companyVouchers,
		},
	})
}

// UseVoucherForCompany marks a voucher as used by a company
func (cvc *CompanyVoucherController) UseVoucherForCompany(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	purchaseID := c.Param("id")
	objID, err := primitive.ObjectIDFromHex(purchaseID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid purchase ID",
		})
	}

	ctx := context.Background()

	// First get the company to get the actual company ID
	companiesCollection := cvc.DB.Collection("companies")
	var company models.Company
	err = companiesCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&company)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Company not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve company information",
		})
	}

	purchasesCollection := cvc.DB.Collection("company_voucher_purchases")

	// Check if the purchase exists and belongs to the company
	var purchase models.CompanyVoucherPurchase
	err = purchasesCollection.FindOne(ctx, bson.M{
		"_id":       objID,
		"companyId": company.ID,
	}).Decode(&purchase)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Voucher purchase not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve voucher purchase",
			Data:    err.Error(),
		})
	}

	// Check if already used
	if purchase.IsUsed {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Voucher has already been used",
		})
	}

	// Mark as used
	update := bson.M{
		"$set": bson.M{
			"isUsed": true,
			"usedAt": time.Now(),
		},
	}

	_, err = purchasesCollection.UpdateByID(ctx, objID, update)
	if err != nil {
		log.Printf("Error using voucher: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to use voucher",
			Data:    err.Error(),
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Voucher used successfully",
	})
}
