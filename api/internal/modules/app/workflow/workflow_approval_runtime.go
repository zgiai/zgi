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
	return h.resumeApprovalWorkflow(ctx, form, nil)
}

func (h *WorkflowHandler) ResumeApprovalWorkflowStream(ctx context.Context, form *approvalruntime.Form, onEvent func(string, map[string]interface{}) error) error {
	return h.resumeApprovalWorkflow(ctx, form, onEvent)
}

func (h *WorkflowHandler) resumeApprovalWorkflow(ctx context.Context, form *approvalruntime.Form, onEvent func(string, map[string]interface{}) error) error {
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

	if run.RuntimeProtocolVersion >= workflowRuntimeProtocolVersionV2 {
		claim, claimErr := claimWorkflowResume(ctx, pauseService, run, pauseRecord.ID)
		if claimErr != nil {
			return claimErr
		}
		ctx = withWorkflowExecutionOwner(ctx, workflowExecutionOwner{WorkflowRunID: run.ID, ExecutionID: claim.ExecutionID, Generation: claim.Generation, PauseID: claim.PauseID, PauseGeneration: claim.PauseGeneration})
		var stopRenewal func()
		ctx, stopRenewal = startWorkflowExecutionLeaseRenewal(ctx, pauseService, *claim)
		defer stopRenewal()
	} else {
		if err := h.resumeLegacyWorkflowContinuation(ctx, workflowService, pauseService, run, "approval_resume"); err != nil {
			return err
		}
	}
	ctx, cancelResume := context.WithCancelCause(ctx)
	defer cancelResume(nil)

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
	eventDispatcher := newWorkflowRunEventDispatcher(run.TenantID, run.AgentID, run.ID, run.RuntimeProtocolVersion < workflowRuntimeProtocolVersionV2, workflowResumeEventHandler(onEvent))
	defer func() { _ = eventDispatcher.Close(ctx) }()
	if run.RuntimeProtocolVersion < workflowRuntimeProtocolVersionV2 {
		if err := eventDispatcher.Dispatch(ctx, workflowpause.EventWorkflowStarted, startedPayload); err != nil {
			return err
		}
	}

	go func() {
		defer close(doneChan)
		h.executeWorkflowStream(ctx, run.TenantID, run.AgentID, req, run.CreatedBy, run.ID, run.ID, run.WorkflowID, systemInputs, run.SequenceNumber, resultChan, errorChan, doneChan, isDraft, runType, run.TriggeredFrom)
	}()

	return h.drainApprovalResumeStream(ctx, pauseService, workflowService, run, resultChan, errorChan, doneChan, resumeStartedAt, runType, systemInputs, inputs, eventDispatcher)
}

func (h *WorkflowHandler) ResumeQuestionAnswerWorkflow(ctx context.Context, workflowRunID string, resumeInputs map[string]interface{}) error {
	return h.resumeQuestionAnswerWorkflow(ctx, workflowRunID, resumeInputs, nil)
}

func (h *WorkflowHandler) ResumeQuestionAnswerWorkflowStream(ctx context.Context, workflowRunID string, resumeInputs map[string]interface{}, onEvent func(string, map[string]interface{}) error) error {
	return h.resumeQuestionAnswerWorkflow(ctx, workflowRunID, resumeInputs, onEvent)
}

