package domain

import (
	"context"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// Teacher Teaching Report
// ─────────────────────────────────────────────────────────────────────────────

// TeacherTeachingReportFilter holds optional date range and teacher UUID filter.
type TeacherTeachingReportFilter struct {
	TeacherUUID string // optional — if empty, returns all teachers
	StartDate   string // YYYY-MM-DD, optional
	EndDate     string // YYYY-MM-DD, optional
}

// TeacherWeeklyBreakdown holds the count of completed classes for a specific week.
type TeacherWeeklyBreakdown struct {
	WeekStart  string `json:"week_start"`  // Monday of the week, format YYYY-MM-DD
	WeekEnd    string `json:"week_end"`    // Sunday of the week
	ClassCount int    `json:"class_count"`
}

// TeacherTeachingReport is the per-teacher summary returned by the report endpoint.
type TeacherTeachingReport struct {
	TeacherUUID       string                   `json:"teacher_uuid"`
	TeacherName       string                   `json:"teacher_name"`
	TeacherEmail      string                   `json:"teacher_email"`
	TeacherPhone      string                   `json:"teacher_phone"`
	Gender            string                   `json:"gender"`
	TotalClasses      int                      `json:"total_classes"`
	WeeklyBreakdown   []TeacherWeeklyBreakdown `json:"weekly_breakdown"`
	PeriodStart       string                   `json:"period_start"`
	PeriodEnd         string                   `json:"period_end"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Repository & UseCase interfaces
// ─────────────────────────────────────────────────────────────────────────────

// ReportRepository handles all reporting queries.
type ReportRepository interface {
	// GetClassHistoriesByStudentUUID fetches all class histories for a student.
	GetClassHistoriesByStudentUUID(ctx context.Context, studentUUID string) (*[]ClassHistory, error)

	// GetTeacherTeachingReport aggregates completed classes per teacher within the filter range.
	GetTeacherTeachingReport(ctx context.Context, filter TeacherTeachingReportFilter) ([]TeacherTeachingReport, error)
}

// ReportUseCase is consumed by the delivery layer.
type ReportUseCase interface {
	// GetClassHistoriesByStudentUUID is accessible to admin and manager.
	GetClassHistoriesByStudentUUID(ctx context.Context, studentUUID string) (*[]ClassHistory, error)

	// GetTeacherTeachingReport generates the teaching report for one or all teachers.
	// Accessible to admin and managers, and to the teacher themselves (repo filters by UUID).
	GetTeacherTeachingReport(ctx context.Context, filter TeacherTeachingReportFilter) ([]TeacherTeachingReport, error)

	// GetMyTeachingReport is the teacher-self variant — enforces UUID to the caller.
	GetMyTeachingReport(ctx context.Context, teacherUUID string, startDate, endDate string) (*TeacherTeachingReport, error)
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

// WeekBoundary returns the Monday (start) and Sunday (end) of the week
// that contains the given time, in the provided timezone.
func WeekBoundary(t time.Time, loc *time.Location) (monday, sunday time.Time) {
	t = t.In(loc)
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7 // Sunday → 7 so Monday is day 1
	}
	monday = time.Date(t.Year(), t.Month(), t.Day()-wd+1, 0, 0, 0, 0, loc)
	sunday = monday.AddDate(0, 0, 6)
	return
}