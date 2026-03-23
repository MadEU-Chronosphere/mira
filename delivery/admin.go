package delivery

import (
	"chronosphere/config"
	"chronosphere/domain"
	"chronosphere/dto"
	"chronosphere/middleware"
	"chronosphere/utils"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// AdminHandler handles admin routes
type AdminHandler struct {
	uc domain.AdminUseCase
}

func NewAdminHandler(app *gin.Engine, uc domain.AdminUseCase, jwtManager *utils.JWTManager) {
	h := &AdminHandler{uc: uc}

	admin := app.Group("/admin")
	admin.Use(config.AuthMiddleware(jwtManager), middleware.AdminOnly())
	{
		// Admin
		admin.PUT("/modify", h.UpdateAdmin)

		// Teacher
		admin.POST("/teachers", h.CreateTeacher)
		admin.PUT("/teachers/modify/:uuid", h.UpdateTeacher)
		admin.GET("/teachers", h.GetAllTeachers)
		admin.GET("/teachers/:uuid", h.GetTeacherByUUID)

		// Manager
		admin.POST("/managers", h.CreateManager)
		// admin.PUT("/managers/modify/:uuid", h.UpdateManager)
		admin.GET("/managers", h.GetAllManagers)
		admin.GET("/managers/:uuid", h.GetManagerByUUID)

		// Student
		admin.GET("/students", h.GetAllStudents)
		admin.GET("/students/filter", h.GetFilteredStudents)
		admin.GET("/students/:uuid", h.GetStudentByUUID)

		// Users
		admin.GET("/users", h.GetAllUsers)
		admin.DELETE("/users/:uuid", h.DeleteUser)
		admin.PUT("/users/:uuid", h.ClearUserDeletedAt)

		// Packages
		admin.POST("/packages", h.CreatePackage)
		admin.PUT("/packages/modify/:id", h.UpdatePackage)
		admin.GET("/packages/:id", h.GetPackagesByID) // NOTE: get all packages, not by id
		admin.DELETE("/packages/:id", h.DeletePackage)
		admin.GET("/packages", h.GetAllPackages)

		// Instruments
		admin.POST("/instruments", h.CreateInstrument)
		admin.PUT("/instruments/modify/:id", h.UpdateInstrument)
		admin.DELETE("/instruments/:id", h.DeleteInstrument)
		admin.GET("/instruments", h.GetAllInstruments)

		// Settings
		admin.GET("/settings", h.GetSetting)
		admin.PUT("/settings", h.UpdateSetting)

		// Assign package to student
		// admin.POST("/assign-package", h.AssignPackageToStudent)

		// Class Histories
		admin.GET("/class-histories", h.GetAllClassHistories)
	}
}

/* ---------- Request DTOs ---------- */

func (h *AdminHandler) UpdateAdmin(c *gin.Context) {
	adminName := utils.GetAPIHitter(c)
	userUUID, exists := c.Get("userUUID")
	if !exists {
		utils.PrintLogInfo(&adminName, 401, "UpdateAdmin", nil)
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Tidak terotorisasi: konteks pengguna tidak ditemukan",
			"message": "Gagal memperbarui profil admin",
		})
		return
	}
	var req dto.UpdateAdminProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.PrintLogInfo(&adminName, 400, "UpdateAdmin - BindJSON", &err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   utils.TranslateValidationError(err),
			"massage": "Gagal memperbarui profil admin",
		})

		return
	}

	defaultImage := os.Getenv("DEFAULT_PROFILE_IMAGE")
	if req.Image == "" {
		req.Image = defaultImage
	}

	user := dto.MakeUpdateAdminProfileRequest(&req)
	user.UUID = userUUID.(string) // assign dari URL, bukan dari JSON
	if err := h.uc.UpdateAdmin(c.Request.Context(), user); err != nil {
		utils.PrintLogInfo(&adminName, 500, "UpdateAdmin - UseCase", &err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   utils.TranslateDBError(err),
			"message": "Gagal memperbarui profil admin",
		})
		return
	}
	utils.PrintLogInfo(&adminName, 200, "UpdateAdmin", nil)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Profil admin berhasil diperbarui",
	})
}

