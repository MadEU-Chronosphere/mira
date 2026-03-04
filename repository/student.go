package repository

import (
	"chronosphere/domain"
	"chronosphere/utils"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

type studentRepository struct {
	db *gorm.DB
}

func NewStudentRepository(db *gorm.DB) domain.StudentRepository {
	return &studentRepository{db: db}
}

// Student's history (your current function fixed):
func (r *studentRepository) GetMyClassHistory(ctx context.Context, studentUUID string) (*[]domain.ClassHistory, error) {
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
		Joins("LEFT JOIN bookings ON class_histories.booking_id = bookings.id").
		Where("bookings.student_uuid = ?", studentUUID). // Filter by student UUID
		Order("bookings.class_date DESC").
		Find(&histories).Error

	if err != nil {
		return nil, fmt.Errorf("failed to fetch class history: %w", err)
	}

	return &histories, nil
}

func (r *studentRepository) CancelBookedClass(
	ctx context.Context,
	bookingID int,
	studentUUID string,
	reason *string,
) (*domain.Booking, error) {

	tx := r.db.WithContext(ctx).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var booking domain.Booking

	// Load booking + schedule
	if err := tx.Preload("Schedule").
		Preload("Schedule.Teacher").
		Preload("Student").
		Preload("PackageUsed").
		Preload("PackageUsed.Package").
		Preload("PackageUsed.Package.Instrument").
		Preload("CancelUser").
		Where("id = ? AND status = ?", bookingID, domain.StatusBooked).
		First(&booking).Error; err != nil {
		tx.Rollback()
		return nil, errors.New("booking tidak ditemukan atau sudah dibatalkan")
	}

	// Ownership check
	if booking.StudentUUID != studentUUID {
		tx.Rollback()
		return nil, errors.New("anda tidak memiliki akses ke booking ini")
	}

	loc, _ := time.LoadLocation("Asia/Makassar")
	now := time.Now().In(loc)

	classDate := booking.ClassDate.In(loc)
	bookedAt := booking.BookedAt.In(loc)

	// Check if class is in the future
	if classDate.Before(now) {
		tx.Rollback()
		return nil, errors.New("tidak bisa membatalkan kelas yang sudah lewat")
	}

	isBookedToday := bookedAt.Year() == now.Year() && bookedAt.YearDay() == now.YearDay()
	isClassToday := classDate.Year() == now.Year() && classDate.YearDay() == now.YearDay()

	// H-1 cancellation rule (24 hours before class)
	minCancelTime := classDate.Add(-24 * time.Hour)
	if now.After(minCancelTime) && !(isBookedToday && isClassToday) {
		tx.Rollback()
		return nil, errors.New("pembatalan hanya bisa dilakukan minimal H-1 (24 jam) sebelum kelas")
	}

	cancelTime := time.Now()

	// 🔁 Update booking status
	if err := tx.Model(&booking).
		UpdateColumns(map[string]interface{}{
			"status":       domain.StatusCancelled,
			"cancelled_at": cancelTime,
			"canceled_by":  studentUUID,
			"notes":        reason,
		}).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("gagal membatalkan booking: %w", err)
	}

	// 🔁 Refund quota to the exact package used in this booking
	if err := tx.Model(&domain.StudentPackage{}).
		Where("id = ?", booking.StudentPackageID).
		Update("remaining_quota", gorm.Expr("remaining_quota + 1")).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("gagal refund quota: %w", err)
	}

	// 🔁 Update schedule availability
	if err := tx.Model(&domain.TeacherSchedule{}).
		Where("id = ?", booking.ScheduleID).
		Update("is_booked", false).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("gagal memperbarui jadwal pengajar: %w", err)
	}

	// 🔁 Update or Insert into ClassHistory
	var history domain.ClassHistory
	err := tx.Where("booking_id = ?", booking.ID).First(&history).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Insert new cancel history
		newHistory := domain.ClassHistory{
			BookingID: booking.ID,
			Status:    domain.StatusCancelled,
			Notes:     reason,
		}
		if err := tx.Create(&newHistory).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("gagal membuat riwayat kelas (cancel): %w", err)
		}
	} else if err == nil {
		// Update existing history
		history.Status = domain.StatusCancelled
		history.Notes = reason
		if err := tx.Save(&history).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("gagal update class history: %w", err)
		}
	} else {
		tx.Rollback()
		return nil, err
	}

	// Commit
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("gagal menyimpan pembatalan: %w", err)
	}

	return &booking, nil
}

