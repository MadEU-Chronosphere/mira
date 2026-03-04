package service

import (
	"chronosphere/domain"
	"chronosphere/utils"
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

type studentUseCase struct {
	repo      domain.StudentRepository
	messenger *whatsmeow.Client
}

func NewStudentUseCase(repo domain.StudentRepository, meowClient *whatsmeow.Client) domain.StudentUseCase {
	return &studentUseCase{repo: repo, messenger: meowClient}
}

func (s *studentUseCase) GetMyClassHistory(ctx context.Context, studentUUID string) (*[]domain.ClassHistory, error) {
	return s.repo.GetMyClassHistory(ctx, studentUUID)
}

func (s *studentUseCase) CancelBookedClass(ctx context.Context, bookingID int, studentUUID string, reason *string) error {
	// set default reason if its nil
	if reason == nil {
		defaultReason := "Alasan tidak diberikan"
		reason = &defaultReason
	}

	data, err := s.repo.CancelBookedClass(ctx, bookingID, studentUUID, reason)
	if err != nil {
		return err
	}

	// Send WhatsApp messages to teacher and student
	if s.messenger != nil {
		s.sendCancelClassNotif(data, reason)
	}

	return nil
}

func (s *studentUseCase) sendCancelClassNotif(booking *domain.Booking, reason *string) {
	// Format the class date and time in Indonesian
	loc, _ := time.LoadLocation("Asia/Makassar") // WITA timezone
	classDate := booking.ClassDate.In(loc)

	// Indonesian day names
	dayName := map[string]string{
		"Monday":    "Senin",
		"Tuesday":   "Selasa",
		"Wednesday": "Rabu",
		"Thursday":  "Kamis",
		"Friday":    "Jumat",
		"Saturday":  "Sabtu",
		"Sunday":    "Minggu",
	}[classDate.Weekday().String()]

	dateStr := classDate.Format("02/01/2006")
	classTime := fmt.Sprintf("%s - %s", booking.Schedule.StartTime, booking.Schedule.EndTime)

	// Message for teacher
	// Gender check
	var teacherMessage string
	if booking.Schedule.Teacher.Gender == "male" {
		teacherMessage = fmt.Sprintf(`*PEMBATALAN KELAS* 

Halo Bapak %s, 

⚠️ Siswa *%s* telah membatalkan kelas dengan detail:
📅 *Hari/Tanggal:* %s, %s
⏰ *Waktu:* %s
⏱️ *Durasi:* %d menit
🎵 *Instrument:* %s

*Alasan:* %s

Terima kasih! 🎵

🌐 Website: %s
🔔 %s Notification System`,
			booking.Schedule.Teacher.Name,
			booking.Student.Name,
			dayName,
			dateStr,
			classTime,
			booking.Schedule.Duration,
			booking.PackageUsed.Package.Instrument.Name,
			*reason,
			os.Getenv("TARGETED_DOMAIN"),
			os.Getenv("APP_NAME"))
	} else {
		teacherMessage = fmt.Sprintf(`*PEMBATALAN KELAS* 

Halo Ibu %s, 

⚠️ Siswa *%s* telah membatalkan kelas dengan detail:
📅 *Hari/Tanggal:* %s, %s
⏰ *Waktu:* %s
⏱️ *Durasi:* %d menit
🎵 *Instrument:* %s

*Alasan:* %s

Terima kasih! 🎵

🌐 Website: %s
🔔 %s Notification System`,
			booking.Schedule.Teacher.Name,
			booking.Student.Name,
			dayName,
			dateStr,
			classTime,
			booking.Schedule.Duration,
			booking.PackageUsed.Package.Instrument.Name,
			*reason,
			os.Getenv("TARGETED_DOMAIN"),
			os.Getenv("APP_NAME"))
	}

	// Message for student
	studentMessage := fmt.Sprintf(`*PEMBATALAN KELAS* 

Halo %s, 

✅ Pembatalan kelas Anda telah berhasil! 

*Detail Kelas:* 
👨‍🏫 *Guru:* %s
📅 *Hari/Tanggal:* %s, %s
⏰ *Waktu:* %s
⏱️ *Durasi:* %d menit
🎵 *Instrument:* %s

*Alasan:* %s

Terima kasih! 🎵

🌐 Website: %s
🔔 %s Notification System`,
		booking.Student.Name,
		booking.Schedule.Teacher.Name,
		dayName,
		dateStr,
		classTime,
		booking.Schedule.Duration,
		booking.PackageUsed.Package.Instrument.Name,
		*reason,
		os.Getenv("TARGETED_DOMAIN"),
		os.Getenv("APP_NAME"))

	// Capture values before launching goroutine to avoid data races on booking.
	tPhone := booking.Schedule.Teacher.Phone
	sPhone := booking.Student.Phone
	tMsg := teacherMessage
	sMsg := studentMessage

	go func() {
		notifyCtx := context.Background()
		teacherPhone := utils.NormalizePhoneNumber(tPhone)
		studentPhone := utils.NormalizePhoneNumber(sPhone)
		teacherJID := types.NewJID(teacherPhone, types.DefaultUserServer)
		studentJID := types.NewJID(studentPhone, types.DefaultUserServer)

		if teacherPhone != "" {
			_, err := s.messenger.SendMessage(notifyCtx, teacherJID, &waE2E.Message{Conversation: &tMsg})
			if err != nil {
				log.Printf("🔕 Failed to send WhatsApp to teacher %s: %v", teacherPhone, err)
			} else {
				log.Printf("🔔 WhatsApp notification sent to teacher: %s", tPhone)
			}
		}
		if studentPhone != "" {
			_, err := s.messenger.SendMessage(notifyCtx, studentJID, &waE2E.Message{Conversation: &sMsg})
			if err != nil {
				log.Printf("🔕 Failed to send WhatsApp to student %s: %v", studentPhone, err)
			} else {
				log.Printf("🔔 WhatsApp notification sent to student: %s", sPhone)
			}
		}
	}()
}

func (s *studentUseCase) BookClass(ctx context.Context, studentUUID string, scheduleID int, instrumentID int) error {
	data, err := s.repo.BookClass(ctx, studentUUID, scheduleID, instrumentID)
	if err != nil {
		return err
	}

	// Send WhatsApp messages to teacher and student
	if s.messenger != nil {
		s.sendBookClassNotif(data)
	}

	return nil
}

func (s *studentUseCase) sendBookClassNotif(booking *domain.Booking) {
	// Format the class date and time in Indonesian
	loc, _ := time.LoadLocation("Asia/Makassar") // WITA timezone
	classDate := booking.ClassDate.In(loc)

	// Indonesian day names
	dayName := map[string]string{
		"Monday":    "Senin",
		"Tuesday":   "Selasa",
		"Wednesday": "Rabu",
		"Thursday":  "Kamis",
		"Friday":    "Jumat",
		"Saturday":  "Sabtu",
		"Sunday":    "Minggu",
	}[classDate.Weekday().String()]

	dateStr := classDate.Format("02/01/2006")
	classTime := fmt.Sprintf("%s - %s", booking.Schedule.StartTime, booking.Schedule.EndTime)

	// Message for teacher
	// Gender check
	var teacherMessage string
	if booking.Schedule.Teacher.Gender == "male" {
		teacherMessage = fmt.Sprintf(`*PEMBERITAHUAN KELAS BARU*

Halo Bapak %s,

Siswa *%s* telah memesan kelas dengan detail:
📅 *Hari/Tanggal:* %s, %s
⏰ *Waktu:* %s
⏱️ *Durasi:* %d menit
🎵 *Instrument:* %s

_Silakan persiapkan materi pengajaran untuk kelas ini. Jangan lupa untuk mencatat hasil kelas setelah kelas selesai._

Terima kasih! 🎵

🌐 Website: %s
🔔 %s Notification System`,
			booking.Schedule.Teacher.Name,
			booking.Student.Name,
			dayName,
			dateStr,
			classTime,
			booking.Schedule.Duration,
			booking.PackageUsed.Package.Instrument.Name,
			os.Getenv("TARGETED_DOMAIN"),
			os.Getenv("APP_NAME"))
	} else {
		teacherMessage = fmt.Sprintf(`*PEMBERITAHUAN KELAS BARU*

Halo Ibu %s,

Siswa *%s* telah memesan kelas dengan detail:
📅 *Hari/Tanggal:* %s, %s
⏰ *Waktu:* %s
⏱️ *Durasi:* %d menit
🎵 *Instrument:* %s

_Silakan persiapkan materi pengajaran untuk kelas ini. Jangan lupa untuk mencatat hasil kelas setelah kelas selesai._

Terima kasih! 🎵

🌐 Website: %s
🔔 %s Notification System`,
			booking.Schedule.Teacher.Name,
			booking.Student.Name,
			dayName,
			dateStr,
			classTime,
			booking.Schedule.Duration,
			booking.PackageUsed.Package.Instrument.Name,
			os.Getenv("TARGETED_DOMAIN"),
			os.Getenv("APP_NAME"))
	}

	// Message for student
	studentMessage := fmt.Sprintf(`*KONFIRMASI PEMESANAN KELAS*

Halo %s,

✅ Pemesanan kelas Anda telah berhasil!

*Detail Kelas:*
👨‍🏫 *Guru:* %s
📅 *Hari/Tanggal:* %s, %s
⏰ *Waktu:* %s
⏱️ *Durasi:* %d menit
🎵 *Instrument:* %s

*Jika ada perubahan:*
- Hubungi guru atau admin
- Batalkan minimal 1 hari sebelum kelas

_Selamat belajar! 🎶_

🌐 Website: %s
🔔 %s Notification System`,
		booking.Student.Name,
		booking.Schedule.Teacher.Name,
		dayName,
		dateStr,
		classTime,
		booking.Schedule.Duration,
		booking.PackageUsed.Package.Instrument.Name,
		os.Getenv("TARGETED_DOMAIN"),
		os.Getenv("APP_NAME"))

	// Capture values before launching goroutine to avoid data races on booking.
	tPhone := booking.Schedule.Teacher.Phone
	sPhone := booking.Student.Phone
	tMsg := teacherMessage
	sMsg := studentMessage

	go func() {
		notifyCtx := context.Background()
		teacherPhone := utils.NormalizePhoneNumber(tPhone)
		studentPhone := utils.NormalizePhoneNumber(sPhone)
		teacherJID := types.NewJID(teacherPhone, types.DefaultUserServer)
		studentJID := types.NewJID(studentPhone, types.DefaultUserServer)

		if teacherPhone != "" {
			_, err := s.messenger.SendMessage(notifyCtx, teacherJID, &waE2E.Message{Conversation: &tMsg})
			if err != nil {
				log.Printf("🔕 Failed to send WhatsApp to teacher %s: %v", teacherPhone, err)
			} else {
				log.Printf("🔔 WhatsApp notification sent to teacher: %s", tPhone)
			}
		}
		if studentPhone != "" {
			_, err := s.messenger.SendMessage(notifyCtx, studentJID, &waE2E.Message{Conversation: &sMsg})
			if err != nil {
				log.Printf("🔕 Failed to send WhatsApp to student %s: %v", studentPhone, err)
			} else {
				log.Printf("🔔 WhatsApp notification sent to student: %s", sPhone)
			}
		}
	}()
}

func (s *studentUseCase) GetMyProfile(ctx context.Context, userUUID string) (*domain.User, error) {
	return s.repo.GetMyProfile(ctx, userUUID)
}

func (s *studentUseCase) UpdateStudentData(ctx context.Context, userUUID string, user domain.User) error {
	return s.repo.UpdateStudentData(ctx, userUUID, user)
}

func (s *studentUseCase) GetAllAvailablePackages(ctx context.Context) (*[]domain.Package, error) {
	return s.repo.GetAllAvailablePackages(ctx)
}

func (s *studentUseCase) GetMyBookedClasses(ctx context.Context, studentUUID string) (*[]domain.Booking, error) {
	return s.repo.GetMyBookedClasses(ctx, studentUUID)
}

func (s *studentUseCase) GetAvailableSchedules(ctx context.Context, studentUUID string) (*[]domain.TeacherSchedule, error) {
	return s.repo.GetAvailableSchedules(ctx, studentUUID)
}
