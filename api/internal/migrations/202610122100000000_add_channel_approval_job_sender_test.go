package migrations

import (
	"strings"
	"testing"
)

func TestChannelApprovalJobSenderMigrationUpgradesExistingTable(t *testing.T) {
	if migration202610122100000000ID <= migration202610122000000000ID {
		t.Fatal("sender columns must be added after the approval job table migration")
	}
	if strings.Contains(addDigitalHumanChannelApprovalJobsSQL, "sender_external_id") {
		t.Fatal("the applied approval job table migration must stay unchanged")
	}
	for _, fragment := range []string{
		"ALTER TABLE public.digital_human_channel_approval_jobs",
		"ADD COLUMN IF NOT EXISTS sender_external_id varchar(255) NOT NULL DEFAULT ''",
		"ADD COLUMN IF NOT EXISTS conversation_type varchar(16) NOT NULL DEFAULT ''",
	} {
		if !strings.Contains(addChannelApprovalJobSenderSQL, fragment) {
			t.Fatalf("migration is missing %q", fragment)
		}
	}
}
