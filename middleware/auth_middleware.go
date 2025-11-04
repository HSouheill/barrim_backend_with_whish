// middleware/auth_middleware.go
package middleware

import (
	"net/http"

	"github.com/HSouheill/barrim_backend/models"
	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// RequireUserType checks if the authenticated user has one of the allowed user types
func RequireUserType(allowedTypes ...string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Get user type from context or token
			userType := ExtractUserType(c)
			c.Logger().Infof("RequireUserType middleware - Path: %s, UserType: %s, AllowedTypes: %v",
				c.Request().URL.Path, userType, allowedTypes)

			// If no user type found, deny access
			if userType == "" {
				c.Logger().Error("Authentication failed: user type not found")
				return c.JSON(http.StatusUnauthorized, models.Response{
					Status:  http.StatusUnauthorized,
					Message: "Authentication failed: user type not found",
				})
			}

			// Check if user type is allowed
			for _, allowedType := range allowedTypes {
				// Handle both formats for sales manager
				if (userType == "sales_manager" && allowedType == "salesManager") ||
					(userType == "salesManager" && allowedType == "sales_manager") ||
					userType == allowedType {
					c.Logger().Infof("Access granted for user type: %s", userType)
					return next(c)
				}
			}

			c.Logger().Errorf("Access denied for user type: %s, allowed types: %v", userType, allowedTypes)
			return c.JSON(http.StatusForbidden, models.Response{
				Status:  http.StatusForbidden,
				Message: "Access denied for your user type",
			})
		}
	}
}

// DebugMiddleware prints token info for debugging
func DebugMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			claims := GetUserFromToken(c)
			if claims != nil {
				c.Logger().Infof("User ID: %s, User Type: %s, Email: %s",
					claims.UserID, claims.UserType, claims.Email)
			} else {
				c.Logger().Info("No user claims found")
			}
			return next(c)
		}
	}
}

// RequireFinancialDashboardAccess allows only admin, or sales manager/manager with 'financial_dashboard_revenue' access
func RequireFinancialDashboardAccess() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			userType := ExtractUserType(c)
			userID := GetUserIDFromToken(c)

			if userType == "admin" {
				return next(c)
			}

			// Get DB from context (assume it's set as 'db' in echo.Context)
			db, ok := c.Get("db").(*mongo.Client)
			if !ok || db == nil {
				return c.JSON(http.StatusInternalServerError, models.Response{
					Status:  http.StatusInternalServerError,
					Message: "Database connection not found in context",
				})
			}

			if userType == "manager" {
				var manager models.Manager
				err := db.Database("barrim").Collection("managers").FindOne(c.Request().Context(), bson.M{"_id": userID}).Decode(&manager)
				if err != nil {
					return c.JSON(http.StatusForbidden, models.Response{
						Status:  http.StatusForbidden,
						Message: "Manager not found or unauthorized",
					})
				}
				if hasRole(manager.RolesAccess, "financial_dashboard_revenue") {
					return next(c)
				}
			}
			if userType == "sales_manager" {
				var salesManager models.SalesManager
				err := db.Database("barrim").Collection("sales_managers").FindOne(c.Request().Context(), bson.M{"_id": userID}).Decode(&salesManager)
				if err != nil {
					return c.JSON(http.StatusForbidden, models.Response{
						Status:  http.StatusForbidden,
						Message: "Sales manager not found or unauthorized",
					})
				}
				if hasRole(salesManager.RolesAccess, "financial_dashboard_revenue") {
					return next(c)
				}
			}
			return c.JSON(http.StatusForbidden, models.Response{
				Status:  http.StatusForbidden,
				Message: "Access denied: insufficient permissions",
			})
		}
	}
}

// hasRole checks if the given role exists in the roles slice
func hasRole(roles []string, required string) bool {
	for _, r := range roles {
		if r == required {
			return true
		}
	}
	return false
}
