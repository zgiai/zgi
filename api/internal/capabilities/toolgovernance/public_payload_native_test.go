package toolgovernance

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestSanitizePublicValuePreservesSafeFactsAndRedactsSecrets(t *testing.T) {
	fakeSlackToken := "xoxb-" + strings.Repeat("1", 20)
	fakeAWSAccessKey := "AKIA" + strings.Repeat("1", 16)
	fakeGoogleAPIKey := "AIza" + strings.Repeat("1", 35)
	original := map[string]any{"kind": "tool_call", "runtime_source": "connector", "result": map[string]any{"operation": map[string]any{"connector_id": "github", "logical_operation_id": "server-operation"}, "outcome_status": "successful"}, "arguments": map[string]any{
		"title": "Safe title", "url": "https://example.com/?access_token=secret-value-123456", "body": "notify " + fakeSlackToken, "aws": fakeAWSAccessKey, "google": fakeGoogleAPIKey, "credentials": map[string]any{"password": "private-password"}, "connection_name": "model supplied label",
	}, "connector_action_instances": map[string]any{"private": "ledger"}, "governance": map[string]any{"frozen_invocation": map[string]any{"secret": "sealed input"}}}
	before, _ := json.Marshal(original)
	public := SanitizePublicValue(original).(map[string]any)
	encoded, _ := json.Marshal(public)
	for _, secret := range []string{"secret-value-123456", "xoxb-", "AKIA123", "AIza123", "private-password", "connector_action_instances", "frozen_invocation"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("private value escaped: %s", secret)
		}
	}
	if _, promoted := public["connection_name"]; promoted {
		t.Fatal("business argument became server presentation")
	}
	if !strings.Contains(string(encoded), "Safe title") || !strings.Contains(string(encoded), "server-operation") || !strings.Contains(string(encoded), "successful") {
		t.Fatal("lost safe native facts")
	}
	after, _ := json.Marshal(original)
	if string(before) != string(after) {
		t.Fatal("mutated server snapshot")
	}
}

func TestSanitizePublicValueKeepsOrdinaryToolsAndBoundsArguments(t *testing.T) {
	ordinary := map[string]any{"kind": "tool_call", "skill_id": "file-manager", "tool_name": "save_file", "status": "success", "arguments": map[string]any{"filename": "report.txt", "content": "safe content"}}
	if got := SanitizePublicValue(ordinary); !reflect.DeepEqual(got, ordinary) {
		t.Fatal("safe ordinary invocation changed")
	}
	fields := map[string]any{"items": strings.Repeat("x", 2000), "password": "sensitive"}
	frozen := FrozenInvocation{SkillID: "file-manager", ToolName: "save_file", Arguments: fields}
	output := SanitizePublicValue(frozen).(map[string]any)
	args := output["arguments"].(map[string]any)
	if args["password"] != publicToolArgumentRedacted || len([]rune(args["items"].(string))) > publicToolArgumentMaxRunes+1 {
		t.Fatal("unbounded or unsafe structured arguments")
	}
	if fields["password"] != "sensitive" {
		t.Fatal("mutated frozen input")
	}
}
