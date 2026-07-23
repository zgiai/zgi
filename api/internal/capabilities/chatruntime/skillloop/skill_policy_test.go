package skillloop

import (
	"strings"
	"testing"
)

func TestUnavailableSkillPolicyStepReturnsStableBlockedEvidence(t *testing.T) {
	result := unavailableSkillPolicyStep("call-1", "custom-skill", "run", map[string]interface{}{"value": "x"}, nil)
	if !result.recoverable || result.usedSkill || result.usedTool {
		t.Fatalf("result flags = recoverable:%v usedSkill:%v usedTool:%v", result.recoverable, result.usedSkill, result.usedTool)
	}
	if result.trace.Status != "blocked" || result.trace.SkillID != "custom-skill" {
		t.Fatalf("trace = %#v", result.trace)
	}
	if result.trace.Arguments["reason_code"] != "organization_skill_unavailable" {
		t.Fatalf("reason code = %#v", result.trace.Arguments["reason_code"])
	}
	if !strings.Contains(result.trace.Error, "no longer enabled") {
		t.Fatalf("trace error = %q", result.trace.Error)
	}
}