func (r *studentRepository) BookClass(
	ctx context.Context, studentUUID string, scheduleID int, instrumentID int) (*domain.Booking, error) {
	tx := r.db.WithContext(ctx).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 1️⃣ Fetch Schedule & Validate
	var schedule domain.TeacherSchedule
	err := tx.Preload("Teacher").
		Preload("TeacherProfile.Instruments").
		Where("id = ? AND deleted_at IS NULL", scheduleID).
		First(&schedule).Error

	if err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("jadwal tidak ditemukan")
		}
		return nil, fmt.Errorf("gagal mengambil jadwal: %w", err)
	}

	if schedule.IsBooked {
		tx.Rollback()
		return nil, errors.New("jadwal sudah dibooking oleh siswa")
	}

	// 2️⃣ Verify Teacher Teaches the Requested Instrument
	teacherTeachesInstrument := false
	var teacherInstrumentNames []string
	for _, inst := range schedule.TeacherProfile.Instruments {
		teacherInstrumentNames = append(teacherInstrumentNames, inst.Name)
		if inst.ID == instrumentID {
			teacherTeachesInstrument = true
		}
	}

	if !teacherTeachesInstrument {
		tx.Rollback()
		return nil, fmt.Errorf(
			"guru ini tidak mengajar instrumen yang dipilih. Guru hanya mengajar: %s",
			strings.Join(teacherInstrumentNames, ", "),
		)
	}

	// 3️⃣ Find Best Student Package (Smart Selection)
	// Criteria: Matches InstrumentID, Matches Schedule Duration, Active, Has Quota
	// Priority: Soonest EndDate
	var studentPackage domain.StudentPackage
	err = tx.Joins("JOIN packages ON packages.id = student_packages.package_id").
		Preload("Package.Instrument").
		Where("student_packages.student_uuid = ?", studentUUID).
		Where("packages.instrument_id = ?", instrumentID).
		Where("packages.duration = ?", schedule.Duration). // Strict duration match
		Where("student_packages.remaining_quota > 0").
		Where("student_packages.end_date >= ?", time.Now()).
		Order("student_packages.end_date ASC"). // Prioritize expiring soonest
		First(&studentPackage).Error

	if err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("tidak ada paket aktif yang sesuai untuk instrumen ini dengan durasi %d menit", schedule.Duration)
		}
		return nil, fmt.Errorf("gagal mencari paket: %w", err)
	}

	// 4️⃣ Room Availability Check
	now := time.Now()
	// Determine room limit based on the PACKAGE's instrument (which matches request)
	// (Note: Package.Instrument was preloaded above)
	isDrum := strings.EqualFold(studentPackage.Package.Instrument.Name, "Drum") ||
		strings.EqualFold(studentPackage.Package.Instrument.Name, "Drums")

	// Calculate class date
	// Parse StartTime string (HH:MM) to time.Time for utility
	startTimeParsed, _ := time.Parse("15:04", schedule.StartTime)
	classDate := utils.GetNextClassDate(schedule.DayOfWeek, startTimeParsed)
	if classDate.Before(now) {
		classDate = classDate.AddDate(0, 0, 7) // Next week
	}

	var bookingCount int64
	query := tx.Model(&domain.Booking{}).
		Joins("JOIN teacher_schedules ts ON ts.id = bookings.schedule_id").
		Joins("JOIN student_packages sp ON sp.id = bookings.student_package_id").
		Joins("JOIN packages p ON p.id = sp.package_id").
		Joins("JOIN instruments i ON i.id = p.instrument_id").
		Where("bookings.status IN ?", []string{domain.StatusBooked, domain.StatusRescheduled}).
		Where("bookings.class_date = ?", classDate).
		Where("ts.start_time = ?", schedule.StartTime)

	if isDrum {
		query = query.Where("i.name ILIKE ?", "Drum%")
	} else {
		query = query.Where("NOT (i.name ILIKE ?)", "Drum%")
	}

	if err := query.Count(&bookingCount).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("gagal memeriksa ketersediaan ruangan: %w", err)
	}

	limit := domain.RegularRoomLimit
	if isDrum {
		limit = domain.DrumRoomLimit
	}

	if bookingCount >= limit {
		tx.Rollback()
		return nil, errors.New("ruangan penuh untuk jam ini")
	}

	// 5️⃣ Check for Existing Booking Conflict (Student side)
	var existingBookingCount int64
	err = tx.Model(&domain.Booking{}).
		Joins("JOIN teacher_schedules ON teacher_schedules.id = bookings.schedule_id").
		Where("bookings.student_uuid = ?", studentUUID).
		Where("bookings.status IN ?", []string{domain.StatusBooked, domain.StatusRescheduled}).
		Where("bookings.class_date = ?", classDate).
		Where("teacher_schedules.start_time = ?", schedule.StartTime).
		Count(&existingBookingCount).Error

	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("gagal memeriksa konflik jadwal: %w", err)
	}

	if existingBookingCount > 0 {
		tx.Rollback()
		return nil, fmt.Errorf(
			"anda sudah memiliki kelas di %s pukul %s. Silakan pilih waktu lain",
			utils.GetDayName(classDate.Weekday()),
			schedule.StartTime,
		)
	}

	// 6️⃣ Execute Booking
	newBooking := domain.Booking{
		StudentUUID:      studentUUID,
		ScheduleID:       schedule.ID,
		StudentPackageID: studentPackage.ID,
		ClassDate:        classDate,
		Status:           domain.StatusBooked,
		BookedAt:         time.Now(),
	}

	if err := tx.Create(&newBooking).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("gagal membuat booking: %w", err)
	}

	// 7️⃣ Update Schedule Status
	if err := tx.Model(&domain.TeacherSchedule{}).
		Where("id = ?", schedule.ID).
		Update("is_booked", true).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("gagal memperbarui status jadwal: %w", err)
	}

	// 8️⃣ Deduct Quota
	if err := tx.Model(&domain.StudentPackage{}).
		Where("id = ?", studentPackage.ID).
		UpdateColumn("remaining_quota", gorm.Expr("remaining_quota - ?", 1)).
		Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("gagal mengurangi kuota paket: %w", err)
	}

	// 9️⃣ Reload Booking with Relations (Crucial for Notifications)
	if err := tx.Preload("Student").
		Preload("Schedule.Teacher").
		Preload("PackageUsed.Package.Instrument").
		First(&newBooking, newBooking.ID).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("gagal memuat data booking: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("gagal menyimpan booking: %w", err)
	}

	return &newBooking, nil
}

