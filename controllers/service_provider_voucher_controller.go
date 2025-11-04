package controllers

import (
	"context"
	"errors"
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

type ServiceProviderVoucherController struct {
	DB *mongo.Database
}

func NewServiceProviderVoucherController(db *mongo.Database) *ServiceProviderVoucherController {
	return &ServiceProviderVoucherController{DB: db}
}

// GetAvailableVouchersForServiceProvider retrieves all active vouchers for service providers
func (spvc *ServiceProviderVoucherController) GetAvailableVouchersForServiceProvider(c echo.Context) error {
	collection := spvc.DB.Collection("vouchers")
	ctx := context.Background()

	// Get current user id from token
	claims := middleware.GetUserFromToken(c)
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	// Determine points from users collection, with fallback to serviceProviders
	points := 0
	usersCollection := spvc.DB.Collection("users")
	var user models.User
	err = usersCollection.FindOne(ctx, bson.M{"_id": userID, "userType": "serviceProvider"}).Decode(&user)
	if err != nil && err != mongo.ErrNoDocuments {
		log.Printf("Error retrieving user: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve user information",
			Data:    err.Error(),
		})
	}
	if user.ServiceProviderInfo != nil {
		points = user.ServiceProviderInfo.Points
	} else {
		serviceProvidersCollection := spvc.DB.Collection("serviceProviders")
		var serviceProvider models.ServiceProvider
		if user.ServiceProviderID != nil {
			_ = serviceProvidersCollection.FindOne(ctx, bson.M{"_id": user.ServiceProviderID}).Decode(&serviceProvider)
		}
		if serviceProvider.ID.IsZero() {
			_ = serviceProvidersCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&serviceProvider)
		}
		if !serviceProvider.ID.IsZero() {
			if serviceProvider.ServiceProviderInfo != nil && serviceProvider.ServiceProviderInfo.Points > 0 {
				points = serviceProvider.ServiceProviderInfo.Points
			} else {
				points = serviceProvider.Points
			}
		}
	}

	// Get vouchers available for service providers
	cursor, err := collection.Find(ctx, bson.M{
		"isActive":       true,
		"targetUserType": "serviceProvider",
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

	// Create service provider vouchers with purchase capability info
	var serviceProviderVouchers []models.ServiceProviderVoucher
	for _, voucher := range vouchers {
		canPurchase := points >= voucher.Points
		serviceProviderVouchers = append(serviceProviderVouchers, models.ServiceProviderVoucher{
			Voucher:               voucher,
			CanPurchase:           canPurchase,
			ServiceProviderPoints: points,
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Available vouchers retrieved successfully",
		Data: map[string]interface{}{
			"count":                 len(serviceProviderVouchers),
			"vouchers":              serviceProviderVouchers,
			"serviceProviderPoints": points,
		},
	})
}

// PurchaseVoucherForServiceProvider allows a service provider to purchase a voucher with points
func (spvc *ServiceProviderVoucherController) PurchaseVoucherForServiceProvider(c echo.Context) error {
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

	// Get the voucher (must be active)
	vouchersCollection := spvc.DB.Collection("vouchers")
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
			Data:    err.Error(),
		})
	}

	// Resolve the service provider document and current points
	usersCollection := spvc.DB.Collection("users")
	var user models.User
	_ = usersCollection.FindOne(ctx, bson.M{"_id": userID, "userType": "serviceProvider"}).Decode(&user)

	serviceProvidersCollection := spvc.DB.Collection("serviceProviders")
	var serviceProvider models.ServiceProvider
	if user.ServiceProviderID != nil {
		_ = serviceProvidersCollection.FindOne(ctx, bson.M{"_id": user.ServiceProviderID}).Decode(&serviceProvider)
	}
	if serviceProvider.ID.IsZero() {
		_ = serviceProvidersCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&serviceProvider)
	}
	if serviceProvider.ID.IsZero() {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Service provider not found for user",
		})
	}

	currentPoints := serviceProvider.Points
	if serviceProvider.ServiceProviderInfo != nil && serviceProvider.ServiceProviderInfo.Points > 0 {
		currentPoints = serviceProvider.ServiceProviderInfo.Points
	}

	// Check if enough points
	if currentPoints < voucher.Points {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Insufficient points",
		})
	}

	// Check if already purchased
	purchasesCollection := spvc.DB.Collection("service_provider_voucher_purchases")
	var existingPurchase models.ServiceProviderVoucherPurchase
	err = purchasesCollection.FindOne(ctx, bson.M{
		"serviceProviderId": serviceProvider.ID,
		"voucherId":         voucherID,
	}).Decode(&existingPurchase)
	if err == nil {
		return c.JSON(http.StatusConflict, models.Response{
			Status:  http.StatusConflict,
			Message: "You have already purchased this voucher",
		})
	}
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		log.Printf("Error checking existing purchase: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to check existing purchases",
			Data:    err.Error(),
		})
	}

	// Create purchase record
	purchase := models.ServiceProviderVoucherPurchase{
		ID:                primitive.NewObjectID(),
		ServiceProviderID: serviceProvider.ID,
		VoucherID:         voucherID,
		PointsUsed:        voucher.Points,
		PurchasedAt:       time.Now(),
		IsUsed:            false,
	}
	_, err = purchasesCollection.InsertOne(ctx, purchase)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to create voucher purchase",
			Data:    err.Error(),
		})
	}

	// Deduct points from service provider (prefer nested points if using nested model)
	updateFilter := bson.M{"_id": serviceProvider.ID}
	update := bson.M{"$inc": bson.M{"points": -voucher.Points}}
	if serviceProvider.ServiceProviderInfo != nil {
		update = bson.M{"$inc": bson.M{"serviceProviderInfo.points": -voucher.Points}}
	}
	_, err = serviceProvidersCollection.UpdateOne(ctx, updateFilter, update)
	if err != nil {
		log.Printf("Error updating service provider points: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to deduct points",
			Data:    err.Error(),
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Voucher purchased successfully",
	})
}

