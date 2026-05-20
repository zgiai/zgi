package definition

import automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"

const defaultCronTimezone = "Asia/Shanghai"

func normalizeScheduleTimezone(scheduleType automationmodel.AutomationScheduleType, timezone string) string {
	if timezone != "" {
		return timezone
	}

	if scheduleType == automationmodel.AutomationScheduleTypeCron {
		return defaultCronTimezone
	}

	return ""
}
