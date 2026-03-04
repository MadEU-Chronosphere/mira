package service

import (
	"chronosphere/domain"
	"context"
)

type userService struct {
	repo domain.UserRepository
}

func NewUserService(repo domain.UserRepository) domain.UserUseCase {
	return &userService{repo: repo}
}

func (s *userService) GetUserByTelephone(ctx context.Context, telephone string) (*domain.User, error) {
	return s.repo.GetUserByTelephone(ctx, telephone)
}

func (s *userService) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	return s.repo.GetUserByEmail(ctx, email)
}

func (s *userService) CreateUser(ctx context.Context, user *domain.User) error {
	return s.repo.CreateUser(ctx, user)
}

func (s *userService) GetAllUsers(ctx context.Context) ([]domain.User, error) {
	return s.repo.GetAllUsers(ctx)
}

func (s *userService) GetUserByUUID(ctx context.Context, uuid string) (*domain.User, error) {
	return s.repo.GetUserByUUID(ctx, uuid)
}

func (s *userService) UpdateUser(ctx context.Context, user *domain.User) error {
	return s.repo.UpdateUser(ctx, user)
}

func (s *userService) Login(ctx context.Context, email, password string) (*domain.User, error) {
	return s.repo.Login(ctx, email, password)
}
