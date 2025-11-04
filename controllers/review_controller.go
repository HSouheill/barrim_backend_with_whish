// controllers/review_controller.go
package controllers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"math"

	"github.com/HSouheill/barrim_backend/middleware"
	"github.com/HSouheill/barrim_backend/models"
	"github.com/HSouheill/barrim_backend/utils"
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type ReviewController struct {
	db *mongo.Client
}

func NewReviewController(db *mongo.Client) *ReviewController {
	return &ReviewController{db: db}
}

// GetReviewsByProviderID retrieves all reviews for a specific service provider
func (rc *ReviewController) GetReviewsByProviderID(c echo.Context) error {
	userID := c.Param("userid")

	// Validate user ID
	objectID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	// Create context
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Find reviews for this provider by user ID
	reviewsCollection := rc.db.Database("barrim").Collection("reviews")

	// Get reviews sorted by most recent first
	findOptions := options.Find()
	findOptions.SetSort(bson.D{{Key: "createdAt", Value: -1}})

	cursor, err := reviewsCollection.Find(ctx, bson.M{"serviceProviderId": objectID}, findOptions)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Error fetching reviews",
		})
	}
	defer cursor.Close(ctx)

	var reviews []models.Review
	if err := cursor.All(ctx, &reviews); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Error parsing reviews",
		})
	}

	return c.JSON(http.StatusOK, models.ReviewsResponse{
		Status:  http.StatusOK,
		Message: "Reviews retrieved successfully",
		Data:    reviews,
	})
}

// CreateReview adds a new review for a service provider
func (rc *ReviewController) CreateReview(c echo.Context) error {
	// Get user from JWT token
	user, err := utils.GetUserFromToken(c, rc.db)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Unauthorized",
		})
	}

	// Parse multipart form data
	if err := c.Request().ParseMultipartForm(10 << 20); err != nil { // 10 MB max
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Failed to parse form data",
		})
	}

	// Get form values
	serviceProviderID := c.FormValue("serviceProviderId")
	ratingStr := c.FormValue("rating")
	comment := c.FormValue("comment")
	mediaType := c.FormValue("mediaType") // "image" or "video"

	// Validate required fields
	if serviceProviderID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Service provider ID is required",
		})
	}

	// Parse rating
	rating, err := strconv.Atoi(ratingStr)
	if err != nil || rating < 1 || rating > 5 {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Rating must be between 1 and 5",
		})
	}

	// Validate provider ID
	providerID, err := primitive.ObjectIDFromHex(serviceProviderID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid provider ID",
		})
	}

	// Handle media upload if present
	var mediaURL, thumbnailURL string
	if mediaType != "" {
		// Validate media type
		if mediaType != "image" && mediaType != "video" {
			return c.JSON(http.StatusBadRequest, models.Response{
				Status:  http.StatusBadRequest,
				Message: "Invalid media type. Must be 'image' or 'video'",
			})
		}

		// Get media file
		file, err := c.FormFile("mediaFile")
		if err != nil {
			return c.JSON(http.StatusBadRequest, models.Response{
				Status:  http.StatusBadRequest,
				Message: "Media file is required when mediaType is specified",
			})
		}

		// Read file data
		src, err := file.Open()
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to read uploaded file",
			})
		}
		defer src.Close()

		// Read file into byte slice
		fileData := make([]byte, file.Size)
		_, err = src.Read(fileData)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to read file data",
			})
		}

		// Generate unique filename
		timestamp := time.Now().Unix()
		uniqueID := primitive.NewObjectID().Hex()
		fileExt := filepath.Ext(file.Filename)
		if fileExt == "" {
			if mediaType == "image" {
				fileExt = ".jpg"
			} else {
				fileExt = ".mp4"
			}
		}
		filename := fmt.Sprintf("reviews/%s/%d_%s%s",
			user.ID.Hex(),
			timestamp,
			uniqueID,
			fileExt,
		)

		// Upload file
		mediaURL, err = utils.UploadFile(fileData, filename, mediaType)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: fmt.Sprintf("Failed to upload media file: %v", err),
			})
		}

		// Generate thumbnail for videos
		if mediaType == "video" {
			thumbnailURL, err = utils.GenerateVideoThumbnail(mediaURL)
			if err != nil {
				log.Printf("Failed to generate video thumbnail: %v", err)
				thumbnailURL = ""
			}
		}
	}

	// Create review
	now := time.Now()
	newReview := models.Review{
		ID:                primitive.NewObjectID(),
		ServiceProviderID: providerID, // This will be the user ID for service providers going forward
		UserID:            user.ID,
		Username:          user.FullName,
		UserProfilePic:    user.ProfilePic,
		Rating:            rating,
		Comment:           comment,
		MediaType:         mediaType,
		MediaURL:          mediaURL,
		ThumbnailURL:      thumbnailURL,
		IsVerified:        false, // Default to false, can be updated by admin
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	// Insert review into database
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	reviewsCollection := rc.db.Database("barrim").Collection("reviews")
	_, err = reviewsCollection.InsertOne(ctx, newReview)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Error creating review",
		})
	}

	// Send notification to the service provider (in-app + FCM)
	go func() {
		title := "You have a new review"
		message := fmt.Sprintf("%s left a new review: %s", user.FullName, comment)
		notifType := "new_review"
		data := map[string]interface{}{
			"reviewId":   newReview.ID.Hex(),
			"reviewerId": user.ID.Hex(),
		}
		_ = utils.SaveNotification(rc.db, providerID, title, message, notifType, data)
		_ = utils.SendFCMNotificationToServiceProvider(rc.db, providerID, title, message, data)
	}()

	// Update service provider average rating
	go rc.updateProviderRating(providerID)

	return c.JSON(http.StatusCreated, models.ReviewResponse{
		Status:  http.StatusCreated,
		Message: "Review created successfully",
		Data:    &newReview,
	})
}

