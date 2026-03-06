package delivery

import (
	"chronosphere/config"
	"chronosphere/domain"
	"chronosphere/dto"
	"chronosphere/middleware"
	"chronosphere/utils"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type TeacherHandler struct {
	tc domain.TeacherUseCase
}

func NewTeacherHandler(app *gin.Engine, tc domain.TeacherUseCase, jwtManager *utils.JWTManager, db *gorm.DB) {
	h := &TeacherHandler{tc: tc}

	teacher := app.Group("/teacher")
	teacher.Use(config.AuthMiddleware(jwtManager), middleware.TeacherOnly(), middleware.ValidateTurnedOffUserMiddleware(db))
	{
		teacher.GET("/profile", h.GetMyProfile)
		teacher.GET("/schedules", h.GetMySchedules)
		teacher.PUT("/modify", h.UpdateTeacherData)
		teacher.POST("/create-available-class", h.AddAvailability)
		teacher.DELETE("/delete-available-class/:id", h.DeleteAddAvailability)
		teacher.GET("/booked", h.GetAllBookedClass)
		teacher.GET("/class-history", h.GetMyClassHistory)
		teacher.DELETE("/cancel/:id", h.CancelBookedClass)
		teacher.PUT("/finish-class/:id", h.FinishClass)
		teacher.DELETE("/delete-availability-by-day/:day", h.DeleteAvailabilityBasedOnDay)

	}
}

func (h *TeacherHandler) DeleteAvailabilityBasedOnDay(c *gin.Context) {
	name := utils.GetAPIHitter(c)
	userUUID, exists := c.Get("userUUID")
	if !exists {
		utils.PrintLogInfo(&name, http.StatusUnauthorized, "DeleteAvailabilityBasedOnDay", nil)
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Tidak terotorisasi: konteks pengguna tidak ditemukan",
			"message": "Gagal menghapus ketersediaan berdasarkan hari",
		})
		return
	}

	dayOfWeek := c.Param("day")

	err := h.tc.DeleteAvailabilityBasedOnDay(c.Request.Context(), userUUID.(string), dayOfWeek)
	if err != nil {
		utils.PrintLogInfo(&name, http.StatusInternalServerError, "DeleteAvailabilityBasedOnDay", &err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Gagal menghapus ketersediaan berdasarkan hari",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Berhasil menghapus ketersediaan untuk %s", dayOfWeek),
	})
}

func (h *TeacherHandler) GetMyClassHistory(c *gin.Context) {
	name := utils.GetAPIHitter(c)
	userUUID, exists := c.Get("userUUID")
	if !exists {
		utils.PrintLogInfo(&name, http.StatusUnauthorized, "GetMyClassHistory", nil)
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Tidak terotorisasi: konteks pengguna tidak ditemukan",
			"message": "Gagal menyelesaikan kelas",
		})
		return
	}

	teacherUUID := userUUID.(string)
	data, err := h.tc.GetMyClassHistory(c.Request.Context(), teacherUUID)
	if err != nil {
		utils.PrintLogInfo(&name, http.StatusInternalServerError, "GetMyClassHistory", &err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Gagal mengambil riwayat kelas",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    data,
	})
}

