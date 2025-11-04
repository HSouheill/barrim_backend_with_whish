package middleware

import (
	"os"
	"strings"

	"github.com/labstack/echo/v4"
	echoMiddleware "github.com/labstack/echo/v4/middleware"
)

// CORSConfig holds CORS configuration
type CORSConfig struct {
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	AllowCredentials bool
	ExposeHeaders    []string
	MaxAge           int
}

// NewCORSConfig creates a new CORS configuration with environment-based origins
func NewCORSConfig() *CORSConfig {
	// Default origins
	origins := []string{
		"http://localhost:3000", // React dev server
		"http://localhost:3001", // Alternative React port
		"http://localhost:8080", // Alternative dev port
		"https://barrim.online",
		"https://www.barrim.online",
		"https://barrim.com",
		"https://www.barrim.com",
	}

	// Add origins from environment variable if set
	if envOrigins := os.Getenv("CORS_ALLOWED_ORIGINS"); envOrigins != "" {
		envOriginList := strings.Split(envOrigins, ",")
		for _, origin := range envOriginList {
			trimmedOrigin := strings.TrimSpace(origin)
			if trimmedOrigin != "" {
				origins = append(origins, trimmedOrigin)
			}
		}
	}

	return &CORSConfig{
		AllowOrigins:     origins,
		AllowMethods:     []string{"GET", "HEAD", "PUT", "PATCH", "POST", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With", "X-API-Key"},
		AllowCredentials: true,
		ExposeHeaders:    []string{"Content-Length", "Content-Type"},
		MaxAge:           86400, // 24 hours
	}
}

// GlobalCORS creates a global CORS middleware
func GlobalCORS() echo.MiddlewareFunc {
	config := NewCORSConfig()

	return echoMiddleware.CORSWithConfig(echoMiddleware.CORSConfig{
		AllowOrigins:     config.AllowOrigins,
		AllowMethods:     config.AllowMethods,
		AllowHeaders:     config.AllowHeaders,
		AllowCredentials: config.AllowCredentials,
		ExposeHeaders:    config.ExposeHeaders,
		MaxAge:           config.MaxAge,
	})
}

// CORSWithConfig creates a CORS middleware with custom configuration
func CORSWithConfig(config *CORSConfig) echo.MiddlewareFunc {
	return echoMiddleware.CORSWithConfig(echoMiddleware.CORSConfig{
		AllowOrigins:     config.AllowOrigins,
		AllowMethods:     config.AllowMethods,
		AllowHeaders:     config.AllowHeaders,
		AllowCredentials: config.AllowCredentials,
		ExposeHeaders:    config.ExposeHeaders,
		MaxAge:           config.MaxAge,
	})
}

// PreflightHandler handles preflight CORS requests
func PreflightHandler() echo.HandlerFunc {
	return func(c echo.Context) error {
		c.Response().Header().Set("Access-Control-Allow-Origin", "*")
		c.Response().Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Response().Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, X-Requested-With, X-API-Key")
		c.Response().Header().Set("Access-Control-Max-Age", "86400")
		return c.NoContent(204)
	}
}
