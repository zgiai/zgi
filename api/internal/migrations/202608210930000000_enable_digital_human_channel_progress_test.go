package migrations

import (
	"strings"
	"testing"
)

func TestEnableDigitalHumanChannelProgressMigrationAllowsOrderedStages(t *testing.T) {
	compact := strings.ToLower(strings.Join(strings.Fields(enableDigitalHumanChannelProgressSQL), " "))
	required := []string{
		"add column if not exists delivery_kind varchar(24) not null default 'final'",
		"add column if not exists delivery_key varchar(128) not null default 'final'",
		"add column if not exists sequence bigint not null default 9223372036854775807",
		"delivery_kind in ('acknowledgement', 'progress', 'final')",
		"drop index if exists public.idx_channel_delivery_outbox_run",
		"on public.channel_delivery_outbox (run_id, sequence, created_at)",
		"on public.channel_delivery_outbox (run_id, delivery_kind, delivery_key)",
	}
	for _, token := range required {
		if !strings.Contains(compact, token) {
			t.Errorf("migration is missing %q", token)
		}
	}
}