func (r *studentRepository) GetMyBookedClasses(ctx context.Context, studentUUID string) (*[]domain.Booking, error) {
	var bookings []domain.Booking

	err := r.db.WithContext(ctx).
		Where("student_uuid = ? AND status IN ?", studentUUID, []string{domain.StatusBooked, domain.StatusRescheduled}).
		Preload("Schedule").
		Preload("PackageUsed").
		Preload("PackageUsed.Package").
		Preload("PackageUsed.Package.Instrument").
		Preload("Schedule.Teacher").
		Preload("Schedule.TeacherProfile.Instruments").
		Order("class_date ASC, booked_at DESC").
		Find(&bookings).Error

	if err != nil {
		return nil, fmt.Errorf("failed to fetch booked classes: %w", err)
	}

	// ✅ Add status indicators
	loc, _ := time.LoadLocation("Asia/Makassar")
	now := time.Now().In(loc)
	for i := range bookings {
		startTimeStr := bookings[i].Schedule.StartTime
		parsedStart, _ := time.Parse("15:04", startTimeStr)

		classStart := time.Date(
			bookings[i].ClassDate.Year(),
			bookings[i].ClassDate.Month(),
			bookings[i].ClassDate.Day(),
			parsedStart.Hour(),
			parsedStart.Minute(),
			0, 0, loc,
		)

		var classEnd time.Time
		is30MinPackage := false

		if bookings[i].PackageUsed.Package != nil {
			is30MinPackage = bookings[i].PackageUsed.Package.Duration == 30
		}

		if is30MinPackage {
			classEnd = classStart.Add(30 * time.Minute)
		} else {
			endTimeStr := bookings[i].Schedule.EndTime
			parsedEnd, _ := time.Parse("15:04", endTimeStr)
			classEnd = time.Date(
				bookings[i].ClassDate.Year(),
				bookings[i].ClassDate.Month(),
				bookings[i].ClassDate.Day(),
				parsedEnd.Hour(),
				parsedEnd.Minute(),
				0, 0, loc,
			)
		}

		switch {
		case now.Before(classStart):
			bookings[i].Status = domain.StatusUpcoming

		// on going case
		case (now.Equal(classStart) || now.After(classStart)) && now.Before(classEnd):
			bookings[i].Status = domain.StatusOngoing
		}

		// finished case
		if now.Equal(classEnd) || now.After(classEnd) {
			bookings[i].IsReadyToFinish = true
		}
	}

	return &bookings, nil
}

