// controllers/subscription_controller.go
package controllers

import (
	"context"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/HSouheill/barrim_backend/middleware"
	"github.com/HSouheill/barrim_backend/models"
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"gopkg.in/gomail.v2"
)

// SubscriptionController handles subscription-related operations
type SubscriptionController struct {
	DB *mongo.Database
}

// NewSubscriptionController creates a new subscription controller
func NewSubscriptionController(db *mongo.Database) *SubscriptionController {
	return &SubscriptionController{DB: db}
}

// GetSubscriptionPlans retrieves all available subscription plans
func (sc *SubscriptionController) GetSubscriptionPlans(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	collection := sc.DB.Collection("subscription_plans")
	cursor, err := collection.Find(ctx, bson.M{})
	if err != nil {
		log.Printf("Error finding subscription plans: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve subscription plans",
			Data:    []models.SubscriptionPlan{},
		})
	}
	defer cursor.Close(ctx)

	var rawPlans []bson.M
	if err = cursor.All(ctx, &rawPlans); err != nil {
		log.Printf("Error decoding raw subscription plans: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve subscription plans",
			Data:    []models.SubscriptionPlan{},
		})
	}

	log.Printf("Found %d subscription plans", len(rawPlans))
	for i, rawPlan := range rawPlans {
		log.Printf("Plan %d raw data: %+v", i, rawPlan)
	}

	var plans []models.SubscriptionPlan = []models.SubscriptionPlan{}
	for _, rawPlan := range rawPlans {
		var plan models.SubscriptionPlan
		bytes, err := bson.Marshal(rawPlan)
		if err != nil {
			log.Printf("Error marshaling plan %+v: %v", rawPlan, err)
			continue
		}
		if err := bson.Unmarshal(bytes, &plan); err != nil {
			log.Printf("Error unmarshaling plan %+v: %v", rawPlan, err)
			continue
		}

		// --- Normalize benefits to always be a flat list of {title, description} ---
		var flatBenefits []map[string]string
		benefitsRaw := plan.Benefits.Value
		if benefitsRaw != nil {
			switch v := benefitsRaw.(type) {
			case []interface{}:
				// Try to parse as a list of maps
				for _, item := range v {
					if m, ok := item.(map[string]interface{}); ok {
						benefit := map[string]string{
							"title":       "",
							"description": "",
						}
						if title, ok := m["title"].(string); ok {
							benefit["title"] = title
						}
						if desc, ok := m["description"].(string); ok {
							benefit["description"] = desc
						}
						flatBenefits = append(flatBenefits, benefit)
					} else if m, ok := item.(map[string]string); ok {
						flatBenefits = append(flatBenefits, map[string]string{
							"title":       m["title"],
							"description": m["description"],
						})
					}
				}
			case map[string]interface{}:
				// Try to parse as a map with 'value' key
				if value, ok := v["value"]; ok {
					switch vv := value.(type) {
					case []interface{}:
						for _, group := range vv {
							if groupList, ok := group.([]interface{}); ok {
								var title, description string
								for _, item := range groupList {
									if m, ok := item.(map[string]interface{}); ok {
										if k, ok := m["Key"].(string); ok {
											if k == "title" {
												title, _ = m["Value"].(string)
											}
											if k == "description" {
												description, _ = m["Value"].(string)
											}
										}
									}
								}
								flatBenefits = append(flatBenefits, map[string]string{
									"title":       title,
									"description": description,
								})
							}
						}
					}
				}
			}
		}
		// Always set to a list, even if empty
		if flatBenefits == nil {
			flatBenefits = []map[string]string{}
		}
		plan.Benefits = models.Benefits{Value: flatBenefits}
		plans = append(plans, plan)
	}

	if len(plans) == 0 {
		log.Printf("No valid subscription plans found after decoding")
		return c.JSON(http.StatusOK, models.Response{
			Status:  http.StatusOK,
			Message: "No subscription plans found",
			Data:    []models.SubscriptionPlan{},
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Subscription plans retrieved successfully",
		Data:    plans,
	})
}