type CreatePackageRequest struct {
	Name            string  `json:"name" binding:"required,min=3,max=50"`
	Duration        int     `json:"duration" binding:"omitempty,oneof=30 60"`
	ExpiredDuration int     `json:"expired_duration"`
	Price           float64 `json:"price" binding:"required,gt=0"`
	PromoPrice      float64 `json:"promo_price,omitempty"`
	IsPromoActive   bool    `json:"is_promo_active,omitempty"`
	IsTrial         bool    `json:"is_trial,omitempty"`
	Quota           int     `json:"quota" binding:"required,gt=0"`
	Description     string  `json:"description,omitempty"`
	InstrumentID    *int    `json:"instrument_id" binding:"required,gt=0"`
}
type UpdatePackageRequest struct {
	Name            string  `json:"name,omitempty" binding:"omitempty,min=3,max=50"`
	Duration        int     `json:"duration" binding:"omitempty,oneof=30 60"`
	ExpiredDuration int     `json:"expired_duration"`
	Quota           int     `json:"quota,omitempty" binding:"omitempty,gt=0"`
	Description     string  `json:"description,omitempty"`
	InstrumentID    *int    `json:"instrument_id,omitempty" binding:"required,gt=0"`
	Price           float64 `json:"price,omitempty" binding:"omitempty,gt=0"`
	PromoPrice      float64 `json:"promo_price,omitempty"`
	IsPromoActive   bool    `json:"is_promo_active,omitempty"`
	IsTrial         bool    `json:"is_trial,omitempty"`
}

type CreateInstrumentRequest struct {
	Name string `json:"name" binding:"required,min=1,max=30"`
}

type UpdateInstrumentRequest struct {
	Name string `json:"name" binding:"required,min=1,max=30"`
}

type AssignPackageRequest struct {
	StudentUUID string `json:"student_uuid" binding:"required,uuid"`
	PackageID   int    `json:"package_id" binding:"required"`
}

/* ---------- Handlers ---------- */
// PACKAGE MANAGEMENT ======================================================================================================
func (h *AdminHandler) GetPackagesByID(c *gin.Context) {
	name := utils.GetAPIHitter(c)
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		utils.PrintLogInfo(&name, 400, "GetPackagesByID - Atoi", &err)
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "ID paket tidak valid"})
		return
	}

	pkg, err := h.uc.GetPackagesByID(c.Request.Context(), id)
	if err != nil {
		utils.PrintLogInfo(&name, 500, "GetPackagesByID - UseCase", &err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	utils.PrintLogInfo(&name, 200, "GetPackagesByID", nil)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": pkg})
}

func (h *AdminHandler) GetAllClassHistories(c *gin.Context) {
	name := utils.GetAPIHitter(c)
	histories, err := h.uc.GetAllClassHistories(c.Request.Context())
	if err != nil {
		utils.PrintLogInfo(&name, 500, "GetAllClassHistories - UseCase", &err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error(), "message": "Gagal mengambil riwayat kelas"})
		return
	}
	utils.PrintLogInfo(&name, 200, "GetAllClassHistories", nil)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": histories})
}

func (h *AdminHandler) CreatePackage(c *gin.Context) {
	var req CreatePackageRequest
	name := utils.GetAPIHitter(c)

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.PrintLogInfo(&name, 400, "CreatePackage - BindJSON", &err)
		c.JSON(http.StatusBadRequest, gin.H{"message": "Gagal membuat paket", "success": false, "error": utils.TranslateValidationError(err)})
		return
	}

	if !req.IsTrial && req.Duration != 30 && req.Duration != 60 {
		utils.PrintLogInfo(&name, 400, "CreatePackage - Minute Payload Failure", nil)
		c.JSON(http.StatusBadRequest, gin.H{"message": "Gagal membuat paket", "success": false, "error": "Durasi paket hanya bisa 30 atau 60 menit"})
		return
	}

	if req.IsTrial {
		req.Duration = 0
		req.InstrumentID = nil
	}

	pkg := &domain.Package{
		Name:            req.Name,
		Price:           req.Price,
		PromoPrice:      req.PromoPrice,
		IsPromoActive:   req.IsPromoActive,
		IsTrial:         req.IsTrial,
		Quota:           req.Quota,
		Duration:        req.Duration,
		Description:     req.Description,
		InstrumentID:    req.InstrumentID,
		ExpiredDuration: req.ExpiredDuration,
	}

	created, err := h.uc.CreatePackage(c.Request.Context(), pkg)
	if err != nil {
		utils.PrintLogInfo(&name, 500, "CreatePackage - UseCase", &err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Gagal membuat paket", "success": false, "error": utils.TranslateDBError(err)})
		return
	}

	utils.PrintLogInfo(&name, 201, "CreatePackage", nil)
	c.JSON(http.StatusCreated, gin.H{"message": "Paket berhasil dibuat", "success": true, "data": created})
}

