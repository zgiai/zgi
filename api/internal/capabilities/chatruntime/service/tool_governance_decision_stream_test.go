package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/dto"
	filemodel "github.com/zgiai/zgi/api/internal/modules/file_process/model"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	builtinconsolenavigation "github.com/zgiai/zgi/api/internal/modules/tools/builtin/consolenavigation"
	builtinfiles "github.com/zgiai/zgi/api/internal/modules/tools/builtin/files"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

func TestToolGovernanceContinuationPlanStateSummaryTracksPendingAndCompletedSteps(t *testing.T) {
	message := &runtimemodel.Message{Metadata: map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": "in_progress",
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "tool:agent-management/delete_agent",
					"status":    operationPlanStepStatusCompleted,
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "delete_agent",
					"arguments": map[string]interface{}{"agent_name": "\u65e7\u667a\u80fd\u4f53"},
				},
				map[string]interface{}{
					"id":        "tool:agent-management/create_agent",
					"status":    operationPlanStepStatusPending,
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "create_agent",
					"arguments": map[string]interface{}{"agent_name": "\u5c0f\u8bf4\u521b\u4f5c\u5927\u5e08"},
				},
			},
			"completion_verification": map[string]interface{}{
				"status": "needs_action",
				"reason": "create_agent has not completed yet",
			},
		},
	}}

	summary := toolGovernanceContinuationPlanStateSummary(message)
	completed := mapSliceFromAny(summary["completed_steps"])
	if len(completed) != 1 || strings.TrimSpace(stringFromAny(completed[0]["tool_name"])) != "delete_agent" {
		t.Fatalf("completed_steps = %#v, want delete_agent completed; summary=%#v", completed, summary)
	}
	pending := mapSliceFromAny(summary["pending_steps"])
	if len(pending) != 1 || strings.TrimSpace(stringFromAny(pending[0]["tool_name"])) != "create_agent" {
		t.Fatalf("pending_steps = %#v, want create_agent pending", pending)
	}
	if _, ok := summary["last_completion_verification"]; !ok {
		t.Fatalf("last_completion_verification missing for pending plan; summary=%#v", summary)
	}
}

func TestToolGovernanceContinuationPlanStateSummaryHidesStaleNeedsActionWhenComplete(t *testing.T) {
	message := &runtimemodel.Message{Metadata: map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": "completed",
			"steps": []interface{}{
				map[string]interface{}{
					"id":        "tool:agent-management/update_agent_config",
					"status":    operationPlanStepStatusCompleted,
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "update_agent_config",
				},
			},
			"completion_verification": map[string]interface{}{
				"status":        "needs_action",
				"missing_steps": []interface{}{"agent-management/get_agent_config"},
			},
		},
	}}

	summary := toolGovernanceContinuationPlanStateSummary(message)
	if completed, ok := summary["all_required_steps_completed"].(bool); !ok || !completed {
		t.Fatalf("all_required_steps_completed = %#v, want true; summary=%#v", summary["all_required_steps_completed"], summary)
	}
	if _, ok := summary["last_completion_verification"]; ok {
		t.Fatalf("last_completion_verification = %#v, want stale needs_action omitted", summary["last_completion_verification"])
	}
}

