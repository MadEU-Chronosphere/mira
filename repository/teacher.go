package repository

import (
	"chronosphere/domain"
	"chronosphere/utils"
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type teacherRepository struct {
	db *gorm.DB
}

func NewTeacherRepository(db *gorm.DB) domain.TeacherRepository {
	return &teacherRepository{db: db}
}



func (r *teacherRepository) DeleteAvailabilityBasedOnDay(ctx context.Context, teacherUUID string, dayOfWeek string) error {
	// Check if there are any booked classes on this day
	var bookedCount int64
	err := r.db.WithContext(ctx).
		Model(&domain.Booking{}).
		Joins("JOIN teacher_schedules ON bookings.schedule_id = teacher_schedules.id").
		Where("teacher_schedules.teacher_uuid = ?", teacherUUID).
		Where("teacher_schedules.day_of_week = ?", dayOfWeek).
		Where("teacher_schedules.deleted_at IS NULL").
		Where("bookings.status IN ?", []string{domain.StatusBooked, domain.StatusUpcoming}).
		Count(&bookedCount).Error

	if err != nil {
		return fmt.Errorf("failed to check booked classes: %w", err)
	}

	// If there are booked classes, prevent deletion
	if bookedCount > 0 {
		return fmt.Errorf("tidak dapat menghapus jadwal, terdapat %d kelas yang sudah dipesan atau akan datang pada hari ini", bookedCount)
	}

	// Soft delete the availability
	result := r.db.WithContext(ctx).
		Model(&domain.TeacherSchedule{}).
		Where("teacher_uuid = ? AND day_of_week = ? AND deleted_at IS NULL", teacherUUID, dayOfWeek).
		Update("deleted_at", time.Now())

	if result.Error != nil {
		return fmt.Errorf("gagal menghapus jadwal: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return errors.New("tidak ada jadwal yang ditemukan untuk hari yang ditentukan")
	}

	return nil
}
func (r *teacherRepository) GetMyClassHistory(ctx context.Context, teacherUUID string) (*[]domain.ClassHistory, error) {
	var histories []domain.ClassHistory

	err := r.db.WithContext(ctx).
		Joins("JOIN bookings ON bookings.id = class_histories.booking_id").
		Joins("JOIN teacher_schedules ON teacher_schedules.id = bookings.schedule_id").
		Where("teacher_schedules.teacher_uuid = ?", teacherUUID). // Filter by teacher
		Preload("Booking").
		Preload("Booking.Schedule").
		Preload("Booking.Schedule.Teacher").        // Preload teacher info (optional)
		Preload("Booking.Schedule.TeacherProfile"). // Preload teacher profile (optional)
		Preload("Booking.Student").
		Preload("Booking.PackageUsed").
		Preload("Booking.PackageUsed.Package").
		Preload("Booking.PackageUsed.Package.Instrument").
		Preload("Documentations").
		Order("class_histories.created_at DESC").
		Find(&histories).Error

	if err != nil {
		return nil, fmt.Errorf("failed to fetch teacher class history: %w", err)
	}

	return &histories, nil
}

func (r *teacherRepository) AddAvailability(ctx context.Context, schedules *[]domain.TeacherSchedule) error {
	// Start a transaction
	tx := r.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}

	// Defer rollback in case of error
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	// ✅ Check for overlaps BEFORE inserting within the transaction
	for _, schedule := range *schedules {
		var count int64

		// Parse the times to compare properly
		startTimeStr := schedule.StartTime
		endTimeStr := schedule.EndTime

		err := tx.
			Model(&domain.TeacherSchedule{}).
			Where("teacher_uuid = ? AND day_of_week = ? AND deleted_at IS NULL",
				schedule.TeacherUUID, schedule.DayOfWeek).
			Where(`
            (start_time::time, end_time::time) OVERLAPS (?::time, ?::time)
        `, startTimeStr, endTimeStr).
			Count(&count).Error

		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to check overlap: %w", err)
		}
		if count > 0 {
			// Format times for display in WITA (UTC+8)
			// loc, _ := time.LoadLocation("Asia/Makassar")
			startWITA := schedule.StartTime
			endWITA := schedule.EndTime

			tx.Rollback()
			return fmt.Errorf("slot waktu %s %s-%s konflik dengan jadwal yang sudah ada",
				schedule.DayOfWeek,
				startWITA,
				endWITA)
		}
	}

	// ✅ If no conflicts, insert all schedules within the transaction
	if err := tx.Create(schedules).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to add schedule: %w", err)
	}

	// Commit the transaction
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func (r *teacherRepository) FinishClass(ctx context.Context, bookingID int, teacherUUID string, payload domain.ClassHistory) error {
	tx := r.db.WithContext(ctx).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 1️⃣ Get booking with package info
	var booking domain.Booking
	err := tx.Preload("Schedule").
		Preload("PackageUsed.Package").
		Where("id = ? AND status = ?", bookingID, domain.StatusBooked).
		First(&booking).Error
	if err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("booking tidak ditemukan atau sudah selesai")
		}
		return fmt.Errorf("gagal mengambil booking: %w", err)
	}

	// 2️⃣ Verify teacher ownership
	if booking.Schedule.TeacherUUID != teacherUUID {
		tx.Rollback()
		return errors.New("anda tidak memiliki akses ke booking ini")
	}

	// 3️⃣ Calculate class times based on package duration
	startTimeStr := booking.Schedule.StartTime
	endTimeStr := booking.Schedule.EndTime // This is always 1 hour later (or 30 mins)

	// Parse string HH:MM
	parsedStart, err := time.Parse("15:04", startTimeStr)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("format waktu mulai tidak valid: %v", err)
	}
	parsedEnd, err := time.Parse("15:04", endTimeStr)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("format waktu selesai tidak valid: %v", err)
	}

	loc, _ := time.LoadLocation("Asia/Makassar")

	classDateLoc := booking.ClassDate.In(loc)

	classStart := time.Date(
		classDateLoc.Year(), classDateLoc.Month(), classDateLoc.Day(),
		parsedStart.Hour(), parsedStart.Minute(), parsedStart.Second(), 0,
		loc,
	)

	// Determine actual class end based on package duration
	var classEnd time.Time
	is30MinPackage := false

	if booking.PackageUsed.Package != nil {
		is30MinPackage = booking.PackageUsed.Package.Duration == 30
	}

	if is30MinPackage {
		// 30-min package: class ends 30 minutes after start
		classEnd = classStart.Add(30 * time.Minute)
	} else {
		// 60-min package: class ends at schedule end time
		classEnd = time.Date(
			classDateLoc.Year(), classDateLoc.Month(), classDateLoc.Day(),
			parsedEnd.Hour(), parsedEnd.Minute(), parsedEnd.Second(), 0,
			loc,
		)
	}

	now := time.Now().In(loc)

	// 4️⃣ Check if class can be finished
	if !booking.IsManual {
		canFinish := now.Equal(classEnd) || now.After(classEnd)
		if !canFinish {
			tx.Rollback()
			remaining := classEnd.Sub(now).Round(time.Minute)

			// Format remaining time for human readability
			remainingHours := int(remaining.Hours())
			remainingMinutes := int(remaining.Minutes()) % 60
			remainingSeconds := int(remaining.Seconds()) % 60

			var timeMsg string

			if remainingHours > 0 {
				if remainingMinutes > 0 {
					timeMsg = fmt.Sprintf("%d jam %d menit", remainingHours, remainingMinutes)
				} else {
					timeMsg = fmt.Sprintf("%d jam", remainingHours)
				}
			} else if remainingMinutes > 0 {
				if remainingSeconds > 0 {
					timeMsg = fmt.Sprintf("%d menit %d detik", remainingMinutes, remainingSeconds)
				} else {
					timeMsg = fmt.Sprintf("%d menit", remainingMinutes)
				}
			} else {
				timeMsg = fmt.Sprintf("%d detik", remainingSeconds)
			}

			// Format start and end times for better context
			startFormatted := classStart.Format("15:04")
			endFormatted := classEnd.Format("15:04")
			dayOfWeek := classStart.Format("Monday")
			classDate := classStart.Format("2006-01-02")

			// translate day of week to Indonesian
			dayOfWeek = utils.TranslateDayOfWeek(dayOfWeek)

			return fmt.Errorf(
				"Kelas belum dapat diselesaikan. Kelas berlangsung %s %s pukul %s - %s. Tunggu %s lagi hingga kelas berakhir",
				dayOfWeek, classDate, startFormatted, endFormatted, timeMsg,
			)
		}
	}

	// 5️⃣ Check if class hasn't started yet (edge case)
	if !booking.IsManual && now.Before(classStart) {
		tx.Rollback()
		startFormatted := classStart.Format("15:04")
		return fmt.Errorf("Kelas belum dimulai. Kelas akan dimulai pukul %s", startFormatted)
	}

	defaultNotes := "Kelas selesai, tanpa catatan"

	if payload.Notes == nil || *payload.Notes == "" {
		payload.Notes = &defaultNotes
	}

	// 6️⃣ Create ClassHistory
	classHistory := domain.ClassHistory{
		BookingID: booking.ID,
		Status:    domain.StatusCompleted,
		Notes:     payload.Notes,
	}

	if err := tx.Create(&classHistory).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("gagal membuat riwayat kelas: %w", err)
	}

	// 7️⃣ Save documentations
	for _, doc := range payload.Documentations {
		doc.ClassHistoryID = classHistory.ID
		if err := tx.Create(&doc).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("gagal menyimpan dokumentasi: %w", err)
		}
	}

	// 8️⃣ Update booking status
	completedAt := time.Now()
	if err := tx.Model(&booking).
		UpdateColumns(map[string]interface{}{
			"status":       domain.StatusCompleted,
			"completed_at": completedAt,
		}).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("gagal memperbarui status booking: %w", err)
	}

	if err := tx.Model(&domain.TeacherSchedule{}).
		Where("id = ?", booking.ScheduleID).
		Update("is_booked", false).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("gagal memperbarui jadwal: %w", err)
	}

	// ✅ Commit
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("gagal menyimpan transaksi: %w", err)
	}

	return nil
}

