package scheduler

import "time"

// NextMonthlyResetDate calculates the next monthly reset date
// subscriptionDay: the day of month from subscription (1-31)
// currentTime: current time
// Uses "month-end clamping" strategy:
// - If subscriptionDay > last day of target month, use last day
// - Example: Jan 31 -> Feb 28/29 -> Mar 31
func NextMonthlyResetDate(subscriptionDay int, currentTime time.Time) time.Time {
	year, month, _ := currentTime.Date()

	// Calculate next month
	nextMonth := month + 1
	nextYear := year
	if nextMonth > 12 {
		nextMonth = 1
		nextYear++
	}

	// Get the last day of next month
	lastDayOfMonth := time.Date(nextYear, nextMonth+1, 0, 0, 0, 0, 0, currentTime.Location()).Day()

	// Apply month-end clamping
	resetDay := subscriptionDay
	if resetDay > lastDayOfMonth {
		resetDay = lastDayOfMonth
	}

	// Keep original hour, minute, second
	return time.Date(nextYear, nextMonth, resetDay,
		currentTime.Hour(), currentTime.Minute(), currentTime.Second(), 0,
		currentTime.Location())
}

// NextMonthlyResetDateFromNow calculates next monthly reset date from now
func NextMonthlyResetDateFromNow(subscriptionDay int) time.Time {
	return NextMonthlyResetDate(subscriptionDay, time.Now())
}

// GetSubscriptionDayOfMonth extracts the day of month from subscription start date
func GetSubscriptionDayOfMonth(startDate time.Time) int {
	return startDate.Day()
}

// IsResetDue checks if the reset time has been reached
func IsResetDue(nextResetAt *time.Time, now time.Time) bool {
	if nextResetAt == nil {
		return false
	}
	return now.After(*nextResetAt) || now.Equal(*nextResetAt)
}

// LastDayOfMonth returns the last day of the given month
func LastDayOfMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// IsSameDay checks if two times are on the same day
func IsSameDay(t1, t2 time.Time) bool {
	y1, m1, d1 := t1.Date()
	y2, m2, d2 := t2.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}

// StartOfDay returns the start of the day (00:00:00)
func StartOfDay(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, t.Location())
}

// EndOfDay returns the end of the day (23:59:59)
func EndOfDay(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 23, 59, 59, 999999999, t.Location())
}
