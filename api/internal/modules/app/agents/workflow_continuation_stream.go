package agents

import (
	"context"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	"github.com/zgiai/zgi/api/pkg/logger"
	"strings"
	"time"
)

func (h *AgentsHandler) streamWorkflowApprovalContinuationDirect(c *gin.Context, scope runtimeservice.Scope, continuation *runtimeservice.WorkflowApprovalContinuation, req agentWorkflowContinuationRequest, resumeInputs map[string]interface{}, approvalContinuation bool, questionContinuation bool) bool {
	if !approvalContinuation && !questionContinuation {
		return false
	}
	streamRunner, ok := h.workflowContinuationRunner.(workflowContinuationStreamRunner)
	if !ok {
		return false
	}
	observeAfter := 0
	if run, loadErr := h.loadAgentWorkflowRunLog(c.Request.Context(), continuation.WorkflowRunID); loadErr == nil {
		observeAfter = h.latestAgentWorkflowContinuationSequence(c.Request.Context(), run.TenantID, continuation.WorkflowRunID)
	}
	workCtx, cancelWork := context.WithTimeout(context.WithoutCancel(c.Request.Context()), agentWorkflowContinuationMaxDuration)
	defer cancelWork()
	state := &agentWorkflowContinuationStreamState{}
	emit := func(event runtimeservice.StreamEvent) error {
		return writeAgentSSEEvent(c, event.ID, event.EventType, event.Payload)
	}
	onWorkflowEvent := func(eventType string, payload map[string]interface{}) error {
		result := h.handleAgentWorkflowContinuationEvent(workCtx, continuation, eventType, payload, emit)
		state.apply(result)
		return nil
	}
	var err error
	if approvalContinuation {
		err = h.resumeAgentWorkflowApprovalStream(workCtx, scope, continuation, req, streamRunner, onWorkflowEvent)
	} else {
		err = streamRunner.ResumeQuestionAnswerWorkflowStream(workCtx, continuation.WorkflowRunID, resumeInputs, onWorkflowEvent)
	}
	if err != nil {
		if errors.Is(err, workflowpause.ErrResumeAlreadyRunning) {
			h.streamWorkflowApprovalContinuation(c, scope, continuation, observeAfter, nil)
			return true
		}
		// The stop endpoint commits the workflow terminal state before it
		// cancels the resume worker. Prefer that durable state over the stale
		// worker's context-canceled/ownership-lost error, otherwise the stopped
		// message is incorrectly finalized again as failed and the optimistic
		// update surfaces as "record not found".
		terminalCtx, cancelTerminal := context.WithTimeout(context.WithoutCancel(workCtx), 5*time.Second)
		if h.finishAgentWorkflowContinuationIfRunTerminal(terminalCtx, scope, continuation, state.WorkflowMessageText, state.HasWorkflowMessage, emit) {
			cancelTerminal()
			return true
		}
		cancelTerminal()
		h.failAgentWorkflowContinuation(context.WithoutCancel(c.Request.Context()), continuation, err, emit)
		return true
	}
	if state.WaitingStatus != "" {
		h.pauseAgentWorkflowContinuation(workCtx, continuation, state.WaitingStatus, emit)
		return true
	}
	if state.Terminal {
		h.finishAgentWorkflowContinuation(workCtx, scope, continuation, state.WorkflowMessageText, state.HasWorkflowMessage, emit)
		return true
	}
	if h.finishAgentWorkflowContinuationIfRunTerminal(workCtx, scope, continuation, state.WorkflowMessageText, state.HasWorkflowMessage, emit) {
		return true
	}
	h.failAgentWorkflowContinuation(context.WithoutCancel(c.Request.Context()), continuation, errors.New("workflow continuation ended without terminal event"), emit)
	return true
}

