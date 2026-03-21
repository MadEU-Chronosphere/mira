package config

import (
	"chronosphere/domain"
	"chronosphere/utils"
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"reflect"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func GetDatabaseURL() string {
	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_NAME"),
		os.Getenv("DB_SSLMODE"),
	)
	return dsn
}

func BootDB() (*gorm.DB, *string, error) {
	// ✅ Protect against panic
	defer func() {
		if r := recover(); r != nil {
			log.Printf("🔥 Database initialization panic recovered: %v", r)
		}
	}()

	address := GetDatabaseURL()

	// Setup logger level (debug mode vs production)
	var gormLogger logger.Interface
	if os.Getenv("APP_ENV") == "development" {
		gormLogger = logger.Default.LogMode(logger.Info)
	} else {
		gormLogger = logger.Default.LogMode(logger.Silent)
	}

	// ✅ Add connection timeout and error handling
	db, err := gorm.Open(postgres.Open(address), &gorm.Config{
		Logger:                                   gormLogger,
		DisableForeignKeyConstraintWhenMigrating: false,
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	})
	if err != nil {
		log.Printf("❌ Failed to connect to database: %v", err)
		return nil, nil, fmt.Errorf("database connection failed: %w", err)
	}

	// Setup connection pool with safer defaults
	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get database instance: %w", err)
	}

	// ✅ Configure connection pool with production-safe values
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)
	sqlDB.SetConnMaxIdleTime(30 * time.Minute)

	// ✅ Test connection with timeout
	if err := testConnection(sqlDB); err != nil {
		return nil, nil, fmt.Errorf("database connection test failed: %w", err)
	}

	// 🔄 Migrate in dependency-safe order with error handling
	if err := runMigrations(db); err != nil {
		return nil, nil, fmt.Errorf("migration failed: %w", err)
	}

	// ✅ Seed initial data with error handling
	if err := seedInitialData(db); err != nil {
		log.Printf("⚠️  Failed to seed initial data: %v", err)
		// Don't fail - seeding is not critical
	}

	log.Print("✅ Connected to ", utils.ColorText("Database", utils.Green), " successfully")
	return db, &address, nil
}

// ✅ Test database connection with timeout
func testConnection(sqlDB *sql.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}

	return nil
}

// ✅ Run migrations with error handling
func runMigrations(db *gorm.DB) error {
	models := []interface{}{
		&domain.User{},
		&domain.Instrument{},
		&domain.TeacherProfile{},
		&domain.TeacherAlbum{},
		&domain.TeacherPayment{},
		&domain.StudentProfile{},
		&domain.Package{},
		&domain.StudentPackage{},
		&domain.TeacherSchedule{},
		&domain.Booking{},
		&domain.Payment{},
		&domain.ClassHistory{},
		&domain.ClassDocumentation{},
		&domain.Setting{},
	}

	for _, m := range models {
		modelName := reflect.TypeOf(m).Elem().Name()

		// ✅ Protect each migration
		if err := func() error {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("🔥 Migration panic for %s: %v", modelName, r)
				}
			}()

			return db.AutoMigrate(m)
		}(); err != nil {
			log.Printf("❌ Failed to migrate model %s: %v", modelName, err)
			return fmt.Errorf("failed to migrate %s: %w", modelName, err)
		}

		log.Printf("✅ Migrated %s", modelName)
	}

	return nil
}

// ✅ Seed initial data with error handling
func seedInitialData(db *gorm.DB) error {
	// Seed admin user
	if err := seedAdminUser(db); err != nil {
		log.Printf("⚠️  Failed to seed admin user: %v", err)
		// Continue - not critical
	}

	// Seed instruments
	if err := seedInstruments(db); err != nil {
		log.Printf("⚠️  Failed to seed instruments: %v", err)
		// Continue - not critical
	}

	return nil
}

// ✅ Seed admin user with protection
func seedAdminUser(db *gorm.DB) error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("🔥 Admin seeding panic recovered: %v", r)
		}
	}()

	var count int64
	if err := db.Model(&domain.User{}).Where("role = ?", domain.RoleAdmin).Count(&count).Error; err != nil {
		return fmt.Errorf("failed to count admin users: %w", err)
	}

	if count == 0 {
		adminEmail := os.Getenv("ADMIN_EMAIL")
		adminPass := os.Getenv("ADMIN_PASSWORD")
		adminName := os.Getenv("ADMIN_NAME")
		adminPhone := os.Getenv("ADMIN_PHONE")
		adminGender := os.Getenv("ADMIN_GENDER")

		if adminEmail == "" || adminPass == "" {
			log.Print("⚠️  Skipping admin seeding — missing ADMIN_EMAIL or ADMIN_PASSWORD in env")
			return nil
		}

		// ✅ Validate password strength
		if len(adminPass) < 8 {
			log.Print("⚠️  Admin password too short, minimum 8 characters required")
			return nil
		}

		hashed, err := bcrypt.GenerateFromPassword([]byte(adminPass), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("failed to hash admin password: %w", err)
		}

		adminUser := domain.User{
			Name:     adminName,
			Email:    adminEmail,
			Phone:    adminPhone,
			Password: string(hashed),
			Role:     domain.RoleAdmin,
			Gender:   adminGender,
		}

		if err := db.Create(&adminUser).Error; err != nil {
			return fmt.Errorf("failed to create admin user: %w", err)
		}

		log.Printf("✅ Seeded admin user: %s", adminEmail)
	}

	return nil
}

// ✅ Seed instruments with protection
func seedInstruments(db *gorm.DB) error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("🔥 Instrument seeding panic recovered: %v", r)
		}
	}()

	commonInstruments := []string{
		"guitar", "piano", "violin", "drums", "bass",
		"ukulele", "vocal", "flute", "saxophone",
	}

	for _, name := range commonInstruments {
		var exists int64
		if err := db.Model(&domain.Instrument{}).
			Where("LOWER(name) = LOWER(?)", name).
			Count(&exists).Error; err != nil {
			log.Printf("⚠️  Failed to check instrument '%s': %v", name, err)
			continue
		}

		if exists == 0 {
			if err := db.Create(&domain.Instrument{Name: name}).Error; err != nil {
				log.Printf("⚠️  Failed to seed instrument '%s': %v", name, err)
				continue
			}
			log.Printf("✅ Seeded instrument: %s", name)
		}
	}

	return nil
}
