package domain

import (
	"context"
)

// ScheduleAvailabilityResult enriches TeacherSchedule with availability flags
// and teacher performance data for the student's schedule browsing view.
type ScheduleAvailabilityResult struct {
	TeacherSchedule

	// Availability flags (computed per student context)
	IsBookedSameDayAndTime *bool `json:"is_booked_same_day_and_time,omitempty"`
	IsDurationCompatible   *bool `json:"is_duration_compatible,omitempty"`
	IsRoomAvailable        *bool `json:"is_room_available,omitempty"`
	IsFullyAvailable       *bool `json:"is_fully_available,omitempty"`

	// Teacher performance (count of completed ClassHistory entries)
	TeacherFinishedClassCount int `json:"teacher_finished_class_count"`
}

type StudentUseCase interface {
	GetMyProfile(ctx context.Context, userUUID string) (*User, error)
	UpdateStudentData(ctx context.Context, userUUID string, user User) error
	GetAllAvailablePackages(ctx context.Context) (*[]Package, *Setting, error)

	// BookClass: instrumentID is nil for regular packages (derived from the package's instrument).
	// For trial packages instrumentID must be provided — the student picks which instrument to study.
	BookClass(ctx context.Context, studentUUID string, scheduleID int, packageID int, instrumentID *int) error

	GetMyBookedClasses(ctx context.Context, studentUUID string) (*[]Booking, error)
	CancelBookedClass(ctx context.Context, bookingID int, studentUUID string, reason *string) error

	// GetAvailableSchedules returns all teacher schedules enriched with availability flags
	// and teacher performance metrics. Trial packages show ALL schedules regardless of instrument/duration.
	GetAvailableSchedules(ctx context.Context, studentUUID string, instrumentID int) (*[]ScheduleAvailabilityResult, error)

	GetMyClassHistory(ctx context.Context, studentUUID string) (*[]ClassHistory, error)
	GetTeacherDetails(ctx context.Context, teacherUUID string) (*User, error)
}

type StudentRepository interface {
	GetMyProfile(ctx context.Context, userUUID string) (*User, error)
	UpdateStudentData(ctx context.Context, userUUID string, user User) error
	GetAllAvailablePackages(ctx context.Context) (*[]Package, *Setting, error)

	// BookClass with explicit packageID. instrumentID is nil for regular packages (resolved from package),
	// required (non-nil) for trial packages.
	BookClass(ctx context.Context, studentUUID string, scheduleID int, packageID int, instrumentID *int) (*Booking, error)

	GetMyBookedClasses(ctx context.Context, studentUUID string) (*[]Booking, error)
	CancelBookedClass(ctx context.Context, bookingID int, studentUUID string, reason *string) (*Booking, error)

	// GetAvailableSchedules with packageID for trial-aware filtering.
	GetAvailableSchedules(ctx context.Context, studentUUID string, instrumentID int) (*[]ScheduleAvailabilityResult, error)

	GetMyClassHistory(ctx context.Context, studentUUID string) (*[]ClassHistory, error)

	// GetTeacherSchedulesBasedOnInstrumentIDs kept for internal use.
	GetTeacherSchedulesBasedOnInstrumentIDs(ctx context.Context, instrumentIDs []int) (*[]TeacherSchedule, error)

	GetTeacherDetails(ctx context.Context, teacherUUID string) (*User, error)
}