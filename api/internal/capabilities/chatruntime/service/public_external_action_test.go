package service

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPublicExternalActionStreamEventRedactsArgumentsWithoutMutatingStoredEvent(t *testing.T) {
	connectionID := "33333333-3333-3333-3333-333333333333"
	original := StreamEvent{EventType: streamEventToolGovernanceDecision, Payload: map[string]interface{}{
		"skill_id": "external-apps", "tool_name": "execute_action", "correlation_id": "corr-1",
		"arguments": map[string]interface{}{
			"integration_id": "github", "action_id": "github.issue.create", "connection_id": connectionID,
			"connection_name": "Team GitHub", "connection_selection": "preferred",
			"arguments": map[string]interface{}{"body": "xoxb-12345678901234567890", "title": "safe"},
		},
		"governance": map[string]interface{}{"assets": []interface{}{map[string]interface{}{
			"id": connectionID, "type": "integration_connection", "name": "Team GitHub",
		}}},
	}}
	public := publicExternalActionStreamEvent(original)
	publicJSON, _ := json.Marshal(public.Payload)
	if strings.Contains(string(publicJSON), "xoxb-") || strings.Contains(string(publicJSON), connectionID) ||
		strings.Contains(string(publicJSON), `"connection_id"`) || !strings.Contains(string(publicJSON), "__zgi_redacted__") {
		t.Fatalf("public event = %s", publicJSON)
	}
	if !strings.Contains(string(publicJSON), `"correlation_id":"corr-1"`) ||
		!strings.Contains(string(publicJSON), `"connection_name":"Team GitHub"`) {
		t.Fatalf("public event lost continuation or safe display identity: %s", publicJSON)
	}
	storedJSON, _ := json.Marshal(original.Payload)
	if !strings.Contains(string(storedJSON), "xoxb-") || !strings.Contains(string(storedJSON), connectionID) {
		t.Fatal("public projection mutated the authoritative stored event")
	}
}

func TestClientVisibleMessageMetadataRedactsExternalInvocationHistory(t *testing.T) {
	connectionID := "33333333-3333-3333-3333-333333333333"
	metadata := map[string]interface{}{"skill_invocations": []interface{}{map[string]interface{}{
		"kind": "tool_governance", "skill_id": "external-apps", "tool_name": "execute_action",
		"governance": map[string]interface{}{"frozen_invocation": map[string]interface{}{
			"skill_id": "external-apps", "tool_name": "execute_action", "provider_id": "external-integrations",
			"arguments": map[string]interface{}{
				"integration_id": "github", "action_id": "github.issue.create", "connection_id": connectionID,
				"connection_name": "Team GitHub", "connection_selection": "preferred",
				"arguments": map[string]interface{}{"url": "https://example.com?access_token=secret-value-123456"},
			},
		}},
	}}}
	visible := clientVisibleMessageMetadata(metadata)
	visibleJSON, _ := json.Marshal(visible)
	if strings.Contains(string(visibleJSON), "secret-value-123456") || strings.Contains(string(visibleJSON), connectionID) ||
		strings.Contains(string(visibleJSON), `"connection_id"`) || !strings.Contains(string(visibleJSON), "__zgi_redacted__") {
		t.Fatalf("visible metadata = %s", visibleJSON)
	}
	storedJSON, _ := json.Marshal(metadata)
	if !strings.Contains(string(storedJSON), "secret-value-123456") || !strings.Contains(string(storedJSON), connectionID) {
		t.Fatal("public metadata projection mutated server state")
	}
}
