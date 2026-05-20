package shared

import (
	"math"
	"time"
)

const (
	MillisecondsPerSecond = 1000.0
	elapsedDecimalScale   = 10.0
)

// DurationMilliseconds returns a duration in milliseconds, rounded to one decimal.
func DurationMilliseconds(duration time.Duration) float64 {
	if duration <= 0 {
		return 0
	}
	return RoundElapsedMilliseconds(duration.Seconds() * MillisecondsPerSecond)
}

// ElapsedMillisecondsBetween returns the wall-clock delta between two timestamps in milliseconds.
func ElapsedMillisecondsBetween(start, finish time.Time) float64 {
	if start.IsZero() || finish.IsZero() || finish.Before(start) {
		return 0
	}
	return DurationMilliseconds(finish.Sub(start))
}

// RoundElapsedMilliseconds normalizes elapsed millisecond values to one decimal.
func RoundElapsedMilliseconds(elapsed float64) float64 {
	if elapsed <= 0 {
		return 0
	}
	return math.Round(elapsed*elapsedDecimalScale) / elapsedDecimalScale
}
