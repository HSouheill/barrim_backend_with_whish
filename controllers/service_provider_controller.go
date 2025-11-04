// controllers/serviceProviders_controller.go
package controllers

import (
	"context"
	"fmt"
	"io"
	"log"
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
)

type ServiceProviderController struct {
	DB *mongo.Database
}

// ServiceProviderFullData represents the complete service provider data including related entities
type ServiceProviderFullData struct {
	ServiceProvider  models.ServiceProvider                      `json:"serviceProvider"`
	Subscriptions    []models.ServiceProviderSubscription        `json:"subscriptions,omitempty"`
	SubscriptionReqs []models.ServiceProviderSubscriptionRequest `json:"subscriptionRequests,omitempty"`
}

func NewServiceProviderController(client *mongo.Client) *ServiceProviderController {
	return &ServiceProviderController{DB: client.Database("barrim")}
}

// GetFullServiceProviderData retrieves complete service provider data including subscriptions and requests
func (spc *ServiceProviderController) GetFullServiceProviderData(c echo.Context) error {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
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

	// Initialize the response structure
	var result ServiceProviderFullData

	// Get service provider data
	err = spc.DB.Collection("serviceProviders").FindOne(ctx, bson.M{
		"$or": []bson.M{
			{"userId": userID},
			{"createdBy": userID},
		},
	}).Decode(&result.ServiceProvider)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Service provider not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve service provider data",
		})
	}

	// Get service provider subscriptions
	subscriptionCursor, err := spc.DB.Collection("serviceProviderSubscriptions").Find(ctx, bson.M{"serviceProviderId": result.ServiceProvider.ID})
	if err != nil && err != mongo.ErrNoDocuments {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve subscriptions",
		})
	}
	if err == nil {
		defer subscriptionCursor.Close(ctx)
		if err = subscriptionCursor.All(ctx, &result.Subscriptions); err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to decode subscriptions",
			})
		}
	}

	// Get subscription requests
	reqCursor, err := spc.DB.Collection("serviceProviderSubscriptionRequests").Find(ctx, bson.M{"serviceProviderId": result.ServiceProvider.ID})
	if err != nil && err != mongo.ErrNoDocuments {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to retrieve subscription requests",
		})
	}
	if err == nil {
		defer reqCursor.Close(ctx)
		if err = reqCursor.All(ctx, &result.SubscriptionReqs); err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to decode subscription requests",
			})
		}
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Complete service provider data retrieved successfully",
		Data:    result,
	})
}

func (spc *ServiceProviderController) GetServiceProviderData(c echo.Context) error {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	claims := middleware.GetUserFromToken(c)
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	var serviceProvider models.ServiceProvider
	// First try to find by userId (standard approach)
	err = spc.DB.Collection("serviceProviders").FindOne(ctx, bson.M{"userId": userID}).Decode(&serviceProvider)
	if err != nil {
		// If not found by userId, try to find by CreatedBy (salesperson-created service providers)
		err = spc.DB.Collection("serviceProviders").FindOne(ctx, bson.M{"createdBy": userID}).Decode(&serviceProvider)
		if err != nil {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Service provider not found",
			})
		}
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Service provider data retrieved successfully",
		Data:    serviceProvider,
	})
}

