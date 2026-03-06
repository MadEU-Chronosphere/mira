package service

import (
	"chronosphere/domain"
	"chronosphere/utils"
	"context"
	"errors"
	"fmt"
	"os"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"golang.org/x/crypto/bcrypt"
)

type adminService struct {
	adminRepo domain.AdminRepository
	messenger *whatsmeow.Client
}

func NewAdminService(adminRepo domain.AdminRepository, meow *whatsmeow.Client) domain.AdminUseCase {
	return &adminService{
		adminRepo: adminRepo,
		messenger: meow,
	}
}

func (s *adminService) UpdateAdmin(ctx context.Context, payload domain.User) error {
	if err := s.adminRepo.UpdateAdmin(ctx, payload); err != nil {
		return errors.New(utils.TranslateDBError(err))
	}
	return nil
}

func (s *adminService) ClearUserDeletedAt(ctx context.Context, userUUID string) error {
	err := s.adminRepo.ClearUserDeletedAt(ctx, userUUID)
	if err != nil {
		return err
	}

	return nil
}

func (s *adminService) GetAllClassHistories(ctx context.Context) (*[]domain.ClassHistory, error) {
	data, err := s.adminRepo.GetAllClassHistories(ctx)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// Managers =====================================================================================================
// TEACHER MANAGEMENT
func (s *adminService) GetAllManagers(ctx context.Context) ([]domain.User, error) {
	data, err := s.adminRepo.GetAllManagers(ctx)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (s *adminService) CreateManager(ctx context.Context, user *domain.User) (*domain.User, error) {
	if user.Name == "" || user.Email == "" || user.Phone == "" || user.Password == "" {
		return nil, errors.New("semua field wajib diisi")
	}

	user.Role = domain.RoleManagement

	hashed, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errors.New("gagal mengenkripsi password")
	}
	user.Password = string(hashed)

	created, err := s.adminRepo.CreateManager(ctx, user)
	if err != nil {
		return nil, errors.New(utils.TranslateDBError(err))
	}

	return created, nil
}

// ✅ Update Teacher Profile
func (s *adminService) UpdateManager(ctx context.Context, user *domain.User) error {
	if user.UUID == "" {
		return errors.New("uuid tidak boleh kosong")
	}

	if err := s.adminRepo.UpdateManager(ctx, user); err != nil {
		return errors.New(utils.TranslateDBError(err))
	}
	return nil
}

func (s *adminService) GetAllManager(ctx context.Context) ([]domain.User, error) {
	teachers, err := s.adminRepo.GetAllManagers(ctx)
	if err != nil {
		return nil, errors.New(utils.TranslateDBError(err))
	}
	return teachers, nil
}

func (s *adminService) GetManagerByUUID(ctx context.Context, uuid string) (*domain.User, error) {
	if uuid == "" {
		return nil, errors.New("uuid tidak boleh kosong")
	}

	teacher, err := s.adminRepo.GetManagerByUUID(ctx, uuid)
	if err != nil {
		return nil, errors.New(utils.TranslateDBError(err))
	}
	return teacher, nil
}

func (s *adminService) GetPackagesByID(ctx context.Context, id int) (*domain.Package, error) {
	if id <= 0 {
		return nil, errors.New("ID paket tidak valid")
	}
	return s.adminRepo.GetPackagesByID(ctx, id)
}

// AssignPackageToStudent assigns a package to a student
func (s *adminService) AssignPackageToStudent(ctx context.Context, studentUUID string, packageID int) error {
	if studentUUID == "" {
		return errors.New("UUID siswa wajib diisi")
	}
	if packageID <= 0 {
		return errors.New("ID paket tidak valid")
	}

	dataStudent, dataPackage, err := s.adminRepo.AssignPackageToStudent(ctx, studentUUID, packageID)
	if err != nil {
		return err
	}

	if s.messenger != nil {
		// Send WhatsApp notification to student
		phoneNormalized := utils.NormalizePhoneNumber(dataStudent.Phone)
		if phoneNormalized != "" && s.messenger != nil {
			msgToStudent := fmt.Sprintf(
				`Halo %s,

Paket les Anda telah aktif!
• Paket: %s (%s)
• Instrument: %s
• Kuota: %d sesi

Silakan login ke aplikasi untuk mulai booking sesi.

Terima kasih,

🌐 Website: %s
🔔 %s Notification System
`,
				dataStudent.Name,
				dataPackage.Name,
				dataPackage.Description,
				dataPackage.Instrument.Name,
				dataPackage.Quota,
				os.Getenv("TARGETED_DOMAIN"),
				os.Getenv("APP_NAME"),
			)

			jid := types.NewJID(phoneNormalized, types.DefaultUserServer)
			waMessage := &waE2E.Message{
				Conversation: &msgToStudent,
			}

			go func() {
				s.messenger.SendMessage(context.Background(), jid, waMessage)
			}()
		}
	}

	return nil
}

// CreatePackage creates a package
func (s *adminService) CreatePackage(ctx context.Context, pkg *domain.Package) (*domain.Package, error) {
	if pkg == nil {
		return nil, errors.New("paket tidak boleh kosong")
	}
	created, err := s.adminRepo.CreatePackage(ctx, pkg)
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (s *adminService) UpdatePackage(ctx context.Context, pkg *domain.Package) error {
	if pkg == nil {
		return errors.New("paket tidak boleh kosong")
	}
	return s.adminRepo.UpdatePackage(ctx, pkg)
}

func (s *adminService) DeletePackage(ctx context.Context, id int) error {
	if id <= 0 {
		return errors.New("ID paket tidak valid")
	}
	return s.adminRepo.DeletePackage(ctx, id)
}

// CreateInstrument creates a new instrument (note: accepts *domain.Instrument)
func (s *adminService) CreateInstrument(ctx context.Context, instrument *domain.Instrument) (*domain.Instrument, error) {
	if instrument == nil {
		return nil, errors.New("instrumen tidak boleh kosong")
	}
	if instrument.Name == "" {
		return nil, errors.New("nama instrumen tidak boleh kosong")
	}
	created, err := s.adminRepo.CreateInstrument(ctx, instrument)
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (s *adminService) UpdateInstrument(ctx context.Context, instrument *domain.Instrument) error {
	if instrument == nil {
		return errors.New("instrumen tidak boleh kosong")
	}
	return s.adminRepo.UpdateInstrument(ctx, instrument)
}

func (s *adminService) DeleteInstrument(ctx context.Context, id int) error {
	if id <= 0 {
		return errors.New("ID instrumen tidak valid")
	}
	return s.adminRepo.DeleteInstrument(ctx, id)
}

// GetAllPackages returns all packages
func (s *adminService) GetAllPackages(ctx context.Context) ([]domain.Package, error) {
	return s.adminRepo.GetAllPackages(ctx)
}

// GetAllInstruments returns all instruments
func (s *adminService) GetAllInstruments(ctx context.Context) ([]domain.Instrument, error) {
	return s.adminRepo.GetAllInstruments(ctx)
}

// GetAllStudents returns all students
func (s *adminService) GetAllStudents(ctx context.Context) ([]domain.User, error) {
	return s.adminRepo.GetAllStudents(ctx)
}

// GetAllUsers returns all users
func (s *adminService) GetAllUsers(ctx context.Context) ([]domain.User, error) {
	return s.adminRepo.GetAllUsers(ctx)
}

// GetStudentByUUID fetches a student by UUID
func (s *adminService) GetStudentByUUID(ctx context.Context, uuid string) (*domain.User, error) {
	if uuid == "" {
		return nil, errors.New("UUID wajib diisi")
	}
	return s.adminRepo.GetStudentByUUID(ctx, uuid)
}

// TEACHER MANAGEMENT
func (s *adminService) CreateTeacher(ctx context.Context, user *domain.User, instrumentIDs []int) (*domain.User, error) {
	// Business validation
	if user.Name == "" || user.Email == "" || user.Phone == "" || user.Password == "" {
		return nil, errors.New("semua field wajib diisi")
	}

	user.Role = domain.RoleTeacher // enforce role

	// Hash password sebelum disimpan
	hashed, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errors.New("gagal mengenkripsi password")
	}
	user.Password = string(hashed)

	created, err := s.adminRepo.CreateTeacher(ctx, user, instrumentIDs)
	if err != nil {
		return nil, errors.New(utils.TranslateDBError(err))
	}

	return created, nil
}

// ✅ Update Teacher Profile
func (s *adminService) UpdateTeacher(ctx context.Context, user *domain.User, instrumentIDs []int) error {
	if user.UUID == "" {
		return errors.New("uuid teacher tidak boleh kosong")
	}

	if err := s.adminRepo.UpdateTeacher(ctx, user, instrumentIDs); err != nil {
		return errors.New(utils.TranslateDBError(err))
	}
	return nil
}

// ✅ Get All Teachers
func (s *adminService) GetAllTeachers(ctx context.Context) ([]domain.User, error) {
	teachers, err := s.adminRepo.GetAllTeachers(ctx)
	if err != nil {
		return nil, errors.New(utils.TranslateDBError(err))
	}
	return teachers, nil
}

// ✅ Get Teacher by UUID
func (s *adminService) GetTeacherByUUID(ctx context.Context, uuid string) (*domain.User, error) {
	if uuid == "" {
		return nil, errors.New("uuid tidak boleh kosong")
	}

	teacher, err := s.adminRepo.GetTeacherByUUID(ctx, uuid)
	if err != nil {
		return nil, errors.New(utils.TranslateDBError(err))
	}
	return teacher, nil
}

// ✅ Delete Teacher
func (s *adminService) DeleteUser(ctx context.Context, uuid string) error {
	if uuid == "" {
		return errors.New("uuid tidak boleh kosong")
	}

	if err := s.adminRepo.DeleteUser(ctx, uuid); err != nil {
		return errors.New(utils.TranslateDBError(err))
	}
	return nil
}
