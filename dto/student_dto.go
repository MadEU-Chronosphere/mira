package dto

import "chronosphere/domain"

type UpdateStudentDataRequest struct {
	Name string `json:"name" binding:"required,min=3,max=50"`
	// Email string  `json:"email" binding:"required,email"`
	Phone  string  `json:"phone" binding:"required,numeric,min=9,max=14"`
	Image  *string `json:"image" binding:"omitempty,url"`
	Gender string  `json:"gender" binding:"required,oneof=male female"`
}

func MapUpdateStudentRequestByStudent(req *UpdateStudentDataRequest) domain.User {
	return domain.User{
		Name: req.Name,
		// Email: req.Email,
		Phone:  req.Phone,
		Image:  req.Image,
		Gender: req.Gender,
	}
}

type BookClassRequest struct {
	ScheduleID   int `json:"schedule_id" binding:"required,min=1"`
	InstrumentID int `json:"instrument_id" binding:"required,min=1"`
}

type CancelBookingRequest struct {
	Reason *string `json:"reason" binding:"omitempty,max=255"`
}
