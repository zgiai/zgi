package workflow

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/agents"
	"github.com/zgiai/zgi/api/internal/modules/app/conversation"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	workflowshared "github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	"github.com/zgiai/zgi/api/pkg/database"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
)

const (
	workflowStreamEndedWithoutFinalResultMessage = "workflow stream ended without final result"
	workflowStreamClientDisconnectedMessage      = "client disconnected"
	workflowStreamDisconnectPersistTimeout       = 5 * time.Second
)

// runWorkflowStream handles streaming workflow execution.
// requestedWorkspaceID is the caller/request scope. Workspace-owned agents still resolve to the app-owned workspace.
func (h *WorkflowHandler) runWorkflowStream(c *gin.Context, requestedWorkspaceID, appID string, req *dto.DraftWorkflowRunRequest, accountID string, isDraft bool, runType string, triggeredFrom string) {
	// Record workflow start time for elapsed time calculation
	workflowStartTime := time.Now()

	prepareWorkflowStreamSSE(c)

	var (
		agent              *agents.Agent
		appWorkspaceID     string
		billingSubjectType string
		err                error
	)

	if ws, ok := h.workflowService.(*WorkflowService); ok && ws.agentsRepo != nil {
		agent, err = ws.agentsRepo.GetByID(c.Request.Context(), appID)
		if err != nil {
			logger.CriticalContext(c.Request.Context(), "failed to resolve agent for workflow stream", "agent_id", appID, err)
			h.sendSSEError(c.Request.Context(), c.Writer, fmt.Sprintf("failed to resolve agent: %v", err))
			return
		}
		billingSubjectType = resolveWorkflowBillingSubjectType(agent)
	}

	if !isOrganizationScopedWorkflowAgent(agent) {
		// Workspace-owned workflows continue to use the app-owned workspace as execution subject.
		appWorkspaceID, err = h.workflowService.GetAgentWorkspaceID(c.Request.Context(), appID)
		if err != nil {
			logger.CriticalContext(c.Request.Context(), "failed to resolve app workspace id for workflow stream", "agent_id", appID, err)
			h.sendSSEError(c.Request.Context(), c.Writer, fmt.Sprintf("failed to resolve app workspace_id: %v", err))
			return
		}
	}

	workspaceID := resolveRunStreamWorkspaceID(agent, requestedWorkspaceID, appWorkspaceID)
	if workspaceID == "" {
		logger.CriticalContext(c.Request.Context(), "failed to resolve app workspace id for workflow stream", "agent_id", appID, fmt.Errorf("empty workspace_id"))
		h.sendSSEError(c.Request.Context(), c.Writer, "failed to resolve app workspace_id: empty workspace_id")
		return
	}
	if !isOrganizationScopedWorkflowAgent(agent) && requestedWorkspaceID != "" && requestedWorkspaceID != workspaceID {
		logger.WarnContext(c.Request.Context(), "ignored requested workspace mismatch",
			zap.String("requested_workspace_id", requestedWorkspaceID),
			zap.String("app_workspace_id", workspaceID),
			zap.String("app_id", appID),
		)
	}

	// Generate task ID (will be replaced with real workflow run ID from database)
	taskID := fmt.Sprintf("task-%s-%d", appID, time.Now().UnixNano())

	// Get the actual workflow ID from database (not agent_id)
	workflowID := appID // fallback to appID if query fails
	if ws, ok := h.workflowService.(*WorkflowService); ok {
		if ws.repo != nil {
			workflow, err := ws.repo.GetDraftWorkflow(c.Request.Context(), appID)
			if err == nil && workflow != nil {
				workflowID = workflow.ID
				logger.DebugContext(c.Request.Context(), "workflow stream draft workflow id loaded",
					zap.String("workflow_id", workflowID),
					zap.String("agent_id", appID),
				)
			} else {
				logger.WarnContext(c.Request.Context(), "failed to load workflow id, using agent id fallback",
					zap.String("agent_id", appID),
					zap.Error(err),
				)
			}
		}
	}

	sequenceNumber := 1

	// isDraft := (runType == "debugging")

	// Note: don't fetch workflow here; let executor goroutine handle fetching and errors via channels to guarantee streaming path can emit workflow_finished

	// Add req.Files to inputs for processing if provided
	if req.Files != nil && len(req.Files) > 0 {
		if req.Inputs == nil {
			req.Inputs = make(map[string]interface{})
		}
		// Convert []FileInfo to []interface{} for processing
		filesInterface := make([]interface{}, len(req.Files))
		for i, f := range req.Files {
			filesInterface[i] = map[string]interface{}{
				"type":            f.Type,
				"transfer_method": f.TransferMethod,
				"url":             f.URL,
				"upload_file_id":  f.UploadFileID,
			}
		}
		req.Inputs["#files#"] = filesInterface
	}

	// Process all file inputs before workflow execution
	processedInputs := h.processAllFileInputs(c.Request.Context(), req.Inputs, workspaceID, appID)
	applyProcessedInputs(req, processedInputs)

	// Process files if provided
	var processedFiles interface{}
	if filesInput, exists := processedInputs["#files#"]; exists && filesInput != nil {
		processedFiles = filesInput
	}

	systemInputs, ok := h.prepareWorkflowStreamSystemInputs(c.Request.Context(), c.Writer, workflowStreamSystemInputParams{
		WorkspaceID:        workspaceID,
		AppID:              appID,
		AccountID:          accountID,
		WorkflowID:         workflowID,
		BillingSubjectType: billingSubjectType,
		ProcessedFiles:     processedFiles,
		Inputs:             req.Inputs,
	})
	if !ok {
		return
	}
	// The conversation may be created while preparing system inputs. Persist it
	// into the durable run inputs so the conversation-level claim and every
	// continuation use the same server-authoritative identity.
	if conversationID, _ := systemInputs["sys.conversation_id"].(string); strings.TrimSpace(conversationID) != "" {
		if req.Inputs == nil {
			req.Inputs = make(map[string]interface{})
		}
		req.Inputs["sys.conversation_id"] = conversationID
	}

	var resumePauseID string
	var resumePauseGeneration int64
	var questionResumeState *workflowpause.State
	if runType == "CONVERSATION_WORKFLOW" {
		resumeState, pauseID, pauseGeneration, ok := h.workflowQuestionAnswerResumeState(c.Request.Context(), workspaceID, appID, systemInputs)
		if ok {
			questionResumeState = resumeState
			systemInputs[workflowResumeStateInputKey] = resumeState
			systemInputs[workflowResumePauseIDInputKey] = pauseID
			resumePauseID = pauseID
			resumePauseGeneration = pauseGeneration
		}
	}

	// triggeredFrom is now passed as a parameter, no need to compute it here

	// Create workflow run log for streaming execution
	var workflowRunLogID string
	var executionOwner workflowExecutionOwner
	var questionSubmittedEvent *workflowpause.RunEventPayload
	defer func() {
		if ws, ok := h.workflowService.(*WorkflowService); ok {
			ws.cleanupWorkflowReusableSessionsWithTimeout(workflowRunLogID)
		}
	}()
	if resumeState, ok := systemInputs[workflowResumeStateInputKey].(*workflowpause.State); ok && resumeState != nil && resumeState.WorkflowRunID != "" {
		workflowRunLogID = resumeState.WorkflowRunID
		systemInputs["sys.workflow_run_id"] = workflowRunLogID
		if ws, ok := h.workflowService.(*WorkflowService); ok {
			run, loadErr := ws.workflowRunLogRepo.GetByID(c.Request.Context(), workflowRunLogID)
			if loadErr != nil {
				h.sendSSEError(c.Request.Context(), c.Writer, fmt.Sprintf("failed to load workflow run for resume: %v", loadErr))
				return
			} else if run.RuntimeProtocolVersion >= workflowRuntimeProtocolVersionV2 {
				pauseService := workflowpause.NewService(database.GetDB())
				submission, submitErr := pauseService.SubmitInteraction(
					c.Request.Context(),
					workflowRunLogID,
					resumePauseID,
					"question-answer",
					workflowpause.EventQuestionAnswerSubmitted,
					buildQuestionAnswerSubmittedEvent(workflowRunLogID, questionResumeState, req.Inputs),
					questionAnswerSubmissionIdempotencyKey(resumePauseID, resumePauseGeneration, questionResumeState, req.Inputs),
				)
				if submitErr != nil {
					h.sendSSEError(c.Request.Context(), c.Writer, fmt.Sprintf("failed to prepare workflow resume: %v", submitErr))
					return
				}
				if submission != nil {
					questionSubmittedEvent = submission.Event
					publishWorkflowCommittedTail(c.Request.Context(), workflowRunLogID, submission.Event)
					for _, pendingEvent := range submission.PendingEvents {
						publishWorkflowCommittedTail(c.Request.Context(), workflowRunLogID, pendingEvent)
					}
					if !submission.ResumeReady {
						if submission.Event != nil {
							sendWorkflowSSEStoredEventForInvocation(c.Request.Context(), c.Writer, triggeredFrom, *submission.Event)
						}
						for _, pendingEvent := range submission.PendingEvents {
							if pendingEvent != nil {
								sendWorkflowSSEStoredEventForInvocation(c.Request.Context(), c.Writer, triggeredFrom, *pendingEvent)
							}
						}
						return
					}
				}
				if err := h.applyDurableWorkflowStreamResumeInputs(
					c.Request.Context(),
					pauseService,
					workflowRunLogID,
					resumePauseID,
					resumePauseGeneration,
					req,
				); err != nil {
					h.sendSSEError(c.Request.Context(), c.Writer, fmt.Sprintf("failed to load workflow resume inputs: %v", err))
					return
				}
				claim, claimErr := claimWorkflowResume(c.Request.Context(), pauseService, run, resumePauseID)
				if claimErr != nil {
					if errors.Is(claimErr, workflowpause.ErrResumeAlreadyRunning) {
						if currentRun, currentErr := ws.workflowRunLogRepo.GetByID(c.Request.Context(), workflowRunLogID); currentErr == nil && currentRun != nil {
							run = currentRun
						}
						latest, _ := pauseService.LatestEventSequence(c.Request.Context(), run.TenantID, run.ID)
						executionID := ""
						if run.ActiveExecutionID != nil {
							executionID = *run.ActiveExecutionID
						}
						sendWorkflowSSEEventForInvocation(c.Request.Context(), c.Writer, triggeredFrom, "workflow_resume_running", map[string]interface{}{
							"workflow_run_id": run.ID,
							"execution_id":    executionID,
							"event_cursor":    latest,
							"resume_state":    "running",
						})
						return
					}
					h.sendSSEError(c.Request.Context(), c.Writer, fmt.Sprintf("failed to claim workflow resume: %v", claimErr))
					return
				} else if claim != nil {
					executionOwner = workflowExecutionOwner{WorkflowRunID: run.ID, ExecutionID: claim.ExecutionID, Generation: claim.Generation, PauseID: claim.PauseID, PauseGeneration: claim.PauseGeneration}
				}
			} else if err := h.resumeLegacyWorkflowContinuation(
				c.Request.Context(), ws, workflowpause.NewService(database.GetDB()), run, "question_stream_resume",
			); err != nil {
				logger.WarnContext(c.Request.Context(), "failed to resume legacy question answer workflow", "workflow_run_id", workflowRunLogID, err)
			}
		}
	} else if ws, ok := h.workflowService.(*WorkflowService); ok {
		workflowRunLogInterface, err := ws.CreateWorkflowRunLog(c.Request.Context(), workspaceID, appID, workflowID, triggeredFrom, req.Inputs, accountID)
		if err != nil {
			var busyErr *WorkflowConversationBusyError
			if errors.As(err, &busyErr) {
				h.sendSSEErrorData(c.Request.Context(), c.Writer, map[string]interface{}{
					"code":            workflowConversationBusyCode,
					"message":         busyErr.Error(),
					"conversation_id": busyErr.ConversationID,
					"workflow_run_id": busyErr.WorkflowRunID,
					"runtime_status":  busyErr.RuntimeStatus,
					"recoverable":     true,
				})
				return
			}
			logger.CriticalContext(c.Request.Context(), "failed to create workflow run log", "agent_id", appID, "workspace_id", workspaceID, err)
			h.sendSSEError(c.Request.Context(), c.Writer, fmt.Sprintf("failed to create durable workflow run: %v", err))
			return
		} else if workflowRunLog, ok := workflowRunLogInterface.(*WorkflowRunLog); ok {
			workflowRunLogID = workflowRunLog.ID
			executionOwner = workflowExecutionOwnerFromRun(workflowRunLog)
			systemInputs["sys.workflow_run_id"] = workflowRunLogID
			logger.Info("Successfully created workflow run log", "workflowRunLogID", workflowRunLogID)
		} else {
			h.sendSSEError(c.Request.Context(), c.Writer, "failed to create durable workflow run: invalid run record")
			return
		}
	} else {
		h.sendSSEError(c.Request.Context(), c.Writer, "failed to create durable workflow run: runtime service unavailable")
		return
	}

	logger.DebugContext(c.Request.Context(), "workflow stream system inputs prepared",
		zap.Int("system_input_count", len(systemInputs)),
		zap.String("workflow_run_id", workflowRunLogID),
	)

	// The execution lifecycle is independent from the POST SSE connection. A
	// disconnected browser can recover through the durable event stream while
	// an explicit stop still cancels this context through the run registry.
	execCtx, cancelExec := newWorkflowExecutionContext(c.Request.Context())
	defer cancelExec()
	execCtx = withWorkflowExecutionOwner(execCtx, executionOwner)
	stopLeaseRenewal := func() {}
	if executionOwner.ExecutionID != "" {
		execCtx, stopLeaseRenewal = startWorkflowExecutionLeaseRenewal(execCtx, workflowpause.NewService(database.GetDB()), workflowpause.ExecutionClaim{
			WorkflowRunID: workflowRunLogID, PauseID: executionOwner.PauseID,
			Generation: executionOwner.Generation, PauseGeneration: executionOwner.PauseGeneration,
			ExecutionID: executionOwner.ExecutionID,
		})
	}
	defer stopLeaseRenewal()
	runtimeCtx := execCtx
	streamVisible := true
	eventDispatcher := newWorkflowRunEventDispatcher(workspaceID, appID, workflowRunLogID, false, func(eventType string, data map[string]interface{}, stored *workflowpause.RunEventPayload) error {
		if !streamVisible {
			return nil
		}
		if stored != nil {
			envelope := *stored
			envelope.Event = eventType
			envelope.Data = data
			sendWorkflowSSEStoredEventForInvocation(c.Request.Context(), c.Writer, triggeredFrom, envelope)
			return nil
		}
		sendWorkflowSSEEventForInvocation(c.Request.Context(), c.Writer, triggeredFrom, eventType, data)
		return nil
	})
	defer func() {
		if err := eventDispatcher.Close(runtimeCtx); err != nil && !errors.Is(err, workflowpause.ErrExecutionOwnershipLost) {
			logger.WarnContext(runtimeCtx, "failed to close workflow event dispatcher", "workflow_run_id", workflowRunLogID, err)
		}
	}()
	durableEventErr := make(chan error, 1)
	sendAndRecordEvent := func(eventType string, data map[string]interface{}) {
		if err := eventDispatcher.Dispatch(runtimeCtx, eventType, data); err != nil {
			select {
			case durableEventErr <- err:
			default:
			}
			cancelExec()
		}
	}

	// Send workflow started event with real database ID
	startedPayload := buildWorkflowStartedEventPayload(
		runType,
		workflowRunLogID,
		workflowID,
		sequenceNumber,
		systemInputs,
		time.Now().Unix(),
	)
	if resumePauseID == "" {
		sendAndRecordEvent(workflowpause.EventWorkflowStarted, startedPayload)
	}
	if resumePauseID != "" {
		questionSubmittedData := buildQuestionAnswerSubmittedEvent(workflowRunLogID, questionResumeState, req.Inputs)
		if questionSubmittedEvent != nil {
			questionSubmittedData["__stored_sequence"] = questionSubmittedEvent.Sequence
			questionSubmittedData["__stored_event_id"] = questionSubmittedEvent.EventID
			questionSubmittedData["__stored_event_payload"] = questionSubmittedEvent
		}
		sendAndRecordEvent(workflowpause.EventQuestionAnswerSubmitted, questionSubmittedData)
	}

	// Create channels for streaming
	resultChan := make(chan *WorkflowStreamEvent, 100)
	errorChan := make(chan error, 10)
	doneChan := make(chan map[string]interface{}, 1)

	// Track if any message events were sent (for conversation workflows)
	messageEventSent := false
	var conversationAnswer strings.Builder
	answerSnapshots := newWorkflowAnswerSnapshotWriter(runType, h, workflowRunLogID, appID, accountID, systemInputs, req.Inputs, triggeredFrom)
	if answerSnapshots != nil {
		defer answerSnapshots.closeWithoutFlush()
	}
	answerPersistCtx := withWorkflowExecutionOwner(runtimeCtx, executionOwner)

	// Register the cancel function with the workflow service for stop functionality
	if ws, ok := h.workflowService.(*WorkflowService); ok {
		ws.RegisterRunningWorkflow(workflowRunLogID, cancelExec)
		// Ensure cleanup when this function returns
		defer ws.UnregisterRunningWorkflow(workflowRunLogID)
	}

	// Start workflow execution in goroutine
	go func() {
		defer close(doneChan)
		h.executeWorkflowStream(execCtx, workspaceID, appID, req, accountID, taskID, workflowRunLogID, workflowID, systemInputs, sequenceNumber, resultChan, errorChan, doneChan, isDraft, runType, triggeredFrom)
	}()

	sendTerminalFailure := func(err error) bool {
		logger.CriticalContext(runtimeCtx, "workflow execution error", "agent_id", appID, "workspace_id", workspaceID, err)
		if c.Writer == nil {
			logger.CriticalContext(runtimeCtx, "response writer is nil in workflow error channel", "agent_id", appID, "workflow_run_id", workflowRunLogID)
			return false
		}

		userEmail := ""
		userName := ""
		if h.accountService != nil {
			if account, accErr := h.accountService.GetAccountByID(runtimeCtx, accountID); accErr == nil && account != nil {
				userEmail = account.Email
				userName = account.Name
			}
		}
		errorPayload := buildWorkflowStreamErrorPayload(err)
		errorMessage := workflowStreamErrorMessage(errorPayload)
		workflowElapsedTime := h.workflowElapsedMillisecondsForEvent(runtimeCtx, workflowRunLogID, ElapsedMillisecondsSince(workflowStartTime))
		totalSteps := 0
		var totalTokens int64
		if ws, ok := h.workflowService.(*WorkflowService); ok {
			totalSteps = ws.workflowRunNodeStepCount(runtimeCtx, workflowRunLogID)
			totalTokens = ws.workflowRunNodeTotalTokens(runtimeCtx, workflowRunLogID)
		}

		errorEventData := map[string]interface{}{
			"id":               workflowRunLogID,
			"workflow_id":      workflowID,
			"sequence_number":  sequenceNumber,
			"status":           "failed",
			"outputs":          map[string]interface{}{},
			"error":            errorPayload,
			"elapsed_time":     workflowElapsedTime,
			"total_tokens":     totalTokens,
			"total_steps":      totalSteps,
			"created_by":       map[string]interface{}{"id": accountID, "name": userName, "email": userEmail},
			"created_at":       time.Now().Unix(),
			"finished_at":      time.Now().Unix(),
			"exceptions_count": 1,
			"files":            []interface{}{},
		}
		if executionOwner.ExecutionID != "" {
			messageStatus := ""
			var messageEnd map[string]interface{}
			conversationID := ""
			if runType == "CONVERSATION_WORKFLOW" && answerSnapshots != nil {
				answerSnapshots.closeWithoutFlush()
				messageStatus = conversation.AgentMessageStatusError
				conversationID = workflowConversationID(systemInputs, req.Inputs)
				messageEnd = map[string]interface{}{
					"id": workflowRunLogID, "message_id": workflowRunLogID,
					"conversation_id": conversationID,
					"status":          conversation.AgentMessageStatusError,
					"created_at":      time.Now().Unix(),
				}
			}
			messageStatus, messageEnd, projectionErr := resolveWorkflowConversationProjection(runtimeCtx, workflowRunLogID, conversationID, messageStatus, messageEnd)
			if projectionErr != nil {
				logger.ErrorContext(runtimeCtx, "failed to resolve workflow error message projection", "workflow_run_id", workflowRunLogID, projectionErr)
				return false
			}
			errorEvent := map[string]interface{}{"message": errorMessage, "error": errorPayload, "workflow_run_id": workflowRunLogID}
			finalizeCtx := withWorkflowExecutionOwner(context.WithoutCancel(runtimeCtx), executionOwner)
			if finalizeErr := finalizeWorkflowRun(finalizeCtx, finalizeWorkflowRunParams{
				WorkflowRunID: workflowRunLogID, Status: "failed", Outputs: map[string]interface{}{},
				ErrorMessage: errorMessage, ElapsedTime: workflowElapsedTime, TotalTokens: totalTokens, TotalSteps: totalSteps,
				ExceptionsCount: 1, FinalAnswer: conversationAnswer.String(), MessageStatus: messageStatus,
				ErrorEvent: errorEvent, MessageEnd: messageEnd, WorkflowFinished: errorEventData,
			}); finalizeErr != nil {
				logger.ErrorContext(runtimeCtx, "failed to durably finalize workflow error", "workflow_run_id", workflowRunLogID, finalizeErr)
				return false
			}
			sendAndRecordEvent(workflowpause.EventError, errorEvent)
			if len(messageEnd) > 0 {
				sendAndRecordEvent(workflowEventMessageEnd, messageEnd)
			}
			sendAndRecordEvent(workflowpause.EventWorkflowFinished, errorEventData)
			return false
		}

		if h.workflowService != nil {
			_ = h.workflowService.UpdateWorkflowRunLogStatus(runtimeCtx, workflowRunLogID, "failed", map[string]interface{}{}, workflowElapsedTime, totalTokens, totalSteps, errorMessage)
		}
		sendAndRecordEvent(workflowpause.EventError, map[string]interface{}{"message": errorMessage, "error": errorPayload})
		sendAndRecordEvent(workflowpause.EventWorkflowFinished, errorEventData)
		return false
	}

	persistExecutionStopped := func() {
		if h.workflowService == nil || workflowRunLogID == "" {
			return
		}
		persistCtx, cancel := context.WithTimeout(runtimeCtx, workflowStreamDisconnectPersistTimeout)
		defer cancel()

		workflowElapsedTime := h.workflowElapsedMillisecondsForEvent(persistCtx, workflowRunLogID, ElapsedMillisecondsSince(workflowStartTime))
		totalSteps := 0
		var totalTokens int64
		if ws, ok := h.workflowService.(*WorkflowService); ok {
			totalSteps = ws.workflowRunNodeStepCount(persistCtx, workflowRunLogID)
			totalTokens = ws.workflowRunNodeTotalTokens(persistCtx, workflowRunLogID)
		}
		if executionOwner.ExecutionID != "" {
			finalAnswer := conversationAnswer.String()
			messageStatus := ""
			messageEnd := map[string]interface{}(nil)
			conversationID := ""
			if runType == "CONVERSATION_WORKFLOW" && answerSnapshots != nil {
				answerSnapshots.closeWithoutFlush()
				messageStatus = conversation.AgentMessageStatusStopped
				conversationID = workflowConversationID(systemInputs, req.Inputs)
				messageEnd = map[string]interface{}{
					"id": workflowRunLogID, "message_id": workflowRunLogID,
					"conversation_id": conversationID,
					"status":          conversation.AgentMessageStatusStopped, "created_at": time.Now().Unix(),
				}
			}
			messageStatus, messageEnd, projectionErr := resolveWorkflowConversationProjection(persistCtx, workflowRunLogID, conversationID, messageStatus, messageEnd)
			if projectionErr != nil {
				logger.WarnContext(persistCtx, "failed to resolve stopped workflow message projection", "workflow_run_id", workflowRunLogID, projectionErr)
				return
			}
			finished := map[string]interface{}{
				"id": workflowRunLogID, "workflow_id": workflowID, "sequence_number": sequenceNumber,
				"status": "stopped", "outputs": map[string]interface{}{}, "error": nil,
				"elapsed_time": workflowElapsedTime, "total_tokens": totalTokens, "total_steps": totalSteps,
				"created_at": time.Now().Unix(), "finished_at": time.Now().Unix(), "exceptions_count": 0,
				"files": []interface{}{},
			}
			finalizeCtx := withWorkflowExecutionOwner(persistCtx, executionOwner)
			if err := finalizeWorkflowRun(finalizeCtx, finalizeWorkflowRunParams{
				WorkflowRunID: workflowRunLogID, Status: "stopped", Outputs: map[string]interface{}{},
				ElapsedTime: workflowElapsedTime, TotalTokens: totalTokens, TotalSteps: totalSteps, FinalAnswer: finalAnswer,
				MessageStatus: messageStatus, MessageEnd: messageEnd, WorkflowFinished: finished,
			}); err != nil {
				if !errors.Is(err, workflowpause.ErrExecutionOwnershipLost) {
					logger.WarnContext(persistCtx, "failed to finalize stopped workflow", "workflow_run_id", workflowRunLogID, err)
				}
				return
			}
			if len(messageEnd) > 0 {
				sendAndRecordEvent(workflowEventMessageEnd, messageEnd)
			}
			sendAndRecordEvent(workflowpause.EventWorkflowFinished, finished)
			return
		}
		if err := h.workflowService.UpdateWorkflowRunLogStatus(persistCtx, workflowRunLogID, "stopped", map[string]interface{}{}, workflowElapsedTime, totalTokens, totalSteps, workflowStreamClientDisconnectedMessage); err != nil {
			logger.WarnContext(persistCtx, "failed to mark workflow execution as stopped", "workflow_run_id", workflowRunLogID, err)
		}
	}

	// Handle the execution independently from the transport. While the POST SSE
	// connection is open events are both persisted and emitted. If the browser
	// disconnects, the same reducer keeps consuming the execution channels and
	// only the durable projection remains active for snapshot+tail recovery.
	handleSelection := func(selection workflowStreamSelection, emitHeartbeat bool) bool {
		switch selection.kind {
		case workflowStreamSelectionResult:
			event := selection.event
			if event == nil {
				logger.CriticalContext(runtimeCtx, "received nil event from workflow result channel", "agent_id", appID, "workflow_run_id", workflowRunLogID)
				return false
			}
			// Check if c.Writer is nil
			if c.Writer == nil {
				logger.CriticalContext(runtimeCtx, "response writer is nil in workflow result channel", "agent_id", appID, "workflow_run_id", workflowRunLogID)
				return false
			}

			// Track if we've sent any message events
			if event.EventType == workflowEventAnswerSnapshotReady {
				var persistErr error
				if answerSnapshots != nil && runType == "CONVERSATION_WORKFLOW" {
					if forceFlush, _ := event.Data["force_flush"].(bool); forceFlush {
						persistErr = answerSnapshots.Persist(answerPersistCtx, workflowAnswerSnapshotText(event.Data), conversation.AgentMessageStatusRunning, true)
					} else {
						answerSnapshots.PersistAsync(answerPersistCtx, workflowAnswerSnapshotText(event.Data), conversation.AgentMessageStatusRunning, false)
					}
				}
				if event.Persisted != nil {
					event.Persisted <- persistErr
				}
				return true
			}

			if event.EventType == "message" && workflowMessageEventKind(event.Data) != workflowMessageKindQuestionAnswerPrompt {
				messageEventSent = true
				if chunk := workflowMessageEventText(event.Data); chunk != "" {
					conversationAnswer.WriteString(chunk)
					if answerSnapshots != nil && runType == "CONVERSATION_WORKFLOW" {
						// Message chunks are the user-visible answer source. Persist
						// them through the bounded snapshot writer as well as sending
						// the live SSE event, so a reconnect observes the same progress.
						answerSnapshots.PersistAsync(answerPersistCtx, conversationAnswer.String(), conversation.AgentMessageStatusRunning, false)
					}
				}
			}

			if event.EventType == "workflow_paused" && runType == "CONVERSATION_WORKFLOW" {
				if executionOwner.ExecutionID != "" {
					if answerSnapshots != nil {
						answerSnapshots.close()
					}
				} else {
					messageStatus := workflowPausedMessageStatus(event.Data)
					if answerSnapshots != nil {
						if err := answerSnapshots.PersistFinal(answerPersistCtx, conversationAnswer.String(), messageStatus); err != nil {
							logger.ErrorContext(runtimeCtx, "failed to flush workflow answer before pause", "workflow_run_id", workflowRunLogID, err)
							return false
						}
					} else {
						h.persistApprovalPauseConversationMessage(runtimeCtx, workflowRunLogID, appID, accountID, systemInputs, req.Inputs, triggeredFrom, conversationAnswer.String())
					}
				}
			}

			// Use c.Writer instead of w for SSE events
			sendAndRecordEvent(event.EventType, event.Data)

			// If this is a workflow_finished event with stopped status, end the stream
			if event.EventType == "workflow_finished" {
				// return false
				if status, ok := event.Data["status"].(string); ok && status == "stopped" {
					logger.Info("Workflow stopped, ending stream")
					return false
				}
			}
			if event.EventType == "workflow_paused" {
				return false
			}
			return true

		case workflowStreamSelectionError:
			return sendTerminalFailure(selection.err)

		case workflowStreamSelectionDone:
			outputs, ok := selection.outputs, selection.ok
			// Check if c.Writer is nil
			if c.Writer == nil {
				logger.CriticalContext(runtimeCtx, "response writer is nil in workflow done channel", "agent_id", appID, "workflow_run_id", workflowRunLogID)
				return false
			}
			if !ok {
				return sendTerminalFailure(errors.New(workflowStreamEndedWithoutFinalResultMessage))
			}

			// Extract workflow status from outputs (internal fields)
			workflowStatus := "succeeded"
			var workflowError interface{} = nil
			exceptionsCount := 0
			totalTokens := 0
			if status, exists := outputs["__workflow_status__"]; exists {
				if s, ok := status.(string); ok {
					workflowStatus = s
				}
				delete(outputs, "__workflow_status__")
			}
			if errMsg, exists := outputs["__workflow_error__"]; exists {
				if workflowStatus == "failed" {
					workflowError = map[string]interface{}{"message": errMsg}
					exceptionsCount = 1
				}
				delete(outputs, "__workflow_error__")
			}
			if tokens, exists := outputs["__total_tokens__"]; exists {
				if t, ok := tokens.(int); ok {
					totalTokens = t
				}
				delete(outputs, "__total_tokens__")
			}

			// Get user account information
			userEmail := ""
			userName := ""
			if account, err := h.accountService.GetAccountByID(runtimeCtx, accountID); err == nil && account != nil {
				userEmail = account.Email
				userName = account.Name
			} else {
				logger.ErrorContext(runtimeCtx, "failed to get account information", "account_id", accountID, err)
			}

			workflowElapsedTime := workflowElapsedMillisecondsFromOutputs(outputs, h.workflowElapsedMillisecondsForEvent(runtimeCtx, workflowRunLogID, ElapsedMillisecondsSince(workflowStartTime)))

			finalConversationAnswer := ""
			var terminalMessageEnd map[string]interface{}
			// For conversation workflows, send message and message_end events BEFORE workflow_finished
			if runType == "CONVERSATION_WORKFLOW" {
				// Get conversation_id from system inputs if available
				conversationID := ""
				if convID, ok := systemInputs["sys.conversation_id"].(string); ok {
					conversationID = convID
				} else {
					logger.WarnContext(runtimeCtx, "conversation id missing for workflow message end event",
						zap.Int("system_input_count", len(systemInputs)),
					)
				}

				logger.DebugContext(runtimeCtx, "sending workflow message end event",
					zap.String("conversation_id", conversationID),
				)

				// If no message events were sent during streaming (e.g., no watched selectors),
				// send a complete message event with the final answer
				if !messageEventSent {
					logger.DebugContext(runtimeCtx, "sending complete workflow message event")

					answer := extractWorkflowAnswer(outputs)
					if answer == "" {
						logger.DebugContext(runtimeCtx, "workflow answer missing for complete message event",
							zap.Int("output_count", len(outputs)),
							zap.Strings("output_keys", workflowOutputKeys(outputs)),
						)
					}

					// Send complete message event with the full answer
					sendAndRecordEvent("message", map[string]interface{}{
						"id":              workflowRunLogID,
						"message_id":      workflowRunLogID,
						"conversation_id": conversationID,
						"answer":          answer,
						"created_at":      time.Now().Unix(),
					})
					if chunk := answer; chunk != "" {
						conversationAnswer.WriteString(chunk)
					}

					logger.DebugContext(runtimeCtx, "sent complete workflow message event",
						zap.Int("answer_length", len(answer)),
					)
				}

				finalAnswer := extractWorkflowAnswer(outputs)
				if finalAnswer == "" {
					finalAnswer = conversationAnswer.String()
				}
				finalConversationAnswer = finalAnswer
				if answerSnapshots != nil {
					answerSnapshots.closeWithoutFlush()
				}

				terminalMessageEnd = map[string]interface{}{
					"id":              workflowRunLogID, // Using workflowRunLogID as message ID
					"message_id":      workflowRunLogID, // Same as id for compatibility
					"conversation_id": conversationID,   // Add conversation_id
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
					"created_at": time.Now().Unix(),
				}
			}

			totalSteps := 0
			if ws, ok := h.workflowService.(*WorkflowService); ok {
				totalSteps = ws.workflowRunNodeStepCount(runtimeCtx, workflowRunLogID)
				if persistedElapsed := ws.workflowRunNodeElapsedMilliseconds(runtimeCtx, workflowRunLogID); persistedElapsed > 0 {
					workflowElapsedTime = persistedElapsed
				}
				if persistedTokens := ws.workflowRunNodeTotalTokens(runtimeCtx, workflowRunLogID); persistedTokens > 0 {
					totalTokens = int(persistedTokens)
				}
			}
			if metadata, ok := terminalMessageEnd["metadata"].(map[string]interface{}); ok {
				if usage, ok := metadata["usage"].(map[string]interface{}); ok {
					usage["total_tokens"] = totalTokens
				}
			}
			if finalConversationAnswer != "" && executionOwner.ExecutionID == "" {
				outputs = workflowOutputsWithConversationAnswer(outputs, finalConversationAnswer)
				if h.workflowService != nil {
					errorMessage := workflowRunEventErrorMessage(workflowError)
					if err := h.workflowService.UpdateWorkflowRunLogStatus(runtimeCtx, workflowRunLogID, workflowStatus, outputs, workflowElapsedTime, int64(totalTokens), totalSteps, errorMessage); err != nil {
						logger.ErrorContext(runtimeCtx, "failed to persist streamed workflow answer", "workflow_run_id", workflowRunLogID, err)
					}
				}
			}

			workflowFinished := map[string]interface{}{
				"id":               workflowRunLogID,
				"workflow_id":      workflowID,
				"sequence_number":  sequenceNumber,
				"status":           workflowStatus,
				"outputs":          outputs,
				"error":            workflowError,
				"elapsed_time":     workflowElapsedTime,
				"total_tokens":     totalTokens,
				"total_steps":      totalSteps,
				"created_by":       map[string]interface{}{"id": accountID, "name": userName, "email": userEmail},
				"created_at":       time.Now().Unix(),
				"finished_at":      time.Now().Unix(),
				"exceptions_count": exceptionsCount,
				"files":            []interface{}{},
			}
			if executionOwner.ExecutionID != "" {
				finalizeCtx := withWorkflowExecutionOwner(runtimeCtx, executionOwner)
				messageStatus := ""
				if terminalMessageEnd != nil {
					messageStatus = workflowStatusToMessageStatus(workflowStatus)
				}
				messageStatus, terminalMessageEnd, projectionErr := resolveWorkflowConversationProjection(runtimeCtx, workflowRunLogID, workflowConversationID(systemInputs, req.Inputs), messageStatus, terminalMessageEnd)
				if projectionErr != nil {
					return sendTerminalFailure(fmt.Errorf("resolve workflow terminal message projection: %w", projectionErr))
				}
				if err := finalizeWorkflowRun(finalizeCtx, finalizeWorkflowRunParams{
					WorkflowRunID:    workflowRunLogID,
					Status:           workflowStatus,
					Outputs:          outputs,
					ErrorMessage:     workflowRunEventErrorMessage(workflowError),
					ElapsedTime:      workflowElapsedTime,
					TotalTokens:      int64(totalTokens),
					TotalSteps:       totalSteps,
					ExceptionsCount:  exceptionsCount,
					FinalAnswer:      finalConversationAnswer,
					MessageStatus:    messageStatus,
					MessageEnd:       terminalMessageEnd,
					WorkflowFinished: workflowFinished,
				}); err != nil {
					if errors.Is(err, workflowpause.ErrExecutionOwnershipLost) {
						logger.WarnContext(runtimeCtx, "stale workflow execution could not finalize", "workflow_run_id", workflowRunLogID, err)
						return false
					}
					return sendTerminalFailure(err)
				}
			}
			if len(terminalMessageEnd) > 0 {
				sendAndRecordEvent(workflowEventMessageEnd, terminalMessageEnd)
			}
			// Send workflow_finished event LAST (after durable message and message_end).
			sendAndRecordEvent(workflowpause.EventWorkflowFinished, workflowFinished)

			return false

		case workflowStreamSelectionContextDone:
			select {
			case durableErr := <-durableEventErr:
				logger.ErrorContext(runtimeCtx, "workflow execution stopped after durable event persistence failed", "workflow_run_id", workflowRunLogID, durableErr)
				return sendTerminalFailure(fmt.Errorf("%w: %w", errWorkflowEventPersistenceFailed, durableErr))
			default:
			}
			cause := workflowshared.ResolveContextError(execCtx, execCtx.Err())
			if errors.Is(cause, workflowpause.ErrExecutionOwnershipLost) {
				logger.WarnContext(runtimeCtx, "workflow execution stopped after ownership loss", "workflow_run_id", workflowRunLogID, cause)
				return false
			}
			if !workflowshared.IsContextCancellation(execCtx, cause) {
				logger.ErrorContext(runtimeCtx, "workflow execution stopped after context failure", "workflow_run_id", workflowRunLogID, cause)
				return sendTerminalFailure(cause)
			}
			logger.Info("workflow execution context stopped", map[string]interface{}{
				"task_id":         taskID,
				"app_id":          appID,
				"workflow_run_id": workflowRunLogID,
			})
			persistExecutionStopped()
			return false

		case workflowStreamSelectionHeartbeat:
			if emitHeartbeat {
				sendWorkflowSSEKeepAlive(c.Request.Context(), c.Writer)
			}
			return true

		default:
			return false
		}
	}

	streamLifecycleComplete := false
	c.Stream(func(_ io.Writer) bool {
		selection := receiveWorkflowStreamSelection(resultChan, errorChan, doneChan, c.Request.Context().Done())
		if selection.kind == workflowStreamSelectionContextDone {
			return false
		}
		keepOpen := handleSelection(selection, true)
		if !keepOpen {
			streamLifecycleComplete = true
		}
		return keepOpen
	})
	if streamLifecycleComplete {
		return
	}

	streamVisible = false
	logger.InfoContext(runtimeCtx, "workflow POST stream disconnected; continuing execution for event recovery", "workflow_run_id", workflowRunLogID)
	for {
		selection := receiveWorkflowStreamSelection(resultChan, errorChan, doneChan, execCtx.Done())
		if !handleSelection(selection, false) {
			return
		}
	}
}

func (h *WorkflowHandler) applyDurableWorkflowStreamResumeInputs(
	ctx context.Context,
	pauseService *workflowpause.Service,
	workflowRunID string,
	pauseID string,
	pauseGeneration int64,
	req *dto.DraftWorkflowRunRequest,
) error {
	if req == nil {
		return fmt.Errorf("workflow run request is required")
	}
	inputs, err := loadDurableWorkflowResumeInputs(
		ctx,
		pauseService,
		workflowRunID,
		&workflowpause.RunPause{ID: pauseID, Generation: pauseGeneration},
		req.Inputs,
	)
	if err != nil {
		return err
	}
	req.Inputs = inputs
	return nil
}

func newWorkflowExecutionContext(requestContext context.Context) (context.Context, context.CancelFunc) {
	return context.WithCancel(context.WithoutCancel(requestContext))
}
