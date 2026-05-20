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

	var resumePauseID string
	if runType == "CONVERSATION_WORKFLOW" {
		resumeState, pauseID, ok := h.workflowQuestionAnswerResumeState(c.Request.Context(), workspaceID, appID, systemInputs)
		if ok {
			systemInputs[workflowResumeStateInputKey] = resumeState
			systemInputs[workflowResumePauseIDInputKey] = pauseID
			resumePauseID = pauseID
		}
	}

	// triggeredFrom is now passed as a parameter, no need to compute it here

	// Create workflow run log for streaming execution
	var workflowRunLogID string
	defer func() {
		if ws, ok := h.workflowService.(*WorkflowService); ok {
			ws.cleanupWorkflowReusableSessionsWithTimeout(workflowRunLogID)
		}
	}()
	if resumeState, ok := systemInputs[workflowResumeStateInputKey].(*workflowpause.State); ok && resumeState != nil && resumeState.WorkflowRunID != "" {
		workflowRunLogID = resumeState.WorkflowRunID
		systemInputs["sys.workflow_run_id"] = workflowRunLogID
		if ws, ok := h.workflowService.(*WorkflowService); ok {
			if err := ws.ResumeWorkflowRunLog(c.Request.Context(), workflowRunLogID); err != nil {
				logger.WarnContext(c.Request.Context(), "failed to resume question answer workflow run log", "workflow_run_id", workflowRunLogID, err)
			}
		}
	} else if ws, ok := h.workflowService.(*WorkflowService); ok {
		workflowRunLogInterface, err := ws.CreateWorkflowRunLog(c.Request.Context(), workspaceID, appID, workflowID, triggeredFrom, req.Inputs, accountID)
		if err != nil {
			logger.CriticalContext(c.Request.Context(), "failed to create workflow run log", "agent_id", appID, "workspace_id", workspaceID, err)
			workflowRunLogID = fmt.Sprintf("run-%s-%d", appID, time.Now().UnixNano())
		} else if workflowRunLog, ok := workflowRunLogInterface.(*WorkflowRunLog); ok {
			workflowRunLogID = workflowRunLog.ID
			systemInputs["sys.workflow_run_id"] = workflowRunLogID
			logger.Info("Successfully created workflow run log", "workflowRunLogID", workflowRunLogID)
		} else {
			// Use fallback ID if type assertion fails
			workflowRunLogID = fmt.Sprintf("run-%s-%d", appID, time.Now().UnixNano())
		}
	} else {
		// Use fallback ID if service cast fails
		workflowRunLogID = fmt.Sprintf("run-%s-%d", appID, time.Now().UnixNano())
	}

	logger.DebugContext(c.Request.Context(), "workflow stream system inputs prepared",
		zap.Int("system_input_count", len(systemInputs)),
		zap.String("workflow_run_id", workflowRunLogID),
	)

	eventRecorder := newWorkflowRunEventRecorder(workspaceID, appID, workflowRunLogID)
	defer eventRecorder.Close()
	sendAndRecordEvent := func(eventType string, data map[string]interface{}) {
		publicData := sanitizeWorkflowEventData(data)
		sendWorkflowSSEEvent(c.Request.Context(), c.Writer, eventType, publicData)
		if eventType != workflowEventMessage && eventType != workflowEventAnswerSnapshotReady {
			eventRecorder.Record(c.Request.Context(), eventType, publicData)
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
	sendAndRecordEvent(workflowpause.EventWorkflowStarted, startedPayload)
	if resumePauseID != "" {
		resumeState, _ := systemInputs[workflowResumeStateInputKey].(*workflowpause.State)
		sendAndRecordEvent(workflowpause.EventQuestionAnswerSubmitted, buildQuestionAnswerSubmittedEvent(workflowRunLogID, resumeState, req.Inputs))
	}

	// Create channels for streaming
	resultChan := make(chan *WorkflowStreamEvent, 100)
	errorChan := make(chan error, 10)
	doneChan := make(chan map[string]interface{}, 1)

	// Track if any message events were sent (for conversation workflows)
	messageEventSent := false
	var conversationAnswer strings.Builder
	answerSnapshots := newAnswerSnapshotWriter(h, workflowRunLogID, appID, accountID, systemInputs, req.Inputs, triggeredFrom)

	// Create a cancellable context for this workflow execution
	execCtx, cancelExec := context.WithCancel(c.Request.Context())

	// Register the cancel function with the workflow service for stop functionality
	if ws, ok := h.workflowService.(*WorkflowService); ok {
		ws.RegisterRunningWorkflow(workflowRunLogID, cancelExec)
		// Ensure cleanup when this function returns
		defer ws.UnregisterRunningWorkflow(workflowRunLogID)
	}

	// Start workflow execution in goroutine
	go func() {
		defer close(doneChan)
		h.executeWorkflowStream(c, execCtx, workspaceID, appID, req, accountID, taskID, workflowRunLogID, workflowID, systemInputs, sequenceNumber, resultChan, errorChan, doneChan, isDraft, runType, triggeredFrom)
	}()

	sendTerminalFailure := func(err error) bool {
		logger.CriticalContext(c.Request.Context(), "workflow execution error", "agent_id", appID, "workspace_id", workspaceID, err)
		if c.Writer == nil {
			logger.CriticalContext(c.Request.Context(), "response writer is nil in workflow error channel", "agent_id", appID, "workflow_run_id", workflowRunLogID)
			return false
		}

		userEmail := ""
		userName := ""
		if h.accountService != nil {
			if account, accErr := h.accountService.GetAccountByID(c.Request.Context(), accountID); accErr == nil && account != nil {
				userEmail = account.Email
				userName = account.Name
			}
		}
		errorPayload := buildWorkflowStreamErrorPayload(err)
		errorMessage := workflowStreamErrorMessage(errorPayload)
		workflowElapsedTime := h.workflowElapsedMillisecondsForEvent(c.Request.Context(), workflowRunLogID, ElapsedMillisecondsSince(workflowStartTime))
		totalSteps := 0
		if ws, ok := h.workflowService.(*WorkflowService); ok {
			totalSteps = ws.workflowRunNodeStepCount(c.Request.Context(), workflowRunLogID)
		}

		errorEventData := map[string]interface{}{
			"id":               workflowRunLogID,
			"workflow_id":      workflowID,
			"sequence_number":  sequenceNumber,
			"status":           "failed",
			"outputs":          map[string]interface{}{},
			"error":            errorPayload,
			"elapsed_time":     workflowElapsedTime,
			"total_tokens":     0,
			"total_steps":      totalSteps,
			"created_by":       map[string]interface{}{"id": accountID, "name": userName, "email": userEmail},
			"created_at":       time.Now().Unix(),
			"finished_at":      time.Now().Unix(),
			"exceptions_count": 1,
			"files":            []interface{}{},
		}
		if h.workflowService != nil {
			_ = h.workflowService.UpdateWorkflowRunLogStatus(c.Request.Context(), workflowRunLogID, "failed", map[string]interface{}{}, workflowElapsedTime, 0, totalSteps, errorMessage)
		}
		if runType == "CONVERSATION_WORKFLOW" && answerSnapshots != nil && conversationAnswer.Len() > 0 {
			answerSnapshots.Persist(c.Request.Context(), conversationAnswer.String(), conversation.AgentMessageStatusError, true)
		}

		sendAndRecordEvent(workflowpause.EventNodeFinished, errorEventData)
		sendAndRecordEvent(workflowpause.EventWorkflowFinished, map[string]interface{}{
			"id":               workflowRunLogID,
			"workflow_id":      workflowID,
			"sequence_number":  sequenceNumber,
			"status":           "failed",
			"outputs":          map[string]interface{}{},
			"error":            errorPayload,
			"elapsed_time":     workflowElapsedTime,
			"total_tokens":     0,
			"total_steps":      totalSteps,
			"created_by":       map[string]interface{}{"id": accountID, "name": userName, "email": userEmail},
			"created_at":       time.Now().Unix(),
			"finished_at":      time.Now().Unix(),
			"exceptions_count": 1,
			"files":            []interface{}{},
		})
		return false
	}

	persistClientDisconnected := func() {
		if h.workflowService == nil || workflowRunLogID == "" {
			return
		}
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), workflowStreamDisconnectPersistTimeout)
		defer cancel()

		workflowElapsedTime := h.workflowElapsedMillisecondsForEvent(persistCtx, workflowRunLogID, ElapsedMillisecondsSince(workflowStartTime))
		totalSteps := 0
		if ws, ok := h.workflowService.(*WorkflowService); ok {
			totalSteps = ws.workflowRunNodeStepCount(persistCtx, workflowRunLogID)
		}
		if err := h.workflowService.UpdateWorkflowRunLogStatus(persistCtx, workflowRunLogID, "stopped", map[string]interface{}{}, workflowElapsedTime, 0, totalSteps, workflowStreamClientDisconnectedMessage); err != nil {
			logger.WarnContext(persistCtx, "failed to mark disconnected workflow stream as stopped", "workflow_run_id", workflowRunLogID, err)
		}
	}

	// Handle streaming response
	c.Stream(func(w io.Writer) bool {
		selection := receiveWorkflowStreamSelection(resultChan, errorChan, doneChan, c.Request.Context().Done())

		switch selection.kind {
		case workflowStreamSelectionResult:
			event := selection.event
			if event == nil {
				logger.CriticalContext(c.Request.Context(), "received nil event from workflow result channel", "agent_id", appID, "workflow_run_id", workflowRunLogID)
				return false
			}
			// Check if c.Writer is nil
			if c.Writer == nil {
				logger.CriticalContext(c.Request.Context(), "response writer is nil in workflow result channel", "agent_id", appID, "workflow_run_id", workflowRunLogID)
				return false
			}

			// Track if we've sent any message events
			if event.EventType == workflowEventAnswerSnapshotReady {
				if answerSnapshots != nil && runType == "CONVERSATION_WORKFLOW" {
					answerSnapshots.PersistAsync(c.Request.Context(), workflowAnswerSnapshotText(event.Data), conversation.AgentMessageStatusRunning, false)
				}
				return true
			}

			if event.EventType == "message" && workflowMessageEventKind(event.Data) != workflowMessageKindQuestionAnswerPrompt {
				messageEventSent = true
				if chunk := workflowMessageEventText(event.Data); chunk != "" {
					conversationAnswer.WriteString(chunk)
				}
			}

			if event.EventType == "workflow_paused" && runType == "CONVERSATION_WORKFLOW" {
				messageStatus := workflowPausedMessageStatus(event.Data)
				if answerSnapshots != nil {
					answerSnapshots.PersistAsync(c.Request.Context(), conversationAnswer.String(), messageStatus, true)
				} else {
					h.persistApprovalPauseConversationMessage(c.Request.Context(), workflowRunLogID, appID, accountID, systemInputs, req.Inputs, triggeredFrom, conversationAnswer.String())
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
				logger.CriticalContext(c.Request.Context(), "response writer is nil in workflow done channel", "agent_id", appID, "workflow_run_id", workflowRunLogID)
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
			if account, err := h.accountService.GetAccountByID(c.Request.Context(), accountID); err == nil && account != nil {
				userEmail = account.Email
				userName = account.Name
			} else {
				logger.ErrorContext(c.Request.Context(), "failed to get account information", "account_id", accountID, err)
			}

			workflowElapsedTime := workflowElapsedMillisecondsFromOutputs(outputs, h.workflowElapsedMillisecondsForEvent(c.Request.Context(), workflowRunLogID, ElapsedMillisecondsSince(workflowStartTime)))

			// For conversation workflows, send message and message_end events BEFORE workflow_finished
			if runType == "CONVERSATION_WORKFLOW" {
				// Get conversation_id from system inputs if available
				conversationID := ""
				if convID, ok := systemInputs["sys.conversation_id"].(string); ok {
					conversationID = convID
				} else {
					logger.WarnContext(c.Request.Context(), "conversation id missing for workflow message end event",
						zap.Int("system_input_count", len(systemInputs)),
					)
				}

				logger.DebugContext(c.Request.Context(), "sending workflow message end event",
					zap.String("conversation_id", conversationID),
				)

				// If no message events were sent during streaming (e.g., no watched selectors),
				// send a complete message event with the final answer
				if !messageEventSent {
					logger.DebugContext(c.Request.Context(), "sending complete workflow message event")

					answer := extractWorkflowAnswer(outputs)
					if answer == "" {
						logger.DebugContext(c.Request.Context(), "workflow answer missing for complete message event",
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

					logger.DebugContext(c.Request.Context(), "sent complete workflow message event",
						zap.Int("answer_length", len(answer)),
					)
				}

				finalAnswer := extractWorkflowAnswer(outputs)
				if finalAnswer == "" {
					finalAnswer = conversationAnswer.String()
				}
				if answerSnapshots != nil {
					answerSnapshots.PersistAsync(c.Request.Context(), finalAnswer, workflowStatusToMessageStatus(workflowStatus), true)
				}

				sendAndRecordEvent("message_end", map[string]interface{}{
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
				})
			}

			// Send workflow_finished event LAST (after message and message_end for conversation workflows)
			sendAndRecordEvent(workflowpause.EventWorkflowFinished, map[string]interface{}{
				"id":               workflowRunLogID,
				"workflow_id":      workflowID,
				"sequence_number":  sequenceNumber,
				"status":           workflowStatus,
				"outputs":          outputs,
				"error":            workflowError,
				"elapsed_time":     workflowElapsedTime,
				"total_tokens":     totalTokens,
				"total_steps":      3,
				"created_by":       map[string]interface{}{"id": accountID, "name": userName, "email": userEmail},
				"created_at":       time.Now().Unix(),
				"finished_at":      time.Now().Unix(),
				"exceptions_count": exceptionsCount,
				"files":            []interface{}{},
			})

			return false

		case workflowStreamSelectionContextDone:
			logger.Info("client disconnected from workflow stream", map[string]interface{}{
				"task_id":         taskID,
				"app_id":          appID,
				"workflow_run_id": workflowRunLogID,
			})
			persistClientDisconnected()
			return false

		case workflowStreamSelectionHeartbeat:
			sendWorkflowSSEKeepAlive(c.Request.Context(), c.Writer)
			return true

		default:
			return false
		}
	})
}