func (h *TeacherHandler) FinishClass(c *gin.Context) {
	name := utils.GetAPIHitter(c)

	// ✅ Get teacher UUID from context
	uuidVal, exists := c.Get("userUUID")
	if !exists {
		utils.PrintLogInfo(&name, http.StatusUnauthorized, "FinishClass - MissingUserUUID", nil)
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Tidak terotorisasi: konteks pengguna tidak ditemukan",
			"message": "Gagal menyelesaikan kelas",
		})
		return
	}
	teacherUUID := uuidVal.(string)

	// ✅ Parse booking ID from URL param
	bookingID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		utils.PrintLogInfo(&name, http.StatusBadRequest, "FinishClass - InvalidBookingID", &err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "ID pemesanan tidak valid",
			"message": "Gagal menyelesaikan kelas",
		})
		return
	}

	// ✅ Bind JSON body to DTO
	var req dto.FinishClassRequest
	req.BookingID = bookingID
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.PrintLogInfo(&name, http.StatusBadRequest, "FinishClass - BindJSON", &err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   utils.TranslateValidationError(err),
			"message": "Gagal menyelesaikan kelas",
		})
		return
	}

	// ✅ Map DTO → domain model (this now returns error)
	payload, err := dto.MapFinishClassRequestToClassHistory(&req, bookingID)
	if err != nil {
		utils.PrintLogInfo(&name, http.StatusBadRequest, "FinishClass - MapDTO", &err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Gagal menyelesaikan kelas",
		})
		return
	}

	// ✅ Call usecase
	if err := h.tc.FinishClass(c.Request.Context(), bookingID, teacherUUID, payload); err != nil {
		status := http.StatusInternalServerError

		// Determine appropriate status code
		errorMsg := err.Error()
		if strings.Contains(errorMsg, "tidak ditemukan") ||
			strings.Contains(errorMsg, "tidak memiliki akses") {
			status = http.StatusForbidden
		} else if strings.Contains(errorMsg, "sudah selesai") ||
			strings.Contains(errorMsg, "belum selesai") {
			status = http.StatusBadRequest
		}

		utils.PrintLogInfo(&name, status, "FinishClass - UseCase", &err)
		c.JSON(status, gin.H{
			"success": false,
			"error":   errorMsg,
			"message": "Gagal menyelesaikan kelas",
		})
		return
	}

	// ✅ Success
	utils.PrintLogInfo(&name, http.StatusOK, "FinishClass", nil)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Kelas berhasil diselesaikan",
	})
}

func (h *TeacherHandler) CancelBookedClass(c *gin.Context) {
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
	bookid := c.Param("id")
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
	err = h.tc.CancelBookedClass(c.Request.Context(), convertedID, userUUID.(string), req.Reason)
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

func (h *TeacherHandler) GetAllBookedClass(c *gin.Context) {
	name := utils.GetAPIHitter(c)
	uuid, theBool := c.Get("userUUID")
	if !theBool {
		utils.PrintLogInfo(&name, 401, "GetAllBookedClass", nil)
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Tidak terotorisasi: konteks pengguna tidak ditemukan",
			"message": "Gagal mendapatkan semua kelas yang dipesan",
		})
		return
	}

	bookedClasses, err := h.tc.GetAllBookedClass(c.Request.Context(), uuid.(string))
	if err != nil {
		utils.PrintLogInfo(&name, 500, "GetAllBookedClass", &err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Gagal mendapatkan semua kelas yang dipesan",
		})
		return
	}

	utils.PrintLogInfo(&name, 200, "GetAllBookedClass", nil)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    bookedClasses,
	})
}

func (h *TeacherHandler) AddAvailability(c *gin.Context) {
	name := utils.GetAPIHitter(c)

	var req dto.AddMultipleAvailabilityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.PrintLogInfo(&name, 400, "AddAvailability - BindJSON", &err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Gagal menambahkan ketersediaan",
			"error":   utils.TranslateValidationError(err),
		})
		return
	}

	// Extract teacher UUID from JWT token
	teacherUUID, exists := c.Get("userUUID")
	if !exists {
		utils.PrintLogInfo(&name, 401, "AddAvailability - MissingUserUUID", nil)
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Gagal menambahkan ketersediaan",
			"error":   "Tidak terotorisasi: konteks pengguna tidak ditemukan",
		})
		return
	}

	teacherID := teacherUUID.(string)

	// Convert DTO to domain models with validation
	schedules, err := h.convertToTeacherSchedules(teacherID, req.SlotsAvailability)
	if err != nil {
		utils.PrintLogInfo(&name, 400, "AddAvailability - ConvertDTO", &err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Gagal menambahkan ketersediaan",
			"error":   err.Error(),
		})
		return
	}

	ctx := c.Request.Context()

	// Call the use case with the converted domain models
	if err := h.tc.AddAvailability(ctx, &schedules); err != nil {
		statusCode := http.StatusInternalServerError
		errorMsg := err.Error()

		// Better error handling for specific cases
		if strings.Contains(errorMsg, "invalid") ||
			strings.Contains(errorMsg, "duplicate") ||
			strings.Contains(errorMsg, "overlapping") ||
			strings.Contains(errorMsg, "must be exactly 1 hour") ||
			strings.Contains(errorMsg, "between 07:00 and 22:00") {
			statusCode = http.StatusBadRequest
		}

		utils.PrintLogInfo(&name, statusCode, "AddAvailability - UseCaseError", &err)
		c.JSON(statusCode, gin.H{
			"success": false,
			"message": "Gagal menambahkan ketersediaan",
			"error":   errorMsg,
		})
		return
	}

	utils.PrintLogInfo(&name, 201, "AddAvailability - Success", nil)
	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": fmt.Sprintf("Berhasil menambahkan %d slot jadwal tersedia.", len(schedules)),
		"data": gin.H{
			"total_slots_added": len(schedules),
			"teacher_uuid":      teacherID,
		},
	})
}

