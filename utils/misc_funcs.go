package utils

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

func TranslateDayOfWeek(dayOfWeek string) string {
	dayOfWeek = strings.ToLower(dayOfWeek)
	switch dayOfWeek {
	case "monday":
		return "Senin"
	case "tuesday":
		return "Selasa"
	case "wednesday":
		return "Rabu"
	case "thursday":
		return "Kamis"
	case "friday":
		return "Jumat"
	case "saturday":
		return "Sabtu"
	case "sunday":
		return "Minggu"
	default:
		return dayOfWeek
	}
}

// Helper function to normalize phone numbers for WhatsApp
func NormalizePhoneNumber(phone string) string {
	// Remove all non-digit characters
	phone = strings.TrimSpace(phone)
	phone = regexp.MustCompile(`[^\d]`).ReplaceAllString(phone, "")

	// Handle Indonesian phone numbers
	if strings.HasPrefix(phone, "0") {
		phone = "62" + phone[1:] // Convert 08... to 628...
	} else if strings.HasPrefix(phone, "62") {
		// Already correct format
	} else if strings.HasPrefix(phone, "+62") {
		phone = phone[1:] // Remove +
	}

	return phone
}

func CalculateEndTime(startTime string, durationHours float64) string {
	const timeLayout = "15:04"
	t, err := time.Parse(timeLayout, startTime)
	if err != nil {
		fmt.Printf("⚠️ Invalid time format: %v\n", err)
		return startTime
	}

	duration := time.Duration(durationHours * float64(time.Hour))
	endTime := t.Add(duration)
	return endTime.Format(timeLayout)
}

// GetNextClassDate calculates the next occurrence of a specific day and time
func GetNextClassDate(dayOfWeek string, startTime time.Time) time.Time {
	loc, err := time.LoadLocation("Asia/Makassar")
	if err != nil {
		loc = time.Local
	}

	dayMap := map[string]time.Weekday{
		"minggu": time.Sunday,
		"senin":  time.Monday,
		"selasa": time.Tuesday,
		"rabu":   time.Wednesday,
		"kamis":  time.Thursday,
		"jumat":  time.Friday,
		"sabtu":  time.Saturday,
	}

	targetDay, ok := dayMap[strings.ToLower(dayOfWeek)]
	if !ok {
		// Fallback to next week same day if invalid
		return time.Now().In(loc).AddDate(0, 0, 7)
	}

	now := time.Now().In(loc)
	currentDay := now.Weekday()

	// Calculate days until target
	daysUntil := int(targetDay - currentDay)
	
	// If today is the target day, check if the class time is still in the future
	if daysUntil == 0 {
		targetTime := time.Date(
			now.Year(),
			now.Month(),
			now.Day(),
			startTime.Hour(),
			startTime.Minute(),
			0, 0, loc,
		)
		
		// If the class time today hasn't passed yet, return today's date
		if targetTime.After(now) {
			return targetTime
		}
		
		// If class time today has passed, we need next week
		daysUntil = 7
	} else if daysUntil < 0 {
		daysUntil += 7
	}

	nextDate := now.AddDate(0, 0, daysUntil)
	targetTime := time.Date(
		nextDate.Year(),
		nextDate.Month(),
		nextDate.Day(),
		startTime.Hour(),
		startTime.Minute(),
		0, 0, loc,
	)

	// Enforce H-1 rule for next occurrences (must be at least 24 hours away)
	// If the next class occurrence is less than 24 hours away (or already passed),
	// the user cannot book it or the class is already locked.
	// Therefore, the next legitimate occurrence is 7 days later.
	if targetTime.Sub(now) < 24*time.Hour {
		targetTime = targetTime.AddDate(0, 0, 7)
	}

	return targetTime
}

// GetDayName returns Indonesian day name from time.Weekday
func GetDayName(weekday time.Weekday) string {
	dayNames := map[time.Weekday]string{
		time.Sunday:    "Minggu",
		time.Monday:    "Senin",
		time.Tuesday:   "Selasa",
		time.Wednesday: "Rabu",
		time.Thursday:  "Kamis",
		time.Friday:    "Jumat",
		time.Saturday:  "Sabtu",
	}
	return dayNames[weekday]
}
