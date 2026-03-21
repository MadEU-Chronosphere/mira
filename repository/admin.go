package repository

import (
	"chronosphere/domain"
	"chronosphere/utils"
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgconn"
	"gorm.io/gorm"
)

type adminRepo struct {
	db *gorm.DB
}

func NewAdminRepository(db *gorm.DB) domain.AdminRepository {
	return &adminRepo{db: db}
}

func (r *adminRepo) UpdateAdmin(ctx context.Context, payload domain.User) error {
	// Mulai transaction
	tx := r.db.WithContext(ctx).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Cek apakah user exists dan belum dihapus
	var existingUser domain.User
	err := tx.Where("uuid = ? AND role = ?", payload.UUID, domain.RoleAdmin).First(&existingUser).Error
	if err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("admin tidak ditemukan")
		}
		return fmt.Errorf("error mencari admin: %w", err)
	}

	// Check email duplicate dengan user lain
	var emailCount int64
	err = tx.Model(&domain.User{}).
		Where("email = ? AND uuid != ?", payload.Email, payload.UUID).
		Count(&emailCount).Error
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("error checking email: %w", err)
	}
	if emailCount > 0 {
		tx.Rollback()
		return errors.New("email sudah digunakan oleh user lain")
	}

	// Check phone duplicate dengan user lain
	var phoneCount int64
	err = tx.Model(&domain.User{}).
		Where("phone = ? AND uuid != ?", payload.Phone, payload.UUID).
		Count(&phoneCount).Error
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("error checking phone: %w", err)
	}
	if phoneCount > 0 {
		tx.Rollback()
		return errors.New("nomor telepon sudah digunakan oleh user lain")
	}

	// Update user data
	err = tx.Model(&domain.User{}).
		Where("uuid = ?", payload.UUID).
		Updates(payload).Error
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("gagal memperbarui data admin: %w", err)
	}

	// Commit transaction jika semua berhasil
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("gagal commit transaction: %w", err)
	}

	return nil
}

