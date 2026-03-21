// middleware/ratelimiter.go
package middleware

import (
	"chronosphere/domain"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

var (
	ctx = context.Background()
	rdb *redis.Client
)

// RateLimitConfig defines rules for different endpoints
type RateLimitConfig struct {
	MaxRequests int           // Maximum requests
	Window      time.Duration // Time window
	Burst       int           // Burst allowance for token bucket
	Algorithm   string        // "fixed_window", "sliding_window", "token_bucket"
	Scope       string        // "ip", "user", "global"
}

// Rate limit rules based on your API analysis
var rateLimitRules = map[string]RateLimitConfig{
	// ==================== AUTHENTICATION ENDPOINTS ====================
	// These are high-risk for brute force attacks
	"ping": {
		MaxRequests: 5,
		Window:      time.Minute,
		Algorithm:   "sliding_window",
		Scope:       "ip",
	},
	"auth_register": {
		MaxRequests: 3, // 3 registrations per hour from same IP
		Window:      time.Hour,
		Algorithm:   "fixed_window",
		Scope:       "ip",
	},
	"auth_login": {
		MaxRequests: 10, // 10 login attempts per 15 minutes
		Window:      15 * time.Minute,
		Algorithm:   "sliding_window",
		Scope:       "ip",
	},
	"auth_me": {
		MaxRequests: 50,
		Window:      time.Minute,
		Algorithm:   "sliding_window",
		Scope:       "ip",
	},
	"auth_verify_otp": {
		MaxRequests: 5, // 5 OTP attempts per 10 minutes
		Window:      10 * time.Minute,
		Algorithm:   "sliding_window",
		Scope:       "ip",
	},
	"auth_forgot_password": {
		MaxRequests: 3, // 3 password reset requests per hour
		Window:      time.Hour,
		Algorithm:   "fixed_window",
		Scope:       "ip",
	},
	"auth_resend_otp": {
		MaxRequests: 3, // 3 OTP resends per hour
		Window:      time.Hour,
		Algorithm:   "fixed_window",
		Scope:       "ip",
	},
	"auth_refresh_token": {
		MaxRequests: 30, // 30 token refreshes per minute
		Window:      time.Minute,
		Algorithm:   "token_bucket",
		Burst:       5,
		Scope:       "user",
	},

	// ==================== PUBLIC/STUDENT ENDPOINTS ====================
	"public_packages": {
		MaxRequests: 60, // 60 requests per minute
		Window:      time.Minute,
		Algorithm:   "sliding_window",
		Scope:       "ip",
	},
	"student_profile": {
		MaxRequests: 30, // 30 profile views per minute
		Window:      time.Minute,
		Algorithm:   "sliding_window",
		Scope:       "user",
	},
	"student_book": {
		MaxRequests: 10, // 10 booking attempts per minute
		Window:      time.Minute,
		Algorithm:   "sliding_window",
		Scope:       "user",
	},
	"student_booked": {
		MaxRequests: 30, // 30 requests per minute
		Window:      time.Minute,
		Algorithm:   "sliding_window",
		Scope:       "user",
	},
	"student_classes": {
		MaxRequests: 60, // 60 schedule views per minute
		Window:      time.Minute,
		Algorithm:   "sliding_window",
		Scope:       "user",
	},
	"student_cancel": {
		MaxRequests: 5, // 5 cancellations per 10 minutes
		Window:      10 * time.Minute,
		Algorithm:   "fixed_window",
		Scope:       "user",
	},

	// ==================== TEACHER ENDPOINTS ====================
	"teacher_profile": {
		MaxRequests: 30, // 30 requests per minute
		Window:      time.Minute,
		Algorithm:   "sliding_window",
		Scope:       "user",
	},
	"teacher_schedules": {
		MaxRequests: 60, // 60 schedule views per minute
		Window:      time.Minute,
		Algorithm:   "sliding_window",
		Scope:       "user",
	},
	"teacher_create_class": {
		MaxRequests: 10, // 10 class creations per minute
		Window:      time.Minute,
		Algorithm:   "sliding_window",
		Scope:       "user",
	},
	"teacher_booked": {
		MaxRequests: 30, // 30 requests per minute
		Window:      time.Minute,
		Algorithm:   "sliding_window",
		Scope:       "user",
	},
	"teacher_cancel": {
		MaxRequests: 5, // 5 cancellations per 10 minutes
		Window:      10 * time.Minute,
		Algorithm:   "fixed_window",
		Scope:       "user",
	},
	"teacher_finish_class": {
		MaxRequests: 10, // 10 class completions per minute
		Window:      time.Minute,
		Algorithm:   "sliding_window",
		Scope:       "user",
	},

	// ==================== ADMIN ENDPOINTS ====================
	// Admin endpoints have higher limits but stricter IP/user scope
	"admin_create": {
		MaxRequests: 10, // 10 creations per minute
		Window:      time.Minute,
		Algorithm:   "sliding_window",
		Scope:       "user", // User-based for admin actions
	},
	"admin_modify": {
		MaxRequests: 20, // 20 modifications per minute
		Window:      time.Minute,
		Algorithm:   "sliding_window",
		Scope:       "user",
	},
	"admin_delete": {
		MaxRequests: 5, // 5 deletions per minute
		Window:      time.Minute,
		Algorithm:   "fixed_window",
		Scope:       "user",
	},
	"admin_view": {
		MaxRequests: 100, // 100 views per minute
		Window:      time.Minute,
		Algorithm:   "sliding_window",
		Scope:       "user",
	},

	// ==================== MANAGER ENDPOINTS ====================
	"manager_students": {
		MaxRequests: 60, // 60 student views per minute
		Window:      time.Minute,
		Algorithm:   "sliding_window",
		Scope:       "user",
	},
	"manager_modify": {
		MaxRequests: 20, // 20 modifications per minute
		Window:      time.Minute,
		Algorithm:   "sliding_window",
		Scope:       "user",
	},

	// ==================== GLOBAL SAFEGUARDS ====================
	"global_ip": {
		MaxRequests: 1000, // 1000 total requests per IP per minute
		Window:      time.Minute,
		Algorithm:   "sliding_window",
		Scope:       "ip",
	},
	"global_user": {
		MaxRequests: 5000, // 5000 total requests per user per minute
		Window:      time.Minute,
		Algorithm:   "sliding_window",
		Scope:       "user",
	},
}

// Initialize rate limiter
func InitRateLimiter(redisClient *redis.Client) {
	rdb = redisClient
}

// Get rate limit rule for endpoint
func getRateLimitRule(path, method string) RateLimitConfig {
	// Default rule for unknown endpoints
	defaultRule := RateLimitConfig{
		MaxRequests: 60,
		Window:      time.Minute,
		Algorithm:   "sliding_window",
		Scope:       "ip",
	}

	// Map endpoints to rate limit rules
	switch {
	// Authentication endpoints
	case strings.Contains(path, "/auth/register"):
		return rateLimitRules["auth_register"]
	case strings.Contains(path, "/ping"):
		return rateLimitRules["ping"]
	case strings.Contains(path, "/auth/login"):
		return rateLimitRules["auth_login"]
	case strings.Contains(path, "/auth/me"):
		return rateLimitRules["auth_me"]
	case strings.Contains(path, "/auth/verify-otp"):
		return rateLimitRules["auth_verify_otp"]
	case strings.Contains(path, "/auth/forgot-password"):
		return rateLimitRules["auth_forgot_password"]
	case strings.Contains(path, "/auth/resend-otp"):
		return rateLimitRules["auth_resend_otp"]
	case strings.Contains(path, "/auth/refresh-token"):
		return rateLimitRules["auth_refresh_token"]

	// Public endpoints
	case path == "/packages" && method == "GET":
		return rateLimitRules["public_packages"]

	// Student endpoints
	case strings.Contains(path, "/student/profile"):
		return rateLimitRules["student_profile"]
	case strings.Contains(path, "/student/book") && method == "POST":
		return rateLimitRules["student_book"]
	case strings.Contains(path, "/student/booked"):
		return rateLimitRules["student_booked"]
	case strings.Contains(path, "/student/classes"):
		return rateLimitRules["student_classes"]
	case strings.Contains(path, "/student/cancel/"):
		return rateLimitRules["student_cancel"]

	// Teacher endpoints
	case strings.Contains(path, "/teacher/profile"):
		return rateLimitRules["teacher_profile"]
	case strings.Contains(path, "/teacher/schedules"):
		return rateLimitRules["teacher_schedules"]
	case strings.Contains(path, "/teacher/create-available-class"):
		return rateLimitRules["teacher_create_class"]
	case strings.Contains(path, "/teacher/booked"):
		return rateLimitRules["teacher_booked"]
	case strings.Contains(path, "/teacher/cancel/"):
		return rateLimitRules["teacher_cancel"]
	case strings.Contains(path, "/teacher/finish-class/"):
		return rateLimitRules["teacher_finish_class"]

	// Admin endpoints - creation
	case (strings.Contains(path, "/admin/teachers") ||
		strings.Contains(path, "/admin/managers") ||
		strings.Contains(path, "/admin/packages") ||
		strings.Contains(path, "/admin/instruments")) && method == "POST":
		return rateLimitRules["admin_create"]

	// Admin endpoints - modification
	case (strings.Contains(path, "/modify") || strings.Contains(path, "/update")) &&
		method == "PUT":
		return rateLimitRules["admin_modify"]

	// Admin endpoints - deletion
	case method == "DELETE" && strings.Contains(path, "/admin/"):
		return rateLimitRules["admin_delete"]

	// Admin endpoints - viewing
	case method == "GET" && strings.Contains(path, "/admin/"):
		return rateLimitRules["admin_view"]

	// Manager endpoints
	case strings.Contains(path, "/manager/students"):
		return rateLimitRules["manager_students"]
	case strings.Contains(path, "/manager/modify"):
		return rateLimitRules["manager_modify"]

	default:
		return defaultRule
	}
}

// Get client identifier based on scope
func getIdentifier(c *gin.Context, scope string) string {
	switch scope {
	case "user":
		// Extract user ID from JWT token or session
		if userID, exists := c.Get("userUUID"); exists {
			return fmt.Sprintf("user:%v", userID)
		}
		// Fallback to IP if no user context
		return fmt.Sprintf("ip:%s", c.ClientIP())
	case "global":
		return "global"
	default: // "ip"
		return fmt.Sprintf("ip:%s", c.ClientIP())
	}
}

// Fixed Window Rate Limiter - Lua Script (Most Reliable)
func fixedWindowRateLimit(key string, config RateLimitConfig) (bool, int, error) {
	redisKey := fmt.Sprintf("rate:fw:%s", key)

	luaScript := `
	local key = KEYS[1]
	local expiry = ARGV[1]
	local limit = tonumber(ARGV[2])
	
	local current = redis.call('GET', key)
	
	if current == false then
		-- First request in this window
		redis.call('SET', key, 1, 'EX', expiry)
		return {1, limit - 1}
	else
		local count = tonumber(current)
		if count >= limit then
			return {count, 0}
		end
		
		-- Increment and return new count
		local new_count = redis.call('INCR', key)
		return {new_count, limit - new_count}
	end
	`

	result, err := rdb.Eval(ctx, luaScript, []string{redisKey},
		int(config.Window.Seconds()), config.MaxRequests).Result()

	if err != nil {
		return false, 0, err
	}

	results := result.([]interface{})
	current := results[0].(int64)
	remaining := results[1].(int64)

	allowed := current <= int64(config.MaxRequests)

	return allowed, int(remaining), nil
}

// Sliding Window Rate Limiter (More Accurate)
func slidingWindowRateLimit(key string, config RateLimitConfig) (bool, int, error) {
	now := time.Now().Unix()
	windowStart := now - int64(config.Window.Seconds())

	redisKey := fmt.Sprintf("rate:sw:%s", key)

	// Lua script for atomic sliding window operations
	luaScript := `
	local key = KEYS[1]
	local now = tonumber(ARGV[1])
	local window_start = tonumber(ARGV[2])
	local max_requests = tonumber(ARGV[3])
	local window_seconds = tonumber(ARGV[4])
	
	-- Remove old requests outside window
	redis.call('ZREMRANGEBYSCORE', key, 0, window_start)
	
	-- Get current count
	local current = redis.call('ZCARD', key)
	
	if current >= max_requests then
		return {0, 0}
	end
	
	-- Add current request
	redis.call('ZADD', key, now, now)
	redis.call('EXPIRE', key, window_seconds + 60) -- Extra 60 seconds for safety
	
	local remaining = max_requests - current - 1
	if remaining < 0 then remaining = 0 end
	
	return {1, remaining}
	`

	result, err := rdb.Eval(ctx, luaScript, []string{redisKey},
		now, windowStart, config.MaxRequests, int(config.Window.Seconds())).Result()

	if err != nil {
		return false, 0, err
	}

	results := result.([]interface{})
	allowed := results[0].(int64) == 1
	remaining := int(results[1].(int64))

	return allowed, remaining, nil
}

// Token Bucket Rate Limiter (Good for burst traffic)
func tokenBucketRateLimit(key string, config RateLimitConfig) (bool, int, error) {
	now := time.Now().Unix()

	redisKey := fmt.Sprintf("rate:tb:%s", key)

	luaScript := `
	local key = KEYS[1]
	local now = tonumber(ARGV[1])
	local max_tokens = tonumber(ARGV[2])
	local refill_rate = tonumber(ARGV[3]) -- tokens per second
	local burst = tonumber(ARGV[4])
	
	local bucket = redis.call('HMGET', key, 'tokens', 'last_update')
	
	local tokens = max_tokens
	local last_update = now
	
	if bucket[1] and bucket[2] then
		tokens = tonumber(bucket[1])
		last_update = tonumber(bucket[2])
		
		-- Calculate refill
		local time_passed = now - last_update
		local refill_tokens = math.floor(time_passed * refill_rate)
		
		if refill_tokens > 0 then
			tokens = math.min(max_tokens + burst, tokens + refill_tokens)
			last_update = now
		end
	end
	
	if tokens < 1 then
		-- Update bucket even when no tokens (to track last_update)
		redis.call('HMSET', key, 'tokens', tokens, 'last_update', last_update)
		redis.call('EXPIRE', key, 3600)
		return {0, 0}
	end
	
	-- Consume one token
	tokens = tokens - 1
	
	-- Update bucket
	redis.call('HMSET', key, 'tokens', tokens, 'last_update', last_update)
	redis.call('EXPIRE', key, 3600)
	
	local remaining = math.floor(tokens)
	if remaining < 0 then remaining = 0 end
	
	return {1, remaining}
	`

	// Calculate refill rate (tokens per second)
	refillRate := float64(config.MaxRequests) / config.Window.Seconds()

	result, err := rdb.Eval(ctx, luaScript, []string{redisKey},
		now, config.MaxRequests, refillRate, config.Burst).Result()

	if err != nil {
		return false, 0, err
	}

	results := result.([]interface{})
	allowed := results[0].(int64) == 1
	remaining := int(results[1].(int64))

	return allowed, remaining, nil
}

// Main Rate Limiter Middleware
// Main Rate Limiter Middleware
// Admin and manager roles are fully exempt — they typically operate from
// the same office network and low request limits would disrupt daily operations.
func RateLimiter() gin.HandlerFunc {
	return func(c *gin.Context) {
		// ── Bypass for admin and manager roles ───────────────────────────────
		// Role is set by AuthMiddleware before this runs on protected routes.
		// On public routes role will be empty — that's fine, they still get limited.
		if role, exists := c.Get("role"); exists {
			r, _ := role.(string)
			if r == domain.RoleAdmin || r == domain.RoleManagement {
				c.Next()
				return
			}
		}

		// ── Get rate limit rule for this endpoint ─────────────────────────────
		rule := getRateLimitRule(c.Request.URL.Path, c.Request.Method)

		// ── Get identifier based on scope ─────────────────────────────────────
		identifier := getIdentifier(c, rule.Scope)
		key := fmt.Sprintf("%s:%s:%s", rule.Scope, c.Request.Method, c.Request.URL.Path)
		fullKey := fmt.Sprintf("%s:%s", key, identifier)

		// ── Global IP safeguard ───────────────────────────────────────────────
		ipIdentifier := fmt.Sprintf("ip:%s", c.ClientIP())
		globalIPKey := fmt.Sprintf("global:ip:%s", ipIdentifier)

		globalAllowed, _, err := slidingWindowRateLimit(globalIPKey, rateLimitRules["global_ip"])
		if err != nil || !globalAllowed {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "Batas permintaan global terlampaui",
				"code":  "RATE_LIMIT_GLOBAL_IP",
			})
			c.Abort()
			return
		}

		// ── Global user safeguard (authenticated non-admin/manager users) ─────
		if userID, exists := c.Get("userUUID"); exists {
			userIdentifier := fmt.Sprintf("user:%v", userID)
			globalUserKey := fmt.Sprintf("global:user:%s", userIdentifier)

			userAllowed, _, err := slidingWindowRateLimit(globalUserKey, rateLimitRules["global_user"])
			if err != nil || !userAllowed {
				c.JSON(http.StatusTooManyRequests, gin.H{
					"error": "Batas permintaan pengguna terlampaui",
					"code":  "RATE_LIMIT_GLOBAL_USER",
				})
				c.Abort()
				return
			}
		}

		// ── Endpoint-specific rate limiting ───────────────────────────────────
		var allowed bool
		var remaining int

		switch rule.Algorithm {
		case "fixed_window":
			allowed, remaining, err = fixedWindowRateLimit(fullKey, rule)
		case "token_bucket":
			allowed, remaining, err = tokenBucketRateLimit(fullKey, rule)
		default: // sliding_window
			allowed, remaining, err = slidingWindowRateLimit(fullKey, rule)
		}

		if err != nil {
			// Don't block requests if Redis fails
			c.Next()
			return
		}

		if !allowed {
			logRateLimitBlock(c, rule, identifier)

			c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", rule.MaxRequests))
			c.Header("X-RateLimit-Remaining", "0")
			c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Add(rule.Window).Unix()))

			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       fmt.Sprintf("Permintaan terlalu sering, harap coba lagi dalam %v", rule.Window.String()),
				"code":        "RATE_LIMIT_EXCEEDED",
				"retry_after": int(rule.Window.Seconds()),
				"limit":       rule.MaxRequests,
				"window":      rule.Window.String(),
			})
			c.Abort()
			return
		}

		c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", rule.MaxRequests))
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Add(rule.Window).Unix()))

		c.Next()
	}
}

