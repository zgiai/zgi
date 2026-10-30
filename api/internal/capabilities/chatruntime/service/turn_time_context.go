package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// defaultTurnTimezone is the timezone of a run whose user has none, the same
// as for DigitalHuman runs.
const defaultTurnTimezone = "Asia/Shanghai"

// turnTimeReuseWindow is how long a continuation keeps the time its turn
// started with, so its requests extend the first run's cached prefix.
const turnTimeReuseWindow = 15 * time.Minute

// applyCurrentTimeContext tells a run today's date (ADR 0043). A model has no
// clock and otherwise assumes the year of its training data, often without
// noticing that it should ask. Only the date is given: it stays the same all
// day, so a turn whose history already holds it leaves it out, and the time of
// day comes from the time tools. A caller that writes its own time guidance,
// such as a DigitalHuman, sets CurrentTimeProvided.
func (s *service) applyCurrentTimeContext(ctx context.Context, scope Scope, parts *chatRequestParts, now time.Time) {
	if parts == nil || parts.CurrentTimeProvided {
		return
	}
	location := s.turnLocation(ctx, scope, parts)
	parts.UserTimezone = location.String()
	turnTime := now
	if started := parts.TurnStartedAt; !started.IsZero() && now.Sub(started) >= 0 && now.Sub(started) < turnTimeReuseWindow {
		turnTime = started
	}
	parts.CurrentTimeContext = currentTimeContext(turnTime, location)
}

func (s *service) turnLocation(ctx context.Context, scope Scope, parts *chatRequestParts) *time.Location {
	name := strings.TrimSpace(parts.UserTimezone)
	if name == "" && s.repos != nil && s.repos.DB != nil && scope.AccountID != uuid.Nil {
		_ = s.repos.DB.WithContext(ctx).Table("accounts").Select("timezone").Where("id = ?", scope.AccountID).Scan(&name).Error
	}
	if name = strings.TrimSpace(name); name != "" {
		if location, err := time.LoadLocation(name); err == nil {
			return location
		}
	}
	if location, err := time.LoadLocation(defaultTurnTimezone); err == nil {
		return location
	}
	return time.FixedZone(defaultTurnTimezone, 8*60*60)
}

func currentTimeContext(now time.Time, location *time.Location) string {
	local := now.In(location)
	return fmt.Sprintf("Today is %s, %s (timezone %s, UTC%s). Read dates and times the user gives in this timezone. For the time of day, or to work out another date (its weekday, the days between two dates, a date some days away), call the time tools when you have them instead of computing it yourself.",
		local.Format("2006-01-02"), local.Weekday().String(), location.String(), local.Format("-07:00"))
}

// preparedUserTimezone is the timezone the time tools default to: the one the
// turn context used, or the run configuration's.
func preparedUserTimezone(prepared *PreparedChat) string {
	if prepared == nil {
		return ""
	}
	if prepared.parts != nil && strings.TrimSpace(prepared.parts.UserTimezone) != "" {
		return strings.TrimSpace(prepared.parts.UserTimezone)
	}
	return strings.TrimSpace(prepared.RunConfig.UserTimezone)
}
