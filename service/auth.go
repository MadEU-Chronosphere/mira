package service

import (
	"chronosphere/domain"
	"chronosphere/utils"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrEmailExists     = errors.New("email already exists")
	ErrTelephoneExists = errors.New("telephone already exists")
)

type authService struct {
	userRepo     domain.UserRepository
	otpRepo      domain.OTPRepository
	accessToken  *utils.JWTManager
	refreshToken *utils.JWTManager
}

func NewAuthService(userRepo domain.UserRepository, otpRepo domain.OTPRepository, secret string) domain.AuthUseCase {
	return &authService{
		userRepo: userRepo,
		otpRepo:  otpRepo,
		// accessToken:  utils.NewJWTManager(secret, time.Hour),
		accessToken:  utils.NewJWTManager(secret, 24*time.Hour),
		refreshToken: utils.NewJWTManager(secret, 7*24*time.Hour),
	}
}

func (s *authService) ChangeEmail(ctx context.Context, userUUID, newEmail, password string) error {
	user, err := s.userRepo.GetUserByUUID(ctx, userUUID)
	if err != nil {
		return err
	}

	// Compare pass
	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password))
	if err != nil {
		return errors.New("invalid password")
	}

	// Cek apakah email baru sudah ada
	_, err = s.userRepo.GetUserByEmail(ctx, newEmail)
	if err == nil {
		return errors.New("email already in use")
	}

	// Update email
	user.Email = newEmail
	return s.userRepo.UpdateUser(ctx, user)
}

func (s *authService) GetRefreshTokenManager() *utils.JWTManager {
	return s.refreshToken
}

func (s *authService) Me(ctx context.Context, userUUID string) (*domain.User, error) {
	user, err := s.userRepo.GetUserByUUID(ctx, userUUID)
	if err != nil {
		return nil, err
	}
	// Hapus password sebelum mengembalikan data user
	user.Password = ""
	return user, nil
}

func (s *authService) ResendOTP(ctx context.Context, email string) error {
	// cek apakah OTP lama ada
	data, err := s.otpRepo.GetOTP(ctx, email)
	if err != nil {
		return fmt.Errorf("failed to get OTP: %w", err)
	}
	if data == nil {
		return errors.New("no pending OTP found, please register or request password reset first")
	}

	// generate OTP baru
	newOTP, err := utils.GenerateOTP(6)
	if err != nil {
		return err
	}

	// simpan ulang dengan TTL baru
	err = s.otpRepo.SaveOTP(
		ctx,
		email,
		newOTP,
		data["password"],
		data["name"],
		data["phone"],
		data["gender"],
		5*time.Minute,
	)
	if err != nil {
		return fmt.Errorf("failed to save new OTP: %w", err)
	}

	// kirim OTP via email
	if err := utils.SendEmail(email, "Your OTP Code", "Your new OTP is: "+newOTP); err != nil {
		return fmt.Errorf("failed to send OTP email: %w", err)
	}

	return nil
}

func (s *authService) Login(ctx context.Context, email, password string) (*domain.AuthTokens, error) {
	user, err := s.userRepo.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, errors.New("email atau password salah")
	}

	if user.DeletedAt != nil {
		return nil, errors.New("akun anda telah dinonaktifkan, silakan hubungi admin untuk informasi lebih lanjut")
	}
	// Compare password
	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password))
	if err != nil {
		return nil, errors.New("email atau password salah")
	}

	// Generate tokens dengan UUID + Role
	accessToken, err := s.accessToken.GenerateToken(user.UUID, user.Role, user.Name)
	if err != nil {
		return nil, err
	}

	refreshToken, err := s.refreshToken.GenerateToken(user.UUID, user.Role, user.Name)
	if err != nil {
		return nil, err
	}

	return &domain.AuthTokens{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

func (s *authService) Register(ctx context.Context, email string, name string, telephone string, password string, gender string) error {
	if _, err := s.userRepo.GetUserByEmail(ctx, email); err == nil {
		return ErrEmailExists
	}
	if _, err := s.userRepo.GetUserByTelephone(ctx, telephone); err == nil {
		return ErrTelephoneExists
	}

	// Hash password
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}
	hashedPassword := string(hashedBytes)

	// Generate OTP
	otp, err := utils.GenerateOTP(6)
	if err != nil {
		return fmt.Errorf("failed to generate OTP: %w", err)
	}

	theMinute := os.Getenv("OTP_TIME")
	otpTime, err := strconv.Atoi(theMinute)
	if err != nil || otpTime <= 0 {
		otpTime = 5
	}

	// Save to Redis
	if err := s.otpRepo.SaveOTP(ctx, email, otp, hashedPassword, name, telephone, gender, time.Duration(otpTime)*time.Minute); err != nil {
		return fmt.Errorf("failed to save OTP: %w", err)
	}

	// Kirim email OTP
	subject := "Your MadEU OTP Code"
	body := fmt.Sprintf("Your OTP code is: %s (valid for %d minutes)", otp, otpTime)

	if err := utils.SendEmail(email, subject, body); err != nil {
		return fmt.Errorf("failed to send OTP email: %w", err)
	}

	return nil
}

