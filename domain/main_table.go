package domain

import (
	"time"
)

const (
	RoleAdmin      = "admin"
	RoleTeacher    = "teacher"
	RoleStudent    = "student"
	RoleManagement = "management"
	RoleFinance    = "finance"

	StatusBooked        = "booked"
	StatusCompleted     = "completed"
	StatusCancelled     = "cancelled"
	StatusRescheduled   = "rescheduled"
	StatusOngoing       = "ongoing"
	StatusUpcoming      = "upcoming"
	StatusClassFinished = "class_finished_waiting_for_approval"
	StatusRebooked      = "rebooked"

	GenderMale   = "male"
	GenderFemale = "female"

	RoomFull      = "Ruangan instrumen penuh"
	RoomAvailable = "Ruangan instrumen tersedia"

	RegularRoomLimit int64 = 8
	DrumRoomLimit    int64 = 3

	DefaultPackageExpiredDuration int = 30
)

type User struct {
	UUID     string  `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"uuid"`
	Name     string  `gorm:"not null;size:50" json:"name"`
	Gender   string  `gorm:"not null;size:10" json:"gender"` // male | female
	Email    string  `gorm:"unique;not null" json:"email"`
	Phone    string  `gorm:"unique;not null;size:14" json:"phone"`
	Password string  `gorm:"not null" json:"-"`
	Role     string  `gorm:"not null" json:"role"`             // student | teacher | admin
	Image    *string `gorm:"type:text" json:"image,omitempty"` // nullable, default NULL

	CreatedAt time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt *time.Time `gorm:"index" json:"deleted_at,omitempty"`

	TeacherProfile *TeacherProfile `gorm:"foreignKey:UserUUID" json:"teacher_profile,omitempty"`
	StudentProfile *StudentProfile `gorm:"foreignKey:UserUUID" json:"student_profile,omitempty"`
}

type StudentProfile struct {
	UserUUID             string           `gorm:"primaryKey;type:uuid;constraint:OnDelete:CASCADE;" json:"user_uuid"`
	Packages             []StudentPackage `gorm:"foreignKey:StudentUUID;constraint:OnDelete:CASCADE;" json:"packages"`
	LatestClassHistories *[]ClassHistory  `gorm:"-" json:"latest_class_histories"`
}

type Package struct {
	ID              int         `gorm:"primaryKey" json:"id"`
	Name            string      `gorm:"not null" json:"name"`
	Price           float64     `gorm:"not null" json:"price"`
	PromoPrice      float64     `gorm:"default:0" json:"promo_price"`
	IsPromoActive   bool        `gorm:"default:false" json:"is_promo_active"`
	IsTrial         bool        `gorm:"default:false" json:"is_trial"` // Paket percobaan/trial
	Quota           int         `gorm:"not null" json:"quota"`
	Duration        int         `gorm:"not null" json:"duration"` // Minutes: 30 or 60
	ExpiredDuration int         `json:"expired_duration"`
	Description     string      `json:"description"`
	InstrumentID    *int        `gorm:"default:null" json:"instrument_id"`
	Instrument      *Instrument `gorm:"foreignKey:InstrumentID" json:"instrument,omitempty"`
	CreatedAt       time.Time   `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time   `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt       *time.Time  `gorm:"index" json:"deleted_at,omitempty"`
}

