package skills

import (
	"context"
	"strings"
	"testing"
)

func TestValidateCatalogSkipsRuntimeManagedToolsAndReportsEverySkill(t *testing.T) {
	// Without a tool manager every manager-provided skill fails validation,
	// which shows whether the validator stops at the first failure.
	err := NewRuntime(nil, nil).ValidateCatalog(context.Background())
	if err == nil {
		t.Fatal("expected validation errors without a tool manager")
	}
	message := err.Error()
	if strings.Contains(message, SkillDigitalHumanMemory) {
		t.Fatalf("runtime-managed skill tools were validated against the tool manager: %v", err)
	}
	if got := strings.Count(message, "tool manager is not configured"); got < 2 {
		t.Fatalf("validation stopped after %d skill(s): %v", got, err)
	}
}