// Helper function to convert DTO to domain models with strict validation
func (h *TeacherHandler) convertToTeacherSchedules(teacherID string, slots []dto.SlotsAvailability) ([]domain.TeacherSchedule, error) {
	var schedules []domain.TeacherSchedule

	// Valid day names in Indonesian (case-insensitive)
	validDays := map[string]string{
		"senin":  "Senin",
		"selasa": "Selasa",
		"rabu":   "Rabu",
		"kamis":  "Kamis",
		"jumat":  "Jumat",
		"sabtu":  "Sabtu",
		"minggu": "Minggu",
	}

	// Get your local timezone (WITA)
	loc, err := time.LoadLocation("Asia/Makassar") // WITA timezone
	if err != nil {
		loc = time.FixedZone("WITA", 8*60*60) // UTC+8 as fallback
	}

	for _, slot := range slots {
		// 1. Validate time format
		startTimeLocal, err := time.ParseInLocation("15:04", slot.StartTime, loc)
		if err != nil {
			return nil, fmt.Errorf("format waktu mulai tidak valid: %s, gunakan format HH:MM", slot.StartTime)
		}

		endTimeLocal, err := time.ParseInLocation("15:04", slot.EndTime, loc)
		if err != nil {
			return nil, fmt.Errorf("format waktu selesai tidak valid: %s, gunakan format HH:MM", slot.EndTime)
		}

		// 2. Validate time range (07:00 to 22:00 WITA)
		minTimeLocal, _ := time.ParseInLocation("15:04", "07:00", loc)
		maxTimeLocal, _ := time.ParseInLocation("15:04", "22:00", loc)

		if startTimeLocal.Before(minTimeLocal) {
			return nil, fmt.Errorf("waktu mulai harus pada atau setelah 07:00")
		}
		if endTimeLocal.After(maxTimeLocal) {
			return nil, fmt.Errorf("waktu selesai harus pada atau sebelum 22:00")
		}

		// 3. Validate duration (must be exactly 1 hour OR 30 minutes)
		duration := endTimeLocal.Sub(startTimeLocal)
		durationMinutes := int(duration.Minutes())

		if durationMinutes != 60 && durationMinutes != 30 {
			return nil, fmt.Errorf("durasi harus tepat 1 jam (60 menit) atau 30 menit, didapat %v menit", durationMinutes)
		}

		// 4. Validate start < end
		if !startTimeLocal.Before(endTimeLocal) {
			return nil, fmt.Errorf("waktu mulai harus sebelum waktu selesai")
		}

		// 5. Validate days of the week
		if len(slot.DayOfTheWeek) == 0 {
			return nil, fmt.Errorf("minimal satu hari harus ditentukan")
		}

		for _, day := range slot.DayOfTheWeek {
			dayLower := strings.ToLower(strings.TrimSpace(day))
			dayName, valid := validDays[dayLower]
			if !valid {
				return nil, fmt.Errorf("hari tidak valid: %s, hari yang valid: Senin, Selasa, Rabu, Kamis, Jumat, Sabtu, Minggu", day)
			}

			// Convert to UTC for database storage - NO LONGER NEEDED as we store string "HH:MM"
			// startTimeUTC := startTimeLocal.UTC()
			// endTimeUTC := endTimeLocal.UTC()

			schedule := domain.TeacherSchedule{
				TeacherUUID: teacherID,
				DayOfWeek:   dayName,
				StartTime:   slot.StartTime,  // "HH:MM"
				EndTime:     slot.EndTime,    // "HH:MM"
				Duration:    durationMinutes, // Will be 60 or 30
			}

			schedules = append(schedules, schedule)
		}
	}

	// 6. Check for duplicates within the request
	seen := make(map[string]bool)
	for _, schedule := range schedules {
		// Format times back to WITA for duplicate check key
		// startWITA := schedule.StartTime.In(loc).Format("15:04")
		// endWITA := schedule.EndTime.In(loc).Format("15:04")
		startWITA := schedule.StartTime
		endWITA := schedule.EndTime
		key := fmt.Sprintf("%s-%s-%s", schedule.DayOfWeek, startWITA, endWITA)

		if seen[key] {
			return nil, fmt.Errorf("jadwal duplikat terdeteksi: %s %s-%s",
				schedule.DayOfWeek,
				startWITA,
				endWITA)
		}
		seen[key] = true
	}

	return schedules, nil
}

