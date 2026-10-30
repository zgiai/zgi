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

// applyCurrentTimeContext tells a run what time it is (ADR 0043). A model has
// no clock and otherwise guesses or calls a tool for today's date. The time is
// part of the turn context, sent after the history, so it changes no cached
// prefix. A caller that writes its own time guidance, such as a DigitalHuman,
// sets CurrentTimeProvided.
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
	return fmt.Sprintf("Current time: %s, %s (timezone %s, UTC%s). Use it for relative dates and times such as \"tomorrow\" or \"next Friday\", and read times the user gives in this timezone.",
		local.Format("2006-01-02 15:04"), local.Weekday().String(), location.String(), local.Format("-07:00"))
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