func normalizeAgentWorkflowQuestionInputs(inputs map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	query := strings.TrimSpace(stringFromAgentWorkflowContinuation(inputs["query"]))
	if query == "" {
		query = strings.TrimSpace(stringFromAgentWorkflowContinuation(inputs["sys.query"]))
	}
	if query != "" {
		out["query"] = query
		out["sys.query"] = query
	}
	optionID := strings.TrimSpace(stringFromAgentWorkflowContinuation(inputs["question_answer_option_id"]))
	if optionID == "" {
		optionID = strings.TrimSpace(stringFromAgentWorkflowContinuation(inputs["option_id"]))
	}
	if optionID != "" {
		out["question_answer_option_id"] = optionID
	}
	nodeID := strings.TrimSpace(stringFromAgentWorkflowContinuation(inputs["question_answer_node_id"]))
	if nodeID == "" {
		nodeID = strings.TrimSpace(stringFromAgentWorkflowContinuation(inputs["node_id"]))
	}
	if nodeID != "" {
		out["question_answer_node_id"] = nodeID
	}
	if round, ok := inputs["question_answer_round"]; ok && round != nil {
		out["question_answer_round"] = round
	} else if round, ok := inputs["round"]; ok && round != nil {
		out["question_answer_round"] = round
	}
	return out
}

func (h *AgentsHandler) streamWorkflowApprovalContinuation(c *gin.Context, scope runtimeservice.Scope, continuation *runtimeservice.WorkflowApprovalContinuation, afterSequence int, resumeErrCh <-chan error) {
	emit := func(event runtimeservice.StreamEvent) error {
		return writeAgentSSEEvent(c, event.ID, event.EventType, event.Payload)
	}
	if h.db == nil {
		h.failAgentWorkflowContinuation(context.WithoutCancel(c.Request.Context()), continuation, errors.New("database is not available"), emit)
		return
	}
	workBaseCtx := context.WithoutCancel(c.Request.Context())
	workCtx, cancelWork := context.WithTimeout(workBaseCtx, agentWorkflowContinuationMaxDuration)
	defer cancelWork()
	run, err := h.loadAgentWorkflowRunLog(workCtx, continuation.WorkflowRunID)
	if err != nil {
		h.failAgentWorkflowContinuation(context.WithoutCancel(c.Request.Context()), continuation, err, emit)
		return
	}
	pauseService := workflowpause.NewService(h.db)
	lastSequence := afterSequence
	passthroughAnswer := strings.Builder{}
	hasPassthroughAnswer := false
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		drained := h.drainAgentWorkflowContinuationEvents(workCtx, continuation, pauseService, run.TenantID, lastSequence, emit)
		lastSequence = drained.NextSequence
		if drained.HasWorkflowMessage {
			hasPassthroughAnswer = true
			applyAgentWorkflowMessage(&passthroughAnswer, drained)
		}
		if drained.Terminal {
			h.finishAgentWorkflowContinuation(workCtx, scope, continuation, passthroughAnswer.String(), hasPassthroughAnswer, emit)
			return
		}
		if drained.WaitingStatus != "" {
			h.pauseAgentWorkflowContinuation(workCtx, continuation, drained.WaitingStatus, emit)
			return
		}
		if h.finishAgentWorkflowContinuationIfRunTerminal(workCtx, scope, continuation, passthroughAnswer.String(), hasPassthroughAnswer, emit) {
			return
		}
		select {
		case <-c.Request.Context().Done():
			h.startWorkflowApprovalContinuationBackground(workBaseCtx, scope, continuation, run.TenantID, lastSequence, passthroughAnswer.String(), hasPassthroughAnswer, resumeErrCh)
			return
		case resumeErr := <-resumeErrCh:
			drained := h.drainAgentWorkflowContinuationEvents(workCtx, continuation, pauseService, run.TenantID, lastSequence, emit)
			lastSequence = drained.NextSequence
			if drained.HasWorkflowMessage {
				hasPassthroughAnswer = true
				applyAgentWorkflowMessage(&passthroughAnswer, drained)
			}
			if drained.Terminal {
				h.finishAgentWorkflowContinuation(workCtx, scope, continuation, passthroughAnswer.String(), hasPassthroughAnswer, emit)
				return
			}
			h.failAgentWorkflowContinuation(context.WithoutCancel(c.Request.Context()), continuation, resumeErr, emit)
			return
		case <-workCtx.Done():
			h.failAgentWorkflowContinuation(context.WithoutCancel(c.Request.Context()), continuation, workCtx.Err(), emit)
			return
		case <-ticker.C:
		}
	}
}

