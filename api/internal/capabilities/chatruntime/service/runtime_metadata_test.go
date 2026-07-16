package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestSkillLoopControlDiagnosticsTracksPhaseMismatchWithoutDoubleCounting(t *testing.T) {
	trace := skills.SkillTrace{
		Kind:   "planner_feedback",
		Status: "blocked",
		Arguments: map[string]interface{}{
			"code":    "operation_plan_phase_mismatch",
			"call_id": "call-1",
		},
	}
	metadata := mergeSkillTraceMetadata(nil, []skills.SkillTrace{trace})
	metadata = mergeSkillTraceMetadata(metadata, []skills.SkillTrace{trace})
	diagnostics := mapFromOperationContext(metadata[skillLoopControlDiagnosticsKey])
	if got := intValueFromAny(diagnostics["tool_decision_without_execution_count"]); got != 1 {
		t.Fatalf("tool_decision_without_execution_count = %d, want 1", got)
	}
	if got := intValueFromAny(diagnostics["operation_plan_phase_mismatch_count"]); got != 1 {
		t.Fatalf("operation_plan_phase_mismatch_count = %d, want 1", got)
	}
}

func TestSkillLoopControlDiagnosticsTracksSuppressedControlTool(t *testing.T) {
	trace := skills.SkillTrace{
		Kind:   "planner_feedback",
		Status: "advisory",
		Arguments: map[string]interface{}{
			"reason_code": "control_tool_not_required",
			"call_id":     "plan-call-1",
		},
	}
	metadata := mergeSkillTraceMetadata(nil, []skills.SkillTrace{trace})
	metadata = mergeSkillTraceMetadata(metadata, []skills.SkillTrace{trace})
	diagnostics := mapFromOperationContext(metadata[skillLoopControlDiagnosticsKey])
	if got := intValueFromAny(diagnostics["suppressed_control_tool_count"]); got != 1 {
		t.Fatalf("suppressed_control_tool_count = %d, want 1", got)
	}
	if got := intValueFromAny(diagnostics["meta_tool_decision_without_execution_count"]); got != 1 {
		t.Fatalf("meta_tool_decision_without_execution_count = %d, want 1", got)
	}
	if got := intValueFromAny(diagnostics["tool_decision_without_execution_count"]); got != 0 {
		t.Fatalf("tool_decision_without_execution_count = %d, want business gap unchanged", got)
	}
}

func TestSkillLoopControlDiagnosticsTracksSuccessfulMetaToolsSeparately(t *testing.T) {
	metadata := mergeSkillTraceMetadata(nil, []skills.SkillTrace{
		{
			Kind: "turn_state", Status: "success",
			Arguments: map[string]interface{}{"call_id": "state-call-1"},
		},
		{
			Kind: "plan_update", Status: "success",
			Arguments: map[string]interface{}{"call_id": "plan-call-1"},
		},
	})
	metadata = mergeSkillTraceMetadata(metadata, []skills.SkillTrace{{
		Kind: "turn_state", Status: "success",
		Arguments: map[string]interface{}{"call_id": "state-call-1"},
	}})
	diagnostics := mapFromOperationContext(metadata[skillLoopControlDiagnosticsKey])
	if got := intValueFromAny(diagnostics["meta_tool_execution_count"]); got != 2 {
		t.Fatalf("meta_tool_execution_count = %d, want 2", got)
	}
	if got := intValueFromAny(diagnostics["tracked_meta_tool_decision_count"]); got != 2 {
		t.Fatalf("tracked_meta_tool_decision_count = %d, want 2", got)
	}
	if got := intValueFromAny(diagnostics["meta_tool_decision_execution_gap"]); got != 0 {
		t.Fatalf("meta_tool_decision_execution_gap = %d, want 0", got)
	}
}

