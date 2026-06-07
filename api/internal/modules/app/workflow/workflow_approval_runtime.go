package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/conversation"
	approvalruntime "github.com/zgiai/zgi/api/internal/modules/app/workflow/approval"
	graph_entities "github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	"github.com/zgiai/zgi/api/pkg/database"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const (
	approvalFormOutputKey         = "__approval_form"
	approvalFormIDOutputKey       = "__approval_form_id"
	workflowResumeStateInputKey   = "sys.workflow_resume_state"
	workflowResumePauseIDInputKey = "sys.workflow_resume_pause_id"
	workflowEventMessage          = "message"
	workflowEventMessageEnd       = "message_end"
)

type approvalRequestedEventContext struct {
	WorkflowRunID string
	NodeID        string
	NodeTitle     string
	IsDraft       bool
	TriggeredFrom string
}

func (h *WorkflowHandler) ResumeApprovalWorkflow(ctx context.Context, form *approvalruntime.Form) error {
	workflowService, ok := h.workflowService.(*WorkflowService)
	if !ok || workflowService == nil || workflowService.workflowRunLogRepo == nil {
		return fmt.Errorf("workflow service is not available")
	}

	run, err := workflowService.workflowRunLogRepo.GetByID(ctx, form.WorkflowRunID)
	if err != nil {
		return fmt.Errorf("load workflow run for approval resume: %w", err)
	}

	pauseService := workflowpause.NewService(database.GetDB())
	pauseRecord, _, pauseState, err := pauseService.GetActiveByWorkflowRunID(ctx, run.ID)
	if err != nil {
		return fmt.Errorf("load workflow pause for approval resume: %w", err)
	}

	inputs := pauseState.Request.Inputs
	if inputs == nil {
		inputs = workflowRunInputs(run)
	}
	responseMode := pauseState.Request.ResponseMode
	if responseMode == "" {
		responseMode = "streaming"
	}

	if err := workflowService.ResumeWorkflowRunLog(ctx, run.ID); err != nil {
		return err
	}

	if err := pauseService.MarkResumed(ctx, run.ID); err != nil {
		logger.WarnContext(ctx, "failed to mark approval pause resumed", "workflow_run_id", run.ID, err)
	}
	h.updateApprovalConversationMessageStatus(ctx, run.ID, conversation.AgentMessageStatusRunning, nil)

	req := &dto.DraftWorkflowRunRequest{
		Inputs:       inputs,
		ResponseMode: responseMode,
	}
	runType := pauseState.RunType
	if runType == "" {
		runType = approvalRunType(run)
	}
	isDraft := run.TriggeredFrom == "debugging" || run.Version == "draft"
	systemInputs := approvalResumeSystemInputs(run, inputs)
	systemInputs[workflowResumeStateInputKey] = pauseState
	systemInputs[workflowResumePauseIDInputKey] = pauseRecord.ID
	if conversationID, _ := systemInputs["sys.conversation_id"].(string); conversationID == "" {
		if storedConversationID := approvalResumeStoredConversationID(ctx, pauseService, run); storedConversationID != "" {
			systemInputs["sys.conversation_id"] = storedConversationID
		}
	}

	resultChan := make(chan *WorkflowStreamEvent, 100)
	errorChan := make(chan error, 10)
	doneChan := make(chan map[string]interface{}, 1)
	resumeStartedAt := time.Now()
	startedPayload := buildWorkflowStartedEventPayload(
		runType,
		run.ID,
		run.WorkflowID,
		run.SequenceNumber,
		systemInputs,
		resumeStartedAt.Unix(),
		workflowStartReasonResumption,
	)
	appendWorkflowRunEvent(ctx, run.TenantID, run.AgentID, run.ID, workflowpause.EventWorkflowStarted, startedPayload)

	go func() {
		defer close(doneChan)
		h.executeWorkflowStream(nil, ctx, run.TenantID, run.AgentID, req, run.CreatedBy, run.ID, run.ID, run.WorkflowID, systemInputs, run.SequenceNumber, resultChan, errorChan, doneChan, isDraft, runType, run.TriggeredFrom)
	}()

	return h.drainApprovalResumeStream(ctx, pauseService, workflowService, run, resultChan, errorChan, doneChan, resumeStartedAt, runType, systemInputs, inputs)
}

