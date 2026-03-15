package delivery

import (
	"chronosphere/config"
	"chronosphere/domain"
	"chronosphere/middleware"
	"chronosphere/utils"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AuthHandler struct {
	authUC domain.AuthUseCase
}

// setRefreshTokenCookie sets the refresh_token cookie with proper
// SameSite and Secure attributes based on the environment.
// Production (cross-domain): SameSite=None + Secure=true
// Development (localhost):   SameSite=Lax  + Secure=false
func setRefreshTokenCookie(c *gin.Context, value string, maxAge int) {
	isProduction := os.Getenv("APP_ENV") == "production"

	sameSite := http.SameSiteLaxMode
	secure := false
	if isProduction {
		sameSite = http.SameSiteNoneMode
		secure = true
	}

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "refresh_token",
		Value:    value,
		MaxAge:   maxAge,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
	})
}

func NewAuthHandler(r *gin.Engine, authUC domain.AuthUseCase, db *gorm.DB) {
	handler := &AuthHandler{authUC: authUC}

	// Ping Route (no rate limiting)
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "pong1",
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
			"message": "Gagal mendapatkan data pengguna dari konteks",
			"error":   "tidak terotorisasi"})
		return
	}

	var req ChangeEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.PrintLogInfo(nil, 400, "ChangeEmail", &err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Permintaan tidak valid",
			"error":   utils.TranslateValidationError(err)})
		return
	}

	if err := h.authUC.ChangeEmail(c.Request.Context(), userUUID.(string), strings.ToLower(req.NewEmail), req.Password); err != nil {
		utils.PrintLogInfo(nil, 401, "ChangeEmail", &err)
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Gagal mengubah email",
			"error":   err.Error()})
		return
	}
	utils.PrintLogInfo(nil, 200, "ChangeEmail", nil)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Email berhasil diubah"})

}