// updateProviderRating calculates and updates the average rating for a service provider
func (rc *ReviewController) updateProviderRating(providerID primitive.ObjectID) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Calculate average rating
	reviewsCollection := rc.db.Database("barrim").Collection("reviews")

	// Pipeline to calculate average rating
	pipeline := []bson.M{
		{"$match": bson.M{"serviceProviderId": providerID}},
		{"$group": bson.M{
			"_id":           nil,
			"averageRating": bson.M{"$avg": "$rating"},
			"count":         bson.M{"$sum": 1},
		}},
	}

	cursor, err := reviewsCollection.Aggregate(ctx, pipeline)
	if err != nil {
		return
	}
	defer cursor.Close(ctx)

	// Get aggregation result
	var results []bson.M
	if err := cursor.All(ctx, &results); err != nil || len(results) == 0 {
		return
	}

	// Extract average rating
	avgRating := results[0]["averageRating"].(float64)

	// Update service provider info
	usersCollection := rc.db.Database("barrim").Collection("users")
	_, err = usersCollection.UpdateOne(
		ctx,
		bson.M{"_id": providerID},
		bson.M{
			"$set": bson.M{
				"serviceProviderInfo.rating": avgRating,
				"updatedAt":                  time.Now(),
			},
		},
	)
}

func (rc *ReviewController) PostReviewReply(c echo.Context) error {
	reviewID := c.Param("id")
	spUser, err := utils.GetUserFromToken(c, rc.db)
	if err != nil || spUser.UserType != "serviceProvider" {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Only service providers can reply to reviews",
		})
	}

	// Parse reply body
	type ReplyRequest struct {
		ReplyText string `json:"replyText"`
	}
	var req ReplyRequest
	if err := c.Bind(&req); err != nil || req.ReplyText == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Reply text is required",
		})
	}

	// Find the review
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	reviewsCollection := rc.db.Database("barrim").Collection("reviews")
	objID, err := primitive.ObjectIDFromHex(reviewID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid review ID",
		})
	}
	var review models.Review
	err = reviewsCollection.FindOne(ctx, bson.M{"_id": objID}).Decode(&review)
	if err != nil {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Review not found",
		})
	}

	// Unified ID validation: Check if the review belongs to this service provider
	isOwner, err := utils.IsServiceProviderOwner(spUser.ID, review.ServiceProviderID, rc.db)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Error validating service provider ownership",
		})
	}
	if !isOwner {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "You can only reply to reviews for your own service provider account",
		})
	}
	if review.Reply != nil {
		return c.JSON(http.StatusConflict, models.Response{
			Status:  http.StatusConflict,
			Message: "This review already has a reply",
		})
	}

	reply := &models.ReviewReply{
		ServiceProviderID: spUser.ID,
		ReplyText:         req.ReplyText,
		CreatedAt:         time.Now(),
	}
	_, err = reviewsCollection.UpdateOne(ctx, bson.M{"_id": objID}, bson.M{"$set": bson.M{"reply": reply, "updatedAt": time.Now()}})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to save reply",
		})
	}

	// Send notification to the review's original author (receiver) - in-app + FCM
	go func() {
		title := "Your review received a reply"
		// Truncate reply text if too long for notification
		replyPreview := req.ReplyText
		if len(replyPreview) > 100 {
			replyPreview = replyPreview[:100] + "..."
		}
		message := fmt.Sprintf("%s replied to your review: %s", spUser.FullName, replyPreview)
		notifType := "review_reply"
		data := map[string]interface{}{
			"reviewId":            review.ID.Hex(),
			"serviceProviderId":   spUser.ID.Hex(),
			"serviceProviderName": spUser.FullName,
		}
		// Save in-app notification
		if err := utils.SaveNotification(rc.db, review.UserID, title, message, notifType, data); err != nil {
			log.Printf("Failed to save notification for review reply: %v", err)
		}
		// Send FCM push notification
		if err := utils.SendFCMNotificationToUser(rc.db, review.UserID, title, message, data); err != nil {
			log.Printf("Failed to send FCM notification for review reply: %v", err)
		}
	}()

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Reply posted successfully",
		Data:    reply,
	})
}