func (r *adminRepo) ClearUserDeletedAt(ctx context.Context, userUUID string) error {
	// First check if user exists and is deleted
	var count int64
	err := r.db.WithContext(ctx).Model(&domain.User{}).
		Where("uuid = ? AND deleted_at IS NOT NULL", userUUID).
		Count(&count).Error

	if err != nil {
		return fmt.Errorf("error memeriksa status pengguna: %w", err)
	}

	if count == 0 {
		return fmt.Errorf("pengguna tidak ditemukan atau tidak dalam status non-aktif")
	}

	// Update directly
	result := r.db.WithContext(ctx).Model(&domain.User{}).
		Where("uuid = ?", userUUID).
		Update("deleted_at", nil)

	if result.Error != nil {
		return fmt.Errorf("gagal mengaktifkan kembali pengguna: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("pengguna tidak ditemukan")
	}

	return nil
}

// Class
func (r *adminRepo) GetAllClassHistories(ctx context.Context) (*[]domain.ClassHistory, error) {
	var histories []domain.ClassHistory

	err := r.db.WithContext(ctx).
		Preload("Booking").
		Preload("Booking.Schedule").
		Preload("Booking.Schedule.Teacher").
		Preload("Booking.Schedule.TeacherProfile").
		Preload("Booking.Schedule.TeacherProfile.Instruments").
		Preload("Booking.Student").
		Preload("Booking.PackageUsed").
		Preload("Booking.PackageUsed.Package").
		Preload("Booking.PackageUsed.Package.Instrument").
		Preload("Documentations").
		Joins("LEFT JOIN bookings ON class_histories.booking_id = bookings.id"). // Join untuk akses class_date
		Order("bookings.class_date DESC").                                       // Sort by actual class date
		Find(&histories).Error

	if err != nil {
		return nil, fmt.Errorf("failed to fetch class history: %w", err)
	}

	return &histories, nil
}

// Managers
func (r *adminRepo) CreateManager(ctx context.Context, user *domain.User) (*domain.User, error) {
	tx := r.db.WithContext(ctx).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 1️⃣ Pastikan user belum ada (by email / phone)
	var existing domain.User
	if err := tx.
		Where("(email = ? OR phone = ?)", user.Email, user.Phone).
		First(&existing).Error; err == nil {
		tx.Rollback()
		return nil, errors.New("email atau nomor telepon sudah digunakan")
	}

	// 2️⃣ Set StudentProfile ke nil karena ini function khusus buat teacher
	user.StudentProfile = nil
	user.TeacherProfile = nil
	defImage := os.Getenv("DEFAULT_PROFILE_IMAGE")

	// FIX: Check if Image is nil first, then check if it's empty
	if user.Image == nil || *user.Image == "" {
		user.Image = &defImage
	}
	// 3️⃣ Buat user baru
	if err := tx.Create(user).Error; err != nil {
		tx.Rollback()
		return nil, errors.New(utils.TranslateDBError(err))
	}

	// 4️⃣ Refresh user untuk dapat UUID (jika belum ada)
	if user.UUID == "" {
		if err := tx.
			Where("email = ? AND deleted_at IS NULL", user.Email).
			First(user).Error; err != nil {
			tx.Rollback()
			return nil, errors.New("gagal mendapatkan UUID user")
		}
	}

	// 8️⃣ Commit transaksi
	if err := tx.Commit().Error; err != nil {
		return nil, errors.New(utils.TranslateDBError(err))
	}

	return user, nil
}

func (r *adminRepo) UpdateManager(ctx context.Context, user *domain.User) error {
	// Mulai transaction
	tx := r.db.WithContext(ctx).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Cek apakah user exists dan belum dihapus
	var existingUser domain.User
	err := tx.Where("uuid = ? AND deleted_at IS NULL", user.UUID).First(&existingUser).Error
	if err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("manager tidak ditemukan")
		}
		return fmt.Errorf("error mencari manager: %w", err)
	}

	// Check email duplicate dengan user lain
	var emailCount int64
	err = tx.Model(&domain.User{}).
		Where("email = ? AND uuid != ? AND deleted_at IS NULL", user.Email, user.UUID).
		Count(&emailCount).Error
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("error checking email: %w", err)
	}
	if emailCount > 0 {
		tx.Rollback()
		return errors.New("email sudah digunakan oleh user lain")
	}

	// Check phone duplicate dengan user lain
	var phoneCount int64
	err = tx.Model(&domain.User{}).
		Where("phone = ? AND uuid != ? AND deleted_at IS NULL", user.Phone, user.UUID).
		Count(&phoneCount).Error
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("error checking phone: %w", err)
	}
	if phoneCount > 0 {
		tx.Rollback()
		return errors.New("nomor telepon sudah digunakan oleh user lain")
	}

	// Update user data
	err = tx.Model(&domain.User{}).
		Where("uuid = ?", user.UUID).
		Updates(user).Error
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("gagal memperbarui data manager: %w", err)
	}

	// Commit transaction jika semua berhasil
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("gagal commit transaction: %w", err)
	}

	return nil
}

func (r *adminRepo) GetAllManagers(ctx context.Context) ([]domain.User, error) {
	var teachers []domain.User
	if err := r.db.WithContext(ctx).
		Where("role = ?", domain.RoleManagement).
		Find(&teachers).Error; err != nil {
		return nil, errors.New(utils.TranslateDBError(err))
	}

	return teachers, nil
}

func (r *adminRepo) GetManagerByUUID(ctx context.Context, uuid string) (*domain.User, error) {
	var teacher domain.User
	if err := r.db.WithContext(ctx).
		Where("uuid = ? AND role = ?", uuid, domain.RoleManagement).
		First(&teacher).Error; err != nil {
		return nil, errors.New(utils.TranslateDBError(err))
	}

	return &teacher, nil
}