func (h *AgentsHandler) latestAgentWorkflowContinuationSequence(ctx context.Context, tenantID string, workflowRunID string) int {
	if h.db == nil || strings.TrimSpace(tenantID) == "" || strings.TrimSpace(workflowRunID) == "" {
		return 0
	}
	pauseService := workflowpause.NewService(h.db)
	payload, err := pauseService.ListEvents(ctx, tenantID, workflowRunID, 0, 1000)
	if err != nil {
		logger.WarnContext(ctx, "failed to list current workflow continuation events", "workflow_run_id", workflowRunID, err)
		return 0
	}
	latest := 0
	for _, event := range payload.Events {
		if event.Sequence > latest {
			latest = event.Sequence
		}
	}
	return latest
}

func (h *AgentsHandler) startWorkflowApprovalContinuationBackground(ctx context.Context, scope runtimeservice.Scope, continuation *runtimeservice.WorkflowApprovalContinuation, tenantID string, afterSequence int, initialPassthroughAnswer string, initialHasPassthroughAnswer bool, resumeErrCh <-chan error) {
	go func() {
		ctx, cancel := context.WithTimeout(ctx, agentWorkflowContinuationMaxDuration)
		defer cancel()
		pauseService := workflowpause.NewService(h.db)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		lastSequence := afterSequence
		passthroughAnswer := strings.Builder{}
		passthroughAnswer.WriteString(initialPassthroughAnswer)
		hasPassthroughAnswer := initialHasPassthroughAnswer
		for {
			drained := h.drainAgentWorkflowContinuationEvents(ctx, continuation, pauseService, tenantID, lastSequence, nil)
			lastSequence = drained.NextSequence
			if drained.HasWorkflowMessage {
				hasPassthroughAnswer = true
				applyAgentWorkflowMessage(&passthroughAnswer, drained)
			}
			if drained.Terminal {
				h.finishAgentWorkflowContinuation(ctx, scope, continuation, passthroughAnswer.String(), hasPassthroughAnswer, nil)
				return
			}
			if drained.WaitingStatus != "" {
				h.pauseAgentWorkflowContinuation(ctx, continuation, drained.WaitingStatus, nil)
				return
			}
			if h.finishAgentWorkflowContinuationIfRunTerminal(ctx, scope, continuation, passthroughAnswer.String(), hasPassthroughAnswer, nil) {
				return
			}
			select {
			case resumeErr := <-resumeErrCh:
				drained := h.drainAgentWorkflowContinuationEvents(ctx, continuation, pauseService, tenantID, lastSequence, nil)
				lastSequence = drained.NextSequence
				if drained.HasWorkflowMessage {
					hasPassthroughAnswer = true
					applyAgentWorkflowMessage(&passthroughAnswer, drained)
				}
				if drained.Terminal {
					h.finishAgentWorkflowContinuation(ctx, scope, continuation, passthroughAnswer.String(), hasPassthroughAnswer, nil)
					return
				}
				h.failAgentWorkflowContinuation(context.WithoutCancel(ctx), continuation, resumeErr, nil)
				return
			case <-ctx.Done():
				h.failAgentWorkflowContinuation(context.WithoutCancel(ctx), continuation, ctx.Err(), nil)
				return
			case <-ticker.C:
			}
		}
	}()
}

func applyAgentWorkflowMessage(target *strings.Builder, result agentWorkflowContinuationDrainResult) {
	if target == nil || !result.HasWorkflowMessage {
		return
	}
	if result.WorkflowMessageReplace {
		target.Reset()
	}
	target.WriteString(result.WorkflowMessageText)
}

type agentWorkflowContinuationDrainResult struct {
	Terminal               bool
	WaitingStatus          string
	NextSequence           int
	WorkflowMessageText    string
	HasWorkflowMessage     bool
	WorkflowMessageReplace bool
}

