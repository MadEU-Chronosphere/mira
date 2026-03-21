package delivery

import (
	"chronosphere/config"
	"chronosphere/domain"
	"chronosphere/middleware"
	"chronosphere/utils"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ReportHandler exposes reporting endpoints for admin, manager, and teacher roles.
type ReportHandler struct {
	uc domain.ReportUseCase
}

// NewReportHandler registers all report routes.
//
// Route overview:
//
//	GET /admin/reports/student/:uuid/class-history   — admin & manager
//	GET /admin/reports/teachers/teaching             — admin & manager (all teachers or ?teacher_uuid=)
//	GET /teacher/reports/teaching                    — teacher (own data only)
func NewReportHandler(
	app *gin.Engine,
	uc domain.ReportUseCase,
	jwtManager *utils.JWTManager,
) {
	h := &ReportHandler{uc: uc}

	// ── Admin + Manager routes ────────────────────────────────────────────────
	adminGroup := app.Group("/admin/reports")
	adminGroup.Use(config.AuthMiddleware(jwtManager), middleware.ManagerAndAdminOnly())
	{
		// GET /admin/reports/student/:uuid/class-history
		// Returns full class history for the given student UUID.
		// Accessible by admin and manager.
		adminGroup.GET("/student/:uuid/class-history", h.GetStudentClassHistory)

		// GET /admin/reports/teachers/teaching?start_date=2025-01-01&end_date=2025-01-31&teacher_uuid=<optional>
		// Returns teaching report for all teachers (or filtered to one if teacher_uuid is given).
		adminGroup.GET("/teachers/teaching", h.GetTeacherTeachingReport)
	}

	// ── Teacher-self route ────────────────────────────────────────────────────
	teacherGroup := app.Group("/teacher/reports")
	teacherGroup.Use(config.AuthMiddleware(jwtManager), middleware.TeacherOnly())
	{
		// GET /teacher/reports/teaching?start_date=2025-01-01&end_date=2025-01-31
		// Returns the calling teacher's own teaching report.
		teacherGroup.GET("/teaching", h.GetMyTeachingReport)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// GET /admin/reports/student/:uuid/class-history
// ─────────────────────────────────────────────────────────────────────────────

func (h *ReportHandler) GetStudentClassHistory(c *gin.Context) {
	name := utils.GetAPIHitter(c)
	studentUUID := c.Param("uuid")

	if studentUUID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "UUID siswa tidak boleh kosong",
			"message": "Gagal mengambil riwayat kelas siswa",
		})
		return
	}

	histories, err := h.uc.GetClassHistoriesByStudentUUID(c.Request.Context(), studentUUID)
	if err != nil {
		utils.PrintLogInfo(&name, 500, "GetStudentClassHistory - UseCase", &err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Gagal mengambil riwayat kelas siswa",
		})
		return
	}

	utils.PrintLogInfo(&name, 200, "GetStudentClassHistory", nil)
	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"student_uuid": studentUUID,
		"data":         histories,
		"message":      "Riwayat kelas siswa berhasil diambil",
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// GET /admin/reports/teachers/teaching
//
// Query params:
//
//	start_date    YYYY-MM-DD   (optional, default: first day of current month)
//	end_date      YYYY-MM-DD   (optional, default: last day of current month)
//	teacher_uuid  UUID         (optional, filter to one teacher)
//
// ─────────────────────────────────────────────────────────────────────────────

func (h *ReportHandler) GetTeacherTeachingReport(c *gin.Context) {
	name := utils.GetAPIHitter(c)

	filter := domain.TeacherTeachingReportFilter{
		TeacherUUID: c.Query("teacher_uuid"),
		StartDate:   c.Query("start_date"),
		EndDate:     c.Query("end_date"),
	}

	reports, err := h.uc.GetTeacherTeachingReport(c.Request.Context(), filter)
	if err != nil {
		utils.PrintLogInfo(&name, 400, "GetTeacherTeachingReport - UseCase", &err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Gagal mengambil laporan mengajar guru",
		})
		return
	}

	utils.PrintLogInfo(&name, 200, "GetTeacherTeachingReport", nil)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"filter": gin.H{
			"start_date":   filter.StartDate,
			"end_date":     filter.EndDate,
			"teacher_uuid": filter.TeacherUUID,
		},
		"total":   len(reports),
		"data":    reports,
		"message": "Laporan mengajar guru berhasil diambil",
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// GET /teacher/reports/teaching
//
// Query params:
//
//	start_date   YYYY-MM-DD   (optional)
//	end_date     YYYY-MM-DD   (optional)
//
// ─────────────────────────────────────────────────────────────────────────────

func (h *ReportHandler) GetMyTeachingReport(c *gin.Context) {
	name := utils.GetAPIHitter(c)

	uuidVal, exists := c.Get("userUUID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "konteks pengguna tidak ditemukan",
			"message": "Gagal mengambil laporan mengajar",
		})
		return
	}

	teacherUUID, _ := uuidVal.(string)

	report, err := h.uc.GetMyTeachingReport(
		c.Request.Context(),
		teacherUUID,
		c.Query("start_date"),
		c.Query("end_date"),
	)
	if err != nil {
		utils.PrintLogInfo(&name, 500, "GetMyTeachingReport - UseCase", &err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Gagal mengambil laporan mengajar",
		})
		return
	}

	utils.PrintLogInfo(&name, 200, "GetMyTeachingReport", nil)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    report,
		"message": "Laporan mengajar berhasil diambil",
	})
}