package migrations

import (
	"strings"
	"testing"
)

func TestChannelApprovalJobMigrationPersistsRecoverableWork(t *testing.T) {
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS public.digital_human_channel_approval_jobs",
		"UNIQUE (inbound_event_id)",
		"'pending', 'running', 'succeeded', 'failed'",
		"idx_digital_human_channel_approval_jobs_claim",
		"text text NOT NULL",
		"sender_external_id varchar(255)",
		"conversation_type varchar(16)",
	} {
		if !strings.Contains(addDigitalHumanChannelApprovalJobsSQL, fragment) {
			t.Fatalf("migration is missing %q", fragment)
		}
	}
}