func (r *teacherRepository) GetMyProfile(ctx context.Context, userUUID string) (*domain.User, error) {
	var teacher domain.User
	if err := r.db.WithContext(ctx).Preload("TeacherProfile.Instruments").Where("uuid = ? AND role = ?", userUUID, "teacher").First(&teacher).Error; err != nil {
		return nil, err
	}
	return &teacher, nil
}

func (r *teacherRepository) UpdateTeacherData(ctx context.Context, uuid string, payload domain.User) error {
	// Mulai transaction
	tx := r.db.WithContext(ctx).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Cek apakah user exists dan belum dihapus
	var existingUser domain.User
	err := tx.Where("uuid = ? AND role = ?", uuid, domain.RoleTeacher).First(&existingUser).Error
	if err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("guru tidak ditemukan")
		}
		return fmt.Errorf("error mencari guru: %w", err)
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
	// 	return errors.New("email sudah digunakan oleh user lain")
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
		return errors.New("nomor telepon sudah digunakan oleh user lain")
	}

	// Update user data
	err = tx.Model(&domain.User{}).
		Where("uuid = ?", uuid).
		Updates(payload).Error
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("gagal memperbarui data guru: %w", err)
	}

	// Update TeacherProfile bio jika ada
	if payload.TeacherProfile != nil {
		// Cek apakah teacher profile sudah ada atau perlu dibuat baru
		var profileCount int64
		err = tx.Model(&domain.TeacherProfile{}).
			Where("user_uuid = ?", uuid).
			Count(&profileCount).Error
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("error checking teacher profile: %w", err)
		}

		if profileCount > 0 {
			// Update existing profile
			err = tx.Model(&domain.TeacherProfile{}).
				Where("user_uuid = ?", uuid).
				Update("bio", payload.TeacherProfile.Bio).Error
		} else {
			// Create new profile
			err = tx.Create(&domain.TeacherProfile{
				UserUUID: uuid,
				Bio:      payload.TeacherProfile.Bio,
			}).Error
		}

		if err != nil {
			tx.Rollback()
			return fmt.Errorf("gagal memperbarui profil guru: %w", err)
		}
	}

	// Commit transaction jika semua berhasil
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("gagal commit transaction: %w", err)
	}

	return nil
}

