package service

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestRunPreparedSkillStreamUsesMainModelTerminalDecisionForLegacyOnlyEvidence(t *testing.T) {
	ctx := context.Background()
	catalogDir := t.TempDir()
	writePostVerifierServiceTestSkill(t, catalogDir, "post-verifier-test")

	llm := &toolGovernanceStreamLLM{
		appChatResponses: []*adapter.ChatResponse{
			postVerifierServiceTestChatResponse("The operation is complete."),
			postVerifierServiceTestChatResponse(`{"status":"pass","reason":"candidate answer is supported by operation plan evidence","missing_steps":[],"unsupported_claims":[],"next_action_hint":"","final_answer":"","final_answer_guidance":""}`),
		},
	}
	runtime := skills.NewRuntimeWithCatalog(nil, nil, catalogDir)
	svc := NewServiceWithSkillRuntime(nil, llm, nil, nil, nil, nil, nil, runtime, nil).(*service)

	conversationID := uuid.New()
	messageID := uuid.New()
	organizationID := uuid.New()
	accountID := uuid.New()
	parts := &chatRequestParts{
		Query:     "Confirm the already completed operation from plan evidence.",
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs:  []string{"post-verifier-test"},
		Provider:  "deepseek",
	}
	metadata := streamingMessageMetadataWithTaskID(parts, messageID.String())
	metadata["operation_plan"] = map[string]interface{}{
		"version":             operationPlanVersion,
		"task_id":             messageID.String(),
		"status":              operationPlanStatusCompleted,
		"pending_next_action": "none",
		"steps":               []interface{}{},
		"step_status":         map[string]interface{}{},
	}
	metadata["operation_ledger"] = map[string]interface{}{
		"version": operationLedgerVersion,
		"status":  operationLedgerStatusObserved,
		"resources": []map[string]interface{}{{
			"id":   "file-1",
			"type": "file",
			"name": "visible.md",
		}},
		"capabilities": []map[string]interface{}{{
			"id":   "file.read",
			"name": "read_file",
		}},
		"risk_summary": map[string]interface{}{
			"level": "low",
		},
	}
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{
			ID:             conversationID,
			OrganizationID: organizationID,
			AccountID:      accountID,
		},
		Message: &runtimemodel.Message{
			ID:             messageID,
			ConversationID: conversationID,
			Metadata:       metadata,
		},
		LLMRequest: &adapter.ChatRequest{
			Messages: []adapter.Message{{Role: "user", Content: parts.Query}},
		},
		Scope: Scope{
			OrganizationID: organizationID,
			AccountID:      accountID,
		},
		parts: parts,
	}

	answer, _, err := svc.runPreparedSkillStream(ctx, ctx, prepared, nil, nil)
	if err != nil {
		t.Fatalf("runPreparedSkillStream() error = %v", err)
	}
	if answer != "The operation is complete." {
		t.Fatalf("answer = %q, want verifier-approved candidate answer", answer)
	}
	if len(llm.appChatRequests) != 1 {
		t.Fatalf("AppChat requests = %d, want one main-model terminal decision", len(llm.appChatRequests))
	}
	if len(llm.streamRequests) != 1 {
		t.Fatalf("AppChatStream requests = %d, want the main-model answer to stream normally", len(llm.streamRequests))
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	verification := mapFromOperationContext(plan["completion_verification"])
	if got := stringFromAny(verification["status"]); got != "pass" {
		t.Fatalf("completion_verification.status = %q, want pass", got)
	}
	if got := stringFromAny(verification["source"]); got != "main_model_final" {
		t.Fatalf("completion_verification.source = %q, want main_model_final", got)
	}
	if got := strings.TrimSpace(stringFromAny(prepared.Message.Metadata["guardrail_count"])); got != "" && got != "0" {
		t.Fatalf("guardrail_count = %s, want no legacy guardrail traces when post verifier is configured", got)
	}
	for _, invocation := range skillInvocationsFromMetadata(prepared.Message.Metadata["skill_invocations"]) {
		if kind := strings.TrimSpace(stringFromAny(invocation["kind"])); kind == "guardrail" {
			t.Fatalf("skill_invocations contains legacy guardrail trace: %#v", invocation)
		}
	}
}

