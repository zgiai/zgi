package service

import (
	"reflect"
	"testing"
)

func TestRemovedOrganizationSkillIDsReturnsStableDisabledSet(t *testing.T) {
	got := removedOrganizationSkillIDs(
		[]string{"custom-b", "custom-a", "custom-a", "calculator"},
		[]string{"calculator", "custom-b"},
	)
	want := []string{"custom-a"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("removedOrganizationSkillIDs() = %#v, want %#v", got, want)
	}
}
