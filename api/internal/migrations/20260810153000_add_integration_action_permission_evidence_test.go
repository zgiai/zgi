package migrations

import (
	"strings"
	"testing"
)

func TestIntegrationActionPermissionEvidenceMigrationContract(t *testing.T) {
	sql := compactSQL(addIntegrationActionPermissionEvidenceSQL)
	for _, expected := range []string{
		"ADD COLUMN verified_action_ids jsonb NOT NULL DEFAULT '[]'::jsonb",
		"ADD COLUMN denied_action_ids jsonb NOT NULL DEFAULT '[]'::jsonb",
		"jsonb_array_length(verified_action_ids) <= 256",
		"jsonb_array_length(denied_action_ids) <= 256",
		"ADD COLUMN action_id varchar(128)",
		"CREATE INDEX idx_integration_connection_health_events_action_observed",
		"WHERE action_id IS NOT NULL",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("action permission evidence migration missing %q: %s", expected, sql)
		}
	}
}

func TestIntegrationActionPermissionEvidenceMigrationRemovesLegacyDingTalkClaims(t *testing.T) {
	sql := compactSQL(addIntegrationActionPermissionEvidenceSQL)
	for _, expected := range []string{
		"SET granted_scopes = '[]'::jsonb",
		"scope_status = 'unknown'",
		"scope_checked_at = NULL",
		"THEN '[\"dingtalk.department.list\"]'::jsonb",
		"WHERE integration_id = 'dingtalk'",
		"AND auth_method_id = 'organization_dingtalk_internal_app'",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("legacy DingTalk backfill missing %q: %s", expected, sql)
		}
	}
}
