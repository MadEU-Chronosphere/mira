package delivery

import (
	"chronosphere/config"
	"chronosphere/domain"
	"chronosphere/middleware"
	"chronosphere/utils"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AuthHandler struct {
	authUC domain.AuthUseCase
}

func NewAuthHandler(r *gin.Engine, authUC domain.AuthUseCase, db *gorm.DB) {
	handler := &AuthHandler{authUC: authUC}

	// Ping Route (no rate limiting)
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "pong",
		})
	})

	// Public routes with stricter rate limiting for auth
	public := r.Group("/auth")
	{
		public.POST("/register", handler.Register)
		public.POST("/verify-otp", handler.VerifyOTP)
		loginRateLimiter := r.Group("/auth")
		loginRateLimiter.POST("/login", handler.Login)
		public.POST("/forgot-password", handler.ForgotPassword)
		public.POST("/reset-password", handler.ResetPassword)
		public.POST("/resend-otp", handler.ResendOTP)
		public.POST("/refresh-token", handler.RefreshToken)
		public.POST("/logout", handler.Logout)
	}

	// Protected routes (use global rate limiting)
	protected := r.Group("/auth")
	protected.Use(config.AuthMiddleware(handler.authUC.GetAccessTokenManager()), middleware.ValidateTurnedOffUserMiddleware(db))
	{
		protected.GET("/me", handler.Me)
		protected.POST("/change-password", handler.ChangePassword)
		protected.POST("/change-email", handler.ChangeEmail)
	}
}

type ChangeEmailRequest struct {
	NewEmail string `json:"new_email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

func (h *AuthHandler) ChangeEmail(c *gin.Context) {
	userUUID, exists := c.Get("userUUID")
	if !exists {
		utils.PrintLogInfo(nil, 401, "ChangePassword", nil)
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Failed to get user from context",
			"error":   "unauthorized"})
		return
	}

	var req ChangeEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.PrintLogInfo(nil, 400, "ChangeEmail", &err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request",
			"error":   utils.TranslateValidationError(err)})
		return
	}

	if err := h.authUC.ChangeEmail(c.Request.Context(), userUUID.(string), strings.ToLower(req.NewEmail), req.Password); err != nil {
		utils.PrintLogInfo(nil, 401, "ChangeEmail", &err)
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Failed to change email",
			"error":   err.Error()})
		return
	}
	utils.PrintLogInfo(nil, 200, "ChangeEmail", nil)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Email changed successfully"})

}

func (h *AuthHandler) Logout(c *gin.Context) {
	// ✅ Clear cookie (for web)
	c.SetCookie(
		"refresh_token",
		"",
		-1, // Expire immediately
		"/",
		"",    // domain
		false, // secure=false for dev
		true,  // HttpOnly
	)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Logout successful",
	})
}

func (h *AuthHandler) RefreshToken(c *gin.Context) {
	// Try to read refresh token from cookie (for Web)
	refreshToken, err := c.Cookie("refresh_token")
	if err != nil || refreshToken == "" {
		// If not found in cookie, try from JSON body (for Mobile)
		var req struct {
			RefreshToken string `json:"refresh_token"`
		}
		if bindErr := c.ShouldBindJSON(&req); bindErr != nil || req.RefreshToken == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "No refresh token provided",
			})
			return
		}
		refreshToken = req.RefreshToken
	}

	// ✅ Verify refresh token
	userUUID, role, name, err := h.authUC.GetRefreshTokenManager().VerifyToken(refreshToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Invalid or expired refresh token",
		})
		return
	}

	// ✅ Check if user still exists in database
	_, err = h.authUC.Me(c.Request.Context(), userUUID)
	if err != nil {
		// User deleted - clear cookie and reject
		c.SetCookie("refresh_token", "", -1, "/", "", false, true)
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "User account not found",
			"error":   "user_deleted",
		})
		return
	}

	// ✅ Generate new access token
	newAccessToken, err := h.authUC.GetAccessTokenManager().GenerateToken(userUUID, role, name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to generate new access token",
			"error":   err.Error(),
		})
		return
	}

	// ✅ (Optional) Generate new refresh token for long sessions
	newRefreshToken, err := h.authUC.GetRefreshTokenManager().GenerateToken(userUUID, role, name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to generate new refresh token",
			"error":   err.Error(),
		})
		return
	}

	// ✅ For web clients, update HttpOnly cookie
	c.SetCookie(
		"refresh_token",
		newRefreshToken,
		60*60*24*7, // 7 days
		"/",
		"",    // ✅ replace in prod
		false, // ✅ secure cookies (HTTPS only)
		true,  // ✅ HttpOnly
	)

	// ✅ Return new access token
	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"message":       "Token refreshed successfully",
		"access_token":  newAccessToken,
		"refresh_token": newRefreshToken,
	})
}

type ResendOTPRequest struct {
	Email string `json:"email" binding:"required,email"`
}

func (h *AuthHandler) Me(c *gin.Context) {
	uuidVal, existsUUID := c.Get("userUUID")
	roleVal, existsRole := c.Get("role")
	if !existsUUID || !existsRole {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Unauthorized: missing user context",
		})
		return
	}

	userUUID, ok := uuidVal.(string)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Invalid user UUID type",
		})
		return
	}

	role, _ := roleVal.(string)

	user, err := h.authUC.Me(c.Request.Context(), userUUID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"role":    role,
		"data":    user,
	})
}

func (h *AuthHandler) ResendOTP(c *gin.Context) {
	var req ResendOTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.PrintLogInfo(nil, 400, "ResendOTP", &err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request",
			"error":   err.Error(),
		})
		return
	}

	if err := h.authUC.ResendOTP(c.Request.Context(), req.Email); err != nil {
		utils.PrintLogInfo(&req.Email, 500, "ResendOTP", &err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to resend OTP",
			"error":   err.Error(),
		})
		return
	}

	utils.PrintLogInfo(&req.Email, 200, "ResendOTP", nil)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "OTP resent successfully",
	})
}

type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required,min=8,max=64"`
	NewPassword string `json:"new_password" binding:"required,min=8,max=64"`
}

