package delivery

import (
	"chronosphere/config"
	"chronosphere/domain"
	"chronosphere/middleware"
	"chronosphere/utils"
	"log"
	"math"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

type PaymentHandler struct {
	uc domain.PaymentUseCase
}

type CreateInvoiceRequest struct {
	PackageID int `json:"package_id" binding:"required"`
}

func NewPaymentHandler(r *gin.Engine, uc domain.PaymentUseCase, jwtManager *utils.JWTManager) {
	handler := &PaymentHandler{uc: uc}

	// Student routes (requires auth)
	student := r.Group("/student")
	student.Use(config.AuthMiddleware(jwtManager), middleware.StudentAndAdminOnly())
	{
		student.POST("/payment/create-invoice", handler.CreateInvoice)
		student.GET("/payment/history", handler.GetPaymentHistory)
	}

	// Admin routes (requires auth + admin only)
	admin := r.Group("/admin")
	admin.Use(config.AuthMiddleware(jwtManager), middleware.AdminOnly())
	{
		admin.GET("/payment/profit", handler.GetTotalProfit)
		admin.GET("/payment/history", handler.GetPaymentHistoryAdmin)
		admin.GET("/payment/summary", handler.GetPackageSummary)
	}

	// Webhook route (public, verified by callback token)
	r.POST("/payment/webhook/xendit", handler.HandleXenditWebhook)
}

// ========================================================================
// Student Endpoints
// ========================================================================

// CreateInvoice creates a new Xendit invoice for the student
func (h *PaymentHandler) CreateInvoice(c *gin.Context) {
	name := utils.GetAPIHitter(c)

	userUUID, exists := c.Get("userUUID")
	if !exists {
		utils.PrintLogInfo(&name, 401, "CreateInvoice", nil)
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Unauthorized: missing user context",
			"message": "Gagal membuat invoice",
		})
		return
	}

	var req CreateInvoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.PrintLogInfo(&name, 400, "CreateInvoice - BindJSON", &err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   utils.TranslateValidationError(err),
			"message": "Gagal membuat invoice",
		})
		return
	}

	payment, err := h.uc.CreateInvoice(c.Request.Context(), userUUID.(string), req.PackageID)
	if err != nil {
		utils.PrintLogInfo(&name, 500, "CreateInvoice - UseCase", &err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Gagal membuat invoice pembayaran",
		})
		return
	}

	utils.PrintLogInfo(&name, 200, "CreateInvoice", nil)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Invoice berhasil dibuat",
		"data": gin.H{
			"invoice_url": payment.InvoiceURL,
			"external_id": payment.ExternalID,
			"amount":      payment.Amount,
			"status":      payment.Status,
		},
	})
}

// GetPaymentHistory returns payment history for the authenticated student
func (h *PaymentHandler) GetPaymentHistory(c *gin.Context) {
	name := utils.GetAPIHitter(c)

	userUUID, exists := c.Get("userUUID")
	if !exists {
		utils.PrintLogInfo(&name, 401, "GetPaymentHistory", nil)
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Unauthorized: missing user context",
			"message": "Gagal mengambil riwayat pembayaran",
		})
		return
	}

	payments, err := h.uc.GetPaymentsByStudent(c.Request.Context(), userUUID.(string))
	if err != nil {
		utils.PrintLogInfo(&name, 500, "GetPaymentHistory", &err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Gagal mengambil riwayat pembayaran",
		})
		return
	}

	utils.PrintLogInfo(&name, 200, "GetPaymentHistory", nil)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    payments,
	})
}

// ========================================================================
// Webhook Endpoint
// ========================================================================

// HandleXenditWebhook handles callback notifications from Xendit
func (h *PaymentHandler) HandleXenditWebhook(c *gin.Context) {
	// 1. Verify callback token
	callbackToken := c.GetHeader("x-callback-token")
	expectedToken := os.Getenv("XENDIT_WEBHOOK_TOKEN")

	if expectedToken == "" {
		log.Println("⚠️  XENDIT_WEBHOOK_TOKEN not configured")
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Webhook not configured",
		})
		return
	}

	if callbackToken != expectedToken {
		log.Printf("❌ Webhook: invalid callback token received")
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Invalid callback token",
		})
		return
	}

	// 2. Parse webhook payload
	var payload domain.XenditWebhookPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		log.Printf("❌ Webhook: failed to parse payload: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid payload",
		})
		return
	}

	log.Printf("📩 Webhook received: external_id=%s status=%s method=%s", payload.ExternalID, payload.Status, payload.PaymentMethod)

	// 3. Process webhook
	if err := h.uc.HandleWebhook(c.Request.Context(), payload); err != nil {
		log.Printf("❌ Webhook: processing error: %v", err)
		// Still return 200 to prevent Xendit from retrying
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Webhook processing error",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Webhook processed successfully",
	})
}

// ========================================================================
// Admin Endpoints
// ========================================================================

// GetTotalProfit returns total revenue from paid payments
func (h *PaymentHandler) GetTotalProfit(c *gin.Context) {
	name := utils.GetAPIHitter(c)

	var filter domain.ProfitFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		utils.PrintLogInfo(&name, 400, "GetTotalProfit - BindQuery", &err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid filter parameters",
			"message": "Gagal mengambil data profit",
		})
		return
	}

	total, err := h.uc.GetTotalProfit(c.Request.Context(), filter)
	if err != nil {
		utils.PrintLogInfo(&name, 500, "GetTotalProfit", &err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Gagal mengambil data profit",
		})
		return
	}

	utils.PrintLogInfo(&name, 200, "GetTotalProfit", nil)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"total_profit": total,
			"filter":       filter,
		},
	})
}

// GetPaymentHistoryAdmin returns all payment history with pagination and filters
func (h *PaymentHandler) GetPaymentHistoryAdmin(c *gin.Context) {
	name := utils.GetAPIHitter(c)

	var filter domain.HistoryFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		utils.PrintLogInfo(&name, 400, "GetPaymentHistoryAdmin - BindQuery", &err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid filter parameters",
			"message": "Gagal mengambil riwayat pembayaran",
		})
		return
	}

	// Default pagination
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.Limit < 1 {
		filter.Limit = 10
	}

	payments, total, err := h.uc.GetPaymentHistory(c.Request.Context(), filter)
	if err != nil {
		utils.PrintLogInfo(&name, 500, "GetPaymentHistoryAdmin", &err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Gagal mengambil riwayat pembayaran",
		})
		return
	}

	utils.PrintLogInfo(&name, 200, "GetPaymentHistoryAdmin", nil)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    payments,
		"pagination": gin.H{
			"page":        filter.Page,
			"limit":       filter.Limit,
			"total":       total,
			"total_pages": int(math.Ceil(float64(total) / float64(filter.Limit))),
		},
	})
}

// GetPackageSummary returns summary of sales per package
func (h *PaymentHandler) GetPackageSummary(c *gin.Context) {
	name := utils.GetAPIHitter(c)

	summaries, err := h.uc.GetPackageSummary(c.Request.Context())
	if err != nil {
		utils.PrintLogInfo(&name, 500, "GetPackageSummary", &err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Gagal mengambil ringkasan paket",
		})
		return
	}

	utils.PrintLogInfo(&name, 200, "GetPackageSummary", nil)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    summaries,
	})
}