func (h *WorkflowHandler) ResumeQuestionAnswerWorkflow(ctx context.Context, workflowRunID string, resumeInputs map[string]interface{}) error {
	workflowService, ok := h.workflowService.(*WorkflowService)
	if !ok || workflowService == nil || workflowService.workflowRunLogRepo == nil {
		return fmt.Errorf("workflow service is not available")
	}
	workflowRunID = strings.TrimSpace(workflowRunID)
	if workflowRunID == "" {
		return fmt.Errorf("workflow_run_id is required")
	}
	run, err := workflowService.workflowRunLogRepo.GetByID(ctx, workflowRunID)
	if err != nil {
		return fmt.Errorf("load workflow run for question answer resume: %w", err)
	}
	pauseService := workflowpause.NewService(database.GetDB())
	pauseRecord, reasons, pauseState, err := pauseService.GetActiveByWorkflowRunID(ctx, run.ID)
	if err != nil {
		return fmt.Errorf("load workflow pause for question answer resume: %w", err)
	}
	hasQuestionReason := false
	for _, reason := range reasons {
		if reason.Type == workflowpause.ReasonTypeQuestionAnswerRequired {
			hasQuestionReason = true
			break
		}
	}
	if !hasQuestionReason {
		return fmt.Errorf("workflow run %s is not waiting for question answer", run.ID)
	}
	inputs := workflowRunInputs(run)
	if pauseState.Request.Inputs != nil {
		inputs = copyWorkflowAnyMap(pauseState.Request.Inputs)
	}
	for key, value := range resumeInputs {
		inputs[key] = value
	}
	responseMode := pauseState.Request.ResponseMode
	if responseMode == "" {
		responseMode = "streaming"
	}
	if err := workflowService.ResumeWorkflowRunLog(ctx, run.ID); err != nil {
		return err
	}
	if err := pauseService.MarkResumed(ctx, run.ID); err != nil {
		logger.WarnContext(ctx, "failed to mark question answer pause resumed", "workflow_run_id", run.ID, err)
	}
	req := &dto.DraftWorkflowRunRequest{
		Inputs:       inputs,
		ResponseMode: responseMode,
	}
	runType := pauseState.RunType
	if runType == "" {
		runType = approvalRunType(run)
	}
	isDraft := run.TriggeredFrom == "debugging" || run.Version == "draft"
	systemInputs := approvalResumeSystemInputs(run, inputs)
	systemInputs[workflowResumeStateInputKey] = pauseState
	systemInputs[workflowResumePauseIDInputKey] = pauseRecord.ID
	resultChan := make(chan *WorkflowStreamEvent, 100)
	errorChan := make(chan error, 10)
	doneChan := make(chan map[string]interface{}, 1)
	resumeStartedAt := time.Now()
	startedPayload := buildWorkflowStartedEventPayload(
		runType,
		run.ID,
		run.WorkflowID,
		run.SequenceNumber,
		systemInputs,
		resumeStartedAt.Unix(),
		workflowStartReasonResumption,
	)
	appendWorkflowRunEvent(ctx, run.TenantID, run.AgentID, run.ID, workflowpause.EventWorkflowStarted, startedPayload)
	appendWorkflowRunEvent(ctx, run.TenantID, run.AgentID, run.ID, workflowpause.EventQuestionAnswerSubmitted, buildQuestionAnswerSubmittedEvent(run.ID, pauseState, req.Inputs))
	go func() {
		defer close(doneChan)
		h.executeWorkflowStream(nil, ctx, run.TenantID, run.AgentID, req, run.CreatedBy, run.ID, run.ID, run.WorkflowID, systemInputs, run.SequenceNumber, resultChan, errorChan, doneChan, isDraft, runType, run.TriggeredFrom)
	}()
	return h.drainApprovalResumeStream(ctx, pauseService, workflowService, run, resultChan, errorChan, doneChan, resumeStartedAt, runType, systemInputs, inputs)
}

func detachWorkflowResumeState(systemInputs map[string]interface{}) (*workflowpause.State, bool) {
	if systemInputs == nil {
		return nil, false
	}

	resumeState, ok := systemInputs[workflowResumeStateInputKey].(*workflowpause.State)
	delete(systemInputs, workflowResumeStateInputKey)
	delete(systemInputs, workflowResumePauseIDInputKey)
	if !ok || resumeState == nil {
		return nil, false
	}

	return resumeState, true
}

func clearResumedNodeVariables(variablePool *graph_entities.VariablePool, nodeID string) {
	if variablePool == nil || nodeID == "" {
		return
	}
	variablePool.Remove([]string{nodeID})
}

func workflowResumePausedNodeIDs(executorState workflowpause.ExecutorState) []string {
	seen := make(map[string]struct{})
	pausedNodeIDs := make([]string, 0, len(executorState.PausedNodeIDs)+1)
	for _, nodeID := range executorState.PausedNodeIDs {
		if nodeID == "" {
			continue
		}
		if _, exists := seen[nodeID]; exists {
			continue
		}
		seen[nodeID] = struct{}{}
		pausedNodeIDs = append(pausedNodeIDs, nodeID)
	}
	if len(pausedNodeIDs) == 0 && executorState.PausedNodeID != "" {
		pausedNodeIDs = append(pausedNodeIDs, executorState.PausedNodeID)
	}
	return pausedNodeIDs
}

func (h *WorkflowHandler) drainApprovalResumeStream(ctx context.Context, pauseService *workflowpause.Service, workflowService *WorkflowService, run *WorkflowRunLog, resultChan <-chan *WorkflowStreamEvent, errorChan <-chan error, doneChan <-chan map[string]interface{}, resumeStartedAt time.Time, runType string, systemInputs map[string]interface{}, resumeInputs map[string]interface{}) error {
	messageEventSent := false
	approvalExpired := false
	answerSnapshots := newAnswerSnapshotWriter(h, run.ID, run.AgentID, run.CreatedBy, systemInputs, resumeInputs, run.TriggeredFrom)
	for {
		selection := receiveWorkflowStreamSelection(resultChan, errorChan, doneChan, ctx.Done())
		switch selection.kind {
		case workflowStreamSelectionResult:
			if selection.event == nil {
				continue
			}
			if selection.event.EventType == workflowEventAnswerSnapshotReady {
				if runType == "CONVERSATION_WORKFLOW" && answerSnapshots != nil {
					answerSnapshots.Persist(ctx, workflowAnswerSnapshotText(selection.event.Data), conversation.AgentMessageStatusRunning, false)
				}
				continue
			}
			if selection.event.EventType == workflowEventMessage {
				messageEventSent = true
			}
			if selection.event.EventType == workflowpause.EventApprovalExpired {
				approvalExpired = true
			}
			eventData := sanitizeWorkflowEventData(selection.event.Data)
			if selection.event.EventType == workflowpause.EventApprovalResultFilled && approvalResultFilledEventAlreadyRecorded(ctx, pauseService, run, eventData) {
				continue
			}
			_ = pauseService.AppendEvent(ctx, workflowpause.AppendEventParams{
				TenantID:      run.TenantID,
				AppID:         run.AgentID,
				WorkflowRunID: run.ID,
				EventType:     selection.event.EventType,
				EventData:     eventData,
			})
			if selection.event.EventType == workflowpause.EventWorkflowPaused {
				h.updateApprovalConversationMessageStatus(ctx, run.ID, conversation.AgentMessageStatusPendingApproval, nil)
				return nil
			}
			if selection.event.EventType == workflowpause.EventWorkflowFinished {
				h.updateApprovalConversationMessageStatus(ctx, run.ID, approvalConversationMessageStatusFromWorkflowEvent(eventData, approvalExpired), nil)
				return nil
			}
		case workflowStreamSelectionError:
			if selection.err == nil {
				continue
			}
			h.persistApprovalResumeError(ctx, pauseService, workflowService, run, selection.err, resumeStartedAt)
			return selection.err
		case workflowStreamSelectionDone:
			if selection.ok {
				h.persistApprovalResumeCompletion(ctx, pauseService, workflowService, run, selection.outputs, resumeStartedAt, runType, systemInputs, resumeInputs, messageEventSent, approvalExpired)
			}
			return nil
		case workflowStreamSelectionContextDone:
			err := ctx.Err()
			if err != nil {
				h.persistApprovalResumeError(ctx, pauseService, workflowService, run, err, resumeStartedAt)
			}
			return ctx.Err()
		case workflowStreamSelectionHeartbeat:
			continue
		default:
			return nil
		}
	}
}

