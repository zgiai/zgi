package handler

import (
	"testing"

	automationmodel "github.com/zgiai/ginext/internal/modules/automation/model"
)

func TestParseStatusesIgnoresAllSentinel(t *testing.T) {
	statuses := parseStatuses("all")
	if len(statuses) != 0 {
		t.Fatalf("expected all sentinel to disable status filtering, got %#v", statuses)
	}
}

func TestParseStatusesKeepsConcreteStatuses(t *testing.T) {
	statuses := parseStatuses("active, paused, all")
	expected := []automationmodel.AutomationTaskStatus{"active", "paused"}

	if len(statuses) != len(expected) {
		t.Fatalf("expected %d statuses, got %#v", len(expected), statuses)
	}
	for index, value := range expected {
		if statuses[index] != value {
			t.Fatalf("unexpected status at %d: got %q want %q", index, statuses[index], value)
		}
	}
}
