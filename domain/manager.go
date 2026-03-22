package domain

import (
	"context"
	"time"
)

type ManagerUseCase interface {
	GetAllStudents(ctx context.Context) ([]User, error)
	GetStudentByUUID(ctx context.Context, uuid string) (*User, error)
	ModifyStudentPackageQuota(ctx context.Context, studentUUID string, packageID int, incomingQuota int) error
	UpdateManager(ctx context.Context, manager *User) error
	UpdateStudent(ctx context.Context, student *User) error
	GetCancelledClassHistories(ctx context.Context) (*[]ClassHistory, error)
	RebookWithSubstitute(ctx context.Context, req RebookInput) (*Booking, error)
	GetAllTeachers(ctx context.Context) ([]User, error)
	GetTeacherSchedules(ctx context.Context, teacherUUID string) ([]TeacherSchedule, error)

	GetSetting(ctx context.Context) (*Setting, error)
	UpdateSetting(ctx context.Context, setting *Setting) error
}

type ManagerRepository interface {
	GetAllStudents(ctx context.Context) ([]User, error)
	GetStudentByUUID(ctx context.Context, uuid string) (*User, error)
	ModifyStudentPackageQuota(ctx context.Context, studentUUID string, packageID int, incomingQuota int) (*User, error)
	UpdateManager(ctx context.Context, manager *User) error
	UpdateStudent(ctx context.Context, student *User) error
	GetCancelledClassHistories(ctx context.Context) (*[]ClassHistory, error)
	RebookWithSubstitute(ctx context.Context, req RebookInput) (*Booking, error)
	GetAllTeachers(ctx context.Context) ([]User, error)
	GetTeacherSchedules(ctx context.Context, teacherUUID string) ([]TeacherSchedule, error)

	GetSetting(ctx context.Context) (*Setting, error)
	UpdateSetting(ctx context.Context, setting *Setting) error
}

type RebookInput struct {
	OriginalBookingID int
	SubScheduleID     int
	ClassDate         time.Time // manager picks the actual date it happened
}