func (spc *ServiceProviderController) UpdateServiceProviderData(c echo.Context) error {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	claims := middleware.GetUserFromToken(c)
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	// Parse request body with enhanced availability structure
	var requestBody struct {
		models.ServiceProvider
		// Enhanced availability structure for specific day-time mapping
		AvailabilitySchedule []struct {
			Date        string   `json:"date"`        // YYYY-MM-DD format for specific dates, or weekday name
			IsWeekday   bool     `json:"isWeekday"`   // true if it's a recurring weekday, false if specific date
			TimeSlots   []string `json:"timeSlots"`   // Array of time ranges like ["09:00-17:00", "19:00-21:00"]
			IsAvailable bool     `json:"isAvailable"` // Whether available on this day
		} `json:"availabilitySchedule,omitempty"`
	}

	if err := c.Bind(&requestBody); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request data",
		})
	}

	updateData := requestBody.ServiceProvider

	// Validate availability data if provided
	if updateData.ServiceProviderInfo != nil {
		// Validate available days format (should be YYYY-MM-DD for specific dates)
		if updateData.ServiceProviderInfo.AvailableDays != nil {
			for i, day := range updateData.ServiceProviderInfo.AvailableDays {
				if day != "" {
					// Check if it's a specific date (YYYY-MM-DD) or weekday
					if _, err := time.Parse("2006-01-02", day); err != nil {
						// If not a date, check if it's a valid weekday
						validWeekdays := []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}
						isValidWeekday := false
						for _, weekday := range validWeekdays {
							if day == weekday {
								isValidWeekday = true
								break
							}
						}
						if !isValidWeekday {
							return c.JSON(http.StatusBadRequest, models.Response{
								Status:  http.StatusBadRequest,
								Message: fmt.Sprintf("Invalid day format at index %d. Use YYYY-MM-DD for specific dates or weekday names (Monday, Tuesday, etc.)", i),
							})
						}
					}
				}
			}
		}

		// Validate available hours format (should be HH:MM-HH:MM)
		if updateData.ServiceProviderInfo.AvailableHours != nil {
			for i, hourRange := range updateData.ServiceProviderInfo.AvailableHours {
				if hourRange != "" {
					// Split the range into start and end times
					times := strings.Split(hourRange, "-")
					if len(times) != 2 {
						return c.JSON(http.StatusBadRequest, models.Response{
							Status:  http.StatusBadRequest,
							Message: fmt.Sprintf("Invalid hour format at index %d. Use HH:MM-HH:MM format (e.g., 09:00-17:00)", i),
						})
					}

					startTime := strings.TrimSpace(times[0])
					endTime := strings.TrimSpace(times[1])

					// Validate time format
					if _, err := time.Parse("15:04", startTime); err != nil {
						return c.JSON(http.StatusBadRequest, models.Response{
							Status:  http.StatusBadRequest,
							Message: fmt.Sprintf("Invalid start time format at index %d. Use HH:MM format (e.g., 09:00)", i),
						})
					}
					if _, err := time.Parse("15:04", endTime); err != nil {
						return c.JSON(http.StatusBadRequest, models.Response{
							Status:  http.StatusBadRequest,
							Message: fmt.Sprintf("Invalid end time format at index %d. Use HH:MM format (e.g., 17:00)", i),
						})
					}

					// Validate that start time is before end time
					start, _ := time.Parse("15:04", startTime)
					end, _ := time.Parse("15:04", endTime)
					if !start.Before(end) {
						return c.JSON(http.StatusBadRequest, models.Response{
							Status:  http.StatusBadRequest,
							Message: fmt.Sprintf("Start time must be before end time at index %d", i),
						})
					}
				}
			}
		}

		// Validate available weekdays
		if updateData.ServiceProviderInfo.AvailableWeekdays != nil {
			validWeekdays := []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}
			for i, weekday := range updateData.ServiceProviderInfo.AvailableWeekdays {
				isValid := false
				for _, validWeekday := range validWeekdays {
					if weekday == validWeekday {
						isValid = true
						break
					}
				}
				if !isValid {
					return c.JSON(http.StatusBadRequest, models.Response{
						Status:  http.StatusBadRequest,
						Message: fmt.Sprintf("Invalid weekday at index %d. Use: Monday, Tuesday, Wednesday, Thursday, Friday, Saturday, Sunday", i),
					})
				}
			}
		}
	}

	// Validate and process availability schedule if provided
	if requestBody.AvailabilitySchedule != nil {
		// Process availability schedule into the standard format
		var availableDays []string
		var availableHours []string
		var availableWeekdays []string

		for i, schedule := range requestBody.AvailabilitySchedule {
			// Validate date format
			if schedule.IsWeekday {
				// Validate weekday name
				validWeekdays := []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}
				isValidWeekday := false
				for _, weekday := range validWeekdays {
					if schedule.Date == weekday {
						isValidWeekday = true
						break
					}
				}
				if !isValidWeekday {
					return c.JSON(http.StatusBadRequest, models.Response{
						Status:  http.StatusBadRequest,
						Message: fmt.Sprintf("Invalid weekday at index %d. Use: Monday, Tuesday, Wednesday, Thursday, Friday, Saturday, Sunday", i),
					})
				}
				availableWeekdays = append(availableWeekdays, schedule.Date)
			} else {
				// Validate specific date format (YYYY-MM-DD)
				if _, err := time.Parse("2006-01-02", schedule.Date); err != nil {
					return c.JSON(http.StatusBadRequest, models.Response{
						Status:  http.StatusBadRequest,
						Message: fmt.Sprintf("Invalid date format at index %d. Use YYYY-MM-DD format for specific dates", i),
					})
				}
				availableDays = append(availableDays, schedule.Date)
			}

			// Validate time slots
			for j, timeSlot := range schedule.TimeSlots {
				if timeSlot != "" {
					// Split the range into start and end times
					times := strings.Split(timeSlot, "-")
					if len(times) != 2 {
						return c.JSON(http.StatusBadRequest, models.Response{
							Status:  http.StatusBadRequest,
							Message: fmt.Sprintf("Invalid time slot format at schedule %d, slot %d. Use HH:MM-HH:MM format (e.g., 09:00-17:00)", i, j),
						})
					}

					startTime := strings.TrimSpace(times[0])
					endTime := strings.TrimSpace(times[1])

					// Validate time format
					if _, err := time.Parse("15:04", startTime); err != nil {
						return c.JSON(http.StatusBadRequest, models.Response{
							Status:  http.StatusBadRequest,
							Message: fmt.Sprintf("Invalid start time format at schedule %d, slot %d. Use HH:MM format (e.g., 09:00)", i, j),
						})
					}
					if _, err := time.Parse("15:04", endTime); err != nil {
						return c.JSON(http.StatusBadRequest, models.Response{
							Status:  http.StatusBadRequest,
							Message: fmt.Sprintf("Invalid end time format at schedule %d, slot %d. Use HH:MM format (e.g., 17:00)", i, j),
						})
					}

					// Validate that start time is before end time
					start, _ := time.Parse("15:04", startTime)
					end, _ := time.Parse("15:04", endTime)
					if !start.Before(end) {
						return c.JSON(http.StatusBadRequest, models.Response{
							Status:  http.StatusBadRequest,
							Message: fmt.Sprintf("Start time must be before end time at schedule %d, slot %d", i, j),
						})
					}

					// Add to available hours
					availableHours = append(availableHours, timeSlot)
				}
			}
		}

		// Update the ServiceProviderInfo with processed availability data
		if updateData.ServiceProviderInfo == nil {
			updateData.ServiceProviderInfo = &models.ServiceProviderInfo{}
		}

		if len(availableDays) > 0 {
			updateData.ServiceProviderInfo.AvailableDays = availableDays
		}
		if len(availableHours) > 0 {
			updateData.ServiceProviderInfo.AvailableHours = availableHours
		}
		if len(availableWeekdays) > 0 {
			updateData.ServiceProviderInfo.AvailableWeekdays = availableWeekdays
		}
	}

	// Set updated timestamp
	updateData.UpdatedAt = time.Now()

	// Find the service provider first to ensure it exists and get current data
	var existingServiceProvider models.ServiceProvider
	err = spc.DB.Collection("serviceProviders").FindOne(ctx, bson.M{
		"$or": []bson.M{
			{"userId": userID},
			{"createdBy": userID},
		},
	}).Decode(&existingServiceProvider)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Service provider not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find service provider",
		})
	}

	// Prepare update operation
	updateFields := bson.M{
		"updatedAt": updateData.UpdatedAt,
	}

	// Handle ServiceProviderInfo updates with proper merging
	if updateData.ServiceProviderInfo != nil {
		// If existing service provider has no ServiceProviderInfo, create it
		if existingServiceProvider.ServiceProviderInfo == nil {
			updateFields["serviceProviderInfo"] = updateData.ServiceProviderInfo
		} else {
			// Merge with existing ServiceProviderInfo
			existingInfo := existingServiceProvider.ServiceProviderInfo

			// Update only the fields that are provided
			if updateData.ServiceProviderInfo.ServiceType != "" {
				existingInfo.ServiceType = updateData.ServiceProviderInfo.ServiceType
			}
			if updateData.ServiceProviderInfo.CustomServiceType != "" {
				existingInfo.CustomServiceType = updateData.ServiceProviderInfo.CustomServiceType
			}
			if updateData.ServiceProviderInfo.Description != "" {
				existingInfo.Description = updateData.ServiceProviderInfo.Description
			}
			if updateData.ServiceProviderInfo.YearsExperience != nil {
				existingInfo.YearsExperience = updateData.ServiceProviderInfo.YearsExperience
			}
			if updateData.ServiceProviderInfo.ProfilePhoto != "" {
				existingInfo.ProfilePhoto = updateData.ServiceProviderInfo.ProfilePhoto
			}
			if updateData.ServiceProviderInfo.CertificateImages != nil {
				existingInfo.CertificateImages = updateData.ServiceProviderInfo.CertificateImages
			}
			if updateData.ServiceProviderInfo.PortfolioImages != nil {
				existingInfo.PortfolioImages = updateData.ServiceProviderInfo.PortfolioImages
			}
			if updateData.ServiceProviderInfo.AvailableHours != nil {
				existingInfo.AvailableHours = updateData.ServiceProviderInfo.AvailableHours
			}
			if updateData.ServiceProviderInfo.AvailableDays != nil {
				existingInfo.AvailableDays = updateData.ServiceProviderInfo.AvailableDays
			}
			if updateData.ServiceProviderInfo.AvailableWeekdays != nil {
				existingInfo.AvailableWeekdays = updateData.ServiceProviderInfo.AvailableWeekdays
			}
			if updateData.ServiceProviderInfo.ApplyToAllMonths {
				existingInfo.ApplyToAllMonths = updateData.ServiceProviderInfo.ApplyToAllMonths
			}
			if updateData.ServiceProviderInfo.Status != "" {
				existingInfo.Status = updateData.ServiceProviderInfo.Status
			}
			if updateData.ServiceProviderInfo.SocialLinks != nil {
				existingInfo.SocialLinks = updateData.ServiceProviderInfo.SocialLinks
			}

			updateFields["serviceProviderInfo"] = existingInfo
		}
	}

	// Handle other fields
	if updateData.BusinessName != "" {
		updateFields["businessName"] = updateData.BusinessName
	}
	if updateData.Category != "" {
		updateFields["category"] = updateData.Category
	}
	if updateData.Email != "" {
		updateFields["email"] = updateData.Email
	}
	if updateData.Phone != "" {
		updateFields["phone"] = updateData.Phone
	}
	if updateData.ContactPerson != "" {
		updateFields["contactPerson"] = updateData.ContactPerson
	}
	if updateData.ContactPhone != "" {
		updateFields["contactPhone"] = updateData.ContactPhone
	}
	if updateData.Country != "" {
		updateFields["country"] = updateData.Country
	}
	if updateData.Governorate != "" {
		updateFields["governorate"] = updateData.Governorate
	}
	if updateData.District != "" {
		updateFields["district"] = updateData.District
	}
	if updateData.City != "" {
		updateFields["city"] = updateData.City
	}
	if updateData.LogoURL != "" {
		updateFields["logo"] = updateData.LogoURL
	}
	if updateData.ProfilePicURL != "" {
		updateFields["profilePicUrl"] = updateData.ProfilePicURL
	}
	if updateData.AdditionalPhones != nil {
		updateFields["additionalPhones"] = updateData.AdditionalPhones
	}
	if updateData.AdditionalEmails != nil {
		updateFields["additionalEmails"] = updateData.AdditionalEmails
	}
	if updateData.ContactInfo.Phone != "" || updateData.ContactInfo.WhatsApp != "" || updateData.ContactInfo.Website != "" {
		updateFields["contactInfo"] = updateData.ContactInfo
	}
	if updateData.ReferralCode != "" {
		updateFields["referralCode"] = updateData.ReferralCode
	}
	if updateData.Points != 0 {
		updateFields["points"] = updateData.Points
	}
	if updateData.CommissionPercent != 0 {
		updateFields["commissionPercent"] = updateData.CommissionPercent
	}
	if updateData.Status != "" {
		updateFields["status"] = updateData.Status
	}

	// Perform the update
	result, err := spc.DB.Collection("serviceProviders").UpdateOne(
		ctx,
		bson.M{"_id": existingServiceProvider.ID},
		bson.M{"$set": updateFields},
	)

	if err != nil {
		log.Printf("Failed to update service provider data: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update service provider data",
		})
	}

	if result.MatchedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Service provider not found",
		})
	}

	// Log the action
	log.Printf("Service provider data updated: ID=%s, UpdatedBy=%s",
		existingServiceProvider.ID.Hex(), claims.UserID)

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Service provider data updated successfully",
	})
}