// saveUploadedFile saves an uploaded file to the specified directory
func (sc *SubscriptionController) saveUploadedFile(file *multipart.FileHeader, directory string) (string, error) {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(directory, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	// Generate unique filename
	ext := filepath.Ext(file.Filename)
	filename := primitive.NewObjectID().Hex() + ext
	filepath := filepath.Join(directory, filename)

	// Open source file
	src, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("failed to open uploaded file: %w", err)
	}
	defer src.Close()

	// Create destination file
	dst, err := os.Create(filepath)
	if err != nil {
		return "", fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dst.Close()

	// Copy file contents
	if _, err = io.Copy(dst, src); err != nil {
		return "", fmt.Errorf("failed to copy file contents: %w", err)
	}

	return filepath, nil
}

// CreateCompanySubscription creates a new subscription request for a company
func (sc *SubscriptionController) CreateCompanySubscription(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user information from token
	claims := middleware.GetUserFromToken(c)
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	// Parse multipart form
	if err := c.Request().ParseMultipartForm(10 << 20); err != nil { // 10 MB max
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Failed to parse form data",
		})
	}

	// Get plan ID from form
	planID := c.FormValue("planId")
	if planID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Plan ID is required",
		})
	}

	// Convert plan ID to ObjectID
	planObjectID, err := primitive.ObjectIDFromHex(planID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid plan ID format",
		})
	}

	// Handle image upload
	var imagePath string
	if file, err := c.FormFile("image"); err == nil {
		// Validate file type
		contentType := file.Header.Get("Content-Type")
		if !strings.HasPrefix(contentType, "image/") {
			return c.JSON(http.StatusBadRequest, models.Response{
				Status:  http.StatusBadRequest,
				Message: "Only image files are allowed",
			})
		}

		// Save the uploaded file
		imagePath, err = sc.saveUploadedFile(file, "uploads/plans")
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to save uploaded image",
			})
		}
	}

	// Find company by user ID
	companyCollection := sc.DB.Collection("companies")
	var company models.Company
	err = companyCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&company)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Company not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find company",
		})
	}

	// Check if there's already a pending subscription request
	subscriptionRequestsCollection := sc.DB.Collection("subscription_requests")
	var existingRequest models.SubscriptionRequest
	err = subscriptionRequestsCollection.FindOne(ctx, bson.M{
		"companyId": company.ID,
		"status":    "pending",
	}).Decode(&existingRequest)

	if err == nil {
		return c.JSON(http.StatusConflict, models.Response{
			Status:  http.StatusConflict,
			Message: "You already have a pending subscription request",
		})
	} else if err != mongo.ErrNoDocuments {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Error checking existing subscription requests",
		})
	}

	// Create subscription request
	subscriptionRequest := models.SubscriptionRequest{
		ID:          primitive.NewObjectID(),
		CompanyID:   company.ID,
		PlanID:      planObjectID,
		Status:      "pending",
		RequestedAt: time.Now(),
		ImagePath:   imagePath, // Add the image path to the request
	}

	// Save subscription request
	_, err = subscriptionRequestsCollection.InsertOne(ctx, subscriptionRequest)
	if err != nil {
		// If saving to database fails, delete the uploaded image
		if imagePath != "" {
			os.Remove(imagePath)
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to create subscription request",
		})
	}

	// Send email notification to admin
	if err := sc.sendAdminNotificationEmail(
		"New Subscription Request",
		fmt.Sprintf("Company %s has requested a subscription. Please review the request.", company.BusinessName),
	); err != nil {
		log.Printf("Failed to send admin notification email: %v", err)
	}

	return c.JSON(http.StatusCreated, models.Response{
		Status:  http.StatusCreated,
		Message: "Subscription request created successfully",
		Data:    subscriptionRequest,
	})
}

