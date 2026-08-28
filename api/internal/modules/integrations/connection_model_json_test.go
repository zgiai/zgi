package integrations

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
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

func TestConnectionViewDoesNotExposeSetupActorID(t *testing.T) {
	actorID := uuid.New()
	payload, err := json.Marshal(newConnectionView(&IntegrationConnection{SetupCompletedBy: &actorID}))
	if err != nil {
		t.Fatalf("marshal connection view: %v", err)
	}
	if string(payload) == "" || containsJSONField(payload, "setup_completed_by") {
		t.Fatalf("connection view exposed the internal setup actor: %s", payload)
	}
}

func containsJSONField(payload []byte, field string) bool {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(payload, &document); err != nil {
		return true
	}
	_, exists := document[field]
	return exists
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
