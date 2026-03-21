package service

import (
	"chronosphere/domain"
	"context"
	"errors"
	"fmt"
)

type teacherPaymentService struct {
	repo        domain.TeacherPaymentRepository
	adminRepo   domain.AdminRepository
}

func NewTeacherPaymentService(
	repo domain.TeacherPaymentRepository,
	adminRepo domain.AdminRepository,
) domain.TeacherPaymentUseCase {
	return &teacherPaymentService{
		repo:      repo,
		adminRepo: adminRepo,
	}
}

// GenerateMonthlyPayments fetches the current commission rate from settings,
// then delegates calculation and insertion to the repository.
func (s *teacherPaymentService) GenerateMonthlyPayments(
	ctx context.Context,
	year int,
	month int,
) ([]domain.TeacherPaymentDetail, error) {
	if year < 2000 || year > 2100 {
		return nil, errors.New("tahun tidak valid")
	}
	if month < 1 || month > 12 {
		return nil, errors.New("bulan tidak valid (1-12)")
	}

	// Fetch commission rate from settings — single source of truth
	setting, err := s.adminRepo.GetSetting(ctx)
	if err != nil {
		return nil, fmt.Errorf("gagal mengambil pengaturan komisi: %w", err)
	}

	details, err := s.repo.GenerateMonthlyPayments(ctx, year, month, setting.TeacherCommission)
	if err != nil {
		return nil, err
	}

	return details, nil
}

func (s *teacherPaymentService) GetAllPayments(ctx context.Context, status string) ([]domain.TeacherPayment, error) {
	// Validate status if provided
	if status != "" &&
		status != domain.TeacherPaymentStatusUnpaid &&
		status != domain.TeacherPaymentStatusPaid {
		return nil, fmt.Errorf(
			"status tidak valid, gunakan '%s' atau '%s'",
			domain.TeacherPaymentStatusUnpaid,
			domain.TeacherPaymentStatusPaid,
		)
	}
	return s.repo.GetAllPayments(ctx, status)
}

func (s *teacherPaymentService) GetPaymentsByTeacher(ctx context.Context, teacherUUID string) ([]domain.TeacherPayment, error) {
	if teacherUUID == "" {
		return nil, errors.New("teacher UUID tidak boleh kosong")
	}
	return s.repo.GetPaymentsByTeacher(ctx, teacherUUID)
}

func (s *teacherPaymentService) MarkAsPaid(
	ctx context.Context,
	paymentID int,
	adminUUID string,
	req domain.MarkPaidRequest,
) error {
	if paymentID <= 0 {
		return errors.New("payment ID tidak valid")
	}
	if adminUUID == "" {
		return errors.New("admin UUID tidak boleh kosong")
	}
	return s.repo.MarkAsPaid(ctx, paymentID, adminUUID, req)
}