// GetReviewReply allows the review's user or the service provider to get the reply
func (rc *ReviewController) GetReviewReply(c echo.Context) error {
	reviewID := c.Param("id")
	user, err := utils.GetUserFromToken(c, rc.db)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, models.Response{
			Status:  http.StatusUnauthorized,
			Message: "Unauthorized",
		})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	reviewsCollection := rc.db.Database("barrim").Collection("reviews")
	objID, err := primitive.ObjectIDFromHex(reviewID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid review ID",
		})
	}
	var review models.Review
	err = reviewsCollection.FindOne(ctx, bson.M{"_id": objID}).Decode(&review)
	if err != nil {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Review not found",
		})
	}
	// Unified ID validation: Allow access if user is the reviewer or the service provider
	if user.ID != review.UserID {
		// Check if user is the service provider owner
		isOwner, err := utils.IsServiceProviderOwner(user.ID, review.ServiceProviderID, rc.db)
		if err != nil || !isOwner {
			return c.JSON(http.StatusForbidden, models.Response{
				Status:  http.StatusForbidden,
				Message: "You are not allowed to view this reply",
			})
		}
	}
	if review.Reply == nil {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "No reply for this review",
		})
	}
	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Reply retrieved successfully",
		Data:    review.Reply,
	})
}

// GetAllReviewsAndRepliesForAdmin retrieves all reviews and replies for admin dashboard
func (rc *ReviewController) GetAllReviewsAndRepliesForAdmin(c echo.Context) error {
	// Check if user is admin
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins can access this resource",
		})
	}

	// Get pagination parameters
	page, _ := strconv.Atoi(c.QueryParam("page"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if page <= 0 {
		page = 1
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Get filter parameters
	serviceProviderID := c.QueryParam("serviceProviderId")
	hasReply := c.QueryParam("hasReply")     // "true", "false", or empty for all
	rating := c.QueryParam("rating")         // specific rating (1-5) or empty for all
	isVerified := c.QueryParam("isVerified") // "true", "false", or empty for all

	// Create context
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Build filter
	filter := bson.M{}

	// Filter by service provider ID if provided
	if serviceProviderID != "" {
		providerObjID, err := primitive.ObjectIDFromHex(serviceProviderID)
		if err != nil {
			return c.JSON(http.StatusBadRequest, models.Response{
				Status:  http.StatusBadRequest,
				Message: "Invalid service provider ID",
			})
		}
		filter["serviceProviderId"] = providerObjID
	}

	// Filter by reply status
	if hasReply == "true" {
		filter["reply"] = bson.M{"$exists": true, "$ne": nil}
	} else if hasReply == "false" {
		filter["reply"] = bson.M{"$exists": false}
	}

	// Filter by rating
	if rating != "" {
		ratingInt, err := strconv.Atoi(rating)
		if err != nil || ratingInt < 1 || ratingInt > 5 {
			return c.JSON(http.StatusBadRequest, models.Response{
				Status:  http.StatusBadRequest,
				Message: "Invalid rating value. Must be between 1 and 5",
			})
		}
		filter["rating"] = ratingInt
	}

	// Filter by verification status
	if isVerified == "true" {
		filter["isVerified"] = true
	} else if isVerified == "false" {
		filter["isVerified"] = false
	}

	// Calculate skip for pagination
	skip := (page - 1) * limit

	// Get reviews with pagination and sorting
	reviewsCollection := rc.db.Database("barrim").Collection("reviews")

	findOptions := options.Find().
		SetSort(bson.D{{Key: "createdAt", Value: -1}}).
		SetSkip(int64(skip)).
		SetLimit(int64(limit))

	cursor, err := reviewsCollection.Find(ctx, filter, findOptions)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Error fetching reviews",
		})
	}
	defer cursor.Close(ctx)

	var reviews []models.Review
	if err := cursor.All(ctx, &reviews); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Error parsing reviews",
		})
	}

	// Get total count for pagination
	totalCount, err := reviewsCollection.CountDocuments(ctx, filter)
	if err != nil {
		log.Printf("Error counting reviews: %v", err)
		totalCount = 0
	}

	// Enrich reviews with additional information
	var enrichedReviews []map[string]interface{}
	for _, review := range reviews {
		// Get service provider information
		var serviceProvider models.User
		err := rc.db.Database("barrim").Collection("users").FindOne(ctx, bson.M{"_id": review.ServiceProviderID}).Decode(&serviceProvider)
		if err != nil {
			log.Printf("Error fetching service provider info for review %s: %v", review.ID.Hex(), err)
		}

		// Get user information
		var user models.User
		err = rc.db.Database("barrim").Collection("users").FindOne(ctx, bson.M{"_id": review.UserID}).Decode(&user)
		if err != nil {
			log.Printf("Error fetching user info for review %s: %v", review.ID.Hex(), err)
		}

		enrichedReview := map[string]interface{}{
			"review": review,
			"serviceProvider": map[string]interface{}{
				"id":       serviceProvider.ID,
				"fullName": serviceProvider.FullName,
				"email":    serviceProvider.Email,
				"phone":    serviceProvider.Phone,
			},
			"user": map[string]interface{}{
				"id":       user.ID,
				"fullName": user.FullName,
				"email":    user.Email,
				"phone":    user.Phone,
			},
		}

		enrichedReviews = append(enrichedReviews, enrichedReview)
	}

	// Calculate statistics
	stats, err := rc.getReviewStatistics(ctx, filter)
	if err != nil {
		log.Printf("Error calculating review statistics: %v", err)
		stats = map[string]interface{}{
			"totalReviews":       0,
			"averageRating":      0.0,
			"reviewsWithReplies": 0,
			"verifiedReviews":    0,
		}
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Reviews and replies retrieved successfully",
		Data: map[string]interface{}{
			"reviews": enrichedReviews,
			"pagination": map[string]interface{}{
				"page":       page,
				"limit":      limit,
				"totalCount": totalCount,
				"totalPages": int(math.Ceil(float64(totalCount) / float64(limit))),
			},
			"statistics": stats,
			"filters": map[string]interface{}{
				"serviceProviderId": serviceProviderID,
				"hasReply":          hasReply,
				"rating":            rating,
				"isVerified":        isVerified,
			},
		},
	})
}