func (spc *ServiceProviderController) UploadLogo(c echo.Context) error {
	// TODO: Implement logo upload functionality
	return c.JSON(http.StatusNotImplemented, models.Response{
		Status:  http.StatusNotImplemented,
		Message: "Logo upload not implemented yet",
	})
}

// UploadPortfolioImage allows service providers to upload portfolio images
func (spc *ServiceProviderController) UploadPortfolioImage(c echo.Context) error {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
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

	// Find the service provider first to ensure it exists
	var serviceProvider models.ServiceProvider
	err = spc.DB.Collection("serviceProviders").FindOne(ctx, bson.M{
		"$or": []bson.M{
			{"userId": userID},
			{"createdBy": userID},
		},
	}).Decode(&serviceProvider)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Service provider not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find service provider",
		})
	}

	// Get file from form
	file, err := c.FormFile("image")
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "No image file uploaded. Please use 'image' as the form field name",
		})
	}

	// Validate file type (only images allowed)
	if !strings.HasPrefix(file.Header.Get("Content-Type"), "image/") {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Only image files are allowed",
		})
	}

	// Open the file
	src, err := file.Open()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to open uploaded file",
		})
	}
	defer src.Close()

	// Generate unique filename
	filename := uuid.New().String() + filepath.Ext(file.Filename)
	uploadDir := "uploads/serviceprovider/portfolio"

	// Ensure directory exists
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		log.Printf("Failed to create upload directory: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to create upload directory",
		})
	}

	// Create destination file path
	uploadPath := filepath.Join(uploadDir, filename)
	dst, err := os.Create(uploadPath)
	if err != nil {
		log.Printf("Failed to create destination file: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to save file",
		})
	}
	defer dst.Close()

	// Copy file content
	if _, err = io.Copy(dst, src); err != nil {
		log.Printf("Failed to copy file: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to save file",
		})
	}

	// Store relative path for API access
	relativePath := "/" + uploadPath

	// Initialize ServiceProviderInfo if it doesn't exist
	if serviceProvider.ServiceProviderInfo == nil {
		serviceProvider.ServiceProviderInfo = &models.ServiceProviderInfo{
			PortfolioImages: []string{},
		}
	}

	// Initialize PortfolioImages slice if it doesn't exist
	if serviceProvider.ServiceProviderInfo.PortfolioImages == nil {
		serviceProvider.ServiceProviderInfo.PortfolioImages = []string{}
	}

	// Append the new portfolio image to the existing array
	portfolioImages := append(serviceProvider.ServiceProviderInfo.PortfolioImages, relativePath)

	// Update the service provider with the new portfolio image
	update := bson.M{
		"$set": bson.M{
			"serviceProviderInfo.portfolioImages": portfolioImages,
			"updatedAt":                           time.Now(),
		},
	}

	result, err := spc.DB.Collection("serviceProviders").UpdateOne(
		ctx,
		bson.M{"_id": serviceProvider.ID},
		update,
	)

	if err != nil {
		log.Printf("Failed to update service provider portfolio: %v", err)
		// Clean up uploaded file if database update fails
		os.Remove(uploadPath)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update service provider portfolio",
		})
	}

	if result.MatchedCount == 0 {
		// Clean up uploaded file if no document was matched
		os.Remove(uploadPath)
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Service provider not found",
		})
	}

	// Log the action
	log.Printf("Portfolio image uploaded: ServiceProviderID=%s, ImagePath=%s, UpdatedBy=%s",
		serviceProvider.ID.Hex(), relativePath, claims.UserID)

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Portfolio image uploaded successfully",
		Data: map[string]interface{}{
			"imagePath":       relativePath,
			"portfolioImages": portfolioImages,
		},
	})
}