// GetPendingSubscriptionRequests retrieves all pending subscription requests (admin only)
func (sc *SubscriptionController) GetPendingSubscriptionRequests(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user information from token
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins can access this endpoint",
		})
	}

	// Find all pending subscription requests
	subscriptionRequestsCollection := sc.DB.Collection("subscription_requests")
	cursor, err := subscriptionRequestsCollection.Find(ctx, bson.M{"status": "pending"})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve subscription requests",
		})
	}
	defer cursor.Close(ctx)

	var requests []models.SubscriptionRequest
	if err = cursor.All(ctx, &requests); err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to decode subscription requests",
		})
	}

	// Get company and plan details for each request
	var enrichedRequests []map[string]interface{}
	for _, req := range requests {
		// Get company details
		var company models.Company
		err = sc.DB.Collection("companies").FindOne(ctx, bson.M{"_id": req.CompanyID}).Decode(&company)
		if err != nil {
			log.Printf("Error getting company details: %v", err)
			continue
		}

		// Get plan details
		var plan models.SubscriptionPlan
		err = sc.DB.Collection("subscription_plans").FindOne(ctx, bson.M{"_id": req.PlanID}).Decode(&plan)
		if err != nil {
			log.Printf("Error getting plan details: %v", err)
			continue
		}

		enrichedRequests = append(enrichedRequests, map[string]interface{}{
			"request": req,
			"company": map[string]interface{}{
				"id":           company.ID,
				"businessName": company.BusinessName,
				"category":     company.Category,
			},
			"plan": plan,
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Pending subscription requests retrieved successfully",
		Data:    enrichedRequests,
	})
}