type agentWorkflowContinuationStreamState struct {
	Terminal               bool
	WaitingStatus          string
	WorkflowMessageText    string
	HasWorkflowMessage     bool
	WorkflowMessageReplace bool
}

func (s *agentWorkflowContinuationStreamState) apply(result agentWorkflowContinuationDrainResult) {
	if result.Terminal {
		s.Terminal = true
	}
	if result.WaitingStatus != "" {
		s.WaitingStatus = result.WaitingStatus
	}
	if result.HasWorkflowMessage {
		s.HasWorkflowMessage = true
		if result.WorkflowMessageReplace {
			s.WorkflowMessageText = result.WorkflowMessageText
		} else {
			s.WorkflowMessageText += result.WorkflowMessageText
		}
		s.WorkflowMessageReplace = result.WorkflowMessageReplace
	}
}

func (h *AgentsHandler) drainAgentWorkflowContinuationEvents(ctx context.Context, continuation *runtimeservice.WorkflowApprovalContinuation, pauseService *workflowpause.Service, tenantID string, afterSequence int, emit func(runtimeservice.StreamEvent) error) agentWorkflowContinuationDrainResult {
	result := agentWorkflowContinuationDrainResult{NextSequence: afterSequence}
	payload, err := pauseService.ListEvents(ctx, tenantID, continuation.WorkflowRunID, afterSequence, 100)
	if err != nil {
		logger.WarnContext(ctx, "failed to list workflow continuation events", "workflow_run_id", continuation.WorkflowRunID, err)
		return result
	}
	messageText := strings.Builder{}
	for _, event := range payload.Events {
		result.NextSequence = event.Sequence
		eventData := copyMapForAgentWorkflowContinuation(event.Data)
		eventData["event_id"] = event.EventID
		eventData["sequence"] = event.Sequence
		eventData["schema_version"] = event.SchemaVersion
		eventData["payload_version"] = event.PayloadVersion
		eventData["execution_id"] = event.ExecutionID
		eventData["pause_id"] = event.PauseID
		eventData["pause_generation"] = event.PauseGeneration
		eventData["occurred_at_ms"] = event.OccurredAtMS
		eventData["recorded_at_ms"] = event.RecordedAtMS
		eventResult := h.handleAgentWorkflowContinuationEvent(ctx, continuation, event.Event, eventData, emit)
		if eventResult.HasWorkflowMessage {
			result.HasWorkflowMessage = true
			if eventResult.WorkflowMessageReplace {
				messageText.Reset()
				result.WorkflowMessageReplace = true
			}
			messageText.WriteString(eventResult.WorkflowMessageText)
		}
		if eventResult.Terminal {
			result.Terminal = true
		}
		if eventResult.WaitingStatus != "" {
			result.WaitingStatus = eventResult.WaitingStatus
		}
	}
	result.WorkflowMessageText = messageText.String()
	return result
}