type Setting struct {
	ID                int       `gorm:"primaryKey" json:"id"`
	RegistrationFee   float64   `gorm:"not null;default:50000" json:"registration_fee"`
	TeacherCommission float64   `gorm:"not null;default:0.4" json:"teacher_commission"`
	CreatedAt         time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt         time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

type StudentPackage struct {
	ID             int       `gorm:"primaryKey" json:"id"`
	StudentUUID    string    `gorm:"type:uuid;not null;constraint:OnDelete:CASCADE,OnUpdate:CASCADE;" json:"student_uuid"`
	PackageID      int       `gorm:"not null;constraint:OnDelete:CASCADE,OnUpdate:CASCADE;" json:"package_id"`
	RemainingQuota int       `gorm:"not null" json:"remaining_quota"`
	PricePaid      float64   `gorm:"not null;default:0" json:"price_paid"` // snapshot at purchase time
	StartDate      time.Time `json:"start_date"`
	EndDate        time.Time `json:"end_date"`

	Package *Package `gorm:"foreignKey:PackageID" json:"package,omitempty"`
}

// TeacherAlbum stores up to 5 profile photos for a teacher
type TeacherAlbum struct {
	ID          int       `gorm:"primaryKey" json:"id"`
	TeacherUUID string    `gorm:"type:uuid;not null;index;constraint:OnDelete:CASCADE;" json:"teacher_uuid"`
	URL         string    `gorm:"type:text;not null" json:"url"`
	Caption     string    `gorm:"size:200" json:"caption,omitempty"`
	Order       int       `gorm:"default:0" json:"order"` // Display order 1-5
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
}

type TeacherProfile struct {
	UserUUID string `gorm:"primaryKey;type:uuid;constraint:OnDelete:CASCADE;" json:"user_uuid"`

	// Basic bio
	Bio string `gorm:"type:text" json:"bio"`

	// Education & credentials
	Education    string `gorm:"type:text" json:"education"`    // e.g. "S1 Pendidikan Musik, ISI Yogyakarta"
	Certificates string `gorm:"type:text" json:"certificates"` // e.g. "Grade 8 ABRSM, Diploma Berklee Online"

	// Professional experience
	YearsOfExperience int    `gorm:"default:0" json:"years_of_experience"` // Tahun pengalaman mengajar
	Experience        string `gorm:"type:text" json:"experience"`          // Narasi pengalaman, pertunjukan, dll

	// Teaching style & specialty
	TeachingStyle string `gorm:"type:text" json:"teaching_style"` // e.g. "Sabar, fun, berbasis teori"
	Specialties   string `gorm:"type:text" json:"specialties"`    // e.g. "Jazz, Classical, Improvisation"

	// Languages (comma-separated or JSON string)
	Languages string `gorm:"size:200" json:"languages"` // e.g. "Bahasa Indonesia, English"

	// Relationships
	Instruments []Instrument   `gorm:"many2many:teacher_instruments;constraint:OnDelete:CASCADE;" json:"instruments"`
	Album       []TeacherAlbum `gorm:"foreignKey:TeacherUUID;references:UserUUID" json:"album,omitempty"` // Max 5
}

type Instrument struct {
	ID        int        `gorm:"primaryKey" json:"id"`
	Name      string     `gorm:"unique;size:30;not null" json:"name"`
	CreatedAt time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt *time.Time `gorm:"index" json:"deleted_at,omitempty"`
}

type TeacherSchedule struct {
	ID          int        `gorm:"primaryKey" json:"id"`
	TeacherUUID string     `gorm:"type:uuid;not null" json:"teacher_uuid"`
	DayOfWeek   string     `gorm:"size:10;not null" json:"day_of_week"`
	StartTime   string     `gorm:"type:varchar(5);not null" json:"start_time"` // Format "HH:MM"
	EndTime     string     `gorm:"type:varchar(5);not null" json:"end_time"`   // Format "HH:MM"
	Duration    int        `gorm:"not null;default:0" json:"duration"`
	IsBooked    bool       `gorm:"default:false" json:"is_booked"`
	CreatedAt   time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt   *time.Time `gorm:"index" json:"deleted_at,omitempty"`

	Teacher        User            `gorm:"foreignKey:TeacherUUID;references:UUID" json:"teacher"`
	TeacherProfile *TeacherProfile `gorm:"foreignKey:UserUUID;references:TeacherUUID" json:"teacher_profile,omitempty"`

	// ✅ Add computed field for next class date
	NextClassDate *time.Time `gorm:"-" json:"next_class_date,omitempty"`

	// Availability flags
	IsBookedSameDayAndTime *bool `gorm:"-" json:"is_booked_same_day_and_time,omitempty"`
	IsDurationCompatible   *bool `gorm:"-" json:"is_duration_compatible,omitempty"` // Student's package duration matches
	IsRoomAvailable        *bool `gorm:"-" json:"is_room_available,omitempty"`      // Room slot available
	IsFullyAvailable       *bool `gorm:"-" json:"is_fully_available,omitempty"`     // Both conditions met
}

type Booking struct {
	ID               int             `gorm:"primaryKey" json:"id"`
	StudentUUID      string          `gorm:"type:uuid;not null" json:"student_uuid"`
	Student          User            `gorm:"foreignKey:StudentUUID;references:UUID" json:"student"`
	ScheduleID       int             `gorm:"not null" json:"schedule_id"`
	Schedule         TeacherSchedule `gorm:"foreignKey:ScheduleID" json:"schedule"`
	StudentPackageID int             `gorm:"not null" json:"student_package_id"`              // ✅ Added this field
	PackageUsed      StudentPackage  `gorm:"foreignKey:StudentPackageID" json:"package_used"` // ✅ Added relationship
	ClassDate        time.Time       `gorm:"not null" json:"class_date"`                      // ✅ Add this field
	InstrumentID     int             `gorm:"not null;default:0" json:"instrument_id"`         // ← NEW
	Status           string          `gorm:"size:20;default:'booked'" json:"status"`
	BookedAt         time.Time       `gorm:"autoCreateTime" json:"booked_at"`
	CompletedAt      *time.Time      `json:"completed_at,omitempty"`
	RescheduledAt    *time.Time      `json:"rescheduled_at,omitempty"`
	CancelledAt      *time.Time      `json:"cancelled_at,omitempty"`
	CanceledBy       *string         `gorm:"type:uuid" json:"canceled_by,omitempty"`
	CancelUser       *User           `gorm:"foreignKey:CanceledBy;references:UUID" json:"cancel_user,omitempty"`
	Notes            *string         `json:"notes,omitempty"`

	IsReadyToFinish bool `gorm:"-" json:"is_ready_to_finish"`
	IsManual        bool `gorm:"default:false" json:"is_manual"`
}

type ClassHistory struct {
	ID        int     `gorm:"primaryKey" json:"id"`
	BookingID int     `gorm:"not null;unique" json:"booking_id"`
	Booking   Booking `gorm:"foreignKey:BookingID;constraint:OnDelete:CASCADE;" json:"booking"`
	Status    string  `gorm:"size:20;default:'completed'" json:"status"`
	Notes     *string `json:"notes,omitempty"`

	Documentations []ClassDocumentation `gorm:"foreignKey:ClassHistoryID" json:"documentations"`
	CreatedAt      time.Time            `gorm:"autoCreateTime" json:"created_at"`
}

type ClassDocumentation struct {
	ID             int       `gorm:"primaryKey" json:"id"`
	ClassHistoryID int       `gorm:"not null;index" json:"class_history_id"`
	URL            string    `gorm:"type:text;not null" json:"url"`
	CreatedAt      time.Time `gorm:"autoCreateTime" json:"created_at"`

	ClassHistory ClassHistory `gorm:"foreignKey:ClassHistoryID;constraint:OnDelete:CASCADE;" json:"-"`
}
