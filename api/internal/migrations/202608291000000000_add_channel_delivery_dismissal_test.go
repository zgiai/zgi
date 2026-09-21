package migrations

import (
	"strings"
	"testing"
)

func TestChannelDeliveryDismissalMigrationKeepsTerminalAuditEvidence(t *testing.T) {
	for _, fragment := range []string{
		"ADD COLUMN dismissed_at timestamptz",
		"ADD COLUMN dismissed_by uuid REFERENCES public.accounts(id)",
		"'dismissed'",
		"status = 'dismissed' AND dismissed_at IS NOT NULL AND dismissed_by IS NOT NULL",
	} {
		if !strings.Contains(addChannelDeliveryDismissalSQL, fragment) {
			t.Fatalf("migration is missing %q", fragment)
		}
	}
}