func (th *TeacherHandler) GetMySchedules(c *gin.Context) {
	name := utils.GetAPIHitter(c)
	userUUID, exists := c.Get("userUUID")
	if !exists {
		utils.PrintLogInfo(&name, 401, "GetMySchedules", nil)
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Tidak terotorisasi: konteks pengguna tidak ditemukan",
			"message": "Gagal mendapatkan jadwal saya",
		})
		return
	}

	teacherSchedules, err := th.tc.GetMySchedules(c.Request.Context(), userUUID.(string))
	if err != nil {
		utils.PrintLogInfo(&name, 500, "GetMySchedules", &err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Gagal mendapatkan jadwal saya",
		})
		return
	}

	utils.PrintLogInfo(&name, 200, "GetMySchedules", nil)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Jadwal berhasil diambil",
		"data":    teacherSchedules, // ✅ not &teacherSchedules
	})
}

func (th *TeacherHandler) GetMyProfile(c *gin.Context) {
	name := utils.GetAPIHitter(c)
	userUUID, exists := c.Get("userUUID")
	if !exists {
		utils.PrintLogInfo(&name, 401, "GetMyProfile", nil)
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Tidak terotorisasi: konteks pengguna tidak ditemukan",
			"message": "Gagal mendapatkan profil saya",
		})
		return
	}

	// Call usecase to get teacher data
	user, err := th.tc.GetMyProfile(c.Request.Context(), userUUID.(string))
	if err != nil {
		utils.PrintLogInfo(&name, 500, "GetMyProfile", &err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Gagal mendapatkan profil saya",
		})
		return
	}

	utils.PrintLogInfo(&name, 200, "GetMyProfile", nil)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    user,
	})
}

func (th *TeacherHandler) UpdateTeacherData(c *gin.Context) {
	name := utils.GetAPIHitter(c)
	userUUID, exists := c.Get("userUUID")

	if !exists {
		utils.PrintLogInfo(&name, 401, "GetMyProfile", nil)
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Tidak terotorisasi: konteks pengguna tidak ditemukan",
			"message": "Gagal memperbarui data guru",
		})
		return
	}
	var req dto.UpdateTeacherProfileRequestByTeacher

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.PrintLogInfo(&name, 400, "UpdateTeacher - BindJSON", &err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   utils.TranslateValidationError(err),
			"message": "Gagal memperbarui data guru",
		})
		return
	}

	filtered := dto.MapCreateTeacherRequestToUserByTeacher(&req)

	if err := th.tc.UpdateTeacherData(c.Request.Context(), userUUID.(string), filtered); err != nil {
		utils.PrintLogInfo(&name, 500, "UpdateTeacher - UseCase", &err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   utils.TranslateDBError(err),
			"message": "Gagal memperbarui data guru",
		})
		return
	}

	utils.PrintLogInfo(&name, 200, "UpdateTeacher", nil)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Profil guru berhasil diperbarui",
	})
}

func (th *TeacherHandler) DeleteAddAvailability(c *gin.Context) {
	name := utils.GetAPIHitter(c)
	userUUID, exists := c.Get("userUUID")
	if !exists {
		utils.PrintLogInfo(&name, 401, "GetMyProfile", nil)
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Gagal menghapus ketersediaan",
			"error":   "Tidak terotorisasi: konteks pengguna tidak ditemukan",
		})
		return
	}

	scheduleID := c.Param("id")
	convertedID, err := strconv.Atoi(scheduleID)
	if err != nil {
		utils.PrintLogInfo(&name, 400, "DeleteAddAvailability - InvalidID", &err)
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Gagal menghapus ketersediaan",
			"error":   "Gagal konversi ID",
		})
		return
	}

	if err := th.tc.DeleteAvailability(c.Request.Context(), convertedID, userUUID.(string)); err != nil {
		utils.PrintLogInfo(&name, 500, "DeleteAddAvailability - UseCase", &err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   utils.TranslateDBError(err),
			"message": "Gagal menghapus ketersediaan",
		})
		return
	}

	utils.PrintLogInfo(&name, 200, "DeleteAddAvailability", nil)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Ketersediaan berhasil dihapus",
	})
}