func (r *studentRepository) GetStudentInstrumentIDs(ctx context.Context, studentUUID string) ([]int, error) {
	var ids []int
	err := r.db.WithContext(ctx).
		Table("student_packages").
		Select("packages.instrument_id").
		Joins("JOIN packages ON packages.id = student_packages.package_id").
		Where("student_packages.student_uuid = ?", studentUUID).
		Where("student_packages.end_date >= ? AND student_packages.remaining_quota > 0", time.Now()).
		Scan(&ids).Error
	return ids, err
}

func (r *studentRepository) GetTeacherSchedulesBasedOnInstrumentIDs(ctx context.Context, instrumentIDs []int) (*[]domain.TeacherSchedule, error) {
	var schedules []domain.TeacherSchedule
	err := r.db.WithContext(ctx).
		Distinct("teacher_schedules.*").
		Table("teacher_schedules").
		Joins("JOIN teacher_profiles ON teacher_profiles.user_uuid = teacher_schedules.teacher_uuid").
		Joins("JOIN teacher_instruments ON teacher_instruments.teacher_profile_user_uuid = teacher_profiles.user_uuid").
		Joins("JOIN users ON users.uuid = teacher_schedules.teacher_uuid").
		Where("teacher_instruments.instrument_id IN ?", instrumentIDs).
		Where("teacher_schedules.deleted_at IS NULL").
		Where("users.deleted_at IS NULL").
		Preload("Teacher").
		Preload("TeacherProfile.Instruments").
		Order("teacher_schedules.day_of_week ASC, teacher_schedules.start_time ASC").
		Find(&schedules).Error

	if err != nil {
		return nil, fmt.Errorf("gagal mengambil jadwal: %w", err)
	}

	return &schedules, nil
}

