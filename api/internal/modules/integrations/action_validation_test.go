package integrations

import (
	"fmt"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/tools"
)

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
