package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestStructuredTurnStateOpenItemsDropsRecoveredFileReadToolMismatch(t *testing.T) {
	invocations := []map[string]interface{}{
		{
			"kind":      "tool_call",
			"status":    "error",
			"skill_id":  skills.SkillAgentManagement,
			"tool_name": "get_agent_config",
			"arguments": map[string]interface{}{
				"file_id":   "file-1",
				"max_chars": 12000,
			},
			"error": "missing required argument(s): agent_id",
		},
		{
			"kind":      "tool_call",
			"status":    "success",
			"skill_id":  skills.SkillFileReader,
			"tool_name": "read_file",
			"arguments": map[string]interface{}{
				"file_id": "file-1",
			},
			"result": map[string]interface{}{
				"status":  "completed",
				"file_id": "file-1",
			},
		},
	}

	if failed := currentTurnFailedOperations(invocations, 6); len(failed) != 0 {
		t.Fatalf("currentTurnFailedOperations() = %#v, want recovered failure omitted", failed)
	}
	if open := structuredTurnStateOpenItemsFromInvocations(invocations, 6); len(open) != 0 {
		t.Fatalf("structuredTurnStateOpenItemsFromInvocations() = %#v, want no active open items", open)
	}
}

func TestApplyOperationPlanCompletionVerificationPassCleansStaleReasonAndSource(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"tool_choice_mode": "model_decides",
			"status":           "running",
		},
	}

	applyOperationPlanCompletionVerificationResultWithSource(
		metadata,
		"pass",
		"completion_gate",
		"最终答案后校验发现当前回答缺少工具结果支持",
		nil,
		nil,
		"",
	)

	plan := mapFromOperationContext(metadata["operation_plan"])
	verification := mapFromOperationContext(plan["completion_verification"])
	if got := stringFromAny(verification["status"]); got != "pass" {
		t.Fatalf("completion_verification.status = %q, want pass; verification=%#v", got, verification)
	}
	if got := stringFromAny(verification["source"]); got != "completion_gate" {
		t.Fatalf("completion_verification.source = %q, want completion_gate; verification=%#v", got, verification)
	}
	if reason := stringFromAny(verification["reason"]); strings.Contains(reason, "缺少工具结果支持") {
		t.Fatalf("completion_verification.reason = %q, want stale failure reason removed", reason)
	}
	summary := mapFromOperationContext(metadata["operation_result_summary"])
	if got := stringFromAny(summary["status"]); got != "completed" {
		t.Fatalf("operation_result_summary.status = %q, want completed; summary=%#v", got, summary)
	}
	if got := stringFromAny(summary["plan_status"]); got != "completed" {
		t.Fatalf("operation_result_summary.plan_status = %q, want completed; summary=%#v", got, summary)
	}
}

func TestModelInvocationFromTraceStoresCompactPayloadByDefault(t *testing.T) {
	t.Setenv("ZGI_AICHAT_MODEL_INVOCATION_DEBUG", "")
	rawPrompt := "PRIVATE MODEL REQUEST BODY " + strings.Repeat("x", 256)
	invocation := modelInvocationFromTrace(skillloop.ModelInvocationTrace{
		Phase: "completion_verifier",
		Round: 2,
		Request: &adapter.ChatRequest{
			Provider: "qwen",
			Model:    "qwen3-6-plus",
			Messages: []adapter.Message{{
				Role:    "user",
				Content: rawPrompt,
			}},
		},
		Response: &adapter.Message{
			Role:    "assistant",
			Content: "done",
		},
	}, "", true)

	if got := stringFromAny(invocation["payload_mode"]); got != "compact" {
		t.Fatalf("payload_mode = %q, want compact; invocation=%#v", got, invocation)
	}
	encoded, err := json.Marshal(invocation)
	if err != nil {
		t.Fatalf("marshal invocation: %v", err)
	}
	if strings.Contains(string(encoded), rawPrompt) {
		t.Fatalf("compact model invocation leaked full request content: %s", string(encoded))
	}
	request := mapFromOperationContext(invocation["request"])
	if got := intValueFromAny(request["message_count"]); got != 1 {
		t.Fatalf("request.message_count = %d, want 1; request=%#v", got, request)
	}
	if _, exists := request["messages"]; exists {
		t.Fatalf("compact request has full messages = %#v", request["messages"])
	}
}
