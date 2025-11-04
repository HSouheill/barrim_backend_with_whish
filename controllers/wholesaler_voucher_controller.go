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

type WholesalerVoucherController struct {
	DB *mongo.Database
}

func NewWholesalerVoucherController(db *mongo.Database) *WholesalerVoucherController {
	return &WholesalerVoucherController{DB: db}
}

// GetAvailableVouchersForWholesaler retrieves all active vouchers for wholesalers
func (wvc *WholesalerVoucherController) GetAvailableVouchersForWholesaler(c echo.Context) error {
	collection := wvc.DB.Collection("vouchers")
	ctx := context.Background()

	// Get wholesaler info to check their points
	claims := middleware.GetUserFromToken(c)
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	// Get wholesaler's current points (by userId or createdBy)
	wholesalersCollection := wvc.DB.Collection("wholesalers")
	var wholesaler models.Wholesaler
	err = wholesalersCollection.FindOne(ctx, bson.M{
		"$or": []bson.M{
			{"userId": userID},
			{"createdBy": userID},
		},
	}).Decode(&wholesaler)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Wholesaler not found",
			})
		}
		log.Printf("Error retrieving wholesaler: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve wholesaler information",
			Data:    err.Error(),
		})
	}

	// Get vouchers available for wholesalers
	cursor, err := collection.Find(ctx, bson.M{
		"isActive":       true,
		"targetUserType": "wholesaler",
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

	// Create wholesaler vouchers with purchase capability info
	var wholesalerVouchers []models.WholesalerVoucher
	for _, voucher := range vouchers {
		canPurchase := wholesaler.Points >= voucher.Points
		wholesalerVouchers = append(wholesalerVouchers, models.WholesalerVoucher{
			Voucher:          voucher,
			CanPurchase:      canPurchase,
			WholesalerPoints: wholesaler.Points,
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Available vouchers retrieved successfully",
		Data: map[string]interface{}{
			"count":            len(wholesalerVouchers),
			"vouchers":         wholesalerVouchers,
			"wholesalerPoints": wholesaler.Points,
		},
	})
}

// PurchaseVoucherForWholesaler allows a wholesaler to purchase a voucher with points
func (wvc *WholesalerVoucherController) PurchaseVoucherForWholesaler(c echo.Context) error {
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
	vouchersCollection := wvc.DB.Collection("vouchers")
	var voucher models.Voucher
	err = vouchersCollection.FindOne(ctx, bson.M{"_id": voucherID, "isActive": true}).Decode(&voucher)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Voucher not found or inactive",
			})
		}
		log.Printf("Error retrieving voucher: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve voucher",
			Data:    err.Error(),
		})
	}

	// Get wholesaler's current points (by userId or createdBy)
	wholesalersCollection := wvc.DB.Collection("wholesalers")
	var wholesaler models.Wholesaler
	err = wholesalersCollection.FindOne(ctx, bson.M{
		"$or": []bson.M{
			{"userId": userID},
			{"createdBy": userID},
		},
	}).Decode(&wholesaler)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Wholesaler not found",
			})
		}
		log.Printf("Error retrieving wholesaler: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve wholesaler information",
			Data:    err.Error(),
		})
	}

	// Check if wholesaler has enough points
	if wholesaler.Points < voucher.Points {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Insufficient points",
		})
	}

	// Check if wholesaler already purchased this voucher
	purchasesCollection := wvc.DB.Collection("wholesaler_voucher_purchases")
	var existingPurchase models.WholesalerVoucherPurchase
	err = purchasesCollection.FindOne(ctx, bson.M{
		"wholesalerId": wholesaler.ID,
		"voucherId":    voucherID,
	}).Decode(&existingPurchase)
	if err == nil {
		return c.JSON(http.StatusConflict, models.Response{
			Status:  http.StatusConflict,
			Message: "You have already purchased this voucher",
		})
	}

	// Create purchase record
	purchase := models.WholesalerVoucherPurchase{
		ID:           primitive.NewObjectID(),
		WholesalerID: wholesaler.ID,
		VoucherID:    voucherID,
		PointsUsed:   voucher.Points,
		PurchasedAt:  time.Now(),
		IsUsed:       false,
	}

	// Insert purchase record
	_, err = purchasesCollection.InsertOne(ctx, purchase)
	if err != nil {
		log.Printf("Error creating purchase record: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to create purchase record",
			Data:    err.Error(),
		})
	}

	// Deduct points from wholesaler using atomic operation
	_, err = wholesalersCollection.UpdateByID(ctx, wholesaler.ID, bson.M{
		"$inc": bson.M{"points": -voucher.Points},
	})
	if err != nil {
		log.Printf("Error deducting points: %v", err)
		// Try to rollback the purchase record
		purchasesCollection.DeleteOne(ctx, bson.M{"_id": purchase.ID})
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to deduct points",
			Data:    err.Error(),
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Voucher purchased successfully",
		Data: map[string]interface{}{
			"purchaseId":      purchase.ID.Hex(),
			"pointsUsed":      voucher.Points,
			"remainingPoints": wholesaler.Points - voucher.Points,
		},
	})
}