func TestRunPreparedSkillStreamAcceptsExplicitFinalAnswerWithoutClosingModelPlanPhases(t *testing.T) {
	ctx := context.Background()
	catalogDir := t.TempDir()
	writePostVerifierServiceTestSkill(t, catalogDir, "post-verifier-test")

	llm := &toolGovernanceStreamLLM{
		appChatResponses: []*adapter.ChatResponse{
			postVerifierServiceTestFinalAnswerResponse("Agent configuration was updated.", ""),
		},
	}
	runtime := skills.NewRuntimeWithCatalog(nil, nil, catalogDir)
	svc := NewServiceWithSkillRuntime(nil, llm, nil, nil, nil, nil, nil, runtime, nil).(*service)

	conversationID := uuid.New()
	messageID := uuid.New()
	organizationID := uuid.New()
	accountID := uuid.New()
	updateStepID := operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")
	parts := &chatRequestParts{
		Query:     "Update the current Agent model and system prompt.",
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs:  []string{"post-verifier-test"},
		Provider:  "deepseek",
	}
	metadata := streamingMessageMetadataWithTaskID(parts, messageID.String())
	metadata["operation_plan"] = map[string]interface{}{
		"version":             operationPlanVersion,
		"task_id":             messageID.String(),
		"status":              operationPlanStatusRunning,
		"pending_next_action": "Run tool:agent-management/update_agent_config",
		"phases": []interface{}{
			map[string]interface{}{
				"id":     "phase-update",
				"step":   "Update the Agent configuration",
				"status": operationPlanStepStatusPending,
			},
		},
		"steps": []interface{}{
			map[string]interface{}{
				"id":        updateStepID,
				"status":    operationPlanStepStatusPending,
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "update_agent_config",
			},
		},
		"step_status": map[string]interface{}{
			updateStepID: operationPlanStepStatusPending,
		},
	}
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{
			ID:             conversationID,
			OrganizationID: organizationID,
			AccountID:      accountID,
		},
		Message: &runtimemodel.Message{
			ID:             messageID,
			ConversationID: conversationID,
			Metadata:       metadata,
		},
		LLMRequest: &adapter.ChatRequest{
			Messages: []adapter.Message{{Role: "user", Content: parts.Query}},
		},
		Scope: Scope{
			OrganizationID: organizationID,
			AccountID:      accountID,
		},
		parts: parts,
	}

	answer, _, err := svc.runPreparedSkillStream(ctx, ctx, prepared, nil, nil)
	if err != nil {
		t.Fatalf("runPreparedSkillStream() error = %v", err)
	}
	if answer != "Agent configuration was updated." {
		t.Fatalf("answer = %q, want the explicit main-model terminal answer", answer)
	}
	if len(llm.appChatRequests) != 1 {
		t.Fatalf("AppChat requests = %d, want one explicit final-answer decision", len(llm.appChatRequests))
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	if got := stringFromAny(plan["status"]); got != operationPlanStatusCompleted {
		t.Fatalf("operation_plan status = %q, want completed after truthful main-model final; plan=%#v", got, plan)
	}
	if got := stringFromAny(plan["pending_next_action"]); got != "none" {
		t.Fatalf("pending_next_action = %q, want none; plan=%#v", got, plan)
	}
	if got := operationPlanStepStatusForTest(plan, updateStepID); got != operationPlanStepStatusPending {
		t.Fatalf("step status = %q, want preserved pending model state; plan=%#v", got, plan)
	}
	if phase := mapSliceFromAny(plan["phases"])[0]; stringFromAny(phase["status"]) != operationPlanStepStatusPending {
		t.Fatalf("phase status = %#v, want preserved pending model state", phase["status"])
	}
	verification := mapFromOperationContext(plan["completion_verification"])
	if got := stringFromAny(verification["status"]); got != "pass" {
		t.Fatalf("completion_verification.status = %q, want pass", got)
	}
	if got := stringFromAny(verification["source"]); got != "main_model_final" {
		t.Fatalf("completion_verification.source = %q, want main_model_final", got)
	}
}

