package migrations

import (
	"strings"
	"testing"
)

func TestDigitalHumanTaskMigrationKeepsAutomationAndSubjectLifecycleSeparate(t *testing.T) {
	for _, fragment := range []string{
		"task_kind varchar(32) NOT NULL DEFAULT 'automation'",
		"work_status varchar(32)",
		"CREATE TABLE IF NOT EXISTS public.automation_task_activities",
		"CREATE TABLE IF NOT EXISTS public.digital_human_channel_task_policies",
		"idx_automation_action_runs_task_action_unique",
		"external_run_id uuid",
	} {
		if !strings.Contains(addDigitalHumanTasksSQL, fragment) {
			t.Fatalf("migration is missing %q", fragment)
		}
	}
}