// GetServiceProviderVouchers retrieves all vouchers purchased by the current service provider
func (spvc *ServiceProviderVoucherController) GetServiceProviderVouchers(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	ctx := context.Background()

	// Resolve service provider ID from user
	usersCollection := spvc.DB.Collection("users")
	var user models.User
	_ = usersCollection.FindOne(ctx, bson.M{"_id": userID, "userType": "serviceProvider"}).Decode(&user)

	serviceProvidersCollection := spvc.DB.Collection("serviceProviders")
	var serviceProvider models.ServiceProvider
	if user.ServiceProviderID != nil {
		_ = serviceProvidersCollection.FindOne(ctx, bson.M{"_id": user.ServiceProviderID}).Decode(&serviceProvider)
	}
	if serviceProvider.ID.IsZero() {
		_ = serviceProvidersCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&serviceProvider)
	}
	if serviceProvider.ID.IsZero() {
		return c.JSON(http.StatusOK, models.Response{
			Status:  http.StatusOK,
			Message: "Service provider vouchers retrieved successfully",
			Data: map[string]interface{}{
				"count":    0,
				"vouchers": []models.ServiceProviderVoucher{},
			},
		})
	}

	// Get service provider's purchased vouchers
	purchasesCollection := spvc.DB.Collection("service_provider_voucher_purchases")
	cursor, err := purchasesCollection.Find(ctx, bson.M{"serviceProviderId": serviceProvider.ID})
	if err != nil {
		log.Printf("Error retrieving service provider vouchers: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve service provider vouchers",
			Data:    err.Error(),
		})
	}
	defer cursor.Close(ctx)

	var purchases []models.ServiceProviderVoucherPurchase
	if err = cursor.All(ctx, &purchases); err != nil {
		log.Printf("Error decoding purchases: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode purchases",
			Data:    err.Error(),
		})
	}

	// Get voucher details for each purchase
	var serviceProviderVouchers []models.ServiceProviderVoucher
	vouchersCollection := spvc.DB.Collection("vouchers")

	for _, purchase := range purchases {
		var voucher models.Voucher
		err := vouchersCollection.FindOne(ctx, bson.M{"_id": purchase.VoucherID}).Decode(&voucher)
		if err != nil {
			log.Printf("Error retrieving voucher %s: %v", purchase.VoucherID.Hex(), err)
			continue
		}

		serviceProviderVouchers = append(serviceProviderVouchers, models.ServiceProviderVoucher{
			Voucher:  voucher,
			Purchase: purchase,
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Service provider vouchers retrieved successfully",
		Data: map[string]interface{}{
			"count":    len(serviceProviderVouchers),
			"vouchers": serviceProviderVouchers,
		},
	})
}

// UseVoucherForServiceProvider marks a voucher as used by a service provider
func (spvc *ServiceProviderVoucherController) UseVoucherForServiceProvider(c echo.Context) error {
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
	purchasesCollection := spvc.DB.Collection("service_provider_voucher_purchases")

	// Resolve service provider ID from user
	usersCollection := spvc.DB.Collection("users")
	var user models.User
	_ = usersCollection.FindOne(ctx, bson.M{"_id": userID, "userType": "serviceProvider"}).Decode(&user)

	serviceProvidersCollection := spvc.DB.Collection("serviceProviders")
	var serviceProvider models.ServiceProvider
	if user.ServiceProviderID != nil {
		_ = serviceProvidersCollection.FindOne(ctx, bson.M{"_id": user.ServiceProviderID}).Decode(&serviceProvider)
	}
	if serviceProvider.ID.IsZero() {
		_ = serviceProvidersCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&serviceProvider)
	}
	if serviceProvider.ID.IsZero() {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Service provider not found",
		})
	}

	// Check if the purchase exists and belongs to the service provider
	var purchase models.ServiceProviderVoucherPurchase
	err = purchasesCollection.FindOne(ctx, bson.M{
		"_id":               objID,
		"serviceProviderId": serviceProvider.ID,
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
