package domain

import (
	"context"
	"time"
)

const (
	TeacherPaymentStatusUnpaid = "unpaid"
	TeacherPaymentStatusPaid   = "paid"
)

// TeacherPayment records the monthly salary calculation for a teacher.
// One record per teacher per period — idempotent on (teacher_uuid, period_start).
type TeacherPayment struct {
	ID           int       `gorm:"primaryKey" json:"id"`
	TeacherUUID  string    `gorm:"type:uuid;not null;index" json:"teacher_uuid"`
	Teacher      User      `gorm:"foreignKey:TeacherUUID;references:UUID" json:"teacher,omitempty"`
	PeriodStart  time.Time `gorm:"not null" json:"period_start"`  // first day of the month
	PeriodEnd    time.Time `gorm:"not null" json:"period_end"`    // last day of the month
	ClassCount   int       `gorm:"not null;default:0" json:"class_count"`
	TotalEarning float64   `gorm:"not null;default:0" json:"total_earning"`
	Status       string    `gorm:"size:20;default:'unpaid'" json:"status"` // unpaid | paid
	ProofImageURL *string  `gorm:"type:text" json:"proof_image_url,omitempty"`
	PaidAt       *time.Time `json:"paid_at,omitempty"`
	PaidByUUID   *string   `gorm:"type:uuid" json:"paid_by_uuid,omitempty"`
	PaidBy       *User     `gorm:"foreignKey:PaidByUUID;references:UUID" json:"paid_by,omitempty"`
	Notes        *string   `gorm:"type:text" json:"notes,omitempty"`
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TeacherPaymentDetail is a breakdown row used in the calculation response.
// Not persisted — returned as part of the generate response so admin can verify before confirming.
type TeacherPaymentDetail struct {
	TeacherUUID   string  `json:"teacher_uuid"`
	TeacherName   string  `json:"teacher_name"`
	TeacherPhone  string  `json:"teacher_phone"`
	ClassCount    int     `json:"class_count"`
	TotalEarning  float64 `json:"total_earning"`
	PeriodStart   string  `json:"period_start"`
	PeriodEnd     string  `json:"period_end"`
}

// MarkPaidRequest is the payload for marking a teacher payment as paid.
type MarkPaidRequest struct {
	ProofImageURL string  `json:"proof_image_url" binding:"required,url"`
	Notes         *string `json:"notes" binding:"omitempty,max=500"`
}

type TeacherPaymentUseCase interface {
	// GenerateMonthlyPayments calculates earnings for all teachers for the given
	// year/month and inserts TeacherPayment records (status: unpaid).
	// Idempotent — skips teachers who already have a record for the same period.
	GenerateMonthlyPayments(ctx context.Context, year int, month int) ([]TeacherPaymentDetail, error)

	// GetAllPayments returns all teacher payment records, optionally filtered by status.
	GetAllPayments(ctx context.Context, status string) ([]TeacherPayment, error)

	// GetPaymentsByTeacher returns payment history for a specific teacher.
	GetPaymentsByTeacher(ctx context.Context, teacherUUID string) ([]TeacherPayment, error)

	// MarkAsPaid marks a payment as paid and stores the proof image + admin UUID.
	MarkAsPaid(ctx context.Context, paymentID int, adminUUID string, req MarkPaidRequest) error
}

type TeacherPaymentRepository interface {
	// GenerateMonthlyPayments calculates and inserts payment records.
	GenerateMonthlyPayments(ctx context.Context, year int, month int, commissionRate float64) ([]TeacherPaymentDetail, error)

	// GetAllPayments returns payment records filtered by status ("" = all).
	GetAllPayments(ctx context.Context, status string) ([]TeacherPayment, error)

	// GetPaymentsByTeacher returns payment records for one teacher.
	GetPaymentsByTeacher(ctx context.Context, teacherUUID string) ([]TeacherPayment, error)

	// MarkAsPaid updates status, proof image, paid_at, and paid_by.
	MarkAsPaid(ctx context.Context, paymentID int, adminUUID string, req MarkPaidRequest) error
}