func TestRunPreparedSkillStreamDoesNotUseLegacyFinalAnswerGuardForConsoleFileDelete(t *testing.T) {
	ctx := context.Background()
	llm := &toolGovernanceStreamLLM{
		appChatResponses: []*adapter.ChatResponse{
			postVerifierServiceTestChatResponse("The file has been deleted."),
			postVerifierServiceTestChatResponse(`{"status":"pass","reason":"candidate answer is supported by delete_file ledger evidence","missing_steps":[],"unsupported_claims":[],"next_action_hint":"","final_answer":"","final_answer_guidance":""}`),
		},
	}
	runtime := skills.NewRuntimeWithCatalog(nil, nil, filepath.Join("..", "..", "..", "modules", "skills", "catalog"))
	svc := NewServiceWithSkillRuntime(nil, llm, nil, nil, nil, nil, nil, runtime, nil).(*service)

	conversationID := uuid.New()
	messageID := uuid.New()
	organizationID := uuid.New()
	accountID := uuid.New()
	parts := consoleFilesSnapshotTestParts("delete the first file", []consoleFilesTestFile{
		{ID: "file-1", Name: "invoice.xlsx", Extension: "xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
	})
	parts.Surface = aiChatSurfaceContextualSidebar
	parts.SkillMode = skillModeAuto
	parts.SkillIDs = []string{skills.SkillFileManager}
	parts.Provider = "deepseek"

	deleteStepID := operationPlanToolStepID(skills.SkillFileManager, "delete_file")
	metadata := streamingMessageMetadataWithTaskID(parts, messageID.String())
	metadata["operation_plan"] = map[string]interface{}{
		"version":             operationPlanVersion,
		"task_id":             messageID.String(),
		"status":              operationPlanStatusCompleted,
		"pending_next_action": "none",
		"steps": []interface{}{
			map[string]interface{}{
				"id":        deleteStepID,
				"status":    operationPlanStepStatusCompleted,
				"skill_id":  skills.SkillFileManager,
				"tool_name": "delete_file",
				"required_evidence": []interface{}{
					"tool_call.status=success",
					"file_id",
					"file_name",
				},
			},
		},
		"step_status": map[string]interface{}{
			deleteStepID: operationPlanStepStatusCompleted,
		},
	}
	metadata["skill_invocations"] = []interface{}{
		map[string]interface{}{
			"kind":      "tool_call",
			"status":    "success",
			"skill_id":  skills.SkillFileManager,
			"tool_name": "delete_file",
			"arguments": map[string]interface{}{
				"file_id": "file-1",
			},
			"result": map[string]interface{}{
				"status":    "completed",
				"effect":    "deleted",
				"file_id":   "file-1",
				"file_name": "invoice.xlsx",
			},
		},
	}
	metadata["operation_ledger"] = map[string]interface{}{
		"version": operationLedgerVersion,
		"status":  operationLedgerStatusObserved,
		"resources": []map[string]interface{}{
			{
				"id":     "file-1",
				"type":   "file",
				"name":   "invoice.xlsx",
				"status": "deleted",
			},
		},
	}
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{
			ID:             conversationID,
			OrganizationID: organizationID,
			AccountID:      accountID,
		},
		Message: &runtimemodel.Message{
			ID:             messageID,
			ConversationID: conversationID,
			Metadata:       metadata,
		},
		LLMRequest: &adapter.ChatRequest{
			Messages: []adapter.Message{{Role: "user", Content: parts.Query}},
		},
		Scope: Scope{
			OrganizationID: organizationID,
			AccountID:      accountID,
		},
		parts: parts,
	}

	answer, _, err := svc.runPreparedSkillStream(ctx, ctx, prepared, nil, nil)
	if err != nil {
		t.Fatalf("runPreparedSkillStream() error = %v", err)
	}
	if answer != "The file has been deleted." {
		t.Fatalf("answer = %q, want verifier-approved delete answer", answer)
	}
	if len(llm.appChatRequests) != 1 {
		t.Fatalf("AppChat requests = %d, want no routine verifier after canonical delete evidence", len(llm.appChatRequests))
	}
	for index, request := range llm.appChatRequests {
		if toolGovernanceStreamRequestContains(request, "Runtime guardrail feedback") {
			t.Fatalf("AppChat request %d contains legacy guardrail feedback: %q", index, toolGovernanceStreamRequestText(request))
		}
	}
	if got := strings.TrimSpace(stringFromAny(prepared.Message.Metadata["guardrail_count"])); got != "" && got != "0" {
		t.Fatalf("guardrail_count = %s, want no legacy guardrail traces when post verifier is configured", got)
	}
	for _, invocation := range skillInvocationsFromMetadata(prepared.Message.Metadata["skill_invocations"]) {
		if kind := strings.TrimSpace(stringFromAny(invocation["kind"])); kind == "guardrail" {
			t.Fatalf("skill_invocations contains legacy guardrail trace: %#v", invocation)
		}
	}
}