// ProcessSubscriptionRequest handles the approval or rejection of a subscription request (admin only)
func (sc *SubscriptionController) ProcessSubscriptionRequest(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user information from token
	claims := middleware.GetUserFromToken(c)
	if claims.UserType != "admin" {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins can access this endpoint",
		})
	}

	// Get request ID from URL parameter
	requestID := c.Param("id")
	if requestID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Request ID is required",
		})
	}

	// Convert request ID to ObjectID
	requestObjectID, err := primitive.ObjectIDFromHex(requestID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request ID format",
		})
	}

	// Parse approval request body
	var approvalReq models.SubscriptionApprovalRequest
	if err := c.Bind(&approvalReq); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}

	// Validate status
	if approvalReq.Status != "approved" && approvalReq.Status != "rejected" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid status. Must be 'approved' or 'rejected'",
		})
	}

	// Get the subscription request
	subscriptionRequestsCollection := sc.DB.Collection("subscription_requests")
	var subscriptionRequest models.SubscriptionRequest
	err = subscriptionRequestsCollection.FindOne(ctx, bson.M{"_id": requestObjectID}).Decode(&subscriptionRequest)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Subscription request not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find subscription request",
		})
	}

	// Check if request is already processed
	if subscriptionRequest.Status != "pending" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Subscription request is already processed",
		})
	}

	// Update subscription request
	update := bson.M{
		"$set": bson.M{
			"status":      approvalReq.Status,
			"adminId":     claims.UserID,
			"adminNote":   approvalReq.AdminNote,
			"processedAt": time.Now(),
		},
	}

	_, err = subscriptionRequestsCollection.UpdateOne(ctx, bson.M{"_id": requestObjectID}, update)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update subscription request",
		})
	}

	// If approved, create the subscription
	if approvalReq.Status == "approved" {
		// Get plan details to calculate end date
		var plan models.SubscriptionPlan
		err = sc.DB.Collection("subscription_plans").FindOne(ctx, bson.M{"_id": subscriptionRequest.PlanID}).Decode(&plan)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to get plan details",
			})
		}

		// Calculate end date based on plan duration
		startDate := time.Now()
		var endDate time.Time
		switch plan.Duration {
		case 1: // Monthly
			endDate = startDate.AddDate(0, 1, 0)
		case 6: // 6 Months
			endDate = startDate.AddDate(0, 6, 0)
		case 12: // 1 Year
			endDate = startDate.AddDate(1, 0, 0)
		default:
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Invalid plan duration",
			})
		}

		// Create subscription
		subscription := models.BranchSubscription{
			ID:        primitive.NewObjectID(),
			BranchID:  subscriptionRequest.CompanyID,
			PlanID:    subscriptionRequest.PlanID,
			StartDate: startDate,
			EndDate:   endDate,
			Status:    "active",
			AutoRenew: false, // Default to false, can be changed later
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		// Save subscription
		subscriptionsCollection := sc.DB.Collection("branch_subscriptions")
		_, err = subscriptionsCollection.InsertOne(ctx, subscription)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to create subscription",
			})
		}

		// Get company details for commission calculation and notification
		var company models.Company
		err = sc.DB.Collection("companies").FindOne(ctx, bson.M{"_id": subscriptionRequest.CompanyID}).Decode(&company)
		if err == nil {
			// Update user status to active when subscription is approved
			usersCollection := sc.DB.Collection("users")
			_, err = usersCollection.UpdateOne(
				ctx,
				bson.M{"companyId": subscriptionRequest.CompanyID},
				bson.M{"$set": bson.M{"status": "active", "updatedAt": time.Now()}},
			)
			if err != nil {
				log.Printf("Failed to update user status: %v", err)
			}
			// Commission calculation logic
			// Check if company was created by a salesperson or by itself (user signup)
			if company.CreatedBy == company.UserID {
				log.Printf("DEBUG: Company was created by user signup, adding subscription price to admin wallet")
				// Add subscription price directly to admin wallet (no commission calculation needed)
				log.Printf("Subscription income added to admin wallet: $%.2f from company '%s' (ID: %s) - User signup subscription",
					plan.Price, company.BusinessName, company.ID.Hex())
			} else if !company.CreatedBy.IsZero() {
				// Company was created by a salesperson, proceed with commission calculation
				salespersonID := company.CreatedBy
				var salesperson models.Salesperson
				err = sc.DB.Collection("salespersons").FindOne(ctx, bson.M{"_id": salespersonID}).Decode(&salesperson)
				if err == nil {
					salesManagerID := salesperson.SalesManagerID
					var salesManager models.SalesManager
					err = sc.DB.Collection("sales_managers").FindOne(ctx, bson.M{"_id": salesManagerID}).Decode(&salesManager)
					if err == nil {
						planPrice := plan.Price
						salespersonPercent := salesperson.CommissionPercent
						salesManagerPercent := salesManager.CommissionPercent

						// Calculate commissions correctly
						// Salesperson gets their percentage directly from the plan price
						salespersonCommission := planPrice * salespersonPercent / 100.0
						// Sales manager gets their percentage directly from the plan price
						salesManagerCommission := planPrice * salesManagerPercent / 100.0

						// Insert commission document using Commission model
						commission := models.Commission{
							ID:                            primitive.NewObjectID(),
							SubscriptionID:                subscription.ID,
							CompanyID:                     company.ID,
							PlanID:                        plan.ID,
							PlanPrice:                     planPrice,
							SalespersonID:                 salesperson.ID,
							SalespersonCommission:         salespersonCommission,
							SalespersonCommissionPercent:  salespersonPercent,
							SalesManagerID:                salesManager.ID,
							SalesManagerCommission:        salesManagerCommission,
							SalesManagerCommissionPercent: salesManagerPercent,
							CreatedAt:                     time.Now(),
							Paid:                          false,
							PaidAt:                        nil,
						}
						_, err := sc.DB.Collection("commissions").InsertOne(ctx, commission)
						if err != nil {
							log.Printf("Failed to insert commission: %v", err)
						} else {
							log.Printf("Commission inserted successfully - Plan Price: $%.2f, Sales Manager Commission: $%.2f, Salesperson Commission: $%.2f",
								planPrice, salesManagerCommission, salespersonCommission)
						}
					}
				}
			}
			// Send email notification to company
			if err := sc.sendCompanyNotificationEmail(
				company.ContactInfo.Phone,
				"Subscription Approved",
				fmt.Sprintf("Your subscription request has been approved. Your subscription is active until %s.", endDate.Format("2006-01-02")),
			); err != nil {
				log.Printf("Failed to send company notification email: %v", err)
			}
		}
	} else {
		// If rejected, send notification to company
		var company models.Company
		err = sc.DB.Collection("companies").FindOne(ctx, bson.M{"_id": subscriptionRequest.CompanyID}).Decode(&company)
		if err == nil {
			// Update user status to inactive when subscription is rejected
			usersCollection := sc.DB.Collection("users")
			_, err = usersCollection.UpdateOne(
				ctx,
				bson.M{"companyId": subscriptionRequest.CompanyID},
				bson.M{"$set": bson.M{"status": "inactive", "updatedAt": time.Now()}},
			)
			if err != nil {
				log.Printf("Failed to update user status: %v", err)
			}
			if err := sc.sendCompanyNotificationEmail(
				company.ContactInfo.Phone,
				"Subscription Request Rejected",
				fmt.Sprintf("Your subscription request has been rejected. Reason: %s", approvalReq.AdminNote),
			); err != nil {
				log.Printf("Failed to send company notification email: %v", err)
			}
		}
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: fmt.Sprintf("Subscription request %s successfully", approvalReq.Status),
	})
}