// GetPortfolioImages retrieves all portfolio images for the authenticated service provider
func (spc *ServiceProviderController) GetPortfolioImages(c echo.Context) error {
	// Create context with timeout
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

	// Find the service provider
	var serviceProvider models.ServiceProvider
	err = spc.DB.Collection("serviceProviders").FindOne(ctx, bson.M{
		"$or": []bson.M{
			{"userId": userID},
			{"createdBy": userID},
		},
	}).Decode(&serviceProvider)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Service provider not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find service provider",
		})
	}

	// Get portfolio images
	portfolioImages := []string{}
	if serviceProvider.ServiceProviderInfo != nil && serviceProvider.ServiceProviderInfo.PortfolioImages != nil {
		portfolioImages = serviceProvider.ServiceProviderInfo.PortfolioImages
	}

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Portfolio images retrieved successfully",
		Data: map[string]interface{}{
			"portfolioImages": portfolioImages,
			"count":           len(portfolioImages),
		},
	})
}

// DeletePortfolioImage deletes a specific portfolio image by index
func (spc *ServiceProviderController) DeletePortfolioImage(c echo.Context) error {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
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

	// Parse request body to get index
	var req struct {
		Index int `json:"index"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body. Please provide 'index' field",
		})
	}

	// Find the service provider
	var serviceProvider models.ServiceProvider
	err = spc.DB.Collection("serviceProviders").FindOne(ctx, bson.M{
		"$or": []bson.M{
			{"userId": userID},
			{"createdBy": userID},
		},
	}).Decode(&serviceProvider)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Service provider not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find service provider",
		})
	}

	// Check if ServiceProviderInfo and PortfolioImages exist
	if serviceProvider.ServiceProviderInfo == nil || serviceProvider.ServiceProviderInfo.PortfolioImages == nil {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "No portfolio images found",
		})
	}

	portfolioImages := serviceProvider.ServiceProviderInfo.PortfolioImages

	// Validate index
	if req.Index < 0 || req.Index >= len(portfolioImages) {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: fmt.Sprintf("Invalid index. Portfolio has %d images (index 0-%d)", len(portfolioImages), len(portfolioImages)-1),
		})
	}

	// Get the image path to delete from filesystem
	imagePath := portfolioImages[req.Index]

	// Remove leading slash for filesystem path
	fileSystemPath := strings.TrimPrefix(imagePath, "/")

	// Remove from array using $pull
	update := bson.M{
		"$pull": bson.M{
			"serviceProviderInfo.portfolioImages": imagePath,
		},
		"$set": bson.M{
			"updatedAt": time.Now(),
		},
	}

	result, err := spc.DB.Collection("serviceProviders").UpdateOne(
		ctx,
		bson.M{"_id": serviceProvider.ID},
		update,
	)

	if err != nil {
		log.Printf("Failed to delete portfolio image: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to delete portfolio image",
		})
	}

	if result.MatchedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Service provider not found",
		})
	}

	// Delete file from filesystem
	if err := os.Remove(fileSystemPath); err != nil {
		// Log error but don't fail the request since DB update succeeded
		log.Printf("Warning: Failed to delete image file %s: %v", fileSystemPath, err)
	} else {
		log.Printf("Successfully deleted image file: %s", fileSystemPath)
	}

	// Get updated portfolio images
	var updatedServiceProvider models.ServiceProvider
	spc.DB.Collection("serviceProviders").FindOne(ctx, bson.M{"_id": serviceProvider.ID}).Decode(&updatedServiceProvider)
	updatedPortfolioImages := []string{}
	if updatedServiceProvider.ServiceProviderInfo != nil && updatedServiceProvider.ServiceProviderInfo.PortfolioImages != nil {
		updatedPortfolioImages = updatedServiceProvider.ServiceProviderInfo.PortfolioImages
	}

	// Log the action
	log.Printf("Portfolio image deleted: ServiceProviderID=%s, ImagePath=%s, Index=%d, UpdatedBy=%s",
		serviceProvider.ID.Hex(), imagePath, req.Index, claims.UserID)

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Portfolio image deleted successfully",
		Data: map[string]interface{}{
			"deletedImagePath": imagePath,
			"portfolioImages":  updatedPortfolioImages,
			"count":            len(updatedPortfolioImages),
		},
	})
}

// UpdatePortfolioImage replaces an existing portfolio image at a specific index with a new image
func (spc *ServiceProviderController) UpdatePortfolioImage(c echo.Context) error {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
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

	// Get index from form
	indexStr := c.FormValue("index")
	if indexStr == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Index is required in form data",
		})
	}

	var index int
	if _, err := fmt.Sscanf(indexStr, "%d", &index); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid index format. Index must be a number",
		})
	}

	// Find the service provider
	var serviceProvider models.ServiceProvider
	err = spc.DB.Collection("serviceProviders").FindOne(ctx, bson.M{
		"$or": []bson.M{
			{"userId": userID},
			{"createdBy": userID},
		},
	}).Decode(&serviceProvider)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Service provider not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find service provider",
		})
	}

	// Check if ServiceProviderInfo and PortfolioImages exist
	if serviceProvider.ServiceProviderInfo == nil || serviceProvider.ServiceProviderInfo.PortfolioImages == nil {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "No portfolio images found",
		})
	}

	portfolioImages := serviceProvider.ServiceProviderInfo.PortfolioImages

	// Validate index
	if index < 0 || index >= len(portfolioImages) {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: fmt.Sprintf("Invalid index. Portfolio has %d images (index 0-%d)", len(portfolioImages), len(portfolioImages)-1),
		})
	}

	// Get file from form
	file, err := c.FormFile("image")
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "No image file uploaded. Please use 'image' as the form field name",
		})
	}

	// Validate file type (only images allowed)
	if !strings.HasPrefix(file.Header.Get("Content-Type"), "image/") {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Only image files are allowed",
		})
	}

	// Open the file
	src, err := file.Open()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to open uploaded file",
		})
	}
	defer src.Close()

	// Get old image path for deletion
	oldImagePath := portfolioImages[index]
	oldFileSystemPath := strings.TrimPrefix(oldImagePath, "/")

	// Generate unique filename for new image
	filename := uuid.New().String() + filepath.Ext(file.Filename)
	uploadDir := "uploads/serviceprovider/portfolio"

	// Ensure directory exists
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		log.Printf("Failed to create upload directory: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to create upload directory",
		})
	}

	// Create destination file path
	uploadPath := filepath.Join(uploadDir, filename)
	dst, err := os.Create(uploadPath)
	if err != nil {
		log.Printf("Failed to create destination file: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to save file",
		})
	}
	defer dst.Close()

	// Copy file content
	if _, err = io.Copy(dst, src); err != nil {
		log.Printf("Failed to copy file: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to save file",
		})
	}

	// Store relative path for API access
	relativePath := "/" + uploadPath

	// Update the portfolio images array at the specific index
	portfolioImages[index] = relativePath

	// Update the service provider with the new portfolio image
	update := bson.M{
		"$set": bson.M{
			"serviceProviderInfo.portfolioImages": portfolioImages,
			"updatedAt":                           time.Now(),
		},
	}

	result, err := spc.DB.Collection("serviceProviders").UpdateOne(
		ctx,
		bson.M{"_id": serviceProvider.ID},
		update,
	)

	if err != nil {
		log.Printf("Failed to update service provider portfolio: %v", err)
		// Clean up uploaded file if database update fails
		os.Remove(uploadPath)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update service provider portfolio",
		})
	}

	if result.MatchedCount == 0 {
		// Clean up uploaded file if no document was matched
		os.Remove(uploadPath)
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Service provider not found",
		})
	}

	// Delete old file from filesystem
	if err := os.Remove(oldFileSystemPath); err != nil {
		// Log error but don't fail the request since DB update succeeded
		log.Printf("Warning: Failed to delete old image file %s: %v", oldFileSystemPath, err)
	} else {
		log.Printf("Successfully deleted old image file: %s", oldFileSystemPath)
	}

	// Log the action
	log.Printf("Portfolio image updated: ServiceProviderID=%s, Index=%d, OldPath=%s, NewPath=%s, UpdatedBy=%s",
		serviceProvider.ID.Hex(), index, oldImagePath, relativePath, claims.UserID)

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Portfolio image updated successfully",
		Data: map[string]interface{}{
			"index":           index,
			"oldImagePath":    oldImagePath,
			"newImagePath":    relativePath,
			"portfolioImages": portfolioImages,
		},
	})
}

// ToggleEntityStatus allows service providers to toggle their own status or admins/managers to toggle any service provider status
func (spc *ServiceProviderController) ToggleEntityStatus(c echo.Context) error {
	claims := middleware.GetUserFromToken(c)

	// Get the service provider ID from URL parameter
	serviceProviderID := c.Param("id")
	if serviceProviderID == "" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Service provider ID is required",
		})
	}

	objID, err := primitive.ObjectIDFromHex(serviceProviderID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid service provider ID format",
		})
	}

	// Check if the user is trying to toggle their own status or if they have admin/manager privileges
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid user ID",
		})
	}

	// If user is a service provider, they can only toggle their own status
	if claims.UserType == "serviceProvider" {
		// Check if the service provider is trying to toggle their own status
		var serviceProvider models.ServiceProvider
		err = spc.DB.Collection("serviceProviders").FindOne(context.Background(), bson.M{"_id": objID}).Decode(&serviceProvider)
		if err != nil {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Service provider not found",
			})
		}

		if serviceProvider.UserID != userID {
			return c.JSON(http.StatusForbidden, models.Response{
				Status:  http.StatusForbidden,
				Message: "Service providers can only toggle their own status",
			})
		}
	} else if claims.UserType == "admin" {
		// Admin can toggle any service provider status
	} else if claims.UserType == "manager" {
		// Check if manager has business_management role
		var manager models.Manager
		err := spc.DB.Collection("managers").FindOne(context.Background(), bson.M{"_id": userID}).Decode(&manager)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to fetch manager",
			})
		}

		// Check if manager has business_management role
		hasBusinessManagement := false
		for _, role := range manager.RolesAccess {
			if role == "business_management" {
				hasBusinessManagement = true
				break
			}
		}

		if !hasBusinessManagement {
			return c.JSON(http.StatusForbidden, models.Response{
				Status:  http.StatusForbidden,
				Message: "Manager does not have business_management role",
			})
		}
	} else if claims.UserType == "sales_manager" {
		// Check if sales manager has business_management role
		var salesManager models.SalesManager
		err := spc.DB.Collection("sales_managers").FindOne(context.Background(), bson.M{"_id": userID}).Decode(&salesManager)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, models.Response{
				Status:  http.StatusInternalServerError,
				Message: "Failed to fetch sales manager",
			})
		}

		// Check if sales manager has business_management role
		hasBusinessManagement := false
		for _, role := range salesManager.RolesAccess {
			if role == "business_management" {
				hasBusinessManagement = true
				break
			}
		}

		if !hasBusinessManagement {
			return c.JSON(http.StatusForbidden, models.Response{
				Status:  http.StatusForbidden,
				Message: "Sales manager does not have business_management role",
			})
		}
	} else {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Only service providers, admins, managers, or sales managers can toggle service provider status",
		})
	}

	// Parse request body
	var req struct {
		Status string `json:"status"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}

	// Validate status
	if req.Status != "active" && req.Status != "inactive" {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Status must be 'active' or 'inactive'",
		})
	}

	// Update the service provider status
	update := bson.M{"$set": bson.M{"status": req.Status, "updatedAt": time.Now()}}
	result, err := spc.DB.Collection("serviceProviders").UpdateOne(
		context.Background(),
		bson.M{"_id": objID},
		update,
	)
	if err != nil {
		log.Printf("Failed to update service provider status: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update service provider status",
		})
	}

	if result.MatchedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Service provider not found",
		})
	}

	// Log the action
	log.Printf("Service provider status updated: ID=%s, Status=%s, UpdatedBy=%s",
		serviceProviderID, req.Status, claims.UserID)

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: fmt.Sprintf("Service provider status updated to '%s' successfully", req.Status),
	})
}