func (h *AgentsHandler) handleAgentWorkflowContinuationEvent(ctx context.Context, continuation *runtimeservice.WorkflowApprovalContinuation, rawEventType string, rawData map[string]interface{}, emit func(runtimeservice.StreamEvent) error) agentWorkflowContinuationDrainResult {
	result := agentWorkflowContinuationDrainResult{}
	eventType := agentWorkflowContinuationEventType(rawEventType)
	if eventType == "" {
		return result
	}
	if agentWorkflowContinuationMode(continuation) == "agent_task_tool" && agentWorkflowAnswerTransportEvent(eventType) {
		return result
	}
	data := copyMapForAgentWorkflowContinuation(rawData)
	data["workflow_run_id"] = continuation.WorkflowRunID
	data["conversation_id"] = continuation.ConversationID.String()
	data["message_id"] = continuation.MessageID.String()
	streamEvent, persistErr := h.chatRuntimeService.RecordWorkflowApprovalContinuationEvent(ctx, continuation, eventType, data)
	if persistErr != nil {
		logger.WarnContext(ctx, "failed to persist workflow continuation event", "workflow_run_id", continuation.WorkflowRunID, "event_type", eventType, persistErr)
		streamEvent, persistErr = h.chatRuntimeService.AppendWorkflowApprovalContinuationStreamEvent(ctx, continuation, eventType, data)
		if persistErr != nil {
			logger.WarnContext(ctx, "failed to append fallback workflow continuation stream event", "workflow_run_id", continuation.WorkflowRunID, "event_type", eventType, persistErr)
		}
	}
	if streamEvent != nil {
		data = streamEvent.Payload
	}
	var userInput gin.H
	if eventType == workflowpause.EventQuestionAnswerRequested {
		userInput = agentWorkflowQuestionUserInputEvent(continuation, data)
		if len(userInput) > 0 {
			metadata := copyMapForAgentWorkflowContinuation(continuation.Metadata)
			metadata["user_input_request"] = map[string]interface{}(userInput)
			continuation.Metadata = metadata
		}
	}
	if emit != nil {
		emitAgentWorkflowContinuationEvent(emit, streamEvent)
		if len(userInput) > 0 {
			h.emitAgentWorkflowContinuationStreamEvent(ctx, continuation, "user_input_requested", userInput, emit)
		}
	}
	if isAgentWorkflowPassthroughMessageEvent(eventType, continuation) {
		chunk := agentWorkflowContinuationMessageChunk(data)
		if chunk != "" {
			result.HasWorkflowMessage = true
			result.WorkflowMessageText = chunk
			result.WorkflowMessageReplace = eventType == "text_replace"
			if eventType != "message" && emit != nil {
				h.emitAgentWorkflowContinuationStreamEvent(ctx, continuation, "message", gin.H{
					"conversation_id": continuation.ConversationID.String(),
					"message_id":      continuation.MessageID.String(),
					"answer":          chunk,
				}, emit)
			}
		}
	}
	if eventType == "workflow_finished" || eventType == "workflow_failed" {
		result.Terminal = true
	}
	if eventType == "approval_requested" {
		result.WaitingStatus = runtimemodel.MessageStatusWaitingApproval
	}
	if eventType == workflowpause.EventQuestionAnswerRequested {
		result.WaitingStatus = runtimemodel.MessageStatusWaitingQuestion
	}
	return result
}

func (h *AgentsHandler) finishAgentWorkflowContinuationIfRunTerminal(ctx context.Context, scope runtimeservice.Scope, continuation *runtimeservice.WorkflowApprovalContinuation, passthroughAnswer string, hasPassthroughAnswer bool, emit func(runtimeservice.StreamEvent) error) bool {
	run, err := h.loadAgentWorkflowRunLog(ctx, continuation.WorkflowRunID)
	if err != nil {
		logger.WarnContext(ctx, "failed to load workflow continuation run status", "workflow_run_id", continuation.WorkflowRunID, err)
		return false
	}
	if !agentWorkflowRunLogTerminal(run.Status) {
		return false
	}
	h.finishAgentWorkflowContinuation(ctx, scope, continuation, passthroughAnswer, hasPassthroughAnswer, emit)
	return true
}