func approvalResultFilledEventAlreadyRecorded(ctx context.Context, pauseService *workflowpause.Service, run *WorkflowRunLog, eventData map[string]interface{}) bool {
	if pauseService == nil || run == nil {
		return false
	}
	formID, _ := eventData["form_id"].(string)
	if formID == "" {
		return false
	}
	payload, err := pauseService.ListEvents(ctx, run.TenantID, run.ID, 0, 200)
	if err != nil {
		logger.WarnContext(ctx, "failed to check approval result filled event duplication", "workflow_run_id", run.ID, "form_id", formID, err)
		return false
	}
	for _, event := range payload.Events {
		if event.Event != workflowpause.EventApprovalResultFilled {
			continue
		}
		if existingFormID, _ := event.Data["form_id"].(string); existingFormID == formID {
			return true
		}
	}
	return false
}

func (h *WorkflowHandler) persistApprovalResumeCompletion(ctx context.Context, pauseService *workflowpause.Service, workflowService *WorkflowService, run *WorkflowRunLog, outputs map[string]interface{}, resumeStartedAt time.Time, runType string, systemInputs map[string]interface{}, resumeInputs map[string]interface{}, messageEventSent bool, approvalExpired bool) {
	if runType == "CONVERSATION_WORKFLOW" {
		previousAnswer := h.approvalExistingConversationAnswer(ctx, run)
		conversationID, answer := h.persistApprovalResumeConversationEvents(ctx, run, outputs, systemInputs, messageEventSent, previousAnswer)
		h.persistApprovalResumeConversationMessage(ctx, run, outputs, systemInputs, resumeInputs, conversationID, answer, approvalConversationMessageStatusFromOutputs(outputs, approvalExpired))
	}
	h.persistApprovalResumeFinished(ctx, pauseService, workflowService, run, outputs, resumeStartedAt)
}

func (h *WorkflowHandler) persistApprovalResumeConversationEvents(ctx context.Context, run *WorkflowRunLog, outputs map[string]interface{}, systemInputs map[string]interface{}, messageEventSent bool, previousAnswer string) (string, string) {
	if run == nil {
		return "", ""
	}
	conversationID := ""
	if value, ok := systemInputs["sys.conversation_id"].(string); ok {
		conversationID = value
	}
	if conversationID == "" {
		conversationID = workflowRunInputConversationID(*run)
	}
	answer := extractWorkflowAnswer(outputs)
	now := time.Now().Unix()
	if !messageEventSent {
		messageAnswer := approvalResumeMessageEventAnswer(answer, previousAnswer)
		if messageAnswer != "" || previousAnswer == "" {
			appendWorkflowRunEvent(ctx, run.TenantID, run.AgentID, run.ID, workflowEventMessage, map[string]interface{}{
				"id":              run.ID,
				"message_id":      run.ID,
				"conversation_id": conversationID,
				"answer":          messageAnswer,
				"created_at":      now,
			})
		}
	}
	appendWorkflowRunEvent(ctx, run.TenantID, run.AgentID, run.ID, workflowEventMessageEnd, map[string]interface{}{
		"id":              run.ID,
		"message_id":      run.ID,
		"conversation_id": conversationID,
		"metadata": map[string]interface{}{
			"annotation_reply":    nil,
			"retriever_resources": []interface{}{},
			"usage": map[string]interface{}{
				"prompt_tokens":         0,
				"prompt_unit_price":     "0.0",
				"prompt_price_unit":     "0.0",
				"prompt_price":          "0.0",
				"completion_tokens":     0,
				"completion_unit_price": "0.0",
				"completion_price_unit": "0.0",
				"completion_price":      "0.0",
				"total_tokens":          0,
			},
		},
		"created_at": now,
	})
	return conversationID, answer
}

func (h *WorkflowHandler) approvalExistingConversationAnswer(ctx context.Context, run *WorkflowRunLog) string {
	if h == nil || h.advancedChatHandler == nil || run == nil || run.ID == "" {
		return ""
	}
	existingMessages, err := h.advancedChatHandler.GetFirstMessagesByWorkflowRunIDs(ctx, []string{run.ID})
	if err != nil {
		logger.WarnContext(ctx, "failed to load existing approval conversation answer", "workflow_run_id", run.ID, err)
		return ""
	}
	if existing := existingMessages[run.ID]; existing != nil {
		return existing.Answer
	}
	return ""
}

