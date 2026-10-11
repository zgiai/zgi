package agentexecution

import (
	"testing"

	"github.com/google/uuid"
)

func TestEntryAdmissionDefaultsAndExplicitOrganizationAllowlist(t *testing.T) {
	organization := uuid.NewString()
	policy, err := ParseEntryAdmission("")
	if err != nil || !policy.Allows(organization, "agent", "console") || policy.Allows(organization, "agent", "webapp") || policy.Allows(organization, "digital_human", "console") {
		t.Fatal("default must enable only Agent console")
	}
	policy, err = ParseEntryAdmission(`{"agent:webapp":["` + organization + `"]}`)
	if err != nil || !policy.Allows(organization, "agent", "webapp") || policy.Allows(uuid.NewString(), "agent", "webapp") || policy.Allows(organization, "agent", "console") {
		t.Fatal("explicit allowlist leaked across organizations or entries")
	}
	for _, raw := range []string{`{}`, `null`, `[]`, `{"agent:task":[]}`, `{"runtime_workspace:console":[]}`, `{"agent:webapp":["*"]}`, `{"agent:webapp":["` + organization + `","` + organization + `"]}`} {
		policy, _ := ParseEntryAdmission(raw)
		if policy.Allows(organization, "agent", "console") || policy.Allows(organization, "agent", "webapp") {
			t.Fatalf("invalid/empty allowlist admitted execution: %s", raw)
		}
	}
}