func TestSkillLoopExecutionDiagnosticsReportsDecisionExecutionGap(t *testing.T) {
	metadata := map[string]interface{}{
		skillLoopControlDiagnosticsKey: map[string]interface{}{
			"tool_decision_without_execution_count": 2,
		},
	}
	applySkillLoopExecutionDiagnostics(metadata, []map[string]interface{}{
		{
			"kind": "tool_call", "status": "success", "runtime_id": "tool-1",
			"skill_id": skills.SkillCalculator, "tool_name": "calculate", "result": map[string]interface{}{"status": "success"},
		},
		{
			"kind": "tool_call", "status": "error", "runtime_id": "tool-2",
			"skill_id": skills.SkillFileReader, "tool_name": "read_file", "error": "not found",
		},
	})
	diagnostics := mapFromOperationContext(metadata[skillLoopControlDiagnosticsKey])
	if got := intValueFromAny(diagnostics["business_tool_execution_count"]); got != 2 {
		t.Fatalf("business_tool_execution_count = %d, want 2", got)
	}
	if got := intValueFromAny(diagnostics["tracked_business_tool_decision_count"]); got != 4 {
		t.Fatalf("tracked_business_tool_decision_count = %d, want 4", got)
	}
	if got := intValueFromAny(diagnostics["tool_decision_execution_rate_percent"]); got != 50 {
		t.Fatalf("tool_decision_execution_rate_percent = %d, want 50", got)
	}
}

func TestModelInvocationRequestPayloadRedactsInlineImageDataURLParts(t *testing.T) {
	imageBody := strings.Repeat("A", 128)
	dataURL := "data:image/jpeg;base64," + imageBody
	req := &adapter.ChatRequest{
		Model: "vision-model",
		Messages: []adapter.Message{{
			Role: "user",
			Content: []adapter.MessageContentPart{
				{
					Type: "image_url",
					ImageURL: &adapter.ImageURL{
						URL:    dataURL,
						Detail: "high",
					},
				},
				{Type: "text", Text: "describe this image"},
			},
		}},
	}

	payload := modelInvocationRequestPayload(req, false)
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if strings.Contains(string(encoded), imageBody) {
		t.Fatalf("payload contains raw inline image data: %s", string(encoded))
	}

	messages := payload["messages"].([]interface{})
	message := messages[0].(map[string]interface{})
	content := message["content"].([]interface{})
	imagePart := content[0].(map[string]interface{})
	imageURL := imagePart["image_url"].(map[string]interface{})
	if got := stringFromAny(imageURL["url"]); got != "data:image/jpeg;base64,<redacted>" {
		t.Fatalf("image url = %q, want redacted data URL", got)
	}
	if imageURL["url_redacted"] != true {
		t.Fatalf("image url summary = %#v, want url_redacted", imageURL)
	}
	if imageURL["url_mime_type"] != "image/jpeg" {
		t.Fatalf("image url mime = %#v, want image/jpeg", imageURL["url_mime_type"])
	}
	if got := intValueFromAny(imageURL["url_base64_chars"]); got != len(imageBody) {
		t.Fatalf("image url base64 chars = %d, want %d", got, len(imageBody))
	}
}

func TestModelInvocationRequestPayloadRedactsEmbeddedImageDataURLText(t *testing.T) {
	imageBody := strings.Repeat("B", 96)
	req := &adapter.ChatRequest{
		Model: "vision-model",
		Messages: []adapter.Message{{
			Role:    "user",
			Content: "please inspect ![chart](data:image/png;base64," + imageBody + ")",
		}},
	}

	payload := modelInvocationRequestPayload(req, false)
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if strings.Contains(string(encoded), imageBody) {
		t.Fatalf("payload contains raw embedded image data: %s", string(encoded))
	}

	messages := payload["messages"].([]interface{})
	message := messages[0].(map[string]interface{})
	content := stringFromAny(message["content"])
	if !strings.Contains(content, "data:image/png;base64,<redacted>") {
		t.Fatalf("content = %q, want embedded data URL redacted", content)
	}
	if message["content_redacted"] != true {
		t.Fatalf("message = %#v, want content_redacted marker for embedded image data", message)
	}
	if got := intValueFromAny(message["content_base64_chars"]); got != len(imageBody) {
		t.Fatalf("content base64 chars = %d, want %d", got, len(imageBody))
	}
}