func (r *adminRepo) UpdateInstrument(ctx context.Context, instrument *domain.Instrument) error {
	var existing domain.Instrument

	err := r.db.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", instrument.ID).First(&existing).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("instrumen tidak ditemukan")
		}
		return errors.New(utils.TranslateDBError(err))
	}

	var count int64
	err = r.db.WithContext(ctx).
		Model(&domain.Instrument{}).
		Where("name = ? AND id != ? AND deleted_at IS NULL", instrument.Name, instrument.ID).
		Count(&count).Error
	if err != nil {
		return errors.New(utils.TranslateDBError(err))
	}
	if count > 0 {
		return errors.New("nama instrumen sudah digunakan")
	}

	// ✅ Update the instrument
	if err := r.db.WithContext(ctx).Save(instrument).Error; err != nil {
		return errors.New(utils.TranslateDBError(err))
	}

	return nil
}

func (r *adminRepo) UpdatePackage(ctx context.Context, pkg *domain.Package) error {

	//check the name
	var existing domain.Package
	if err := r.db.WithContext(ctx).
		Where("id = ? AND deleted_at IS NULL", pkg.ID).
		First(&existing).Error; err != nil {

		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("paket tidak ditemukan")
		}
		return errors.New(utils.TranslateDBError(err))
	}

	//check the name
	var nameExistStruct domain.Package
	err := r.db.WithContext(ctx).Model(&domain.Package{}).Where("name = ? AND id != ? AND deleted_at IS NULL", pkg.Name, pkg.ID).First(&nameExistStruct).Error
	if err == nil {

		return errors.New("nama paket sudah digunakan")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.New(utils.TranslateDBError(err))
	}

	// check instrument id exists
	var instrumentCount int64
	err = r.db.WithContext(ctx).Model(&domain.Instrument{}).
		Where("id = ? AND deleted_at IS NULL", pkg.InstrumentID).
		Count(&instrumentCount).Error
	if err != nil {
		return errors.New(utils.TranslateDBError(err))
	}
	if instrumentCount == 0 {
		return errors.New("instrumen tidak ditemukan")
	}

	if err := r.db.WithContext(ctx).Save(pkg).Error; err != nil {
		return errors.New(utils.TranslateDBError(err))
	}

	return nil
}

// AssignPackageToStudent assigns a package to a student
func (r *adminRepo) AssignPackageToStudent(ctx context.Context, studentUUID string, packageID int) (*domain.User, *domain.Package, error) {
	tx := r.db.WithContext(ctx).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()
 
	// 1. Check student existence
	var student domain.User
	if err := tx.Where("uuid = ? AND role = ? AND deleted_at IS NULL", studentUUID, domain.RoleStudent).
		First(&student).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, errors.New("siswa tidak ditemukan")
		}
		return nil, nil, errors.New(utils.TranslateDBError(err))
	}
 
	// 2. Check package existence
	var pkg domain.Package
	if err := tx.Preload("Instrument").
		Where("id = ? AND deleted_at IS NULL", packageID).
		First(&pkg).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, errors.New("paket tidak ditemukan")
		}
		return nil, nil, errors.New(utils.TranslateDBError(err))
	}
 
	// 3. Ensure student profile exists
	var studentProfile domain.StudentProfile
	if err := tx.Where("user_uuid = ?", studentUUID).First(&studentProfile).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			studentProfile = domain.StudentProfile{UserUUID: studentUUID}
			if err := tx.Create(&studentProfile).Error; err != nil {
				tx.Rollback()
				return nil, nil, errors.New(utils.TranslateDBError(err))
			}
		} else {
			tx.Rollback()
			return nil, nil, errors.New(utils.TranslateDBError(err))
		}
	}
 
	// 4. Snapshot the effective price at purchase time.
	//    Use promo price when promo is active and promo price is set,
	//    otherwise fall back to the base price.
	//    This value is stored permanently on the StudentPackage row so that
	//    teacher commission calculations remain accurate even if the package
	//    price or promo status changes later.
	pricePaid := pkg.Price
	if pkg.IsPromoActive && pkg.PromoPrice > 0 {
		pricePaid = pkg.PromoPrice
	}
 
	// 5. Assign new package with snapshotted price
	newSub := domain.StudentPackage{
		StudentUUID:    studentUUID,
		PackageID:      packageID,
		RemainingQuota: pkg.Quota,
		PricePaid:      pricePaid,
		StartDate:      time.Now(),
		EndDate:        time.Now().AddDate(0, 0, pkg.ExpiredDuration),
	}
 
	if err := tx.Create(&newSub).Error; err != nil {
		tx.Rollback()
		return nil, nil, errors.New(utils.TranslateDBError(err))
	}
 
	if err := tx.Commit().Error; err != nil {
		return nil, nil, errors.New(utils.TranslateDBError(err))
	}
 
	return &student, &pkg, nil
}
 