func (h *WorkflowHandler) resumeQuestionAnswerWorkflow(ctx context.Context, workflowRunID string, resumeInputs map[string]interface{}, onEvent func(string, map[string]interface{}) error) error {
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
	var submittedEvent *workflowpause.RunEventPayload
	if run.RuntimeProtocolVersion >= workflowRuntimeProtocolVersionV2 {
		submission, submitErr := pauseService.SubmitInteraction(
			ctx,
			run.ID,
			pauseRecord.ID,
			"question-answer",
			workflowpause.EventQuestionAnswerSubmitted,
			buildQuestionAnswerSubmittedEvent(run.ID, pauseState, inputs),
			questionAnswerSubmissionIdempotencyKey(pauseRecord.ID, pauseRecord.Generation, pauseState, inputs),
		)
		if submitErr != nil {
			return submitErr
		}
		if submission != nil {
			submittedEvent = submission.Event
			publishWorkflowCommittedTail(ctx, run.ID, submission.Event)
			for _, pendingEvent := range submission.PendingEvents {
				publishWorkflowCommittedTail(ctx, run.ID, pendingEvent)
			}
			if !submission.ResumeReady {
				if onEvent != nil && submission.Event != nil {
					data := copyWorkflowAnyMap(submission.Event.Data)
					data["sequence"] = submission.Event.Sequence
					data["event_id"] = submission.Event.EventID
					if err := onEvent(workflowpause.EventQuestionAnswerSubmitted, data); err != nil {
						return err
					}
				}
				for _, pendingEvent := range submission.PendingEvents {
					if onEvent == nil || pendingEvent == nil {
						continue
					}
					data := copyWorkflowAnyMap(pendingEvent.Data)
					data["sequence"] = pendingEvent.Sequence
					data["event_id"] = pendingEvent.EventID
					if err := onEvent(pendingEvent.Event, data); err != nil {
						return err
					}
				}
				return nil
			}
		}
		claim, claimErr := claimWorkflowResume(ctx, pauseService, run, pauseRecord.ID)
		if claimErr != nil {
			return claimErr
		}
		ctx = withWorkflowExecutionOwner(ctx, workflowExecutionOwner{WorkflowRunID: run.ID, ExecutionID: claim.ExecutionID, Generation: claim.Generation, PauseID: claim.PauseID, PauseGeneration: claim.PauseGeneration})
		var stopRenewal func()
		ctx, stopRenewal = startWorkflowExecutionLeaseRenewal(ctx, pauseService, *claim)
		defer stopRenewal()
	} else {
		if err := h.resumeLegacyWorkflowContinuation(ctx, workflowService, pauseService, run, "question_resume"); err != nil {
			return err
		}
	}
	ctx, cancelResume := context.WithCancelCause(ctx)
	defer cancelResume(nil)
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
	eventDispatcher := newWorkflowRunEventDispatcher(run.TenantID, run.AgentID, run.ID, run.RuntimeProtocolVersion < workflowRuntimeProtocolVersionV2, workflowResumeEventHandler(onEvent))
	defer func() { _ = eventDispatcher.Close(ctx) }()
	if run.RuntimeProtocolVersion < workflowRuntimeProtocolVersionV2 {
		if err := eventDispatcher.Dispatch(ctx, workflowpause.EventWorkflowStarted, startedPayload); err != nil {
			return err
		}
	}
	questionSubmittedData := buildQuestionAnswerSubmittedEvent(run.ID, pauseState, req.Inputs)
	if submittedEvent != nil {
		questionSubmittedData["__stored_sequence"] = submittedEvent.Sequence
		questionSubmittedData["__stored_event_id"] = submittedEvent.EventID
		questionSubmittedData["__stored_event_payload"] = submittedEvent
	}
	if err := eventDispatcher.Dispatch(ctx, workflowpause.EventQuestionAnswerSubmitted, questionSubmittedData); err != nil {
		return err
	}
	go func() {
		defer close(doneChan)
		h.executeWorkflowStream(ctx, run.TenantID, run.AgentID, req, run.CreatedBy, run.ID, run.ID, run.WorkflowID, systemInputs, run.SequenceNumber, resultChan, errorChan, doneChan, isDraft, runType, run.TriggeredFrom)
	}()
	return h.drainApprovalResumeStream(ctx, pauseService, workflowService, run, resultChan, errorChan, doneChan, resumeStartedAt, runType, systemInputs, inputs, eventDispatcher)
}

