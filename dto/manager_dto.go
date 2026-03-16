package dto

import (
	"chronosphere/domain"
	"strings"
)

type RebookRequest struct {
	OriginalBookingID int    `json:"original_booking_id" binding:"required,gt=0"`
	SubScheduleID     int    `json:"sub_schedule_id" binding:"required,gt=0"`
	ClassDate         string `json:"class_date" binding:"required"` // "YYYY-MM-DD"
}

type ManagerUpdateStudentRequest struct {
	Name     string  `json:"name" binding:"omitempty,min=3,max=50"`
	Gender   string  `json:"gender" binding:"omitempty,oneof=male female"`
	Email    string  `json:"email" binding:"omitempty,email"`
	Phone    string  `json:"phone" binding:"omitempty,numeric,min=9,max=14"`
	Password string  `json:"password" binding:"omitempty,min=8,max=16"`
	Image    *string `json:"image" binding:"omitempty,url"`
}

func MapUpdateStudentRequest(req *ManagerUpdateStudentRequest) *domain.User {
	user := &domain.User{}

	if req.Name != "" {
		user.Name = req.Name
	}
	if req.Gender != "" {
		user.Gender = req.Gender
	}
	if req.Email != "" {
		user.Email = req.Email
	}
	if req.Phone != "" {
		user.Phone = req.Phone
	}
	if req.Image != nil {
		user.Image = req.Image
	}

	return user
}

// Request untuk Create Teacher
type CreateManagerRequest struct {
	Name     string  `json:"name" binding:"required,min=3,max=50"`
	Email    string  `json:"email" binding:"required,email"`
	Phone    string  `json:"phone" binding:"required,numeric,min=9,max=14"`
	Password string  `json:"password" binding:"required,min=8"`
	Gender   string  `json:"gender" binding:"required,oneof=male female"`
	Image    *string `json:"image" binding:"omitempty,url"`
	Bio      *string `json:"bio" binding:"omitempty,max=500"`
}

// Request untuk Update Teacher
type UpdateManagerProfileRequest struct {
	Name   string  `json:"name" binding:"required,min=3,max=50"`
	Email  string  `json:"email" binding:"required,email"`
	Phone  string  `json:"phone" binding:"required,numeric,min=9,max=14"`
	Image  *string `json:"image" binding:"omitempty,url"`
	Gender string  `json:"gender" binding:"required,oneof=male female"`
}

type UpdateManagerProfileRequestByManager struct {
	Name   string  `json:"name" binding:"required,min=3,max=50"`
	Email  string  `json:"email" binding:"required,email"`
	Phone  string  `json:"phone" binding:"required,numeric,min=9,max=14"`
	Image  *string `json:"image" binding:"omitempty,url"`
	Gender string  `json:"gender" binding:"required,oneof=male female"`
}

func MapCreateManagerRequestToUserByManager(req *UpdateManagerProfileRequestByManager) domain.User {
	return domain.User{
		Name:   req.Name,
		Email:  strings.ToLower(req.Email),
		Phone:  req.Phone,
		Image:  req.Image,
		Gender: req.Gender,
	}
}

// Mapper: Convert DTO → Domain
func MapCreateManagerRequestToUser(req *CreateManagerRequest) *domain.User {
	return &domain.User{
		Name:     req.Name,
		Email:    strings.ToLower(req.Email),
		Phone:    req.Phone,
		Password: req.Password,
		Role:     domain.RoleManagement,
		Image:    req.Image,
		Gender:   req.Gender,
	}
}

func MapUpdateManagerRequestToUser(req *UpdateManagerProfileRequest) *domain.User {
	return &domain.User{
		Name:   req.Name,
		Email:  req.Email,
		Phone:  req.Phone,
		Image:  req.Image,
		Gender: req.Gender,
	}
}

type UpdateManagerRequest struct {
	UUID   string `json:"uuid" binding:"required,uuid"`
	Name   string `json:"name" binding:"required,min=3,max=50"`
	Gender string `json:"gender" binding:"required,oneof=male female"`
	Phone  string `json:"phone" binding:"required,numeric,min=9,max=14"`
	Image  string `json:"image" binding:"omitempty,url"`
}

func MakeUpdateManagerRequest(req *UpdateManagerRequest) *domain.User {
	return &domain.User{
		UUID:   req.UUID,
		Name:   req.Name,
		Phone:  req.Phone,
		Image:  &req.Image,
		Gender: req.Gender,
	}
}