func (h *AuthHandler) Logout(c *gin.Context) {
	// ✅ Clear cookie (for web)
	setRefreshTokenCookie(c, "", -1)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Berhasil keluar",
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
				"message": "Token penyegaran tidak diberikan",
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
			"message": "Token penyegaran tidak valid atau sudah kadaluarsa",
		})
		return
	}

	// ✅ Check if user still exists in database
	_, err = h.authUC.Me(c.Request.Context(), userUUID)
	if err != nil {
		// User deleted - clear cookie and reject
		setRefreshTokenCookie(c, "", -1)
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Akun pengguna tidak ditemukan",
			"error":   "pengguna_dihapus",
		})
		return
	}

	// ✅ Generate new access token
	newAccessToken, err := h.authUC.GetAccessTokenManager().GenerateToken(userUUID, role, name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Gagal membuat token akses baru",
			"error":   err.Error(),
		})
		return
	}

	// ✅ (Optional) Generate new refresh token for long sessions
	newRefreshToken, err := h.authUC.GetRefreshTokenManager().GenerateToken(userUUID, role, name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Gagal membuat token penyegaran baru",
			"error":   err.Error(),
		})
		return
	}

	// ✅ For web clients, update HttpOnly cookie
	setRefreshTokenCookie(c, newRefreshToken, 60*60*24*7)

	// ✅ Return new access token
	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"message":       "Token berhasil diperbaharui",
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
			"message": "Tidak terotorisasi: konteks pengguna tidak ditemukan",
		})
		return
	}

	userUUID, ok := uuidVal.(string)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Tipe UUID pengguna tidak valid",
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
			"message": "Permintaan tidak valid",
			"error":   err.Error(),
		})
		return
	}

	if err := h.authUC.ResendOTP(c.Request.Context(), req.Email); err != nil {
		utils.PrintLogInfo(&req.Email, 500, "ResendOTP", &err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Gagal mengirim ulang OTP",
			"error":   err.Error(),
		})
		return
	}

	utils.PrintLogInfo(&req.Email, 200, "ResendOTP", nil)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "OTP berhasil dikirim ulang",
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
			"message": "Gagal mendaftar",
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
			"message": "Gagal mendaftar",
			"error":   err.Error(),
		})
		return
	}
	utils.PrintLogInfo(&req.Email, 200, "Register", nil)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "OTP telah dikirim ke email Anda",
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
			"message": "Permintaan tidak valid",
			"success": false,
			"error":   err.Error()})
		return
	}
	if err := h.authUC.VerifyOTP(c.Request.Context(), req.Email, req.OTP); err != nil {
		utils.PrintLogInfo(&req.Email, 401, "VerifyOTP", &err)
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "Gagal memverifikasi OTP",
			"success": false,
			"error":   err.Error()})
		return
	}

	utils.PrintLogInfo(&req.Email, 200, "VerifyOTP", nil)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Pengguna berhasil dibuat"})
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
			"message": "Permintaan tidak valid",
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
			"message": "Login gagal",
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// ✅ Detect platform: Native Mobile App vs Web Browser
	// Only match native app HTTP clients, NOT mobile web browsers
	// Mobile web browsers (Chrome/Safari on phone) should use cookies like desktop
	userAgent := c.Request.Header.Get("User-Agent")
	lowerUA := strings.ToLower(userAgent)
	isMobileApp := strings.Contains(lowerUA, "okhttp") || // Android native (OkHttp)
		strings.Contains(lowerUA, "dart") || // Flutter (Dart)
		strings.Contains(lowerUA, "alamofire") || // iOS Swift (Alamofire)
		strings.Contains(lowerUA, "cfnetwork") // iOS native (URLSession)

	if !isMobileApp {
		// ✅ For WEB: store refresh_token in HttpOnly secure cookie
		setRefreshTokenCookie(c, tokens.RefreshToken, 60*60*24*7)

		utils.PrintLogInfo(&loweredEmail, 200, "Login", nil)
		c.JSON(http.StatusOK, gin.H{
			"success":      true,
			"access_token": tokens.AccessToken,
			"message":      "Login berhasil",
		})
		return
	}

	// ✅ For MOBILE: return both tokens
	utils.PrintLogInfo(&loweredEmail, 200, "Login", nil)
	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"access_token":  tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
		"message":       "Login berhasil",
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
			"message": "Permintaan tidak valid",
			"error":   err.Error()})
		return
	}

	if err := h.authUC.ForgotPassword(c.Request.Context(), req.Email); err != nil {
		utils.PrintLogInfo(&req.Email, 500, "ForgotPassword", &err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Gagal memproses permintaan",
			"error":   err.Error()})
		return
	}

	utils.PrintLogInfo(&req.Email, 200, "ForgotPassword", nil)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "OTP telah dikirim untuk reset password"})
}

func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.PrintLogInfo(nil, 400, "ResetPassword", &err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Permintaan tidak valid",
			"error":   err.Error()})
		return
	}

	if err := h.authUC.ResetPassword(c.Request.Context(), req.Email, req.OTP, req.NewPassword); err != nil {
		utils.PrintLogInfo(&req.Email, 401, "ResetPassword", &err)
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Gagal mereset password",
			"error":   err.Error()})
		return
	}

	utils.PrintLogInfo(&req.Email, 200, "ResetPassword", nil)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Password berhasil direset"})
}

func (h *AuthHandler) ChangePassword(c *gin.Context) {

	userUUID, exists := c.Get("userUUID")
	if !exists {
		utils.PrintLogInfo(nil, 401, "ChangePassword", nil)
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Gagal mendapatkan data pengguna dari konteks",
			"error":   "tidak terotorisasi"})
		return
	}

	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.PrintLogInfo(nil, 400, "ChangePassword", &err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Permintaan tidak valid",
			"error":   utils.TranslateValidationError(err)})
		return
	}

	if err := h.authUC.ChangePassword(c.Request.Context(), userUUID.(string), req.OldPassword, req.NewPassword); err != nil {
		utils.PrintLogInfo(nil, 401, "ChangePassword", &err)
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Gagal mengubah password",
			"error":   err.Error()})
		return
	}

	utils.PrintLogInfo(nil, 200, "ChangePassword", nil)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Password berhasil diubah"})
}