func (h *WorkflowHandler) StopWorkflowContinuation(ctx context.Context, workflowRunID string, accountID string) error {
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
		return fmt.Errorf("load workflow run for stop: %w", err)
	}
	if err := workflowService.StopWorkflowTask(ctx, run.TenantID, run.AgentID, run.ID, accountID); err != nil {
		return err
	}
	if run.RuntimeProtocolVersion >= workflowRuntimeProtocolVersionV2 {
		return nil
	}
	now := time.Now().Unix()
	payload := map[string]interface{}{
		"id":              run.ID,
		"workflow_run_id": run.ID,
		"workflow_id":     run.WorkflowID,
		"status":          "stopped",
		"error":           "Workflow stopped by user.",
		"created_at":      now,
	}
	appendLegacyWorkflowStopEvents(ctx, run, payload)
	return nil
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

func workflowResumeEventHandler(onEvent func(string, map[string]interface{}) error) workflowRunEventHandler {
	if onEvent == nil {
		return nil
	}
	return func(eventType string, data map[string]interface{}, stored *workflowpause.RunEventPayload) error {
		return onEvent(eventType, data)
	}
}

func (h *WorkflowHandler) drainApprovalResumeStream(ctx context.Context, pauseService *workflowpause.Service, workflowService *WorkflowService, run *WorkflowRunLog, resultChan <-chan *WorkflowStreamEvent, errorChan <-chan error, doneChan <-chan map[string]interface{}, resumeStartedAt time.Time, runType string, systemInputs map[string]interface{}, resumeInputs map[string]interface{}, eventDispatcher *workflowRunEventDispatcher) error {
	messageEventSent := false
	approvalExpired := false
	existingAnswer := h.approvalExistingConversationAnswer(ctx, run)
	conversationAnswer := newWorkflowConversationAnswerAccumulator(existingAnswer)
	answerSnapshots := newWorkflowAnswerSnapshotWriter(runType, h, run.ID, run.AgentID, run.CreatedBy, systemInputs, resumeInputs, run.TriggeredFrom)
	if answerSnapshots != nil {
		defer answerSnapshots.closeWithoutFlush()
		answerSnapshots.SeedPersistedAnswer(existingAnswer)
	}
	for {
		selection := receiveWorkflowStreamSelection(resultChan, errorChan, doneChan, ctx.Done())
		switch selection.kind {
		case workflowStreamSelectionResult:
			if selection.event == nil {
				continue
			}
			if selection.event.EventType == workflowEventAnswerSnapshotReady {
				var persistErr error
				if runType == "CONVERSATION_WORKFLOW" && answerSnapshots != nil {
					conversationAnswer.Merge(workflowAnswerSnapshotText(selection.event.Data))
					if forceFlush, _ := selection.event.Data["force_flush"].(bool); forceFlush {
						persistErr = answerSnapshots.Persist(ctx, conversationAnswer.String(), conversation.AgentMessageStatusRunning, true)
					} else {
						answerSnapshots.PersistAsync(ctx, conversationAnswer.String(), conversation.AgentMessageStatusRunning, false)
					}
				}
				if selection.event.Persisted != nil {
					selection.event.Persisted <- persistErr
				}
				continue
			}
			if selection.event.EventType == workflowEventMessage && workflowMessageEventKind(selection.event.Data) != workflowMessageKindQuestionAnswerPrompt {
				messageEventSent = true
				conversationAnswer.Append(workflowMessageEventText(selection.event.Data))
				if runType == "CONVERSATION_WORKFLOW" && answerSnapshots != nil {
					// Approval continuations do not have a long-lived POST SSE
					// transport. Feed every visible message into the coalescing
					// writer so snapshot+tail observers can see the answer while a
					// container is still running. The writer bounds database writes
					// to the 750ms/4KiB checkpoint cadence.
					answerSnapshots.PersistAsync(ctx, conversationAnswer.String(), conversation.AgentMessageStatusRunning, false)
				}
			}
			if selection.event.EventType == workflowpause.EventApprovalExpired {
				approvalExpired = true
			}
			eventData := sanitizeWorkflowEventData(selection.event.Data)
			if selection.event.EventType == workflowpause.EventWorkflowFinished && run.RuntimeProtocolVersion >= workflowRuntimeProtocolVersionV2 {
				// The executor terminal signal is transient. The done branch owns the
				// durable FinalizeRun transaction and only publishes after it commits.
				continue
			}
			if selection.event.EventType == workflowpause.EventWorkflowFinished && runType == "CONVERSATION_WORKFLOW" {
				eventData = workflowEventDataWithConversationAnswer(eventData, conversationAnswer.String())
				if answerSnapshots != nil {
					if err := answerSnapshots.PersistFinal(ctx, conversationAnswer.String(), approvalConversationMessageStatusFromWorkflowEvent(eventData, approvalExpired)); err != nil {
						return err
					}
				}
				persistWorkflowRunConversationAnswer(ctx, workflowService, run, eventData, conversationAnswer.String())
			}
			if selection.event.EventType == workflowpause.EventApprovalResultFilled && approvalResultFilledEventAlreadyRecorded(ctx, pauseService, run, eventData) {
				continue
			}
			if selection.event.EventType == workflowpause.EventWorkflowPaused {
				if run.RuntimeProtocolVersion >= workflowRuntimeProtocolVersionV2 {
					if answerSnapshots != nil {
						answerSnapshots.closeWithoutFlush()
					}
				} else if answerSnapshots != nil {
					if err := answerSnapshots.PersistFinal(ctx, conversationAnswer.String(), conversation.AgentMessageStatusPendingApproval); err != nil {
						return err
					}
				}
				if err := eventDispatcher.Dispatch(ctx, selection.event.EventType, eventData); err != nil {
					return err
				}
				return nil
			}
			if err := eventDispatcher.Dispatch(ctx, selection.event.EventType, eventData); err != nil {
				return err
			}
			if selection.event.EventType == workflowpause.EventWorkflowFinished {
				return nil
			}
		case workflowStreamSelectionError:
			if selection.err == nil {
				continue
			}
			if runType == "CONVERSATION_WORKFLOW" && answerSnapshots != nil {
				if run.RuntimeProtocolVersion >= workflowRuntimeProtocolVersionV2 {
					answerSnapshots.closeWithoutFlush()
				} else if err := answerSnapshots.PersistFinal(ctx, conversationAnswer.String(), conversation.AgentMessageStatusError); err != nil {
					return err
				}
			}
			h.persistApprovalResumeError(ctx, pauseService, workflowService, run, selection.err, resumeStartedAt, eventDispatcher, conversationAnswer.String())
			return selection.err
		case workflowStreamSelectionDone:
			if selection.ok {
				if runType == "CONVERSATION_WORKFLOW" && answerSnapshots != nil {
					if run.RuntimeProtocolVersion >= workflowRuntimeProtocolVersionV2 {
						answerSnapshots.closeWithoutFlush()
					} else if err := answerSnapshots.PersistFinal(ctx, conversationAnswer.String(), conversation.AgentMessageStatusCompleted); err != nil {
						return err
					}
				}
				if err := h.persistApprovalResumeCompletion(ctx, pauseService, workflowService, run, selection.outputs, resumeStartedAt, runType, systemInputs, resumeInputs, messageEventSent, approvalExpired, eventDispatcher, conversationAnswer.String()); err != nil {
					return err
				}
			}
			return nil
		case workflowStreamSelectionContextDone:
			err := ctx.Err()
			if err != nil {
				h.persistApprovalResumeError(ctx, pauseService, workflowService, run, err, resumeStartedAt, eventDispatcher, conversationAnswer.String())
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

func (h *WorkflowHandler) persistApprovalResumeCompletion(ctx context.Context, pauseService *workflowpause.Service, workflowService *WorkflowService, run *WorkflowRunLog, outputs map[string]interface{}, resumeStartedAt time.Time, runType string, systemInputs map[string]interface{}, resumeInputs map[string]interface{}, messageEventSent bool, approvalExpired bool, eventDispatcher *workflowRunEventDispatcher, streamedAnswer string) error {
	if runType == "CONVERSATION_WORKFLOW" {
		previousAnswer := h.approvalExistingConversationAnswer(ctx, run)
		conversationID, answer, terminalMessageEnd := h.persistApprovalResumeConversationEvents(ctx, run, outputs, systemInputs, messageEventSent, previousAnswer, streamedAnswer)
		messageStatus := approvalConversationMessageStatusFromOutputs(outputs, approvalExpired)
		// Agent-embedded conversational workflows project their answer into the
		// host chat runtime message, not into agents_messages. Requiring a
		// workflow-owned message projection here would roll back an otherwise
		// successful terminal transaction after an approval continuation.
		if run.RuntimeProtocolVersion >= workflowRuntimeProtocolVersionV2 {
			var err error
			messageStatus, terminalMessageEnd, err = resolveWorkflowConversationProjection(ctx, run.ID, conversationID, messageStatus, terminalMessageEnd)
			if err != nil {
				return fmt.Errorf("resolve resumed workflow conversation projection: %w", err)
			}
		}
		if run.RuntimeProtocolVersion < workflowRuntimeProtocolVersionV2 {
			h.persistApprovalResumeConversationMessage(ctx, run, outputs, systemInputs, resumeInputs, conversationID, answer, messageStatus)
		}
		outputs = workflowOutputsWithConversationAnswer(outputs, answer)
		if run.RuntimeProtocolVersion < workflowRuntimeProtocolVersionV2 {
			persistWorkflowRunConversationAnswerFromOutputs(ctx, workflowService, run, outputs, answer)
		}
		return h.persistApprovalResumeFinished(ctx, pauseService, workflowService, run, outputs, resumeStartedAt, eventDispatcher, answer, messageStatus, terminalMessageEnd)
	}
	return h.persistApprovalResumeFinished(ctx, pauseService, workflowService, run, outputs, resumeStartedAt, eventDispatcher, "", "", nil)
}

func workflowOwnsConversationProjection(ctx context.Context, workflowRunID, conversationID string) (bool, error) {
	if strings.TrimSpace(workflowRunID) == "" || strings.TrimSpace(conversationID) == "" {
		return false, nil
	}
	var count int64
	err := database.GetDB().WithContext(ctx).Model(&conversation.AgentMessage{}).
		Where("workflow_run_id = ? AND conversation_id = ? AND deleted_at IS NULL", workflowRunID, conversationID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func resolveWorkflowConversationProjection(ctx context.Context, workflowRunID, conversationID, messageStatus string, messageEnd map[string]interface{}) (string, map[string]interface{}, error) {
	if messageStatus == "" || len(messageEnd) == 0 {
		return "", nil, nil
	}
	ownsProjection, err := workflowOwnsConversationProjection(ctx, workflowRunID, conversationID)
	if err != nil {
		return "", nil, err
	}
	if !ownsProjection {
		return "", nil, nil
	}
	return messageStatus, messageEnd, nil
}

func (h *WorkflowHandler) persistApprovalResumeConversationEvents(ctx context.Context, run *WorkflowRunLog, outputs map[string]interface{}, systemInputs map[string]interface{}, messageEventSent bool, previousAnswer string, streamedAnswer string) (string, string, map[string]interface{}) {
	if run == nil {
		return "", "", nil
	}
	conversationID := ""
	if value, ok := systemInputs["sys.conversation_id"].(string); ok {
		conversationID = value
	}
	if conversationID == "" {
		conversationID = workflowRunInputConversationID(*run)
	}
	answer := extractWorkflowAnswer(outputs)
	if answer != "" {
		// The terminal workflow output is authoritative. Streamed chunks and
		// snapshots are a live projection and must not override it.
		answer = mergeApprovalConversationAnswer(previousAnswer, answer)
	} else if streamedAnswer != "" {
		answer = mergeApprovalConversationAnswer(previousAnswer, streamedAnswer)
	}
	now := time.Now().Unix()
	if !messageEventSent && run.RuntimeProtocolVersion < workflowRuntimeProtocolVersionV2 {
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
	messageEnd := map[string]interface{}{
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
	}
	if run.RuntimeProtocolVersion < workflowRuntimeProtocolVersionV2 {
		appendWorkflowRunEvent(ctx, run.TenantID, run.AgentID, run.ID, workflowEventMessageEnd, messageEnd)
	}
	return conversationID, answer, messageEnd
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

type workflowConversationAnswerAccumulator struct {
	baseAnswer     string
	streamedAnswer string
}

func newWorkflowConversationAnswerAccumulator(existingAnswer string) *workflowConversationAnswerAccumulator {
	return &workflowConversationAnswerAccumulator{baseAnswer: existingAnswer}
}

func (a *workflowConversationAnswerAccumulator) Append(chunk string) {
	if a == nil || chunk == "" {
		return
	}
	a.streamedAnswer += chunk
}

func (a *workflowConversationAnswerAccumulator) Merge(snapshot string) {
	if a == nil {
		return
	}
	// answer_snapshot_ready is an absolute snapshot for the current execution,
	// not another delta. Replacing the live tail prevents cumulative snapshots
	// from being appended after their corresponding message chunks.
	a.streamedAnswer = snapshot
}

func (a *workflowConversationAnswerAccumulator) String() string {
	if a == nil {
		return ""
	}
	return mergeApprovalConversationAnswer(a.baseAnswer, a.streamedAnswer)
}

func workflowOutputsWithConversationAnswer(outputs map[string]interface{}, answer string) map[string]interface{} {
	result := copyWorkflowAnyMap(outputs)
	if answer != "" && extractWorkflowAnswer(result) == "" {
		result["answer"] = answer
	}
	return result
}

func workflowEventDataWithConversationAnswer(data map[string]interface{}, answer string) map[string]interface{} {
	result := copyWorkflowAnyMap(data)
	outputs, _ := result["outputs"].(map[string]interface{})
	result["outputs"] = workflowOutputsWithConversationAnswer(outputs, answer)
	return result
}

func persistWorkflowRunConversationAnswer(ctx context.Context, workflowService *WorkflowService, run *WorkflowRunLog, eventData map[string]interface{}, answer string) {
	if workflowService == nil || run == nil || answer == "" {
		return
	}
	outputs, _ := eventData["outputs"].(map[string]interface{})
	status, _ := eventData["status"].(string)
	if status == "" {
		status = "succeeded"
	}
	elapsed, _ := workflowFloatValue(eventData["elapsed_time"])
	totalTokens, _ := workflowEventInt(eventData["total_tokens"])
	totalSteps, _ := workflowEventInt(eventData["total_steps"])
	errorMessage := workflowRunEventErrorMessage(eventData["error"])
	if err := workflowService.UpdateWorkflowRunLogStatus(ctx, run.ID, status, workflowOutputsWithConversationAnswer(outputs, answer), elapsed, int64(totalTokens), totalSteps, errorMessage); err != nil {
		logger.ErrorContext(ctx, "failed to persist workflow conversation answer", "workflow_run_id", run.ID, err)
	}
}

func persistWorkflowRunConversationAnswerFromOutputs(ctx context.Context, workflowService *WorkflowService, run *WorkflowRunLog, outputs map[string]interface{}, answer string) {
	if workflowService == nil || run == nil || answer == "" {
		return
	}
	persistedOutputs := workflowOutputsWithConversationAnswer(outputs, answer)
	status := "succeeded"
	if value, ok := persistedOutputs["__workflow_status__"].(string); ok && value != "" {
		status = value
	}
	errorMessage, _ := persistedOutputs["__workflow_error__"].(string)
	totalTokens, _ := workflowEventInt(persistedOutputs["__total_tokens__"])
	elapsed, _ := workflowFloatValue(persistedOutputs[workflowInternalElapsedTimeKey])
	delete(persistedOutputs, "__workflow_status__")
	delete(persistedOutputs, "__workflow_error__")
	delete(persistedOutputs, "__total_tokens__")
	delete(persistedOutputs, workflowInternalElapsedTimeKey)
	totalSteps := workflowService.workflowRunNodeStepCount(ctx, run.ID)
	if err := workflowService.UpdateWorkflowRunLogStatus(ctx, run.ID, status, persistedOutputs, elapsed, int64(totalTokens), totalSteps, errorMessage); err != nil {
		logger.ErrorContext(ctx, "failed to persist completed workflow conversation answer", "workflow_run_id", run.ID, err)
	}
}

func workflowRunEventErrorMessage(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case map[string]interface{}:
		if message, ok := typed["message"].(string); ok {
			return message
		}
	}
	return ""
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

func (h *WorkflowHandler) persistApprovalResumeFinished(ctx context.Context, pauseService *workflowpause.Service, workflowService *WorkflowService, run *WorkflowRunLog, outputs map[string]interface{}, resumeStartedAt time.Time, eventDispatcher *workflowRunEventDispatcher, finalAnswer, messageStatus string, messageEnd map[string]interface{}) error {
	if pauseService == nil || run == nil {
		return nil
	}
	fallbackElapsed := ElapsedMillisecondsSince(resumeStartedAt)
	if workflowService != nil {
		fallbackElapsed = workflowService.workflowRunElapsedMillisecondsForEvent(ctx, run.ID, fallbackElapsed)
	}
	elapsed := workflowElapsedMillisecondsFromOutputs(outputs, fallbackElapsed)
	if workflowService != nil {
		if persistedElapsed := workflowService.workflowRunNodeElapsedMilliseconds(ctx, run.ID); persistedElapsed > 0 {
			elapsed = persistedElapsed
		}
	}
	eventData := workflowFinishedEventFromOutputs(run, workflowService, outputs, elapsed)
	if metadata, ok := messageEnd["metadata"].(map[string]interface{}); ok {
		if usage, ok := metadata["usage"].(map[string]interface{}); ok {
			usage["total_tokens"], _ = workflowEventInt(eventData["total_tokens"])
		}
	}
	if run.RuntimeProtocolVersion >= workflowRuntimeProtocolVersionV2 {
		status, _ := eventData["status"].(string)
		totalTokens, _ := workflowEventInt(eventData["total_tokens"])
		totalSteps, _ := workflowEventInt(eventData["total_steps"])
		exceptionsCount, _ := workflowEventInt(eventData["exceptions_count"])
		finalOutputs, _ := eventData["outputs"].(map[string]interface{})
		if finalOutputs == nil {
			finalOutputs = map[string]interface{}{}
		}
		if err := finalizeWorkflowRun(ctx, finalizeWorkflowRunParams{
			WorkflowRunID:    run.ID,
			Status:           status,
			Outputs:          finalOutputs,
			ErrorMessage:     workflowRunEventErrorMessage(eventData["error"]),
			ElapsedTime:      elapsed,
			TotalTokens:      int64(totalTokens),
			TotalSteps:       totalSteps,
			ExceptionsCount:  exceptionsCount,
			FinalAnswer:      finalAnswer,
			MessageStatus:    messageStatus,
			MessageEnd:       messageEnd,
			WorkflowFinished: eventData,
		}); err != nil {
			return fmt.Errorf("finalize resumed workflow: %w", err)
		}
		if len(messageEnd) > 0 {
			if err := eventDispatcher.Dispatch(ctx, workflowEventMessageEnd, messageEnd); err != nil {
				return err
			}
		}
	}
	return eventDispatcher.Dispatch(ctx, workflowpause.EventWorkflowFinished, eventData)
}

func (h *WorkflowHandler) persistApprovalResumeError(ctx context.Context, pauseService *workflowpause.Service, workflowService *WorkflowService, run *WorkflowRunLog, err error, resumeStartedAt time.Time, eventDispatcher *workflowRunEventDispatcher, finalAnswer string) {
	if pauseService == nil || run == nil || err == nil {
		return
	}
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	errorPayload := buildWorkflowStreamErrorPayload(err)
	errorMessage := workflowStreamErrorMessage(errorPayload)
	elapsed := ElapsedMillisecondsSince(resumeStartedAt)
	totalSteps := 0
	var totalTokens int64
	if workflowService != nil {
		elapsed = workflowService.workflowRunElapsedMillisecondsForEvent(persistCtx, run.ID, elapsed)
		totalSteps = workflowService.workflowRunNodeStepCount(persistCtx, run.ID)
		totalTokens = workflowService.workflowRunNodeTotalTokens(persistCtx, run.ID)
	}
	workflowFinished := map[string]interface{}{
		"id":               run.ID,
		"workflow_id":      run.WorkflowID,
		"sequence_number":  run.SequenceNumber,
		"status":           "failed",
		"outputs":          map[string]interface{}{},
		"error":            errorPayload,
		"elapsed_time":     elapsed,
		"total_tokens":     totalTokens,
		"total_steps":      totalSteps,
		"created_by":       map[string]interface{}{"id": run.CreatedBy, "name": "", "email": ""},
		"created_at":       time.Now().Unix(),
		"finished_at":      time.Now().Unix(),
		"exceptions_count": 1,
		"files":            []interface{}{},
	}
	errorEvent := map[string]interface{}{"message": errorMessage, "error": errorPayload, "workflow_run_id": run.ID}
	if run.RuntimeProtocolVersion >= workflowRuntimeProtocolVersionV2 {
		messageStatus := ""
		var messageCount int64
		if countErr := database.GetDB().WithContext(persistCtx).Model(&conversation.AgentMessage{}).
			Where("workflow_run_id = ? AND deleted_at IS NULL", run.ID).Count(&messageCount).Error; countErr == nil && messageCount > 0 {
			messageStatus = conversation.AgentMessageStatusError
		}
		if finalizeErr := finalizeWorkflowRun(persistCtx, finalizeWorkflowRunParams{
			WorkflowRunID: run.ID, Status: "failed", Outputs: map[string]interface{}{}, ErrorMessage: errorMessage,
			ElapsedTime: elapsed, TotalTokens: totalTokens, TotalSteps: totalSteps, ExceptionsCount: 1, FinalAnswer: finalAnswer,
			MessageStatus: messageStatus, ErrorEvent: errorEvent, WorkflowFinished: workflowFinished,
		}); finalizeErr != nil {
			logger.ErrorContext(persistCtx, "failed to durably finalize resumed workflow error", "workflow_run_id", run.ID, finalizeErr)
			return
		}
		eventDispatcher.Dispatch(persistCtx, workflowpause.EventError, errorEvent)
		eventDispatcher.Dispatch(persistCtx, workflowpause.EventWorkflowFinished, workflowFinished)
		return
	}
	h.updateApprovalConversationMessageStatus(persistCtx, run.ID, conversation.AgentMessageStatusError, &errorMessage)
	if workflowService != nil {
		_ = workflowService.UpdateWorkflowRunLogStatus(persistCtx, run.ID, "failed", map[string]interface{}{}, elapsed, totalTokens, totalSteps, errorMessage)
	}
	eventDispatcher.Dispatch(persistCtx, workflowpause.EventError, errorEvent)
	eventDispatcher.Dispatch(persistCtx, workflowpause.EventWorkflowFinished, workflowFinished)
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
		if persistedTokens := workflowService.workflowRunNodeTotalTokens(context.Background(), run.ID); persistedTokens > 0 {
			totalTokens = int(persistedTokens)
		}
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
