package controllers

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/HSouheill/barrim_backend/middleware"
	"github.com/HSouheill/barrim_backend/models"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type AdsController struct {
	DB *mongo.Database
}

func NewAdsController(db *mongo.Database) *AdsController {
	return &AdsController{DB: db}
}

// PostAd allows admin to upload an ad image (POST /api/admin/ads)
func (ac *AdsController) PostAd(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "admin" && claims.UserType != "super_admin" {
		return c.JSON(http.StatusForbidden, echo.Map{"message": "Only admins can post ads."})
	}

	file, err := c.FormFile("image")
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"message": "Image file is required."})
	}
	openedFile, err := file.Open()
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"message": "Could not open uploaded image."})
	}
	defer openedFile.Close()

	uuidName := uuid.New().String() + filepath.Ext(file.Filename)
	uploadDir := "uploads/ads/"
	if _, err := os.Stat(uploadDir); os.IsNotExist(err) {
		os.MkdirAll(uploadDir, 0755)
	}
	targetPath := filepath.Join(uploadDir, uuidName)
	outFile, err := os.Create(targetPath)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"message": "Failed to save image"})
	}
	defer outFile.Close()
	if _, err := io.Copy(outFile, openedFile); err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"message": "Failed to store image"})
	}

	// Convert string UserID to ObjectID
	createdByObjID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"message": "Invalid admin user ID."})
	}

	ad := models.Ad{
		ID:        primitive.NewObjectID(),
		ImageURL:  "/" + targetPath, // return as relative API path
		CreatedBy: createdByObjID,
		CreatedAt: time.Now(),
		IsActive:  true,
	}
	ctx := context.Background()
	_, err = ac.DB.Collection("ads").InsertOne(ctx, ad)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"message": "Failed to save ad"})
	}

	return c.JSON(http.StatusOK, echo.Map{"message": "Ad posted successfully", "data": ad})
}

// GetAds fetches all active ads, for users and admins
func (ac *AdsController) GetAds(c echo.Context) error {
	ctx := context.Background()
	findOptions := options.Find().SetSort(bson.M{"createdAt": -1})
	cursor, err := ac.DB.Collection("ads").Find(ctx, bson.M{"isActive": true}, findOptions)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"message": "Failed to fetch ads"})
	}
	defer cursor.Close(ctx)

	var ads []models.Ad
	for cursor.Next(ctx) {
		var ad models.Ad
		if err := cursor.Decode(&ad); err == nil {
			ads = append(ads, ad)
		}
	}
	return c.JSON(http.StatusOK, models.AdsResponse{
		Status:  http.StatusOK,
		Message: "List of ads",
		Data:    ads,
	})
}

// DeleteAd allows admin to delete an ad by ID and remove its image file
func (ac *AdsController) DeleteAd(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "admin" && claims.UserType != "super_admin" {
		return c.JSON(http.StatusForbidden, echo.Map{"message": "Only admins can delete ads."})
	}

	idHex := c.Param("id")
	adID, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"message": "Invalid ad ID."})
	}

	ctx := context.Background()
	var ad models.Ad
	err = ac.DB.Collection("ads").FindOne(ctx, bson.M{"_id": adID}).Decode(&ad)
	if err == mongo.ErrNoDocuments {
		return c.JSON(http.StatusNotFound, echo.Map{"message": "Ad not found."})
	} else if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"message": "Failed to fetch ad."})
	}

	// Delete DB record first
	_, err = ac.DB.Collection("ads").DeleteOne(ctx, bson.M{"_id": adID})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"message": "Failed to delete ad."})
	}

	// Remove image file if stored locally and path looks local
	if ad.ImageURL != "" {
		path := strings.TrimPrefix(ad.ImageURL, "/")
		if strings.HasPrefix(path, "uploads/") {
			_ = os.Remove(path)
		}
	}

	return c.JSON(http.StatusOK, echo.Map{"message": "Ad deleted successfully."})
}
