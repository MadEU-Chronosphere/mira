package service

import (
	"chronosphere/domain"
	"chronosphere/utils"
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"golang.org/x/crypto/bcrypt"
)

func NewManagerService(managerRepo domain.ManagerRepository, meow *whatsmeow.Client) domain.ManagerUseCase {
	return &managerService{
		managerRepo: managerRepo,
		messenger:   meow,
	}
}

type managerService struct {
	managerRepo domain.ManagerRepository
	messenger   *whatsmeow.Client
}

func (s *managerService) GetCancelledClassHistories(ctx context.Context) (*[]domain.ClassHistory, error) {
	return s.managerRepo.GetCancelledClassHistories(ctx)
}

func (s *managerService) RebookWithSubstitute(ctx context.Context, req domain.RebookInput) (*domain.Booking, error) {
	booking, err := s.managerRepo.RebookWithSubstitute(ctx, req)
	if err != nil {
		return nil, err
	}

	// Notify substitute teacher via WhatsApp
	if s.messenger != nil {
		loc, _ := time.LoadLocation("Asia/Makassar")
		classDate := booking.ClassDate.In(loc)
		dayName := map[string]string{
			"Monday": "Senin", "Tuesday": "Selasa", "Wednesday": "Rabu",
			"Thursday": "Kamis", "Friday": "Jumat", "Saturday": "Sabtu", "Sunday": "Minggu",
		}[classDate.Weekday().String()]

		salutation := "Bapak"
		if booking.Schedule.Teacher.Gender == "female" {
			salutation = "Ibu"
		}

		msg := fmt.Sprintf(
			`*PENUGASAN GURU PENGGANTI*

Halo %s %s,

Anda ditugaskan sebagai guru pengganti untuk kelas berikut:
👤 *Siswa:* %s
📅 *Hari/Tanggal:* %s, %s
⏰ *Waktu:* %s - %s
🎵 *Instrumen:* %s

Kelas ini adalah pengganti dari kelas yang dibatalkan. Silakan selesaikan kelas dan tambahkan catatan melalui aplikasi.

🌐 %s
🔔 %s Notification System`,
			salutation,
			booking.Schedule.Teacher.Name,
			booking.Student.Name,
			dayName,
			classDate.Format("02/01/2006"),
			booking.Schedule.StartTime,
			booking.Schedule.EndTime,
			booking.PackageUsed.Package.Instrument.Name,
			"https://www.madeu.app",
			os.Getenv("APP_NAME"),
		)

		phone := utils.NormalizePhoneNumber(booking.Schedule.Teacher.Phone)
		if phone != "" {
			jid := types.NewJID(phone, types.DefaultUserServer)
			waMsg := &waE2E.Message{Conversation: &msg}
			go func() {
				s.messenger.SendMessage(context.Background(), jid, waMsg)
			}()
		}
	}

	return booking, nil
}

// Setting
func (s *managerService) GetSetting(ctx context.Context) (*domain.Setting, error) {
	return s.managerRepo.GetSetting(ctx)
}

func (s *managerService) UpdateSetting(ctx context.Context, setting *domain.Setting) error {
	if setting == nil {
		return errors.New("pengaturan tidak valid")
	}
	return s.managerRepo.UpdateSetting(ctx, setting)
}

// Students =====================================================================================================

func (s *managerService) UpdateStudent(ctx context.Context, student *domain.User) error {
	if student.UUID == "" {
		return errors.New("uuid siswa tidak boleh kosong")
	}

	// Only hash password if a new one was provided
	if student.Password != "" {
		hashed, err := bcrypt.GenerateFromPassword([]byte(student.Password), bcrypt.DefaultCost)
		if err != nil {
			return errors.New("gagal mengenkripsi password")
		}
		student.Password = string(hashed)
	}

	if err := s.managerRepo.UpdateStudent(ctx, student); err != nil {
		return errors.New(utils.TranslateDBError(err))
	}
	return nil
}

// ✅ Get All Students
func (s *managerService) GetAllStudents(ctx context.Context) ([]domain.User, error) {
	data, err := s.managerRepo.GetAllStudents(ctx)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (s *managerService) UpdateManager(ctx context.Context, manager *domain.User) error {
	return s.managerRepo.UpdateManager(ctx, manager)
}

// ✅ Get Student by UUID
func (s *managerService) GetStudentByUUID(ctx context.Context, uuid string) (*domain.User, error) {
	data, err := s.managerRepo.GetStudentByUUID(ctx, uuid)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// ✅ Modify Student Package Quota
func (s *managerService) ModifyStudentPackageQuota(ctx context.Context, studentUUID string, packageID int, incomingQuota int) error {
	data, err := s.managerRepo.ModifyStudentPackageQuota(ctx, studentUUID, packageID, incomingQuota)
	if err != nil {
		return err
	}

	// Send notification to student
	phoneNormalized := utils.NormalizePhoneNumber(data.Phone)
	if phoneNormalized != "" && s.messenger != nil {
		msgToStudent := fmt.Sprintf(
			`*NOTIFIKASI PENYESUAIAN KUOTA*

Halo %s,

Telah dilakukan penyesuaian kuota paket les Anda:
📊 Kuota saat ini: %d sesi
Kuota yang telah dikembalikan dapat segera digunakan untuk penjadwalan sesi berikutnya.

Terima kasih atas pengertiannya.

🌐 Website: %s
🔔 %s Notification System
`,
			data.Name,
			incomingQuota,
			"https://www.madeu.app",
			os.Getenv("APP_NAME"),
		)

		jid := types.NewJID(phoneNormalized, types.DefaultUserServer)
		waMessage := &waE2E.Message{
			Conversation: &msgToStudent,
		}

		go s.messenger.SendMessage(context.Background(), jid, waMessage)
	}

	return nil
}