func TestRunPreparedSkillStreamKeepsMainModelAnswerWithoutRecheckingFailedEvidence(t *testing.T) {
	ctx := context.Background()
	llm := &toolGovernanceStreamLLM{
		appChatResponses: []*adapter.ChatResponse{
			postVerifierServiceTestChatResponse("The file has been saved to File Management."),
			postVerifierServiceTestChatResponse(`{"status":"pass","reason":"candidate answer looks complete","missing_steps":[],"unsupported_claims":[],"next_action_hint":"","final_answer":"","final_answer_guidance":""}`),
		},
	}
	runtime := skills.NewRuntimeWithCatalog(nil, nil, filepath.Join("..", "..", "..", "modules", "skills", "catalog"))
	svc := NewServiceWithSkillRuntime(nil, llm, nil, nil, nil, nil, nil, runtime, nil).(*service)

	conversationID := uuid.New()
	messageID := uuid.New()
	organizationID := uuid.New()
	accountID := uuid.New()
	parts := &chatRequestParts{
		Query:     "Save the generated file into File Management.",
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs:  []string{skills.SkillFileManager},
		Provider:  "deepseek",
	}
	saveStepID := operationPlanToolStepID(skills.SkillFileManager, "save_file_to_management")
	metadata := streamingMessageMetadataWithTaskID(parts, messageID.String())
	metadata["operation_plan"] = map[string]interface{}{
		"version":             operationPlanVersion,
		"task_id":             messageID.String(),
		"status":              operationPlanStatusFailed,
		"pending_next_action": "none",
		"steps": []interface{}{
			map[string]interface{}{
				"id":        saveStepID,
				"status":    operationPlanStepStatusFailed,
				"skill_id":  skills.SkillFileManager,
				"tool_name": "save_file_to_management",
			},
		},
		"step_status": map[string]interface{}{
			saveStepID: operationPlanStepStatusFailed,
		},
	}
	metadata["skill_invocations"] = []interface{}{
		map[string]interface{}{
			"kind":      "tool_call",
			"status":    "error",
			"skill_id":  skills.SkillFileManager,
			"tool_name": "save_file_to_management",
			"error":     "workspace permission denied",
			"result": map[string]interface{}{
				"status":     "error",
				"error":      "workspace permission denied",
				"error_code": "permission_denied",
			},
		},
	}
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{
			ID:             conversationID,
			OrganizationID: organizationID,
			AccountID:      accountID,
		},
		Message: &runtimemodel.Message{
			ID:             messageID,
			ConversationID: conversationID,
			Metadata:       metadata,
		},
		LLMRequest: &adapter.ChatRequest{
			Messages: []adapter.Message{{Role: "user", Content: parts.Query}},
		},
		Scope: Scope{
			OrganizationID: organizationID,
			AccountID:      accountID,
		},
		parts: parts,
	}

	answer, _, err := svc.runPreparedSkillStream(ctx, ctx, prepared, nil, nil)
	if err != nil {
		t.Fatalf("runPreparedSkillStream() error = %v", err)
	}
	if answer != "The file has been saved to File Management." {
		t.Fatalf("answer = %q, want verifier-approved main-model candidate", answer)
	}
	if len(llm.appChatRequests) != 1 {
		t.Fatalf("AppChat requests = %d, want one main-model terminal decision", len(llm.appChatRequests))
	}
}

