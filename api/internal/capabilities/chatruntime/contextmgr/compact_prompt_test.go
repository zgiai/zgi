package contextmgr

import (
	"crypto/sha256"
	"fmt"
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
	for _, expected := range []string{"CRITICAL: Respond with TEXT ONLY", "newer messages that build on this context will follow", "Tool calls will be rejected"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("prompt missing %q", expected)
		}
	}
}

func TestClaudeCompactTemplateSnapshots(t *testing.T) {
	cases := map[string]struct {
		value string
		hash  string
	}{
		"base":    {value: GetCompactPrompt(""), hash: "042f6d6d58d27ee21ab5ed7a248c5a3fbb24019a3738cc683af2644f8b9d55eb"},
		"partial": {value: GetPartialCompactPrompt("", "from"), hash: "86e8aeeeaedfca9cb12e8963376644a3c0b7f36adacb22b04d90687dbab0a59c"},
		"up_to":   {value: GetPartialCompactPrompt("", "up_to"), hash: "ce99ed3b6e51a59811453c391287b5617011701e11b2f96a7fa40948e214a569"},
		"wrapper": {value: GetCompactUserSummaryMessage("<summary>snapshot</summary>", true, "", true), hash: "e5f216cd688f4abe1611e2150addad323a2633ebf78345b7aeb631d88bbaaf9c"},
	}
	for name, testCase := range cases {
		digest := fmt.Sprintf("%x", sha256.Sum256([]byte(testCase.value)))
		if digest != testCase.hash {
			t.Errorf("%s compact template hash = %s", name, digest)
		}
	}
}
