package digitalhuman

import (
	"context"
	"fmt"
	"strings"
	"time"

	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
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
	return fmt.Sprintf("Current time: %s, %s (the user's timezone %s, UTC%s; UTC now %s). Use it for relative times such as \"in five minutes\" or \"tomorrow morning\". Read times the user gives in their timezone and give task schedules timezone %s. When you mention a time, say it the way people do, such as \"下午 3:43\", \"明天早上 9 点\" or \"每周五下午 5 点\": no seconds, no year unless it is not this year, and no timezone unless it differs from the user's or they ask.",
		local.Format("2006-01-02 15:04"), local.Weekday().String(), location.String(), local.Format("-07:00"),
		now.UTC().Format(time.RFC3339), location.String())
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

// addSystemGuidance adds guidance that stays the same from run to run of a
// conversation, such as the memory and task rules, to the system prompt, where
// providers cache it with the rest of the prefix (ADR 0032).
func addSystemGuidance(config *chatruntime.RunConfig, guidance string) {
	guidance = strings.TrimSpace(guidance)
	if config == nil || guidance == "" {
		return
	}
	if strings.TrimSpace(config.SystemPrompt) == "" {
		config.SystemPrompt = guidance
		return
	}
	config.SystemPrompt = strings.TrimSpace(config.SystemPrompt) + "\n\n" + guidance
}

// addTurnContext adds guidance that changes from run to run, such as the
// time, memory or channel context. It is sent after the history instead of in
// the system prompt, which then stays identical across runs and can be served
// from the provider's prompt cache (ADR 0032).
func addTurnContext(config *chatruntime.RunConfig, guidance string) {
	guidance = strings.TrimSpace(guidance)
	if config == nil || guidance == "" {
		return
	}
	config.TurnContextSections = append(config.TurnContextSections, guidance)
	if strings.TrimSpace(config.TurnContext) == "" {
		config.TurnContext = guidance
		return
	}
	config.TurnContext = strings.TrimSpace(config.TurnContext) + "\n\n" + guidance
}
