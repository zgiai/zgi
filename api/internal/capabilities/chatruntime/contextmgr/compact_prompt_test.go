package contextmgr

import (
	"strings"
	"testing"
)

func TestFormatCompactSummaryStripsAnalysisScratchpad(t *testing.T) {
	formatted := FormatCompactSummary("<analysis>private draft</analysis>\n<summary>useful summary</summary>")
	if formatted != "Summary:\nuseful summary" || strings.Contains(formatted, "private draft") {
		t.Fatalf("FormatCompactSummary() = %q", formatted)
	}
}

func TestCompactSummaryRequiresNonEmptySummaryBlock(t *testing.T) {
	for _, value := range []string{"plain text", "<summary></summary>", "<analysis>draft</analysis>"} {
		if validCompactSummary(value) {
			t.Fatalf("validCompactSummary(%q) = true", value)
		}
	}
	if !validCompactSummary("<analysis>draft</analysis><summary>usable state</summary>") {
		t.Fatal("valid compact response was rejected")
	}
}

func TestCompactPromptIncludesNoToolsGuard(t *testing.T) {
	prompt := GetPartialCompactPrompt("", "up_to")
	for _, expected := range []string{"CRITICAL: Respond with TEXT ONLY", "newer messages that build on it will follow", "Tool calls will be rejected"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("prompt missing %q", expected)
		}
	}
}

// The summary serves an agent product: it keeps what is known about the user,
// exact identifiers and references, and the user's language; a summary stored
// for later turns does not ask them to continue a task (ADR 0038).
func TestCompactPromptFitsAnAgentProduct(t *testing.T) {
	prompt := GetPartialCompactPrompt("", "up_to")
	for _, expected := range []string{"About the user", "Copy identifiers", "agent-context://", "in the language the user writes in", "approvals given or refused"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("prompt missing %q", expected)
		}
	}
	for _, unwanted := range []string{"Files and Code Sections", "full code snippets", "Bash, Grep"} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("prompt still contains %q", unwanted)
		}
	}
	if stored := GetCompactUserSummaryMessage("<summary>s</summary>", false, "", true); strings.Contains(stored, "Continue the current task") {
		t.Fatalf("stored summary asks to continue a task: %q", stored)
	}
	if inTurn := GetCompactUserSummaryMessage("<summary>s</summary>", true, "", true); !strings.Contains(inTurn, "Continue the current task") {
		t.Fatalf("in-turn summary should continue the task: %q", inTurn)
	}
}