// UpdateServiceProviderDescription allows service providers to update their description
func (spc *ServiceProviderController) UpdateServiceProviderDescription(c echo.Context) error {
	// Create context with timeout
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

	// Parse request body
	var req struct {
		Description string `json:"description"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Invalid request body",
		})
	}

	// Validate description length (optional validation)
	if len(req.Description) > 1000 {
		return c.JSON(http.StatusBadRequest, models.Response{
			Status:  http.StatusBadRequest,
			Message: "Description must be less than 1000 characters",
		})
	}

	// Find the service provider first to ensure it exists
	var serviceProvider models.ServiceProvider
	err = spc.DB.Collection("serviceProviders").FindOne(ctx, bson.M{
		"$or": []bson.M{
			{"userId": userID},
			{"createdBy": userID},
		},
	}).Decode(&serviceProvider)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "Service provider not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to find service provider",
		})
	}

	// Update the description in the serviceProviderInfo field
	update := bson.M{
		"$set": bson.M{
			"serviceProviderInfo.description": req.Description,
			"updatedAt":                       time.Now(),
		},
	}

	result, err := spc.DB.Collection("serviceProviders").UpdateOne(
		ctx,
		bson.M{"_id": serviceProvider.ID},
		update,
	)

	if err != nil {
		log.Printf("Failed to update service provider description: %v", err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Failed to update service provider description",
		})
	}

	if result.MatchedCount == 0 {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "Service provider not found",
		})
	}

	// Log the action
	log.Printf("Service provider description updated: ID=%s, UpdatedBy=%s",
		serviceProvider.ID.Hex(), claims.UserID)

	return c.JSON(http.StatusOK, models.Response{
		Status:  http.StatusOK,
		Message: "Service provider description updated successfully",
	})
}
