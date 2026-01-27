package main

import (
	"chronosphere/config"
	"chronosphere/delivery"
	"chronosphere/repository"
	"chronosphere/service"
	"chronosphere/utils"
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️  .env file not found, using system environment variables")
	}

	// ✅ Register custom validators
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		utils.RegisterCustomValidations(v)
	}

	// Boot DB
	db, _, err := config.BootDB()
	if err != nil {
		log.Fatal("❌ Failed to connect to database: ", err)
	}

	// Redis config
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		log.Fatal("❌ Failed to fetch Redis address from env")
	}

	redisPass := os.Getenv("REDIS_PASSWORD")
	if redisPass == "" {
		log.Fatal("❌ Failed to fetch Redis password from env")
	}

	redisClient := config.InitRedisDB(redisAddr, redisPass, 0)
	// JWT secret validation
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("❌ JWT_SECRET not found in .env")
	}
	if len(jwtSecret) < 32 {
		log.Fatal("❌ JWT_SECRET must be at least 32 characters for security. Generate one with: openssl rand -base64 32")
	}

	// Init repositories
	authRepo := repository.NewAuthRepository(db)
	studentRepo := repository.NewStudentRepository(db)
	teacherRepo := repository.NewTeacherRepository(db)
	managerRepo := repository.NewManagerRepository(db)
	adminRepo := repository.NewAdminRepository(db)
	otpRepo := repository.NewOTPRedisRepository(redisClient)

	// Init services
	studentService := service.NewStudentUseCase(studentRepo)
	managementService := service.NewManagerService(managerRepo)
	adminService := service.NewAdminService(adminRepo)
	teacherService := service.NewTeacherService(teacherRepo)
	authService := service.NewAuthService(authRepo, otpRepo, jwtSecret)

	// RATE LIMITER
	// middleware.InitRateLimiter(redisClient)

	// Init Gin
	app := gin.Default()
	config.InitMiddleware(app)

	// ========================================================================
	// INIT HANDLERS
	// ========================================================================
	delivery.NewAuthHandler(app, authService, db)
	delivery.NewManagerHandler(app, managementService, authService.GetAccessTokenManager(), db)
	delivery.NewStudentHandler(app, studentService, authService.GetAccessTokenManager())
	delivery.NewAdminHandler(app, adminService, authService.GetAccessTokenManager())
	delivery.NewTeacherHandler(app, teacherService, authService.GetAccessTokenManager(), db)

	// ========================================================================
	// GRACEFUL SHUTDOWN SETUP
	// ========================================================================
	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8080"
	}
	srvAddr := ":" + port

	// Create HTTP server with custom configuration
	srv := &http.Server{
		Addr:           srvAddr,
		Handler:        app,
		ReadTimeout:    15 * time.Second,
		WriteTimeout:   15 * time.Second,
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1MB
	}

	// Start server in a goroutine
	go func() {
		log.Printf("🚀 Server running at http://localhost%s", srvAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("❌ Server error: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("🛑 Shutting down server...")

	// The context is used to inform the server it has 10 seconds to finish
	// the request it is currently handling
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("⚠️  Server forced to shutdown: %v", err)
	}

	log.Println("✅ Server exited gracefully")
}

// Helper function to get environment variable as int with default
func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}