func (h *AdminHandler) UpdatePackage(c *gin.Context) {
	name := utils.GetAPIHitter(c)
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		utils.PrintLogInfo(&name, 400, "UpdatePackage - Atoi", &err)
		c.JSON(http.StatusBadRequest, gin.H{"message": "Gagal memperbarui paket", "success": false, "error": "ID paket tidak valid"})
		return
	}

	var req UpdatePackageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.PrintLogInfo(&name, 400, "UpdatePackage - BindJSON", &err)
		c.JSON(http.StatusBadRequest, gin.H{"message": "Gagal memperbarui paket", "success": false, "error": utils.TranslateValidationError(err)})
		return
	}

	pkg := &domain.Package{ID: id}
	if req.Name != "" {
		pkg.Name = req.Name
	}
	if req.Quota != 0 {
		pkg.Quota = req.Quota
	}

	if !req.IsTrial && req.Duration != 30 && req.Duration != 60 {
		utils.PrintLogInfo(&name, 400, "UpdatePackage - Minute Payload Failure", nil)
		c.JSON(http.StatusBadRequest, gin.H{"message": "Gagal memperbarui paket", "success": false, "error": "Durasi paket hanya bisa 30 atau 60 menit"})
		return
	}

	if req.IsTrial {
		req.Duration = 0
		req.InstrumentID = nil
	}

	if req.ExpiredDuration != 0 {
		pkg.ExpiredDuration = req.ExpiredDuration
	}
	
	pkg.Duration = req.Duration
	pkg.InstrumentID = req.InstrumentID
	pkg.Description = req.Description
	pkg.Price = req.Price
	pkg.PromoPrice = req.PromoPrice
	pkg.IsPromoActive = req.IsPromoActive
	pkg.IsTrial = req.IsTrial

	if err := h.uc.UpdatePackage(c.Request.Context(), pkg); err != nil {
		utils.PrintLogInfo(&name, 500, "UpdatePackage - UseCase", &err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Gagal memperbarui paket", "success": false, "error": utils.TranslateDBError(err)})
		return
	}

	utils.PrintLogInfo(&name, 200, "UpdatePackage", nil)
	c.JSON(http.StatusOK, gin.H{"message": "Paket berhasil diperbarui", "success": true})
}

func (h *AdminHandler) DeletePackage(c *gin.Context) {
	name := utils.GetAPIHitter(c)
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		utils.PrintLogInfo(&name, 400, "DeletePackage - Atoi", &err)
		c.JSON(http.StatusBadRequest, gin.H{"message": "Gagal menghapus paket", "success": false, "error": "ID paket tidak valid"})
		return
	}

	if err := h.uc.DeletePackage(c.Request.Context(), id); err != nil {
		utils.PrintLogInfo(&name, 500, "DeletePackage - UseCase", &err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Gagal menghapus paket", "success": false, "error": utils.TranslateDBError(err)})
		return
	}

	utils.PrintLogInfo(&name, 200, "DeletePackage", nil)
	c.JSON(http.StatusOK, gin.H{"message": "Paket berhasil dihapus", "success": true})
}

