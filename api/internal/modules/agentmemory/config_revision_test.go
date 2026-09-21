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

func TestConfigRevisionIncludesDisabledSlots(t *testing.T) {
	base := []RuntimeSlot{{Key: "profile", Enabled: true}}
	withDisabled := append([]RuntimeSlot(nil), base...)
	withDisabled = append(withDisabled, RuntimeSlot{Key: "preference", Enabled: false, SortOrder: 1})

	baseRevision := ConfigRevision(true, false, base)
	if got := ConfigRevision(true, false, withDisabled); got == baseRevision {
		t.Fatalf("disabled slot kept revision %q", got)
	}
}
