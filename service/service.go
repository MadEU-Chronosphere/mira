package service

import (
	"chronosphere/domain"
	"context"
	"errors"
	"fmt"
)

type reportService struct {
	repo domain.ReportRepository
}

func NewReportService(repo domain.ReportRepository) domain.ReportUseCase {
	return &reportService{repo: repo}
}

// ─────────────────────────────────────────────────────────────────────────────
// GetClassHistoriesByStudentUUID
// ─────────────────────────────────────────────────────────────────────────────

func (s *reportService) GetClassHistoriesByStudentUUID(
	ctx context.Context,
	studentUUID string,
) (*[]domain.ClassHistory, error) {
	if studentUUID == "" {
		return nil, errors.New("UUID siswa tidak boleh kosong")
	}
	return s.repo.GetClassHistoriesByStudentUUID(ctx, studentUUID)
}

// ─────────────────────────────────────────────────────────────────────────────
// GetTeacherTeachingReport  (admin / manager — can query any teacher)
// ─────────────────────────────────────────────────────────────────────────────

func (s *reportService) GetTeacherTeachingReport(
	ctx context.Context,
	filter domain.TeacherTeachingReportFilter,
) ([]domain.TeacherTeachingReport, error) {
	if filter.StartDate != "" && filter.EndDate != "" && filter.StartDate > filter.EndDate {
		return nil, errors.New("start_date tidak boleh lebih besar dari end_date")
	}
	return s.repo.GetTeacherTeachingReport(ctx, filter)
}

// ─────────────────────────────────────────────────────────────────────────────
// GetMyTeachingReport  (teacher — scoped to themselves)
// ─────────────────────────────────────────────────────────────────────────────

func (s *reportService) GetMyTeachingReport(
	ctx context.Context,
	teacherUUID string,
	startDate, endDate string,
) (*domain.TeacherTeachingReport, error) {
	if teacherUUID == "" {
		return nil, errors.New("UUID guru tidak boleh kosong")
	}

	filter := domain.TeacherTeachingReportFilter{
		TeacherUUID: teacherUUID,
		StartDate:   startDate,
		EndDate:     endDate,
	}

	reports, err := s.repo.GetTeacherTeachingReport(ctx, filter)
	if err != nil {
		return nil, err
	}

	if len(reports) == 0 {
		// Return empty report with zero counts — not an error
		return &domain.TeacherTeachingReport{
			TeacherUUID:     teacherUUID,
			TotalClasses:    0,
			WeeklyBreakdown: []domain.TeacherWeeklyBreakdown{},
			PeriodStart:     filter.StartDate,
			PeriodEnd:       filter.EndDate,
		}, nil
	}

	if len(reports) > 1 {
		// Should never happen when filtering by UUID, but guard defensively
		return nil, fmt.Errorf("data tidak konsisten: lebih dari satu laporan ditemukan untuk guru ini")
	}

	return &reports[0], nil
}