func TestRunToolGovernanceDecisionStreamRejectsWithoutTools(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	accountID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()
	provider := "deepseek"
	now := time.Now().UTC()

	conversation := &runtimemodel.Conversation{
		ID:                   conversationID,
		OrganizationID:       organizationID,
		AccountID:            accountID,
		CallerType:           runtimemodel.ConversationCallerAIChat,
		Title:                "Files",
		Status:               runtimemodel.ConversationStatusNormal,
		RuntimeStatus:        runtimemodel.ConversationRuntimeStatusIdle,
		CurrentLeafMessageID: &messageID,
		Metadata:             map[string]interface{}{},
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	message := &runtimemodel.Message{
		ID:              messageID,
		ConversationID:  conversationID,
		Query:           "Delete the selected report file",
		Status:          runtimemodel.MessageStatusWaitingApproval,
		ModelProvider:   &provider,
		ModelName:       "deepseek-chat",
		ModelParameters: map[string]interface{}{"temperature": 0.2},
		Metadata:        pendingToolGovernanceDecisionMetadata("corr-1"),
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	messageRepo := &toolGovernanceStreamMessageRepo{message: message}
	conversationRepo := &toolGovernanceStreamConversationRepo{conversation: conversation}
	llm := &toolGovernanceStreamLLM{
		streamChunks: []string{"I kept the file. ", "No deletion was performed."},
	}
	svc := NewService(&repository.Repositories{
		Access:       toolGovernanceStreamAccessRepo{},
		Conversation: conversationRepo,
		Message:      messageRepo,
	}, llm).(*service)
	svc.events = newStreamEventStore(nil)

	var events []StreamEvent
	result, err := svc.RunToolGovernanceDecisionStream(
		ctx,
		Scope{OrganizationID: organizationID, AccountID: accountID},
		conversationID,
		messageID,
		"corr-1",
		runtimedto.ToolGovernanceDecisionRequest{Action: "reject", Reason: "keep it for audit"},
		func(event StreamEvent) error {
			events = append(events, event)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("RunToolGovernanceDecisionStream() error = %v", err)
	}
	if result.Status != runtimemodel.MessageStatusCompleted {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	if result.Answer != "I kept the file. No deletion was performed." {
		t.Fatalf("result answer = %q", result.Answer)
	}
	if !messageRepo.updateMetadataAnyStatusCalled {
		t.Fatal("UpdateMetadataAnyStatus was not called for the reject decision")
	}
	if !messageRepo.updateCompletedCalled {
		t.Fatal("UpdateCompleted was not called")
	}
	if !conversationRepo.updateAfterMessageCalled {
		t.Fatal("UpdateAfterMessage was not called")
	}
	if message.Status != runtimemodel.MessageStatusCompleted || message.Answer != result.Answer {
		t.Fatalf("message = status %q answer %q, want completed result answer", message.Status, message.Answer)
	}
	if conversation.RuntimeStatus != runtimemodel.ConversationRuntimeStatusIdle || conversation.ActiveMessageID != nil {
		t.Fatalf("conversation runtime = %q active %#v, want idle with no active message", conversation.RuntimeStatus, conversation.ActiveMessageID)
	}
	if conversation.CurrentLeafMessageID == nil || *conversation.CurrentLeafMessageID != messageID {
		t.Fatalf("conversation current leaf = %#v, want %s", conversation.CurrentLeafMessageID, messageID)
	}

	if len(llm.streamRequests) != 1 {
		t.Fatalf("AppChatStream calls = %d, want 1", len(llm.streamRequests))
	}
	streamReq := llm.streamRequests[0]
	if !streamReq.Stream {
		t.Fatal("reject continuation request is not streaming")
	}
	if len(streamReq.Tools) != 0 || len(streamReq.Functions) != 0 {
		t.Fatalf("reject continuation request tools = %#v functions = %#v, want none", streamReq.Tools, streamReq.Functions)
	}
	if streamReq.Provider != "deepseek" || streamReq.Model != "deepseek-chat" {
		t.Fatalf("request identity = provider %q model %q", streamReq.Provider, streamReq.Model)
	}
	if streamReq.Temperature == nil || *streamReq.Temperature != 0.2 {
		t.Fatalf("request temperature = %#v, want 0.2", streamReq.Temperature)
	}
	requestText := toolGovernanceStreamRequestText(streamReq)
	for _, want := range []string{
		"Do not execute or claim the rejected action",
		"Answer in the user's language",
		"Delete the selected report file",
		"keep it for audit",
		"not_executed",
	} {
		if !strings.Contains(requestText, want) {
			t.Fatalf("reject continuation request missing %q in %q", want, requestText)
		}
	}
	for _, hidden := range []string{
		"corr-1",
		"Rejected governance event JSON",
	} {
		if strings.Contains(requestText, hidden) {
			t.Fatalf("reject continuation request exposed %q in %q", hidden, requestText)
		}
	}

	metadataEvent, ok := toolGovernanceDecisionEventFromMetadata(message.Metadata, "corr-1")
	if !ok {
		t.Fatalf("tool governance decision not persisted in metadata: %#v", message.Metadata)
	}
	if metadataEvent["approval_status"] != "rejected" {
		t.Fatalf("approval_status = %#v, want rejected", metadataEvent["approval_status"])
	}
	governance := governanceMapFromAny(metadataEvent["governance"])
	if governance["approval_status"] != "rejected" || governance["requires_approval"] != false {
		t.Fatalf("governance = %#v, want rejected and no longer requiring approval", governance)
	}
	approvalResult := governanceMapFromAny(governance["approval_result"])
	modelFeedback := governanceMapFromAny(approvalResult["model_feedback"])
	if modelFeedback["status"] != "user_rejected" {
		t.Fatalf("model feedback = %#v, want user_rejected", modelFeedback)
	}
	if grants := mapSliceFromAny(message.Metadata["tool_governance_one_shot_grants"]); len(grants) != 0 {
		t.Fatalf("one-shot grants = %#v, want none for rejection", grants)
	}
	if grants := mapSliceFromAny(conversation.Metadata["tool_governance_session_grants"]); len(grants) != 0 {
		t.Fatalf("session grants = %#v, want none for rejection", grants)
	}
	continuation := governanceMapFromAny(message.Metadata["tool_governance_continuation"])
	if continuation["status"] != "completed" || continuation["approval_status"] != "rejected" {
		t.Fatalf("tool_governance_continuation = %#v, want completed rejected", continuation)
	}
	if continuation["action"] != "reject" || continuation["reason"] != "keep it for audit" {
		t.Fatalf("tool_governance_continuation = %#v, want rejected action and reason", continuation)
	}

	assertToolGovernanceStreamEvents(t, events)
}

func TestRunToolGovernanceDecisionStreamApproveExecutesBuiltinDeleteBeforeAnswer(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	accountID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()
	provider := "deepseek"
	now := time.Now().UTC()

	metadata := pendingToolGovernanceDecisionMetadata("corr-approve")
	metadata["configured_skill_ids"] = []interface{}{skills.SkillFileManager}
	invocation := metadata["skill_invocations"].([]interface{})[0].(map[string]interface{})
	governance := invocation["governance"].(map[string]interface{})
	approvalEvent := governance["approval_event"].(map[string]interface{})
	approvalEvent["assets"] = []interface{}{
		map[string]interface{}{
			"id":           "file-1",
			"type":         "file",
			"name":         "report.pdf",
			"workspace_id": "workspace-1",
		},
	}
	approvalEvent["grant"] = map[string]interface{}{
		"conversation_id": conversationID.String(),
		"organization_id": organizationID.String(),
		"user_id":         accountID.String(),
		"skill_id":        skills.SkillFileManager,
		"provider_type":   "builtin",
		"provider_id":     "files",
		"tool_id":         "file.delete",
		"effect":          "delete",
		"asset_type":      "file",
		"risk_level":      "high",
		"assets": []interface{}{
			map[string]interface{}{"id": "file-1", "type": "file", "name": "report.pdf"},
		},
	}
	approvalEvent["frozen_invocation"] = toolgovernance.NewFrozenInvocation(toolgovernance.FrozenInvocationRequest{
		CorrelationID: "corr-approve",
		Manifest: toolgovernance.Manifest{
			ToolID:    "file.delete",
			SkillID:   skills.SkillFileManager,
			Effect:    toolgovernance.EffectDelete,
			AssetType: "file",
			RiskLevel: toolgovernance.RiskLevelHigh,
		},
		SkillID:      skills.SkillFileManager,
		ToolName:     "delete_file",
		ProviderType: "builtin",
		ProviderID:   "files",
		Arguments: map[string]interface{}{
			"file_id": "file-1",
		},
		Assets: []toolgovernance.AssetRef{
			{ID: "file-1", Type: "file", Name: "report.pdf", WorkspaceID: "workspace-1"},
		},
		Now: now,
		TTL: 7 * 24 * time.Hour,
	})

	conversation := &runtimemodel.Conversation{
		ID:             conversationID,
		OrganizationID: organizationID,
		AccountID:      accountID,
		CallerType:     runtimemodel.ConversationCallerAIChat,
		Title:          "Files",
		Status:         runtimemodel.ConversationStatusNormal,
		RuntimeStatus:  runtimemodel.ConversationRuntimeStatusIdle,
		Metadata:       map[string]interface{}{},
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	message := &runtimemodel.Message{
		ID:              messageID,
		ConversationID:  conversationID,
		Query:           "Delete report.pdf",
		Status:          runtimemodel.MessageStatusWaitingApproval,
		ModelProvider:   &provider,
		ModelName:       "deepseek-chat",
		ModelParameters: map[string]interface{}{},
		Metadata:        metadata,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	conversation.CurrentLeafMessageID = &messageID

	workspaceID := "workspace-1"
	fileService := &toolGovernanceStreamFileService{
		files: map[string]*dto.UploadFile{
			"file-1": {
				ID:             "file-1",
				OrganizationID: organizationID.String(),
				WorkspaceID:    &workspaceID,
				Name:           "report.pdf",
				Extension:      "pdf",
				MimeType:       "application/pdf",
				CreatedBy:      accountID.String(),
				CreatedAt:      now,
			},
		},
	}
	workspacePerms := &toolGovernanceStreamWorkspacePermissionService{allowed: true}
	runtime := newToolGovernanceStreamSkillRuntime(t, fileService, workspacePerms)
	llm := &toolGovernanceStreamLLM{
		streamChunks: []string{"Deleted report.pdf."},
	}
	messageRepo := &toolGovernanceStreamMessageRepo{message: message}
	conversationRepo := &toolGovernanceStreamConversationRepo{conversation: conversation}
	svc := NewServiceWithSkillRuntime(
		&repository.Repositories{
			Access:       toolGovernanceStreamAccessRepo{},
			Conversation: conversationRepo,
			Message:      messageRepo,
			SkillConfig:  toolGovernanceStreamSkillConfigRepo{skillID: skills.SkillFileManager},
		},
		llm,
		nil,
		toolGovernanceStreamModelSpecResolver{},
		nil,
		nil,
		nil,
		runtime,
		nil,
	).(*service)
	svc.events = newStreamEventStore(nil)

	var events []StreamEvent
	result, err := svc.RunToolGovernanceDecisionStream(
		ctx,
		Scope{OrganizationID: organizationID, AccountID: accountID},
		conversationID,
		messageID,
		"corr-approve",
		runtimedto.ToolGovernanceDecisionRequest{Action: "approve"},
		func(event StreamEvent) error {
			events = append(events, event)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("RunToolGovernanceDecisionStream() error = %v", err)
	}
	if result.Status != runtimemodel.MessageStatusCompleted {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	if result.Answer != "已删除文件「report.pdf」。" {
		t.Fatalf("result answer = %q, want fast-path delete confirmation", result.Answer)
	}
	if len(fileService.deleted) != 1 || fileService.deleted[0] != "file-1" {
		t.Fatalf("deleted files = %#v, want one delete for approved file-1", fileService.deleted)
	}
	if len(workspacePerms.codes) != 1 || workspacePerms.codes[0] != workspacemodel.WorkspacePermissionFileManage {
		t.Fatalf("workspace permission checks = %#v, want file manage check", workspacePerms.codes)
	}
	if len(llm.appChatRequests) != 0 {
		t.Fatalf("AppChat requests = %d, want no model tool-planning calls", len(llm.appChatRequests))
	}
	if len(llm.streamRequests) != 0 {
		t.Fatalf("AppChatStream requests = %d, want no summary call after fast path", len(llm.streamRequests))
	}

	metadataEvent, ok := toolGovernanceDecisionEventFromMetadata(message.Metadata, "corr-approve")
	if !ok {
		t.Fatalf("tool governance decision not persisted in metadata: %#v", message.Metadata)
	}
	if metadataEvent["approval_status"] != "approved" {
		t.Fatalf("approval_status = %#v, want approved", metadataEvent["approval_status"])
	}
	if grants := mapSliceFromAny(message.Metadata["tool_governance_one_shot_grants"]); len(grants) != 1 {
		t.Fatalf("one-shot grants = %#v, want one approved grant", grants)
	}
	if grants := mapSliceFromAny(conversation.Metadata["tool_governance_session_grants"]); len(grants) != 0 {
		t.Fatalf("session grants = %#v, want none without remember_for_session", grants)
	}
	if guardrails := intValueFromAny(message.Metadata["guardrail_count"]); guardrails != 0 {
		t.Fatalf("guardrail_count = %d in %#v, want no final answer guard replan", guardrails, message.Metadata)
	}
	if toolCalls := intValueFromAny(message.Metadata["tool_call_count"]); toolCalls != 1 {
		t.Fatalf("tool_call_count = %d in %#v, want 1 from delete tool execution", toolCalls, message.Metadata)
	}
	if !toolGovernanceStreamHasInvocation(message.Metadata, "tool_call", skills.SkillFileManager, "delete_file", "success") {
		t.Fatalf("metadata skill_invocations = %#v, want successful file-manager/delete_file tool call", message.Metadata["skill_invocations"])
	}
	clientAction := governanceMapFromAny(message.Metadata["client_action_continuation"])
	if len(clientAction) != 0 {
		t.Fatalf("client_action_continuation = %#v, want no asset observation when delete fast path completes", clientAction)
	}
	continuation := governanceMapFromAny(message.Metadata["tool_governance_continuation"])
	if continuation["status"] != "completed" || continuation["approval_status"] != "approved" {
		t.Fatalf("tool_governance_continuation = %#v, want completed approved", continuation)
	}
	assertToolGovernanceApprovedStreamEvents(t, events)
}

func TestRunClientActionContinuationStreamAssetObservationUsesVerifierWithoutRepeatingTools(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	accountID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()
	provider := "deepseek"
	now := time.Now().UTC()
	actionID := "asset_observation:corr-approve"
	event := map[string]interface{}{
		"action_id":   actionID,
		"action_type": "asset_observation",
		"status":      clientActionStatusWaiting,
		"skill_id":    skills.SkillFileManager,
		"tool_name":   "delete_file",
		"effect":      "delete",
		"asset_type":  "file",
		"assets": []interface{}{
			map[string]interface{}{"type": "file", "name": "report.pdf"},
		},
	}
	metadata := mergeClientActionMetadata(map[string]interface{}{
		"surface":              "sidebar",
		"configured_skill_ids": []string{skills.SkillFileManager},
	}, event)
	metadata["client_action_continuation"] = compactSkillInvocation(event)

	conversation := &runtimemodel.Conversation{
		ID:                   conversationID,
		OrganizationID:       organizationID,
		AccountID:            accountID,
		CallerType:           runtimemodel.ConversationCallerAIChat,
		Title:                "Files",
		Status:               runtimemodel.ConversationStatusNormal,
		RuntimeStatus:        runtimemodel.ConversationRuntimeStatusIdle,
		CurrentLeafMessageID: &messageID,
		Metadata:             map[string]interface{}{},
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	message := &runtimemodel.Message{
		ID:              messageID,
		ConversationID:  conversationID,
		Query:           "Delete report.pdf",
		Status:          runtimemodel.MessageStatusWaitingClientAction,
		ModelProvider:   &provider,
		ModelName:       "deepseek-chat",
		ModelParameters: map[string]interface{}{},
		Metadata:        metadata,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	runtime := newToolGovernanceStreamSkillRuntime(t, &toolGovernanceStreamFileService{}, &toolGovernanceStreamWorkspacePermissionService{allowed: true})
	finalAnswer := "已确认 report.pdf 已从当前页面消失。"
	llm := &toolGovernanceStreamLLM{
		appChatResponses: []*adapter.ChatResponse{
			toolGovernanceStreamChatResponse(finalAnswer),
			toolGovernanceStreamChatResponse(`{"status":"pass","reason":"client action observation confirms the file is gone","unsupported_claims":[],"missing_steps":[],"next_action_hint":"","final_answer":"","final_answer_guidance":""}`),
		},
	}
	messageRepo := &toolGovernanceStreamMessageRepo{message: message}
	conversationRepo := &toolGovernanceStreamConversationRepo{conversation: conversation}
	svc := NewServiceWithSkillRuntime(
		&repository.Repositories{
			Access:       toolGovernanceStreamAccessRepo{},
			Conversation: conversationRepo,
			Message:      messageRepo,
			SkillConfig:  toolGovernanceStreamSkillConfigRepo{skillID: skills.SkillFileManager},
		},
		llm,
		nil,
		toolGovernanceStreamModelSpecResolver{},
		nil,
		nil,
		nil,
		runtime,
		nil,
	).(*service)
	svc.events = newStreamEventStore(nil)

	var events []StreamEvent
	result, err := svc.RunClientActionContinuationStream(
		ctx,
		Scope{OrganizationID: organizationID, AccountID: accountID},
		conversationID,
		messageID,
		actionID,
		runtimedto.ClientActionResultRequest{
			Status:         "succeeded",
			Surface:        "sidebar",
			RuntimeContext: "当前文件页面已经刷新，report.pdf 不再出现在文件列表中。",
			Result: map[string]interface{}{
				"observed": true,
			},
		},
		func(event StreamEvent) error {
			events = append(events, event)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("RunClientActionContinuationStream() error = %v", err)
	}
	if result.Status != runtimemodel.MessageStatusCompleted {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	if result.Answer != finalAnswer {
		t.Fatalf("result answer = %q", result.Answer)
	}
	if len(llm.appChatRequests) != 2 {
		t.Fatalf("AppChat requests = %d, want planning answer plus completion verifier", len(llm.appChatRequests))
	}
	if len(llm.streamRequests) != 0 {
		t.Fatalf("AppChatStream requests = %d, want completion-verifier skill loop to suppress direct final streaming", len(llm.streamRequests))
	}
	if !toolGovernanceStreamRequestContains(llm.appChatRequests[0], "do not repeat the same side-effecting tool") {
		t.Fatalf("skill-loop continuation request missing client action safety instruction: %q", toolGovernanceStreamRequestText(llm.appChatRequests[0]))
	}
	if !toolGovernanceStreamRequestContains(llm.appChatRequests[1], "completion post-verifier") {
		t.Fatalf("second AppChat request = %q, want completion verifier", toolGovernanceStreamRequestText(llm.appChatRequests[1]))
	}
	if !messageRepo.updateCompletedCalled {
		t.Fatal("UpdateCompleted was not called")
	}
	if message.Status != runtimemodel.MessageStatusCompleted {
		t.Fatalf("message status = %q, want completed", message.Status)
	}
	if !toolGovernanceStreamHasInvocation(message.Metadata, "client_action", skills.SkillFileManager, "delete_file", clientActionStatusSucceeded) {
		t.Fatalf("metadata skill_invocations = %#v, want succeeded client action", message.Metadata["skill_invocations"])
	}
	for _, event := range events {
		if event.EventType == streamEventSkillCallStart {
			t.Fatalf("events = %#v, want no skill call after asset observation", events)
		}
	}
}

func TestBeginClientActionContinuationRejectsDuplicateAfterClaim(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	accountID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()
	provider := "deepseek"
	actionID := "route_navigation:open-files"
	now := time.Now().UTC()
	event := map[string]interface{}{
		"action_id":   actionID,
		"action_type": "route_navigation",
		"status":      clientActionStatusWaiting,
		"skill_id":    skills.SkillConsoleNavigator,
		"tool_name":   "navigate",
		"href":        "/console/files",
	}
	metadata := mergeClientActionMetadata(map[string]interface{}{
		"surface": "contextual_sidebar",
	}, event)
	metadata["client_action_continuation"] = compactSkillInvocation(event)
	conversation := &runtimemodel.Conversation{
		ID:                   conversationID,
		OrganizationID:       organizationID,
		AccountID:            accountID,
		CallerType:           runtimemodel.ConversationCallerAIChat,
		Title:                "Files",
		Status:               runtimemodel.ConversationStatusNormal,
		RuntimeStatus:        runtimemodel.ConversationRuntimeStatusIdle,
		CurrentLeafMessageID: &messageID,
		Metadata:             map[string]interface{}{},
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	message := &runtimemodel.Message{
		ID:              messageID,
		ConversationID:  conversationID,
		Query:           "Open files",
		Status:          runtimemodel.MessageStatusWaitingClientAction,
		ModelProvider:   &provider,
		ModelName:       "deepseek-chat",
		ModelParameters: map[string]interface{}{},
		Metadata:        metadata,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	svc := &service{
		repos: &repository.Repositories{
			Access:       toolGovernanceStreamAccessRepo{},
			Conversation: &toolGovernanceStreamConversationRepo{conversation: conversation},
			Message:      &toolGovernanceStreamMessageRepo{message: message},
		},
	}

	first, err := svc.beginClientActionContinuation(
		ctx,
		Scope{OrganizationID: organizationID, AccountID: accountID},
		conversationID,
		messageID,
		actionID,
	)
	if err != nil {
		t.Fatalf("first beginClientActionContinuation() error = %v", err)
	}
	if first == nil || message.Status != runtimemodel.MessageStatusStreaming {
		t.Fatalf("first continuation did not claim message, status = %q", message.Status)
	}

	duplicate, err := svc.beginClientActionContinuation(
		ctx,
		Scope{OrganizationID: organizationID, AccountID: accountID},
		conversationID,
		messageID,
		actionID,
	)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("duplicate beginClientActionContinuation() error = %v, want ErrInvalidInput", err)
	}
	if !errors.Is(err, ErrContinuationAlreadyRunning) {
		t.Fatalf("duplicate beginClientActionContinuation() error = %v, want ErrContinuationAlreadyRunning", err)
	}
	if duplicate != nil {
		t.Fatalf("duplicate continuation = %#v, want nil", duplicate)
	}
}

func TestRunClientActionContinuationStreamAgentUpdatedRouteUsesVerifierWithoutRepeatingTools(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	accountID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()
	provider := "deepseek"
	now := time.Now().UTC()
	actionID := "route_navigation:update-agent-config"
	event := map[string]interface{}{
		"action_id":   actionID,
		"action_type": "route_navigation",
		"status":      clientActionStatusWaiting,
		"skill_id":    skills.SkillConsoleNavigator,
		"tool_name":   "navigate",
		"href":        "/console/agents/agent-1/agent",
		"label":       "Agent detail",
		"reason":      "open_updated_agent_detail",
		"result": map[string]interface{}{
			"event_type": "page_navigation_requested",
			"href":       "/console/agents/agent-1/agent",
			"label":      "Agent detail",
			"reason":     "open_updated_agent_detail",
		},
	}
	metadata := mergeClientActionMetadata(map[string]interface{}{
		"surface":              "contextual_sidebar",
		"configured_skill_ids": []string{skills.SkillAgentManagement, skills.SkillConsoleNavigator},
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":      "tool_call",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "update_agent_config",
				"status":    "success",
				"result": map[string]interface{}{
					"agent_id":       "agent-1",
					"agent_name":     "Test Agent",
					"model_provider": "deepseek",
					"model":          "deepseek-chat",
					"href":           "/console/agents/agent-1/agent",
				},
			},
		},
	}, event)
	metadata["client_action_continuation"] = compactSkillInvocation(event)

	conversation := &runtimemodel.Conversation{
		ID:                   conversationID,
		OrganizationID:       organizationID,
		AccountID:            accountID,
		CallerType:           runtimemodel.ConversationCallerAIChat,
		Title:                "Agent",
		Status:               runtimemodel.ConversationStatusNormal,
		RuntimeStatus:        runtimemodel.ConversationRuntimeStatusIdle,
		CurrentLeafMessageID: &messageID,
		Metadata:             map[string]interface{}{},
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	message := &runtimemodel.Message{
		ID:              messageID,
		ConversationID:  conversationID,
		Query:           "Replace the current Agent model with DeepSeek-Chat (V3)",
		Status:          runtimemodel.MessageStatusWaitingClientAction,
		ModelProvider:   &provider,
		ModelName:       "deepseek-chat",
		ModelParameters: map[string]interface{}{},
		Metadata:        metadata,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	finalAnswer := "已确认模型已更新为 DeepSeek-Chat (V3)。"
	llm := &toolGovernanceStreamLLM{
		appChatResponses: []*adapter.ChatResponse{
			toolGovernanceStreamChatResponse(finalAnswer),
			toolGovernanceStreamChatResponse(`{"status":"pass","reason":"agent update tool result and route refresh are present in the ledger","unsupported_claims":[],"missing_steps":[],"next_action_hint":"","final_answer":"","final_answer_guidance":""}`),
		},
	}
	messageRepo := &toolGovernanceStreamMessageRepo{message: message}
	conversationRepo := &toolGovernanceStreamConversationRepo{conversation: conversation}
	svc := NewServiceWithSkillRuntime(
		&repository.Repositories{
			Access:       toolGovernanceStreamAccessRepo{},
			Conversation: conversationRepo,
			Message:      messageRepo,
			SkillConfig:  toolGovernanceStreamSkillConfigRepo{skillID: skills.SkillAgentManagement},
		},
		llm,
		nil,
		toolGovernanceStreamModelSpecResolver{},
		nil,
		nil,
		nil,
		newToolGovernanceStreamSkillRuntime(t, &toolGovernanceStreamFileService{}, &toolGovernanceStreamWorkspacePermissionService{allowed: true}),
		nil,
	).(*service)
	svc.events = newStreamEventStore(nil)

	var events []StreamEvent
	result, err := svc.RunClientActionContinuationStream(
		ctx,
		Scope{OrganizationID: organizationID, AccountID: accountID},
		conversationID,
		messageID,
		actionID,
		runtimedto.ClientActionResultRequest{
			Status:         "succeeded",
			Surface:        "contextual_sidebar",
			RuntimeContext: "当前 Agent 详情页已经刷新，模型显示为 DeepSeek-Chat (V3)。",
			Result: map[string]interface{}{
				"loaded_href": "/console/agents/agent-1/agent",
			},
		},
		func(event StreamEvent) error {
			events = append(events, event)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("RunClientActionContinuationStream() error = %v", err)
	}
	if result.Status != runtimemodel.MessageStatusCompleted {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	if result.Answer != finalAnswer {
		t.Fatalf("result answer = %q", result.Answer)
	}
	if len(llm.appChatRequests) != 2 {
		t.Fatalf("AppChat requests = %d, want planning answer plus completion verifier", len(llm.appChatRequests))
	}
	if len(llm.streamRequests) != 0 {
		t.Fatalf("AppChatStream requests = %d, want completion-verifier skill loop to suppress direct final streaming", len(llm.streamRequests))
	}
	if !toolGovernanceStreamRequestContains(llm.appChatRequests[0], "do not call console-navigator/navigate again for the same route") {
		t.Fatalf("skill-loop continuation request missing route continuation instruction: %q", toolGovernanceStreamRequestText(llm.appChatRequests[0]))
	}
	if !toolGovernanceStreamRequestContains(llm.appChatRequests[1], "completion post-verifier") {
		t.Fatalf("second AppChat request = %q, want completion verifier", toolGovernanceStreamRequestText(llm.appChatRequests[1]))
	}
	if !messageRepo.updateCompletedCalled {
		t.Fatal("UpdateCompleted was not called")
	}
	if !toolGovernanceStreamHasInvocation(message.Metadata, "client_action", skills.SkillConsoleNavigator, "navigate", clientActionStatusSucceeded) {
		t.Fatalf("metadata skill_invocations = %#v, want succeeded route client action", message.Metadata["skill_invocations"])
	}
	for _, event := range events {
		if event.EventType == streamEventSkillCallStart {
			t.Fatalf("events = %#v, want no skill call after Agent route refresh", events)
		}
	}
}

func TestBeginToolGovernanceContinuationRejectsAlreadyStreamingMessage(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	accountID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()
	now := time.Now().UTC()

	conversation := &runtimemodel.Conversation{
		ID:                   conversationID,
		OrganizationID:       organizationID,
		AccountID:            accountID,
		CallerType:           runtimemodel.ConversationCallerAIChat,
		Status:               runtimemodel.ConversationStatusNormal,
		RuntimeStatus:        runtimemodel.ConversationRuntimeStatusStreaming,
		CurrentLeafMessageID: &messageID,
		ActiveMessageID:      &messageID,
		Metadata:             map[string]interface{}{},
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	message := &runtimemodel.Message{
		ID:             messageID,
		ConversationID: conversationID,
		Query:          "Delete report.pdf",
		Status:         runtimemodel.MessageStatusStreaming,
		Metadata:       pendingToolGovernanceDecisionMetadata("corr-approve"),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	svc := NewService(&repository.Repositories{
		Access:       toolGovernanceStreamAccessRepo{},
		Conversation: &toolGovernanceStreamConversationRepo{conversation: conversation},
		Message:      &toolGovernanceStreamMessageRepo{message: message},
	}, &toolGovernanceStreamLLM{}).(*service)

	continuation, err := svc.beginToolGovernanceContinuation(
		ctx,
		Scope{OrganizationID: organizationID, AccountID: accountID},
		conversationID,
		messageID,
		"corr-approve",
	)
	if err == nil {
		t.Fatalf("beginToolGovernanceContinuation() continuation = %#v, want already running error", continuation)
	}
	if !errors.Is(err, ErrInvalidInput) || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("beginToolGovernanceContinuation() error = %v, want already running ErrInvalidInput", err)
	}
	if !errors.Is(err, ErrContinuationAlreadyRunning) {
		t.Fatalf("beginToolGovernanceContinuation() error = %v, want ErrContinuationAlreadyRunning", err)
	}
}

func TestRunToolGovernanceDecisionStreamAlreadyRunningRecoversActiveStream(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	accountID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()
	now := time.Now().UTC()

	conversation := &runtimemodel.Conversation{
		ID:                   conversationID,
		OrganizationID:       organizationID,
		AccountID:            accountID,
		CallerType:           runtimemodel.ConversationCallerAIChat,
		Status:               runtimemodel.ConversationStatusNormal,
		RuntimeStatus:        runtimemodel.ConversationRuntimeStatusIdle,
		CurrentLeafMessageID: &messageID,
		Metadata:             map[string]interface{}{},
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	message := &runtimemodel.Message{
		ID:             messageID,
		ConversationID: conversationID,
		Query:          "Delete report.pdf",
		Status:         runtimemodel.MessageStatusWaitingApproval,
		Metadata:       pendingToolGovernanceDecisionMetadata("corr-approve"),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	messageRepo := &toolGovernanceStreamMessageRepo{message: message}
	conversationRepo := &toolGovernanceStreamConversationRepo{conversation: conversation}
	llm := &toolGovernanceStreamLLM{}
	svc := NewService(&repository.Repositories{
		Access:       toolGovernanceStreamAccessRepo{},
		Conversation: conversationRepo,
		Message:      messageRepo,
	}, llm).(*service)

	if _, err := svc.SubmitToolGovernanceDecision(
		ctx,
		Scope{OrganizationID: organizationID, AccountID: accountID},
		conversationID,
		messageID,
		"corr-approve",
		runtimedto.ToolGovernanceDecisionRequest{Action: "approve"},
	); err != nil {
		t.Fatalf("SubmitToolGovernanceDecision() error = %v", err)
	}

	conversation.RuntimeStatus = runtimemodel.ConversationRuntimeStatusStreaming
	conversation.ActiveMessageID = &messageID
	message.Status = runtimemodel.MessageStatusStreaming

	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer redisServer.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer redisClient.Close()
	svc.events = newStreamEventStore(redisClient)
	appendRecoveredContinuationEvents(t, ctx, svc.events, conversationID, messageID)

	var events []StreamEvent
	result, err := svc.RunToolGovernanceDecisionStream(
		ctx,
		Scope{OrganizationID: organizationID, AccountID: accountID},
		conversationID,
		messageID,
		"corr-approve",
		runtimedto.ToolGovernanceDecisionRequest{Action: "approve"},
		func(event StreamEvent) error {
			events = append(events, event)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("RunToolGovernanceDecisionStream() error = %v", err)
	}
	if result == nil || result.Status != runtimemodel.MessageStatusStreaming {
		t.Fatalf("result = %#v, want streaming recovery result", result)
	}
	if len(llm.appChatRequests) != 0 || len(llm.streamRequests) != 0 {
		t.Fatalf("LLM calls = app %d stream %d, want none while reconnecting active stream", len(llm.appChatRequests), len(llm.streamRequests))
	}
	if messageRepo.updateCompletedCalled || message.Status != runtimemodel.MessageStatusStreaming {
		t.Fatalf("message status = %q completedCalled=%v, want existing active stream untouched", message.Status, messageRepo.updateCompletedCalled)
	}
	assertRecoveredContinuationEvents(t, events)
}

func TestRunToolGovernanceDecisionStreamResolvedDuplicateRecoversCompletedStream(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	accountID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()
	now := time.Now().UTC()

	metadata := pendingToolGovernanceDecisionMetadata("corr-approve")
	event := map[string]interface{}{
		"correlation_id":    "corr-approve",
		"action":            toolGovernanceActionApprove,
		"approval_status":   toolGovernanceApprovalStatusApproved,
		"skill_id":          skills.SkillFileManager,
		"tool_name":         "delete_file",
		"status":            "approved",
		"resolved_at":       now.Format(time.RFC3339),
		"resolved_by":       accountID.String(),
		"requires_approval": false,
	}
	metadata = mergeToolGovernanceDecisionMetadata(metadata, event)
	metadata = resolveToolGovernanceContinuationMetadata(metadata, "corr-approve", event)

	conversation := &runtimemodel.Conversation{
		ID:                   conversationID,
		OrganizationID:       organizationID,
		AccountID:            accountID,
		CallerType:           runtimemodel.ConversationCallerAIChat,
		Status:               runtimemodel.ConversationStatusNormal,
		RuntimeStatus:        runtimemodel.ConversationRuntimeStatusIdle,
		CurrentLeafMessageID: &messageID,
		Metadata:             map[string]interface{}{},
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	message := &runtimemodel.Message{
		ID:             messageID,
		ConversationID: conversationID,
		Query:          "Delete report.pdf",
		Answer:         "Done",
		Status:         runtimemodel.MessageStatusCompleted,
		Metadata:       metadata,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	messageRepo := &toolGovernanceStreamMessageRepo{message: message}
	conversationRepo := &toolGovernanceStreamConversationRepo{conversation: conversation}
	llm := &toolGovernanceStreamLLM{}
	svc := NewService(&repository.Repositories{
		Access:       toolGovernanceStreamAccessRepo{},
		Conversation: conversationRepo,
		Message:      messageRepo,
	}, llm).(*service)

	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer redisServer.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer redisClient.Close()
	svc.events = newStreamEventStore(redisClient)
	appendRecoveredContinuationEvents(t, ctx, svc.events, conversationID, messageID)

	var events []StreamEvent
	result, err := svc.RunToolGovernanceDecisionStream(
		ctx,
		Scope{OrganizationID: organizationID, AccountID: accountID},
		conversationID,
		messageID,
		"corr-approve",
		runtimedto.ToolGovernanceDecisionRequest{Action: "approve"},
		func(event StreamEvent) error {
			events = append(events, event)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("RunToolGovernanceDecisionStream() error = %v", err)
	}
	if result == nil || result.Status != runtimemodel.MessageStatusStreaming {
		t.Fatalf("result = %#v, want recovery result", result)
	}
	if len(llm.appChatRequests) != 0 || len(llm.streamRequests) != 0 {
		t.Fatalf("LLM calls = app %d stream %d, want none for resolved duplicate", len(llm.appChatRequests), len(llm.streamRequests))
	}
	if messageRepo.updateCompletedCalled || message.Status != runtimemodel.MessageStatusCompleted {
		t.Fatalf("message status = %q completedCalled=%v, want existing completed message untouched", message.Status, messageRepo.updateCompletedCalled)
	}
	assertRecoveredContinuationEvents(t, events)
}

func TestRunClientActionContinuationStreamAlreadyRunningRecoversActiveStream(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	accountID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()
	provider := "deepseek"
	now := time.Now().UTC()
	actionID := "route_navigation:open-files"
	event := map[string]interface{}{
		"action_id":   actionID,
		"action_type": "route_navigation",
		"status":      clientActionStatusWaiting,
		"skill_id":    skills.SkillConsoleNavigator,
		"tool_name":   "navigate",
		"href":        "/console/files",
	}
	metadata := mergeClientActionMetadata(map[string]interface{}{
		"surface":              "contextual_sidebar",
		"configured_skill_ids": []string{skills.SkillConsoleNavigator},
	}, event)
	metadata["client_action_continuation"] = compactSkillInvocation(event)

	conversation := &runtimemodel.Conversation{
		ID:                   conversationID,
		OrganizationID:       organizationID,
		AccountID:            accountID,
		CallerType:           runtimemodel.ConversationCallerAIChat,
		Title:                "Files",
		Status:               runtimemodel.ConversationStatusNormal,
		RuntimeStatus:        runtimemodel.ConversationRuntimeStatusStreaming,
		CurrentLeafMessageID: &messageID,
		ActiveMessageID:      &messageID,
		Metadata:             map[string]interface{}{},
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	message := &runtimemodel.Message{
		ID:              messageID,
		ConversationID:  conversationID,
		Query:           "Open files",
		Status:          runtimemodel.MessageStatusStreaming,
		ModelProvider:   &provider,
		ModelName:       "deepseek-chat",
		ModelParameters: map[string]interface{}{},
		Metadata:        metadata,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	messageRepo := &toolGovernanceStreamMessageRepo{message: message}
	llm := &toolGovernanceStreamLLM{}
	svc := NewService(&repository.Repositories{
		Access:       toolGovernanceStreamAccessRepo{},
		Conversation: &toolGovernanceStreamConversationRepo{conversation: conversation},
		Message:      messageRepo,
	}, llm).(*service)

	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer redisServer.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer redisClient.Close()
	svc.events = newStreamEventStore(redisClient)
	appendRecoveredContinuationEvents(t, ctx, svc.events, conversationID, messageID)

	var events []StreamEvent
	result, err := svc.RunClientActionContinuationStream(
		ctx,
		Scope{OrganizationID: organizationID, AccountID: accountID},
		conversationID,
		messageID,
		actionID,
		runtimedto.ClientActionResultRequest{
			Status:  "succeeded",
			Surface: "contextual_sidebar",
			Result: map[string]interface{}{
				"loaded_href": "/console/files",
			},
		},
		func(event StreamEvent) error {
			events = append(events, event)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("RunClientActionContinuationStream() error = %v", err)
	}
	if result == nil || result.Status != runtimemodel.MessageStatusStreaming {
		t.Fatalf("result = %#v, want streaming recovery result", result)
	}
	if len(llm.appChatRequests) != 0 || len(llm.streamRequests) != 0 {
		t.Fatalf("LLM calls = app %d stream %d, want none while reconnecting active stream", len(llm.appChatRequests), len(llm.streamRequests))
	}
	if messageRepo.updateCompletedCalled || message.Status != runtimemodel.MessageStatusStreaming {
		t.Fatalf("message status = %q completedCalled=%v, want existing active stream untouched", message.Status, messageRepo.updateCompletedCalled)
	}
	assertRecoveredContinuationEvents(t, events)
}

func TestRunClientActionContinuationStreamResolvedDuplicateRecoversCompletedStream(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	accountID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()
	provider := "deepseek"
	now := time.Now().UTC()
	actionID := "route_navigation:open-files"
	event := map[string]interface{}{
		"action_id":   actionID,
		"action_type": "route_navigation",
		"status":      clientActionStatusSucceeded,
		"skill_id":    skills.SkillConsoleNavigator,
		"tool_name":   "navigate",
		"href":        "/console/files",
		"result": map[string]interface{}{
			"loaded_href": "/console/files",
		},
	}
	metadata := mergeClientActionMetadata(map[string]interface{}{
		"surface":              "contextual_sidebar",
		"configured_skill_ids": []string{skills.SkillConsoleNavigator},
	}, event)
	metadata["client_action_continuation"] = compactSkillInvocation(event)

	conversation := &runtimemodel.Conversation{
		ID:                   conversationID,
		OrganizationID:       organizationID,
		AccountID:            accountID,
		CallerType:           runtimemodel.ConversationCallerAIChat,
		Title:                "Files",
		Status:               runtimemodel.ConversationStatusNormal,
		RuntimeStatus:        runtimemodel.ConversationRuntimeStatusIdle,
		CurrentLeafMessageID: &messageID,
		Metadata:             map[string]interface{}{},
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	message := &runtimemodel.Message{
		ID:              messageID,
		ConversationID:  conversationID,
		Query:           "Open files",
		Answer:          "Done",
		Status:          runtimemodel.MessageStatusCompleted,
		ModelProvider:   &provider,
		ModelName:       "deepseek-chat",
		ModelParameters: map[string]interface{}{},
		Metadata:        metadata,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	messageRepo := &toolGovernanceStreamMessageRepo{message: message}
	llm := &toolGovernanceStreamLLM{}
	svc := NewService(&repository.Repositories{
		Access:       toolGovernanceStreamAccessRepo{},
		Conversation: &toolGovernanceStreamConversationRepo{conversation: conversation},
		Message:      messageRepo,
	}, llm).(*service)

	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer redisServer.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer redisClient.Close()
	svc.events = newStreamEventStore(redisClient)
	appendRecoveredContinuationEvents(t, ctx, svc.events, conversationID, messageID)

	var events []StreamEvent
	result, err := svc.RunClientActionContinuationStream(
		ctx,
		Scope{OrganizationID: organizationID, AccountID: accountID},
		conversationID,
		messageID,
		actionID,
		runtimedto.ClientActionResultRequest{
			Status:  "succeeded",
			Surface: "contextual_sidebar",
			Result: map[string]interface{}{
				"loaded_href": "/console/files",
			},
		},
		func(event StreamEvent) error {
			events = append(events, event)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("RunClientActionContinuationStream() error = %v", err)
	}
	if result == nil || result.Status != runtimemodel.MessageStatusStreaming {
		t.Fatalf("result = %#v, want recovery result", result)
	}
	if len(llm.appChatRequests) != 0 || len(llm.streamRequests) != 0 {
		t.Fatalf("LLM calls = app %d stream %d, want none for resolved duplicate", len(llm.appChatRequests), len(llm.streamRequests))
	}
	if messageRepo.updateCompletedCalled || message.Status != runtimemodel.MessageStatusCompleted {
		t.Fatalf("message status = %q completedCalled=%v, want existing completed message untouched", message.Status, messageRepo.updateCompletedCalled)
	}
	assertRecoveredContinuationEvents(t, events)
}

func TestRunToolGovernanceDecisionStreamApproveToolFailureReturnsErrorToModel(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	accountID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()
	provider := "deepseek"
	now := time.Now().UTC()

	metadata := pendingToolGovernanceDecisionMetadata("corr-approve")
	metadata["configured_skill_ids"] = []interface{}{skills.SkillFileManager}
	invocation := metadata["skill_invocations"].([]interface{})[0].(map[string]interface{})
	governance := invocation["governance"].(map[string]interface{})
	approvalEvent := governance["approval_event"].(map[string]interface{})
	approvalEvent["assets"] = []interface{}{
		map[string]interface{}{
			"id":           "file-1",
			"type":         "file",
			"name":         "report.pdf",
			"workspace_id": "workspace-1",
		},
	}
	approvalEvent["grant"] = map[string]interface{}{
		"conversation_id": conversationID.String(),
		"organization_id": organizationID.String(),
		"user_id":         accountID.String(),
		"skill_id":        skills.SkillFileManager,
		"provider_type":   "builtin",
		"provider_id":     "files",
		"tool_id":         "file.delete",
		"effect":          "delete",
		"asset_type":      "file",
		"risk_level":      "high",
		"assets": []interface{}{
			map[string]interface{}{"id": "file-1", "type": "file", "name": "report.pdf"},
		},
	}
	approvalEvent["frozen_invocation"] = toolgovernance.NewFrozenInvocation(toolgovernance.FrozenInvocationRequest{
		CorrelationID: "corr-approve",
		Manifest: toolgovernance.Manifest{
			ToolID:    "file.delete",
			SkillID:   skills.SkillFileManager,
			Effect:    toolgovernance.EffectDelete,
			AssetType: "file",
			RiskLevel: toolgovernance.RiskLevelHigh,
		},
		SkillID:      skills.SkillFileManager,
		ToolName:     "delete_file",
		ProviderType: "builtin",
		ProviderID:   "files",
		Arguments: map[string]interface{}{
			"file_id": "file-1",
		},
		Assets: []toolgovernance.AssetRef{
			{ID: "file-1", Type: "file", Name: "report.pdf", WorkspaceID: "workspace-1"},
		},
		Now: now,
		TTL: 7 * 24 * time.Hour,
	})

	conversation := &runtimemodel.Conversation{
		ID:             conversationID,
		OrganizationID: organizationID,
		AccountID:      accountID,
		CallerType:     runtimemodel.ConversationCallerAIChat,
		Title:          "Files",
		Status:         runtimemodel.ConversationStatusNormal,
		RuntimeStatus:  runtimemodel.ConversationRuntimeStatusIdle,
		Metadata:       map[string]interface{}{},
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	conversation.CurrentLeafMessageID = &messageID
	message := &runtimemodel.Message{
		ID:              messageID,
		ConversationID:  conversationID,
		Query:           "Delete report.pdf",
		Status:          runtimemodel.MessageStatusWaitingApproval,
		ModelProvider:   &provider,
		ModelName:       "deepseek-chat",
		ModelParameters: map[string]interface{}{},
		Metadata:        metadata,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	fileService := &toolGovernanceStreamFileService{files: map[string]*dto.UploadFile{}}
	workspacePerms := &toolGovernanceStreamWorkspacePermissionService{allowed: true}
	runtime := newToolGovernanceStreamSkillRuntime(t, fileService, workspacePerms)
	llm := &toolGovernanceStreamLLM{
		streamChunks: []string{"删除失败：file file-1 not found。"},
	}
	messageRepo := &toolGovernanceStreamMessageRepo{message: message}
	conversationRepo := &toolGovernanceStreamConversationRepo{conversation: conversation}
	svc := NewServiceWithSkillRuntime(
		&repository.Repositories{
			Access:       toolGovernanceStreamAccessRepo{},
			Conversation: conversationRepo,
			Message:      messageRepo,
			SkillConfig:  toolGovernanceStreamSkillConfigRepo{skillID: skills.SkillFileManager},
		},
		llm,
		nil,
		toolGovernanceStreamModelSpecResolver{},
		nil,
		nil,
		nil,
		runtime,
		nil,
	).(*service)
	svc.events = newStreamEventStore(nil)

	var events []StreamEvent
	result, err := svc.RunToolGovernanceDecisionStream(
		ctx,
		Scope{OrganizationID: organizationID, AccountID: accountID},
		conversationID,
		messageID,
		"corr-approve",
		runtimedto.ToolGovernanceDecisionRequest{Action: "approve"},
		func(event StreamEvent) error {
			events = append(events, event)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("RunToolGovernanceDecisionStream() error = %v", err)
	}
	if result.Status != runtimemodel.MessageStatusCompleted {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	if result.Answer != "删除失败：file file-1 not found。" {
		t.Fatalf("result answer = %q, want model-visible failure summary", result.Answer)
	}
	if len(fileService.deleted) != 0 {
		t.Fatalf("deleted files = %#v, want none after execution failure", fileService.deleted)
	}
	if messageRepo.updateErrorCalled {
		t.Fatal("UpdateError was called; approved execution failures should be returned to the model")
	}
	if !messageRepo.updateCompletedCalled {
		t.Fatal("UpdateCompleted was not called")
	}
	if message.Status != runtimemodel.MessageStatusCompleted || message.Error != nil {
		t.Fatalf("message status/error = %q/%#v, want completed with no message error", message.Status, message.Error)
	}
	if len(llm.streamRequests) != 1 {
		t.Fatalf("AppChatStream requests = %d, want one execution-failure summary call", len(llm.streamRequests))
	}
	streamReq := llm.streamRequests[0]
	if toolGovernanceStreamRequestHasTool(streamReq, skills.MetaToolCallSkillTool) {
		t.Fatalf("execution-failure summary request should not expose %s tool", skills.MetaToolCallSkillTool)
	}
	for _, want := range []string{
		"runtime attempted to execute the frozen invocation exactly once, but it failed",
		"recoverable model feedback",
		"Runtime failure feedback:\nfile report.pdf not found",
		"\"error\":\"file report.pdf not found\"",
	} {
		if !toolGovernanceStreamRequestContains(streamReq, want) {
			t.Fatalf("execution-failure request missing %q in %q", want, toolGovernanceStreamRequestText(streamReq))
		}
	}
	for _, hidden := range []string{
		`"execution_error"`,
		`"file_id"`,
		`"workspace_id"`,
		"file-1",
		"workspace-1",
		"corr-approve",
	} {
		if toolGovernanceStreamRequestContains(streamReq, hidden) {
			t.Fatalf("execution-failure request exposed %q in %q", hidden, toolGovernanceStreamRequestText(streamReq))
		}
	}
	if !toolGovernanceStreamHasInvocation(message.Metadata, "tool_call", skills.SkillFileManager, "delete_file", "error") {
		t.Fatalf("metadata skill_invocations = %#v, want failed file-manager/delete_file tool call", message.Metadata["skill_invocations"])
	}

	assertToolGovernanceApprovedFailureStreamEvents(t, events)
}

func TestRunToolGovernanceDecisionStreamApproveToolFailureWithOperationPlanUsesVerifier(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	accountID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()
	provider := "deepseek"
	now := time.Now().UTC()

	metadata := pendingToolGovernanceDecisionMetadata("corr-approve")
	metadata["configured_skill_ids"] = []interface{}{skills.SkillFileManager}
	metadata["operation_plan"] = map[string]interface{}{
		"version":             operationPlanVersion,
		"intent":              "delete_visible_file",
		"status":              operationPlanStatusRunning,
		"pending_next_action": "Delete resolved file",
		"steps": []interface{}{
			map[string]interface{}{
				"id":        operationPlanToolStepID(skills.SkillFileManager, "delete_file"),
				"status":    operationPlanStepStatusPending,
				"skill_id":  skills.SkillFileManager,
				"tool_name": "delete_file",
			},
		},
	}
	invocation := metadata["skill_invocations"].([]interface{})[0].(map[string]interface{})
	governance := invocation["governance"].(map[string]interface{})
	approvalEvent := governance["approval_event"].(map[string]interface{})
	approvalEvent["assets"] = []interface{}{
		map[string]interface{}{
			"id":           "file-1",
			"type":         "file",
			"name":         "report.pdf",
			"workspace_id": "workspace-1",
		},
	}
	approvalEvent["grant"] = map[string]interface{}{
		"conversation_id": conversationID.String(),
		"organization_id": organizationID.String(),
		"user_id":         accountID.String(),
		"skill_id":        skills.SkillFileManager,
		"provider_type":   "builtin",
		"provider_id":     "files",
		"tool_id":         "file.delete",
		"effect":          "delete",
		"asset_type":      "file",
		"risk_level":      "high",
		"assets": []interface{}{
			map[string]interface{}{"id": "file-1", "type": "file", "name": "report.pdf"},
		},
	}
	approvalEvent["frozen_invocation"] = toolgovernance.NewFrozenInvocation(toolgovernance.FrozenInvocationRequest{
		CorrelationID: "corr-approve",
		Manifest: toolgovernance.Manifest{
			ToolID:    "file.delete",
			SkillID:   skills.SkillFileManager,
			Effect:    toolgovernance.EffectDelete,
			AssetType: "file",
			RiskLevel: toolgovernance.RiskLevelHigh,
		},
		SkillID:      skills.SkillFileManager,
		ToolName:     "delete_file",
		ProviderType: "builtin",
		ProviderID:   "files",
		Arguments: map[string]interface{}{
			"file_id": "file-1",
		},
		Assets: []toolgovernance.AssetRef{
			{ID: "file-1", Type: "file", Name: "report.pdf", WorkspaceID: "workspace-1"},
		},
		Now: now,
		TTL: 7 * 24 * time.Hour,
	})
	conversation := &runtimemodel.Conversation{
		ID:                   conversationID,
		OrganizationID:       organizationID,
		AccountID:            accountID,
		CallerType:           runtimemodel.ConversationCallerAIChat,
		Title:                "Files",
		Status:               runtimemodel.ConversationStatusNormal,
		RuntimeStatus:        runtimemodel.ConversationRuntimeStatusIdle,
		CurrentLeafMessageID: &messageID,
		Metadata:             map[string]interface{}{},
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	message := &runtimemodel.Message{
		ID:              messageID,
		ConversationID:  conversationID,
		Query:           "Delete report.pdf",
		Status:          runtimemodel.MessageStatusWaitingApproval,
		ModelProvider:   &provider,
		ModelName:       "deepseek-chat",
		ModelParameters: map[string]interface{}{},
		Metadata:        metadata,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	fileService := &toolGovernanceStreamFileService{}
	runtime := newToolGovernanceStreamSkillRuntime(t, fileService, &toolGovernanceStreamWorkspacePermissionService{allowed: true})
	finalAnswer := "Deletion failed: report.pdf does not exist."
	llm := &toolGovernanceStreamLLM{
		appChatResponses: []*adapter.ChatResponse{
			toolGovernanceStreamChatResponse(finalAnswer),
			toolGovernanceStreamChatResponse(`{"status":"pass","reason":"tool result confirms delete failed","unsupported_claims":[],"missing_steps":[],"next_action_hint":"","final_answer":"","final_answer_guidance":""}`),
		},
	}
	messageRepo := &toolGovernanceStreamMessageRepo{message: message}
	conversationRepo := &toolGovernanceStreamConversationRepo{conversation: conversation}
	svc := NewServiceWithSkillRuntime(
		&repository.Repositories{
			Access:       toolGovernanceStreamAccessRepo{},
			Conversation: conversationRepo,
			Message:      messageRepo,
			SkillConfig:  toolGovernanceStreamSkillConfigRepo{skillID: skills.SkillFileManager},
		},
		llm,
		nil,
		toolGovernanceStreamModelSpecResolver{},
		nil,
		nil,
		nil,
		runtime,
		nil,
	).(*service)
	svc.events = newStreamEventStore(nil)

	var events []StreamEvent
	result, err := svc.RunToolGovernanceDecisionStream(
		ctx,
		Scope{OrganizationID: organizationID, AccountID: accountID},
		conversationID,
		messageID,
		"corr-approve",
		runtimedto.ToolGovernanceDecisionRequest{Action: "approve"},
		func(event StreamEvent) error {
			events = append(events, event)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("RunToolGovernanceDecisionStream() error = %v", err)
	}
	if result.Status != runtimemodel.MessageStatusCompleted {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	if len(llm.appChatRequests) != 2 {
		t.Fatalf("AppChat requests = %d, want failure answer plus completion verifier", len(llm.appChatRequests))
	}
	if len(llm.streamRequests) != 0 {
		t.Fatalf("AppChatStream requests = %d, want operation-plan failure to use skill loop verifier", len(llm.streamRequests))
	}
	if !toolGovernanceStreamRequestContains(llm.appChatRequests[1], "completion post-verifier") {
		t.Fatalf("second AppChat request = %q, want completion verifier", toolGovernanceStreamRequestText(llm.appChatRequests[1]))
	}
	if !strings.Contains(result.Answer, "没有被工具结果确认成功") ||
		!strings.Contains(result.Answer, "file-manager/delete_file") ||
		!strings.Contains(result.Answer, "file file-1 not found") {
		t.Fatalf("result answer = %q, want verifier-backed failed-plan answer", result.Answer)
	}
	if strings.Contains(strings.ToLower(result.Answer), "success") {
		t.Fatalf("result answer = %q, want no success claim after failed tool", result.Answer)
	}
	if len(fileService.deleted) != 0 {
		t.Fatalf("deleted files = %#v, want none after execution failure", fileService.deleted)
	}
	if !messageRepo.updateCompletedCalled {
		t.Fatal("UpdateCompleted was not called")
	}
	if !toolGovernanceStreamHasInvocation(message.Metadata, "tool_call", skills.SkillFileManager, "delete_file", "error") {
		t.Fatalf("metadata skill_invocations = %#v, want failed file-manager/delete_file tool call", message.Metadata["skill_invocations"])
	}
	if !modelInvocationMetadataHasPhase(message.Metadata, "completion_verifier") {
		t.Fatalf("metadata model_invocations = %#v, want completion_verifier trace", message.Metadata["model_invocations"])
	}
	assertToolGovernanceApprovedFailureStreamEvents(t, events)
}

func TestSubmitToolGovernanceDecisionApproveRememberForSessionPersistsConversationGrant(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	accountID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()
	now := time.Now().UTC()

	metadata := pendingToolGovernanceDecisionMetadata("corr-session")
	invocation := metadata["skill_invocations"].([]interface{})[0].(map[string]interface{})
	governance := invocation["governance"].(map[string]interface{})
	approvalEvent := governance["approval_event"].(map[string]interface{})
	approvalEvent["assets"] = []interface{}{
		map[string]interface{}{
			"id":           "file-1",
			"type":         "file",
			"name":         "report.pdf",
			"workspace_id": "workspace-1",
			"source":       "console.files",
		},
	}
	approvalEvent["grant"] = map[string]interface{}{
		"conversation_id": conversationID.String(),
		"skill_id":        skills.SkillFileManager,
		"provider_type":   "builtin",
		"provider_id":     "files",
		"tool_id":         "file.delete",
		"effect":          "delete",
		"asset_type":      "file",
		"risk_level":      "high",
		"assets": []interface{}{
			map[string]interface{}{
				"id":           "file-1",
				"type":         "file",
				"name":         "report.pdf",
				"workspace_id": "workspace-1",
			},
		},
	}

	conversation := &runtimemodel.Conversation{
		ID:             conversationID,
		OrganizationID: organizationID,
		AccountID:      accountID,
		CallerType:     runtimemodel.ConversationCallerAIChat,
		Title:          "Files",
		Status:         runtimemodel.ConversationStatusNormal,
		RuntimeStatus:  runtimemodel.ConversationRuntimeStatusIdle,
		Metadata:       map[string]interface{}{},
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	message := &runtimemodel.Message{
		ID:             messageID,
		ConversationID: conversationID,
		Query:          "Delete report.pdf",
		Status:         runtimemodel.MessageStatusWaitingApproval,
		Metadata:       metadata,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	messageRepo := &toolGovernanceStreamMessageRepo{message: message}
	conversationRepo := &toolGovernanceStreamConversationRepo{conversation: conversation}
	svc := NewService(&repository.Repositories{
		Access:       toolGovernanceStreamAccessRepo{},
		Conversation: conversationRepo,
		Message:      messageRepo,
	}, &toolGovernanceStreamLLM{}).(*service)

	response, err := svc.SubmitToolGovernanceDecision(
		ctx,
		Scope{OrganizationID: organizationID, AccountID: accountID},
		conversationID,
		messageID,
		"corr-session",
		runtimedto.ToolGovernanceDecisionRequest{Action: "approve", RememberForSession: true},
	)
	if err != nil {
		t.Fatalf("SubmitToolGovernanceDecision() error = %v", err)
	}
	if response.ApprovalStatus != "approved" || !response.RememberForSession {
		t.Fatalf("response = %#v, want approved remembered session", response)
	}
	if response.SessionGrant["conversation_id"] != conversationID.String() ||
		response.SessionGrant["organization_id"] != organizationID.String() ||
		response.SessionGrant["user_id"] != accountID.String() ||
		response.SessionGrant["skill_id"] != skills.SkillFileManager ||
		response.SessionGrant["provider_type"] != "builtin" ||
		response.SessionGrant["provider_id"] != "files" ||
		response.SessionGrant["tool_id"] != "file.delete" ||
		response.SessionGrant["effect"] != "delete" ||
		response.SessionGrant["asset_type"] != "file" ||
		response.SessionGrant["risk_level"] != "high" ||
		response.SessionGrant["approval_correlation_id"] != "corr-session" {
		t.Fatalf("session grant = %#v, want identity/runtime/tool/effect/asset/risk scoped grant", response.SessionGrant)
	}
	if response.SessionGrant["expires_at"] == "" {
		t.Fatalf("session grant = %#v, want expires_at", response.SessionGrant)
	}
	grantAssets := mapSliceFromAny(response.SessionGrant["assets"])
	if len(grantAssets) != 1 || grantAssets[0]["id"] != "file-1" || grantAssets[0]["workspace_id"] != "workspace-1" {
		t.Fatalf("session grant assets = %#v, want approved file asset", grantAssets)
	}
	if !messageRepo.updateMetadataAnyStatusCalled {
		t.Fatal("UpdateMetadataAnyStatus was not called for approved decision")
	}
	if !conversationRepo.updateMetadataCalled {
		t.Fatal("UpdateMetadata was not called for remembered session grant")
	}
	if grants := mapSliceFromAny(message.Metadata["tool_governance_one_shot_grants"]); len(grants) != 1 {
		t.Fatalf("one-shot grants = %#v, want one approved grant on current message", grants)
	} else if grants[0]["organization_id"] != organizationID.String() ||
		grants[0]["user_id"] != accountID.String() ||
		grants[0]["skill_id"] != skills.SkillFileManager ||
		grants[0]["provider_type"] != "builtin" ||
		grants[0]["provider_id"] != "files" ||
		grants[0]["expires_at"] == "" {
		t.Fatalf("one-shot grant = %#v, want complete scoped grant", grants[0])
	}
	conversationGrants := mapSliceFromAny(conversation.Metadata["tool_governance_session_grants"])
	if len(conversationGrants) != 1 {
		t.Fatalf("conversation session grants = %#v, want one remembered grant", conversationGrants)
	}
	if conversationGrants[0]["conversation_id"] != conversationID.String() ||
		conversationGrants[0]["organization_id"] != organizationID.String() ||
		conversationGrants[0]["user_id"] != accountID.String() ||
		conversationGrants[0]["skill_id"] != skills.SkillFileManager ||
		conversationGrants[0]["provider_type"] != "builtin" ||
		conversationGrants[0]["provider_id"] != "files" ||
		conversationGrants[0]["approval_correlation_id"] != "corr-session" ||
		conversationGrants[0]["expires_at"] == "" {
		t.Fatalf("conversation session grant = %#v, want complete scoped approval grant", conversationGrants[0])
	}
	continuation := governanceMapFromAny(message.Metadata["tool_governance_continuation"])
	if continuation["status"] != "approved" ||
		continuation["approval_status"] != "approved" ||
		continuation["remember_for_session"] != true {
		t.Fatalf("tool_governance_continuation = %#v, want approved remembered session", continuation)
	}

	params := applySkillToolGovernanceRuntimeParameters(nil, &PreparedChat{Conversation: conversation})
	governanceParams := governanceMapFromAny(params[skillToolGovernanceRuntimeKey])
	runtimeGrants := mapSliceFromAny(governanceParams["session_grants"])
	if len(runtimeGrants) != 1 ||
		runtimeGrants[0]["organization_id"] != organizationID.String() ||
		runtimeGrants[0]["user_id"] != accountID.String() ||
		runtimeGrants[0]["skill_id"] != skills.SkillFileManager ||
		runtimeGrants[0]["provider_type"] != "builtin" ||
		runtimeGrants[0]["provider_id"] != "files" ||
		runtimeGrants[0]["tool_id"] != "file.delete" ||
		runtimeGrants[0]["approval_correlation_id"] != "corr-session" ||
		runtimeGrants[0]["expires_at"] == "" {
		t.Fatalf("runtime session grants = %#v, want remembered complete scoped file.delete grant", runtimeGrants)
	}
}

func assertToolGovernanceStreamEvents(t *testing.T, events []StreamEvent) {
	t.Helper()
	if len(events) == 0 {
		t.Fatal("stream events = none")
	}
	seen := map[string]bool{}
	for _, event := range events {
		seen[event.EventType] = true
		if event.EventType == streamEventToolGovernanceDecision {
			if event.Payload["approval_status"] != "rejected" {
				t.Fatalf("decision event approval_status = %#v, want rejected", event.Payload["approval_status"])
			}
			governance := governanceMapFromAny(event.Payload["governance"])
			if governance["requires_approval"] != false {
				t.Fatalf("decision event governance = %#v, want no longer requiring approval", governance)
			}
		}
		if event.EventType == streamEventMessageEnd {
			if event.Payload["status"] != runtimemodel.MessageStatusCompleted {
				t.Fatalf("message_end status = %#v, want completed", event.Payload["status"])
			}
		}
	}
	for _, want := range []string{
		streamEventMessageStart,
		streamEventToolGovernanceDecision,
		streamEventMessage,
		streamEventMessageEnd,
	} {
		if !seen[want] {
			t.Fatalf("stream events missing %q in %#v", want, events)
		}
	}
}

func appendRecoveredContinuationEvents(t *testing.T, ctx context.Context, store *streamEventStore, conversationID, messageID uuid.UUID) {
	t.Helper()
	payloadBase := map[string]interface{}{
		"conversation_id": conversationID.String(),
		"message_id":      messageID.String(),
	}
	for _, item := range []struct {
		eventType string
		payload   map[string]interface{}
	}{
		{eventType: streamEventMessageStart, payload: copyStringAnyMap(payloadBase)},
		{eventType: streamEventMessage, payload: map[string]interface{}{
			"conversation_id": conversationID.String(),
			"message_id":      messageID.String(),
			"answer":          "recovering existing stream",
		}},
		{eventType: streamEventMessageEnd, payload: map[string]interface{}{
			"conversation_id": conversationID.String(),
			"message_id":      messageID.String(),
			"status":          runtimemodel.MessageStatusCompleted,
			"metadata":        map[string]interface{}{},
		}},
	} {
		if _, err := store.append(ctx, messageID, conversationID, item.eventType, item.payload); err != nil {
			t.Fatalf("append %s event: %v", item.eventType, err)
		}
	}
}

func assertRecoveredContinuationEvents(t *testing.T, events []StreamEvent) {
	t.Helper()
	if len(events) == 0 {
		t.Fatal("recovered stream events = none")
	}
	seen := map[string]bool{}
	for _, event := range events {
		if event.EventType == streamEventError {
			t.Fatalf("recovered stream events = %#v, want no stream error", events)
		}
		seen[event.EventType] = true
	}
	for _, want := range []string{streamEventMessageStart, streamEventMessage, streamEventMessageEnd} {
		if !seen[want] {
			t.Fatalf("recovered stream events missing %q in %#v", want, events)
		}
	}
}

func assertToolGovernanceApprovedStreamEvents(t *testing.T, events []StreamEvent) {
	t.Helper()
	seen := map[string]bool{}
	var approvedDecision bool
	var finalMessage bool
	for _, event := range events {
		seen[event.EventType] = true
		if event.EventType == streamEventToolGovernanceDecision && event.Payload["approval_status"] == "approved" {
			approvedDecision = true
		}
		if event.EventType == streamEventToolGovernanceDecision && event.Payload["decision"] == toolgovernance.DecisionStatusAllowed {
			t.Fatalf("events = %#v, want approved decision only; execution result belongs to skill_call_end", events)
		}
		if event.EventType == streamEventSkillCallEnd && event.Payload["tool_name"] == "delete_file" {
			if event.Payload["status"] != "success" {
				t.Fatalf("delete_file event status = %#v, want success", event.Payload["status"])
			}
		}
		if event.EventType == streamEventClientActionRequired && event.Payload["action_type"] == "asset_observation" {
			t.Fatalf("events = %#v, want no asset observation client action after delete fast path", events)
		}
		if event.EventType == streamEventMessage && strings.Contains(stringFromAny(event.Payload["answer"]), "已删除文件") {
			finalMessage = true
		}
	}
	for _, want := range []string{
		streamEventMessageStart,
		streamEventToolGovernanceDecision,
		streamEventSkillCallStart,
		streamEventSkillCallEnd,
		streamEventMessage,
		streamEventMessageEnd,
	} {
		if !seen[want] {
			t.Fatalf("stream events missing %q in %#v", want, events)
		}
	}
	if !approvedDecision {
		t.Fatalf("events = %#v, want approved tool governance decision", events)
	}
	if !finalMessage {
		t.Fatalf("events = %#v, want fast-path final message", events)
	}
}

func assertToolGovernanceApprovedFailureStreamEvents(t *testing.T, events []StreamEvent) {
	t.Helper()
	seen := map[string]bool{}
	var approvedDecision bool
	var toolError bool
	for _, event := range events {
		seen[event.EventType] = true
		if event.EventType == streamEventError {
			t.Fatalf("events = %#v, want no top-level stream error", events)
		}
		if event.EventType == streamEventToolGovernanceDecision && event.Payload["approval_status"] == "approved" {
			approvedDecision = true
		}
		if event.EventType == streamEventToolGovernanceDecision && event.Payload["decision"] == toolgovernance.DecisionStatusAllowed {
			t.Fatalf("events = %#v, want approved decision only; execution error belongs to skill_call_error", events)
		}
		if event.EventType == streamEventSkillCallError && event.Payload["tool_name"] == "delete_file" {
			toolError = true
			if message := stringFromAny(event.Payload["message"]); !strings.Contains(message, "file file-1 not found") {
				t.Fatalf("skill_call_error message = %q, want file not found", message)
			}
		}
		if event.EventType == streamEventMessageEnd {
			if event.Payload["status"] != runtimemodel.MessageStatusCompleted {
				t.Fatalf("message_end status = %#v, want completed", event.Payload["status"])
			}
		}
	}
	for _, want := range []string{
		streamEventMessageStart,
		streamEventToolGovernanceDecision,
		streamEventSkillCallStart,
		streamEventSkillCallError,
		streamEventMessage,
		streamEventMessageEnd,
	} {
		if !seen[want] {
			t.Fatalf("stream events missing %q in %#v", want, events)
		}
	}
	if !approvedDecision {
		t.Fatalf("events = %#v, want approved tool governance decision", events)
	}
	if !toolError {
		t.Fatalf("events = %#v, want delete_file skill_call_error", events)
	}
}

func toolGovernanceStreamRequestText(req *adapter.ChatRequest) string {
	if req == nil {
		return ""
	}
	parts := make([]string, 0, len(req.Messages))
	for _, message := range req.Messages {
		parts = append(parts, messageContentText(message.Content))
	}
	return strings.Join(parts, "\n")
}

func toolGovernanceStreamRequestContains(req *adapter.ChatRequest, want string) bool {
	return strings.Contains(toolGovernanceStreamRequestText(req), want)
}

func toolGovernanceStreamRequestHasTool(req *adapter.ChatRequest, toolName string) bool {
	if req == nil {
		return false
	}
	for _, tool := range req.Tools {
		if strings.EqualFold(strings.TrimSpace(tool.Function.Name), strings.TrimSpace(toolName)) {
			return true
		}
	}
	return false
}

func toolGovernanceStreamHasInvocation(metadata map[string]interface{}, kind string, skillID string, toolName string, status string) bool {
	for _, invocation := range skillInvocationsFromMetadata(metadata["skill_invocations"]) {
		if strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["kind"])), kind) &&
			strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["skill_id"])), skillID) &&
			strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["tool_name"])), toolName) &&
			strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["status"])), status) {
			return true
		}
	}
	return false
}

func modelInvocationMetadataHasPhase(metadata map[string]interface{}, phase string) bool {
	for _, invocation := range modelInvocationsFromMetadata(metadata["model_invocations"]) {
		if strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["phase"])), phase) {
			return true
		}
	}
	return false
}

func toolGovernanceStreamSkillToolCall(callID string, skillID string, toolName string, arguments map[string]interface{}) adapter.ToolCall {
	payload, _ := json.Marshal(map[string]interface{}{
		"skill_id":  skillID,
		"tool_name": toolName,
		"arguments": arguments,
	})
	return adapter.ToolCall{
		ID:   callID,
		Type: "function",
		Function: adapter.FunctionCall{
			Name:      skills.MetaToolCallSkillTool,
			Arguments: string(payload),
		},
	}
}

func toolGovernanceStreamChatResponse(content string) *adapter.ChatResponse {
	return &adapter.ChatResponse{
		Choices: []adapter.Choice{{
			Message: adapter.Message{Role: "assistant", Content: content},
		}},
		Usage: &adapter.Usage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}
}

func newToolGovernanceStreamSkillRuntime(t *testing.T, fileService *toolGovernanceStreamFileService, workspacePerms *toolGovernanceStreamWorkspacePermissionService) *skills.Runtime {
	t.Helper()
	catalogDir := t.TempDir()
	root := filepath.Join(catalogDir, skills.SkillFileManager)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir skill root: %v", err)
	}
	skill := `---
name: file-manager
description: Governed file manager service test skill.
when_to_use: Use when testing AIChat approval continuation.
provider_type: builtin
provider_id: files
runtime_type: tool
tools:
  - delete_file
tool_governance:
  delete_file:
    tool_id: file.delete
    skill_id: file-manager
    domain: files
    effect: delete
    asset_type: file
    risk_level: high
    requires_asset_resolution: true
    permission_scopes:
      - file:manage
    default_approval_policy: always_ask
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
---

# Governed Files

Use governed file tools.
`
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(skill), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	navigationRoot := filepath.Join(catalogDir, skills.SkillConsoleNavigator)
	if err := os.MkdirAll(navigationRoot, 0o755); err != nil {
		t.Fatalf("mkdir navigation skill root: %v", err)
	}
	navigationSkill := `---
name: console-navigator
description: Console navigation service test skill.
when_to_use: Use when testing AIChat client-side route continuation.
provider_type: builtin
provider_id: console-navigation
runtime_type: tool
tools:
  - navigate
---

# Console Navigation

Use console navigation tools.
`
	if err := os.WriteFile(filepath.Join(navigationRoot, "SKILL.md"), []byte(navigationSkill), 0o644); err != nil {
		t.Fatalf("write navigation SKILL.md: %v", err)
	}
	agentRoot := filepath.Join(catalogDir, skills.SkillAgentManagement)
	if err := os.MkdirAll(agentRoot, 0o755); err != nil {
		t.Fatalf("mkdir agent management skill root: %v", err)
	}
	agentSkill := `---
name: agent-management
description: Agent management service test skill.
when_to_use: Use when testing AIChat Agent operation continuation.
provider_type: builtin
provider_id: agent-management
runtime_type: tool
tools:
  - get_agent_config
  - update_agent_config
---

# Agent Management

Use agent management tools.
`
	if err := os.WriteFile(filepath.Join(agentRoot, "SKILL.md"), []byte(agentSkill), 0o644); err != nil {
		t.Fatalf("write agent management SKILL.md: %v", err)
	}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(builtinfiles.NewProvider(fileService, nil, workspacePerms)); err != nil {
		t.Fatalf("register files provider: %v", err)
	}
	if err := manager.RegisterProvider(builtinconsolenavigation.NewProvider()); err != nil {
		t.Fatalf("register console navigation provider: %v", err)
	}
	return skills.NewRuntimeWithCatalog(tools.NewToolEngine(manager), manager, catalogDir).
		WithToolGovernanceGateway(skills.NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy()))
}

type toolGovernanceStreamAccessRepo struct {
	repository.AccessRepository
}

func (toolGovernanceStreamAccessRepo) IsOrganizationMember(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return true, nil
}

type toolGovernanceStreamConversationRepo struct {
	repository.ConversationRepository
	conversation             *runtimemodel.Conversation
	updateAfterMessageCalled bool
	updateMetadataCalled     bool
}

func (r *toolGovernanceStreamConversationRepo) GetScoped(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*runtimemodel.Conversation, error) {
	return r.conversation, nil
}

func (r *toolGovernanceStreamConversationRepo) GetByCallerScoped(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ uuid.UUID, _ string, _ *uuid.UUID) (*runtimemodel.Conversation, error) {
	return r.conversation, nil
}

func (r *toolGovernanceStreamConversationRepo) UpdateAfterMessage(_ context.Context, _ uuid.UUID, leafMessageID uuid.UUID) error {
	r.updateAfterMessageCalled = true
	r.conversation.CurrentLeafMessageID = &leafMessageID
	r.conversation.RuntimeStatus = runtimemodel.ConversationRuntimeStatusIdle
	r.conversation.ActiveMessageID = nil
	r.conversation.UpdatedAt = time.Now()
	return nil
}

func (r *toolGovernanceStreamConversationRepo) UpdateMetadata(_ context.Context, _ uuid.UUID, metadata map[string]interface{}) error {
	r.updateMetadataCalled = true
	r.conversation.Metadata = copyStringAnyMap(metadata)
	return nil
}

func (r *toolGovernanceStreamConversationRepo) FinishContinuationMessage(_ context.Context, _ uuid.UUID, _ uuid.UUID) error {
	r.conversation.RuntimeStatus = runtimemodel.ConversationRuntimeStatusIdle
	r.conversation.ActiveMessageID = nil
	r.conversation.UpdatedAt = time.Now()
	return nil
}

type toolGovernanceStreamMessageRepo struct {
	repository.MessageRepository
	message                       *runtimemodel.Message
	updateMetadataCalled          bool
	updateMetadataAnyStatusCalled bool
	updateCompletedCalled         bool
	updateErrorCalled             bool
}

func (r *toolGovernanceStreamMessageRepo) GetScoped(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*runtimemodel.Message, error) {
	return r.message, nil
}

func (r *toolGovernanceStreamMessageRepo) ListBranch(context.Context, uuid.UUID, int) ([]*runtimemodel.Message, error) {
	return []*runtimemodel.Message{}, nil
}

func (r *toolGovernanceStreamMessageRepo) UpdateMetadata(_ context.Context, _ uuid.UUID, metadata map[string]interface{}) error {
	r.updateMetadataCalled = true
	r.message.Metadata = copyStringAnyMap(metadata)
	r.message.UpdatedAt = time.Now()
	return nil
}

func (r *toolGovernanceStreamMessageRepo) UpdateMetadataAnyStatus(_ context.Context, _ uuid.UUID, metadata map[string]interface{}) error {
	r.updateMetadataAnyStatusCalled = true
	r.message.Metadata = copyStringAnyMap(metadata)
	r.message.UpdatedAt = time.Now()
	return nil
}

func (r *toolGovernanceStreamMessageRepo) UpdateWaitingClientAction(_ context.Context, _ uuid.UUID, metadata map[string]interface{}) error {
	r.message.Status = runtimemodel.MessageStatusWaitingClientAction
	r.message.Metadata = copyStringAnyMap(metadata)
	r.message.UpdatedAt = time.Now()
	return nil
}

func (r *toolGovernanceStreamMessageRepo) UpdateCompleted(_ context.Context, _ uuid.UUID, answer string, metadata map[string]interface{}) error {
	r.updateCompletedCalled = true
	r.message.Answer = answer
	r.message.Status = runtimemodel.MessageStatusCompleted
	r.message.Error = nil
	r.message.Metadata = copyStringAnyMap(metadata)
	r.message.UpdatedAt = time.Now()
	return nil
}

func (r *toolGovernanceStreamMessageRepo) UpdateError(_ context.Context, _ uuid.UUID, message string) error {
	r.updateErrorCalled = true
	r.message.Status = runtimemodel.MessageStatusError
	r.message.Error = &message
	r.message.UpdatedAt = time.Now()
	return nil
}

type toolGovernanceStreamSkillConfigRepo struct {
	repository.OrganizationSkillConfigRepository
	skillID string
}

func (r toolGovernanceStreamSkillConfigRepo) ListByOrganization(context.Context, uuid.UUID) ([]*runtimemodel.OrganizationSkillConfig, error) {
	return []*runtimemodel.OrganizationSkillConfig{
		{SkillID: r.skillID, Enabled: true},
	}, nil
}

type toolGovernanceStreamModelSpecResolver struct{}

func (toolGovernanceStreamModelSpecResolver) Resolve(context.Context, uuid.UUID, string, string) (ModelSpec, bool, error) {
	return ModelSpec{SupportsToolCall: true}, true, nil
}

type toolGovernanceStreamFileService struct {
	files   map[string]*dto.UploadFile
	content map[string]string
	deleted []string
}

func (s *toolGovernanceStreamFileService) GetFileByID(_ context.Context, fileID string) (*dto.UploadFile, error) {
	if s == nil || s.files[fileID] == nil {
		return nil, errors.New("file not found")
	}
	return s.files[fileID], nil
}

func (s *toolGovernanceStreamFileService) GetFile(_ context.Context, fileID string) (string, error) {
	if s == nil {
		return "", errors.New("file not found")
	}
	content, ok := s.content[fileID]
	if !ok {
		return "", errors.New("file content not found")
	}
	return content, nil
}

func (s *toolGovernanceStreamFileService) UploadFile(context.Context, string, []byte, string, string, string, filemodel.CreatedByRole, *interfaces.FileSource, *string, bool, bool) (*dto.UploadFile, error) {
	return nil, errors.New("upload not supported in this test")
}

func (s *toolGovernanceStreamFileService) GetFileURL(_ context.Context, fileID string) (string, error) {
	if s == nil || s.files[fileID] == nil {
		return "", errors.New("file not found")
	}
	return "https://files.example/" + fileID, nil
}

func (s *toolGovernanceStreamFileService) DeleteFiles(_ context.Context, fileIDs []string) error {
	for _, fileID := range fileIDs {
		if s.files[fileID] == nil {
			return errors.New("file not found")
		}
		s.deleted = append(s.deleted, fileID)
		delete(s.files, fileID)
	}
	return nil
}

type toolGovernanceStreamWorkspacePermissionService struct {
	allowed bool
	codes   []workspacemodel.WorkspacePermissionCode
}

func (s *toolGovernanceStreamWorkspacePermissionService) CheckWorkspacePermission(_ context.Context, _, _, _ string, code workspacemodel.WorkspacePermissionCode) (bool, error) {
	s.codes = append(s.codes, code)
	return s.allowed, nil
}

type toolGovernanceStreamLLM struct {
	streamChunks     []string
	streamRequests   []*adapter.ChatRequest
	appContexts      []*llmclient.AppContext
	appChatResponses []*adapter.ChatResponse
	appChatRequests  []*adapter.ChatRequest
}

func (f *toolGovernanceStreamLLM) Chat(context.Context, string, *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	return nil, errors.New("unexpected Chat call")
}

func (f *toolGovernanceStreamLLM) ChatStream(context.Context, string, *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	return nil, errors.New("unexpected ChatStream call")
}

func (f *toolGovernanceStreamLLM) CreateResponse(context.Context, string, *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, errors.New("unexpected CreateResponse call")
}

func (f *toolGovernanceStreamLLM) Embed(context.Context, string, *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, errors.New("unexpected Embed call")
}

func (f *toolGovernanceStreamLLM) CreateImage(context.Context, string, *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return nil, errors.New("unexpected CreateImage call")
}

func (f *toolGovernanceStreamLLM) Rerank(context.Context, string, *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, errors.New("unexpected Rerank call")
}

func (f *toolGovernanceStreamLLM) AppChat(_ context.Context, appCtx *llmclient.AppContext, req *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	f.appContexts = append(f.appContexts, appCtx)
	f.appChatRequests = append(f.appChatRequests, cloneChatRequest(req))
	if len(f.appChatRequests) > len(f.appChatResponses) {
		return nil, errors.New("unexpected AppChat call")
	}
	return f.appChatResponses[len(f.appChatRequests)-1], nil
}

func (f *toolGovernanceStreamLLM) AppChatStream(_ context.Context, appCtx *llmclient.AppContext, req *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	f.appContexts = append(f.appContexts, appCtx)
	f.streamRequests = append(f.streamRequests, cloneChatRequest(req))
	ch := make(chan adapter.StreamResponse, len(f.streamChunks)+1)
	for _, chunk := range f.streamChunks {
		ch <- adapter.StreamResponse{
			Choices: []adapter.StreamChoice{
				{Delta: adapter.Message{Content: chunk}},
			},
		}
	}
	ch <- adapter.StreamResponse{
		Usage: &adapter.Usage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
		Done: true,
	}
	close(ch)
	return ch, nil
}

func (f *toolGovernanceStreamLLM) AppCreateResponse(context.Context, *llmclient.AppContext, *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, errors.New("unexpected AppCreateResponse call")
}

func (f *toolGovernanceStreamLLM) AppEmbed(context.Context, *llmclient.AppContext, *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, errors.New("unexpected AppEmbed call")
}

func (f *toolGovernanceStreamLLM) AppCreateImage(context.Context, *llmclient.AppContext, *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return nil, errors.New("unexpected AppCreateImage call")
}

func (f *toolGovernanceStreamLLM) AppRerank(context.Context, *llmclient.AppContext, *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, errors.New("unexpected AppRerank call")
}
