package migrations

import (
	"strings"
	"testing"
)

func TestRecurringDigitalHumanTaskStatusRepairKeepsSeriesActive(t *testing.T) {
	for _, fragment := range []string{
		"work_status = 'pending'",
		"task_kind = 'digital_human'",
		"schedule_type = 'cron'",
		"status = 'active'",
		"work_status = 'completed'",
		"revision = revision + 1",
		"automation_task_activities",
	} {
		if !strings.Contains(repairRecurringDigitalHumanTaskStatusSQL, fragment) {
			t.Fatalf("repair migration is missing %q", fragment)
		}
	}
	for _, fragment := range []string{
		"automation_tasks_recurring_completion_check",
		"schedule_type <> 'cron'",
		"work_status <> 'completed'",
	} {
		if !strings.Contains(enforceRecurringDigitalHumanTaskStatusSQL, fragment) {
			t.Fatalf("recurring task constraint is missing %q", fragment)
		}
	}
}
