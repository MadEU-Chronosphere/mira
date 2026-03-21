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

	// Public: list of all packages (no auth required)
	r.GET("/packages", handler.GetAllAvailablePackages)

	student := r.Group("/student")
	student.Use(config.AuthMiddleware(jwtManager), middleware.StudentOnly())
	{
		student.GET("/profile", handler.GetMyProfile)
		student.POST("/book", handler.BookClass)
		student.GET("/booked", handler.GetMyBookedClasses)

		// GET /student/classes?package_id=<studentPackageID>
		// package_id is required: it drives trial vs regular logic and instrument/duration filtering.
		student.GET("/classes", handler.GetAvailableSchedules)

		student.PUT("/modify", handler.UpdateStudentData)
		student.DELETE("/cancel/:booking_id", handler.CancelBookedClass)
		student.GET("/class-history", handler.GetMyClassHistory)
		student.GET("/teacher-details/:teacher_uuid", handler.GetTeacherDetails)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// GetTeacherDetails
// ─────────────────────────────────────────────────────────────────────────────

func (h *StudentHandler) GetTeacherDetails(c *gin.Context) {
	name := utils.GetAPIHitter(c)
	teacherUUID := c.Param("teacher_uuid")

	teacherDetails, err := h.studUC.GetTeacherDetails(c.Request.Context(), teacherUUID)
	if err != nil {
		utils.PrintLogInfo(&name, 500, "GetTeacherDetails", &err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Gagal mengambil detail guru",
		})
		return
	}

	utils.PrintLogInfo(&name, 200, "GetTeacherDetails", nil)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": teacherDetails})
}

// ─────────────────────────────────────────────────────────────────────────────
// GetMyClassHistory
// ─────────────────────────────────────────────────────────────────────────────

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
	c.JSON(http.StatusOK, gin.H{"success": true, "data": histories})
}

// ─────────────────────────────────────────────────────────────────────────────
// CancelBookedClass
// ─────────────────────────────────────────────────────────────────────────────

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

	if err := h.studUC.CancelBookedClass(c.Request.Context(), convertedID, userUUID.(string), req.Reason); err != nil {
		utils.PrintLogInfo(&name, 500, "CancelBookedClass", &err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Gagal membatalkan kelas yang dipesan",
		})
		return
	}

	utils.PrintLogInfo(&name, 200, "CancelBookedClass", nil)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Kelas yang dipesan berhasil dibatalkan"})
}

// ─────────────────────────────────────────────────────────────────────────────
// BookClass
//
// Request body: { schedule_id, package_id, instrument_id }
//   - package_id: the student_packages.id to use (must belong to the student and be active).
//   - instrument_id: which instrument to study (required; for trial packages the student
//     picks the instrument because their trial package is instrument-agnostic).
// ─────────────────────────────────────────────────────────────────────────────

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

	if err := h.studUC.BookClass(
		c.Request.Context(),
		userUUID.(string),
		payload.ScheduleID,
		payload.PackageID,
		payload.InstrumentID, // *int — nil for regular packages, required for trial
	); err != nil {
		utils.PrintLogInfo(&name, 500, "BookClass", &err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Gagal memesan kelas",
		})
		return
	}

	utils.PrintLogInfo(&name, 200, "BookClass", nil)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Kelas berhasil dipesan"})
}

// ─────────────────────────────────────────────────────────────────────────────
// GetAvailableSchedules
//
// Query param: package_id (required) — the student_packages.id the student intends to use.
// Response includes:
//   - All relevant teacher schedules enriched with availability flags.
//   - teacher_finished_class_count per schedule for frontend performance sorting.
//
// Trial packages: all active teacher schedules are returned (no instrument/duration filter).
// ─────────────────────────────────────────────────────────────────────────────

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

	packageIDStr := c.Query("package_id")
	if packageIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "package_id wajib diisi",
			"message": "Gagal mengambil jadwal tersedia",
		})
		return
	}

	packageID, err := strconv.Atoi(packageIDStr)
	if err != nil || packageID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "package_id tidak valid",
			"message": "Gagal mengambil jadwal tersedia",
		})
		return
	}

	schedules, err := h.studUC.GetAvailableSchedules(c.Request.Context(), userUUID.(string), packageID)
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
	c.JSON(http.StatusOK, gin.H{"success": true, "data": schedules})
}

// ─────────────────────────────────────────────────────────────────────────────
// GetMyBookedClasses
// ─────────────────────────────────────────────────────────────────────────────

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
	c.JSON(http.StatusOK, gin.H{"success": true, "data": bookings})
}

// ─────────────────────────────────────────────────────────────────────────────
// GetAllAvailablePackages
// ─────────────────────────────────────────────────────────────────────────────

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
	c.JSON(http.StatusOK, gin.H{"success": true, "data": packages})
}

// ─────────────────────────────────────────────────────────────────────────────
// GetMyProfile
// ─────────────────────────────────────────────────────────────────────────────

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
	c.JSON(http.StatusOK, gin.H{"success": true, "data": user})
}

// ─────────────────────────────────────────────────────────────────────────────
// UpdateStudentData
// ─────────────────────────────────────────────────────────────────────────────

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
	if err := h.studUC.UpdateStudentData(c.Request.Context(), userUUID.(string), filteredPayload); err != nil {
		utils.PrintLogInfo(&name, 500, "UpdateStudentData", &err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Gagal memperbarui data siswa",
		})
		return
	}

	utils.PrintLogInfo(&name, 200, "UpdateStudentData", nil)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Data siswa berhasil diperbarui"})
}