package service

import (
	"chronosphere/domain"
	"context"
	"fmt"
	"log"
	"os"
	"time"

	xendit "github.com/xendit/xendit-go/v6"
	invoice "github.com/xendit/xendit-go/v6/invoice"
	"gorm.io/gorm"
)

type paymentService struct {
	paymentRepo  domain.PaymentRepository
	adminRepo    domain.AdminRepository
	xenditClient *xendit.APIClient
	db           *gorm.DB
}

func NewPaymentService(paymentRepo domain.PaymentRepository, adminRepo domain.AdminRepository, db *gorm.DB) domain.PaymentUseCase {
	apiKey := os.Getenv("XENDIT_SECRET_KEY")
	if apiKey == "" {
		log.Println("⚠️  XENDIT_SECRET_KEY not set, payment features will not work")
	}

	client := xendit.NewClient(apiKey)

	return &paymentService{
		paymentRepo:  paymentRepo,
		adminRepo:    adminRepo,
		xenditClient: client,
		db:           db,
	}
}

func (s *paymentService) CreateInvoice(ctx context.Context, studentUUID string, packageID int) (*domain.Payment, error) {
	// 1. Validate student exists
	student, err := s.adminRepo.GetStudentByUUID(ctx, studentUUID)
	if err != nil {
		return nil, fmt.Errorf("siswa tidak ditemukan: %w", err)
	}

	// 2. Validate package exists
	pkg, err := s.adminRepo.GetPackagesByID(ctx, packageID)
	if err != nil {
		return nil, fmt.Errorf("paket tidak ditemukan: %w", err)
	}

	// 3. Calculate total amount (package + fixed fees)
	totalAmount := domain.RegistrationFee + domain.SPPFee + pkg.Price

	// 4. Generate external ID
	shortUUID := studentUUID
	if len(shortUUID) > 8 {
		shortUUID = shortUUID[:8]
	}
	externalID := fmt.Sprintf("MADEU-%s-%d", shortUUID, time.Now().UnixMilli())

	// 5. Build description
	description := fmt.Sprintf("Pembayaran Paket %s - %s", pkg.Name, student.Name)

	// 6. Get redirect URLs
	siteURL := os.Getenv("NEXT_PUBLIC_SITE_URL")
	if siteURL == "" {
		siteURL = "http://localhost:3000"
	}
	successURL := fmt.Sprintf("%s/dashboard/panel/student/payment/success", siteURL)
	failureURL := fmt.Sprintf("%s/dashboard/panel/student/payment/failed", siteURL)

	// 7. Build invoice items for Xendit
	items := []invoice.InvoiceItem{
		*invoice.NewInvoiceItem("Biaya Pendaftaran", float32(domain.RegistrationFee), 1),
		*invoice.NewInvoiceItem("Biaya SPP Bulanan", float32(domain.SPPFee), 1),
		*invoice.NewInvoiceItem(fmt.Sprintf("Paket %s (%dx pertemuan)", pkg.Name, pkg.Quota), float32(pkg.Price), 1),
	}

	// 8. Build customer
	customer := invoice.CustomerObject{}
	customer.GivenNames = *invoice.NewNullableString(&student.Name)
	customer.Email = *invoice.NewNullableString(&student.Email)
	if student.Phone != "" {
		customer.MobileNumber = *invoice.NewNullableString(&student.Phone)
	}

	// 9. Create Xendit invoice request
	currency := "IDR"
	locale := "id"
	shouldSendEmail := true
	invoiceDuration := "86400" // 24 hours in seconds

	createReq := *invoice.NewCreateInvoiceRequest(externalID, totalAmount)
	createReq.Description = &description
	createReq.PayerEmail = &student.Email
	createReq.Currency = &currency
	createReq.Locale = &locale
	createReq.ShouldSendEmail = &shouldSendEmail
	createReq.InvoiceDuration = &invoiceDuration
	createReq.SuccessRedirectUrl = &successURL
	createReq.FailureRedirectUrl = &failureURL
	createReq.Items = items
	createReq.Customer = &customer
	createReq.Metadata = map[string]interface{}{
		"student_uuid": studentUUID,
		"package_id":   packageID,
	}

	// 10. Call Xendit API
	inv, _, xenditErr := s.xenditClient.InvoiceApi.CreateInvoice(ctx).
		CreateInvoiceRequest(createReq).
		Execute()

	if xenditErr != nil {
		log.Printf("❌ Xendit CreateInvoice error: %v", xenditErr)
		return nil, fmt.Errorf("gagal membuat invoice pembayaran: %v", xenditErr)
	}

	// 11. Save payment record (only after Xendit succeeds)
	invoiceID := ""
	if inv.Id != nil {
		invoiceID = *inv.Id
	}

	payment := &domain.Payment{
		StudentUUID:     studentUUID,
		PackageID:       packageID,
		XenditInvoiceID: invoiceID,
		ExternalID:      externalID,
		Amount:          totalAmount,
		Status:          domain.PaymentStatusPending,
		InvoiceURL:      inv.InvoiceUrl,
	}

	if err := s.paymentRepo.CreatePayment(ctx, payment); err != nil {
		log.Printf("❌ Failed to save payment record: %v", err)
		return nil, fmt.Errorf("gagal menyimpan data pembayaran: %w", err)
	}

	log.Printf("✅ Invoice created: %s | Amount: %.0f | Student: %s", externalID, totalAmount, student.Name)

	return payment, nil
}

