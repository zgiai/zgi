package migrations

import (
	"strings"
	"testing"
)

func TestPostConnectorBillingAttemptPrincipalAttributionRollbackPreservesGrantCascade(t *testing.T) {
	upper := strings.ToUpper(rollbackBillingAttemptPrincipalAttributionSQL)
	if strings.Contains(upper, "ON DELETE RESTRICT") {
		t.Fatal("rollback must not restore the access-grant foreign key as RESTRICT")
	}
	if !strings.Contains(upper, "REFERENCES PUBLIC.LLM_DEVELOPER_ACCESS_GRANTS(ID) ON DELETE CASCADE") {
		t.Fatal("rollback must preserve the developer-access CASCADE contract")
	}
}