func (h *AdminHandler) AssignPackageToStudent(c *gin.Context) {
	var req AssignPackageRequest
	name := utils.GetAPIHitter(c)

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.PrintLogInfo(&name, 400, "AssignPackageToStudent - BindJSON", &err)
		c.JSON(http.StatusBadRequest, gin.H{"message": "Gagal menetapkan paket", "success": false, "error": utils.TranslateValidationError(err)})
		return
	}

	if err := h.uc.AssignPackageToStudent(c.Request.Context(), req.StudentUUID, req.PackageID); err != nil {
		utils.PrintLogInfo(&name, 500, "AssignPackageToStudent - UseCase", &err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Gagal menetapkan paket", "success": false, "error": utils.TranslateDBError(err)})
		return
	}

	utils.PrintLogInfo(&name, 200, "AssignPackageToStudent", nil)
	c.JSON(http.StatusOK, gin.H{"message": "Paket berhasil ditetapkan ke siswa", "success": true})
}

// TEACHER =====================================================================================================
func (h *AdminHandler) CreateTeacher(c *gin.Context) {
	var req dto.CreateTeacherRequest // pakai DTO
	adminName := utils.GetAPIHitter(c)

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.PrintLogInfo(&adminName, 400, "CreateTeacher - BindJSON", &err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   utils.TranslateValidationError(err),
			"massage": "Gagal membuat guru",
		})
		return
	}

	user := dto.MapCreateTeacherRequestToUser(&req)

	created, err := h.uc.CreateTeacher(c.Request.Context(), user, req.InstrumentIDs)
	if err != nil {
		utils.PrintLogInfo(&adminName, 500, "CreateTeacher - UseCase", &err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   utils.TranslateDBError(err),
			"massage": "Gagal membuat guru",
		})
		return
	}

	utils.PrintLogInfo(&adminName, 201, "CreateTeacher", nil)
	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    created,
		"message": "Guru berhasil dibuat",
	})
}

func (h *AdminHandler) UpdateTeacher(c *gin.Context) {
	uuid := c.Param("uuid") // ambil UUID dari URL
	var req dto.UpdateTeacherProfileRequest
	adminName := utils.GetAPIHitter(c)

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.PrintLogInfo(&adminName, 400, "UpdateTeacher - BindJSON", &err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   utils.TranslateValidationError(err),
			"massage": "Gagal memperbarui profil guru",
		})
		return
	}

	user := dto.MapUpdateTeacherRequestToUser(&req)
	user.UUID = uuid // assign dari URL, bukan dari JSON

	if err := h.uc.UpdateTeacher(c.Request.Context(), user, req.InstrumentIDs); err != nil {
		utils.PrintLogInfo(&adminName, 500, "UpdateTeacher - UseCase", &err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   utils.TranslateDBError(err),
			"message": "Gagal memperbarui profil guru",
		})
		return
	}

	utils.PrintLogInfo(&adminName, 200, "UpdateTeacher", nil)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Profil guru berhasil diperbarui",
	})
}

func (h *AdminHandler) GetAllTeachers(c *gin.Context) {
	teachers, err := h.uc.GetAllTeachers(c.Request.Context())
	if err != nil {
		utils.PrintLogInfo(nil, 500, "GetAllTeachers - UseCase", &err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error(), "message": "Gagal mengambil data guru"})
		return
	}
	utils.PrintLogInfo(nil, 200, "GetAllTeachers", nil)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": teachers, "message": "Data guru berhasil diambil"})
}

func (h *AdminHandler) GetTeacherByUUID(c *gin.Context) {
	uuid := c.Param("uuid")
	teacher, err := h.uc.GetTeacherByUUID(c.Request.Context(), uuid)
	if err != nil {
		utils.PrintLogInfo(&uuid, 500, "GetTeacherByUUID - UseCase", &err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error(), "message": "Gagal mengambil data guru"})
		return
	}

	utils.PrintLogInfo(&uuid, 200, "GetTeacherByUUID", nil)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": teacher, "message": "Data guru berhasil diambil"})
}

