package integrations

import (
	"encoding/json"
	"testing"
)

func TestConnectionViewSerializesEmptyCollectionsAsArrays(t *testing.T) {
	view := newConnectionView(&IntegrationConnection{})

	payload, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal connection view: %v", err)
	}

	var document map[string]json.RawMessage
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatalf("unmarshal connection view: %v", err)
	}
	for _, field := range []string{"granted_scopes", "missing_required_scopes"} {
		if got := string(document[field]); got != "[]" {
			t.Fatalf("%s = %s, want []", field, got)
		}
	}
}

func TestConnectionPermissionSummarySerializesEmptyCollectionsAsArrays(t *testing.T) {
	summary := BuildConnectionPermissionSummary(
		&IntegrationConnection{},
		ProviderDefinition{},
	)

	payload, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal permission summary: %v", err)
	}

	var document map[string]json.RawMessage
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatalf("unmarshal permission summary: %v", err)
	}
	for _, field := range []string{
		"adapted_capabilities",
		"identity_permissions",
		"lifecycle_permissions",
		"provider_permissions",
		"unknown_permissions",
		"missing_permissions",
	} {
		if got := string(document[field]); got != "[]" {
			t.Fatalf("%s = %s, want []", field, got)
		}
	}
}