func (r *studentRepository) GetAvailableSchedules(ctx context.Context, studentUUID string) (*[]domain.TeacherSchedule, error) {
	// ────────────────────────────────────────────────
	// 1. Get student + active packages + instruments
	// ────────────────────────────────────────────────
	var student domain.User
	err := r.db.WithContext(ctx).
		Preload("StudentProfile.Packages", "end_date >= ? AND remaining_quota > 0", time.Now()).
		Preload("StudentProfile.Packages.Package.Instrument").
		Where("uuid = ? AND role = ? AND deleted_at IS NULL", studentUUID, domain.RoleStudent).
		First(&student).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("student tidak ditemukan")
		}
		return nil, fmt.Errorf("gagal mengambil data student: %w", err)
	}

	if student.StudentProfile == nil || len(student.StudentProfile.Packages) == 0 {
		return &[]domain.TeacherSchedule{}, nil
	}

	// 2. Map valid instruments and durations from student packages
	// Map: InstrumentID -> Map[DurationInMinutes] -> true
	validPackages := make(map[int]map[int]bool)
	var validInstrumentIDs []int

	for _, sp := range student.StudentProfile.Packages {
		if sp.Package == nil {
			continue
		}

		instID := sp.Package.InstrumentID
		duration := sp.Package.Duration // e.g., 30 or 60

		_, exists := validPackages[instID]
		if !exists {
			validPackages[instID] = make(map[int]bool)
			validInstrumentIDs = append(validInstrumentIDs, instID)
		}
		validPackages[instID][duration] = true
	}

	if len(validInstrumentIDs) == 0 {
		return &[]domain.TeacherSchedule{}, nil
	}

	// ────────────────────────────────────────────────
	// 3. Fetch candidate schedules
	// ────────────────────────────────────────────────
	var schedules []domain.TeacherSchedule
	err = r.db.WithContext(ctx).
		Distinct("teacher_schedules.*").
		Table("teacher_schedules").
		Joins("JOIN teacher_profiles ON teacher_profiles.user_uuid = teacher_schedules.teacher_uuid").
		Joins("JOIN teacher_instruments ON teacher_instruments.teacher_profile_user_uuid = teacher_profiles.user_uuid").
		Joins("JOIN users ON users.uuid = teacher_schedules.teacher_uuid").
		Where("teacher_instruments.instrument_id IN ?", validInstrumentIDs).
		Where("teacher_schedules.deleted_at IS NULL").
		Where("users.deleted_at IS NULL").
		Preload("Teacher").
		Preload("TeacherProfile.Instruments").
		Order("teacher_schedules.day_of_week ASC, teacher_schedules.start_time ASC").
		Find(&schedules).Error

	if err != nil {
		return nil, fmt.Errorf("gagal mengambil jadwal: %w", err)
	}

	// ────────────────────────────────────────────────
	// 4. Enrich & Filter Schedules
	// ────────────────────────────────────────────────
	var availableSchedules []domain.TeacherSchedule

	for i, v := range schedules {
		sch := &schedules[i]

		// A. Construct next class date
		startTimeParsed, _ := time.Parse("15:04", sch.StartTime)
		next := utils.GetNextClassDate(sch.DayOfWeek, startTimeParsed)
		sch.NextClassDate = &next

		// B. Check Duration Compatibility
		scheduleDuration := v.Duration

		isCompatible := false

		isDrumTarget := false

		// Check compatibility based on instrument AND duration
		for _, teacherInst := range sch.TeacherProfile.Instruments {
			// Check if student has a package for this instrument
			if durMap, ok := validPackages[teacherInst.ID]; ok {
				// Check if duration matches
				if durMap[scheduleDuration] {
					isCompatible = true
				}

				// Identify instrument type for room check
				if strings.EqualFold(teacherInst.Name, "drum") || strings.EqualFold(teacherInst.Name, "drums") {
					isDrumTarget = true
				}
			}
		}

		// ✅ CHANGED: Do NOT continue/skip if incompatible. Just flag it.
		sch.IsDurationCompatible = ptrBool(isCompatible)

		// C. Check Room Availability
		var bookingCount int64
		query := r.db.WithContext(ctx).Model(&domain.Booking{}).
			Joins("JOIN teacher_schedules ts ON ts.id = bookings.schedule_id").
			Joins("JOIN student_packages sp ON sp.id = bookings.student_package_id").
			Joins("JOIN packages p ON p.id = sp.package_id").
			Joins("JOIN instruments i ON i.id = p.instrument_id").
			Where("bookings.status IN ?", []string{domain.StatusBooked, domain.StatusRescheduled}).
			Where("bookings.class_date = ?", next).
			Where("ts.start_time = ?", sch.StartTime)

		if isDrumTarget {
			query = query.Where("i.name ILIKE ?", "drum%")
		} else {
			query = query.Where("NOT (i.name ILIKE ?)", "drum%")
		}

		if err := query.Count(&bookingCount).Error; err != nil {
			fmt.Printf("Error checking room availability: %v\n", err)
			sch.IsRoomAvailable = ptrBool(false)
		} else {
			limit := domain.RegularRoomLimit
			if isDrumTarget {
				limit = domain.DrumRoomLimit
			}
			sch.IsRoomAvailable = ptrBool(bookingCount < limit)
		}

		// Check for Existing Booking Conflict (Student side)
		var existingBookingCount int64
		err = r.db.WithContext(ctx).Model(&domain.Booking{}).
			Joins("JOIN teacher_schedules ts ON ts.id = bookings.schedule_id").
			Where("bookings.student_uuid = ?", studentUUID).
			Where("bookings.status IN ?", []string{domain.StatusBooked, domain.StatusRescheduled}).
			Where("bookings.class_date = ?", next).
			Where("ts.start_time = ?", sch.StartTime).
			Count(&existingBookingCount).Error

		if err != nil {
			fmt.Printf("Error checking existing booking: %v\n", err)
			sch.IsBookedSameDayAndTime = ptrBool(false)
		} else {
			sch.IsBookedSameDayAndTime = ptrBool(existingBookingCount > 0)
		}

		// D. Fully Available
		// is_booked is a permanent per-slot flag (true once ever booked), not a
		// per-occurrence flag — it must NOT be used here. The per-date conflict
		// check (IsBookedSameDayAndTime) already covers whether this specific
		// upcoming occurrence is taken.
		fully := *sch.IsRoomAvailable && *sch.IsDurationCompatible && !sch.IsBooked && !*sch.IsBookedSameDayAndTime
		sch.IsFullyAvailable = ptrBool(fully)

		availableSchedules = append(availableSchedules, *sch)
	}

	return &availableSchedules, nil
}

