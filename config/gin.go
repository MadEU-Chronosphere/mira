package config

import (
	"chronosphere/middleware"
	"chronosphere/utils"
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func InitMiddleware(app *gin.Engine) {
	// ✅ Protect against panic in middleware setup
	defer func() {
		if r := recover(); r != nil {
			log.Printf("🔥 Middleware initialization panic recovered: %v", r)
		}
	}()

	// CORS Middleware with safe defaults
	corsOrigins := os.Getenv("ALLOW_ORIGINS")
	if corsOrigins == "" {
		corsOrigins = "http://localhost:3000"
		log.Println("⚠️  ALLOW_ORIGINS not set, using default: http://localhost:3000")
	}

	app.Use(cors.New(cors.Config{
		AllowOrigins:     strings.Split(corsOrigins, ","),
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Logging & Recovery (built-in Gin recovery)
	app.Use(gin.Recovery())

	// Security Headers
	app.Use(securityHeadersMiddleware())

	// Timeout Middleware with error handling
	app.Use(timeoutMiddleware(30 * time.Second))

	// Rate limiter
	app.Use(middleware.RateLimiter())

}

// ✅ Security headers middleware with error protection
func securityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("🔥 Security headers panic recovered: %v", r)
				c.Next()
			}
		}()

		c.Writer.Header().Set("X-Content-Type-Options", "nosniff")
		c.Writer.Header().Set("X-Frame-Options", "DENY")
		c.Writer.Header().Set("X-XSS-Protection", "1; mode=block")
		c.Writer.Header().Set("Content-Security-Policy", "default-src 'self'")

		// Only set HSTS in production with HTTPS
		if os.Getenv("APP_ENV") == "production" {
			c.Writer.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}

		c.Writer.Header().Set("Referrer-Policy", "no-referrer")
		c.Next()
	}
}

// ✅ Timeout middleware with safe error handling
func timeoutMiddleware(timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("🔥 Timeout middleware panic recovered: %v", r)
				c.JSON(http.StatusInternalServerError, gin.H{
					"success": false,
					"message": "Internal server error",
					"error":   "server_error",
				})
				c.Abort()
			}
		}()

		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()

		c.Request = c.Request.WithContext(ctx)

		finished := make(chan struct{})
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("🔥 Request handler panic in timeout middleware: %v", r)
				}
			}()

			c.Next()
			close(finished)
		}()

		select {
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				c.JSON(http.StatusGatewayTimeout, gin.H{
					"success": false,
					"message": "Request timed out",
					"error":   "timeout",
				})
				c.Abort()
			}
		case <-finished:
			// Request completed normally
		}
	}
}

func AuthMiddleware(jwtManager *utils.JWTManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		// ✅ Protect against panic
		defer func() {
			if r := recover(); r != nil {
				log.Printf("🔥 Auth middleware panic recovered: %v", r)
				c.JSON(http.StatusInternalServerError, gin.H{
					"success": false,
					"message": "Authentication error",
					"error":   "auth_error",
				})
				c.Abort()
			}
		}()

		authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "Need Credential to Access this Resource (Authorization header missing)",
			})
			c.Abort()
			return
		}

		if !strings.HasPrefix(authHeader, "Bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "Invalid authorization header format",
			})
			c.Abort()
			return
		}

		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

		// ✅ Safe token verification
		userUUID, role, name, err := func() (string, string, string, error) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("🔥 Token verification panic: %v", r)
				}
			}()
			return jwtManager.VerifyToken(tokenStr)
		}()

		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "Invalid or expired token",
				"error":   "invalid_token",
			})
			c.Abort()
			return
		}

		// Save to context
		c.Set("userUUID", userUUID)
		c.Set("role", role)
		c.Set("name", name)

		c.Next()
	}
}
