package migrations

import (
	"strings"
	"testing"
)

func TestPostConnectorBillingAttemptGrantVersionMigrationContract(t *testing.T) {
	up := strings.ToUpper(addBillingAttemptGrantVersionSQL)
	if !strings.Contains(up, "ADD COLUMN IF NOT EXISTS GRANT_AUTHORIZATION_VERSION BIGINT") {
		t.Fatalf("up migration must add grant authorization version: %s", addBillingAttemptGrantVersionSQL)
	}
	down := strings.ToUpper(rollbackBillingAttemptGrantVersionSQL)
	if !strings.Contains(down, "DROP COLUMN IF EXISTS GRANT_AUTHORIZATION_VERSION") {
		t.Fatalf("down migration must drop grant authorization version: %s", rollbackBillingAttemptGrantVersionSQL)
	}
}
