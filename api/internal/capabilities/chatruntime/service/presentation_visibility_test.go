package service

import "testing"

func TestInvocationPresentationVisibilityMarksAssetObservations(t *testing.T) {
	cases := []struct {
		name       string
		invocation map[string]interface{}
		want       string
	}{
		{"action type", map[string]interface{}{"kind": "client_action", "action_type": "asset_observation"}, "internal"},
		{"action id", map[string]interface{}{"kind": "client_action", "action_id": "asset_observation:corr-1"}, "internal"},
		{"runtime id", map[string]interface{}{"kind": "client_action", "runtime_id": "client_action:asset_observation:corr-1"}, "internal"},
		{"page navigation", map[string]interface{}{"kind": "client_action", "action_type": "route_navigation"}, ""},
		{"tool call", map[string]interface{}{"kind": "tool_call", "action_type": "asset_observation"}, ""},
		{"already marked", map[string]interface{}{"kind": "tool_governance", "presentation_visibility": "internal"}, "internal"},
	}
	for _, tc := range cases {
		if got := invocationPresentationVisibility(tc.invocation); got != tc.want {
			t.Fatalf("%s: visibility = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestClientVisibleMessageMetadataMarksStoredAssetObservations(t *testing.T) {
	metadata := map[string]interface{}{
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":        "client_action",
				"action_type": "asset_observation",
				"action_id":   "asset_observation:corr-1",
				"status":      "succeeded",
			},
			map[string]interface{}{
				"kind":      "tool_call",
				"skill_id":  "agent-management",
				"tool_name": "update_agent_identity",
				"status":    "success",
			},
		},
	}

	visible := ClientVisibleMessageMetadata(metadata)
	invocations := skillInvocationsFromMetadata(visible["skill_invocations"])
	if len(invocations) != 2 {
		t.Fatalf("visible skill_invocations = %#v, want both records", invocations)
	}
	if got := stringFromAny(invocations[0]["presentation_visibility"]); got != "internal" {
		t.Fatalf("observation visibility = %q, want internal", got)
	}
	if got := invocations[1]["presentation_visibility"]; got != nil {
		t.Fatalf("tool call visibility = %#v, want none", got)
	}
	stored := skillInvocationsFromMetadata(metadata["skill_invocations"])
	if stored[0]["presentation_visibility"] != nil {
		t.Fatal("the stored metadata was changed")
	}
}

func TestCopyInvocationRuntimeFieldsCarriesPresentationVisibility(t *testing.T) {
	payload := map[string]interface{}{"action_type": "asset_observation"}
	copyInvocationRuntimeFields(payload, map[string]interface{}{
		"kind":                    "client_action",
		"runtime_id":              "client_action:asset_observation:corr-1",
		"presentation_visibility": "internal",
	})
	if payload["presentation_visibility"] != "internal" {
		t.Fatalf("payload = %#v, want the visibility the client reads", payload)
	}
}
