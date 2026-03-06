package delivery

import (
	"chronosphere/config"
	"chronosphere/domain"
	"chronosphere/dto"
	"chronosphere/middleware"
	"chronosphere/utils"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type StudentHandler struct {
	studUC domain.StudentUseCase
}

func NewStudentHandler(r *gin.Engine, studUC domain.StudentUseCase, jwtManager *utils.JWTManager) {
	handler := &StudentHandler{studUC: studUC}
	r.GET("/packages", handler.GetAllAvailablePackages)

	student := r.Group("/student")

	student.Use(config.AuthMiddleware(jwtManager), middleware.StudentOnly())
	{
		student.GET("/profile", handler.GetMyProfile)
		student.POST("/book", handler.BookClass)
		student.GET("/booked", handler.GetMyBookedClasses)
		student.GET("/classes", handler.GetAvailableSchedules)
		student.PUT("/modify", handler.UpdateStudentData)
		student.DELETE("/cancel/:booking_id", handler.CancelBookedClass)
		student.GET("/class-history", handler.GetMyClassHistory)

	}

}

func (h *StudentHandler) GetMyClassHistory(c *gin.Context) {
	name := utils.GetAPIHitter(c)

	userUUID, exists := c.Get("userUUID")
	if !exists {
		utils.PrintLogInfo(&name, 401, "GetMyClassHistory", nil)
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Tidak terotorisasi: konteks pengguna tidak ditemukan",
			"message": "Gagal mengambil riwayat kelas saya",
		})
		return
	}

	histories, err := h.studUC.GetMyClassHistory(c.Request.Context(), userUUID.(string))
	if err != nil {
		utils.PrintLogInfo(&name, 500, "GetMyClassHistory", &err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Gagal mengambil riwayat kelas saya",
		})
		return
	}

	utils.PrintLogInfo(&name, 200, "GetMyClassHistory", nil)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    histories,
	})
}

func (h *StudentHandler) CancelBookedClass(c *gin.Context) {
	name := utils.GetAPIHitter(c)

	userUUID, exists := c.Get("userUUID")
	if !exists {
		utils.PrintLogInfo(&name, 401, "CancelBookedClass", nil)
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Tidak terotorisasi: konteks pengguna tidak ditemukan",
			"message": "Gagal membatalkan kelas yang dipesan",
		})
		return
	}

	// 🔹 Parse booking_id
	bookid := c.Param("booking_id")
	convertedID, err := strconv.Atoi(bookid)
	if err != nil {
		utils.PrintLogInfo(&name, 400, "CancelBookedClass", &err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Parameter ID pemesanan tidak valid",
			"message": "Gagal membatalkan kelas yang dipesan",
		})
		return
	}

	// 🔹 Parse request body for optional reason
	var req dto.CancelBookingRequest
	if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
		utils.PrintLogInfo(&name, 400, "CancelBookedClass", &err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Body permintaan tidak valid",
			"message": "Gagal membatalkan kelas yang dipesan",
		})
		return
	}

	if req.Reason != nil && len(*req.Reason) == 0 {
		req.Reason = nil
	}

	// 🔹 Call use case with reason
	err = h.studUC.CancelBookedClass(c.Request.Context(), convertedID, userUUID.(string), req.Reason)
	if err != nil {
		utils.PrintLogInfo(&name, 500, "CancelBookedClass", &err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Gagal membatalkan kelas yang dipesan",
		})
		return
	}

	utils.PrintLogInfo(&name, 200, "CancelBookedClass", nil)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Kelas yang dipesan berhasil dibatalkan",
	})
}

func (h *StudentHandler) BookClass(c *gin.Context) {
	name := utils.GetAPIHitter(c)

	userUUID, exists := c.Get("userUUID")
	if !exists {
		utils.PrintLogInfo(&name, 401, "BookClass", nil)
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Tidak terotorisasi: konteks pengguna tidak ditemukan",
			"message": "Gagal memesan kelas",
		})
		return
	}

	var payload dto.BookClassRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		utils.PrintLogInfo(&name, 400, "BookClass", &err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   utils.TranslateValidationError(err),
			"message": "Gagal memesan kelas",
		})
		return
	}

	err := h.studUC.BookClass(c.Request.Context(), userUUID.(string), payload.ScheduleID, payload.InstrumentID)
	if err != nil {
		utils.PrintLogInfo(&name, 500, "BookClass", &err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Gagal memesan kelas",
		})
		return
	}

	utils.PrintLogInfo(&name, 200, "BookClass", nil)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Kelas berhasil dipesan",
	})
}

