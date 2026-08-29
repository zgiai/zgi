package integrations

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestCanonicalizeActionInputUnifiesExplicitSelfOperationTarget(t *testing.T) {
	action := ActionDefinition{
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"recipient_type": map[string]interface{}{"type": "string"},
				"recipient_id": map[string]interface{}{
					"type":               "string",
					"x-zgi-discard-when": map[string]interface{}{"argument": "recipient_type", "equals": "self"},
				},
				"text": map[string]interface{}{"type": "string"},
			},
			"required": []interface{}{"recipient_type", "text"},
			"allOf": []interface{}{map[string]interface{}{
				"if": map[string]interface{}{
					"properties": map[string]interface{}{"recipient_type": map[string]interface{}{"const": "self"}},
					"required":   []interface{}{"recipient_type"},
				},
				"else": map[string]interface{}{"required": []interface{}{"recipient_id"}},
			}},
		},
		SuccessDeduplication: &SuccessDeduplicationDefinition{
			TargetArgumentPaths: []string{"recipient_id", "recipient_type"},
		},
	}
	first := CanonicalizeActionInput(action, map[string]interface{}{
		"recipient_type": "self", "recipient_id": "ignored-a", "text": "hello",
	})
	second := CanonicalizeActionInput(action, map[string]interface{}{
		"recipient_type": "self", "recipient_id": "ignored-b", "text": "hello",
	})
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("canonical self operations differ: first=%#v second=%#v", first, second)
	}
	if _, exists := first["recipient_id"]; exists {
		t.Fatalf("canonical self operation retained irrelevant recipient_id: %#v", first)
	}
	nonSelf := CanonicalizeActionInput(action, map[string]interface{}{
		"recipient_type": "open_id", "recipient_id": "ou_target", "text": "hello",
	})
	if nonSelf["recipient_id"] != "ou_target" {
		t.Fatalf("canonical non-self operation lost recipient_id: %#v", nonSelf)
	}

	delete(action.InputSchema, "allOf")
	unauthorized := CanonicalizeActionInput(action, map[string]interface{}{
		"recipient_type": "self", "recipient_id": "must-remain", "text": "hello",
	})
	if unauthorized["recipient_id"] != "must-remain" {
		t.Fatalf("discard rule without a matching conditional schema changed the target: %#v", unauthorized)
	}
}

func TestValidateActionInputReturnsSafeRecoverableFeedback(t *testing.T) {
	action := testAction("feishu.contact.search", "search_contacts")
	action.SchemaRevision = "schema-v1"
	rawSecret := "Bearer secret-should-never-appear"

	err := ValidateActionInput("feishu", action, map[string]interface{}{
		"name": rawSecret,
	})
	if err == nil {
		t.Fatal("ValidateActionInput() error = nil, want schema mismatch")
	}
	if ErrorCode(err) != ErrorCodeInvalidInput {
		t.Fatalf("ErrorCode() = %q, want %q", ErrorCode(err), ErrorCodeInvalidInput)
	}
	feedback := ActionInputValidationFeedback(err)
	if feedback["reason_code"] != ActionValidationReasonSchemaMismatch ||
		feedback["failure_stage"] != ActionValidationStagePreflight ||
		feedback["provider_request_sent"] != false {
		t.Fatalf("feedback = %#v", feedback)
	}
	issues, ok := feedback["argument_errors"].([]tools.JSONSchemaValidationIssue)
	if !ok || len(issues) == 0 || issues[0].Path != "query" {
		t.Fatalf("argument_errors = %#v, want required query issue", feedback["argument_errors"])
	}
	encoded := fmt.Sprint(feedback)
	for _, forbidden := range []string{rawSecret, "secret-should-never-appear"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("safe feedback exposed %q in %q", forbidden, encoded)
		}
	}
}

func TestValidateActionInputAcceptsCurrentSchema(t *testing.T) {
	action := testAction("feishu.contact.search", "search_contacts")
	if err := ValidateActionInput("feishu", action, map[string]interface{}{"query": "Yang"}); err != nil {
		t.Fatalf("ValidateActionInput() error = %v", err)
	}
}
