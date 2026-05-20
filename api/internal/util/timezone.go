package util

import (
	"time"
)

// ToCST Convert to Beijing time zone
func ToCST(t *time.Time) *time.Time {
	var cst = time.FixedZone("CST", 8*3600)
	res := time.Date(
		t.Year(), t.Month(), t.Day(),
		t.Hour(), t.Minute(), t.Second(), t.Nanosecond(),
		cst,
	)
	return &res
}
