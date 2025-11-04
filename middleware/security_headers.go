// middleware/security_headers.go
package middleware

import (
	"strings"

	"github.com/labstack/echo/v4"
)

func SecurityHeaders() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Response().Header().Set("X-Frame-Options", "DENY")
			c.Response().Header().Set("X-Content-Type-Options", "nosniff")
			c.Response().Header().Set("X-XSS-Protection", "1; mode=block")
			c.Response().Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			c.Response().Header().Set("Content-Security-Policy", "default-src 'self'")
			return next(c)
		}
	}
}

func SecurityHeadersWithConfig(config SecurityConfig) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			h := c.Response().Header()

			// Security headers
			h.Set("X-Frame-Options", "DENY")
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-XSS-Protection", "1; mode=block")
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
			h.Set("Content-Security-Policy", buildCSP(config))
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

			// Remove potentially sensitive headers
			h.Del("Server")
			h.Del("X-Powered-By")

			return next(c)
		}
	}
}

type SecurityConfig struct {
	AllowedDomains []string
	AllowInlineJS  bool
	AllowEval      bool
}

func buildCSP(config SecurityConfig) string {
	csp := []string{
		"default-src 'self'",
		"img-src 'self' data: https:",
		"style-src 'self' 'unsafe-inline'",
	}

	if config.AllowInlineJS {
		csp = append(csp, "script-src 'self' 'unsafe-inline'")
	} else {
		csp = append(csp, "script-src 'self'")
	}

	if len(config.AllowedDomains) > 0 {
		csp = append(csp, "connect-src 'self' "+strings.Join(config.AllowedDomains, " "))
	}

	return strings.Join(csp, "; ")
}
