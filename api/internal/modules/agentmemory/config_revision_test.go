package agentmemory

import "testing"

func TestConfigRevisionNormalizesLegacySlotShape(t *testing.T) {
	legacy := []RuntimeSlot{{
		Key:         " PROFILE ",
		Name:        " Profile ",
		Description: " Stable user profile ",
		Enabled:     true,
	}}
	canonical := []RuntimeSlot{{
		Key:         "profile",
		Name:        "Profile",
		Description: "Stable user profile",
		MaxChars:    2000,
		Enabled:     true,
	}}

	got := ConfigRevision(true, false, legacy)
	want := ConfigRevision(true, false, canonical)
	if got != want {
		t.Fatalf("legacy revision = %q, want %q", got, want)
	}
}

func TestConfigRevisionKeepsRevisionsOfSlotsWithoutWriteRules(t *testing.T) {
	// The revision of a slot without write rules must not change, or every
	// published Agent's memory fence would stop matching.
	plain := []RuntimeSlot{{Key: "profile", Name: "Profile", Description: "Stable user profile", MaxChars: 2000, Enabled: true}}
	if got := ConfigRevision(true, false, plain); got != "a42104b798f466e34fa26fd6aaccd78850fd77dc7dc13d7430c72e91286b75c6" {
		t.Fatalf("revision of a plain slot changed: %s", got)
	}
	readOnly := []RuntimeSlot{{Key: "profile", Name: "Profile", Description: "Stable user profile", MaxChars: 2000, Enabled: true, ReadOnly: true}}
	manualOnly := []RuntimeSlot{{Key: "profile", Name: "Profile", Description: "Stable user profile", MaxChars: 2000, Enabled: true, ManualOnly: true}}
	if ConfigRevision(true, false, readOnly) == ConfigRevision(true, false, plain) || ConfigRevision(true, false, manualOnly) == ConfigRevision(true, false, plain) {
		t.Fatal("write rules do not change the revision")
	}
}

func TestConfigRevisionIncludesDisabledSlots(t *testing.T) {
	base := []RuntimeSlot{{Key: "profile", Enabled: true}}
	withDisabled := append([]RuntimeSlot(nil), base...)
	withDisabled = append(withDisabled, RuntimeSlot{Key: "preference", Enabled: false, SortOrder: 1})

	baseRevision := ConfigRevision(true, false, base)
	if got := ConfigRevision(true, false, withDisabled); got == baseRevision {
		t.Fatalf("disabled slot kept revision %q", got)
	}
}