// ✅ Get all schedules owned by a teacher
func (r *teacherRepository) GetMySchedules(ctx context.Context, teacherUUID string) (*[]domain.TeacherSchedule, error) {
	var schedules []domain.TeacherSchedule
	err := r.db.WithContext(ctx).
		Where("teacher_uuid = ? AND deleted_at IS NULL", teacherUUID).
		Order("day_of_week, start_time").
		Find(&schedules).Error
	return &schedules, err
}

func (r *teacherRepository) DeleteAvailability(ctx context.Context, scheduleID int, teacherUUID string) error {
	var schedule domain.TeacherSchedule
	err := r.db.WithContext(ctx).
		Where("id = ? AND teacher_uuid = ? AND deleted_at IS NULL", scheduleID, teacherUUID).
		First(&schedule).Error
	if err != nil {
		return errors.New("jadwal tidak ditemukan")
	}

	// 1️⃣ Check if this schedule has any active bookings
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&domain.Booking{}).
		Where("schedule_id = ? AND status IN ?", scheduleID, []string{"booked", "rescheduled"}).
		Count(&count).Error; err != nil {
		return err
	}

	if count > 0 {
		return errors.New("jadwal ini sudah dipesan, tidak bisa dihapus. harap lakukan pembatalan terlebih dahulu")
	}

	// 2️⃣ Check if the class has already completed (linked to ClassHistory)
	// var completedCount int64
	// if err := r.db.WithContext(ctx).
	// 	Model(&domain.ClassHistory{}).
	// 	Where("booking_id IN (SELECT id FROM bookings WHERE schedule_id = ?)", scheduleID).
	// 	Count(&completedCount).Error; err != nil {
	// 	return err
	// }

	// if completedCount > 0 {
	// 	return errors.New("jadwal ini sudah memiliki riwayat kelas dan tidak dapat dihapus")
	// }

	// 3️⃣ Soft delete (mark as deleted)
	if err := r.db.WithContext(ctx).Model(&domain.TeacherSchedule{}).
		Where("id = ?", scheduleID).
		Update("deleted_at", time.Now()).Error; err != nil {
		return err
	}

	return nil
}

