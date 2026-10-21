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

func TestAllOrganizationsEntryAdmissionSkipsTheAllowlist(t *testing.T) {
	policy := AllOrganizationsEntryAdmission()
	org := "00000000-0000-4000-8000-000000000123"
	for _, entry := range []struct{ application, entry string }{
		{"agent", "console"}, {"agent", "webapp"}, {"agent", "external-api"},
		{"digital_human", "console"}, {"digital_human", "webapp"}, {"digital_human", "external-api"},
		{"digital_human", "channel"}, {"digital_human", "task"},
	} {
		if !policy.Allows(org, entry.application, entry.entry) {
			t.Fatalf("%s:%s was refused for an authenticated organization", entry.application, entry.entry)
		}
	}
	if policy.Allows("", "digital_human", "console") {
		t.Fatal("an empty organization was admitted")
	}
	if policy.Allows(org, "agent", "channel") || policy.Allows(org, "workflow", "console") {
		t.Fatal("an unsupported entry was admitted")
	}
}