// sendCompanyNotificationEmail sends a notification email to a company
func (sc *SubscriptionController) sendCompanyNotificationEmail(phone, subject, body string) error {
	// TODO: Implement actual email sending to company
	// For now, just log the notification
	log.Printf("Company notification (to %s): %s - %s", phone, subject, body)
	return nil
}

// GetCurrentSubscription retrieves the current active subscription for a company
func (sc *SubscriptionController) GetCurrentSubscription(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user information from token
	claims := middleware.GetUserFromToken(c)
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	// Find company by user ID
	companyCollection := sc.DB.Collection("companies")
	var company models.Company
	err = companyCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&company)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Company not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find company",
		})
	}

	// Find active subscription
	subscriptionsCollection := sc.DB.Collection("branch_subscriptions")
	var subscription models.BranchSubscription
	err = subscriptionsCollection.FindOne(ctx, bson.M{
		"branchId": company.ID,
		"status":   "active",
		"endDate":  bson.M{"$gt": time.Now()},
	}).Decode(&subscription)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusOK, models.Response{
				Status:  http.StatusOK,
				Message: "No active subscription found",
				Data:    nil,
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find subscription",
		})
	}

	// Get plan details
	var plan models.SubscriptionPlan
	err = sc.DB.Collection("subscription_plans").FindOne(ctx, bson.M{"_id": subscription.PlanID}).Decode(&plan)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to get plan details",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Current subscription retrieved successfully",
		Data: map[string]interface{}{
			"subscription": subscription,
			"plan":         plan,
		},
	})
}

// sendAdminNotificationEmail sends a general notification email to the admin
func (sc *SubscriptionController) sendAdminNotificationEmail(subject, body string) error {
	adminEmail := os.Getenv("ADMIN_EMAIL")
	if adminEmail == "" {
		log.Println("Admin email not configured for notifications")
		return fmt.Errorf("admin email not configured")
	}

	// Check if SMTP environment variables are set
	smtpHost := os.Getenv("SMTP_HOST")
	smtpUser := os.Getenv("SMTP_USER")
	smtpPass := os.Getenv("SMTP_PASS")
	fromEmail := os.Getenv("FROM_EMAIL")

	// Use FROM_EMAIL as sender, if not available, fall back to SMTP_USER
	senderEmail := fromEmail
	if senderEmail == "" {
		senderEmail = smtpUser
	}

	if smtpHost == "" || smtpUser == "" || smtpPass == "" || senderEmail == "" {
		log.Println("SMTP configuration is incomplete for notifications")
		return fmt.Errorf("SMTP configuration is incomplete: check SMTP_HOST, SMTP_USER, SMTP_PASS, and FROM_EMAIL")
	}

	// Get SMTP port from environment or default to 2525
	smtpPortStr := os.Getenv("SMTP_PORT")
	smtpPort := 2525 // Default port
	if smtpPortStr != "" {
		portNum, err := strconv.Atoi(smtpPortStr)
		if err == nil && portNum > 0 {
			smtpPort = portNum
		}
	}

	m := gomail.NewMessage()
	m.SetHeader("From", senderEmail)
	m.SetHeader("To", adminEmail)
	m.SetHeader("Subject", subject)
	m.SetBody("text/plain", body)

	d := gomail.NewDialer(
		smtpHost,
		smtpPort,
		smtpUser,
		smtpPass,
	)

	// Try to send the email
	err := d.DialAndSend(m)
	if err != nil {
		log.Printf("Failed to send notification email: %v", err)
		return fmt.Errorf("failed to send email: %w", err)
	}

	log.Printf("Admin notification email sent successfully for subject: %s", subject)
	return nil
}

