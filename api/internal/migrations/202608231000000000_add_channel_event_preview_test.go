package migrations

import (
	"strings"
	"testing"
)

func TestAddChannelEventPreviewMigration(t *testing.T) {
	compact := strings.ToLower(strings.Join(strings.Fields(addChannelEventPreviewSQL), " "))
	for _, required := range []string{
		"add column if not exists conversation_type varchar(16) not null default ''",
		"add column if not exists content_preview varchar(512) not null default ''",
		"add column if not exists mention_only boolean not null default false",
	} {
		if !strings.Contains(compact, required) {
			t.Errorf("migration is missing %q", required)
		}
	}
}