func approvalResumeMessageEventAnswer(answer, previousAnswer string) string {
	if answer == "" || previousAnswer == "" {
		return answer
	}
	if strings.HasPrefix(answer, previousAnswer) {
		return strings.TrimPrefix(answer, previousAnswer)
	}
	return answer
}

func (h *WorkflowHandler) persistApprovalResumeConversationMessage(ctx context.Context, run *WorkflowRunLog, outputs map[string]interface{}, systemInputs map[string]interface{}, resumeInputs map[string]interface{}, conversationID string, answer string, messageStatus string) {
	if h == nil || h.advancedChatHandler == nil || run == nil || conversationID == "" {
		return
	}

	workflowRunUUID, err := uuid.Parse(run.ID)
	if err != nil {
		logger.WarnContext(ctx, "invalid workflow run id for approval resume message", "workflow_run_id", run.ID, err)
		return
	}
	existingMessages, err := h.advancedChatHandler.GetFirstMessagesByWorkflowRunIDs(ctx, []string{run.ID})
	if err != nil {
		logger.ErrorContext(ctx, "failed to check existing approval resume message", "workflow_run_id", run.ID, err)
		return
	}

	agentUUID, err := uuid.Parse(run.AgentID)
	if err != nil {
		logger.WarnContext(ctx, "invalid agent id for approval resume message", "agent_id", run.AgentID, err)
		return
	}
	conversationUUID, err := uuid.Parse(conversationID)
	if err != nil {
		logger.WarnContext(ctx, "invalid conversation id for approval resume message", "conversation_id", conversationID, "workflow_run_id", run.ID, err)
		return
	}

	inputs := approvalResumeMessageInputs(run, resumeInputs)
	query := approvalResumeQuery(systemInputs, inputs)
	if answer == "" {
		answer = extractWorkflowAnswer(outputs)
	}

	fromSource := approvalResumeFromSource(run, inputs)
	invokeFrom := approvalResumeInvokeFrom(run, inputs)
	userID := approvalResumeUserID(run, systemInputs)
	fromUserUUID, err := uuid.Parse(userID)
	if err != nil {
		logger.WarnContext(ctx, "invalid user id for approval resume message", "user_id", userID, "workflow_run_id", run.ID, err)
		return
	}

	var createdBy *uuid.UUID
	if createdByUUID, err := uuid.Parse(run.CreatedBy); err == nil {
		createdBy = &createdByUUID
	}

	if existing := existingMessages[run.ID]; existing != nil {
		answer = mergeApprovalConversationAnswer(existing.Answer, answer)
		messageData := approvalConversationMessageData{
			Query:      query,
			Answer:     answer,
			Status:     messageStatus,
			FromSource: fromSource,
			InvokeFrom: invokeFrom,
			FromUserID: fromUserUUID,
			CreatedBy:  createdBy,
			WebAppID:   run.WebAppID,
			Inputs:     inputs,
		}
		if err := updateApprovalConversationMessage(ctx, h, existing, messageData); err != nil {
			logger.ErrorContext(ctx, "failed to update approval resume conversation message", "conversation_id", conversationID, "workflow_run_id", run.ID, err)
		}
		return
	}

	_, err = h.advancedChatHandler.CreateWorkflowMessageWithInputsAndStatus(
		agentUUID,
		conversationUUID,
		workflowRunUUID,
		query,
		answer,
		fromSource,
		invokeFrom,
		fromUserUUID,
		createdBy,
		run.WebAppID,
		inputs,
		messageStatus,
	)
	if err != nil {
		logger.ErrorContext(ctx, "failed to create approval resume conversation message", "conversation_id", conversationID, "workflow_run_id", run.ID, err)
	}
}

func (h *WorkflowHandler) persistApprovalPauseConversationMessage(ctx context.Context, workflowRunID, agentID, accountID string, systemInputs map[string]interface{}, requestInputs map[string]interface{}, triggeredFrom string, answer string) {
	if h == nil || h.advancedChatHandler == nil || workflowRunID == "" {
		return
	}

	conversationID := workflowConversationID(systemInputs, requestInputs)
	if conversationID == "" {
		return
	}

	existingMessages, err := h.advancedChatHandler.GetFirstMessagesByWorkflowRunIDs(ctx, []string{workflowRunID})
	if err != nil {
		logger.ErrorContext(ctx, "failed to check existing approval pause message", "workflow_run_id", workflowRunID, err)
		return
	}
	if existingMessages[workflowRunID] != nil {
		existing := existingMessages[workflowRunID]
		messageData, err := buildApprovalPauseConversationMessageData(workflowRunID, agentID, accountID, conversationID, systemInputs, requestInputs, triggeredFrom, mergeApprovalConversationAnswer(existing.Answer, answer))
		if err != nil {
			logger.WarnContext(ctx, "invalid existing approval pause message data", "workflow_run_id", workflowRunID, err)
			return
		}
		if err := updateApprovalConversationMessage(ctx, h, existing, messageData); err != nil {
			logger.ErrorContext(ctx, "failed to update existing approval pause message", "conversation_id", conversationID, "workflow_run_id", workflowRunID, err)
		}
		return
	}

	messageData, err := buildApprovalPauseConversationMessageData(workflowRunID, agentID, accountID, conversationID, systemInputs, requestInputs, triggeredFrom, answer)
	if err != nil {
		logger.WarnContext(ctx, "invalid approval pause message data", "workflow_run_id", workflowRunID, err)
		return
	}

	_, err = h.advancedChatHandler.CreateWorkflowMessageWithInputsAndStatus(
		messageData.AgentID,
		messageData.ConversationID,
		messageData.WorkflowRunID,
		messageData.Query,
		messageData.Answer,
		messageData.FromSource,
		messageData.InvokeFrom,
		messageData.FromUserID,
		messageData.CreatedBy,
		messageData.WebAppID,
		messageData.Inputs,
		messageData.Status,
	)
	if err != nil {
		logger.ErrorContext(ctx, "failed to create approval pause conversation message", "conversation_id", conversationID, "workflow_run_id", workflowRunID, err)
	}
}