func (h *AgentsHandler) finishAgentWorkflowContinuation(ctx context.Context, scope runtimeservice.Scope, continuation *runtimeservice.WorkflowApprovalContinuation, passthroughAnswer string, hasPassthroughAnswer bool, emit func(runtimeservice.StreamEvent) error) {
	run, err := h.loadAgentWorkflowRunLog(ctx, continuation.WorkflowRunID)
	if err != nil {
		h.failAgentWorkflowContinuation(ctx, continuation, err, emit)
		return
	}
	outputs := run.OutputsMap()
	finalPassthroughAnswer, hasFinalPassthroughAnswer := agentWorkflowContinuationFinalPassthroughAnswer(outputs, passthroughAnswer, hasPassthroughAnswer)
	if strings.EqualFold(strings.TrimSpace(run.Status), "stopped") {
		answer := ""
		if hasFinalPassthroughAnswer {
			answer = finalPassthroughAnswer
		}
		metadata, stopErr := h.chatRuntimeService.CompleteWorkflowApprovalContinuation(ctx, continuation, answer, runtimemodel.MessageStatusStopped)
		if stopErr != nil {
			logger.WarnContext(ctx, "failed to stop workflow continuation message", "workflow_run_id", continuation.WorkflowRunID, stopErr)
			return
		}
		h.emitAgentWorkflowContinuationStreamEvent(ctx, continuation, "message_end", gin.H{
			"conversation_id": continuation.ConversationID.String(),
			"message_id":      continuation.MessageID.String(),
			"status":          runtimemodel.MessageStatusStopped,
			"metadata":        metadata,
		}, emit)
		return
	}
	if hasFinalPassthroughAnswer && agentWorkflowContinuationMode(continuation) == "agent_conversation_delegate" {
		if hasPassthroughAnswer && finalPassthroughAnswer != passthroughAnswer {
			h.emitAgentWorkflowContinuationStreamEvent(ctx, continuation, "message_retract", gin.H{
				"conversation_id": continuation.ConversationID.String(),
				"message_id":      continuation.MessageID.String(),
				"content":         passthroughAnswer,
			}, emit)
		}
		if !hasPassthroughAnswer || finalPassthroughAnswer != passthroughAnswer {
			h.emitAgentWorkflowContinuationStreamEvent(ctx, continuation, "message", gin.H{
				"conversation_id": continuation.ConversationID.String(),
				"message_id":      continuation.MessageID.String(),
				"answer":          finalPassthroughAnswer,
			}, emit)
		}
		metadata, err := h.chatRuntimeService.CompleteWorkflowApprovalContinuation(ctx, continuation, finalPassthroughAnswer, completionContinuationStatus(run.Status))
		if err != nil {
			h.failAgentWorkflowContinuation(ctx, continuation, err, emit)
			return
		}
		h.emitAgentWorkflowContinuationStreamEvent(ctx, continuation, "message_end", gin.H{
			"conversation_id": continuation.ConversationID.String(),
			"message_id":      continuation.MessageID.String(),
			"status":          runtimemodel.MessageStatusCompleted,
			"metadata":        metadata,
		}, emit)
		return
	}
	if shouldSummarizeAgentWorkflowContinuation(continuation, run.Status, outputs) {
		errorMessage := ""
		if run.Error != nil {
			errorMessage = *run.Error
		}
		result, summaryErr := h.chatRuntimeService.SummarizeWorkflowApprovalContinuation(ctx, scope, continuation, runtimeservice.WorkflowContinuationSummaryRequest{
			WorkflowRunID: continuation.WorkflowRunID,
			Status:        run.Status,
			Outputs:       outputs,
			Error:         errorMessage,
		}, func(event runtimeservice.StreamEvent) error {
			emitAgentWorkflowContinuationEvent(emit, &event)
			return nil
		})
		if summaryErr != nil {
			if runtimeservice.IsFinalizedStreamError(summaryErr) {
				return
			}
			h.failAgentWorkflowContinuation(ctx, continuation, summaryErr, emit)
			return
		}
		if result != nil && result.Status != "" && result.Status != runtimemodel.MessageStatusCompleted {
			return
		}
		metadata := map[string]interface{}{}
		if result != nil {
			metadata = result.Metadata
		}
		h.emitAgentWorkflowContinuationStreamEvent(ctx, continuation, "message_end", gin.H{
			"conversation_id": continuation.ConversationID.String(),
			"message_id":      continuation.MessageID.String(),
			"status":          runtimemodel.MessageStatusCompleted,
			"metadata":        metadata,
		}, emit)
		return
	}
	status := "direct_output"
	if strings.EqualFold(strings.TrimSpace(run.Status), "failed") {
		status = "failed"
	}
	if _, err := h.chatRuntimeService.UpdateWorkflowApprovalContinuationStatus(ctx, continuation, status); err != nil {
		h.failAgentWorkflowContinuation(ctx, continuation, err, emit)
		return
	}
	answer := agentWorkflowContinuationAnswer(continuation.AgentType, continuation.WorkflowRunID, run.Status, outputs, run.Error)
	if strings.TrimSpace(answer) != "" {
		h.emitAgentWorkflowContinuationStreamEvent(ctx, continuation, "message", gin.H{
			"conversation_id": continuation.ConversationID.String(),
			"message_id":      continuation.MessageID.String(),
			"answer":          answer,
		}, emit)
	}
	metadata, err := h.chatRuntimeService.CompleteWorkflowApprovalContinuation(ctx, continuation, answer, completionContinuationStatus(run.Status))
	if err != nil {
		h.failAgentWorkflowContinuation(ctx, continuation, err, emit)
		return
	}
	h.emitAgentWorkflowContinuationStreamEvent(ctx, continuation, "message_end", gin.H{
		"conversation_id": continuation.ConversationID.String(),
		"message_id":      continuation.MessageID.String(),
		"status":          runtimemodel.MessageStatusCompleted,
		"metadata":        metadata,
	}, emit)
}

