package executionruntime

import "testing"

// One deployment setting admits every authenticated organization: in that mode
// the per-entry organization allowlist is not consulted, even when it is
// empty, explicit or malformed.
func TestEntryAdmissionFromEnvFollowsTheOrganizationMode(t *testing.T) {
	org := "00000000-0000-4000-8000-000000000123"
	for _, allowlist := range []string{"", "{}", `{"agent:webapp":["00000000-0000-4000-8000-000000000001"]}`, "not json"} {
		t.Setenv(OrganizationAccessModeEnv, "all_authenticated")
		t.Setenv(EntryAdmissionEnv, allowlist)
		entries, err := EntryAdmissionFromEnv()
		if err != nil {
			t.Fatalf("allowlist %q: %v", allowlist, err)
		}
		if !entries.Allows(org, "digital_human", "console") || !entries.Allows(org, "digital_human", "channel") {
			t.Fatalf("allowlist %q: DigitalHuman entries refused in all_authenticated mode", allowlist)
		}
		if entries.Allows("", "digital_human", "console") {
			t.Fatalf("allowlist %q: empty organization admitted", allowlist)
		}
	}

	t.Setenv(OrganizationAccessModeEnv, "allowlist")
	t.Setenv(EntryAdmissionEnv, "")
	entries, err := EntryAdmissionFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if entries.Allows(org, "digital_human", "console") || !entries.Allows(org, "agent", "console") {
		t.Fatal("allowlist mode no longer keeps the legacy console-only admission")
	}
	t.Setenv(EntryAdmissionEnv, "not json")
	if _, err := EntryAdmissionFromEnv(); err == nil {
		t.Fatal("allowlist mode accepted a malformed entry allowlist")
	}
}
