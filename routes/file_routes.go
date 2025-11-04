package routes

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/HSouheill/barrim_backend/models"
	"github.com/labstack/echo/v4"
)

// RegisterFileRoutes sets up all file serving routes
func RegisterFileRoutes(e *echo.Echo) {
	// File serving routes
	e.GET("/uploads/*", ServeFile)
	e.GET("/uploads/bookings/*", ServeFile)
	e.GET("/uploads/thumbnails/*", ServeFile)
	e.GET("/uploads/vouchers/*", ServeFile)
	e.GET("/uploads/:filename", ServeImage)
}

// ServeImage handles serving uploaded images
func ServeImage(c echo.Context) error {
	path := c.Param("*")
	if path == "" {
		path = c.Param("filename")
	}

	// Try various potential paths
	potentialPaths := []string{
		filepath.Join("uploads", path),
		filepath.Join("uploads", "serviceprovider", path),
		filepath.Join("uploads", "serviceprovider", "portfolio", path),
		filepath.Join("uploads", "profiles", path),
		filepath.Join("uploads", "logos", path),
		filepath.Join("uploads", "vouchers", path),
	}

	for _, filePath := range potentialPaths {
		if _, err := os.Stat(filePath); !os.IsNotExist(err) {
			return c.File(filePath)
		}
	}

	return c.JSON(http.StatusNotFound, models.Response{
		Status:  http.StatusNotFound,
		Message: "Image not found",
	})
}

// ServeFile handles serving uploaded files with proper security checks
func ServeFile(c echo.Context) error {
	path := c.Param("*")
	if path == "" {
		return c.JSON(http.StatusNotFound, models.Response{
			Status:  http.StatusNotFound,
			Message: "File not found - no path provided",
		})
	}

	// Clean the path to prevent directory traversal
	cleanPath := filepath.Clean(path)
	if cleanPath == ".." || strings.HasPrefix(cleanPath, "../") {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Access denied - invalid path",
		})
	}

	// Construct full file path
	fullPath := filepath.Join("uploads", cleanPath)

	// Debug logging
	log.Printf("Attempting to serve file: %s", fullPath)

	// Check if file exists
	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("File not found: %s", fullPath)
			return c.JSON(http.StatusNotFound, models.Response{
				Status:  http.StatusNotFound,
				Message: "File not found: " + fullPath,
			})
		}
		log.Printf("Error accessing file %s: %v", fullPath, err)
		return c.JSON(http.StatusInternalServerError, models.Response{
			Status:  http.StatusInternalServerError,
			Message: "Error accessing file: " + err.Error(),
		})
	}

	// Don't allow directory listing
	if info.IsDir() {
		return c.JSON(http.StatusForbidden, models.Response{
			Status:  http.StatusForbidden,
			Message: "Access denied - directory listing not allowed",
		})
	}

	// Set cache headers
	c.Response().Header().Set("Cache-Control", "public, max-age=31536000") // Cache for 1 year
	c.Response().Header().Set("Expires", time.Now().AddDate(1, 0, 0).Format(time.RFC1123))

	log.Printf("Successfully serving file: %s", fullPath)
	// Serve the file
	return c.File(fullPath)
}
