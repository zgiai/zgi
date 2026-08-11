package service

import (
	"reflect"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestNativeSkillProjectionBudgetUsesRemainingPromptBudget(t *testing.T) {
	prepared := &PreparedChat{parts: &chatRequestParts{ContextControl: map[string]interface{}{
		"prompt_budget":           50000,
		"estimated_prompt_tokens": 10000,
	}}}
	want := 120000 - nativeSkillProjectionOverheadChars(nil, nil)
	if got := nativeSkillProjectionBudgetChars(prepared, nil, nil); got != want {
		t.Fatalf("nativeSkillProjectionBudgetChars() = %d, want %d", got, want)
	}
}

func TestNativeSkillPriorityPrefersPlanThenHistoryThenBindings(t *testing.T) {
	prepared := &PreparedChat{
		PreferredRestoredSkillID: "continuation-skill",
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"phases": []interface{}{map[string]interface{}{
					"expected_action": map[string]interface{}{"skill_id": "plan-skill"},
				}},
			},
			"skill_invocations": []interface{}{
				map[string]interface{}{"skill_id": "history-old"},
				map[string]interface{}{"skill_id": "history-new"},
			},
		}},
		parts: &chatRequestParts{SkillIDs: []string{"binding-a", "binding-b"}},
	}
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{
		{Metadata: skills.SkillMetadata{ID: "fallback"}},
	}}
	want := []string{"continuation-skill", "plan-skill", "history-new", "history-old", "binding-a", "binding-b", "fallback"}
	if got := nativeSkillPriorityIDs(prepared, resolved); !reflect.DeepEqual(got, want) {
		t.Fatalf("nativeSkillPriorityIDs() = %#v, want %#v", got, want)
	}
}

func TestNativeSkillDiagnosticsRecordsActivationAndSkipReasons(t *testing.T) {
	diagnostics := nativeSkillDiagnostics(skills.NativeToolSet{
		ActiveSkillIDs:   []string{"active"},
		ToolBindings:     map[string]skills.NativeToolBinding{"run": {SkillID: "active", ToolName: "real_run"}},
		SkippedSkills:    []skills.NativeSkillSkip{{SkillID: "skipped", Reason: "schema_unavailable"}},
		InstructionChars: 100,
		SchemaChars:      50,
		BudgetChars:      1000,
	})
	if got := stringSliceFromAny(diagnostics["active_skill_ids"]); !reflect.DeepEqual(got, []string{"active"}) {
		t.Fatalf("active_skill_ids = %#v", got)
	}
	skipped := mapSliceFromAny(diagnostics["skipped_skills"])
	if len(skipped) != 1 || stringFromAny(skipped[0]["reason"]) != "schema_unavailable" {
		t.Fatalf("skipped_skills = %#v", skipped)
	}
}

func TestNativeSkillProtocolKeepsPersistedModeAndProtectsLegacyContinuation(t *testing.T) {
	progressive := &PreparedChat{Message: &runtimemodel.Message{Metadata: map[string]interface{}{
		"native_skill_protocol": skills.NativeSkillProtocolProgressiveV1,
	}}}
	if got := nativeSkillProtocolForPrepared(progressive); got != skills.NativeSkillProtocolProgressiveV1 {
		t.Fatalf("persisted protocol = %q, want progressive_v1", got)
	}

	legacyContinuation := &PreparedChat{
		Continuation: true,
		Message:      &runtimemodel.Message{Metadata: map[string]interface{}{}},
	}
	if got := nativeSkillProtocolForPrepared(legacyContinuation); got != skills.NativeSkillProtocolPreloadV1 {
		t.Fatalf("legacy continuation protocol = %q, want native_preload_v1", got)
	}
}

func TestNativeSkillProtocolDefaultsToProgressiveDisclosure(t *testing.T) {
	prepared := &PreparedChat{Message: &runtimemodel.Message{Metadata: map[string]interface{}{}}}
	if got := nativeSkillProtocolForPrepared(prepared); got != skills.NativeSkillProtocolProgressiveV1 {
		t.Fatalf("protocol = %q, want progressive_v1", got)
	}
}

func TestReplacementRootKeepsPersistedNativeSkillProtocol(t *testing.T) {
	source := &runtimemodel.Message{
		ID:             uuid.New(),
		ConversationID: uuid.New(),
		Metadata: map[string]interface{}{
			"native_skill_protocol": skills.NativeSkillProtocolProgressiveV1,
		},
	}
	replacement := replacementRootMessage(source, &chatRequestParts{ExecutionMode: executionModeNativeToolLoop})
	if got := stringFromAny(replacement.Metadata["native_skill_protocol"]); got != skills.NativeSkillProtocolProgressiveV1 {
		t.Fatalf("native_skill_protocol = %q, want progressive_v1", got)
	}
}

func TestNativeInitialActiveSkillsUsesOnlyDeterministicSignals(t *testing.T) {
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{
		{Metadata: skills.SkillMetadata{ID: "bound-only", Name: "Bound helper"}},
		{Metadata: skills.SkillMetadata{ID: "planned", Name: "Planning helper"}},
		{Metadata: skills.SkillMetadata{ID: "used", Name: "Used helper"}},
		{Metadata: skills.SkillMetadata{ID: "explicit", Name: "File Generator"}},
	}}
	prepared := &PreparedChat{
		PreferredRestoredSkillID: "planned",
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"skill_invocations": []interface{}{map[string]interface{}{
				"kind": "tool_call", "status": "success", "skill_id": "used",
			}},
		}},
		parts: &chatRequestParts{
			Query:    "请使用 File Generator 生成报告",
			SkillIDs: []string{"bound-only", "planned", "used", "explicit"},
		},
	}

	got := nativeInitialActiveSkillIDs(prepared, resolved)
	want := []string{"planned", "used", "explicit"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("nativeInitialActiveSkillIDs() = %#v, want %#v", got, want)
	}
}

func TestNativeInitialActiveSkillsDoesNotTreatDuplicateNameAsExplicit(t *testing.T) {
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{
		{Metadata: skills.SkillMetadata{ID: "writer-a", Name: "Report Writer"}},
		{Metadata: skills.SkillMetadata{ID: "writer-b", Name: "Report Writer"}},
	}}
	prepared := &PreparedChat{
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{}},
		parts:   &chatRequestParts{Query: "Use Report Writer for this task"},
	}
	if got := nativeInitialActiveSkillIDs(prepared, resolved); len(got) != 0 {
		t.Fatalf("nativeInitialActiveSkillIDs() = %#v, want duplicate name to remain a candidate", got)
	}
}

func TestNativeSkillCatalogTokenBudgetUsesTwoPercentOfSafeContext(t *testing.T) {
	prepared := &PreparedChat{parts: &chatRequestParts{ContextControl: map[string]interface{}{
		"safe_context_limit": 50000,
	}}}
	if got := nativeSkillCatalogTokenBudget(prepared); got != 1000 {
		t.Fatalf("nativeSkillCatalogTokenBudget() = %d, want 1000", got)
	}
}

func TestNativeSkillIDsWithoutKeepsCandidateSlotsForInactiveSkills(t *testing.T) {
	got := nativeSkillIDsWithout([]string{"active", "candidate-a", "candidate-b"}, []string{"ACTIVE"})
	want := []string{"candidate-a", "candidate-b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("nativeSkillIDsWithout() = %#v, want %#v", got, want)
	}
}
