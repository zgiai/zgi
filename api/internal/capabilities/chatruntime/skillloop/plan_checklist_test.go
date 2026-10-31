package skillloop

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/contextmgr"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

// update_plan records the model's checklist; the runtime checks its shape
// only (ADR 0049).
func TestUpdatePlanRecordsAChecklist(t *testing.T) {
	step := (&Runner{}).handleUpdatePlanCall("call-plan", map[string]interface{}{
		"explanation": "split the work",
		"plan": []interface{}{
			map[string]interface{}{"step": "Compute the totals", "status": "completed"},
			map[string]interface{}{"step": "Write the CSV", "status": "in_progress"},
		},
	}, nil, 1)
	if step.fatalErr != nil || step.trace.Status != "success" {
		t.Fatalf("handleUpdatePlanCall() = %#v", step)
	}
	plan := evidenceMapsFromAny(step.trace.Result["plan"])
	if len(plan) != 2 || plan[0]["id"] != "step-1" || plan[1]["status"] != "in_progress" {
		t.Fatalf("plan = %#v", plan)
	}

	for _, bad := range []interface{}{
		nil,
		[]interface{}{map[string]interface{}{"step": "A", "status": "done"}},
		[]interface{}{map[string]interface{}{"step": "", "status": "pending"}},
		[]interface{}{
			map[string]interface{}{"step": "A", "status": "in_progress"},
			map[string]interface{}{"step": "B", "status": "in_progress"},
		},
	} {
		step := (&Runner{}).handleUpdatePlanCall("call-bad", map[string]interface{}{"plan": bad}, nil, 1)
		if step.trace.Status == "success" || !step.recoverable {
			t.Fatalf("plan %#v was accepted: %#v", bad, step)
		}
	}
}

// A final answer that leaves checklist steps open is held back once, so the
// model finishes them or marks them skipped; the second answer is accepted.
func TestRunnerRemindsOnceOfOpenPlanSteps(t *testing.T) {
	runtime := skills.NewRuntime(nil, nil)
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata:     skills.SkillMetadata{ID: "prompt-only", RuntimeType: skills.SkillRuntimeTypePrompt},
		Instructions: "Answer concisely.",
	}}}
	toolSet := runtime.BuildNativeToolSet(context.Background(), resolved, skills.NativeToolSetOptions{BudgetChars: 10000})
	planCall := adapter.ToolCall{ID: "plan-1", Type: "function", Function: adapter.FunctionCall{
		Name:      skills.MetaToolUpdatePlan,
		Arguments: `{"plan":[{"step":"Compute","status":"completed"},{"step":"Write the file","status":"pending"}]}`,
	}}
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{planCall}}, FinishReason: "tool_calls"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "Here is the total."}, FinishReason: "stop"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "The file was not needed after all."}, FinishReason: "stop"}}},
	}}
	var calls []bool
	runner := &Runner{LLMClient: fakeLLM, SkillRuntime: runtime, AppContext: &llmclient.AppContext{},
		OnModelInvocation: func(trace ModelInvocationTrace) { calls = append(calls, trace.PlanReminded) }}
	prepared := NewPreparedChat("conv-plan", "msg-plan", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "Compute the total and write a file"}},
	})

	answer, _, err := runner.Run(context.Background(), RunRequest{Prepared: prepared, Resolved: resolved, NativeToolSet: &toolSet})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "The file was not needed after all." || fakeLLM.appChatCalls != 3 {
		t.Fatalf("answer = %q after %d calls, want the answer after one reminder", answer, fakeLLM.appChatCalls)
	}
	reminded := fakeLLM.appChatRequests[2].Messages
	if !strings.Contains(assistantMessageText(reminded[len(reminded)-1]), "Write the file") {
		t.Fatalf("reminder request = %#v, want the open step named", reminded[len(reminded)-1])
	}
	if len(calls) != 3 || calls[1] || !calls[2] {
		t.Fatalf("plan_reminded per model call = %v, want only the call after the reminder", calls)
	}
}

// A run that continues the turn, as after an approval, starts from the
// checklist the turn already holds, so its open steps are still pointed out.
func TestContinuedRunKeepsTheTurnChecklist(t *testing.T) {
	runtime := skills.NewRuntime(nil, nil)
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata:     skills.SkillMetadata{ID: "prompt-only", RuntimeType: skills.SkillRuntimeTypePrompt},
		Instructions: "Answer concisely.",
	}}}
	toolSet := runtime.BuildNativeToolSet(context.Background(), resolved, skills.NativeToolSetOptions{BudgetChars: 10000})
	fakeLLM := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "Approved and sent."}, FinishReason: "stop"}}},
		{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "Sent; the report was dropped."}, FinishReason: "stop"}}},
	}}
	metadata := map[string]interface{}{"operation_plan": map[string]interface{}{
		"version": planChecklistVersion,
		"phases": []interface{}{
			map[string]interface{}{"id": "step-1", "step": "Send the message", "status": "in_progress"},
			map[string]interface{}{"id": "step-2", "step": "Write the report", "status": "pending"},
		},
	}}
	runner := &Runner{LLMClient: fakeLLM, SkillRuntime: runtime, AppContext: &llmclient.AppContext{}}
	prepared := NewPreparedChat("conv-plan", "msg-plan", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "Send the message and write the report"}},
	})
	answer, _, err := runner.Run(context.Background(), RunRequest{
		Prepared: prepared, Resolved: resolved, NativeToolSet: &toolSet,
		CurrentMetadata: func() map[string]interface{} { return metadata },
	})
	if err != nil || answer != "Sent; the report was dropped." || fakeLLM.appChatCalls != 2 {
		t.Fatalf("answer = %q after %d calls (err %v), want one reminder of the stored checklist", answer, fakeLLM.appChatCalls, err)
	}
	if !strings.Contains(runnerTestMessageText(fakeLLM.appChatRequests[0].Messages), "[pending] Write the report") {
		t.Fatal("the continued run's first request does not show the stored checklist")
	}
}

