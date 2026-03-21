package repository

import (
	"chronosphere/domain"
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type reportRepo struct {
	db *gorm.DB
}

func NewReportRepository(db *gorm.DB) domain.ReportRepository {
	return &reportRepo{db: db}
}

// ─────────────────────────────────────────────────────────────────────────────
// GetClassHistoriesByStudentUUID
// ─────────────────────────────────────────────────────────────────────────────

func (r *reportRepo) GetClassHistoriesByStudentUUID(
	ctx context.Context,
	studentUUID string,
) (*[]domain.ClassHistory, error) {
	var histories []domain.ClassHistory

	err := r.db.WithContext(ctx).
		Preload("Booking").
		Preload("Booking.Schedule").
		Preload("Booking.Schedule.Teacher").
		Preload("Booking.Schedule.TeacherProfile").
		Preload("Booking.Schedule.TeacherProfile.Instruments").
		Preload("Booking.Student").
		Preload("Booking.PackageUsed").
		Preload("Booking.PackageUsed.Package").
		Preload("Booking.PackageUsed.Package.Instrument").
		Preload("Documentations").
		Joins("LEFT JOIN bookings ON class_histories.booking_id = bookings.id").
		Where("bookings.student_uuid = ?", studentUUID).
		Order("bookings.class_date DESC").
		Find(&histories).Error

	if err != nil {
		return nil, fmt.Errorf("gagal mengambil riwayat kelas siswa: %w", err)
	}

	return &histories, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// GetTeacherTeachingReport
//
// Aggregates completed ClassHistory rows per teacher and breaks them down by
// ISO calendar week (Monday–Sunday). An optional date range filters by
// bookings.class_date. If TeacherUUID is set, only that teacher is returned.
// ─────────────────────────────────────────────────────────────────────────────

func (r *reportRepo) GetTeacherTeachingReport(
	ctx context.Context,
	filter domain.TeacherTeachingReportFilter,
) ([]domain.TeacherTeachingReport, error) {

	// ── 1. Build the date range for the query ─────────────────────────────────
	loc, _ := time.LoadLocation("Asia/Makassar")

	// Default: current calendar month
	now := time.Now().In(loc)
	startDate := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	endDate := startDate.AddDate(0, 1, 0).Add(-time.Nanosecond)

	if filter.StartDate != "" {
		if t, err := time.ParseInLocation("2006-01-02", filter.StartDate, loc); err == nil {
			startDate = t
		}
	}
	if filter.EndDate != "" {
		if t, err := time.ParseInLocation("2006-01-02", filter.EndDate, loc); err == nil {
			// end of that day
			endDate = time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, loc)
		}
	}

	// ── 2. Fetch raw rows (one per completed class) ──────────────────────────
	// We pull enough data to build weekly breakdowns in Go rather than doing
	// complex SQL GROUPING SETS, which keeps the query portable.
	type rawRow struct {
		TeacherUUID  string
		TeacherName  string
		TeacherEmail string
		TeacherPhone string
		Gender       string
		ClassDate    time.Time
	}

	var rows []rawRow

	q := r.db.WithContext(ctx).
		Table("class_histories ch").
		Select(`
			u.uuid         AS teacher_uuid,
			u.name         AS teacher_name,
			u.email        AS teacher_email,
			u.phone        AS teacher_phone,
			u.gender       AS gender,
			b.class_date   AS class_date
		`).
		Joins("JOIN bookings b ON b.id = ch.booking_id").
		Joins("JOIN teacher_schedules ts ON ts.id = b.schedule_id").
		Joins("JOIN users u ON u.uuid = ts.teacher_uuid").
		Where("ch.status = ?", domain.StatusCompleted).
		Where("b.class_date >= ? AND b.class_date <= ?", startDate, endDate)

	if filter.TeacherUUID != "" {
		q = q.Where("u.uuid = ?", filter.TeacherUUID)
	}

	if err := q.Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("gagal mengambil data laporan mengajar: %w", err)
	}

	// ── 3. Aggregate in Go ────────────────────────────────────────────────────
	type teacherKey = string // UUID
	type weekKey = string    // "YYYY-MM-DD" (monday)

	// We need deterministic ordering of weeks, so collect them in a map then sort.
	type teacherData struct {
		name       string
		email      string
		phone      string
		gender     string
		weekCounts map[weekKey]int
		weeks      []weekKey // insertion-ordered (chronological)
	}

	byTeacher := make(map[teacherKey]*teacherData)

	for _, row := range rows {
		td, ok := byTeacher[row.TeacherUUID]
		if !ok {
			td = &teacherData{
				name:       row.TeacherName,
				email:      row.TeacherEmail,
				phone:      row.TeacherPhone,
				gender:     row.Gender,
				weekCounts: make(map[weekKey]int),
			}
			byTeacher[row.TeacherUUID] = td
		}

		monday, _ := domain.WeekBoundary(row.ClassDate, loc)
		wk := monday.Format("2006-01-02")
		if td.weekCounts[wk] == 0 {
			td.weeks = append(td.weeks, wk)
		}
		td.weekCounts[wk]++
	}

	// ── 4. Build response slice ───────────────────────────────────────────────
	// We want a stable output order: sort teachers by name.
	reports := make([]domain.TeacherTeachingReport, 0, len(byTeacher))

	for uuid, td := range byTeacher {
		// Sort weeks chronologically (they're ISO dates so lexicographic = chronological)
		sortedWeeks := sortedStringSlice(td.weeks)

		breakdowns := make([]domain.TeacherWeeklyBreakdown, 0, len(sortedWeeks))
		total := 0
		for _, wk := range sortedWeeks {
			cnt := td.weekCounts[wk]
			total += cnt

			monday, _ := time.ParseInLocation("2006-01-02", wk, loc)
			sunday := monday.AddDate(0, 0, 6)

			breakdowns = append(breakdowns, domain.TeacherWeeklyBreakdown{
				WeekStart:  monday.Format("2006-01-02"),
				WeekEnd:    sunday.Format("2006-01-02"),
				ClassCount: cnt,
			})
		}

		reports = append(reports, domain.TeacherTeachingReport{
			TeacherUUID:     uuid,
			TeacherName:     td.name,
			TeacherEmail:    td.email,
			TeacherPhone:    td.phone,
			Gender:          td.gender,
			TotalClasses:    total,
			WeeklyBreakdown: breakdowns,
			PeriodStart:     startDate.Format("2006-01-02"),
			PeriodEnd:       endDate.Format("2006-01-02"),
		})
	}

	// Sort final slice by teacher name for deterministic output
	sortReportsByName(reports)

	return reports, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Sorting helpers (no extra dependency needed)
// ─────────────────────────────────────────────────────────────────────────────

func sortedStringSlice(s []string) []string {
	out := make([]string, len(s))
	copy(out, s)
	// Simple insertion sort — week count is small (max ~52)
	for i := 1; i < len(out); i++ {
		key := out[i]
		j := i - 1
		for j >= 0 && out[j] > key {
			out[j+1] = out[j]
			j--
		}
		out[j+1] = key
	}
	return out
}

func sortReportsByName(reports []domain.TeacherTeachingReport) {
	for i := 1; i < len(reports); i++ {
		key := reports[i]
		j := i - 1
		for j >= 0 && reports[j].TeacherName > key.TeacherName {
			reports[j+1] = reports[j]
			j--
		}
		reports[j+1] = key
	}
}