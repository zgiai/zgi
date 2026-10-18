package connectorfinalize

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestDigitalHumanPreservationProof(t *testing.T) {
	const org, workspace, installation, legacy = "org-1", "ws-1", "inst-1", "legacy-connection"
	owned := func(connection string) row {
		return row{"id": installation, "organization_id": org, "workspace_id": workspace, "provider": "feishu", "connection_id": connection, "deleted_at": nil}
	}
	credential := row{"installation_id": installation, "organization_id": org, "workspace_id": workspace, "provider": "feishu",
		"credential_version": json.Number("1"), "encrypted_credentials": "v2.native.sealed"}
	delivery := func(connection string) row {
		return row{"installation_id": installation, "organization_id": org, "workspace_id": workspace, "connection_id": connection}
	}
	codes := func(data snapshot) []string {
		var report Report
		validateDigitalHumanPreservation(data, &report)
		result := []string{}
		for _, issue := range report.Issues {
			result = append(result, issue.Code)
		}
		return result
	}
	tests := []struct {
		name string
		data snapshot
		want []string
	}{
		{"preserved", snapshot{"channel_installations": {owned(installation)}, "channel_installation_credentials": {credential}, "channel_delivery_outbox": {delivery(installation)}}, []string{}},
		{"not preserved", snapshot{"channel_installations": {owned(legacy)}, "channel_delivery_outbox": {delivery(legacy)}}, []string{"installation_credential_preservation_required"}},
		{"stale delivery locator", snapshot{"channel_installations": {owned(installation)}, "channel_installation_credentials": {credential}, "channel_delivery_outbox": {delivery(legacy)}}, []string{"delivery_connection_locator_stale"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := codes(tt.data); !slices.Equal(got, tt.want) {
				t.Fatalf("issues = %v, want %v", got, tt.want)
			}
		})
	}
}