type RegisterRequest struct {
	Name     string `json:"name" binding:"required,min=3,max=50"`
	Phone    string `json:"phone" binding:"required,min=10,max=14,numeric"`
	Email    string `json:"email" binding:"required,email"`
	Gender   string `json:"gender" binding:"required,oneof=male female"`
	Password string `json:"password" binding:"required,min=8,max=64"`
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.PrintLogInfo(nil, 400, "Register", &err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Failed to register",
			"error":   utils.TranslateValidationError(err),
		})
		return
	}

	// role hardcoded student
	if err := h.authUC.Register(
		c.Request.Context(),
		strings.ToLower(req.Email),
		req.Name,
		req.Phone,
		req.Password,
		req.Gender,
	); err != nil {
		utils.PrintLogInfo(&req.Email, 409, "Register", &err)
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"message": "Failed to register",
			"error":   err.Error(),
		})
		return
	}
	utils.PrintLogInfo(&req.Email, 200, "Register", nil)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "OTP sent to your email",
	})
}

type VerifyOTPRequest struct {
	Email string `json:"email" binding:"required,email"`
	OTP   string `json:"otp" binding:"required"`
}

func (h *AuthHandler) VerifyOTP(c *gin.Context) {
	var req VerifyOTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.PrintLogInfo(nil, 400, "VerifyOTP", &err)
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Invalid request",
			"success": false,
			"error":   err.Error()})
		return
	}
	if err := h.authUC.VerifyOTP(c.Request.Context(), req.Email, req.OTP); err != nil {
		utils.PrintLogInfo(&req.Email, 401, "VerifyOTP", &err)
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "Failed to verify OTP",
			"success": false,
			"error":   err.Error()})
		return
	}

	utils.PrintLogInfo(&req.Email, 200, "VerifyOTP", nil)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "User created successfully"})
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.PrintLogInfo(nil, 400, "Login", &err)
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Invalid request",
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	loweredEmail := strings.ToLower(req.Email)
	tokens, err := h.authUC.Login(c.Request.Context(), loweredEmail, req.Password)
	if err != nil {
		utils.PrintLogInfo(&loweredEmail, 401, "Login", &err)
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "Login failed",
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// ✅ Detect platform (Web or Mobile)
	userAgent := c.Request.Header.Get("User-Agent")
	isMobile := strings.Contains(strings.ToLower(userAgent), "okhttp") || // Android
		strings.Contains(strings.ToLower(userAgent), "ios") || // iOS
		strings.Contains(strings.ToLower(userAgent), "mobile")

	if !isMobile {
		// ✅ For WEB: store refresh_token in HttpOnly secure cookie
		c.SetCookie(
			"refresh_token",
			tokens.RefreshToken, // ✅ correct variable
			60*60*24*7,          // 7 days
			"/",
			"",    // ⚠️ change to your actual domain in production
			false, // ✅ secure (HTTPS only)
			true,  // ✅ HttpOnly
		)

		utils.PrintLogInfo(&loweredEmail, 200, "Login", nil)
		// ✅ Also return refresh_token in body for cross-domain deployment
		// Cookie tidak bekerja untuk cross-domain (Vercel ↔ Heroku)
		c.JSON(http.StatusOK, gin.H{
			"success":       true,
			"access_token":  tokens.AccessToken,
			"refresh_token": tokens.RefreshToken, // ✅ ADDED for cross-domain support
			"message":       "Login successful",
		})
		return
	}

	// ✅ For MOBILE: return both tokens
	utils.PrintLogInfo(&loweredEmail, 200, "Login", nil)
	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"access_token":  tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
		"message":       "Login successful",
	})
}