func (h *StudentHandler) GetAvailableSchedules(c *gin.Context) {
	name := utils.GetAPIHitter(c)
	userUUID, exists := c.Get("userUUID")
	if !exists {
		utils.PrintLogInfo(&name, 401, "GetAvailableSchedules", nil)
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Tidak terotorisasi: konteks pengguna tidak ditemukan",
			"message": "Gagal mengambil jadwal tersedia",
		})
		return
	}

	schedules, err := h.studUC.GetAvailableSchedules(c.Request.Context(), userUUID.(string))
	if err != nil {
		utils.PrintLogInfo(&name, 500, "GetAvailableSchedules", &err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Gagal mengambil jadwal tersedia",
		})
		return
	}

	utils.PrintLogInfo(&name, 200, "GetAvailableSchedules", nil)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    schedules,
	})
}

func (h *StudentHandler) GetMyBookedClasses(c *gin.Context) {
	name := utils.GetAPIHitter(c)

	userUUID, exists := c.Get("userUUID")
	if !exists {
		utils.PrintLogInfo(&name, 401, "GetMyBookedClasses", nil)
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Tidak terotorisasi: konteks pengguna tidak ditemukan",
			"message": "Gagal mengambil kelas yang dipesan",
		})
		return
	}

	bookings, err := h.studUC.GetMyBookedClasses(c.Request.Context(), userUUID.(string))
	if err != nil {
		utils.PrintLogInfo(&name, 500, "GetMyBookedClasses", &err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Gagal mengambil kelas yang dipesan",
		})
		return
	}

	utils.PrintLogInfo(&name, 200, "GetMyBookedClasses", nil)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    bookings,
	})
}

func (h *StudentHandler) GetAllAvailablePackages(c *gin.Context) {

	packages, err := h.studUC.GetAllAvailablePackages(c.Request.Context())
	if err != nil {
		utils.PrintLogInfo(nil, 500, "GetAllAvailablePackages", &err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Gagal mengambil paket tersedia",
		})
		return
	}

	utils.PrintLogInfo(nil, 200, "GetAllAvailablePackages", nil)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    packages,
	})
}

func (h *StudentHandler) GetMyProfile(c *gin.Context) {
	name := utils.GetAPIHitter(c)
	userUUID, exists := c.Get("userUUID")
	if !exists {
		utils.PrintLogInfo(&name, 401, "GetMyProfile", nil)
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Tidak terotorisasi: konteks pengguna tidak ditemukan",
			"message": "Gagal mengambil profil saya",
		})
		return
	}

	// Call usecase to get teacher data
	user, err := h.studUC.GetMyProfile(c.Request.Context(), userUUID.(string))
	if err != nil {
		utils.PrintLogInfo(&name, 500, "GetMyProfile", &err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Gagal mengambil profil saya",
		})
		return
	}

	utils.PrintLogInfo(&name, 200, "GetMyProfile", nil)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    user,
	})
}

func (h *StudentHandler) UpdateStudentData(c *gin.Context) {
	name := utils.GetAPIHitter(c)
	userUUID, exists := c.Get("userUUID")
	if !exists {
		utils.PrintLogInfo(&name, 401, "UpdateStudentData", nil)
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Tidak terotorisasi: konteks pengguna tidak ditemukan",
			"message": "Gagal memperbarui data siswa",
		})
		return
	}

	var payload dto.UpdateStudentDataRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		utils.PrintLogInfo(&name, 400, "UpdateStudentData", &err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   utils.TranslateValidationError(err),
			"message": "Payload permintaan tidak valid",
		})
		return
	}

	filteredPayload := dto.MapUpdateStudentRequestByStudent(&payload)
	err := h.studUC.UpdateStudentData(c.Request.Context(), userUUID.(string), filteredPayload)
	if err != nil {
		utils.PrintLogInfo(&name, 500, "UpdateStudentData", &err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Gagal memperbarui data siswa",
		})
		return
	}

	utils.PrintLogInfo(&name, 200, "UpdateStudentData", nil)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Data siswa berhasil diperbarui",
	})
}
