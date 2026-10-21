package migrations

import (
	"strings"
	"testing"
)

func TestChannelProcessingMarkerMigrationPersistsRecoverableLifecycle(t *testing.T) {
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS public.channel_processing_markers",
		"UNIQUE (inbound_event_id)",
		"'pending_add', 'adding', 'active', 'removing', 'removed', 'failed'",
		"idx_channel_processing_markers_claim",
		"provider_marker_id",
	} {
		if !strings.Contains(addChannelProcessingMarkersSQL, fragment) {
			t.Fatalf("migration is missing %q", fragment)
		}
	}
}
