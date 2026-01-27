package repository

import (
	"chronosphere/domain"
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
		startTimeStr := schedule.StartTime.Format("15:04:05")
		endTimeStr := schedule.EndTime.Format("15:04:05")

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
			loc, _ := time.LoadLocation("Asia/Makassar")
			startWITA := schedule.StartTime.In(loc).Format("15:04")
			endWITA := schedule.EndTime.In(loc).Format("15:04")

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
		Preload("StudentPackage.Package").
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
	startTime := booking.Schedule.StartTime
	scheduleEndTime := booking.Schedule.EndTime // This is always 1 hour later

	classStart := time.Date(
		booking.ClassDate.Year(), booking.ClassDate.Month(), booking.ClassDate.Day(),
		startTime.Hour(), startTime.Minute(), startTime.Second(), 0,
		booking.ClassDate.Location(),
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
			booking.ClassDate.Year(), booking.ClassDate.Month(), booking.ClassDate.Day(),
			scheduleEndTime.Hour(), scheduleEndTime.Minute(), scheduleEndTime.Second(), 0,
			booking.ClassDate.Location(),
		)
	}

	now := time.Now().In(classEnd.Location())

	// 4️⃣ Check if class can be finished
	canFinish := now.After(classEnd)

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

		var packageType string
		if is30MinPackage {
			packageType = "30 menit"
		} else {
			packageType = "60 menit"
		}

		return fmt.Errorf(
			"Kelas %s belum dapat diselesaikan. Kelas berlangsung pukul %s - %s. Tunggu %s lagi hingga kelas berakhir",
			packageType, startFormatted, endFormatted, timeMsg,
		)
	}

	// 5️⃣ Check if class hasn't started yet (edge case)
	if now.Before(classStart) {
		tx.Rollback()
		startFormatted := classStart.Format("15:04")
		return fmt.Errorf("Kelas belum dimulai. Kelas akan dimulai pukul %s", startFormatted)
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

	// 9️⃣ IMPORTANT: Check if the other half of schedule is available
	// If 30-min package used first half (13:00-13:30), second half (13:30-14:00) should be freed
	if is30MinPackage {
		// Check if there's another booking for the second half
		var secondHalfBooking domain.Booking
		err := tx.Where("schedule_id = ? AND class_date = ? AND status = ?",
			booking.ScheduleID, booking.ClassDate, domain.StatusBooked).
			Where("id != ?", booking.ID).
			First(&secondHalfBooking).Error

		if errors.Is(err, gorm.ErrRecordNotFound) {
			// No booking for second half, keep schedule available
			// Do nothing - schedule already marked as available
		} else if err == nil {
			// There's a booking for second half, DON'T free the schedule
			// Do nothing - keep schedule booked
		} else {
			tx.Rollback()
			return fmt.Errorf("gagal memeriksa jadwal: %w", err)
		}
	} else {
		// 60-min package used entire slot, free the schedule
		if err := tx.Model(&domain.TeacherSchedule{}).
			Where("id = ?", booking.ScheduleID).
			Update("is_booked", false).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("gagal memperbarui jadwal: %w", err)
		}
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

	now := time.Now()
	for i := range bookings {
		// Combine ClassDate with Schedule time components
		startTime := bookings[i].Schedule.StartTime
		classDate := bookings[i].ClassDate

		// Create actual datetime by combining date from ClassDate with time from Schedule
		classStart := time.Date(
			classDate.Year(), classDate.Month(), classDate.Day(),
			startTime.Hour(), startTime.Minute(), startTime.Second(), startTime.Nanosecond(),
			startTime.Location(),
		)

		// Calculate end time
		duration := bookings[i].Schedule.EndTime.Sub(startTime)
		classEnd := classStart.Add(duration)

		// Check if 30-minute package
		is30MinPackage := false
		if bookings[i].PackageUsed.Package != nil {
			is30MinPackage = bookings[i].PackageUsed.Package.Duration == 30
		}

		switch {
		case now.Before(classStart):
			bookings[i].Status = domain.StatusUpcoming
			bookings[i].IsReadyToFinish = false

		case now.After(classStart) && now.Before(classEnd):
			bookings[i].Status = domain.StatusOngoing

			if is30MinPackage {
				halfwayPoint := classStart.Add(duration / 2)
				bookings[i].IsReadyToFinish = now.After(halfwayPoint)
			} else {
				bookings[i].IsReadyToFinish = false
			}

		case now.After(classEnd):
			bookings[i].IsReadyToFinish = true
		}
	}

	return &bookings, nil
}

func (r *teacherRepository) CancelBookedClass(
	ctx context.Context,
	bookingID int,
	teacherUUID string,
	reason *string,
) error {

	tx := r.db.WithContext(ctx).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var booking domain.Booking

	// Load booking + schedule
	if err := tx.Preload("Schedule").
		Where("id = ? AND status = ?", bookingID, domain.StatusBooked).
		First(&booking).Error; err != nil {
		tx.Rollback()
		return errors.New("booking tidak ditemukan atau sudah dibatalkan")
	}

	// Ownership check
	if booking.Schedule.TeacherUUID != teacherUUID {
		tx.Rollback()
		return errors.New("anda tidak memiliki akses ke booking ini")
	}

	// Check if class is in the future
	if booking.ClassDate.Before(time.Now()) {
		tx.Rollback()
		return errors.New("tidak bisa membatalkan kelas yang sudah lewat")
	}

	// H-1 cancellation rule (24 hours before class)
	minCancelTime := booking.ClassDate.Add(-24 * time.Hour)
	if time.Now().After(minCancelTime) {
		tx.Rollback()
		return errors.New("pembatalan hanya bisa dilakukan minimal H-1 (24 jam) sebelum kelas")
	}

	// Default reason
	if reason == nil || *reason == "" {
		defaultReason := "Alasan tidak diberikan"
		reason = &defaultReason
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
		return fmt.Errorf("gagal membatalkan booking: %w", err)
	}

	// 🔁 Refund quota to the exact package used in this booking
	if err := tx.Model(&domain.StudentPackage{}).
		Where("id = ?", booking.StudentPackageID).
		Update("remaining_quota", gorm.Expr("remaining_quota + 1")).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("gagal refund quota: %w", err)
	}

	// 🔁 Update schedule availability
	if err := tx.Model(&domain.TeacherSchedule{}).
		Where("id = ?", booking.ScheduleID).
		Update("is_booked", false).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("gagal memperbarui jadwal pengajar: %w", err)
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
			return fmt.Errorf("gagal membuat riwayat kelas (cancel): %w", err)
		}
	} else if err == nil {
		// Update existing history
		history.Status = domain.StatusCancelled
		history.Notes = reason
		if err := tx.Save(&history).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("gagal update class history: %w", err)
		}
	} else {
		tx.Rollback()
		return err
	}

	// Commit
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("gagal menyimpan pembatalan: %w", err)
	}

	return nil
}
