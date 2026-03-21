package repository

import (
	"chronosphere/domain"
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type teacherPaymentRepo struct {
	db *gorm.DB
}

func NewTeacherPaymentRepository(db *gorm.DB) domain.TeacherPaymentRepository {
	return &teacherPaymentRepo{db: db}
}

// ─────────────────────────────────────────────────────────────────────────────
// GenerateMonthlyPayments
//
// Calculates earnings for every teacher who completed at least one class in
// the given period. Skips teachers who already have a payment record for the
// same period (idempotent — safe to call multiple times).
//
// Earning per class = StudentPackage.PricePaid × commissionRate
// ─────────────────────────────────────────────────────────────────────────────

func (r *teacherPaymentRepo) GenerateMonthlyPayments(
	ctx context.Context,
	year int,
	month int,
	commissionRate float64,
) ([]domain.TeacherPaymentDetail, error) {

	// Period boundaries (full calendar month, UTC)
	periodStart := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Nanosecond) // last nanosecond of the month

	// ── 1. Aggregate completed classes per teacher for the period ─────────────
	// Each row: teacher_uuid, class_count, total_price_paid
	type aggRow struct {
		TeacherUUID    string
		ClassCount     int
		TotalPricePaid float64
	}

	var rows []aggRow
	err := r.db.WithContext(ctx).
		Table("class_histories ch").
		Select(`
			ts.teacher_uuid                         AS teacher_uuid,
			COUNT(ch.id)                            AS class_count,
			SUM(sp.price_paid)                      AS total_price_paid
		`).
		Joins("JOIN bookings b ON b.id = ch.booking_id").
		Joins("JOIN teacher_schedules ts ON ts.id = b.schedule_id").
		Joins("JOIN student_packages sp ON sp.id = b.student_package_id").
		Where("ch.status = ?", domain.StatusCompleted).
		Where("ch.created_at >= ? AND ch.created_at <= ?", periodStart, periodEnd).
		Group("ts.teacher_uuid").
		Scan(&rows).Error

	if err != nil {
		return nil, fmt.Errorf("gagal menghitung kelas: %w", err)
	}

	if len(rows) == 0 {
		return []domain.TeacherPaymentDetail{}, nil
	}

	// ── 2. Load teacher details for response ──────────────────────────────────
	teacherUUIDs := make([]string, len(rows))
	for i, row := range rows {
		teacherUUIDs[i] = row.TeacherUUID
	}

	var teachers []domain.User
	if err := r.db.WithContext(ctx).
		Where("uuid IN ? AND role = ?", teacherUUIDs, domain.RoleTeacher).
		Find(&teachers).Error; err != nil {
		return nil, fmt.Errorf("gagal memuat data guru: %w", err)
	}

	teacherMap := make(map[string]domain.User, len(teachers))
	for _, t := range teachers {
		teacherMap[t.UUID] = t
	}

	// ── 3. Fetch existing payment records for this period (idempotency check) ─
	var existing []domain.TeacherPayment
	if err := r.db.WithContext(ctx).
		Where("period_start = ? AND period_end = ?", periodStart, periodEnd).
		Find(&existing).Error; err != nil {
		return nil, fmt.Errorf("gagal memeriksa data pembayaran existing: %w", err)
	}

	existingMap := make(map[string]bool, len(existing))
	for _, e := range existing {
		existingMap[e.TeacherUUID] = true
	}

	// ── 4. Insert new records + build response ────────────────────────────────
	var details []domain.TeacherPaymentDetail

	for _, row := range rows {
		earning := row.TotalPricePaid * commissionRate
		teacher := teacherMap[row.TeacherUUID]

		details = append(details, domain.TeacherPaymentDetail{
			TeacherUUID:  row.TeacherUUID,
			TeacherName:  teacher.Name,
			TeacherPhone: teacher.Phone,
			ClassCount:   row.ClassCount,
			TotalEarning: earning,
			PeriodStart:  periodStart.Format("2006-01-02"),
			PeriodEnd:    periodEnd.Format("2006-01-02"),
		})

		// Skip if already generated for this period
		if existingMap[row.TeacherUUID] {
			continue
		}

		record := domain.TeacherPayment{
			TeacherUUID:  row.TeacherUUID,
			PeriodStart:  periodStart,
			PeriodEnd:    periodEnd,
			ClassCount:   row.ClassCount,
			TotalEarning: earning,
			Status:       domain.TeacherPaymentStatusUnpaid,
		}

		if err := r.db.WithContext(ctx).Create(&record).Error; err != nil {
			return nil, fmt.Errorf("gagal menyimpan data pembayaran untuk guru %s: %w", row.TeacherUUID, err)
		}
	}

	return details, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// GetAllPayments
// ─────────────────────────────────────────────────────────────────────────────

func (r *teacherPaymentRepo) GetAllPayments(ctx context.Context, status string) ([]domain.TeacherPayment, error) {
	var payments []domain.TeacherPayment

	q := r.db.WithContext(ctx).
		Preload("Teacher").
		Preload("PaidBy").
		Order("period_start DESC, teacher_uuid ASC")

	if status != "" {
		q = q.Where("status = ?", status)
	}

	if err := q.Find(&payments).Error; err != nil {
		return nil, fmt.Errorf("gagal mengambil data pembayaran: %w", err)
	}

	return payments, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// GetPaymentsByTeacher
// ─────────────────────────────────────────────────────────────────────────────

func (r *teacherPaymentRepo) GetPaymentsByTeacher(ctx context.Context, teacherUUID string) ([]domain.TeacherPayment, error) {
	var payments []domain.TeacherPayment

	if err := r.db.WithContext(ctx).
		Preload("Teacher").
		Preload("PaidBy").
		Where("teacher_uuid = ?", teacherUUID).
		Order("period_start DESC").
		Find(&payments).Error; err != nil {
		return nil, fmt.Errorf("gagal mengambil riwayat pembayaran guru: %w", err)
	}

	return payments, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// MarkAsPaid
// ─────────────────────────────────────────────────────────────────────────────

func (r *teacherPaymentRepo) MarkAsPaid(
	ctx context.Context,
	paymentID int,
	adminUUID string,
	req domain.MarkPaidRequest,
) error {
	var payment domain.TeacherPayment
	if err := r.db.WithContext(ctx).First(&payment, paymentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("data pembayaran tidak ditemukan")
		}
		return fmt.Errorf("gagal mencari data pembayaran: %w", err)
	}

	if payment.Status == domain.TeacherPaymentStatusPaid {
		return errors.New("pembayaran ini sudah ditandai sebagai lunas")
	}

	now := time.Now()
	updates := map[string]interface{}{
		"status":          domain.TeacherPaymentStatusPaid,
		"proof_image_url": req.ProofImageURL,
		"paid_at":         now,
		"paid_by_uuid":    adminUUID,
		"notes":           req.Notes,
	}

	if err := r.db.WithContext(ctx).
		Model(&domain.TeacherPayment{}).
		Where("id = ?", paymentID).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("gagal memperbarui status pembayaran: %w", err)
	}

	return nil
}