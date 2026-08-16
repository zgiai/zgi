package toolgovernance

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSanitizeExternalActionPublicValueRedactsSecretsAndKeepsServerValueDetached(t *testing.T) {
	connectionID := "33333333-3333-3333-3333-333333333333"
	original := map[string]interface{}{
		"skill_id":  "external-apps",
		"tool_name": "execute_action",
		"arguments": map[string]interface{}{
			"integration_id":       "github",
			"action_id":            "github.issue.create",
			"connection_id":        connectionID,
			"connection_name":      "Team GitHub",
			"connection_selection": "preferred",
			"arguments": map[string]interface{}{
				"title":       "Safe issue title",
				"url":         "https://example.com/callback?access_token=secret-value-123456",
				"body":        "notify xoxb-12345678901234567890",
				"aws":         "AKIA1234567890123456",
				"google":      "AIza12345678901234567890123456789012345",
				"credentials": map[string]interface{}{"password": "database-password"},
			},
		},
		"governance": map[string]interface{}{
			"assets": []interface{}{map[string]interface{}{
				"id": connectionID, "type": "integration_connection", "name": "Team GitHub",
			}},
			"frozen_invocation": map[string]interface{}{
				"skill_id": "external-apps", "tool_name": "execute_action", "provider_id": "external-integrations",
				"arguments": map[string]interface{}{
					"integration_id": "github", "action_id": "github.issue.create", "connection_id": connectionID,
					"connection_name": "Team GitHub", "connection_selection": "preferred",
				},
			},
		},
		"result": map[string]interface{}{
			"reference":                   "provider-" + connectionID,
			"key-" + connectionID:         "hidden",
			"unrelated_public_identifier": "visible",
		},
	}

	sanitized, ok := SanitizeExternalActionPublicValue(original).(map[string]interface{})
	if !ok {
		t.Fatalf("sanitized type = %T", sanitized)
	}
	raw, err := json.Marshal(sanitized)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(raw)
	for _, secret := range []string{"secret-value-123456", "xoxb-", "AKIA123", "AIza123", "database-password"} {
		if strings.Contains(encoded, secret) {
			t.Fatalf("public payload contains %q: %s", secret, encoded)
		}
	}
	if strings.Contains(encoded, connectionID) || strings.Contains(encoded, `"connection_id"`) {
		t.Fatalf("public payload exposed an internal connection UUID: %s", encoded)
	}
	if !strings.Contains(encoded, publicExternalHiddenReference) || strings.Contains(encoded, "redacted internal connection identifier") {
		t.Fatalf("public payload did not use the stable hidden-reference sentinel: %s", encoded)
	}
	if !strings.Contains(encoded, "Team GitHub") || !strings.Contains(encoded, `"connection_selection":"preferred"`) {
		t.Fatalf("public payload lost safe connection identity: %s", encoded)
	}
	if !strings.Contains(encoded, "Safe issue title") || !strings.Contains(encoded, publicExternalArgumentRedacted) {
		t.Fatalf("public payload lost safe context or redaction marker: %s", encoded)
	}
	originalArguments := original["arguments"].(map[string]interface{})["arguments"].(map[string]interface{})
	if !strings.Contains(originalArguments["url"].(string), "secret-value-123456") {
		t.Fatal("sanitizer mutated the authoritative server-side arguments")
	}
	if original["arguments"].(map[string]interface{})["connection_id"] != connectionID {
		t.Fatal("sanitizer mutated the authoritative server-side connection identity")
	}
}