func (r *teacherRepository) GetAllBookedClass(ctx context.Context, teacherUUID string) (*[]domain.Booking, error) {
	var bookings []domain.Booking

	err := r.db.WithContext(ctx).
		Preload("Student").
		Preload("Student.StudentProfile").
		Preload("PackageUsed").
		Preload("PackageUsed.Package").
		Preload("PackageUsed.Package.Instrument").
		Preload("Schedule").
		Preload("Schedule.Teacher").
		Where("schedule_id IN (SELECT id FROM teacher_schedules WHERE teacher_uuid = ? AND deleted_at IS NULL)", teacherUUID).
		Where("status = ?", domain.StatusBooked).
		Find(&bookings).Error

	if err != nil {
		return nil, err
	}

	loc, _ := time.LoadLocation("Asia/Makassar")
	now := time.Now().In(loc)
	for i := range bookings {
		// Combine ClassDate with Schedule time components
		startTimeStr := bookings[i].Schedule.StartTime
		endTimeStr := bookings[i].Schedule.EndTime
		classDate := bookings[i].ClassDate

		// Parse "HH:MM"
		parsedStart, _ := time.Parse("15:04", startTimeStr) // Ignore error as stored data should be valid
		parsedEnd, _ := time.Parse("15:04", endTimeStr)

		// Create actual datetime by combining date from ClassDate with time from Schedule
		// Make sure to use loc timezone for class start & end so comparison is accurate.
		classDateLoc := classDate.In(loc)
		classStart := time.Date(
			classDateLoc.Year(), classDateLoc.Month(), classDateLoc.Day(),
			parsedStart.Hour(), parsedStart.Minute(), 0, 0,
			loc,
		)

		// Calculate end time
		// Using parsed times for duration calculation
		duration := parsedEnd.Sub(parsedStart)
		// Or use struct Duration if trusted. Duration field is in minutes.
		// duration := time.Duration(bookings[i].Schedule.Duration) * time.Minute

		classEnd := classStart.Add(duration)

		// ongoing case
		if now.Equal(classStart) || now.After(classStart) {
			bookings[i].Status = domain.StatusOngoing
		}

		switch {
		case now.Before(classStart):
			bookings[i].Status = domain.StatusUpcoming
			bookings[i].IsReadyToFinish = false

		case (now.Equal(classStart) || now.After(classStart)) && now.Before(classEnd):
			bookings[i].Status = domain.StatusOngoing

		case now.Equal(classEnd) || now.After(classEnd):
			bookings[i].IsReadyToFinish = true
			bookings[i].Status = domain.StatusClassFinished
		}

	}

	// ✅ Populate LatestClassHistories for each student
	for i := range bookings {
		histories := make([]domain.ClassHistory, 0)

		// Get instrument ID from the booked package
		var instrumentID int
		if bookings[i].PackageUsed.Package != nil {
			instrumentID = *bookings[i].PackageUsed.Package.InstrumentID
		}

		// Fetch completed class histories for this student, filtered by instrument
		err := r.db.WithContext(ctx).
			Model(&domain.ClassHistory{}).
			Preload("Booking").
			Preload("Booking.Schedule").
			Preload("Booking.Schedule.Teacher").
			Preload("Booking.PackageUsed").
			Preload("Booking.PackageUsed.Package").
			Preload("Booking.PackageUsed.Package.Instrument").
			Joins("JOIN bookings ON bookings.id = class_histories.booking_id").
			Joins("JOIN student_packages ON student_packages.id = bookings.student_package_id").
			Joins("JOIN packages ON packages.id = student_packages.package_id").
			Where("bookings.student_uuid = ?", bookings[i].StudentUUID).
			Where("packages.instrument_id = ?", instrumentID).
			Where("class_histories.status = ?", domain.StatusCompleted).
			Order("class_histories.created_at DESC").
			Find(&histories).Error

		if err != nil {
			fmt.Printf("⚠️ Failed to fetch histories for student %s: %v\n", bookings[i].StudentUUID, err)
		}

		if bookings[i].Student.StudentProfile == nil {
			bookings[i].Student.StudentProfile = &domain.StudentProfile{
				UserUUID: bookings[i].StudentUUID,
			}
		}

		bookings[i].Student.StudentProfile.LatestClassHistories = &histories
	}

	return &bookings, nil
}