func (s *paymentService) HandleWebhook(ctx context.Context, payload domain.XenditWebhookPayload) error {
	// 1. Find payment by external_id
	payment, err := s.paymentRepo.GetPaymentByExternalID(ctx, payload.ExternalID)
	if err != nil {
		log.Printf("⚠️  Webhook: payment not found for external_id: %s", payload.ExternalID)
		return fmt.Errorf("payment tidak ditemukan: %w", err)
	}

	// 2. Skip if already processed (idempotent)
	if payment.Status == domain.PaymentStatusPaid {
		log.Printf("ℹ️  Webhook: payment %s already processed (status: PAID)", payload.ExternalID)
		return nil
	}

	// 3. Process based on status
	switch payload.Status {
	case "PAID", "SETTLED":
		// Use DB transaction for atomic update + package assignment
		txErr := s.db.Transaction(func(tx *gorm.DB) error {
			// Parse paid_at time
			var paidAt *time.Time
			if payload.PaidAt != "" {
				t, parseErr := time.Parse(time.RFC3339, payload.PaidAt)
				if parseErr == nil {
					paidAt = &t
				}
			}
			if paidAt == nil {
				now := time.Now()
				paidAt = &now
			}

			method := &payload.PaymentMethod
			if *method == "" {
				method = nil
			}

			// Update payment status
			if err := s.paymentRepo.UpdatePaymentStatus(ctx, payload.ExternalID, domain.PaymentStatusPaid, method, paidAt); err != nil {
				log.Printf("❌ Webhook: failed to update payment status: %v", err)
				return err
			}

			// Auto-assign package to student
			if err := s.autoAssignPackage(ctx, payment.StudentUUID, payment.PackageID); err != nil {
				log.Printf("⚠️  Webhook: auto-assign failed, admin can assign manually: %v", err)
				// Don't return error — payment is already marked as paid
				// Admin can manually assign later if auto-assign fails
			}

			return nil
		})

		if txErr != nil {
			log.Printf("❌ Webhook: transaction failed: %v", txErr)
			return txErr
		}

		log.Printf("✅ Payment completed: %s | Student: %s | Package: %d", payload.ExternalID, payment.StudentUUID, payment.PackageID)

	case "EXPIRED":
		if err := s.paymentRepo.UpdatePaymentStatus(ctx, payload.ExternalID, domain.PaymentStatusExpired, nil, nil); err != nil {
			log.Printf("❌ Webhook: failed to update expired status: %v", err)
			return err
		}
		log.Printf("⏰ Payment expired: %s", payload.ExternalID)

	default:
		log.Printf("ℹ️  Webhook: unhandled status %s for %s", payload.Status, payload.ExternalID)
	}

	return nil
}

func (s *paymentService) autoAssignPackage(ctx context.Context, studentUUID string, packageID int) error {
	_, _, err := s.adminRepo.AssignPackageToStudent(ctx, studentUUID, packageID)
	if err != nil {
		return fmt.Errorf("gagal mengaktifkan paket: %w", err)
	}
	log.Printf("✅ Auto-assigned package %d to student %s", packageID, studentUUID)
	return nil
}

func (s *paymentService) GetPaymentsByStudent(ctx context.Context, studentUUID string) ([]domain.Payment, error) {
	return s.paymentRepo.GetPaymentsByStudent(ctx, studentUUID)
}

// ========================================================================
// Admin Reporting Methods
// ========================================================================

func (s *paymentService) GetTotalProfit(ctx context.Context, filter domain.ProfitFilter) (float64, error) {
	return s.paymentRepo.GetTotalProfit(ctx, filter)
}

func (s *paymentService) GetPaymentHistory(ctx context.Context, filter domain.HistoryFilter) ([]domain.Payment, int64, error) {
	return s.paymentRepo.GetPaymentHistory(ctx, filter)
}

func (s *paymentService) GetPackageSummary(ctx context.Context) ([]domain.PackageSummary, error) {
	return s.paymentRepo.GetPackageSummary(ctx)
}