type ForgotPasswordRequest struct {
	Email string `json:"email" binding:"required,email"`
}

type ResetPasswordRequest struct {
	Email       string `json:"email" binding:"required,email"`
	OTP         string `json:"otp" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

func (h *AuthHandler) ForgotPassword(c *gin.Context) {
	var req ForgotPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.PrintLogInfo(nil, 400, "ForgotPassword", &err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request",
			"error":   err.Error()})
		return
	}

	if err := h.authUC.ForgotPassword(c.Request.Context(), req.Email); err != nil {
		utils.PrintLogInfo(&req.Email, 500, "ForgotPassword", &err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to process request",
			"error":   err.Error()})
		return
	}

	utils.PrintLogInfo(&req.Email, 200, "ForgotPassword", nil)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "OTP sent for reset password"})
}

func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.PrintLogInfo(nil, 400, "ResetPassword", &err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request",
			"error":   err.Error()})
		return
	}

	if err := h.authUC.ResetPassword(c.Request.Context(), req.Email, req.OTP, req.NewPassword); err != nil {
		utils.PrintLogInfo(&req.Email, 401, "ResetPassword", &err)
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Failed to reset password",
			"error":   err.Error()})
		return
	}

	utils.PrintLogInfo(&req.Email, 200, "ResetPassword", nil)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Password reset successfully"})
}

func (h *AuthHandler) ChangePassword(c *gin.Context) {

	userUUID, exists := c.Get("userUUID")
	if !exists {
		utils.PrintLogInfo(nil, 401, "ChangePassword", nil)
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Failed to get user from context",
			"error":   "unauthorized"})
		return
	}

	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.PrintLogInfo(nil, 400, "ChangePassword", &err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request",
			"error":   utils.TranslateValidationError(err)})
		return
	}

	if err := h.authUC.ChangePassword(c.Request.Context(), userUUID.(string), req.OldPassword, req.NewPassword); err != nil {
		utils.PrintLogInfo(nil, 401, "ChangePassword", &err)
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Failed to change password",
			"error":   err.Error()})
		return
	}

	utils.PrintLogInfo(nil, 200, "ChangePassword", nil)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Password changed successfully"})
}