// GetWholesalerVouchers retrieves all vouchers purchased by the current wholesaler
func (wvc *WholesalerVoucherController) GetWholesalerVouchers(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	ctx := context.Background()

	// Get wholesaler ID (by userId or createdBy)
	wholesalersCollection := wvc.DB.Collection("wholesalers")
	var wholesaler models.Wholesaler
	err = wholesalersCollection.FindOne(ctx, bson.M{
		"$or": []bson.M{
			{"userId": userID},
			{"createdBy": userID},
		},
	}).Decode(&wholesaler)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Wholesaler not found",
			})
		}
		log.Printf("Error retrieving wholesaler: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve wholesaler information",
			Data:    err.Error(),
		})
	}

	wholesalerID := wholesaler.ID

	// Get wholesaler's purchased vouchers
	purchasesCollection := wvc.DB.Collection("wholesaler_voucher_purchases")
	cursor, err := purchasesCollection.Find(ctx, bson.M{"wholesalerId": wholesalerID})
	if err != nil {
		log.Printf("Error retrieving wholesaler vouchers: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve wholesaler vouchers",
			Data:    err.Error(),
		})
	}
	defer cursor.Close(ctx)

	var purchases []models.WholesalerVoucherPurchase
	if err = cursor.All(ctx, &purchases); err != nil {
		log.Printf("Error decoding purchases: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode purchases",
			Data:    err.Error(),
		})
	}

	// Get voucher details for each purchase
	var wholesalerVouchers []models.WholesalerVoucher
	vouchersCollection := wvc.DB.Collection("vouchers")

	for _, purchase := range purchases {
		var voucher models.Voucher
		err := vouchersCollection.FindOne(ctx, bson.M{"_id": purchase.VoucherID}).Decode(&voucher)
		if err != nil {
			log.Printf("Error retrieving voucher %s: %v", purchase.VoucherID.Hex(), err)
			continue
		}

		wholesalerVouchers = append(wholesalerVouchers, models.WholesalerVoucher{
			Voucher:  voucher,
			Purchase: purchase,
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Wholesaler vouchers retrieved successfully",
		Data: map[string]interface{}{
			"count":    len(wholesalerVouchers),
			"vouchers": wholesalerVouchers,
		},
	})
}

// UseVoucherForWholesaler marks a voucher as used by a wholesaler
func (wvc *WholesalerVoucherController) UseVoucherForWholesaler(c echo.Context) error {
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

	// Get wholesaler ID (by userId or createdBy)
	wholesalersCollection := wvc.DB.Collection("wholesalers")
	var wholesaler models.Wholesaler
	err = wholesalersCollection.FindOne(ctx, bson.M{
		"$or": []bson.M{
			{"userId": userID},
			{"createdBy": userID},
		},
	}).Decode(&wholesaler)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Wholesaler not found",
			})
		}
		log.Printf("Error retrieving wholesaler: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve wholesaler information",
			Data:    err.Error(),
		})
	}

	purchasesCollection := wvc.DB.Collection("wholesaler_voucher_purchases")

	// Check if the purchase exists and belongs to the wholesaler
	var purchase models.WholesalerVoucherPurchase
	err = purchasesCollection.FindOne(ctx, bson.M{
		"_id":          objID,
		"wholesalerId": wholesaler.ID,
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
