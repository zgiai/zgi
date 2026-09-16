package service

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPublicToolStreamEventRedactsWithoutMutatingStoredEvent(t *testing.T) {
	fakeSlackToken := "xoxb-" + strings.Repeat("1", 20)
	original := StreamEvent{EventType: streamEventToolGovernanceDecision, Payload: map[string]any{"skill_id": "file-manager", "tool_name": "save_file", "correlation_id": "corr-1", "arguments": map[string]any{"filename": "safe.txt", "content": fakeSlackToken}, "governance": map[string]any{"frozen_invocation": map[string]any{"private": "input"}}}}
	public := publicToolStreamEvent(original)
	encoded, _ := json.Marshal(public.Payload)
	if strings.Contains(string(encoded), "xoxb-") || strings.Contains(string(encoded), "frozen_invocation") || !strings.Contains(string(encoded), "corr-1") || !strings.Contains(string(encoded), "safe.txt") {
		t.Fatal("invalid public tool projection")
	}
	stored, _ := json.Marshal(original.Payload)
	if !strings.Contains(string(stored), fakeSlackToken) || !strings.Contains(string(stored), "frozen_invocation") {
		t.Fatal("mutated durable event")
	}
}

func TestClientVisibleMessageMetadataHidesPrivateConnectorLedger(t *testing.T) {
	metadata := map[string]any{"connector_action_instances": map[string]any{"private": "ledger"}, "skill_invocations": []any{map[string]any{"kind": "tool_call", "runtime_source": "connector", "result": map[string]any{"state": "completed", "outcome_id": "public-outcome", "outcome_status": "successful"}}}}
	public := clientVisibleMessageMetadata(metadata)
	encoded, _ := json.Marshal(public)
	if strings.Contains(string(encoded), "connector_action_instances") || !strings.Contains(string(encoded), "public-outcome") {
		t.Fatal("invalid native metadata projection")
	}
	if metadata["connector_action_instances"] == nil {
		t.Fatal("mutated replay ledger")
	}
}