type approvalConversationMessageData struct {
	WorkflowRunID  uuid.UUID
	AgentID        uuid.UUID
	ConversationID uuid.UUID
	Query          string
	Answer         string
	Status         string
	FromSource     string
	InvokeFrom     string
	FromUserID     uuid.UUID
	CreatedBy      *uuid.UUID
	WebAppID       *string
	Inputs         map[string]interface{}
}

func buildApprovalPauseConversationMessageData(workflowRunID, agentID, accountID, conversationID string, systemInputs map[string]interface{}, requestInputs map[string]interface{}, triggeredFrom string, answer string) (approvalConversationMessageData, error) {
	run := &WorkflowRunLog{
		ID:            workflowRunID,
		AgentID:       agentID,
		CreatedBy:     accountID,
		TriggeredFrom: triggeredFrom,
		WebAppID:      approvalWebAppID(requestInputs),
	}
	if approvalResumeFromSource(run, requestInputs) == string(UserFromEndUser) {
		run.CreatedByRole = CreatedByRoleEndUser
	} else {
		run.CreatedByRole = CreatedByRoleAccount
	}

	workflowRunUUID, err := uuid.Parse(workflowRunID)
	if err != nil {
		return approvalConversationMessageData{}, fmt.Errorf("parse workflow run id: %w", err)
	}
	agentUUID, err := uuid.Parse(agentID)
	if err != nil {
		return approvalConversationMessageData{}, fmt.Errorf("parse agent id: %w", err)
	}
	conversationUUID, err := uuid.Parse(conversationID)
	if err != nil {
		return approvalConversationMessageData{}, fmt.Errorf("parse conversation id: %w", err)
	}
	fromUserUUID, err := uuid.Parse(approvalResumeUserID(run, systemInputs))
	if err != nil {
		return approvalConversationMessageData{}, fmt.Errorf("parse user id: %w", err)
	}

	var createdBy *uuid.UUID
	if createdByUUID, err := uuid.Parse(accountID); err == nil {
		createdBy = &createdByUUID
	}
	inputs := copyWorkflowAnyMap(requestInputs)
	return approvalConversationMessageData{
		WorkflowRunID:  workflowRunUUID,
		AgentID:        agentUUID,
		ConversationID: conversationUUID,
		Query:          approvalResumeQuery(systemInputs, inputs),
		Answer:         answer,
		Status:         conversation.AgentMessageStatusPendingApproval,
		FromSource:     approvalResumeFromSource(run, inputs),
		InvokeFrom:     approvalResumeInvokeFrom(run, inputs),
		FromUserID:     fromUserUUID,
		CreatedBy:      createdBy,
		WebAppID:       run.WebAppID,
		Inputs:         inputs,
	}, nil
}

func updateApprovalConversationMessage(ctx context.Context, h *WorkflowHandler, message *conversation.AgentMessage, data approvalConversationMessageData) error {
	message.Query = data.Query
	message.Answer = data.Answer
	message.Status = approvalConversationMessageStatus(data.Status)
	message.Error = nil
	message.FromSource = data.FromSource
	message.InvokeFrom = &data.InvokeFrom
	message.CreatedBy = data.CreatedBy
	message.WebAppID = data.WebAppID
	message.FromEndUserID = nil
	message.FromAccountID = nil
	if data.FromSource == string(UserFromEndUser) {
		message.FromEndUserID = &data.FromUserID
	} else {
		message.FromAccountID = &data.FromUserID
	}
	if err := message.SetInputsFromMap(data.Inputs); err != nil {
		return err
	}
	if err := message.SetMessageFromArray([]interface{}{
		map[string]interface{}{"role": "user", "content": data.Query},
		map[string]interface{}{"role": "assistant", "content": data.Answer},
	}); err != nil {
		return err
	}
	return h.advancedChatHandler.messageService.UpdateMessage(ctx, message)
}

func approvalConversationMessageStatus(status string) string {
	if status != "" {
		return status
	}
	return conversation.AgentMessageStatusCompleted
}

func mergeApprovalConversationAnswer(existingAnswer, nextAnswer string) string {
	if existingAnswer == "" {
		return nextAnswer
	}
	if nextAnswer == "" {
		return existingAnswer
	}
	if strings.HasPrefix(nextAnswer, existingAnswer) {
		return nextAnswer
	}
	if strings.HasPrefix(existingAnswer, nextAnswer) {
		return existingAnswer
	}
	return existingAnswer + nextAnswer
}

func (h *WorkflowHandler) updateApprovalConversationMessageStatus(ctx context.Context, workflowRunID string, status string, messageError *string) {
	if h == nil || h.advancedChatHandler == nil || workflowRunID == "" || status == "" {
		return
	}
	existingMessages, err := h.advancedChatHandler.GetFirstMessagesByWorkflowRunIDs(ctx, []string{workflowRunID})
	if err != nil {
		logger.ErrorContext(ctx, "failed to load approval conversation message for status update", "workflow_run_id", workflowRunID, err)
		return
	}
	existing := existingMessages[workflowRunID]
	if existing == nil {
		return
	}
	if err := h.advancedChatHandler.messageService.UpdateMessageStatus(ctx, existing.ID, status, messageError); err != nil {
		logger.ErrorContext(ctx, "failed to update approval conversation message status", "workflow_run_id", workflowRunID, "status", status, err)
	}
}