// Managers =====================================================================================================
func (h *AdminHandler) CreateManager(c *gin.Context) {
	var req dto.CreateManagerRequest // pakai DTO
	adminName := utils.GetAPIHitter(c)

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.PrintLogInfo(&adminName, 400, "CreateManager - BindJSON", &err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   utils.TranslateValidationError(err),
			"massage": "Gagal membuat manajer",
		})
		return
	}

	user := dto.MapCreateManagerRequestToUser(&req)

	created, err := h.uc.CreateManager(c.Request.Context(), user)
	if err != nil {
		utils.PrintLogInfo(&adminName, 500, "CreateManager - UseCase", &err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   utils.TranslateDBError(err),
			"massage": "Gagal membuat manajer",
		})
		return
	}

	utils.PrintLogInfo(&adminName, 201, "CreateManager", nil)
	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    created,
		"message": "Manajer berhasil dibuat",
	})
}

func (h *AdminHandler) UpdateManager(c *gin.Context) {
	uuid := c.Param("uuid") // ambil UUID dari URL
	var req dto.UpdateManagerProfileRequest
	adminName := utils.GetAPIHitter(c)

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.PrintLogInfo(&adminName, 400, "UpdateManager - BindJSON", &err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   utils.TranslateValidationError(err),
			"massage": "Gagal memperbarui profil manajer",
		})
		return
	}

	user := dto.MapUpdateManagerRequestToUser(&req)
	user.UUID = uuid // assign dari URL, bukan dari JSON

	if err := h.uc.UpdateManager(c.Request.Context(), user); err != nil {
		utils.PrintLogInfo(&adminName, 500, "UpdateManager - UseCase", &err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   utils.TranslateDBError(err),
			"message": "Gagal memperbarui profil manajer",
		})
		return
	}

	utils.PrintLogInfo(&adminName, 200, "UpdateManager", nil)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Profil manajer berhasil diperbarui",
	})
}

func (h *AdminHandler) GetAllManagers(c *gin.Context) {
	managers, err := h.uc.GetAllManagers(c.Request.Context())
	if err != nil {
		utils.PrintLogInfo(nil, 500, "GetAllManagers - UseCase", &err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Gagal mengambil data manajer",
		},
		)
		return
	}
	utils.PrintLogInfo(nil, 200, "GetAllManagers", nil)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": managers, "message": "Data manajer berhasil diambil"})
}

func (h *AdminHandler) GetManagerByUUID(c *gin.Context) {
	uuid := c.Param("uuid")
	teacher, err := h.uc.GetManagerByUUID(c.Request.Context(), uuid)
	if err != nil {
		utils.PrintLogInfo(&uuid, 500, "GetManagerByUUID - UseCase", &err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error(), "message": "Gagal mengambil data manajer"})
		return
	}

	utils.PrintLogInfo(&uuid, 200, "GetManagerByUUID", nil)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": teacher, "message": "Data manajer berhasil diambil"})
}

func (h *AdminHandler) DeleteUser(c *gin.Context) {
	uuid := c.Param("uuid")
	name := utils.GetAPIHitter(c)
	if err := h.uc.DeleteUser(c.Request.Context(), uuid); err != nil {
		utils.PrintLogInfo(&name, 500, "Turn Off - UseCase", &err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error(), "message": "Gagal menonaktifkan pengguna"})
		return
	}
	utils.PrintLogInfo(&name, 200, "Turn Off", nil)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Pengguna berhasil dinonaktifkan"})
}

func (h *AdminHandler) CreateInstrument(c *gin.Context) {
	var req CreateInstrumentRequest
	name := utils.GetAPIHitter(c)
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.PrintLogInfo(&name, 400, "CreateInstrument - BindJSON", &err)
		c.JSON(http.StatusBadRequest, gin.H{"message": "Gagal membuat instrumen", "success": false, "error": err.Error()})
		return
	}

	inst := &domain.Instrument{Name: strings.ToLower(req.Name)}
	created, err := h.uc.CreateInstrument(c.Request.Context(), inst)
	if err != nil {
		utils.PrintLogInfo(&name, 500, "CreateInstrument - UseCase", &err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Gagal membuat instrumen", "success": false, "error": err.Error()})
		return
	}
	utils.PrintLogInfo(&name, 201, "CreateInstrument", nil)
	c.JSON(http.StatusCreated, gin.H{"message": "Instrumen berhasil dibuat", "success": true, "data": created})
}