// getReviewStatistics calculates statistics for reviews
func (rc *ReviewController) getReviewStatistics(ctx context.Context, filter bson.M) (map[string]interface{}, error) {
	reviewsCollection := rc.db.Database("barrim").Collection("reviews")

	// Get total reviews count
	totalReviews, err := reviewsCollection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Calculate average rating
	pipeline := []bson.M{
		{"$match": filter},
		{"$group": bson.M{
			"_id":           nil,
			"averageRating": bson.M{"$avg": "$rating"},
			"totalReviews":  bson.M{"$sum": 1},
		}},
	}

	cursor, err := reviewsCollection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []bson.M
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}

	var averageRating float64
	if len(results) > 0 {
		if avg, exists := results[0]["averageRating"]; exists && avg != nil {
			averageRating = avg.(float64)
		}
	}

	// Count reviews with replies
	replyFilter := bson.M{}
	for key, value := range filter {
		replyFilter[key] = value
	}
	replyFilter["reply"] = bson.M{"$exists": true, "$ne": nil}
	reviewsWithReplies, err := reviewsCollection.CountDocuments(ctx, replyFilter)
	if err != nil {
		return nil, err
	}

	// Count verified reviews
	verifiedFilter := bson.M{}
	for key, value := range filter {
		verifiedFilter[key] = value
	}
	verifiedFilter["isVerified"] = true
	verifiedReviews, err := reviewsCollection.CountDocuments(ctx, verifiedFilter)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"totalReviews":       totalReviews,
		"averageRating":      averageRating,
		"reviewsWithReplies": reviewsWithReplies,
		"verifiedReviews":    verifiedReviews,
	}, nil
}