// CreatePackage inserts a new package
func (r *adminRepo) CreatePackage(ctx context.Context, pkg *domain.Package) (*domain.Package, error) {
	err := r.db.WithContext(ctx).Where("name = ? AND deleted_at IS NULL", pkg.Name).First(&domain.Package{}).Error
	if err == nil {
		return nil, errors.New("nama paket sudah digunakan")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errors.New(utils.TranslateDBError(err))
	}

	//check instrument id exists
	var instrumentCount int64
	err = r.db.WithContext(ctx).Model(&domain.Instrument{}).
		Where("id = ? AND deleted_at IS NULL", pkg.InstrumentID).
		Count(&instrumentCount).Error
	if err != nil {
		return nil, errors.New(utils.TranslateDBError(err))
	}
	if instrumentCount == 0 {
		return nil, errors.New("instrumen tidak ditemukan")
	}

	if err := r.db.WithContext(ctx).Create(pkg).Error; err != nil {
		return nil, errors.New(utils.TranslateDBError(err))
	}

	pkg.Instrument.ID = *pkg.InstrumentID
	return pkg, nil
}

// ✅ Create Instrument
func (r *adminRepo) CreateInstrument(ctx context.Context, instrument *domain.Instrument) (*domain.Instrument, error) {
	// Cek apakah sudah ada instrument dengan nama sama & belum dihapus
	var existing domain.Instrument
	if err := r.db.WithContext(ctx).
		Where("name = ? AND deleted_at IS NULL", instrument.Name).
		First(&existing).Error; err == nil {
		// Sudah ada, return error user-friendly
		return nil, errors.New(utils.TranslateDBError(&pgconn.PgError{
			Code:    "23505",
			Message: "duplicate key value violates unique constraint \"instruments_name_key\"",
		}))

	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		// Error lain saat check
		return nil, errors.New(utils.TranslateDBError(err))
	}

	// Simpan instrument baru
	if err := r.db.WithContext(ctx).Create(instrument).Error; err != nil {
		return nil, errors.New(utils.TranslateDBError(err))
	}

	return instrument, nil
}

// GetAllPackages returns all packages
func (r *adminRepo) GetAllPackages(ctx context.Context) ([]domain.Package, error) {
	var packages []domain.Package
	if err := r.db.WithContext(ctx).
		Where("deleted_at IS NULL").
		Preload("Instrument", "deleted_at IS NULL"). // ✅ Preload instrument yang aktif
		Find(&packages).Error; err != nil {
		return nil, errors.New(utils.TranslateDBError(err))
	}

	return packages, nil
}

// ✅ Get All Instruments (skip soft deleted)
func (r *adminRepo) GetAllInstruments(ctx context.Context) ([]domain.Instrument, error) {
	var instruments []domain.Instrument

	if err := r.db.WithContext(ctx).
		Where("deleted_at IS NULL").
		Find(&instruments).Error; err != nil {
		return nil, errors.New(utils.TranslateDBError(err))
	}

	return instruments, nil
}

// GetAllUsers returns all users
func (r *adminRepo) GetAllUsers(ctx context.Context) ([]domain.User, error) {
	var users []domain.User
	err := r.db.WithContext(ctx).Find(&users).Error
	return users, err
}

// GetAllStudents returns all users with role=student
func (r *adminRepo) GetAllStudents(ctx context.Context) ([]domain.User, error) {
	var students []domain.User
	err := r.db.WithContext(ctx).
		Where("role = ? AND deleted_at IS NULL", domain.RoleStudent).
		Find(&students).Error

	return students, err
}

