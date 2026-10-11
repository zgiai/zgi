package migrations

import (
	"strings"
	"testing"
)

func TestDigitalHumanRunsMigrationDefinesDurableReplaySafeQueue(t *testing.T) {
	compact := strings.ToLower(strings.Join(strings.Fields(createDigitalHumanRunsSQL), " "))
	required := []string{
		"create table if not exists public.digital_human_runs",
		"create table if not exists public.digital_human_run_outbox",
		"invocation_event jsonb not null",
		"run_spec jsonb not null",
		"execution_snapshot jsonb not null",
		"execution_trace jsonb not null default '{}'::jsonb",
		"unique index if not exists idx_digital_human_runs_idempotency",
		"unique index if not exists idx_digital_human_run_outbox_run",
		"cancel_requested_at timestamptz",
		"'unknown'",
	}
	for _, token := range required {
		if !strings.Contains(compact, token) {
			t.Errorf("migration is missing %q", token)
		}
	}
	for _, forbidden := range []string{"agent_id", "access_token", "api_key", "password", "cookie"} {
		if strings.Contains(compact, forbidden) {
			t.Errorf("migration contains forbidden token %q", forbidden)
		}
	}
}