// ToggleReviewVerification allows admin to toggle the verification status of a review
func (rc *ReviewController) ToggleReviewVerification(c echo.Context) error {
	// Check if user is admin
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins can verify reviews",
		})
	}

	reviewID := c.Param("id")
	if reviewID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Review ID is required",
		})
	}

	objID, err := primitive.ObjectIDFromHex(reviewID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid review ID format",
		})
	}

	// Parse request body
	var req struct {
		IsVerified bool `json:"isVerified"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}

	// Update review verification status
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	reviewsCollection := rc.db.Database("barrim").Collection("reviews")
	result, err := reviewsCollection.UpdateOne(
		ctx,
		bson.M{"_id": objID},
		bson.M{
			"$set": bson.M{
				"isVerified": req.IsVerified,
				"updatedAt":  time.Now(),
			},
		},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update review verification status",
		})
	}

	if result.MatchedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Review not found",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: fmt.Sprintf("Review verification status updated to %v", req.IsVerified),
	})
}

// DeleteReview allows admin to delete a review
func (rc *ReviewController) DeleteReview(c echo.Context) error {
	// Check if user is admin
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins can delete reviews",
		})
	}

	reviewID := c.Param("id")
	if reviewID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Review ID is required",
		})
	}

	objID, err := primitive.ObjectIDFromHex(reviewID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid review ID format",
		})
	}

	// Get review details before deletion for media cleanup
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	reviewsCollection := rc.db.Database("barrim").Collection("reviews")
	var review models.Review
	err = reviewsCollection.FindOne(ctx, bson.M{"_id": objID}).Decode(&review)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Review not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch review",
		})
	}

	// Delete the review
	result, err := reviewsCollection.DeleteOne(ctx, bson.M{"_id": objID})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to delete review",
		})
	}

	if result.DeletedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Review not found",
		})
	}

	// Update service provider average rating after deletion
	go rc.updateProviderRating(review.ServiceProviderID)

	// TODO: Clean up media files if needed
	// if review.MediaURL != "" {
	//     utils.DeleteFile(review.MediaURL)
	// }
	// if review.ThumbnailURL != "" {
	//     utils.DeleteFile(review.ThumbnailURL)
	// }

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Review deleted successfully",
	})
}