func approvalConversationMessageStatusFromOutputs(outputs map[string]interface{}, approvalExpired bool) string {
	if approvalExpired || workflowOutputsContainApprovalExpired(outputs) {
		return conversation.AgentMessageStatusExpired
	}
	if rawStatus, ok := outputs["__workflow_status__"].(string); ok {
		return workflowStatusToMessageStatus(rawStatus)
	}
	return conversation.AgentMessageStatusCompleted
}

func approvalConversationMessageStatusFromWorkflowEvent(eventData map[string]interface{}, approvalExpired bool) string {
	if approvalExpired {
		return conversation.AgentMessageStatusExpired
	}
	if rawStatus, ok := eventData["status"].(string); ok {
		return workflowStatusToMessageStatus(rawStatus)
	}
	return conversation.AgentMessageStatusCompleted
}

func workflowStatusToMessageStatus(workflowStatus string) string {
	switch workflowStatus {
	case "failed", "error":
		return conversation.AgentMessageStatusError
	case "stopped":
		return conversation.AgentMessageStatusStopped
	case "paused":
		return conversation.AgentMessageStatusPendingApproval
	default:
		return conversation.AgentMessageStatusCompleted
	}
}

func workflowOutputsContainApprovalExpired(value interface{}) bool {
	switch typed := value.(type) {
	case map[string]interface{}:
		if actionID, ok := typed["approval_action_id"].(string); ok && actionID == approvalruntime.ActionExpired {
			return true
		}
		if edgeHandle, ok := typed["approval_edge_source_handle"].(string); ok && edgeHandle == approvalruntime.ActionExpired {
			return true
		}
		for _, nested := range typed {
			if workflowOutputsContainApprovalExpired(nested) {
				return true
			}
		}
	case []interface{}:
		for _, nested := range typed {
			if workflowOutputsContainApprovalExpired(nested) {
				return true
			}
		}
	}
	return false
}

func workflowConversationID(systemInputs map[string]interface{}, requestInputs map[string]interface{}) string {
	if value, ok := systemInputs["sys.conversation_id"].(string); ok && value != "" {
		return value
	}
	if value, ok := requestInputs["sys.conversation_id"].(string); ok && value != "" {
		return value
	}
	return ""
}

func approvalWebAppID(inputs map[string]interface{}) *string {
	if value, ok := inputs["sys.web_app_id"].(string); ok && value != "" {
		return &value
	}
	return nil
}

func approvalResumeMessageInputs(run *WorkflowRunLog, resumeInputs map[string]interface{}) map[string]interface{} {
	if len(resumeInputs) > 0 {
		return copyWorkflowAnyMap(resumeInputs)
	}
	return workflowRunInputs(run)
}

func approvalResumeQuery(systemInputs map[string]interface{}, inputs map[string]interface{}) string {
	if value, ok := systemInputs["sys.query"].(string); ok {
		return value
	}
	if value, ok := inputs["sys.query"].(string); ok {
		return value
	}
	if value, ok := inputs["query"].(string); ok {
		return value
	}
	return ""
}

func approvalResumeUserID(run *WorkflowRunLog, systemInputs map[string]interface{}) string {
	if value, ok := systemInputs["sys.user_id"].(string); ok && value != "" {
		return value
	}
	if run == nil {
		return ""
	}
	return run.CreatedBy
}

func approvalResumeFromSource(run *WorkflowRunLog, inputs map[string]interface{}) string {
	if convParams, ok := inputs["conversation_params"].(map[string]interface{}); ok {
		if value, ok := convParams["from_source"].(string); ok && value != "" {
			return value
		}
	}
	if run != nil && run.CreatedByRole == CreatedByRoleEndUser {
		return string(UserFromEndUser)
	}
	return string(UserFromAccount)
}

func approvalResumeInvokeFrom(run *WorkflowRunLog, inputs map[string]interface{}) string {
	if convParams, ok := inputs["conversation_params"].(map[string]interface{}); ok {
		if value, ok := convParams["invoke_from"].(string); ok && value != "" {
			return value
		}
	}
	if run != nil && run.TriggeredFrom == string(InvokeFromWebApp) {
		return string(InvokeFromWebApp)
	}
	if run != nil && (run.TriggeredFrom == "debugging" || run.Version == "draft") {
		return string(InvokeFromDebugger)
	}
	return string(InvokeFromWorkflow)
}

func (h *WorkflowHandler) persistApprovalResumeFinished(ctx context.Context, pauseService *workflowpause.Service, workflowService *WorkflowService, run *WorkflowRunLog, outputs map[string]interface{}, resumeStartedAt time.Time) {
	if pauseService == nil || run == nil {
		return
	}
	fallbackElapsed := ElapsedMillisecondsSince(resumeStartedAt)
	if workflowService != nil {
		fallbackElapsed = workflowService.workflowRunElapsedMillisecondsForEvent(ctx, run.ID, fallbackElapsed)
	}
	elapsed := workflowElapsedMillisecondsFromOutputs(outputs, fallbackElapsed)
	eventData := workflowFinishedEventFromOutputs(run, workflowService, outputs, elapsed)
	appendWorkflowRunEvent(ctx, run.TenantID, run.AgentID, run.ID, workflowpause.EventWorkflowFinished, eventData)
}

