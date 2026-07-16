package workflow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/diagnosis"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	workflow_shared "github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/streamscheduler"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// executeWorkflowStream executes workflow and sends events through channels.
func (h *WorkflowHandler) executeWorkflowStream(c *gin.Context, ctx context.Context, workspaceID, appID string, req *dto.DraftWorkflowRunRequest, accountID, taskID, workflowRunID, workflowID string, systemInputs map[string]interface{}, sequenceNumber int, resultChan chan<- *WorkflowStreamEvent, errorChan chan<- error, doneChan chan<- map[string]interface{}, isDraft bool, runType string, triggeredFrom string) {
	// Record workflow start time for elapsed time calculation
	workflowStartTime := time.Now()

	defer close(resultChan)
	defer close(errorChan)
	// Panic safety to route panic as error
	defer func() {
		if r := recover(); r != nil {
			var panicErr error
			switch v := r.(type) {
			case error:
				panicErr = v
			default:
				panicErr = fmt.Errorf("panic: %v", v)
			}
			select {
			case errorChan <- panicErr:
			default:
			}
		}
	}()

	workflowData, err := h.loadWorkflowStreamData(ctx, workspaceID, appID, runType, isDraft)
	if err != nil {
		errorChan <- err
		return
	}

	h.updateConversationWorkflowConfigAsync(ctx, appID, workflowID, runType)

	streamGraph, err := buildWorkflowStreamGraph(ctx, workflowData)
	if err != nil {
		errorChan <- err
		return
	}
	graphData := streamGraph.GraphData
	nodeMap := streamGraph.NodeMap
	runtimeNodeMap := streamGraph.RuntimeNodeMap
	edgeMap := streamGraph.EdgeMap
	reverseEdgeMap := streamGraph.ReverseEdgeMap
	startNodeID := streamGraph.StartNodeID
	streamWatchConfig := streamGraph.WatchConfig
	watchedSelectors := streamWatchConfig.watchedSelectors
	answerCoordinator := newAnswerOutputCoordinator(runType, workflowRunID, systemInputs, streamGraph, resultChan)

	streamRuntime, err := h.prepareWorkflowStreamRuntime(ctx, appID, workflowID, workflowRunID)
	if err != nil {
		errorChan <- err
		return
	}
	workflowService := streamRuntime.WorkflowService
	workflowElapsedTracker := streamRuntime.ElapsedTracker
	executor := streamRuntime.Executor
	hydrateWorkflowStreamInputs(ctx, executor, req)

	sharedVariablePool, err := buildWorkflowStreamVariablePool(ctx, graphData, systemInputs, req.Inputs, startNodeID, workflowStreamVariablePoolScope{
		ConversationAccess: h.advancedChatHandler,
		VariableLoader:     h.advancedChatHandler,
		AgentID:            appID,
		AccountID:          accountID,
	})
	if err != nil {
		errorChan <- err
		return
	}
	resumeState, hasResumeState := detachWorkflowResumeState(systemInputs)
	if !hasResumeState {
		h.executeWorkflowStreamGraphEngine(ctx, workspaceID, appID, req, accountID, workflowRunID, workflowID, systemInputs, sequenceNumber, resultChan, errorChan, doneChan, isDraft, runType, triggeredFrom, streamGraph, streamRuntime, workflowStartTime, answerCoordinator)
		return
	}

	// Execute workflow nodes with support for parallel branches
	// Use a queue-based approach to handle multiple branches
	nodeQueue := []string{startNodeID}
	nodeIndex := 1
	// Track completed nodes to avoid cycles and redundant execution
	completedNodes := make(map[string]bool)
	// Track failed nodes for final status determination
	failedNodes := make(map[string]string) // nodeID -> error message

	// Track predecessor for each node
	nodePredecessors := make(map[string]*string)

	// Track outputs by node ID for internal logic (like branch selection)
	executionOutputs := make(map[string]map[string]interface{})

	// Track final workflow outputs (flattened). Loop nodes are also allowed to contribute
	// final outputs because their child response nodes are projected onto the container output.
	allNodeOutputs := make(map[string]interface{})
	// Track total tokens across all LLM nodes
	totalWorkflowTokens := 0
	conversationMessageChunkNodes := make(map[string]bool)

	if hasResumeState {
		pausedNodeIDs := workflowResumePausedNodeIDs(resumeState.ExecutorState)
		if len(pausedNodeIDs) == 0 {
			errorChan <- fmt.Errorf("workflow resume state missing paused node id")
			return
		}
		for _, pausedNodeID := range pausedNodeIDs {
			if _, exists := nodeMap[pausedNodeID]; !exists {
				errorChan <- fmt.Errorf("workflow resume node %s not found", pausedNodeID)
				return
			}
		}

		nodeQueue = append(append([]string(nil), pausedNodeIDs...), resumeState.ExecutorState.NodeQueue...)
		completedNodes = copyWorkflowBoolMap(resumeState.ExecutorState.CompletedNodes)
		failedNodes = copyWorkflowStringMap(resumeState.ExecutorState.FailedNodes)
		executionOutputs = copyWorkflowNestedMap(resumeState.ExecutorState.ExecutionOutputs)
		allNodeOutputs = copyWorkflowAnyMap(resumeState.ExecutorState.AllNodeOutputs)
		nodeIndex = resumeState.ExecutorState.NodeIndex
		if nodeIndex <= 0 {
			nodeIndex = 1
		}
		totalWorkflowTokens = resumeState.ExecutorState.TotalTokens
		if resumeState.ExecutorState.PredecessorNodeID != nil {
			nodePredecessors[pausedNodeIDs[0]] = resumeState.ExecutorState.PredecessorNodeID
		}
		workflowpause.RestoreVariablePoolSnapshot(sharedVariablePool, resumeState.VariablePool)
		questionAnswerResume := false
		for _, pausedNodeID := range pausedNodeIDs {
			if workflowStreamNodeType(nodeMap, pausedNodeID) != string(workflow_shared.QuestionAnswer) {
				clearResumedNodeVariables(sharedVariablePool, pausedNodeID)
			} else {
				questionAnswerResume = true
			}
		}
		if questionAnswerResume {
			var requestInputs map[string]interface{}
			if req != nil {
				requestInputs = req.Inputs
			}
			restoreQuestionAnswerResumeInputs(sharedVariablePool, systemInputs, requestInputs, resumeState)
		}
		if answerCoordinator != nil {
			if err := answerCoordinator.RestorePauseSnapshot(resumeState.AnswerOutput); err != nil {
				logger.WarnContext(ctx, "failed to restore answer output pause state", "workflow_run_id", workflowRunID, err)
			}
		}
	}

	for len(nodeQueue) > 0 {
		// Pop the first node from the queue
		currentNodeID := nodeQueue[0]
		nodeQueue = nodeQueue[1:]

		// Skip if already completed (in case of duplicate queue entries)
		if completedNodes[currentNodeID] {
			continue
		}

		// Check if all upstream dependencies are satisfied
		upstreamNodes := reverseEdgeMap[currentNodeID]
		allUpstreamComplete := true
		for _, upstreamNode := range upstreamNodes {
			if !completedNodes[upstreamNode] {
				allUpstreamComplete = false
				break
			}
		}
		if !allUpstreamComplete {
			// Re-queue this node to be processed later
			nodeQueue = append(nodeQueue, currentNodeID)
			continue
		}

		// Check if node should be run or skipped (conditional branching)
		shouldRun := false
		if len(upstreamNodes) == 0 {
			shouldRun = true
		} else {
			for _, upstreamID := range upstreamNodes {
				// If upstream was skipped, it cannot activate this node
				if failedNodes[upstreamID] == "skipped" {
					continue
				}

				// Check which handle connects upstream to current
				var connectingHandle string
				if targetsByHandle, ok := edgeMap[upstreamID]; ok {
					for handle, targets := range targetsByHandle {
						for _, target := range targets {
							if target == currentNodeID {
								connectingHandle = handle
								break
							}
						}
						if connectingHandle != "" {
							break
						}
					}
				}

				// Check upstream output to see which handle was selected.
				// Uses __edge_source_handle__ (populated from EdgeSourceHandle field in node run result).
				selectedHandle := "source" // default
				if upstreamOutputs, ok := executionOutputs[upstreamID]; ok {
					if eh, ok := upstreamOutputs["__edge_source_handle__"].(string); ok && eh != "" {
						selectedHandle = eh
					}
					// Check for loop/iteration (not implemented yet, default to source)
				}

				// If handles match, this upstream activates the current node
				if connectingHandle == "" || connectingHandle == selectedHandle {
					shouldRun = true
					break
				}
			}
		}

		if !shouldRun {
			logger.Info("Node skipped due to inactive upstream branch", "nodeID", currentNodeID)
			completedNodes[currentNodeID] = true
			failedNodes[currentNodeID] = "skipped" // Re-use failedNodes to track skipped status
			if answerCoordinator != nil {
				answerCoordinator.MarkNodeSkipped(currentNodeID)
			}

			// IMPORTANT: Still enqueue downstream nodes so the "skipped" status can propagate.
			// Otherwise, deeper nodes in the inactive branch are never visited, remain incomplete,
			// and can deadlock merge nodes that depend on all upstreams being completed.
			streamscheduler.EnqueueDownstreams(&nodeQueue, edgeMap, currentNodeID, completedNodes, nodePredecessors)

			// Skipped nodes don't need to send events to frontend.
			// The user can see which branch was taken by looking at the if-else node output.
			continue
		}

		predecessorNodeID := nodePredecessors[currentNodeID]
		// Check for context cancellation first (triggered by stop API)
		select {
		case <-ctx.Done():
			logger.Info("Context canceled, stopping workflow execution", "error", ctx.Err())

			workflowElapsedTime := workflowElapsedTracker.elapsedOrFallback(ElapsedMillisecondsSince(workflowStartTime))
			h.sendWorkflowStoppedEvent(ctx, resultChan, workflowStoppedEventParams{
				AccountLookupContext: context.Background(),
				AccountID:            accountID,
				WorkflowRunID:        workflowRunID,
				WorkflowID:           workflowID,
				SequenceNumber:       sequenceNumber,
				Outputs:              allNodeOutputs,
				ErrorMessage:         "workflow stopped by user",
				ElapsedTime:          workflowElapsedTime,
				TotalTokens:          totalWorkflowTokens,
				TotalSteps:           nodeIndex - 1,
			})
			return
		default:
			// Continue execution
		}

		// Check if workflow has been stopped before processing each node
		if workflowService != nil {
			// Check if the workflow run status is stopped
			runInterface, err := workflowService.GetWorkflowRunByID(ctx, workflowRunID)
			if err == nil && runInterface != nil {
				if run, ok := runInterface.(*WorkflowRunLog); ok && run.Status == dto.WorkflowRunStatusStopped {
					logger.Info("Workflow has been stopped, sending workflow_finished event", "workflowRunID", workflowRunID)

					// Workflow execution will be stopped by returning from this function
					logger.Info("Stopping workflow execution")

					workflowElapsedTime := workflowElapsedTracker.elapsedOrFallback(ElapsedMillisecondsSince(workflowStartTime))
					h.sendWorkflowStoppedEvent(ctx, resultChan, workflowStoppedEventParams{
						AccountLookupContext: ctx,
						AccountID:            accountID,
						WorkflowRunID:        workflowRunID,
						WorkflowID:           workflowID,
						SequenceNumber:       sequenceNumber,
						Outputs:              allNodeOutputs,
						ErrorMessage:         "workflow stopped by user",
						ElapsedTime:          workflowElapsedTime,
						TotalTokens:          0,
						TotalSteps:           nodeIndex - 1,
					})
					return
				}
			}
		}

		node, exists := nodeMap[currentNodeID]
		if !exists {
			errorChan <- fmt.Errorf("node %s not found", currentNodeID)
			return
		}

		nodeData, ok := node["data"].(map[string]interface{})
		if !ok {
			errorChan <- fmt.Errorf("invalid node data for node %s", currentNodeID)
			return
		}

		nodeType, ok := nodeData["type"].(string)
		if !ok {
			errorChan <- fmt.Errorf("invalid node type for node %s", currentNodeID)
			return
		}

		title, _ := nodeData["title"].(string)
		if title == "" {
			title = fmt.Sprintf("Node %s", currentNodeID)
		}

		// Generate node execution ID
		nodeExecutionID := fmt.Sprintf("exec-%s-%d", currentNodeID, time.Now().UnixNano())

		// Prepare node inputs based on node type and configuration
		logger.Info("About to call getNodeInputs", "nodeID", currentNodeID, "nodeType", nodeType)
		nodeInputs := h.getNodeInputs(currentNodeID, nodeType, nodeData, systemInputs, req.Inputs, sharedVariablePool)
		logger.Info("getNodeInputs completed", "nodeID", currentNodeID, "inputsCount", len(nodeInputs))

		// Create workflow node runtime log for streaming execution
		var nodeLogID string
		if workflowService != nil {
			nodeLogInterface, err := workflowService.CreateWorkflowNodeRuntimeLog(ctx, workspaceID, appID, workflowID, "workflow-run", workflowRunID, currentNodeID, nodeType, title, nodeIndex, predecessorNodeID, nodeInputs, accountID)
			if err != nil {
				logger.ErrorContext(ctx, "failed to create workflow node runtime log", "node_id", currentNodeID, "node_type", nodeType, err)
				// Continue execution even if logging fails
			} else if nodeLog, ok := nodeLogInterface.(*WorkflowNodeRuntimeLog); ok {
				nodeLogID = nodeLog.ID
			}
		}

		// Record node start time for elapsed time calculation
		nodeStartTime := time.Now()

		resultChan <- buildWorkflowNodeStartedStreamEvent(workflowNodeStartedEventParams{
			NodeExecutionID:   nodeExecutionID,
			NodeID:            currentNodeID,
			NodeType:          nodeType,
			Title:             title,
			NodeIndex:         nodeIndex,
			PredecessorNodeID: predecessorNodeID,
			Inputs:            nodeInputs,
			CreatedAt:         nodeStartTime,
		})
		if answerCoordinator != nil && nodeType == "answer" {
			answerCoordinator.MarkAnswerActive(currentNodeID)
		}

		// Build proper node config with id and data fields
		nodeConfig := map[string]interface{}{
			"id":   currentNodeID,
			"data": nodeData,
		}

		streamCallback := newWorkflowStreamChunkCallback(
			ctx,
			resultChan,
			nodeMap,
			systemInputs,
			runType,
			watchedSelectors,
			streamWatchConfig,
			workflowRunID,
			conversationMessageChunkNodes,
			answerCoordinator,
		)
		iterationCallback := newWorkflowIterationCallback(ctx, resultChan, runtimeNodeMap)
		loopCallback := newWorkflowLoopCallback(ctx, resultChan, runtimeNodeMap)
		internalNodeCallback := newWorkflowInternalNodeCallback(ctx, resultChan, runtimeNodeMap, answerCoordinator)

		// Execute single node using executor with all callbacks
		nodeResult, err := executor.ExecuteWorkflowNodeWithAllCallbacks(
			ctx,
			currentNodeID,
			workflow_shared.NodeType(nodeType),
			nodeConfig,
			nodeInputs,
			sharedVariablePool,
			graphData,
			streamCallback,
			iterationCallback,
			loopCallback,
			internalNodeCallback,
		)

		// Handle node execution result
		var status string
		var outputs map[string]interface{}
		var nodeError any
		var nodeErrorMessage string

		if err != nil {
			// Check if the error is due to context cancellation (user stopped the workflow)
			// Check both errors.Is() for direct errors and string matching for wrapped errors
			errMsg := err.Error()
			isCanceled := errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
				strings.Contains(errMsg, "context canceled") || strings.Contains(errMsg, "context deadline exceeded")

			if isCanceled {
				// User canceled the workflow - set status to "stopped" with friendly message
				status = "stopped"
				errMsg := "Workflow canceled by user"
				nodeError = map[string]interface{}{"message": errMsg}
				nodeErrorMessage = errMsg
				outputs = make(map[string]interface{})

				logger.Info("Node execution canceled by user", "nodeID", currentNodeID)

				elapsedTime := ElapsedMillisecondsSince(nodeStartTime)
				finishedAt := time.Now()

				// Update workflow node runtime log with stopped status
				if nodeLogID != "" && workflowService != nil {
					updateErr := workflowService.UpdateWorkflowNodeRuntimeLog(ctx, nodeLogID, status, outputs, nil, nil, elapsedTime, errMsg)
					if updateErr != nil {
						logger.ErrorContext(ctx, "failed to update workflow node runtime log", "node_id", currentNodeID, "node_type", nodeType, updateErr)
					}
				}
				workflowElapsedTracker.recordNodeElapsed(nodeLogID, elapsedTime)

				resultChan <- buildWorkflowNodeFinishedStreamEvent(workflowNodeFinishedEventParams{
					NodeExecutionID:   nodeExecutionID,
					NodeID:            currentNodeID,
					NodeType:          nodeType,
					Title:             title,
					NodeIndex:         nodeIndex,
					PredecessorNodeID: predecessorNodeID,
					Inputs:            nodeInputs,
					ProcessData:       nil,
					Outputs:           outputs,
					Status:            status,
					Error:             nodeError,
					ElapsedTime:       elapsedTime,
					ExecutionMetadata: map[string]interface{}{},
					CreatedAt:         nodeStartTime,
					FinishedAt:        finishedAt,
				})
				if answerCoordinator != nil {
					answerCoordinator.MarkNodeFinished(currentNodeID, nodeType, status, outputs, err)
				}

				workflowElapsedTime := workflowElapsedTracker.elapsedOrFallback(ElapsedMillisecondsSince(workflowStartTime))
				h.sendWorkflowStoppedEvent(ctx, resultChan, workflowStoppedEventParams{
					AccountLookupContext: context.Background(),
					AccountID:            accountID,
					WorkflowRunID:        workflowRunID,
					WorkflowID:           workflowID,
					SequenceNumber:       sequenceNumber,
					Outputs:              allNodeOutputs,
					ErrorMessage:         "Workflow canceled by user",
					ElapsedTime:          workflowElapsedTime,
					TotalTokens:          totalWorkflowTokens,
					TotalSteps:           nodeIndex,
				})
				return
			}

			// Real error - set status to "failed"
			status = "failed"
			failedErrorPayload := buildWorkflowStreamErrorPayload(err)
			failedErrMsg := workflowStreamErrorMessage(failedErrorPayload)
			nodeError = failedErrorPayload
			nodeErrorMessage = failedErrMsg
			outputs = make(map[string]interface{})
			processData := map[string]interface{}{}
			executionMetadata := map[string]interface{}{}
			if nodeResult != nil {
				if nodeResult.Outputs != nil {
					outputs = nodeResult.Outputs
				}
				if nodeResult.ProcessData != nil {
					processData = nodeResult.ProcessData
				}
				executionMetadata = workflowExecutionMetadataToMap(nodeResult.Metadata)
			}

			// Record failed node for parallel branch handling
			failedNodes[currentNodeID] = failedErrMsg
			logger.CriticalContext(ctx, "node execution failed", "node_id", currentNodeID, "node_type", nodeType, err)

			// Report error to Sentry
			sentry.WithScope(func(scope *sentry.Scope) {
				scope.SetTag("node_id", currentNodeID)
				scope.SetTag("node_type", nodeType)
				scope.SetTag("workflow_id", workflowID)
				scope.SetExtra("node_inputs", nodeInputs)
				scope.SetExtra("workflow_run_id", workflowRunID)
				sentry.CaptureException(err)
			})

			elapsedTime := ElapsedMillisecondsSince(nodeStartTime)
			finishedAt := time.Now()

			// Update workflow node runtime log with failed status
			if nodeLogID != "" && workflowService != nil {
				updateErr := workflowService.UpdateWorkflowNodeRuntimeLog(ctx, nodeLogID, status, outputs, processData, executionMetadata, elapsedTime, failedErrMsg)
				if updateErr != nil {
					logger.ErrorContext(ctx, "failed to update workflow node runtime log", "node_id", currentNodeID, "node_type", nodeType, updateErr)
				}

				// Diagnostic Context Capture (Automatic on failure)
				if h.diagnoser != nil {
					// Build diag context
					diagErrCtx := &diagnosis.ErrorContext{
						NodeLogID:     nodeLogID,
						WorkflowID:    workflowID,
						WorkflowRunID: workflowRunID,
						NodeID:        currentNodeID,
						NodeType:      nodeType,
						NodeName:      title,
						ErrorMessage:  failedErrMsg,
						ErrorStack:    err.Error(), // Full Go error chain for diagnosis
						InputSnapshot: nodeInputs,
					}
					// Capture what we need to safely go async
					engine := executor.GetEngine(workflowRunID)
					predID := ""
					if predecessorNodeID != nil {
						predID = *predecessorNodeID
					}

					// Copy nodeMap and executionOutputs for async use (shallow copy is safe since values are read-only at this point)
					capturedNodeMap := nodeMap
					capturedExecOutputs := executionOutputs

					dc := &diagnosis.DiagnoseContext{
						ErrCtx:           diagErrCtx,
						Engine:           engine,
						PredecessorID:    predID,
						NodeMap:          capturedNodeMap,
						ExecutionOutputs: capturedExecOutputs,
					}

					go func() {
						bgCtx := context.Background()
						logger.Info("Capturing diagnostic context for node failure", "nodeLogID", diagErrCtx.NodeLogID)

						// Capture context snapshots without calling LLM
						res := h.diagnoser.ExtractResult(bgCtx, dc)
						logger.Info("Diagnostic context extracted",
							"nodeLogID", diagErrCtx.NodeLogID,
							"nodeYAMLLen", len(res.NodeYAML),
							"inputSnapshotLen", len(res.InputSnapshot),
							"upstreamYAMLLen", len(res.UpstreamYAML),
							"upstreamOutputsLen", len(res.UpstreamOutputs))

						// Update DB with context and error stack immediately
						logRepo := workflowService.workflowNodeRuntimeLogRepo
						if logRepo != nil {
							logger.Info("Attempting to persist diagnostic context to DB...", "nodeLogID", diagErrCtx.NodeLogID)
							err := logRepo.UpdateDiagnosisYaml(bgCtx, diagErrCtx.NodeLogID, string(diagErrCtx.ErrorType), diagErrCtx.ErrorStack, "", "none", 0, 0, false, res.NodeYAML, res.UpstreamYAML, res.InputSnapshot, res.UpstreamOutputs)
							if err != nil {
								logger.Error("Failed to persist node diagnostic context: %v", err)
							} else {
								logger.Info("Node diagnostic context persisted", "nodeLogID", diagErrCtx.NodeLogID)
							}
						} else {
							logger.Error("Failed to persist node diagnostic context: logRepo is nil")
						}
					}()
				}
			}
			workflowElapsedTracker.recordNodeElapsed(nodeLogID, elapsedTime)

			resultChan <- buildWorkflowNodeFinishedStreamEvent(workflowNodeFinishedEventParams{
				NodeExecutionID:   nodeExecutionID,
				NodeID:            currentNodeID,
				NodeType:          nodeType,
				Title:             title,
				NodeIndex:         nodeIndex,
				PredecessorNodeID: predecessorNodeID,
				Inputs:            nodeInputs,
				ProcessData:       processData,
				Outputs:           outputs,
				Status:            status,
				Error:             nodeError,
				ElapsedTime:       elapsedTime,
				ExecutionMetadata: executionMetadata,
				CreatedAt:         nodeStartTime,
				FinishedAt:        finishedAt,
			})
			if answerCoordinator != nil {
				answerCoordinator.MarkNodeFinished(currentNodeID, nodeType, status, outputs, err)
			}

			// Mark node as completed so upstream-dependency checks work correctly.
			completedNodes[currentNodeID] = true
			nodeIndex++

			// Node returned a hard error (no error strategy configured, or a real system
			// error). Terminate the entire workflow by pushing to errorChan.
			// The c.Stream errorChan case will send workflow_finished(failed) to the frontend.
			errorChan <- fmt.Errorf("node %s failed: %w", currentNodeID, err)
			return
		} else {
			// When err is nil, nodeResult should not be nil
			if nodeResult == nil {
				logger.CriticalContext(ctx, "node result is nil despite no error", "node_id", currentNodeID, "node_type", nodeType, fmt.Errorf("unexpected nil nodeResult"))
				return
			}

			status = string(nodeResult.Status)
			outputs = nodeResult.Outputs

			// Accumulate tokens from LLM nodes
			if nodeResult.LLMUsage != nil && nodeResult.LLMUsage.TotalTokens > 0 {
				totalWorkflowTokens += nodeResult.LLMUsage.TotalTokens
				logger.Info("Accumulated LLM tokens", "nodeID", currentNodeID, "nodeTokens", nodeResult.LLMUsage.TotalTokens, "totalWorkflowTokens", totalWorkflowTokens)
			}

			// Store outputs in executionOutputs for internal routing logic.
			// Also record EdgeSourceHandle (set by error-strategy / fail-branch) so
			// shouldRun can correctly activate the failure branch instead of the standard "source" branch.
			if outputs == nil {
				outputs = make(map[string]interface{})
			}
			if nodeResult.EdgeSourceHandle != "" {
				outputs["__edge_source_handle__"] = nodeResult.EdgeSourceHandle
			}
			executionOutputs[currentNodeID] = outputs
			addWorkflowStreamOutputsToVariablePool(sharedVariablePool, currentNodeID, outputs)

			// Populates the dedicated visual `error` field in the event payload
			// so that UI draws standard red error badges even when it's an intercepted EXCEPTION.
			if nodeResult.ErrMsg != "" {
				errMsgCopy := nodeResult.ErrMsg
				if nodeResult.ErrType != "" {
					errMsgCopy = fmt.Sprintf("[%s] %s", nodeResult.ErrType, errMsgCopy)
				}
				nodeError = map[string]interface{}{"message": errMsgCopy}
				nodeErrorMessage = errMsgCopy
			}

			allNodeOutputs = mergeWorkflowOutputsForNode(allNodeOutputs, nodeType, outputs)
		}

		if answerCoordinator != nil {
			answerCoordinator.MarkNodeFinished(currentNodeID, nodeType, status, outputs, nodeResult.Err)
			answerCoordinator.MarkSelectedHandleReachable(currentNodeID, status, nodeResult.EdgeSourceHandle)
		}

		if answerCoordinator == nil {
			if messageEvent := buildConversationAnswerMessageEvent(runType, workflowRunID, workflowConversationIDFromSystemInputs(systemInputs), currentNodeID, nodeType, outputs, conversationMessageChunkNodes[currentNodeID]); messageEvent != nil {
				resultChan <- messageEvent
				conversationMessageChunkNodes[currentNodeID] = true
			}
		}

		if nodeResult.Status == workflow_shared.PAUSED {
			responseMode := "streaming"
			if req != nil && req.ResponseMode != "" {
				responseMode = req.ResponseMode
			}
			handleWorkflowStreamPause(workflowStreamPauseParams{
				Ctx:                    ctx,
				WorkspaceID:            workspaceID,
				AppID:                  appID,
				WorkflowRunID:          workflowRunID,
				WorkflowID:             workflowID,
				RunType:                runType,
				TriggeredFrom:          triggeredFrom,
				CurrentNodeID:          currentNodeID,
				Title:                  title,
				NodeLogID:              nodeLogID,
				IsDraft:                isDraft,
				SequenceNumber:         sequenceNumber,
				NodeIndex:              nodeIndex,
				TotalWorkflowTokens:    totalWorkflowTokens,
				NodeType:               nodeType,
				Outputs:                outputs,
				ProcessData:            nodeResult.ProcessData,
				ExecutionMetadata:      workflowExecutionMetadataToMap(nodeResult.Metadata),
				AllNodeOutputs:         allNodeOutputs,
				NodeQueue:              nodeQueue,
				CompletedNodes:         completedNodes,
				FailedNodes:            failedNodes,
				ExecutionOutputs:       executionOutputs,
				PredecessorNodeID:      predecessorNodeID,
				RequestInputs:          req.Inputs,
				ResponseMode:           responseMode,
				SharedVariablePool:     sharedVariablePool,
				WorkflowService:        workflowService,
				WorkflowElapsedTracker: workflowElapsedTracker,
				ResultChan:             resultChan,
				NodeStartTime:          nodeStartTime,
				AnswerCoordinator:      answerCoordinator,
			})
			return
		}

		if nodeType == string(workflow_shared.Approval) {
			eventType, eventData := buildApprovalCompletionEvent(workflowRunID, currentNodeID, title, outputs, nodeResult.ProcessData)
			if eventType != "" && len(eventData) > 0 {
				resultChan <- &WorkflowStreamEvent{
					EventType: eventType,
					Data:      eventData,
				}
			}
		}

		elapsedTime := ElapsedMillisecondsSince(nodeStartTime)

		// Update workflow node runtime log with results.
		if nodeLogID != "" && workflowService != nil {
			errMsgForLog := nodeErrorMessage
			updateErr := workflowService.UpdateWorkflowNodeRuntimeLog(
				ctx,
				nodeLogID,
				status,
				outputs,
				nodeResult.ProcessData,
				workflowExecutionMetadataToMap(nodeResult.Metadata),
				elapsedTime,
				errMsgForLog,
			)
			if updateErr != nil {
				logger.ErrorContext(ctx, "failed to update workflow node runtime log", "node_id", currentNodeID, "node_type", nodeType, updateErr)
			}
		}
		workflowElapsedTracker.recordNodeElapsed(nodeLogID, elapsedTime)

		finishedAt := time.Now()

		resultChan <- buildWorkflowNodeFinishedStreamEvent(workflowNodeFinishedEventParams{
			NodeExecutionID:   nodeExecutionID,
			NodeID:            currentNodeID,
			NodeType:          nodeType,
			Title:             title,
			NodeIndex:         nodeIndex,
			PredecessorNodeID: predecessorNodeID,
			Inputs:            nodeInputs,
			ProcessData:       nodeResult.ProcessData,
			Outputs:           outputs,
			OutputHandle:      workflowOutputHandleFromOutputs(edgeMap, currentNodeID, status, outputs),
			Status:            status,
			Error:             nodeError,
			ElapsedTime:       elapsedTime,
			ExecutionMetadata: workflowExecutionMetadataToMap(nodeResult.Metadata),
			CreatedAt:         nodeStartTime,
			FinishedAt:        finishedAt,
		})

		// Move to next node based on EdgeSourceHandle from node result
		// Mark current node as completed
		completedNodes[currentNodeID] = true

		// Get EdgeSourceHandle from node execution result if available
		// Note: nodeResult is guaranteed to be non-nil at this point (checked in else branch above)
		edgeSourceHandle := "source" // default handle
		if nodeResult.EdgeSourceHandle != "" {
			edgeSourceHandle = nodeResult.EdgeSourceHandle
			logger.Info("Using EdgeSourceHandle from node result", "nodeID", currentNodeID, "edgeSourceHandle", edgeSourceHandle)
		}

		// Find next nodes based on current node and edge source handle
		// With parallel branches, there can be multiple next nodes
		// Queue all next nodes regardless of handle selection so they can be processed (and skipped if needed)
		nextNodeIDs := streamscheduler.EnqueueDownstreams(&nodeQueue, edgeMap, currentNodeID, completedNodes, nodePredecessors)
		logger.Info("Moving to next nodes", "currentNodeID", currentNodeID, "edgeSourceHandle", edgeSourceHandle, "nextNodeIDs", nextNodeIDs)
		nodeIndex++

		// Check for context cancellation after node transition
		select {
		case <-ctx.Done():
			logger.Info("Context canceled after node transition, stopping workflow", "error", ctx.Err())

			workflowElapsedTime := workflowElapsedTracker.elapsedOrFallback(ElapsedMillisecondsSince(workflowStartTime))
			h.sendWorkflowStoppedEvent(ctx, resultChan, workflowStoppedEventParams{
				AccountLookupContext: context.Background(),
				AccountID:            accountID,
				WorkflowRunID:        workflowRunID,
				WorkflowID:           workflowID,
				SequenceNumber:       sequenceNumber,
				Outputs:              allNodeOutputs,
				ErrorMessage:         "workflow stopped by user",
				ElapsedTime:          workflowElapsedTime,
				TotalTokens:          totalWorkflowTokens,
				TotalSteps:           nodeIndex,
			})
			return
		default:
			// Continue to next node
		}
	}

	finalizeWorkflowStreamExecution(workflowStreamFinalizeParams{
		Ctx:                    ctx,
		WorkflowRunID:          workflowRunID,
		WorkflowService:        workflowService,
		WorkflowElapsedTracker: workflowElapsedTracker,
		WorkflowStartTime:      workflowStartTime,
		FailedNodes:            failedNodes,
		AllNodeOutputs:         allNodeOutputs,
		TotalWorkflowTokens:    totalWorkflowTokens,
		NodeIndex:              nodeIndex,
		DoneChan:               doneChan,
		AnswerCoordinator:      answerCoordinator,
	})
}
