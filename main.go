package main

import (
	"log"
	"os"
	"time"

	"mime"

	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	echoMiddleware "github.com/labstack/echo/v4/middleware"

	// "go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/HSouheill/barrim_backend/config"
	"github.com/HSouheill/barrim_backend/controllers"
	"github.com/HSouheill/barrim_backend/middleware"
	"github.com/HSouheill/barrim_backend/repositories"
	"github.com/HSouheill/barrim_backend/routes"
	"github.com/HSouheill/barrim_backend/websocket"
)

// CustomValidator is a custom validator for Echo
type CustomValidator struct {
	validator *validator.Validate
}

// Validate validates the request body
func (cv *CustomValidator) Validate(i interface{}) error {
	return cv.validator.Struct(i)
}

func main() {
	// Load .env file
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: .env file not found")
	}

	// Ensure correct MIME type for SVG files
	_ = mime.AddExtensionType(".svg", "image/svg+xml")

	// Initialize Firebase
	config.InitFirebase()

	// Connect to Redis
	config.ConnectRedis()

	// Connect to database
	client := config.ConnectDB()
	barrimDB := client.Database("barrim") // Ensure consistent database reference

	// Create WebSocket hub
	wsHub := websocket.NewHub()
	go wsHub.Run()

	// Create a new Echo instance
	e := echo.New()

	// Initialize custom validator
	customValidator := &CustomValidator{validator: validator.New()}
	e.Validator = customValidator

	// Initialize rate limiter
	rateLimiter := middleware.NewRateLimiter()

	// Middleware
	e.Use(echoMiddleware.Logger())
	e.Use(echoMiddleware.Recover())
	e.Use(echoMiddleware.CORS())
	e.Use(echoMiddleware.Secure())
	e.Use(rateLimiter.RateLimit())
	e.Use(middleware.SecurityHeadersWithConfig(middleware.SecurityConfig{
		AllowedDomains: []string{"*"}, // Configure this based on your needs
		AllowInlineJS:  true,          // Set to false in production
		AllowEval:      false,
	}))

	e.Match([]string{"GET", "HEAD"}, "/", func(c echo.Context) error {
		return c.JSON(200, map[string]string{
			"status":  "OK",
			"message": "Barrim Backend is running",
			"version": "1.0",
		})
	})

	e.Match([]string{"GET", "HEAD"}, "/health", func(c echo.Context) error {
		return c.JSON(200, map[string]string{
			"status":   "healthy",
			"database": "connected",
		})
	})

	e.Use(httpsRedirect())

	authGroup := e.Group("/api")
	authGroup.Use(middleware.JWTMiddleware())
	authGroup.Use(middleware.ActivityTracker(client))

	// Initialize repositories
	userRepo := repositories.NewUserRepository(client)

	// Initialize controllers
	authController := controllers.NewAuthController(client)
	userController := controllers.NewUserController(client, userRepo)
	companyController := controllers.NewCompanyController(client)
	companyReferralController := controllers.NewCompanyReferralController(client)
	subscriptionController := controllers.NewSubscriptionController(barrimDB)
	branchSubscriptionController := controllers.NewBranchSubscriptionController(barrimDB)
	companyVoucherController := controllers.NewCompanyVoucherController(barrimDB)
	serviceProviderVoucherController := controllers.NewServiceProviderVoucherController(barrimDB)
	wholesalerVoucherController := controllers.NewWholesalerVoucherController(barrimDB)

	// Register company routes
	routes.RegisterCompanyRoutes(e, companyController, companyReferralController, subscriptionController, branchSubscriptionController, companyVoucherController)

	// Register wholesaler routes (including subscription and referral routes)
	// Use the same barrimDB instance consistently
	routes.RegisterWholesalerRoutes(e, barrimDB, wholesalerVoucherController)
	routes.RegisterWholesalerReferralRoutes(e, client)

	// Register service provider routes
	routes.RegisterServiceProviderRoutes(e, barrimDB, serviceProviderVoucherController)

	// Register unified referral routes (handles all user types)
	routes.RegisterUnifiedReferralRoutes(e, client)

	// Add public WebSocket endpoint (no authentication required for initial connection)
	// e.GET("/api/ws", func(c echo.Context) error {
	// 	// Handle WebSocket upgrade without authentication
	// 	// Authentication can be handled after connection is established
	// 	return websocket.HandleWebSocket(c, wsHub, primitive.NilObjectID)
	// })

	// Setup remaining routes
	routes.SetupRoutes(e, client, wsHub, authController, userController)

	// Register admin routes AFTER general routes to avoid conflicts
	routes.RegisterAdminRoutes(e, barrimDB, wsHub)

	// Start the inactive user checker in a goroutine
	go func() {
		for {
			middleware.MarkInactiveUsers(client, 30*time.Minute)
			time.Sleep(5 * time.Minute)
		}
	}()

	// Ensure uploads directory exists
	os.MkdirAll("uploads", 0755)
	os.MkdirAll("uploads/vouchers", 0755)
	os.MkdirAll("uploads/bookings", 0755)
	os.MkdirAll("uploads/category", 0755)
	os.MkdirAll("uploads/certificates", 0755)
	os.MkdirAll("uploads/companies", 0755)
	os.MkdirAll("uploads/logo", 0755)
	os.MkdirAll("uploads/logos", 0755)
	os.MkdirAll("uploads/profiles", 0755)
	os.MkdirAll("uploads/serviceprovider", 0755)
	os.MkdirAll("uploads/videos", 0755)
	os.MkdirAll("uploads/thumbnails", 0755)
	os.MkdirAll("uploads/reviews", 0755)

	// Add this to your Echo server setup
	e.Static("/uploads", "uploads")

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	e.Logger.Fatal(e.Start(":" + port))
}

func httpsRedirect() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if c.Request().Header.Get("X-Forwarded-Proto") == "http" {
				return c.Redirect(301, "https://"+c.Request().Host+c.Request().RequestURI)
			}
			return next(c)
		}
	}
}

//ios
