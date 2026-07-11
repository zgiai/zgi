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