func ptrBool(v bool) *bool {
	return &v
}

func (r *studentRepository) GetAllAvailablePackages(ctx context.Context) (*[]domain.Package, error) {
	var packages []domain.Package
	if err := r.db.WithContext(ctx).Preload("Instrument").Where("deleted_at IS NULL").Find(&packages).Error; err != nil {
		return nil, err
	}
	return &packages, nil
}

func (r *studentRepository) GetMyProfile(ctx context.Context, userUUID string) (*domain.User, error) {
	var student domain.User
	err := r.db.WithContext(ctx).
		Preload("StudentProfile.Packages", "end_date >= ? AND remaining_quota > 0", time.Now()).
		Preload("StudentProfile.Packages.Package.Instrument").
		Where("uuid = ? AND role = ? AND deleted_at IS NULL", userUUID, domain.RoleStudent).
		First(&student).Error
	if err != nil {
		return nil, err
	}
	return &student, nil
}

func (r *studentRepository) UpdateStudentData(ctx context.Context, uuid string, payload domain.User) error {
	// Mulai transaction
	tx := r.db.WithContext(ctx).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Cek apakah user exists dan belum dihapus
	var existingUser domain.User
	err := tx.Where("uuid = ? AND role = ? AND deleted_at IS NULL", uuid, domain.RoleStudent).First(&existingUser).Error
	if err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("pengguna tidak ditemukan")
		}
		return fmt.Errorf("error mencari pengguna: %w", err)
	}

	// Check email duplicate dengan user lain
	// var emailCount int64
	// err = tx.Model(&domain.User{}).
	// 	Where("email = ? AND uuid != ?", payload.Email, uuid).
	// 	Count(&emailCount).Error
	// if err != nil {
	// 	tx.Rollback()
	// 	return fmt.Errorf("error checking email: %w", err)
	// }
	// if emailCount > 0 {
	// 	tx.Rollback()
	// 	return errors.New("email sudah digunakan oleh pengguna lain")
	// }

	// Check phone duplicate dengan user lain
	var phoneCount int64
	err = tx.Model(&domain.User{}).
		Where("phone = ? AND uuid != ?", payload.Phone, uuid).
		Count(&phoneCount).Error
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("error checking phone: %w", err)
	}
	if phoneCount > 0 {
		tx.Rollback()
		return errors.New("nomor telepon sudah digunakan oleh pengguna lain")
	}

	// Update user data
	err = tx.Model(&domain.User{}).
		Where("uuid = ?", uuid).
		Updates(payload).Error
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("gagal memperbarui data pengguna: %w", err)
	}

	// Commit transaction jika semua berhasil
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("gagal commit transaction: %w", err)
	}

	return nil
}