func (h *AdminHandler) UpdateInstrument(c *gin.Context) {
	name := utils.GetAPIHitter(c)
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		utils.PrintLogInfo(&name, 400, "UpdateInstrument - Atoi", &err)
		c.JSON(http.StatusBadRequest, gin.H{"message": "Gagal memperbarui instrumen", "success": false, "error": "ID instrumen tidak valid"})
		return
	}

	var req UpdateInstrumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.PrintLogInfo(&name, 400, "UpdateInstrument - BindJSON", &err)
		c.JSON(http.StatusBadRequest, gin.H{"message": "Gagal memperbarui instrumen", "success": false, "error": utils.TranslateValidationError(err)})
		return
	}

	inst := &domain.Instrument{ID: id}
	if req.Name != "" {
		inst.Name = req.Name
	}

	loweredName := strings.ToLower(inst.Name)
	inst.Name = loweredName

	if err := h.uc.UpdateInstrument(c.Request.Context(), inst); err != nil {
		utils.PrintLogInfo(&name, 500, "UpdateInstrument - UseCase", &err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error(), "message": "Gagal memperbarui instrumen"})
		return
	}

	utils.PrintLogInfo(&name, 200, "UpdateInstrument", nil)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Instrumen berhasil diperbarui"})
}

func (h *AdminHandler) DeleteInstrument(c *gin.Context) {
	name := utils.GetAPIHitter(c)

	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		utils.PrintLogInfo(&name, 400, "DeleteInstrument - Atoi", &err)
		c.JSON(http.StatusBadRequest, gin.H{"message": "Gagal menghapus instrumen", "success": false, "error": "ID instrumen tidak valid"})
		return
	}

	if err := h.uc.DeleteInstrument(c.Request.Context(), id); err != nil {
		utils.PrintLogInfo(&name, 500, "DeleteInstrument - UseCase", &err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Gagal menghapus instrumen", "success": false, "error": err.Error()})
		return
	}

	utils.PrintLogInfo(&name, 200, "DeleteInstrument", nil)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Instrumen berhasil dihapus"})
}

func (h *AdminHandler) GetAllPackages(c *gin.Context) {
	name := utils.GetAPIHitter(c)
	pkgs, err := h.uc.GetAllPackages(c.Request.Context())
	if err != nil {
		utils.PrintLogInfo(&name, 500, "GetAllPackages - UseCase", &err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	utils.PrintLogInfo(&name, 200, "GetAllPackages", nil)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": pkgs, "message": "Data paket berhasil diambil"})
}

func (h *AdminHandler) GetAllInstruments(c *gin.Context) {
	name := utils.GetAPIHitter(c)

	insts, err := h.uc.GetAllInstruments(c.Request.Context())
	if err != nil {
		utils.PrintLogInfo(&name, 500, "GetAllInstruments - UseCase", &err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	utils.PrintLogInfo(&name, 200, "GetAllInstruments", nil)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": insts, "message": "Data instrumen berhasil diambil"})
}

func (h *AdminHandler) GetAllUsers(c *gin.Context) {
	users, err := h.uc.GetAllUsers(c.Request.Context())
	if err != nil {
		utils.PrintLogInfo(nil, 500, "GetAllUsers - UseCase", &err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error(), "message": "Gagal mengambil data pengguna"})
		return
	}
	utils.PrintLogInfo(nil, 200, "GetAllUsers", nil)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": users, "message": "Data pengguna berhasil diambil"})
}

func (h *AdminHandler) GetAllStudents(c *gin.Context) {
	name := utils.GetAPIHitter(c)
	students, err := h.uc.GetAllStudents(c.Request.Context())
	if err != nil {
		utils.PrintLogInfo(&name, 500, "GetAllStudents - UseCase", &err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error(), "message": "Gagal mengambil data siswa"})
		return
	}
	utils.PrintLogInfo(&name, 200, "GetAllStudents", nil)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": students, "message": "Data siswa berhasil diambil"})
}

// GetFilteredStudents returns students filtered by activity status.
// Query param: status = active | inactive_short | inactive_long | all (default: all)
// - active:          has at least one active package
// - inactive_short:  no active package, last purchase was < 3 months ago
// - inactive_long:   no active package, last purchase was > 3 months ago (or never bought)
func (h *AdminHandler) GetFilteredStudents(c *gin.Context) {
	name := utils.GetAPIHitter(c)

	statusParam := c.DefaultQuery("status", "all")
	var filter domain.StudentActivityFilter

	switch statusParam {
	case string(domain.StudentFilterActive):
		filter = domain.StudentFilterActive
	case string(domain.StudentFilterInactiveShort):
		filter = domain.StudentFilterInactiveShort
	case string(domain.StudentFilterInactiveLong):
		filter = domain.StudentFilterInactiveLong
	default:
		filter = domain.StudentFilterAll
	}

	students, err := h.uc.GetFilteredStudents(c.Request.Context(), filter)
	if err != nil {
		utils.PrintLogInfo(&name, 500, "GetFilteredStudents - UseCase", &err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
			"message": "Gagal mengambil data siswa berdasarkan filter",
		})
		return
	}

	utils.PrintLogInfo(&name, 200, "GetFilteredStudents", nil)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    students,
		"filter":  statusParam,
		"message": "Data siswa berhasil diambil",
	})
}

