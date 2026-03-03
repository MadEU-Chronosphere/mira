package bootstrap

import (
	"chronosphere/config"
	"chronosphere/delivery"
	"chronosphere/middleware"
	"chronosphere/repository"
	"chronosphere/service"
	"chronosphere/utils"
	"fmt"
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
	"gorm.io/gorm"
)

func InitializeAppWithoutWhatsappNotification() (*gin.Engine, *gorm.DB) {
	fmt.Println("🚀 Starting in NO-WA mode")
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

	// init WA message
	// WhatsappClient, _, err := config.InitWA(*addr)
	// if err != nil {
	// 	log.Fatal("❌ Failed to connect to WhatsApp: ", err)
	// }

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
	paymentRepo := repository.NewPaymentRepository(db)

	// Init services
	studentService := service.NewStudentUseCase(studentRepo, nil)
	managementService := service.NewManagerService(managerRepo, nil)
	adminService := service.NewAdminService(adminRepo, nil)
	teacherService := service.NewTeacherService(teacherRepo, nil)
	authService := service.NewAuthService(authRepo, otpRepo, jwtSecret)
	paymentService := service.NewPaymentService(paymentRepo, adminRepo, db, nil)

	// RATE LIMITER
	middleware.InitRateLimiter(redisClient)

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
	delivery.NewPaymentHandler(app, paymentService, authService.GetAccessTokenManager())

	return app, db
}

func InitializeAppWithoutRateLimiter() (*gin.Engine, *gorm.DB) {
	// Load .env
	fmt.Println("🚀 Starting in NO-LIMITER mode")
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️  .env file not found, using system environment variables")
	}

	// ✅ Register custom validators
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		utils.RegisterCustomValidations(v)
	}

	// Boot DB
	db, addr, err := config.BootDB()
	if err != nil {
		log.Fatal("❌ Failed to connect to database: ", err)
	}

	// init WA message
	WhatsappClient, _, err := config.InitWA(*addr)
	if err != nil {
		log.Fatal("❌ Failed to connect to WhatsApp: ", err)
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
	paymentRepo := repository.NewPaymentRepository(db)

	// Init services
	studentService := service.NewStudentUseCase(studentRepo, WhatsappClient)
	managementService := service.NewManagerService(managerRepo, WhatsappClient)
	adminService := service.NewAdminService(adminRepo, WhatsappClient)
	teacherService := service.NewTeacherService(teacherRepo, WhatsappClient)
	authService := service.NewAuthService(authRepo, otpRepo, jwtSecret)
	paymentService := service.NewPaymentService(paymentRepo, adminRepo, db, WhatsappClient)

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
	delivery.NewPaymentHandler(app, paymentService, authService.GetAccessTokenManager())

	return app, db
}

func InitializeFullApp() (*gin.Engine, *gorm.DB) {
	// Load .env
	fmt.Println("🚀 Starting in FULL mode")
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️  .env file not found, using system environment variables")
	}

	// ✅ Register custom validators
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		utils.RegisterCustomValidations(v)
	}

	// Boot DB
	db, addr, err := config.BootDB()
	if err != nil {
		log.Fatal("❌ Failed to connect to database: ", err)
	}

	// init WA message
	WhatsappClient, _, err := config.InitWA(*addr)
	if err != nil {
		log.Fatal("❌ Failed to connect to WhatsApp: ", err)
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
	paymentRepo := repository.NewPaymentRepository(db)

	// Init services
	studentService := service.NewStudentUseCase(studentRepo, WhatsappClient)
	managementService := service.NewManagerService(managerRepo, WhatsappClient)
	adminService := service.NewAdminService(adminRepo, WhatsappClient)
	teacherService := service.NewTeacherService(teacherRepo, WhatsappClient)
	authService := service.NewAuthService(authRepo, otpRepo, jwtSecret)
	paymentService := service.NewPaymentService(paymentRepo, adminRepo, db, WhatsappClient)

	// RATE LIMITER
	middleware.InitRateLimiter(redisClient)

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
	delivery.NewPaymentHandler(app, paymentService, authService.GetAccessTokenManager())

	return app, db
}
