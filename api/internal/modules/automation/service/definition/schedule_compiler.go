package definition

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
)

func compileNextRunAt(
	scheduleType automationmodel.AutomationScheduleType,
	timezone string,
	config map[string]interface{},
) (*time.Time, error) {
	return compileNextRunAtFrom(scheduleType, timezone, config, time.Now())
}

func compileNextRunAtFrom(
	scheduleType automationmodel.AutomationScheduleType,
	timezone string,
	config map[string]interface{},
	now time.Time,
) (*time.Time, error) {
	switch scheduleType {
	case automationmodel.AutomationScheduleTypeOnce:
		runAt, err := parseOnceRunAt(config)
		if err != nil {
			return nil, err
		}

		if timezone == "" {
			return &runAt, nil
		}

		location, err := time.LoadLocation(timezone)
		if err != nil {
			return nil, fmt.Errorf("load schedule timezone %s: %w", timezone, err)
		}

		normalized := runAt.In(location)
		return &normalized, nil
	case automationmodel.AutomationScheduleTypeCron:
		timezone = normalizeScheduleTimezone(scheduleType, timezone)
		location, err := time.LoadLocation(timezone)
		if err != nil {
			return nil, fmt.Errorf("load schedule timezone %s: %w", timezone, err)
		}

		schedule, err := parseCronScheduleInLocation(config, location)
		if err != nil {
			return nil, err
		}

		nextRunAt := schedule.Next(now)
		if nextRunAt.IsZero() {
			return nil, fmt.Errorf("cron schedule produced zero next runtime")
		}
		return &nextRunAt, nil
	default:
		return nil, fmt.Errorf("schedule type %s is not implemented in MVP service compiler yet", scheduleType)
	}
}

func advanceTaskAfterDispatch(task *automationmodel.AutomationTask) (*time.Time, automationmodel.AutomationTaskStatus, error) {
	switch task.ScheduleType {
	case automationmodel.AutomationScheduleTypeOnce:
		return nil, automationmodel.AutomationTaskStatusCompleted, nil
	case automationmodel.AutomationScheduleTypeCron:
		if task.NextRunAt == nil {
			return nil, "", fmt.Errorf("cron task %s missing next_run_at for dispatch advancement", task.ID)
		}

		location, err := time.LoadLocation(task.Timezone)
		if err != nil {
			return nil, "", fmt.Errorf("load schedule timezone %s: %w", task.Timezone, err)
		}

		schedule, err := parseCronScheduleInLocation(task.ScheduleConfig, location)
		if err != nil {
			return nil, "", err
		}

		nextRunAt := schedule.Next(*task.NextRunAt)
		if nextRunAt.IsZero() {
			return nil, "", fmt.Errorf("cron task %s produced zero next runtime during advancement", task.ID)
		}
		return &nextRunAt, automationmodel.AutomationTaskStatusActive, nil
	default:
		return nil, "", fmt.Errorf("schedule type %s is not implemented in MVP dispatch advancement yet", task.ScheduleType)
	}
}

func parseOnceRunAt(config map[string]interface{}) (time.Time, error) {
	runAtRaw, ok := config["run_at"]
	if !ok {
		return time.Time{}, fmt.Errorf("once schedule config missing run_at")
	}

	switch value := runAtRaw.(type) {
	case string:
		if value == "" {
			return time.Time{}, fmt.Errorf("once schedule run_at must be a non-empty string or unix timestamp")
		}
		runAt, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return time.Time{}, fmt.Errorf("parse once schedule run_at: %w", err)
		}
		return runAt, nil
	case float64:
		return time.Unix(int64(value), 0), nil
	case int64:
		return time.Unix(value, 0), nil
	case int:
		return time.Unix(int64(value), 0), nil
	default:
		return time.Time{}, fmt.Errorf("once schedule run_at must be a non-empty string or unix timestamp")
	}
}

func parseCronSchedule(config map[string]interface{}) (cron.Schedule, error) {
	cronExpr, err := parseCronExpression(config)
	if err != nil {
		return nil, err
	}

	schedule, err := cron.ParseStandard(cronExpr)
	if err != nil {
		return nil, fmt.Errorf("parse cron schedule cron_expr: %w", err)
	}

	return schedule, nil
}

func parseCronScheduleInLocation(config map[string]interface{}, location *time.Location) (cron.Schedule, error) {
	cronExpr, err := parseCronExpression(config)
	if err != nil {
		return nil, err
	}
	if location != nil {
		cronExpr = fmt.Sprintf("CRON_TZ=%s %s", location.String(), cronExpr)
	}

	schedule, err := cron.ParseStandard(cronExpr)
	if err != nil {
		return nil, fmt.Errorf("parse cron schedule cron_expr: %w", err)
	}

	return schedule, nil
}

func parseCronExpression(config map[string]interface{}) (string, error) {
	cronExprRaw, ok := config["cron_expr"]
	if !ok {
		return "", fmt.Errorf("cron schedule config missing cron_expr")
	}

	cronExpr, ok := cronExprRaw.(string)
	if !ok || cronExpr == "" {
		return "", fmt.Errorf("cron schedule cron_expr must be a non-empty string")
	}

	return cronExpr, nil
}
