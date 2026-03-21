package dto

import "chronosphere/domain"

type UpdateStudentDataRequest struct {
	Name   string  `json:"name" binding:"required,min=3,max=50"`
	Phone  string  `json:"phone" binding:"required,numeric,min=9,max=14"`
	Image  *string `json:"image" binding:"omitempty,url"`
	Gender string  `json:"gender" binding:"required,oneof=male female"`
}

func MapUpdateStudentRequestByStudent(req *UpdateStudentDataRequest) domain.User {
	return domain.User{
		Name:   req.Name,
		Phone:  req.Phone,
		Image:  req.Image,
		Gender: req.Gender,
	}
}

// BookClassRequest:
//   - schedule_id:    always required
//   - package_id:    always required (student_packages.id)
//   - instrument_id: optional for regular packages (derived from package),
//     required for trial packages (student picks which instrument to study)
type BookClassRequest struct {
	ScheduleID   int  `json:"schedule_id" binding:"required,min=1"`
	PackageID    int  `json:"package_id" binding:"required,min=1"`
	InstrumentID *int `json:"instrument_id" binding:"omitempty,min=1"`
}

type CancelBookingRequest struct {
	Reason *string `json:"reason" binding:"omitempty,max=255"`
}