func (h *WorkflowHandler) persistApprovalResumeError(ctx context.Context, pauseService *workflowpause.Service, workflowService *WorkflowService, run *WorkflowRunLog, err error, resumeStartedAt time.Time) {
	if pauseService == nil || run == nil || err == nil {
		return
	}
	errorPayload := buildWorkflowStreamErrorPayload(err)
	errorMessage := workflowStreamErrorMessage(errorPayload)
	h.updateApprovalConversationMessageStatus(ctx, run.ID, conversation.AgentMessageStatusError, &errorMessage)
	elapsed := ElapsedMillisecondsSince(resumeStartedAt)
	totalSteps := 0
	if workflowService != nil {
		elapsed = workflowService.workflowRunElapsedMillisecondsForEvent(ctx, run.ID, elapsed)
		totalSteps = workflowService.workflowRunNodeStepCount(ctx, run.ID)
		_ = workflowService.UpdateWorkflowRunLogStatus(ctx, run.ID, "failed", map[string]interface{}{}, elapsed, 0, totalSteps, errorMessage)
	}
	_ = pauseService.AppendEvent(ctx, workflowpause.AppendEventParams{
		TenantID:      run.TenantID,
		AppID:         run.AgentID,
		WorkflowRunID: run.ID,
		EventType:     workflowpause.EventError,
		EventData:     map[string]interface{}{"message": errorMessage},
	})
	_ = pauseService.AppendEvent(ctx, workflowpause.AppendEventParams{
		TenantID:      run.TenantID,
		AppID:         run.AgentID,
		WorkflowRunID: run.ID,
		EventType:     workflowpause.EventWorkflowFinished,
		EventData: map[string]interface{}{
			"id":               run.ID,
			"workflow_id":      run.WorkflowID,
			"sequence_number":  run.SequenceNumber,
			"status":           "failed",
			"outputs":          map[string]interface{}{},
			"error":            errorPayload,
			"elapsed_time":     elapsed,
			"total_tokens":     0,
			"total_steps":      totalSteps,
			"created_by":       map[string]interface{}{"id": run.CreatedBy, "name": "", "email": ""},
			"created_at":       time.Now().Unix(),
			"finished_at":      time.Now().Unix(),
			"exceptions_count": 1,
			"files":            []interface{}{},
		},
	})
}

func workflowFinishedEventFromOutputs(run *WorkflowRunLog, workflowService *WorkflowService, outputs map[string]interface{}, elapsed float64) map[string]interface{} {
	finalOutputs := make(map[string]interface{})
	for key, value := range outputs {
		finalOutputs[key] = value
	}

	status := "succeeded"
	var workflowError interface{}
	exceptionsCount := 0
	totalTokens := 0
	if rawStatus, exists := finalOutputs["__workflow_status__"]; exists {
		if value, ok := rawStatus.(string); ok && value != "" {
			status = value
		}
		delete(finalOutputs, "__workflow_status__")
	}
	if rawError, exists := finalOutputs["__workflow_error__"]; exists {
		if status == "failed" {
			workflowError = map[string]interface{}{"message": rawError}
			exceptionsCount = 1
		}
		delete(finalOutputs, "__workflow_error__")
	}
	if rawTokens, exists := finalOutputs["__total_tokens__"]; exists {
		switch value := rawTokens.(type) {
		case int:
			totalTokens = value
		case int64:
			totalTokens = int(value)
		case float64:
			totalTokens = int(value)
		}
		delete(finalOutputs, "__total_tokens__")
	}
	delete(finalOutputs, workflowInternalElapsedTimeKey)
	totalSteps := 0
	if workflowService != nil && run != nil {
		totalSteps = workflowService.workflowRunNodeStepCount(context.Background(), run.ID)
	}
	return map[string]interface{}{
		"id":               run.ID,
		"workflow_id":      run.WorkflowID,
		"sequence_number":  run.SequenceNumber,
		"status":           status,
		"outputs":          finalOutputs,
		"error":            workflowError,
		"elapsed_time":     elapsed,
		"total_tokens":     totalTokens,
		"total_steps":      totalSteps,
		"created_by":       map[string]interface{}{"id": run.CreatedBy, "name": "", "email": ""},
		"created_at":       time.Now().Unix(),
		"finished_at":      time.Now().Unix(),
		"exceptions_count": exceptionsCount,
		"files":            []interface{}{},
	}
}

func approvalRunType(run *WorkflowRunLog) string {
	if run != nil && run.Type == dto.WorkflowTypeChat {
		return "CONVERSATION_WORKFLOW"
	}
	return "WORKFLOW"
}

func workflowRunInputs(run *WorkflowRunLog) map[string]interface{} {
	inputs := make(map[string]interface{})
	if run == nil || run.Inputs == nil || *run.Inputs == "" {
		return inputs
	}
	if err := json.Unmarshal([]byte(*run.Inputs), &inputs); err != nil {
		return make(map[string]interface{})
	}
	return inputs
}

func approvalResumeSystemInputs(run *WorkflowRunLog, inputs map[string]interface{}) map[string]interface{} {
	systemInputs := map[string]interface{}{
		"sys.user_id":         run.CreatedBy,
		"sys.agent_id":        run.AgentID,
		"sys.workflow_id":     run.WorkflowID,
		"sys.workflow_run_id": run.ID,
		"sys.tenant_id":       run.TenantID,
		"sys.workspace_id":    run.TenantID,
	}

	if files, exists := inputs["#files#"]; exists {
		systemInputs["sys.files"] = files
	}
	for _, key := range []string{
		"sys.organization_id",
		"sys.billing_subject_type",
		"sys.conversation_id",
		"sys.query",
		"sys.dialogue_count",
		"sys.conversation_history",
		"sys.parent_message_id",
	} {
		if value, exists := inputs[key]; exists {
			systemInputs[key] = value
		}
	}
	if _, exists := systemInputs["sys.dialogue_count"]; !exists {
		systemInputs["sys.dialogue_count"] = 1
	}
	return systemInputs
}