// PauseSubscription pauses an active subscription
func (sc *SubscriptionController) PauseSubscription(c echo.Context) error {
	// TODO: Implement pausing subscription logic
	return c.JSON(http.StatusNotImplemented, models.Response{
		Status:  http.StatusNotImplemented,
		Message: "Pausing subscription not implemented yet",
	})
}

// RenewSubscription renews an expired or paused subscription
func (sc *SubscriptionController) RenewSubscription(c echo.Context) error {
	// TODO: Implement renewing subscription logic
	return c.JSON(http.StatusNotImplemented, models.Response{
		Status:  http.StatusNotImplemented,
		Message: "Renewing subscription not implemented yet",
	})
}

// CancelSubscription cancels an active subscription
func (sc *SubscriptionController) CancelSubscription(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user information from token
	claims := middleware.GetUserFromToken(c)
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	// Find company by user ID
	companyCollection := sc.DB.Collection("companies")
	var company models.Company
	err = companyCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&company)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Company not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find company",
		})
	}

	// Find and update active subscription
	subscriptionsCollection := sc.DB.Collection("branch_subscriptions")
	update := bson.M{
		"$set": bson.M{
			"status":    "cancelled",
			"autoRenew": false,
			"updatedAt": time.Now(),
		},
	}

	result, err := subscriptionsCollection.UpdateOne(ctx, bson.M{
		"branchId": company.ID,
		"status":   "active",
	}, update)

	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to cancel subscription",
		})
	}

	if result.MatchedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "No active subscription found",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Subscription cancelled successfully",
	})
}

// GetSubscriptionRemainingTime retrieves the remaining time of the current active subscription
func (sc *SubscriptionController) GetSubscriptionRemainingTime(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user information from token
	claims := middleware.GetUserFromToken(c)
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	// Find company by user ID
	companyCollection := sc.DB.Collection("companies")
	var company models.Company
	err = companyCollection.FindOne(ctx, bson.M{"userId": userID}).Decode(&company)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Company not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find company",
		})
	}

	// Find active subscription
	subscriptionsCollection := sc.DB.Collection("branch_subscriptions")
	var subscription models.BranchSubscription
	err = subscriptionsCollection.FindOne(ctx, bson.M{
		"branchId": company.ID,
		"status":   "active",
		"endDate":  bson.M{"$gt": time.Now()},
	}).Decode(&subscription)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusOK, models.Response{
				Status:  http.StatusOK,
				Message: "No active subscription found",
				Data: map[string]interface{}{
					"hasActiveSubscription": false,
					"remainingTime":         nil,
				},
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find subscription",
		})
	}

	// Calculate remaining time
	now := time.Now()
	remainingTime := subscription.EndDate.Sub(now)

	// Calculate days, hours, minutes, and seconds
	days := int(remainingTime.Hours() / 24)
	hours := int(remainingTime.Hours()) % 24
	minutes := int(remainingTime.Minutes()) % 60
	seconds := int(remainingTime.Seconds()) % 60

	// Format remaining time in a human-readable way
	remainingTimeFormatted := fmt.Sprintf("%dd %dh %dm %ds", days, hours, minutes, seconds)

	// Calculate percentage of subscription used
	totalDuration := subscription.EndDate.Sub(subscription.StartDate)
	usedDuration := now.Sub(subscription.StartDate)
	percentageUsed := (usedDuration.Seconds() / totalDuration.Seconds()) * 100

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Subscription remaining time retrieved successfully",
		Data: map[string]interface{}{
			"hasActiveSubscription": true,
			"remainingTime": map[string]interface{}{
				"days":           days,
				"hours":          hours,
				"minutes":        minutes,
				"seconds":        seconds,
				"formatted":      remainingTimeFormatted,
				"percentageUsed": fmt.Sprintf("%.1f%%", percentageUsed),
				"endDate":        subscription.EndDate.Format(time.RFC3339),
			},
		},
	})
}

