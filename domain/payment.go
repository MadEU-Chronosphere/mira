package domain

import (
	"context"
	"time"
)

// Payment status constants
const (
	PaymentStatusPending = "PENDING"
	PaymentStatusPaid    = "PAID"
	PaymentStatusExpired = "EXPIRED"
	PaymentStatusFailed  = "FAILED"
)

// Fixed fees
const (
	RegistrationFee float64 = 50000
	SPPFee          float64 = 200000
)

type Payment struct {
	ID              int        `gorm:"primaryKey" json:"id"`
	StudentUUID     string     `gorm:"type:uuid;not null" json:"student_uuid"`
	PackageID       int        `gorm:"not null" json:"package_id"`
	XenditInvoiceID string     `gorm:"unique;not null" json:"xendit_invoice_id"`
	ExternalID      string     `gorm:"unique;not null" json:"external_id"`
	Amount          float64    `gorm:"not null" json:"amount"`
	Status          string     `gorm:"size:20;default:'PENDING'" json:"status"`
	PaymentMethod   *string    `json:"payment_method,omitempty"`
	PaidAt          *time.Time `json:"paid_at,omitempty"`
	InvoiceURL      string     `gorm:"type:text" json:"invoice_url"`
	CreatedAt       time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"autoUpdateTime" json:"updated_at"`

	Student User    `gorm:"foreignKey:StudentUUID;references:UUID" json:"student,omitempty"`
	Package Package `gorm:"foreignKey:PackageID" json:"package,omitempty"`
}

// XenditWebhookPayload represents the webhook callback from Xendit
type XenditWebhookPayload struct {
	ID                     string  `json:"id"`
	ExternalID             string  `json:"external_id"`
	UserID                 string  `json:"user_id"`
	Status                 string  `json:"status"`
	MerchantName           string  `json:"merchant_name"`
	Amount                 float64 `json:"amount"`
	PayerEmail             string  `json:"payer_email,omitempty"`
	Description            string  `json:"description,omitempty"`
	PaymentMethod          string  `json:"payment_method,omitempty"`
	PaymentChannel         string  `json:"payment_channel,omitempty"`
	PaidAt                 string  `json:"paid_at,omitempty"`
	Currency               string  `json:"currency,omitempty"`
	PaymentDestination     string  `json:"payment_destination,omitempty"`
	SuccessRedirectURL     string  `json:"success_redirect_url,omitempty"`
	FailureRedirectURL     string  `json:"failure_redirect_url,omitempty"`
	CreditCardChargeID     string  `json:"credit_card_charge_id,omitempty"`
	AdjustedReceivedAmount float64 `json:"adjusted_received_amount,omitempty"`
	BankCode               string  `json:"bank_code,omitempty"`
	EwalletType            string  `json:"ewallet_type,omitempty"`
	OnDemandLink           string  `json:"on_demand_link,omitempty"`
	RecurringPaymentID     string  `json:"recurring_payment_id,omitempty"`
}

// ========================================================================
// Admin Reporting Types (from Chrono)
// ========================================================================

type ProfitFilter struct {
	StartDate string `form:"start_date"` // YYYY-MM-DD
	EndDate   string `form:"end_date"`   // YYYY-MM-DD
}

type HistoryFilter struct {
	Page      int    `form:"page,default=1"`
	Limit     int    `form:"limit,default=10"`
	StartDate string `form:"start_date"`
	EndDate   string `form:"end_date"`
	Status    string `form:"status"` // PENDING, PAID, EXPIRED, FAILED
}

type PackageSummary struct {
	PackageName  string  `json:"package_name"`
	TotalSold    int     `json:"total_sold"`
	TotalRevenue float64 `json:"total_revenue"`
}

// ========================================================================
// Interfaces
// ========================================================================

type PaymentUseCase interface {
	// Student
	CreateInvoice(ctx context.Context, studentUUID string, packageID int) (*Payment, error)
	HandleWebhook(ctx context.Context, payload XenditWebhookPayload) error
	GetPaymentsByStudent(ctx context.Context, studentUUID string) ([]Payment, error)

	// Admin Reporting
	GetTotalProfit(ctx context.Context, filter ProfitFilter) (float64, error)
	GetPaymentHistory(ctx context.Context, filter HistoryFilter) ([]Payment, int64, error)
	GetPackageSummary(ctx context.Context) ([]PackageSummary, error)
}

type PaymentRepository interface {
	CreatePayment(ctx context.Context, payment *Payment) error
	GetPaymentByExternalID(ctx context.Context, externalID string) (*Payment, error)
	UpdatePaymentStatus(ctx context.Context, externalID string, status string, method *string, paidAt *time.Time) error
	GetPaymentsByStudent(ctx context.Context, studentUUID string) ([]Payment, error)

	// Admin Reporting
	GetTotalProfit(ctx context.Context, filter ProfitFilter) (float64, error)
	GetPaymentHistory(ctx context.Context, filter HistoryFilter) ([]Payment, int64, error)
	GetPackageSummary(ctx context.Context) ([]PackageSummary, error)
}
