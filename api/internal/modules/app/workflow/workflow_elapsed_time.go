package workflow

import (
	"math"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
)

const millisecondsPerSecond = 1000.0

// ElapsedMillisecondsSince returns elapsed wall-clock time in milliseconds.
func ElapsedMillisecondsSince(start time.Time) float64 {
	if start.IsZero() {
		return 0
	}
	return durationMilliseconds(time.Since(start))
}

func durationMilliseconds(duration time.Duration) float64 {
	return shared.DurationMilliseconds(duration)
}

func elapsedMillisecondsBetween(start, finish time.Time) float64 {
	return shared.ElapsedMillisecondsBetween(start, finish)
}

func workflowStoredElapsedMilliseconds(elapsed float64, createdAt time.Time, finishedAt *time.Time) float64 {
	if elapsed <= 0 {
		return elapsed
	}

	if finishedAt == nil || createdAt.IsZero() || finishedAt.Before(createdAt) {
		return roundElapsedMilliseconds(elapsed)
	}

	wallClockMS := durationMilliseconds(finishedAt.Sub(createdAt))
	if wallClockMS <= 0 {
		return roundElapsedMilliseconds(elapsed)
	}

	secondsAsMS := elapsed * millisecondsPerSecond
	if elapsedCloseToWallClock(secondsAsMS, wallClockMS) && !elapsedCloseToWallClock(elapsed, wallClockMS) {
		return roundElapsedMilliseconds(secondsAsMS)
	}
	return roundElapsedMilliseconds(elapsed)
}

func workflowStoredRunElapsedMilliseconds(elapsed float64, createdAt time.Time, finishedAt *time.Time) float64 {
	if elapsed <= 0 {
		return elapsed
	}
	if elapsed >= 1 {
		return roundElapsedMilliseconds(elapsed)
	}
	return workflowStoredElapsedMilliseconds(elapsed, createdAt, finishedAt)
}

func elapsedCloseToWallClock(candidateMS float64, wallClockMS float64) bool {
	if candidateMS <= 0 || wallClockMS <= 0 {
		return false
	}
	diff := math.Abs(candidateMS - wallClockMS)
	if diff <= 1 {
		return true
	}
	return diff/wallClockMS <= 0.3
}

func roundElapsedMilliseconds(elapsed float64) float64 {
	return shared.RoundElapsedMilliseconds(elapsed)
}