// CreateSubscriptionPlan creates a new subscription plan (admin only)
func (sc *SubscriptionController) CreateSubscriptionPlan(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get user information from token
	claims := middleware.GetUserFromToken(c)
	if claims.UserType == "admin" {
		// allow
	} else if claims.UserType == "manager" {
		// Fetch manager from DB
		var manager models.Manager
		err := sc.DB.Collection("managers").FindOne(context.Background(), bson.M{"_id": claims.UserID}).Decode(&manager)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to fetch manager",
			})
		}
		if !hasRole(manager.RolesAccess, "referral_program_monitoring") {
			return c.JSON(http.StatusForbidden, models.Response{
				Status:  http.StatusForbidden,
				Message: "Manager does not have referral_program_monitoring role",
			})
		}
	} else if claims.UserType == "sales_manager" {
		// Fetch sales manager from DB
		var salesManager models.SalesManager
		err := sc.DB.Collection("sales_managers").FindOne(context.Background(), bson.M{"_id": claims.UserID}).Decode(&salesManager)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to fetch sales manager",
			})
		}
		if !hasRole(salesManager.RolesAccess, "referral_program_monitoring") {
			return c.JSON(http.StatusForbidden, models.Response{
				Status:  http.StatusForbidden,
				Message: "Sales manager does not have referral_program_monitoring role",
			})
		}
	} else {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only admins, managers, or sales managers can create subscription plans",
		})
	}

	// Parse request body
	var req models.SubscriptionPlanRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}

	// Validate required fields
	if req.Title == "" || req.Price < 0 || req.Duration <= 0 {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Missing required fields: title, price, and duration are required. Price must be 0 or greater.",
		})
	}

	// Validate duration
	validDurations := map[int]bool{
		1:  true, // Monthly
		6:  true, // 6 Months
		12: true, // 1 Year
	}
	if !validDurations[req.Duration] {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid duration. Must be one of: 1 (Monthly), 6 (6 Months), 12 (1 Year)",
		})
	}

	// Validate plan type
	if req.Type != "company" && req.Type != "wholesaler" && req.Type != "serviceProvider" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid plan type. Must be one of: company, wholesaler, serviceProvider",
		})
	}

	var plans []models.SubscriptionPlan

	// Create plans based on type
	if req.Type == "serviceProvider" {
		// Create single plan for service provider
		plans = []models.SubscriptionPlan{
			{
				ID:        primitive.NewObjectID(),
				Title:     req.Title,
				Price:     req.Price,
				Duration:  req.Duration,
				Type:      "serviceProvider",
				Benefits:  models.Benefits{Value: req.Benefits},
				IsActive:  req.IsActive,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		}
	} else {
		// Create plans for both company and wholesaler
		plans = []models.SubscriptionPlan{
			{
				ID:        primitive.NewObjectID(),
				Title:     req.Title,
				Price:     req.Price,
				Duration:  req.Duration,
				Type:      "company",
				Benefits:  models.Benefits{Value: req.Benefits},
				IsActive:  req.IsActive,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			{
				ID:        primitive.NewObjectID(),
				Title:     req.Title,
				Price:     req.Price,
				Duration:  req.Duration,
				Type:      "wholesaler",
				Benefits:  models.Benefits{Value: req.Benefits},
				IsActive:  req.IsActive,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		}
	}

	// Save plans
	subscriptionPlansCollection := sc.DB.Collection("subscription_plans")
	var planDocs []interface{}
	for _, plan := range plans {
		planDocs = append(planDocs, plan)
	}

	_, err := subscriptionPlansCollection.InsertMany(ctx, planDocs)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to create subscription plans",
		})
	}

	successMessage := "Subscription plan created successfully"
	if req.Type != "serviceProvider" {
		successMessage = "Subscription plans created successfully for both companies and wholesalers"
	}

	return c.JSON(http.StatusCreated, models.Response{
		Status:  http.StatusCreated,
		Message: successMessage,
		Data:    plans,
	})
}