func TestApplyProviderPromptUsageCalibrationUsesActualPromptTokens(t *testing.T) {
	metadata := map[string]interface{}{
		"context_control": map[string]interface{}{
			"estimated_prompt_tokens": 5000,
			"prompt_budget":           64000,
		},
	}

	calibrated := applyProviderPromptUsageCalibration(metadata, &adapter.Usage{PromptTokens: 55000})
	contextControl := mapFromOperationContext(calibrated["context_control"])
	if got := intValueFromAny(contextControl["provider_prompt_tokens"]); got != 55000 {
		t.Fatalf("provider_prompt_tokens = %d, want 55000", got)
	}
	if got := intValueFromAny(contextControl["calibrated_prompt_tokens"]); got != 55000 {
		t.Fatalf("calibrated_prompt_tokens = %d, want 55000", got)
	}
	if got, ok := contextControl["prompt_estimate_scale"].(float64); !ok || got != 11 {
		t.Fatalf("prompt_estimate_scale = %#v, want 11", contextControl["prompt_estimate_scale"])
	}
	if got := stringFromAny(contextControl["prompt_estimate_source"]); got != "provider_usage" {
		t.Fatalf("prompt_estimate_source = %q, want provider_usage", got)
	}
}

func TestApplyProviderPromptUsageCalibrationUsesFinalRequestEstimate(t *testing.T) {
	metadata := map[string]interface{}{
		"context_control": map[string]interface{}{
			"estimated_prompt_tokens": 5601,
			"prompt_budget":           64000,
		},
	}
	calibrated := applyProviderPromptUsageCalibrationWithEstimate(metadata, &adapter.Usage{PromptTokens: 40167}, 39000, "deepseek", "deepseek", "deepseek-chat")
	control := mapFromOperationContext(calibrated["context_control"])
	if got := intValueFromAny(control["base_estimated_prompt_tokens"]); got != 5601 {
		t.Fatalf("base estimate = %d, want 5601", got)
	}
	if got := intValueFromAny(control["estimated_prompt_tokens"]); got != 39000 {
		t.Fatalf("final estimate = %d, want 39000", got)
	}
	if got := stringFromAny(control["prompt_estimate_version"]); got != "chat_request.v1" {
		t.Fatalf("estimate version = %q", got)
	}
	if scale, ok := control["prompt_estimate_scale"].(float64); !ok || scale < 1 || scale > 1.1 {
		t.Fatalf("prompt_estimate_scale = %#v", control["prompt_estimate_scale"])
	}
}

func TestProviderPromptUsageCalibrationIsScopedByProviderAndModel(t *testing.T) {
	metadata := map[string]interface{}{
		"context_control": map[string]interface{}{"prompt_budget": 64000},
	}
	metadata = applyProviderPromptUsageCalibrationWithEstimate(metadata, &adapter.Usage{PromptTokens: 2000}, 1000, "fallback", "provider-a", "shared-model")
	metadata = applyProviderPromptUsageCalibrationWithEstimate(metadata, &adapter.Usage{PromptTokens: 900}, 1000, "fallback", "provider-b", "shared-model")

	calibrations := mapFromOperationContext(metadata["prompt_usage_calibration"])
	a := mapFromOperationContext(calibrations["provider-a/shared-model"])
	b := mapFromOperationContext(calibrations["provider-b/shared-model"])
	if got, ok := a["prompt_estimate_scale"].(float64); !ok || got != 2 {
		t.Fatalf("provider A scale = %#v, want 2", a["prompt_estimate_scale"])
	}
	if got, ok := b["prompt_estimate_scale"].(float64); !ok || got != 0.9 {
		t.Fatalf("provider B scale = %#v, want 0.9", b["prompt_estimate_scale"])
	}
	if got := stringFromAny(a["estimate_version"]); got != promptEstimateVersionChatRequest {
		t.Fatalf("provider A estimate version = %q", got)
	}

	metadata = applyProviderPromptUsageCalibrationWithEstimate(metadata, &adapter.Usage{PromptTokens: 1500}, 1000, "fallback", "provider-a", "shared-model")
	calibrations = mapFromOperationContext(metadata["prompt_usage_calibration"])
	b = mapFromOperationContext(calibrations["provider-b/shared-model"])
	if got, ok := b["prompt_estimate_scale"].(float64); !ok || got != 0.9 {
		t.Fatalf("provider B scale changed after provider A update: %#v", b["prompt_estimate_scale"])
	}
}