// GetFilteredStudents returns students filtered by activity status:
//   - active:         has at least one active package (remaining_quota > 0 AND end_date >= now)
//   - inactive_short: no active package, last package purchase was within the last 3 months
//   - inactive_long:  no active package, last package purchase was more than 3 months ago (or never purchased)
//   - all (default):  returns all students without filter
func (r *adminRepo) GetFilteredStudents(ctx context.Context, filter domain.StudentActivityFilter) ([]domain.User, error) {
	var students []domain.User
	threeMonthsAgo := time.Now().AddDate(0, -3, 0)
	baseQuery := r.db.WithContext(ctx).Where("role = ? AND deleted_at IS NULL", domain.RoleStudent)

	switch filter {
	case domain.StudentFilterActive:
		// Has an active student_package
		err := baseQuery.
			Where(`uuid IN (
				SELECT DISTINCT sp.student_uuid
				FROM student_packages sp
				WHERE sp.remaining_quota > 0
				  AND sp.end_date >= ?
			)`, time.Now()).
			Find(&students).Error
		return students, err

	case domain.StudentFilterInactiveShort:
		// No active package AND last purchase was < 3 months ago
		err := baseQuery.
			Where(`uuid NOT IN (
				SELECT DISTINCT sp.student_uuid
				FROM student_packages sp
				WHERE sp.remaining_quota > 0
				  AND sp.end_date >= ?
			)`, time.Now()).
			Where(`uuid IN (
				SELECT DISTINCT sp2.student_uuid
				FROM student_packages sp2
				WHERE sp2.start_date >= ?
			)`, threeMonthsAgo).
			Find(&students).Error
		return students, err

	case domain.StudentFilterInactiveLong:
		// No active package AND (never purchased OR last purchase was > 3 months ago)
		err := baseQuery.
			Where(`uuid NOT IN (
				SELECT DISTINCT sp.student_uuid
				FROM student_packages sp
				WHERE sp.remaining_quota > 0
				  AND sp.end_date >= ?
			)`, time.Now()).
			Where(`uuid NOT IN (
				SELECT DISTINCT sp2.student_uuid
				FROM student_packages sp2
				WHERE sp2.start_date >= ?
			)`, threeMonthsAgo).
			Find(&students).Error
		return students, err

	default: // StudentFilterAll or empty/unknown
		err := baseQuery.Find(&students).Error
		return students, err
	}
}

// GetStudentByUUID fetches a student by UUID
func (r *adminRepo) GetStudentByUUID(ctx context.Context, uuid string) (*domain.User, error) {
	var student domain.User
	err := r.db.WithContext(ctx).
		Preload("StudentProfile.Packages", "end_date >= ? AND remaining_quota > 0", time.Now()).
		Preload("StudentProfile.Packages.Package.Instrument").
		Where("uuid = ? AND role = ? AND deleted_at IS NULL", uuid, domain.RoleStudent).
		First(&student).Error
	if err != nil {
		return nil, err
	}
	return &student, nil
}

// ✅ Delete Instrument (soft delete aware)
func (r *adminRepo) DeleteInstrument(ctx context.Context, id int) error {
	// Cek apakah instrument masih aktif (belum dihapus)
	var existing domain.Instrument
	if err := r.db.WithContext(ctx).
		Where("id = ? AND deleted_at IS NULL", id).
		First(&existing).Error; err != nil {

		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New(utils.TranslateDBError(err))
		}
		return errors.New(utils.TranslateDBError(err))
	}

	// Lakukan soft delete
	if err := r.db.WithContext(ctx).Delete(&existing).Error; err != nil {
		return errors.New(utils.TranslateDBError(err))
	}

	return nil
}
func (r *adminRepo) DeletePackage(ctx context.Context, id int) error {
	return r.db.WithContext(ctx).Delete(&domain.Package{}, "id = ?", id).Error
}