// A request whose compaction dropped the update_plan result gets the
// checklist back; one that still holds it does not.
func TestChecklistIsRestoredWhenTheRequestLostIt(t *testing.T) {
	steps := []map[string]interface{}{{"step": "Compute", "status": "completed"}, {"step": "Write", "status": "in_progress"}}
	restored := checklistMissingFromRequest([]adapter.Message{{Role: "user", Content: "summary of earlier work"}}, steps)
	if restored == nil || !strings.Contains(assistantMessageText(*restored), "[in_progress] Write") {
		t.Fatalf("restored = %#v", restored)
	}
	kept := []adapter.Message{skills.ToolResultMessage("plan-1", map[string]interface{}{"status": "recorded", "plan": steps})}
	if checklistMissingFromRequest(kept, steps) != nil {
		t.Fatal("the checklist was added although the request holds it")
	}
	earlier := []map[string]interface{}{{"step": "Compute", "status": "in_progress"}, {"step": "Write", "status": "pending"}}
	stale := []adapter.Message{skills.ToolResultMessage("plan-0", map[string]interface{}{"status": "recorded", "plan": earlier})}
	if checklistMissingFromRequest(stale, steps) == nil {
		t.Fatal("an earlier checklist in the request kept the current one from being restored")
	}
	if checklistMissingFromRequest(nil, nil) != nil {
		t.Fatal("a run without a checklist got one")
	}
}

func restoredChecklistTestSteps(count int, width int) []map[string]interface{} {
	steps := make([]map[string]interface{}, 0, count)
	for index := 0; index < count; index++ {
		steps = append(steps, map[string]interface{}{
			"step":   fmt.Sprintf("step %02d %s", index+1, strings.Repeat("x", width)),
			"status": "pending",
		})
	}
	return steps
}

// A restored checklist belongs to one request: the next round does not carry
// it on, so every request holds exactly one copy (ADR 0049).
func TestRestoredChecklistIsSentOnceEveryRound(t *testing.T) {
	manager, err := contextmgr.New(contextmgr.Config{AgentRunID: "checklist-rounds", ModelContextWindow: 64_000, MaxOutputTokens: 4_000}, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	runner := &Runner{AppContext: &llmclient.AppContext{}, ContextManager: manager, planSteps: restoredChecklistTestSteps(3, 10)}
	messages := []adapter.Message{{Role: "system", Content: "persona"}, {Role: "user", Content: "finish the checklist"}}
	for round := 1; round <= 5; round++ {
		request, err := runner.prepareContextRequest(context.Background(), &adapter.ChatRequest{Model: "test-model", Messages: messages})
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Count(runnerTestMessageText(request.Messages), "Your current checklist:"); got != 1 {
			t.Fatalf("round %d sends %d checklists, want 1", round, got)
		}
		callID := fmt.Sprintf("call-%d", round)
		call := adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: callID, Type: "function", Function: adapter.FunctionCall{Name: "read", Arguments: "{}"}}}}
		result, err := runner.finishContextRequest(context.Background(), request, planningResult{message: call})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(runnerTestMessageText(result.contextMessages), "Your current checklist:") {
			t.Fatalf("round %d hands the restored checklist on to the next round", round)
		}
		messages = append(result.contextMessages, call, adapter.Message{Role: "tool", ToolCallID: callID, Content: "rows"})
	}
}

// The restored checklist is fitted with the rest of the request: what the
// runner sends needs no further fitting (ADR 0049).
func TestRestoredChecklistCountsAgainstTheContextBudget(t *testing.T) {
	newManager := func(id string) *contextmgr.Manager {
		manager, err := contextmgr.New(contextmgr.Config{AgentRunID: id, ModelContextWindow: 8_192, MaxOutputTokens: 1_024}, nil, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		return manager
	}
	messages := []adapter.Message{{Role: "system", Content: "persona"}}
	for turn := 0; turn < 60; turn++ {
		messages = append(messages,
			adapter.Message{Role: "user", Content: fmt.Sprintf("question %d %s", turn, strings.Repeat("detail ", 40))},
			adapter.Message{Role: "assistant", Content: fmt.Sprintf("answer %d %s", turn, strings.Repeat("finding ", 40))},
		)
	}
	messages = append(messages, adapter.Message{Role: "user", Content: "continue the checklist"})
	maxTokens := 1_024
	runner := &Runner{AppContext: &llmclient.AppContext{}, ContextManager: newManager("checklist-budget"), planSteps: restoredChecklistTestSteps(16, 190)}
	request, err := runner.prepareContextRequest(context.Background(), &adapter.ChatRequest{Model: "test-model", MaxTokens: &maxTokens, Messages: messages})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(runnerTestMessageText(request.Messages), "step 16") {
		t.Fatal("the request lost the restored checklist")
	}
	_, decision, err := newManager("checklist-budget-check").PrepareBeforeModelCall(context.Background(), request)
	if err != nil || decision.Action != contextmgr.DecisionNone {
		t.Fatalf("the sent request needs fitting again: action=%s prompt=%d budget=%d err=%v", decision.Action, decision.FinalPromptTokens, decision.Budget.PromptBudget, err)
	}
}