func TestLegacyPromptEstimateDoesNotCreateReusableCalibration(t *testing.T) {
	metadata := map[string]interface{}{
		"context_control": map[string]interface{}{
			"estimated_prompt_tokens": 1000,
		},
	}
	metadata = applyProviderPromptUsageCalibrationWithEstimate(metadata, &adapter.Usage{PromptTokens: 2000}, 0, "", "provider-a", "model-a")
	if calibrations := mapFromOperationContext(metadata["prompt_usage_calibration"]); len(calibrations) != 0 {
		t.Fatalf("legacy estimate created reusable calibration: %#v", calibrations)
	}
}

func TestModelInvocationFromTraceStoresCountOnlyPromptDiagnostics(t *testing.T) {
	t.Setenv("ZGI_AICHAT_MODEL_INVOCATION_DEBUG", "")
	secret := "DO_NOT_PERSIST_THIS_TOOL_SCHEMA"
	invocation := modelInvocationFromTrace(skillloop.ModelInvocationTrace{
		Phase: "skill_planning",
		Request: &adapter.ChatRequest{
			Model: "deepseek-chat",
			Tools: []adapter.Tool{{
				Type: "function",
				Function: adapter.Function{
					Name:        "private_tool",
					Description: secret,
				},
			}},
		},
		Usage:                      &adapter.Usage{PromptTokens: 240},
		PromptChars:                900,
		RequestChars:               1200,
		EstimatedPromptTokens:      200,
		PromptEstimator:            "fallback:conservative",
		PromptComponentTokens:      map[string]int{"messages": 80, "tools": 120},
		PromptComponentChars:       map[string]int{"messages": 320, "tools": 580},
		BudgetSafeContextLimit:     1200,
		BudgetPromptLimit:          800,
		BudgetOriginalPromptTokens: 350,
		BudgetCompressionChars:     map[string]int{"historical_tool_payloads": 700},
		BudgetSavedChars:           700,
		BudgetMaxTokensClamped:     true,
		BudgetOriginalMaxTokens:    1000,
		BudgetEffectiveMaxTokens:   960,
	}, "", true)

	if got := intValueFromAny(invocation["prompt_chars"]); got != 900 {
		t.Fatalf("prompt_chars = %d, want legacy value 900", got)
	}
	if got := intValueFromAny(invocation["request_chars"]); got != 1200 {
		t.Fatalf("request_chars = %d, want 1200", got)
	}
	if got := intValueFromAny(invocation["estimated_prompt_tokens"]); got != 200 {
		t.Fatalf("estimated_prompt_tokens = %d, want 200", got)
	}
	if got := stringFromAny(invocation["prompt_estimator"]); got != "fallback:conservative" {
		t.Fatalf("prompt_estimator = %q", got)
	}
	if got := mapFromOperationContext(invocation["prompt_component_tokens"]); intValueFromAny(got["tools"]) != 120 {
		t.Fatalf("prompt_component_tokens = %#v", got)
	}
	if scale, ok := invocation["prompt_estimate_scale"].(float64); !ok || scale != 1.2 {
		t.Fatalf("prompt_estimate_scale = %#v, want 1.2", invocation["prompt_estimate_scale"])
	}
	if got := intValueFromAny(invocation["budget_saved_chars"]); got != 700 {
		t.Fatalf("budget_saved_chars = %d, want 700", got)
	}
	if got := mapFromOperationContext(invocation["budget_compression_chars"]); intValueFromAny(got["historical_tool_payloads"]) != 700 {
		t.Fatalf("budget_compression_chars = %#v", got)
	}
	if clamped, ok := invocation["budget_max_tokens_clamped"].(bool); !ok || !clamped {
		t.Fatalf("budget_max_tokens_clamped = %#v, want true", invocation["budget_max_tokens_clamped"])
	}
	encoded, err := json.Marshal(invocation)
	if err != nil {
		t.Fatalf("marshal invocation: %v", err)
	}
	if strings.Contains(string(encoded), secret) {
		t.Fatalf("prompt diagnostics leaked request content: %s", string(encoded))
	}
}