func approvalResumeStoredConversationID(ctx context.Context, pauseService *workflowpause.Service, run *WorkflowRunLog) string {
	if run == nil {
		return ""
	}
	if conversationID := workflowRunInputConversationID(*run); conversationID != "" {
		return conversationID
	}
	if pauseService == nil {
		return ""
	}

	payload, err := pauseService.ListEvents(ctx, run.TenantID, run.ID, 0, 100)
	if err != nil {
		logger.WarnContext(ctx, "failed to load approval resume conversation id from events", "workflow_run_id", run.ID, err)
		return ""
	}
	for _, event := range payload.Events {
		if event.Event != workflowpause.EventWorkflowStarted {
			continue
		}
		if conversationID, ok := event.Data["conversation_id"].(string); ok && conversationID != "" {
			return conversationID
		}
		inputs, ok := event.Data["inputs"].(map[string]interface{})
		if !ok {
			continue
		}
		if conversationID, ok := inputs["sys.conversation_id"].(string); ok && conversationID != "" {
			return conversationID
		}
	}
	return ""
}

func persistApprovalPause(ctx context.Context, tenantID, appID, workflowRunID, nodeID string, reasons []workflowpause.Reason, state workflowpause.State) {
	service := workflowpause.NewService(database.GetDB())
	if _, err := service.Save(ctx, workflowpause.SaveParams{
		TenantID:      tenantID,
		AppID:         appID,
		WorkflowRunID: workflowRunID,
		NodeID:        nodeID,
		Reason:        workflowpause.ReasonTypeApprovalRequired,
		State:         state,
		Reasons:       reasons,
	}); err != nil {
		logger.WarnContext(ctx, "failed to save approval pause state", "workflow_run_id", workflowRunID, err)
	}
}

func buildApprovalRequestedEvent(ctx context.Context, eventContext approvalRequestedEventContext, outputs map[string]interface{}) map[string]interface{} {
	payload, ok := outputs[approvalFormOutputKey].(approvalruntime.FormPayload)
	if !ok {
		return nil
	}
	event := map[string]interface{}{
		"form_id":         payload.ID,
		"workflow_run_id": eventContext.WorkflowRunID,
		"node_id":         eventContext.NodeID,
		"node_title":      eventContext.NodeTitle,
		"content":         payload.Content,
		"fields":          payload.Fields,
		"actions":         payload.Actions,
		"submit_methods":  approvalRequestedSubmitMethods(payload.SubmitMethods),
		"expires_at":      payload.ExpirationAt,
	}
	if token := approvalRequestedToken(ctx, payload, eventContext); token != "" {
		event["token"] = token
	}
	return event
}

func approvalRequestedSubmitMethods(methods approvalruntime.SubmitMethods) map[string]interface{} {
	webAppEnabled := true
	if methods.WebApp.Enabled != nil {
		webAppEnabled = *methods.WebApp.Enabled
	}
	return map[string]interface{}{
		"webapp": map[string]interface{}{
			"enabled": webAppEnabled,
		},
		"email": map[string]interface{}{
			"enabled": methods.Email.Enabled,
		},
		"sms": map[string]interface{}{
			"enabled": methods.SMS.Enabled,
		},
	}
}

func approvalRequestedToken(ctx context.Context, payload approvalruntime.FormPayload, eventContext approvalRequestedEventContext) string {
	if payload.Token != "" {
		return payload.Token
	}
	if !approvalDebugTokenAllowed(eventContext) {
		return ""
	}
	token, err := approvalruntime.NewService(database.GetDB()).DebugAccessTokenByFormID(ctx, payload.ID)
	if err != nil {
		logger.WarnContext(ctx, "failed to load debug approval token", "form_id", payload.ID, err)
		return ""
	}
	return token
}

func approvalDebugTokenAllowed(eventContext approvalRequestedEventContext) bool {
	return eventContext.IsDraft || eventContext.TriggeredFrom == "debugging"
}

func buildApprovalCompletionEvent(workflowRunID, nodeID, nodeTitle string, nodeResultOutputs map[string]interface{}, processData map[string]interface{}) (string, map[string]interface{}) {
	actionID, _ := nodeResultOutputs["approval_action_id"].(string)
	if actionID == "" {
		return "", nil
	}
	formID, _ := processData["form_id"].(string)
	if actionID == approvalruntime.ActionExpired {
		expiresAt := processData["expires_at"]
		return workflowpause.EventApprovalExpired, map[string]interface{}{
			"form_id":         formID,
			"workflow_run_id": workflowRunID,
			"node_id":         nodeID,
			"node_title":      nodeTitle,
			"expires_at":      expiresAt,
		}
	}

	actionLabel, _ := nodeResultOutputs["approval_action_label"].(string)
	renderedContent, _ := nodeResultOutputs["approval_rendered_content"].(string)
	inputs := make(map[string]interface{})
	for key, value := range nodeResultOutputs {
		switch key {
		case "approval_action_id", "approval_action_label", "approval_rendered_content":
			continue
		default:
			inputs[key] = value
		}
	}
	return workflowpause.EventApprovalResultFilled, map[string]interface{}{
		"form_id":          formID,
		"workflow_run_id":  workflowRunID,
		"node_id":          nodeID,
		"node_title":       nodeTitle,
		"action_id":        actionID,
		"action_label":     actionLabel,
		"inputs":           inputs,
		"rendered_content": renderedContent,
	}
}

func copyWorkflowAnyMap(input map[string]interface{}) map[string]interface{} {
	if input == nil {
		return map[string]interface{}{}
	}
	output := make(map[string]interface{}, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func copyWorkflowBoolMap(input map[string]bool) map[string]bool {
	if input == nil {
		return map[string]bool{}
	}
	output := make(map[string]bool, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func copyWorkflowStringMap(input map[string]string) map[string]string {
	if input == nil {
		return map[string]string{}
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func copyWorkflowNestedMap(input map[string]map[string]interface{}) map[string]map[string]interface{} {
	if input == nil {
		return map[string]map[string]interface{}{}
	}
	output := make(map[string]map[string]interface{}, len(input))
	for key, value := range input {
		output[key] = copyWorkflowAnyMap(value)
	}
	return output
}
