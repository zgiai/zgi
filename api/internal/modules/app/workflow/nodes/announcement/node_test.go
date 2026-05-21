package announcement

import (
	"testing"
	"time"
)

func TestFormatExpirationTimeForOutputUsesLocationAndTimezoneName(t *testing.T) {
	location, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	expiration := time.Date(2026, 5, 21, 10, 30, 0, 0, time.UTC)
	got := formatExpirationTimeForOutput(expiration, location, "Asia/Tokyo")
	want := "2026-05-21 19:30:00 Asia/Tokyo"
	if got != want {
		t.Fatalf("formatExpirationTimeForOutput() = %q, want %q", got, want)
	}
}
