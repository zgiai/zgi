package service

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestMergeSkillTraceMetadataPersistsLoadedSkillInstructionState(t *testing.T) {
	metadata := mergeSkillTraceMetadata(map[string]interface{}{}, []skills.SkillTrace{{
		Kind:    "skill_load",
		SkillID: skills.SkillAgentManagement,
		Status:  "success",
		Result: map[string]interface{}{
			"instruction_digest": "sha256:digest",
			"instruction_chars":  34700,
			"effective_version":  "sha256:digest",
			"policy_state":       "allowed",
			"access_status":      "authorized",
		},
	}})

	state := mapSliceFromAny(metadata["loaded_skill_state"])
	if len(state) != 1 {
		t.Fatalf("loaded_skill_state = %#v, want one record", state)
	}
	if got := stringFromAny(state[0]["skill_id"]); got != skills.SkillAgentManagement {
		t.Fatalf("skill_id = %q, want %q", got, skills.SkillAgentManagement)
	}
	if got := stringFromAny(state[0]["instruction_digest"]); got != "sha256:digest" {
		t.Fatalf("instruction_digest = %q, want sha256:digest", got)
	}
	if got := intValueFromAny(state[0]["instruction_chars"]); got != 34700 {
		t.Fatalf("instruction_chars = %d, want 34700", got)
	}
	if got := intValueFromAny(state[0]["loaded_sequence"]); got <= 0 {
		t.Fatalf("loaded_sequence = %d, want positive", got)
	}
	if got := intValueFromAny(state[0]["load_sequence"]); got <= 0 {
		t.Fatalf("load_sequence = %d, want positive", got)
	}
	if got := stringFromAny(state[0]["effective_version"]); got != "sha256:digest" {
		t.Fatalf("effective_version = %q, want digest", got)
	}
	if got := stringFromAny(state[0]["policy_state"]); got != "allowed" {
		t.Fatalf("policy_state = %q, want allowed", got)
	}
	if got := stringFromAny(state[0]["access_status"]); got != "authorized" {
		t.Fatalf("access_status = %q, want authorized", got)
	}
}
