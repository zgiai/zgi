package migrations

import (
	"strings"
	"testing"
)

func TestScheduledChannelDeliveryMigrationKeepsReplyScopeStrict(t *testing.T) {
	for _, fragment := range []string{
		"NEW.inbound_event_id IS NOT NULL",
		"run_origin <> 'schedule'",
		"run_reply_installation",
		"run_reply_conversation",
		"scheduled channel delivery reply address mismatch",
		"enforce_channel_delivery_scope_consistency",
	} {
		if !strings.Contains(allowScheduledChannelDeliveriesSQL, fragment) {
			t.Fatalf("migration is missing %q", fragment)
		}
	}
}