// GetSubscriptionPlan retrieves a specific subscription plan by ID
func (sc *SubscriptionController) GetSubscriptionPlan(c echo.Context) error {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid ID format",
		})
	}

	var plan models.SubscriptionPlan
	err = sc.DB.Collection("subscription_plans").FindOne(context.Background(), bson.M{"_id": id}).Decode(&plan)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Subscription plan not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch subscription plan",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Subscription plan retrieved successfully",
		Data:    plan,
	})
}

// UpdateSubscriptionPlan updates a specific subscription plan
func (sc *SubscriptionController) UpdateSubscriptionPlan(c echo.Context) error {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid ID format",
		})
	}

	var req models.SubscriptionPlanRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}

	update := bson.M{
		"$set": bson.M{
			"title":     req.Title,
			"price":     req.Price,
			"duration":  req.Duration,
			"type":      req.Type,
			"benefits":  req.Benefits,
			"isActive":  req.IsActive,
			"updatedAt": time.Now(),
		},
	}

	result, err := sc.DB.Collection("subscription_plans").UpdateOne(
		context.Background(),
		bson.M{"_id": id},
		update,
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update subscription plan",
		})
	}

	if result.MatchedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Subscription plan not found",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Subscription plan updated successfully",
	})
}

// DeleteSubscriptionPlan deletes a specific subscription plan
func (sc *SubscriptionController) DeleteSubscriptionPlan(c echo.Context) error {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid ID format",
		})
	}

	result, err := sc.DB.Collection("subscription_plans").DeleteOne(
		context.Background(),
		bson.M{"_id": id},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to delete subscription plan",
		})
	}

	if result.DeletedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Subscription plan not found",
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Subscription plan deleted successfully",
	})
}

// GetServiceProviderSubscriptionPlans retrieves all available subscription plans for service providers
func (sc *SubscriptionController) GetServiceProviderSubscriptionPlans(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	collection := sc.DB.Collection("subscription_plans")
	cursor, err := collection.Find(ctx, bson.M{
		"type":     "serviceProvider",
		"isActive": true,
	})
	if err != nil {
		log.Printf("Error finding service provider subscription plans: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve subscription plans",
		})
	}
	defer cursor.Close(ctx)

	var plans []models.SubscriptionPlan
	if err = cursor.All(ctx, &plans); err != nil {
		log.Printf("Error decoding service provider subscription plans: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve subscription plans",
		})
	}

	if len(plans) == 0 {
		return c.JSON(http.StatusOK, models.Response{
			Status:  http.StatusOK,
			Message: "No subscription plans found",
			Data:    []models.SubscriptionPlan{},
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Service provider subscription plans retrieved successfully",
		Data:    plans,
	})
}

func (sc *SubscriptionController) GetWholesalerPlans(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	collection := sc.DB.Collection("subscription_plans")
	cursor, err := collection.Find(ctx, bson.M{
		"type":     "wholesaler",
		"isActive": true,
	})
	if err != nil {
		log.Printf("Error finding wholesaler subscription plans: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve subscription plans",
		})
	}
	defer cursor.Close(ctx)

	var plans []models.SubscriptionPlan
	if err = cursor.All(ctx, &plans); err != nil {
		log.Printf("Error decoding wholesaler subscription plans: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve subscription plans",
		})
	}

	if len(plans) == 0 {
		return c.JSON(http.StatusOK, models.Response{
			Status:  http.StatusOK,
			Message: "No subscription plans found",
			Data:    []models.SubscriptionPlan{},
		})
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Wholesaler subscription plans retrieved successfully",
		Data:    plans,
	})
}