// Log rate limit blocks for monitoring
func logRateLimitBlock(c *gin.Context, rule RateLimitConfig, identifier string) {
	// In production, you'd want to log this to your monitoring system
	// For now, we'll just print to console
	fmt.Printf("[RATE_LIMIT] Blocked request: %s %s | Identifier: %s | Rule: %+v\n",
		c.Request.Method, c.Request.URL.Path, identifier, rule)
}

// Admin endpoint to view rate limit status
func RateLimitStatusHandler(c *gin.Context) {
	// Only accessible to admins
	userRole, exists := c.Get("role")
	if !exists || userRole != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Forbidden"})
		return
	}

	// Get current counts for debugging
	status := gin.H{
		"rules":     rateLimitRules,
		"timestamp": time.Now().Unix(),
	}

	c.JSON(http.StatusOK, status)
}

// Cleanup expired rate limit keys (run as background job)
func CleanupExpiredRateLimits() {
	ticker := time.NewTicker(time.Hour)

	go func() {
		for range ticker.C {
			// Scan and delete old rate limit keys
			// This is optional as Redis automatically expires keys
			// but good for cleanup
			ctx := context.Background()
			iter := rdb.Scan(ctx, 0, "rate:*", 1000).Iterator()

			for iter.Next(ctx) {
				key := iter.Val()
				ttl, err := rdb.TTL(ctx, key).Result()
				if err == nil && ttl < 0 {
					// Key has no expiry, delete it
					rdb.Del(ctx, key)
				}
			}
		}
	}()
}
