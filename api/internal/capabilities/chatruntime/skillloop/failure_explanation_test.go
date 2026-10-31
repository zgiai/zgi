package skillloop

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestToolFailureExplanationOnlyDescribesProviderRequestStateWhenObserved(t *testing.T) {
	unknown := buildToolFailureExplanationEvidenceWithRuntimeState(skills.SkillTrace{
		Kind: "tool_call", Status: "error", Error: "provider failed", Arguments: map[string]interface{}{},
	}, nil, "failed", nil)
	if unknown.ProviderRequestSent != nil {
		t.Fatalf("unknown provider request state = %#v, want omitted", unknown.ProviderRequestSent)
	}

	preflight := buildToolFailureExplanationEvidenceWithRuntimeState(skills.SkillTrace{
		Kind: "tool_call", Status: "error", Error: "integration_invalid_input",
		Arguments: map[string]interface{}{
			"reason_code":           "action_arguments_schema_mismatch",
			"provider_request_sent": false,
		},
	}, nil, "failed", nil)
	if preflight.ProviderRequestSent == nil || *preflight.ProviderRequestSent {
		t.Fatalf("preflight provider request state = %#v, want explicit false", preflight.ProviderRequestSent)
	}
}
