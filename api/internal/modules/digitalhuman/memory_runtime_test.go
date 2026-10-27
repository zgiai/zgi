package digitalhuman

import (
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/memoryruntime"
)

func TestDigitalHumanMemoryGuidanceKeepsDurableDirectivePolicy(t *testing.T) {
	t.Parallel()
	content := renderDigitalHumanMemoryGuidance(true, true)
	for _, expected := range []string{"self/decision", "published identity", "Never store a conversational role directive as self/identity", "Decide at runtime", "not from trigger words", "Proactive memory maintenance is enabled", "memory_edit"} {
		if !strings.Contains(content, expected) {
			t.Errorf("context missing durable directive policy %q: %s", expected, content)
		}
	}
}

func TestValidateMemoryToolModeAllowsProactiveOnlyWhenOn(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		args             memoryToolArguments
		proactiveAllowed bool
		wantError        bool
	}{
		{name: "explicit durable instruction", args: memoryToolArguments{Action: memoryruntime.MutationActionUpsert, Mode: digitalHumanMemoryModeExplicit, Evidence: "主要职责是为产品输出运营软文"}},
		{name: "ordinary content is not screened by code patterns", args: memoryToolArguments{Action: memoryruntime.MutationActionUpsert, Mode: digitalHumanMemoryModeExplicit, Evidence: "以后你就作为"}},
		{name: "proactive upsert enabled", args: memoryToolArguments{Action: memoryruntime.MutationActionUpsert, Mode: digitalHumanMemoryModeProactive, Evidence: "运营文案写手"}, proactiveAllowed: true},
		{name: "proactive disabled", args: memoryToolArguments{Action: memoryruntime.MutationActionUpsert, Mode: digitalHumanMemoryModeProactive, Evidence: "运营文案写手"}, wantError: true},
		{name: "proactive clear rejected", args: memoryToolArguments{Action: memoryruntime.MutationActionClear, Mode: digitalHumanMemoryModeProactive, Evidence: "运营文案写手"}, proactiveAllowed: true, wantError: true},
		{name: "unknown mode rejected", args: memoryToolArguments{Action: memoryruntime.MutationActionUpsert, Mode: "silent", Evidence: "运营文案写手"}, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := validateMemoryToolMode(test.args, test.proactiveAllowed)
			if (err != nil) != test.wantError {
				t.Fatalf("validateMemoryToolMode() error = %v, wantError %v", err, test.wantError)
			}
		})
	}
}

func TestSafeMemoryArtifactRefsKeepsIdentifiersOnly(t *testing.T) {
	t.Parallel()
	refs := safeMemoryArtifactRefs(map[string]interface{}{
		"conversation_artifacts": []interface{}{map[string]interface{}{
			"artifact_id": "artifact-1", "artifact_type": "report", "file_id": "file-1",
			"content": "must-not-persist", "token": "secret",
		}},
	})
	value := string(refs)
	for _, expected := range []string{"artifact-1", "report", "file-1"} {
		if !strings.Contains(value, expected) {
			t.Errorf("artifact refs missing %q: %s", expected, value)
		}
	}
	for _, forbidden := range []string{"must-not-persist", "secret", "content", "token"} {
		if strings.Contains(value, forbidden) {
			t.Errorf("artifact refs contain %q: %s", forbidden, value)
		}
	}
}