func (h *AdminHandler) GetStudentByUUID(c *gin.Context) {
	name := utils.GetAPIHitter(c)
	uuid := c.Param("uuid")
	student, err := h.uc.GetStudentByUUID(c.Request.Context(), uuid)
	if err != nil {
		utils.PrintLogInfo(&name, 500, "GetStudentByUUID - UseCase", &err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error(), "message": "Gagal mengambil data siswa"})
		return
	}

	utils.PrintLogInfo(&name, 200, "GetStudentByUUID", nil)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": student, "message": "Data siswa berhasil diambil"})
}

func (h *AdminHandler) ClearUserDeletedAt(c *gin.Context) {
	name := utils.GetAPIHitter(c)
	uuid := c.Param("uuid")
	if err := h.uc.ClearUserDeletedAt(c.Request.Context(), uuid); err != nil {
		utils.PrintLogInfo(&name, 500, "ClearUserDeletedAt - UseCase", &err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error(), "message": "Gagal memulihkan pengguna"})
		return
	}
	utils.PrintLogInfo(&name, 200, "ClearUserDeletedAt", nil)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Pengguna berhasil dipulihkan"})
}

// SETTINGS =====================================================================================================
type UpdateSettingRequest struct {
	RegistrationFee   float64 `json:"registration_fee" binding:"required,min=0"`
	TeacherCommission float64 `json:"teacher_commission" binding:"required,min=0"`
}

func (h *AdminHandler) GetSetting(c *gin.Context) {
	name := utils.GetAPIHitter(c)
	setting, err := h.uc.GetSetting(c.Request.Context())
	if err != nil {
		utils.PrintLogInfo(&name, 500, "GetSetting - UseCase", &err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error(), "message": "Gagal mengambil pengaturan"})
		return
	}
	utils.PrintLogInfo(&name, 200, "GetSetting", nil)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": setting, "message": "Pengaturan berhasil diambil"})
}

func (h *AdminHandler) UpdateSetting(c *gin.Context) {
	name := utils.GetAPIHitter(c)
	var req UpdateSettingRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.PrintLogInfo(&name, 400, "UpdateSetting - BindJSON", &err)
		c.JSON(http.StatusBadRequest, gin.H{"message": "Gagal memperbarui pengaturan", "success": false, "error": utils.TranslateValidationError(err)})
		return
	}

	setting := &domain.Setting{
		RegistrationFee: req.RegistrationFee,
	}

	if err := h.uc.UpdateSetting(c.Request.Context(), setting); err != nil {
		utils.PrintLogInfo(&name, 500, "UpdateSetting - UseCase", &err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error(), "message": "Gagal memperbarui pengaturan"})
		return
	}

	utils.PrintLogInfo(&name, 200, "UpdateSetting", nil)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Pengaturan berhasil diperbarui"})
}
