package repository

import (
	"chronosphere/domain"
	"context"
	"os"

	"gorm.io/gorm"
)

type userRepository struct {
	db *gorm.DB
}

func NewAuthRepository(db *gorm.DB) domain.UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) GetUserByTelephone(ctx context.Context, telephone string) (*domain.User, error) {
	var user domain.User
	if err := r.db.WithContext(ctx).First(&user, "phone = ?", telephone).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *userRepository) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	var user domain.User
	if err := r.db.WithContext(ctx).Where("email = ? AND deleted_at IS NULL", email).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *userRepository) CreateUser(ctx context.Context, user *domain.User) error {
	defImage := os.Getenv("DEFAULT_PROFILE_IMAGE")

	// FIX: Check if Image is nil first, then check if it's empty
	if user.Image == nil || *user.Image == "" {
		user.Image = &defImage
	}

	return r.db.WithContext(ctx).Create(user).Error
}
func (r *userRepository) GetAllUsers(ctx context.Context) ([]domain.User, error) {
	var users []domain.User
	if err := r.db.WithContext(ctx).
		Preload("TeacherProfile.Instruments").
		Preload("StudentProfile.Packages.Package").
		Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

func (r *userRepository) GetUserByUUID(ctx context.Context, uuid string) (*domain.User, error) {
	var user domain.User
	if err := r.db.WithContext(ctx).
		Preload("TeacherProfile.Instruments").
		Preload("StudentProfile.Packages.Package").
		First(&user, "uuid = ?", uuid).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *userRepository) UpdateUser(ctx context.Context, user *domain.User) error {
	return r.db.WithContext(ctx).Save(user).Error
}

func (r *userRepository) Login(ctx context.Context, email, password string) (*domain.User, error) {
	var user domain.User
	// Explicitly exclude soft-deleted accounts so deactivated users cannot log in.
	if err := r.db.WithContext(ctx).
		Where("email = ? AND password = ? AND deleted_at IS NULL", email, password).
		First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}