// TEACHER MANAGEMENT
func (r *adminRepo) CreateTeacher(ctx context.Context, user *domain.User, instrumentIDs []int) (*domain.User, error) {
	tx := r.db.WithContext(ctx).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 1️⃣ Pastikan user belum ada (by email / phone)
	var existing domain.User
	if err := tx.
		Where("(email = ? OR phone = ?)", user.Email, user.Phone).
		First(&existing).Error; err == nil {
		tx.Rollback()
		return nil, errors.New("email atau nomor telepon sudah digunakan")
	}

	if len(instrumentIDs) > 0 {
		// 6a. Validasi: Pastikan semua instrument IDs ada di database
		var validInstruments []domain.Instrument
		if err := tx.
			Where("id IN ? AND deleted_at IS NULL", instrumentIDs).
			Find(&validInstruments).Error; err != nil {
			tx.Rollback()
			return nil, errors.New("gagal memvalidasi instrumen")
		}

		// 6b. Cek apakah jumlah instrument yang ditemukan sama dengan yang diminta
		if len(validInstruments) != len(instrumentIDs) {
			tx.Rollback()

			// Cari instrument IDs yang tidak valid
			var foundIDs []int
			for _, inst := range validInstruments {
				foundIDs = append(foundIDs, inst.ID)
			}

			invalidIDs := findMissingIDs(instrumentIDs, foundIDs)
			return nil, fmt.Errorf("instrumen dengan ID %v tidak ditemukan atau sudah dihapus", invalidIDs)
		}

	}
	// 2️⃣ Set StudentProfile ke nil karena ini function khusus buat teacher
	user.StudentProfile = nil
	defImage := os.Getenv("DEFAULT_PROFILE_IMAGE")

	// FIX: Check if Image is nil first, then check if it's empty
	if user.Image == nil || *user.Image == "" {
		user.Image = &defImage
	}

	// 3️⃣ Buat user baru
	if err := tx.Create(user).Error; err != nil {
		tx.Rollback()
		return nil, errors.New(utils.TranslateDBError(err))
	}

	// 4️⃣ Refresh user untuk dapat UUID (jika belum ada)
	if user.UUID == "" {
		if err := tx.
			Where("email = ? AND deleted_at IS NULL", user.Email).
			First(user).Error; err != nil {
			tx.Rollback()
			return nil, errors.New("gagal mendapatkan UUID user")
		}
	}

	// 7️⃣ Preload data lengkap untuk response
	if err := tx.
		Preload("TeacherProfile.Instruments", "deleted_at IS NULL"). // ✅ Filter instruments yang aktif
		Where("uuid = ? AND deleted_at IS NULL", user.UUID).
		First(user).Error; err != nil {
		tx.Rollback()
		return nil, errors.New("gagal memuat data user")
	}

	// 8️⃣ Commit transaksi
	if err := tx.Commit().Error; err != nil {
		return nil, errors.New(utils.TranslateDBError(err))
	}

	return user, nil
}

// Helper function untuk mencari IDs yang tidak valid
func findMissingIDs(requestedIDs, foundIDs []int) []int {
	foundMap := make(map[int]bool)
	for _, id := range foundIDs {
		foundMap[id] = true
	}

	var missing []int
	for _, id := range requestedIDs {
		if !foundMap[id] {
			missing = append(missing, id)
		}
	}
	return missing
}

func (r *adminRepo) UpdateTeacher(ctx context.Context, user *domain.User, instrumentIDs []int) error {
	// Mulai transaction
	tx := r.db.WithContext(ctx).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Check instruments exist
	if len(instrumentIDs) > 0 {
		var instrumentCount int64
		err := tx.Model(&domain.Instrument{}).
			Where("id IN ? AND deleted_at IS NULL", instrumentIDs).
			Count(&instrumentCount).Error

		if err != nil {
			tx.Rollback()
			return fmt.Errorf("error checking instruments: %w", err)
		}

		if instrumentCount != int64(len(instrumentIDs)) {
			tx.Rollback()
			return errors.New("salah satu atau lebih instrumen tidak ditemukan")
		}
	}

	// Update TeacherProfile bio jika ada
	if user.TeacherProfile != nil {
		// Update many-to-many relationship untuk instruments
		if len(instrumentIDs) > 0 {
			// Hapus associations yang lama
			err := tx.Model(&domain.TeacherProfile{UserUUID: user.UUID}).
				Association("Instruments").
				Clear()
			if err != nil {
				tx.Rollback()
				return fmt.Errorf("gagal menghapus instrumen lama: %w", err)
			}

			// Tambahkan associations yang baru
			var instruments []domain.Instrument
			for _, id := range instrumentIDs {
				instruments = append(instruments, domain.Instrument{ID: id})
			}

			err = tx.Model(&domain.TeacherProfile{UserUUID: user.UUID}).
				Association("Instruments").
				Append(instruments)
			if err != nil {
				tx.Rollback()
				return fmt.Errorf("gagal menambahkan instrumen baru: %w", err)
			}
		}
	}

	// Commit transaction jika semua berhasil
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("gagal commit transaction: %w", err)
	}

	return nil
}

