package main

import "testing"

func TestRevokedBindingsCannotBeResurrectedByHostSnapshot(t *testing.T) {
	source := fixture(t)
	source["agent_resource_bindings"][0]["revoked_at"] = "2026-09-01T00:00:00Z"
	plan, err := makePlan(source, testKeys())
	if err != nil {
		t.Fatal(err)
	}
	converted := applyMemory(t, source, plan)
	for _, binding := range converted["connector_target_bindings"] {
		if text(binding, "status") == "active" {
			t.Fatal("revoked binding became active")
		}
	}
	if len(list(object(converted["agents_configs"][0]["agent_mode"])["connector_bindings"])) != 0 {
		t.Fatal("revoked binding survived in host snapshot")
	}
	if result := verifySnapshot(converted, testKeys()); len(result.Issues) > 0 {
		t.Fatalf("%#v", result.Issues)
	}
}

func TestConstraintIntersectionKeepsDenialsAndRejectsMalformedBoundary(t *testing.T) {
	a := ownerConstraint(ownerID, []string{"github.user.get"})
	a["deny_reasons"] = []string{"explicit-denial"}
	b := ownerConstraint(ownerID, []string{"github.user.get"})
	result := intersectConstraints(a, b)
	if len(stringsOf(result["deny_reasons"])) != 1 {
		t.Fatal("explicit denial lost")
	}
	b["target_boundaries"] = []any{row{"type": "agent", "id": "*", "scope": "*", "version_id": "*", "unknown_scope": true}}
	if constraintValid(b, ownerID) {
		t.Fatal("unrecognized restriction accepted")
	}
}
