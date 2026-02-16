package repository

import (
	"chronosphere/domain"
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type paymentRepo struct {
	db *gorm.DB
}

func NewPaymentRepository(db *gorm.DB) domain.PaymentRepository {
	return &paymentRepo{db: db}
}

func (r *paymentRepo) CreatePayment(ctx context.Context, payment *domain.Payment) error {
	return r.db.WithContext(ctx).Create(payment).Error
}

func (r *paymentRepo) GetPaymentByExternalID(ctx context.Context, externalID string) (*domain.Payment, error) {
	var payment domain.Payment
	err := r.db.WithContext(ctx).
		Preload("Student").
		Preload("Package").
		Preload("Package.Instrument").
		Where("external_id = ?", externalID).
		First(&payment).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("payment tidak ditemukan")
		}
		return nil, err
	}
	return &payment, nil
}

func (r *paymentRepo) UpdatePaymentStatus(ctx context.Context, externalID string, status string, method *string, paidAt *time.Time) error {
	updates := map[string]interface{}{
		"status": status,
	}
	if method != nil {
		updates["payment_method"] = *method
	}
	if paidAt != nil {
		updates["paid_at"] = *paidAt
	}

	result := r.db.WithContext(ctx).
		Model(&domain.Payment{}).
		Where("external_id = ?", externalID).
		Updates(updates)

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("payment dengan external_id %s tidak ditemukan", externalID)
	}
	return nil
}

func (r *paymentRepo) GetPaymentsByStudent(ctx context.Context, studentUUID string) ([]domain.Payment, error) {
	var payments []domain.Payment
	err := r.db.WithContext(ctx).
		Preload("Package").
		Preload("Package.Instrument").
		Where("student_uuid = ?", studentUUID).
		Order("created_at DESC").
		Find(&payments).Error
	if err != nil {
		return nil, err
	}
	return payments, nil
}

// ========================================================================
// Admin Reporting Methods
// ========================================================================

// GetTotalProfit calculates total revenue from paid payments
func (r *paymentRepo) GetTotalProfit(ctx context.Context, filter domain.ProfitFilter) (float64, error) {
	var total float64
	query := r.db.WithContext(ctx).Model(&domain.Payment{}).
		Where("status = ?", domain.PaymentStatusPaid)

	if filter.StartDate != "" {
		query = query.Where("DATE(paid_at) >= ?", filter.StartDate)
	}
	if filter.EndDate != "" {
		query = query.Where("DATE(paid_at) <= ?", filter.EndDate)
	}

	err := query.Select("COALESCE(SUM(amount), 0)").Scan(&total).Error
	if err != nil {
		return 0, fmt.Errorf("failed to calculate profit: %w", err)
	}
	return total, nil
}

// GetPaymentHistory retrieves payment history with pagination and filters
func (r *paymentRepo) GetPaymentHistory(ctx context.Context, filter domain.HistoryFilter) ([]domain.Payment, int64, error) {
	var payments []domain.Payment
	var total int64

	query := r.db.WithContext(ctx).Model(&domain.Payment{}).
		Preload("Student").
		Preload("Package").
		Preload("Package.Instrument")

	if filter.StartDate != "" {
		query = query.Where("DATE(created_at) >= ?", filter.StartDate)
	}
	if filter.EndDate != "" {
		query = query.Where("DATE(created_at) <= ?", filter.EndDate)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count payments: %w", err)
	}

	offset := (filter.Page - 1) * filter.Limit
	err := query.Order("created_at DESC").
		Limit(filter.Limit).
		Offset(offset).
		Find(&payments).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to fetch payment history: %w", err)
	}

	return payments, total, nil
}

// GetPackageSummary retrieves summary of sales per package
func (r *paymentRepo) GetPackageSummary(ctx context.Context) ([]domain.PackageSummary, error) {
	var summaries []domain.PackageSummary

	err := r.db.WithContext(ctx).Model(&domain.Payment{}).
		Select("packages.name as package_name, COUNT(payments.id) as total_sold, SUM(payments.amount) as total_revenue").
		Joins("JOIN packages ON packages.id = payments.package_id").
		Where("payments.status = ?", domain.PaymentStatusPaid).
		Group("packages.id, packages.name").
		Scan(&summaries).Error

	if err != nil {
		return nil, fmt.Errorf("failed to fetch package summary: %w", err)
	}

	return summaries, nil
}