func (r *adminRepo) GetAllTeachers(ctx context.Context) ([]domain.User, error) {
	var teachers []domain.User
	if err := r.db.WithContext(ctx).
		Where("role = ?", domain.RoleTeacher).
		Find(&teachers).Error; err != nil {
		return nil, errors.New(utils.TranslateDBError(err))
	}

	return teachers, nil
}

func (r *adminRepo) GetTeacherByUUID(ctx context.Context, uuid string) (*domain.User, error) {
	var teacher domain.User
	if err := r.db.WithContext(ctx).
		Preload("TeacherProfile.Instruments").
		Where("uuid = ? AND role = ?", uuid, domain.RoleTeacher).
		First(&teacher).Error; err != nil {
		return nil, errors.New(utils.TranslateDBError(err))
	}

	return &teacher, nil
}

func (r *adminRepo) DeleteUser(ctx context.Context, uuid string) error {
	tx := r.db.WithContext(ctx).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 1️⃣ Check if user exists
	var user domain.User
	if err := tx.
		Where("uuid = ? AND deleted_at IS NULL", uuid).
		First(&user).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("user tidak ditemukan")
		}
		return fmt.Errorf("gagal mencari user: %w", err)
	}

	if user.Role == domain.RoleStudent || user.Role == domain.RoleAdmin {
		return fmt.Errorf("pengguna tidak dapat di nonaktifkan")
	}

	// check if they had any booking
	var bookingCount int64
	err := tx.Model(&domain.TeacherSchedule{}).
		Where("teacher_uuid = ? AND is_booked = ?", uuid, true).
		Count(&bookingCount).Error
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("gagal menghitung booking: %w", err)
	}
	if bookingCount > 0 {
		tx.Rollback()
		return fmt.Errorf("guru masih memiliki kelas yang terbooking")
	}

	// 4️⃣ Soft delete user
	if err := tx.Model(&domain.User{}).
		Where("uuid = ?", uuid).
		Update("deleted_at", time.Now()).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("gagal menghapus user: %w", err)
	}

	// 5️⃣ Commit transaction
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("gagal commit transaksi: %w", err)
	}

	return nil
}

func (r *adminRepo) GetPackagesByID(ctx context.Context, id int) (*domain.Package, error) {
	var pkg domain.Package
	err := r.db.WithContext(ctx).Preload("Instrument", "deleted_at IS NULL").First(&pkg, id).Error

	if err != nil {
		return nil, err
	}

	return &pkg, nil
}

// Setting Management
func (r *adminRepo) GetSetting(ctx context.Context) (*domain.Setting, error) {
	var setting domain.Setting
	err := r.db.WithContext(ctx).First(&setting).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// If not found, create a default setting and return
			setting = domain.Setting{
				RegistrationFee: 50000,
			}
			errCreate := r.db.WithContext(ctx).Create(&setting).Error
			if errCreate != nil {
				return nil, errors.New(utils.TranslateDBError(errCreate))
			}
			return &setting, nil
		}
		return nil, errors.New(utils.TranslateDBError(err))
	}
	return &setting, nil
}

func (r *adminRepo) UpdateSetting(ctx context.Context, setting *domain.Setting) error {
	var existing domain.Setting
	err := r.db.WithContext(ctx).First(&existing).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Create it if it doesn't exist yet
			setting.ID = 1
			return r.db.WithContext(ctx).Create(setting).Error
		}
		return errors.New(utils.TranslateDBError(err))
	}

	setting.ID = existing.ID // preserve ID
	if err := r.db.WithContext(ctx).Save(setting).Error; err != nil {
		return errors.New(utils.TranslateDBError(err))
	}
	return nil
}
