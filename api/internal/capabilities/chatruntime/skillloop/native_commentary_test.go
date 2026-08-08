package skillloop

import (
	"fmt"
	"strings"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestNativeCommentaryClassifiesBusinessAndControlTurns(t *testing.T) {
	toolSet := nativeCommentaryTestToolSet()
	businessCall := nativeCommentaryTestCall("business-1", "calculate")
	controlCall := nativeCommentaryTestCall("control-1", skills.MetaToolUpdatePlan)

	tests := []struct {
		name        string
		calls       []adapter.ToolCall
		wantProcess bool
		wantReason  string
	}{
		{name: "business", calls: []adapter.ToolCall{businessCall}, wantProcess: true},
		{name: "mixed business and control", calls: []adapter.ToolCall{controlCall, businessCall}, wantProcess: true},
		{name: "control only with safe task update", calls: []adapter.ToolCall{controlCall}, wantProcess: true},
		{name: "unknown call", calls: []adapter.ToolCall{nativeCommentaryTestCall("unknown-1", "unknown_function")}, wantReason: nativeCommentaryRejectNoBusinessTool},
		{name: "malformed business call", calls: []adapter.ToolCall{{ID: "bad-1", Type: "function", Function: adapter.FunctionCall{Name: "calculate", Arguments: `{`}}}, wantReason: nativeCommentaryRejectNoBusinessTool},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := newNativeCommentaryState("qwen-plus", nil)
			decision := state.classify("The source data is ready; the next action will produce the requested result.", test.calls, toolSet)
			if got := decision.disposition == nativeCommentaryDispositionProcess; got != test.wantProcess {
				t.Fatalf("decision = %#v, want process=%v", decision, test.wantProcess)
			}
			if decision.reason != test.wantReason {
				t.Fatalf("reason = %q, want %q", decision.reason, test.wantReason)
			}
		})
	}
}

func TestNativeCommentaryKeepsSafeTaskUpdateDuringControlTurn(t *testing.T) {
	decision := newNativeCommentaryState("deepseek-v4-flash", nil).classify(
		"I will first locate and read the source chapter, then continue the story, generate the requested file, and update the existing configuration without changing its other sections.",
		[]adapter.ToolCall{nativeCommentaryTestCall("control-1", skills.MetaToolActivateSkills)},
		nativeCommentaryTestToolSet(),
	)
	if decision.disposition != nativeCommentaryDispositionProcess || decision.reason != "" {
		t.Fatalf("decision = %#v, want safe control-turn update preserved as process text", decision)
	}
}

func TestNativeCommentaryUsesTokenBudgetsAcrossLanguages(t *testing.T) {
	toolSet := nativeCommentaryTestToolSet()
	call := []adapter.ToolCall{nativeCommentaryTestCall("business-1", "calculate")}
	for name, content := range map[string]string{
		"Chinese":  "现有数据已经足以支持分析；接下来会整理关键结论并生成可交付结果。",
		"Arabic":   "أصبحت البيانات الأساسية جاهزة، وستنتج الخطوة التالية النتيجة المطلوبة.",
		"Japanese": "必要な根拠を確認できました。次の処理で利用可能な結果を作成します。",
		"Emoji":    "The evidence is ready ✅; the next action will produce the requested output 📄.",
	} {
		t.Run(name, func(t *testing.T) {
			decision := newNativeCommentaryState("qwen-plus", nil).classify(content, call, toolSet)
			if decision.disposition != nativeCommentaryDispositionProcess || decision.tokens <= 0 || decision.tokens > nativeCommentaryMaxTokens {
				t.Fatalf("decision = %#v, want accepted token-estimated commentary", decision)
			}
		})
	}

	tooLong := strings.Repeat("evidence is ready and the next action will produce the result ", 80)
	decision := newNativeCommentaryState("qwen-plus", nil).classify(tooLong, call, toolSet)
	if decision.reason != nativeCommentaryRejectTokenLimit {
		t.Fatalf("long commentary decision = %#v, want token limit rejection", decision)
	}
}

func TestNativeCommentaryRejectsInternalIdentifiers(t *testing.T) {
	toolSet := nativeCommentaryTestToolSet()
	call := []adapter.ToolCall{nativeCommentaryTestCall("business-call-1", "calculate")}
	for _, content := range []string{
		"I will call calculate with the prepared values.",
		"The internal skill is calculator and the result follows.",
		"The invocation business-call-1 is ready.",
		"The internal identifier is 6d14e97b-8e38-4205-b0ec-ac32e0017c31.",
		"I will run update_plan before continuing.",
	} {
		decision := newNativeCommentaryState("qwen-plus", nil).classify(content, call, toolSet)
		if decision.reason != nativeCommentaryRejectInternalIdentity {
			t.Fatalf("content %q decision = %#v, want internal identifier rejection", content, decision)
		}
	}
}

func TestNativeCommentaryEnforcesCountAndRestoresPersistedBudget(t *testing.T) {
	toolSet := nativeCommentaryTestToolSet()
	calls := []adapter.ToolCall{nativeCommentaryTestCall("business-1", "calculate")}
	state := newNativeCommentaryState("qwen-plus", nil)
	for index := 0; index < nativeCommentaryMaxCount; index++ {
		decision := state.classify(fmt.Sprintf("Evidence batch %d is ready; the next action will produce the requested result.", index+1), calls, toolSet)
		if decision.disposition != nativeCommentaryDispositionProcess {
			t.Fatalf("commentary %d decision = %#v, want process", index+1, decision)
		}
	}
	if decision := state.classify("Another stage is ready and will produce the remaining result.", calls, toolSet); decision.reason != nativeCommentaryRejectCountLimit {
		t.Fatalf("seventh commentary decision = %#v, want count rejection", decision)
	}

	metadata := map[string]interface{}{
		"presentation": map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"kind": "text", "content_phase": "process", "content": "Prior evidence is ready."},
				map[string]interface{}{"kind": "event", "event_type": "skill_call_start"},
			},
		},
	}
	restored := newNativeCommentaryState("qwen-plus", metadata)
	if restored.count != 1 || restored.totalTokens <= 0 {
		t.Fatalf("restored state = %#v, want one persisted process segment", restored)
	}
}

func TestNativeCommentaryEnforcesCumulativeTokenBudget(t *testing.T) {
	state := newNativeCommentaryState("qwen-plus", nil)
	state.totalTokens = nativeCommentaryTurnTokenLimit - 1
	decision := state.classify(
		"The evidence is ready and the next action will produce the requested result.",
		[]adapter.ToolCall{nativeCommentaryTestCall("business-1", "calculate")},
		nativeCommentaryTestToolSet(),
	)
	if decision.reason != nativeCommentaryRejectTurnBudget {
		t.Fatalf("decision = %#v, want cumulative budget rejection", decision)
	}
}

func nativeCommentaryTestToolSet() *skills.NativeToolSet {
	return &skills.NativeToolSet{
		ActiveSkillIDs: []string{skills.SkillCalculator},
		ToolBindings: map[string]skills.NativeToolBinding{
			"calculate": {SkillID: skills.SkillCalculator, ToolName: "calculate"},
		},
	}
}

func nativeCommentaryTestCall(id string, name string) adapter.ToolCall {
	return adapter.ToolCall{
		ID:   id,
		Type: "function",
		Function: adapter.FunctionCall{
			Name:      name,
			Arguments: `{}`,
		},
	}
}
