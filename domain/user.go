package domain

import (
	"context"
)

// USE CASE

type UserUseCase interface {
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetUserByTelephone(ctx context.Context, telephone string) (*User, error)
	CreateUser(ctx context.Context, user *User) error
	GetAllUsers(ctx context.Context) ([]User, error)
	GetUserByUUID(ctx context.Context, uuid string) (*User, error)
	UpdateUser(ctx context.Context, user *User) error
	Login(ctx context.Context, email, password string) (*User, error)
}

// REPOSITORY
type UserRepository interface {
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetUserByTelephone(ctx context.Context, telephone string) (*User, error)
	CreateUser(ctx context.Context, user *User) error
	GetAllUsers(ctx context.Context) ([]User, error)
	GetUserByUUID(ctx context.Context, uuid string) (*User, error)
	UpdateUser(ctx context.Context, user *User) error
	Login(ctx context.Context, email, password string) (*User, error)
}
