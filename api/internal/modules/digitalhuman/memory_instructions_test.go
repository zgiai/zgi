package digitalhuman

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

func TestValidateMemoryPolicyBoundsInstructions(t *testing.T) {
	for name, policy := range map[string]map[string]any{
		"none":       {"enabled": true},
		"empty":      {"instructions": ""},
		"at limit":   {"instructions": strings.Repeat("记", maxMemoryInstructionsRunes)},
		"null value": {"instructions": nil},
	} {
		if err := validateMemoryPolicy(policy); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	for name, policy := range map[string]map[string]any{
		"too long":   {"instructions": strings.Repeat("记", maxMemoryInstructionsRunes+1)},
		"not a text": {"instructions": []string{"a"}},
	} {
		if err := validateMemoryPolicy(policy); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
	cleared := map[string]any{"enabled": true, "instructions": "  "}
	if err := validateMemoryPolicy(cleared); err != nil || len(cleared) != 1 {
		t.Fatalf("cleared instructions stayed in the policy: %v, %v", cleared, err)
	}
	trimmed := map[string]any{"instructions": "  Keep complaints.  "}
	if err := validateMemoryPolicy(trimmed); err != nil || trimmed["instructions"] != "Keep complaints." {
		t.Fatalf("instructions were not trimmed: %v, %v", trimmed, err)
	}
}

func TestMemoryInstructionsReachTheExtractionPromptBelowTheRules(t *testing.T) {
	if got := memoryInstructionsFromPolicy(datatypes.JSON(`{"enabled":true,"instructions":"  Remember complaints.  "}`)); got != "Remember complaints." {
		t.Fatalf("instructions = %q", got)
	}
	if got := memoryInstructionsFromPolicy(datatypes.JSON(`{}`)); got != "" || renderMemoryInstructions(got) != "" {
		t.Fatal("a policy without instructions rendered some")
	}
	input := MemoryExtractionInput{
		Job: &MemoryExtractionJob{ConversationID: ptrUUID(uuid.New())}, Run: &Run{ID: uuid.New()},
		Source:       MemoryExtractionSource{Query: "q", Answer: "a"},
		Instructions: "Keep customer complaints; ignore small talk.",
	}
	prompt, err := memoryExtractionPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	rules := strings.Index(prompt, "Never propose identity changes")
	instructions := strings.Index(prompt, "<memory_instructions>\nKeep customer complaints; ignore small talk.\n</memory_instructions>")
	if rules < 0 || instructions < rules || !strings.Contains(prompt, "cannot override the rules above") {
		t.Fatalf("instructions are not below the fixed rules:\n%s", prompt)
	}
	input.Instructions = ""
	if prompt, _ := memoryExtractionPrompt(input); strings.Contains(prompt, "memory_instructions") {
		t.Fatal("an empty instruction added a section")
	}
}