func TestSanitizeExternalActionPublicValueHandlesFrozenInvocationStruct(t *testing.T) {
	connectionID := "33333333-3333-3333-3333-333333333333"
	frozen := &FrozenInvocation{
		SkillID: "external-apps", ToolName: "execute_action", ProviderID: "external-integrations",
		Arguments: map[string]interface{}{
			"integration_id": "github", "action_id": "github.issue.create", "connection_id": connectionID,
			"connection_name": "Team GitHub", "connection_selection": "preferred",
			"arguments": map[string]interface{}{"apiKey": "sk-1234567890abcdefghijklmnop", "title": "safe"},
		},
	}
	sanitized := SanitizeExternalActionPublicValue(map[string]interface{}{"frozen_invocation": frozen})
	raw, err := json.Marshal(sanitized)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "sk-123456") || strings.Contains(string(raw), connectionID) ||
		strings.Contains(string(raw), `"connection_id"`) || !strings.Contains(string(raw), publicExternalArgumentRedacted) {
		t.Fatalf("sanitized frozen invocation = %s", raw)
	}
	if frozen.Arguments["connection_id"] != connectionID {
		t.Fatal("public sanitization mutated the server frozen invocation")
	}
}

func TestSanitizeExternalActionPublicValueRedactsBatchItemsAndHidesBatchIdentity(t *testing.T) {
	original := map[string]interface{}{
		"skill_id": "external-apps", "tool_name": "execute_action",
		"arguments": map[string]interface{}{
			"integration_id": "feishu", "action_id": "feishu.message.send_user",
			"batch_items": []interface{}{
				map[string]interface{}{"recipient_id": "ou_safe", "text": "hello"},
				map[string]interface{}{"recipient_id": "ou_safe", "text": "Bearer secret-value-1234567890"},
			},
			"operation_batch": map[string]interface{}{
				"batch_id":           "batch-0123456789abcdef01234567",
				"operation_item_ids": []interface{}{"item-001-0123456789abcdef", "item-002-fedcba9876543210"},
				"item_count":         2, "frozen_items_digest": strings.Repeat("a", 64),
			},
		},
	}
	sanitized := SanitizeExternalActionPublicValue(original)
	raw, err := json.Marshal(sanitized)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(raw)
	for _, forbidden := range []string{"secret-value", "operation_batch", "batch-0123", "item-001"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("public batch payload contains %q: %s", forbidden, encoded)
		}
	}
	if !strings.Contains(encoded, "ou_safe") || !strings.Contains(encoded, publicExternalArgumentRedacted) {
		t.Fatalf("public batch payload lost safe summary or redaction marker: %s", encoded)
	}
	if _, exists := original["arguments"].(map[string]interface{})["operation_batch"]; !exists {
		t.Fatal("sanitizer mutated authoritative batch identity")
	}
}

func TestSanitizeExternalActionPublicValueDoesNotPromoteBusinessArgumentAsConnectionLabel(t *testing.T) {
	connectionID := "33333333-3333-3333-3333-333333333333"
	sanitized := SanitizeExternalActionPublicValue(map[string]interface{}{
		"skill_id": "external-apps", "tool_name": "execute_action",
		"arguments": map[string]interface{}{
			"integration_id": "github", "action_id": "github.issue.create", "connection_id": connectionID,
			"arguments": map[string]interface{}{"connection_name": "Spoofed account label"},
		},
	}).(map[string]interface{})
	if _, promoted := sanitized["connection_name"]; promoted {
		t.Fatalf("public payload promoted an untrusted action argument: %#v", sanitized)
	}
	raw, _ := json.Marshal(sanitized)
	if strings.Contains(string(raw), connectionID) {
		t.Fatalf("public payload exposed internal connection UUID: %s", raw)
	}
}

func TestSanitizeExternalActionPublicValueDoesNotRewriteUnrelatedInvocation(t *testing.T) {
	original := map[string]interface{}{
		"skill_id": "file-manager", "tool_name": "delete_file",
		"arguments": map[string]interface{}{"arguments": map[string]interface{}{"token": "visible-by-existing-file-governance-contract"}},
	}
	sanitized := SanitizeExternalActionPublicValue(original).(map[string]interface{})
	raw, _ := json.Marshal(sanitized)
	if !strings.Contains(string(raw), "visible-by-existing-file-governance-contract") {
		t.Fatalf("unrelated invocation was rewritten: %s", raw)
	}
}