func TestRunPreparedSkillStreamKeepsMainModelAnswerWithoutRouteRecheck(t *testing.T) {
	ctx := context.Background()
	llm := &toolGovernanceStreamLLM{
		appChatResponses: []*adapter.ChatResponse{
			postVerifierServiceTestChatResponse("You are now on the Files page."),
			postVerifierServiceTestChatResponse(`{"status":"pass","reason":"candidate answer looks complete","missing_steps":[],"unsupported_claims":[],"next_action_hint":"","final_answer":"","final_answer_guidance":""}`),
		},
	}
	runtime := skills.NewRuntimeWithCatalog(nil, nil, filepath.Join("..", "..", "..", "modules", "skills", "catalog"))
	svc := NewServiceWithSkillRuntime(nil, llm, nil, nil, nil, nil, nil, runtime, nil).(*service)

	conversationID := uuid.New()
	messageID := uuid.New()
	organizationID := uuid.New()
	accountID := uuid.New()
	parts := &chatRequestParts{
		Query:     "Open the Files page.",
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs:  []string{skills.SkillConsoleNavigator},
		Provider:  "deepseek",
	}
	navigateStepID := operationPlanToolStepID(skills.SkillConsoleNavigator, "navigate")
	metadata := streamingMessageMetadataWithTaskID(parts, messageID.String())
	metadata["operation_plan"] = map[string]interface{}{
		"version":             operationPlanVersion,
		"task_id":             messageID.String(),
		"status":              operationPlanStatusFailed,
		"pending_next_action": "none",
		"steps": []interface{}{
			map[string]interface{}{
				"id":        navigateStepID,
				"status":    operationPlanStepStatusFailed,
				"skill_id":  skills.SkillConsoleNavigator,
				"tool_name": "navigate",
			},
		},
		"step_status": map[string]interface{}{
			navigateStepID: operationPlanStepStatusFailed,
		},
	}
	metadata["client_actions"] = []interface{}{
		map[string]interface{}{
			"kind":        "client_action",
			"status":      "failed",
			"skill_id":    skills.SkillConsoleNavigator,
			"tool_name":   "navigate",
			"action_type": "route_navigation",
			"result": map[string]interface{}{
				"status": "failed",
				"error":  "route did not finish loading",
			},
		},
	}
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{
			ID:             conversationID,
			OrganizationID: organizationID,
			AccountID:      accountID,
		},
		Message: &runtimemodel.Message{
			ID:             messageID,
			ConversationID: conversationID,
			Metadata:       metadata,
		},
		LLMRequest: &adapter.ChatRequest{
			Messages: []adapter.Message{{Role: "user", Content: parts.Query}},
		},
		Scope: Scope{
			OrganizationID: organizationID,
			AccountID:      accountID,
		},
		parts: parts,
	}

	answer, _, err := svc.runPreparedSkillStream(ctx, ctx, prepared, nil, nil)
	if err != nil {
		t.Fatalf("runPreparedSkillStream() error = %v", err)
	}
	if answer != "You are now on the Files page." {
		t.Fatalf("answer = %q, want verifier-approved main-model candidate", answer)
	}
	if len(llm.appChatRequests) != 1 {
		t.Fatalf("AppChat requests = %d, want one main-model terminal decision", len(llm.appChatRequests))
	}
}

func writePostVerifierServiceTestSkill(t *testing.T, catalogDir string, skillID string) {
	t.Helper()
	root := filepath.Join(catalogDir, skillID)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir skill root: %v", err)
	}
	content := `---
name: post-verifier-test
description: Test-only prompt skill for AIChat post verifier service tests.
when_to_use: Use in post verifier service tests.
runtime_type: prompt
---

# Post Verifier Test

Use only the provided evidence.
`
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
}

func postVerifierServiceTestChatResponse(content string) *adapter.ChatResponse {
	return &adapter.ChatResponse{
		Choices: []adapter.Choice{{
			Message: adapter.Message{Role: "assistant", Content: content},
		}},
	}
}

func postVerifierServiceTestFinalAnswerResponse(answer string, planJSON string) *adapter.ChatResponse {
	arguments := `{"answer":` + strconv.Quote(answer)
	if strings.TrimSpace(planJSON) != "" {
		arguments += `,"plan":` + planJSON
	}
	arguments += `}`
	return &adapter.ChatResponse{
		Choices: []adapter.Choice{{
			Message: adapter.Message{
				Role: "assistant",
				ToolCalls: []adapter.ToolCall{{
					ID:   "final-answer",
					Type: "function",
					Function: adapter.FunctionCall{
						Name:      skills.MetaToolFinalAnswer,
						Arguments: arguments,
					},
				}},
			},
		}},
	}
}
