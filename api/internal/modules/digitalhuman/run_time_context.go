package digitalhuman

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// defaultRunTimezone is the timezone of a run whose account sets none, the
// default the rest of the platform's schedules use.
const defaultRunTimezone = "Asia/Shanghai"

type runTimezoneKey struct{}

// runLocation is the timezone of the account a run serves: the member in
// the console, the channel's owner in a channel, the task's owner in a
// scheduled run.
func (s *RuntimeService) runLocation(ctx context.Context, run *Run) *time.Location {
	name := ""
	if repository, ok := s.store.(*GormRepository); ok && run != nil && run.ActorID != nil {
		_ = repository.db.WithContext(ctx).Table("accounts").Select("timezone").Where("id = ?", *run.ActorID).Scan(&name).Error
	}
	return loadRunLocation(name)
}

func loadRunLocation(name string) *time.Location {
	if name = strings.TrimSpace(name); name != "" {
		if location, err := time.LoadLocation(name); err == nil {
			return location
		}
	}
	if location, err := time.LoadLocation(defaultRunTimezone); err == nil {
		return location
	}
	return time.FixedZone(defaultRunTimezone, 8*60*60)
}

// currentTimeGuidance tells the model what time it is. A model has no clock
// of its own and otherwise guesses, which breaks relative times such as "in
// five minutes" and every task schedule.
func currentTimeGuidance(now time.Time, location *time.Location) string {
	local := now.In(location)
	return fmt.Sprintf("Current time: %s (%s, timezone %s, UTC%s); UTC %s. Use it for relative times such as \"in five minutes\" or \"tomorrow morning\". Times the user gives are in %s unless they name another timezone, and so are task schedules: give them timezone %s.",
		local.Format("2006-01-02 15:04"), local.Weekday().String(), location.String(), local.Format("-07:00"),
		now.UTC().Format(time.RFC3339), location.String(), location.String())
}

func withRunTimezone(ctx context.Context, location *time.Location) context.Context {
	if location == nil {
		return ctx
	}
	return context.WithValue(ctx, runTimezoneKey{}, location.String())
}

func runTimezone(ctx context.Context) string {
	name, _ := ctx.Value(runTimezoneKey{}).(string)
	return name
}
