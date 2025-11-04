// middleware/rate_limiter.go
package middleware

import (
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"golang.org/x/time/rate"
)

type RateLimiter struct {
	ips            map[string]*rate.Limiter
	blockedIPs     map[string]time.Time
	mu             *sync.RWMutex
	defaultLimit   rate.Limit
	defaultBurst   int
	blockDuration  time.Duration
	endpointLimits map[string]struct {
		limit rate.Limit
		burst int
	}
}

func NewRateLimiter() *RateLimiter {
	limiter := &RateLimiter{
		ips:           make(map[string]*rate.Limiter),
		blockedIPs:    make(map[string]time.Time),
		mu:            &sync.RWMutex{},
		defaultLimit:  rate.Every(100 * time.Millisecond), // 10 requests per second
		defaultBurst:  20,                                 // Allow bursts of 20 requests
		blockDuration: 5 * time.Minute,                    // Block for 5 minutes instead of 1 hour
		endpointLimits: make(map[string]struct {
			limit rate.Limit
			burst int
		}),
	}

	// Set specific limits for different endpoints
	// Login endpoint - strict rate limiting to prevent brute force attacks
	limiter.endpointLimits["/api/admin/login"] = struct {
		limit rate.Limit
		burst int
	}{
		limit: rate.Every(2 * time.Second), // 0.5 requests per second (1 every 2 seconds)
		burst: 5,                           // Allow burst of 5 attempts
	}

	limiter.endpointLimits["/api/auth/signup"] = struct {
		limit rate.Limit
		burst int
	}{
		limit: rate.Every(500 * time.Millisecond), // 2 requests per second
		burst: 5,
	}

	// Add more lenient limits for branch-related endpoints
	limiter.endpointLimits["/api/wholesalers"] = struct {
		limit rate.Limit
		burst int
	}{
		limit: rate.Every(50 * time.Millisecond), // 20 requests per second
		burst: 50,
	}

	limiter.endpointLimits["/api/wholesalers/branches"] = struct {
		limit rate.Limit
		burst int
	}{
		limit: rate.Every(50 * time.Millisecond), // 20 requests per second
		burst: 50,
	}

	// Start cleanup routine
	go limiter.cleanupBlockedIPs()

	return limiter
}

func (r *RateLimiter) cleanupBlockedIPs() {
	for {
		time.Sleep(1 * time.Hour)
		r.mu.Lock()
		now := time.Now()
		for ip, blockUntil := range r.blockedIPs {
			if now.After(blockUntil) {
				delete(r.blockedIPs, ip)
				// Also remove the limiter to reset its state
				delete(r.ips, ip)
			}
		}
		r.mu.Unlock()
	}
}

func (r *RateLimiter) RateLimit() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ip := c.RealIP()

			// Exclude /uploads path from rate limiting
			ds := c.Request().URL.Path
			if strings.HasPrefix(ds, "/uploads/") {
				return next(c)
			}

			// Check if IP is blocked and handle expired blocks
			r.mu.Lock()
			if blockUntil, blocked := r.blockedIPs[ip]; blocked {
				if time.Now().Before(blockUntil) {
					r.mu.Unlock()
					return c.JSON(429, map[string]string{
						"message":    "IP address blocked due to too many requests",
						"retryAfter": blockUntil.Format(time.RFC3339),
					})
				}
				// Block has expired - remove it and reset the limiter
				delete(r.blockedIPs, ip)
				delete(r.ips, ip) // Reset the limiter state
			}
			r.mu.Unlock()

			// Get endpoint-specific limits
			path := c.Path()
			limit := r.defaultLimit
			burst := r.defaultBurst

			if endpointLimit, exists := r.endpointLimits[path]; exists {
				limit = endpointLimit.limit
				burst = endpointLimit.burst
			}

			limiter := r.getLimiter(ip, limit, burst)
			if !limiter.Allow() {
				// Block the IP
				r.mu.Lock()
				r.blockedIPs[ip] = time.Now().Add(r.blockDuration)
				r.mu.Unlock()

				return c.JSON(429, map[string]string{
					"message":    "Too many requests",
					"retryAfter": time.Now().Add(r.blockDuration).Format(time.RFC3339),
				})
			}

			return next(c)
		}
	}
}

func (r *RateLimiter) getLimiter(ip string, limit rate.Limit, burst int) *rate.Limiter {
	r.mu.Lock()
	defer r.mu.Unlock()

	limiter, exists := r.ips[ip]
	if !exists {
		limiter = rate.NewLimiter(limit, burst)
		r.ips[ip] = limiter
	}
	return limiter
}