func (r *teacherRepository) CancelBookedClass(
	ctx context.Context,
	bookingID int,
	teacherUUID string,
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
	if booking.Schedule.TeacherUUID != teacherUUID {
		tx.Rollback()
		return nil, errors.New("anda tidak memiliki akses ke booking ini")
	}

	loc, _ := time.LoadLocation("Asia/Makassar")
	now := time.Now().In(loc)
	classDateLoc := booking.ClassDate.In(loc)

	// Check if class is in the future
	if classDateLoc.Before(now) {
		tx.Rollback()
		return nil, errors.New("tidak bisa membatalkan kelas yang sudah lewat")
	}

	// H-1 cancellation rule (24 hours before class)
	minCancelTime := classDateLoc.Add(-24 * time.Hour)
	if now.After(minCancelTime) {
		tx.Rollback()
		return nil, errors.New("pembatalan hanya bisa dilakukan minimal H-1 (24 jam) sebelum kelas")
	}

	cancelTime := time.Now()

	// 🔁 Update booking status
	if err := tx.Model(&booking).
		UpdateColumns(map[string]interface{}{
			"status":       domain.StatusCancelled,
			"cancelled_at": cancelTime,
			"canceled_by":  teacherUUID,
			"notes":        reason,
		}).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("gagal membatalkan booking: %w", err)
	}

	// 🔁 Refund quota to the exact package used in this booking
	// skip if the booking is using a trial package
	if !booking.PackageUsed.Package.IsTrial {
		if err := tx.Model(&domain.StudentPackage{}).
			Where("id = ?", booking.StudentPackageID).
			Update("remaining_quota", gorm.Expr("remaining_quota + 1")).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("gagal refund quota: %w", err)
		}
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

	// ✅ Reload Booking with CancelUser for notification
	// We need to do this AFTER commit or look up using the same transaction before commit.
	// Since we committed, we can use a new query or just reload.
	// To be safe and simple, let's reload it.
	if err := r.db.WithContext(ctx).
		Preload("Schedule").
		Preload("Schedule.Teacher").
		Preload("Student").
		Preload("PackageUsed").
		Preload("PackageUsed.Package").
		Preload("PackageUsed.Package.Instrument").
		Preload("CancelUser").
		First(&booking, booking.ID).Error; err != nil {
		// Log error but don't fail the request since cancellation is done
		fmt.Printf("⚠️ Failed to reload booking details: %v\n", err)
	}

	return &booking, nil
}