func (h *AgentsHandler) pauseAgentWorkflowContinuation(ctx context.Context, continuation *runtimeservice.WorkflowApprovalContinuation, status string, emit func(runtimeservice.StreamEvent) error) {
	metadata, err := h.chatRuntimeService.PauseWorkflowApprovalContinuation(ctx, continuation, status)
	if err != nil {
		h.failAgentWorkflowContinuation(ctx, continuation, err, emit)
		return
	}
	h.emitAgentWorkflowContinuationStreamEvent(ctx, continuation, "message_end", gin.H{
		"conversation_id": continuation.ConversationID.String(),
		"message_id":      continuation.MessageID.String(),
		"status":          status,
		"metadata":        metadata,
	}, emit)
}

func (h *AgentsHandler) failAgentWorkflowContinuation(ctx context.Context, continuation *runtimeservice.WorkflowApprovalContinuation, cause error, emit func(runtimeservice.StreamEvent) error) {
	message := "workflow continuation timed out before completion"
	if cause != nil && !errors.Is(cause, context.DeadlineExceeded) {
		message = fmt.Sprintf("workflow continuation stopped before completion: %v", cause)
	}
	metadata, err := h.chatRuntimeService.FailWorkflowApprovalContinuation(ctx, continuation, message)
	if err != nil {
		emitAgentWorkflowContinuationError(emit, err)
		return
	}
	h.emitAgentWorkflowContinuationStreamEvent(ctx, continuation, "error", gin.H{
		"conversation_id": continuation.ConversationID.String(),
		"message_id":      continuation.MessageID.String(),
		"message":         message,
	}, emit)
	h.emitAgentWorkflowContinuationStreamEvent(ctx, continuation, "message_end", gin.H{
		"conversation_id": continuation.ConversationID.String(),
		"message_id":      continuation.MessageID.String(),
		"status":          runtimemodel.MessageStatusError,
		"metadata":        metadata,
	}, emit)
}

func (h *AgentsHandler) emitAgentWorkflowContinuationStreamEvent(ctx context.Context, continuation *runtimeservice.WorkflowApprovalContinuation, eventType string, payload gin.H, emit func(runtimeservice.StreamEvent) error) *runtimeservice.StreamEvent {
	event, err := h.chatRuntimeService.AppendWorkflowApprovalContinuationStreamEvent(ctx, continuation, eventType, payload)
	if err != nil {
		logger.WarnContext(ctx, "failed to append workflow continuation stream event", "workflow_run_id", continuation.WorkflowRunID, "event_type", eventType, err)
		event = &runtimeservice.StreamEvent{
			EventType: eventType,
			Payload:   payload,
			CreatedAt: time.Now().Unix(),
		}
	}
	emitAgentWorkflowContinuationEvent(emit, event)
	return event
}

func emitAgentWorkflowContinuationEvent(emit func(runtimeservice.StreamEvent) error, event *runtimeservice.StreamEvent) {
	if emit == nil || event == nil {
		return
	}
	_ = emit(*event)
}

func emitAgentWorkflowContinuationError(emit func(runtimeservice.StreamEvent) error, err error) {
	if err == nil {
		return
	}
	emitAgentWorkflowContinuationEvent(emit, &runtimeservice.StreamEvent{
		EventType: "error",
		Payload:   gin.H{"message": err.Error()},
		CreatedAt: time.Now().Unix(),
	})
}