func (s *authService) VerifyOTP(ctx context.Context, email, otp string) error {
	data, valid, err := s.otpRepo.VerifyOTP(ctx, email, otp)
	if err != nil {
		return fmt.Errorf("failed to verify OTP: %w", err)
	}
	if !valid {
		return errors.New("invalid or expired OTP")
	}

	// SELALU gunakan hash dari Redis (tidak perlu test dengan password hardcoded)
	// Hash sudah diverifikasi saat registrasi
	user := &domain.User{
		Name:     data["name"],
		Email:    email,
		Phone:    data["phone"],
		Password: data["password"], // Gunakan hash dari Redis
		Role:     "student",
		Gender:   data["gender"],
	}

	if err := s.userRepo.CreateUser(ctx, user); err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	storedUser, err := s.userRepo.GetUserByEmail(ctx, email)
	if err != nil {
		log.Printf("❌ VERIFY OTP: Failed to fetch user after creation: %v", err)
	} else {
		log.Printf("✅ VERIFY OTP: User created successfully - UUID: %s", storedUser.UUID)
	}

	// Clean up OTP
	if err := s.otpRepo.DeleteOTP(ctx, email); err != nil {
		log.Printf("WARNING: Failed to delete OTP: %v", err)
	}

	return nil
}

func (s *authService) ForgotPassword(ctx context.Context, email string) error {
	user, err := s.userRepo.GetUserByEmail(ctx, email)
	if err != nil {
		return errors.New("email not found")
	}

	otp, err := utils.GenerateOTP(6)
	if err != nil {
		return err
	}

	if err := s.otpRepo.SaveOTP(ctx, email, otp, "", "", "", "", 5*time.Minute); err != nil {
		return err
	}

	// Kirim email OTP
	subject := "MadEU Reset Password OTP"
	body := fmt.Sprintf("Halo %s,\n\nKode OTP untuk reset password akun Anda adalah: %s\nKode ini hanya berlaku selama 5 menit.\n\nJika Anda tidak merasa melakukan permintaan ini, abaikan email ini.",
		user.Name, otp)
	if err := utils.SendEmail(email, subject, body); err != nil {
		return fmt.Errorf("failed to send OTP email: %w", err)
	}

	return nil
}

func (s *authService) ResetPassword(ctx context.Context, email, otp, newPassword string) error {
	_, valid, err := s.otpRepo.VerifyOTP(ctx, email, otp)
	if err != nil {
		return err
	}
	if !valid {
		return errors.New("invalid or expired OTP")
	}

	// Hash new password
	hashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user, err := s.userRepo.GetUserByEmail(ctx, email)
	if err != nil {
		return err
	}
	user.Password = string(hashed)

	if err := s.userRepo.UpdateUser(ctx, user); err != nil {
		return err
	}

	_ = s.otpRepo.DeleteOTP(ctx, email)
	return nil
}

func (s *authService) ChangePassword(ctx context.Context, userUUID, oldPassword, newPassword string) error {
	user, err := s.userRepo.GetUserByUUID(ctx, userUUID)
	if err != nil {
		return err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(oldPassword)); err != nil {
		return errors.New("old password mismatch")
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user.Password = string(hashed)
	return s.userRepo.UpdateUser(ctx, user)
}

func (s *authService) GetAccessTokenManager() *utils.JWTManager {
	return s.accessToken
}
