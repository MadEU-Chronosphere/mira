package middleware

import (
	"chronosphere/domain"
	"chronosphere/utils"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// role checking middleware
func AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		name := utils.GetAPIHitter(c)
		role, exists := c.Get("role")
		if !exists || role != domain.RoleAdmin {
			utils.PrintLogInfo(&name, 403, "AdminOnly Middleware - Role Check", nil)
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "Admin access required",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

func TeacherAndAdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		name := utils.GetAPIHitter(c)
		role, exists := c.Get("role")
		if !exists || role == domain.RoleStudent {
			utils.PrintLogInfo(&name, 403, "Admin and Teacher only Middleware - Role Check", nil)
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "Admin and Teacher access required",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

func StudentAndAdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		name := utils.GetAPIHitter(c)
		role, exists := c.Get("role")
		if !exists || role == domain.RoleTeacher {
			utils.PrintLogInfo(&name, 403, "Admin and Student only Middleware - Role Check", nil)
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "Admin and Student access required",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

func StudentOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		name := utils.GetAPIHitter(c)
		role, _ := c.Get("role")
		if role != domain.RoleStudent {
			utils.PrintLogInfo(&name, 403, "Student only Middleware - Role Check", nil)
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "Student access required",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

func ManagerAndAdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		name := utils.GetAPIHitter(c)
		role, exists := c.Get("role")
		if !exists {
			utils.PrintLogInfo(&name, 403, "Admin and Manager only Middleware - Role Check", nil)
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "Admin and Manager access required",
			})
			c.Abort()
			return
		}

		// Check if role is either Admin or Manager
		if role != domain.RoleAdmin && role != domain.RoleManagement {
			utils.PrintLogInfo(&name, 403, "Admin and Manager only Middleware - Role Check", nil)
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "Admin and Manager access required",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

func ManagerOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		name := utils.GetAPIHitter(c)
		role, exists := c.Get("role")
		if !exists {
			utils.PrintLogInfo(&name, 403, "Manager only Middleware - Role Check", nil)
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "Manager access required",
			})
			c.Abort()
			return
		}

		// Check if role is either Admin or Manager
		if role != domain.RoleManagement {
			utils.PrintLogInfo(&name, 403, "Manager only Middleware - Role Check", nil)
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "Manager access required",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

func TeacherOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		name := utils.GetAPIHitter(c)
		role, exists := c.Get("role")
		if !exists || role == domain.RoleStudent || role == domain.RoleManagement || role == domain.RoleAdmin {
			utils.PrintLogInfo(&name, 403, "Teacher only Middleware - Role Check", nil)
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "Teacher access required",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

func ValidateTurnedOffUserMiddleware(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := utils.GetAPIHitter(c)
		role, exists := c.Get("role")
		if !exists {
			utils.PrintLogInfo(&name, 403, "Role Check Failure", nil)
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "User role checker failed / not found",
			})
			c.Abort()
			return
		}

		if role != domain.RoleTeacher && role != domain.RoleManagement {
			c.Next()
			return
		}

		userUUID, exists := c.Get("userUUID")
		if !exists {
			utils.PrintLogInfo(&name, 403, "User UUID checker failure", nil)
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "User UUID not found",
			})
			c.Abort()
			return
		}

		var user domain.User
		err := db.Model(domain.User{}).Where("uuid = ?", userUUID.(string)).First(&user).Error
		if err != nil {
			utils.PrintLogInfo(&name, 500, "Database error when fetching user", &err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Pengguna tidak ditemukan",
				"error":   err.Error(),
			})
			c.Abort()
			return
		}

		if user.DeletedAt != nil {
			utils.PrintLogInfo(&name, 403, "User account is turned off", nil)
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   "Akun anda telah dinonaktifkan, silakan hubungi admin untuk informasi lebih lanjut",
				"message": "Akun dinonaktifkan",
			})
			c.Abort()
			return
		}
	}
}
