package workflow

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/zgiai/ginext/internal/dto"
	"github.com/zgiai/ginext/internal/modules/app/workflow/diagnosis"
	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine"
	graph_entities "github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	workflow_shared "github.com/zgiai/ginext/internal/modules/app/workflow/shared"
	"github.com/zgiai/ginext/pkg/logger"
)

type workflowStreamGraphNodeState struct {
	NodeLogID   string
	NodeType    string
	Title       string
	NodeIndex   int
	Predecessor *string
	Inputs      map[string]interface{}
	StartedAt   time.Time
}

type workflowStreamGraphRunState struct {
	mu                       sync.Mutex
	nodeStates               map[string]workflowStreamGraphNodeState
	nodeIndex                int
	failedNodes              map[string]string
	allNodeOutputs           map[string]interface{}
	nodeOutputs              map[string]map[string]interface{}
	totalWorkflowTokens      int
	conversationMessageNodes map[string]bool
	systemInputs             map[string]interface{}
	requestInputs            map[string]interface{}
}

func (h *WorkflowHandler) executeWorkflowStreamGraphEngine(
	ctx context.Context,
	workspaceID string,
	appID string,
	req *dto.DraftWorkflowRunRequest,
	accountID string,
	workflowRunID string,
	workflowID string,
	systemInputs map[string]interface{},
	sequenceNumber int,
	resultChan chan<- *WorkflowStreamEvent,
	errorChan chan<- error,
	doneChan chan<- map[string]interface{},
	isDraft bool,
	runType string,
	triggeredFrom string,
	streamGraph *workflowStreamGraph,
	streamRuntime workflowStreamRuntime,
	workflowStartTime time.Time,
	answerCoordinator *answerOutputCoordinator,
) {
	if streamRuntime.Executor == nil {
		errorChan <- fmt.Errorf("workflow executor not available")
		return
	}

	var requestInputs map[string]interface{}
	if req != nil {
		requestInputs = req.Inputs
	}

	runState := &workflowStreamGraphRunState{
		nodeStates:               make(map[string]workflowStreamGraphNodeState),
		failedNodes:              make(map[string]string),
		allNodeOutputs:           make(map[string]interface{}),
		nodeOutputs:              make(map[string]map[string]interface{}),
		conversationMessageNodes: make(map[string]bool),
		systemInputs:             systemInputs,
		requestInputs:            requestInputs,
	}

	streamCallback := newWorkflowStreamChunkCallback(
		ctx,
		resultChan,
		streamGraph.NodeMap,
		systemInputs,
		runType,
		streamGraph.WatchConfig.watchedSelectors,
		streamGraph.WatchConfig,
		workflowRunID,
		runState.conversationMessageNodes,
		answerCoordinator,
	)

	callbacks := graph_engine.EngineCallbacks{
		Stream:       streamCallback,
		Iteration:    newWorkflowIterationCallback(ctx, resultChan, streamGraph.RuntimeNodeMap),
		Loop:         newWorkflowLoopCallback(ctx, resultChan, streamGraph.RuntimeNodeMap),
		InternalNode: newWorkflowInternalNodeCallback(ctx, resultChan, streamGraph.RuntimeNodeMap, answerCoordinator),
		NodeStarted: func(nodeID string, nodeType string, inputs map[string]any) {
			h.onWorkflowGraphStreamNodeStarted(ctx, workflowGraphStreamNodeStartedParams{
				WorkspaceID:       workspaceID,
				AppID:             appID,
				WorkflowID:        workflowID,
				WorkflowRunID:     workflowRunID,
				AccountID:         accountID,
				NodeID:            nodeID,
				NodeType:          nodeType,
				Inputs:            inputs,
				StreamGraph:       streamGraph,
				WorkflowService:   streamRuntime.WorkflowService,
				ResultChan:        resultChan,
				RunState:          runState,
				AnswerCoordinator: answerCoordinator,
			})
		},
		NodeFinished: func(nodeID string, nodeType string, status string, outputs map[string]any, edgeSourceHandle string, err error) {
			h.onWorkflowGraphStreamNodeFinished(ctx, workflowGraphStreamNodeFinishedParams{
				WorkflowRunID:          workflowRunID,
				NodeID:                 nodeID,
				NodeType:               nodeType,
				Status:                 status,
				Outputs:                outputs,
				EdgeSourceHandle:       edgeSourceHandle,
				Err:                    err,
				RunType:                runType,
				ResultChan:             resultChan,
				WorkflowService:        streamRuntime.WorkflowService,
				WorkflowElapsedTracker: streamRuntime.ElapsedTracker,
				StreamGraph:            streamGraph,
				RunState:               runState,
				AnswerCoordinator:      answerCoordinator,
				WorkflowID:             workflowID,
				Executor:               streamRuntime.Executor,
			})
		},
		NodeFinishedDetailed: func(event graph_engine.NodeFinishedEvent) {
			if answerCoordinator != nil {
				answerCoordinator.MarkSelectedHandleReachable(event.NodeID, event.Status, event.EdgeSourceHandle)
			}
		},
		NodeSkipped: func(nodeID string, nodeType string) {
			if answerCoordinator != nil {
				answerCoordinator.MarkNodeSkipped(nodeID)
			}
		},
		ReadyBatch: func(scope graph_engine.ReadyBatchScope, nodeIDs []string) {
			if answerCoordinator != nil {
				answerCoordinator.RegisterReadyBatch(answerScopeFromReadyBatch(scope), nodeIDs)
			}
		},
	}

	engineInputs := copyWorkflowAnyMap(requestInputs)
	for key, value := range systemInputs {
		engineInputs[key] = value
	}

	result, err := streamRuntime.Executor.ExecuteSimpleWorkflowWithRunIDAndCallbacks(ctx, workflowRunID, streamGraph.GraphData, engineInputs, callbacks)
	if err != nil {
		errorChan <- err
		return
	}
	if result != nil && result.Error != nil {
		errorChan <- result.Error
		return
	}

	if result != nil {
		if pausedSnapshots := workflowGraphPausedSnapshots(result.NodeExecutions); len(pausedSnapshots) > 0 {
			h.handleWorkflowGraphStreamPause(ctx, workflowGraphStreamPauseParams{
				WorkspaceID:            workspaceID,
				AppID:                  appID,
				WorkflowRunID:          workflowRunID,
				WorkflowID:             workflowID,
				SystemInputs:           systemInputs,
				SequenceNumber:         sequenceNumber,
				IsDraft:                isDraft,
				RunType:                runType,
				TriggeredFrom:          triggeredFrom,
				Request:                req,
				ResultChan:             resultChan,
				WorkflowService:        streamRuntime.WorkflowService,
				WorkflowElapsedTracker: streamRuntime.ElapsedTracker,
				StreamGraph:            streamGraph,
				RunState:               runState,
				ExecutionResult:        result,
				PausedSnapshots:        pausedSnapshots,
				AnswerCoordinator:      answerCoordinator,
			})
			return
		}
	}

	runState.mu.Lock()
	finalOutputs := copyWorkflowAnyMap(runState.allNodeOutputs)
	failedNodes := copyWorkflowStringMap(runState.failedNodes)
	totalTokens := runState.totalWorkflowTokens
	nodeIndex := runState.nodeIndex + 1
	runState.mu.Unlock()

	finalizeWorkflowStreamExecution(workflowStreamFinalizeParams{
		Ctx:                    ctx,
		WorkflowRunID:          workflowRunID,
		WorkflowService:        streamRuntime.WorkflowService,
		WorkflowElapsedTracker: streamRuntime.ElapsedTracker,
		WorkflowStartTime:      workflowStartTime,
		FailedNodes:            failedNodes,
		AllNodeOutputs:         finalOutputs,
		TotalWorkflowTokens:    totalTokens,
		NodeIndex:              nodeIndex,
		DoneChan:               doneChan,
		AnswerCoordinator:      answerCoordinator,
	})
}

type workflowGraphStreamNodeStartedParams struct {
	WorkspaceID       string
	AppID             string
	WorkflowID        string
	WorkflowRunID     string
	AccountID         string
	NodeID            string
	NodeType          string
	Inputs            map[string]any
	StreamGraph       *workflowStreamGraph
	WorkflowService   *WorkflowService
	ResultChan        chan<- *WorkflowStreamEvent
	RunState          *workflowStreamGraphRunState
	AnswerCoordinator *answerOutputCoordinator
}

func (h *WorkflowHandler) onWorkflowGraphStreamNodeStarted(ctx context.Context, params workflowGraphStreamNodeStartedParams) {
	startedAt := time.Now()
	title := workflowNodeTitle(params.StreamGraph.NodeMap, params.NodeID, fmt.Sprintf("Node %s", params.NodeID))
	predecessor := workflowStreamFirstPredecessor(params.StreamGraph.ReverseEdgeMap, params.NodeID)
	inputs := copyWorkflowAnyMap(params.Inputs)

	// The graph engine calls onNodeStarted before the node executes, so
	// state.Inputs is nil at this point. Hydrate from the variable pool
	// using getNodeInputs (mirrors the legacy executeWorkflowStream path).
	if len(inputs) == 0 {
		nodeData := workflowStreamNodeData(params.StreamGraph.NodeMap, params.NodeID)
		var variablePool *graph_entities.VariablePool
		if params.RunState != nil && params.RunState.systemInputs != nil {
			// Try to get the variable pool from the executor's active engine
			if ws, ok := h.workflowService.(*WorkflowService); ok {
				if executor := ws.GetExecutor(); executor != nil {
					if we, ok := executor.(*WorkflowExecutor); ok {
						if engine := we.GetEngine(params.WorkflowRunID); engine != nil {
							if rs := engine.GetRuntimeState(); rs != nil {
								variablePool = rs.VariablePool
							}
						}
					}
				}
			}
		}
		if variablePool != nil && nodeData != nil {
			inputs = h.getNodeInputs(params.NodeID, params.NodeType, nodeData, params.RunState.systemInputs, params.RunState.requestInputs, variablePool)
			logger.DebugContext(ctx, "hydrated node inputs from variable pool in graph engine path",
				"node_id", params.NodeID,
				"node_type", params.NodeType,
				"input_count", len(inputs),
			)
		}
	}

	params.RunState.mu.Lock()
	params.RunState.nodeIndex++
	nodeIndex := params.RunState.nodeIndex
	params.RunState.mu.Unlock()

	nodeLogID := ""
	if params.WorkflowService != nil {
		nodeLog, err := params.WorkflowService.CreateWorkflowNodeRuntimeLog(ctx, params.WorkspaceID, params.AppID, params.WorkflowID, "workflow-run", params.WorkflowRunID, params.NodeID, params.NodeType, title, nodeIndex, predecessor, inputs, params.AccountID)
		if err != nil {
			logger.ErrorContext(ctx, "failed to create workflow node runtime log", "node_id", params.NodeID, "node_type", params.NodeType, err)
		} else if typed, ok := nodeLog.(*WorkflowNodeRuntimeLog); ok {
			nodeLogID = typed.ID
		}
	}

	params.RunState.mu.Lock()
	params.RunState.nodeStates[params.NodeID] = workflowStreamGraphNodeState{
		NodeLogID:   nodeLogID,
		NodeType:    params.NodeType,
		Title:       title,
		NodeIndex:   nodeIndex,
		Predecessor: predecessor,
		Inputs:      inputs,
		StartedAt:   startedAt,
	}
	params.RunState.mu.Unlock()

	params.ResultChan <- buildWorkflowNodeStartedStreamEvent(workflowNodeStartedEventParams{
		NodeExecutionID:   fmt.Sprintf("exec-%s-%d", params.NodeID, startedAt.UnixNano()),
		NodeID:            params.NodeID,
		NodeType:          params.NodeType,
		Title:             title,
		NodeIndex:         nodeIndex,
		PredecessorNodeID: predecessor,
		Inputs:            inputs,
		CreatedAt:         startedAt,
	})
	if params.AnswerCoordinator != nil && params.NodeType == "answer" {
		params.AnswerCoordinator.MarkAnswerActive(params.NodeID)
	}
}

type workflowGraphStreamNodeFinishedParams struct {
	WorkflowRunID          string
	NodeID                 string
	NodeType               string
	Status                 string
	Outputs                map[string]any
	EdgeSourceHandle       string
	Err                    error
	RunType                string
	ResultChan             chan<- *WorkflowStreamEvent
	WorkflowService        *WorkflowService
	WorkflowElapsedTracker *workflowElapsedTracker
	StreamGraph            *workflowStreamGraph
	RunState               *workflowStreamGraphRunState
	AnswerCoordinator      *answerOutputCoordinator
	WorkflowID             string
	Executor               *WorkflowExecutor
}

func (h *WorkflowHandler) onWorkflowGraphStreamNodeFinished(ctx context.Context, params workflowGraphStreamNodeFinishedParams) {
	finishedAt := time.Now()

	params.RunState.mu.Lock()
	state := params.RunState.nodeStates[params.NodeID]
	params.RunState.mu.Unlock()
	if state.StartedAt.IsZero() {
		state = workflowStreamGraphNodeState{
			NodeType:  params.NodeType,
			Title:     workflowNodeTitle(nil, params.NodeID, fmt.Sprintf("Node %s", params.NodeID)),
			NodeIndex: 1,
			StartedAt: finishedAt,
		}
	}

	outputs := copyWorkflowAnyMap(params.Outputs)
	nodeError := workflowGraphStreamNodeError(params.Err)
	errorMessage := ""
	if params.Err != nil {
		errorMessage = params.Err.Error()
	}

	params.RunState.mu.Lock()
	params.RunState.allNodeOutputs = mergeWorkflowOutputsForNode(params.RunState.allNodeOutputs, params.NodeType, outputs)
	params.RunState.nodeOutputs[params.NodeID] = outputs
	if params.Status == string(workflow_shared.PAUSED) {
		params.RunState.mu.Unlock()
		return
	}
	if errorMessage != "" {
		params.RunState.failedNodes[params.NodeID] = errorMessage
	}
	if params.Status != "succeeded" && params.Status != "" && params.Status != "skipped" {
		if _, exists := params.RunState.failedNodes[params.NodeID]; !exists {
			params.RunState.failedNodes[params.NodeID] = params.Status
		}
	}
	params.RunState.mu.Unlock()

	elapsedTime := ElapsedMillisecondsSince(state.StartedAt)
	if state.NodeLogID != "" && params.WorkflowService != nil {
		err := params.WorkflowService.UpdateWorkflowNodeRuntimeLog(ctx, state.NodeLogID, params.Status, outputs, nil, nil, elapsedTime, errorMessage)
		if err != nil {
			logger.ErrorContext(ctx, "failed to update workflow node runtime log", "node_id", params.NodeID, "node_type", params.NodeType, err)
		}

		// Diagnostic Context Capture (Automatic on failure)
		isFailed := params.Status == "failed" || params.Status == "exception" || params.Err != nil
		if h.diagnoser != nil && isFailed {
			// Capture what we need to safely go async
			var engine *graph_engine.WorkflowEngine
			if params.Executor != nil {
				engine = params.Executor.GetEngine(params.WorkflowRunID)
			}

			// Try to recover the hydrated inputs directly from the engine state since onNodeFinished doesn't pass them
			inputSnapshot := state.Inputs
			logger.Info("Initial inputSnapshot from state", "nodeID", params.NodeID, "len", len(inputSnapshot))

			if engine != nil {
				results := engine.GetNodeResults()
				if engState, ok := results[params.NodeID]; ok && engState != nil && engState.Inputs != nil {
					inputSnapshot = engState.Inputs
					logger.Info("Recovered inputSnapshot from engine", "nodeID", params.NodeID, "len", len(engState.Inputs))
				} else {
					logger.Info("Failed to recover inputSnapshot from engine", "nodeID", params.NodeID, "ok", ok, "engStateNil", engState == nil)
				}
			} else {
				logger.Warn("Engine is nil, cannot recover inputSnapshot", "nodeID", params.NodeID)
			}

			// Fallback: if inputSnapshot is empty and node failed early, manually assemble it
			if len(inputSnapshot) == 0 && engine != nil {
				if nodeData, ok := params.StreamGraph.NodeMap[params.NodeID]; ok {
					inputSnapshot = h.getNodeInputs(
						params.NodeID,
						params.NodeType,
						nodeData,
						params.RunState.systemInputs,
						params.RunState.requestInputs,
						engine.GetRuntimeState().VariablePool,
					)
					logger.Info("Recovered inputSnapshot using getNodeInputs fallback", "nodeID", params.NodeID, "len", len(inputSnapshot))
				}
			}

			// Build diag context
			diagErrCtx := &diagnosis.ErrorContext{
				NodeLogID:     state.NodeLogID,
				WorkflowID:    params.WorkflowID,
				WorkflowRunID: params.WorkflowRunID,
				NodeID:        params.NodeID,
				NodeType:      params.NodeType,
				NodeName:      state.Title,
				ErrorMessage:  errorMessage,
				ErrorStack:    errorMessage, // Use error message as stack if full stack not available
				InputSnapshot: inputSnapshot,
			}
			if params.Err != nil {
				diagErrCtx.ErrorStack = params.Err.Error()
			}

			predID := ""
			if state.Predecessor != nil {
				predID = *state.Predecessor
			}

			dc := &diagnosis.DiagnoseContext{
				ErrCtx:           diagErrCtx,
				Engine:           engine,
				PredecessorID:    predID,
				NodeMap:          params.StreamGraph.NodeMap,
				ExecutionOutputs: params.RunState.nodeOutputs,
			}

			go func() {
				bgCtx := context.Background()
				logger.Info("Capturing diagnostic context for node failure (graph engine)", "nodeLogID", diagErrCtx.NodeLogID)

				// Capture context snapshots without calling LLM
				res := h.diagnoser.ExtractResult(bgCtx, dc)
				logger.Info("Diagnostic context extracted",
					"nodeLogID", diagErrCtx.NodeLogID,
					"nodeYAMLLen", len(res.NodeYAML),
					"inputSnapshotLen", len(res.InputSnapshot),
					"upstreamYAMLLen", len(res.UpstreamYAML),
					"upstreamOutputsLen", len(res.UpstreamOutputs))

				// Update DB with context and error stack immediately
				logRepo := params.WorkflowService.workflowNodeRuntimeLogRepo
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
	if params.WorkflowElapsedTracker != nil {
		params.WorkflowElapsedTracker.recordNodeElapsed(state.NodeLogID, elapsedTime)
	}

	params.ResultChan <- buildWorkflowNodeFinishedStreamEvent(workflowNodeFinishedEventParams{
		NodeExecutionID:   fmt.Sprintf("exec-%s-%d", params.NodeID, state.StartedAt.UnixNano()),
		NodeID:            params.NodeID,
		NodeType:          params.NodeType,
		Title:             state.Title,
		NodeIndex:         state.NodeIndex,
		PredecessorNodeID: state.Predecessor,
		Inputs:            state.Inputs,
		ProcessData:       nil,
		Outputs:           outputs,
		OutputHandle:      workflowOutputHandleForNode(workflowStreamGraphEdgeMap(params.StreamGraph), params.NodeID, params.Status, params.EdgeSourceHandle),
		Status:            params.Status,
		Error:             nodeError,
		ElapsedTime:       elapsedTime,
		ExecutionMetadata: map[string]interface{}{},
		CreatedAt:         state.StartedAt,
		FinishedAt:        finishedAt,
	})
	if params.AnswerCoordinator != nil {
		params.AnswerCoordinator.MarkNodeFinished(params.NodeID, params.NodeType, params.Status, outputs, params.Err)
	}
}

type workflowGraphStreamPauseParams struct {
	WorkspaceID            string
	AppID                  string
	WorkflowRunID          string
	WorkflowID             string
	SystemInputs           map[string]interface{}
	SequenceNumber         int
	IsDraft                bool
	RunType                string
	TriggeredFrom          string
	Request                *dto.DraftWorkflowRunRequest
	ResultChan             chan<- *WorkflowStreamEvent
	WorkflowService        *WorkflowService
	WorkflowElapsedTracker *workflowElapsedTracker
	StreamGraph            *workflowStreamGraph
	RunState               *workflowStreamGraphRunState
	ExecutionResult        *WorkflowExecutionResult
	PausedSnapshots        []graph_engine.NodeExecutionSnapshot
	AnswerCoordinator      *answerOutputCoordinator
}

func (h *WorkflowHandler) handleWorkflowGraphStreamPause(ctx context.Context, params workflowGraphStreamPauseParams) {
	params.RunState.mu.Lock()
	finalOutputs := copyWorkflowAnyMap(params.RunState.allNodeOutputs)
	failedNodes := copyWorkflowStringMap(params.RunState.failedNodes)
	totalTokens := params.RunState.totalWorkflowTokens
	params.RunState.mu.Unlock()

	pausedNodeIDs := make([]string, 0, len(params.PausedSnapshots))
	pausedNodes := make([]workflowStreamPausedNode, 0, len(params.PausedSnapshots))
	for _, snapshot := range params.PausedSnapshots {
		pausedNodeID := snapshot.NodeID
		if pausedNodeID == "" {
			continue
		}
		nodeType := string(snapshot.NodeType)
		if nodeType == "" {
			nodeType = workflowStreamNodeType(params.StreamGraph.NodeMap, pausedNodeID)
		}

		params.RunState.mu.Lock()
		nodeState := params.RunState.nodeStates[pausedNodeID]
		params.RunState.mu.Unlock()

		if nodeState.Title == "" {
			nodeState.Title = workflowNodeTitle(params.StreamGraph.NodeMap, pausedNodeID, fmt.Sprintf("Node %s", pausedNodeID))
		}
		if nodeState.NodeType == "" {
			nodeState.NodeType = nodeType
		}
		if nodeState.StartedAt.IsZero() {
			nodeState.StartedAt = snapshot.StartTime
		}
		if nodeState.StartedAt.IsZero() {
			nodeState.StartedAt = time.Now()
		}
		if nodeState.NodeIndex <= 0 {
			nodeState.NodeIndex = 1
		}
		if nodeState.Predecessor == nil {
			nodeState.Predecessor = workflowStreamFirstPredecessor(params.StreamGraph.ReverseEdgeMap, pausedNodeID)
		}

		pausedNodeIDs = append(pausedNodeIDs, pausedNodeID)
		pausedNodes = append(pausedNodes, workflowStreamPausedNode{
			NodeID:            pausedNodeID,
			Title:             nodeState.Title,
			NodeLogID:         nodeState.NodeLogID,
			NodeType:          nodeState.NodeType,
			NodeIndex:         nodeState.NodeIndex,
			Outputs:           copyWorkflowAnyMap(snapshot.Outputs),
			ProcessData:       copyWorkflowAnyMap(snapshot.ProcessData),
			ExecutionMetadata: copyWorkflowAnyMap(snapshot.Metadata),
			PredecessorNodeID: nodeState.Predecessor,
			NodeStartTime:     nodeState.StartedAt,
		})
	}
	if len(pausedNodes) == 0 {
		return
	}

	nodeQueue, completedNodes, snapshotFailedNodes, executionOutputs := workflowGraphPauseExecutorState(params.ExecutionResult.NodeExecutions, pausedNodeIDs)
	for nodeID, errMessage := range snapshotFailedNodes {
		if _, exists := failedNodes[nodeID]; !exists {
			failedNodes[nodeID] = errMessage
		}
	}

	responseMode := "streaming"
	requestInputs := map[string]interface{}{}
	if params.Request != nil {
		requestInputs = params.Request.Inputs
		if params.Request.ResponseMode != "" {
			responseMode = params.Request.ResponseMode
		}
	}

	var variablePool *graph_entities.VariablePool
	if params.ExecutionResult != nil && params.ExecutionResult.RuntimeState != nil {
		variablePool = params.ExecutionResult.RuntimeState.VariablePool
	}
	handleWorkflowStreamPause(workflowStreamPauseParams{
		Ctx:                    ctx,
		WorkspaceID:            params.WorkspaceID,
		AppID:                  params.AppID,
		WorkflowRunID:          params.WorkflowRunID,
		WorkflowID:             params.WorkflowID,
		RunType:                params.RunType,
		TriggeredFrom:          params.TriggeredFrom,
		CurrentNodeID:          pausedNodes[0].NodeID,
		Title:                  pausedNodes[0].Title,
		NodeLogID:              pausedNodes[0].NodeLogID,
		IsDraft:                params.IsDraft,
		SequenceNumber:         params.SequenceNumber,
		NodeIndex:              pausedNodes[0].NodeIndex,
		TotalWorkflowTokens:    totalTokens,
		NodeType:               pausedNodes[0].NodeType,
		Outputs:                copyWorkflowAnyMap(pausedNodes[0].Outputs),
		ProcessData:            copyWorkflowAnyMap(pausedNodes[0].ProcessData),
		ExecutionMetadata:      copyWorkflowAnyMap(pausedNodes[0].ExecutionMetadata),
		AllNodeOutputs:         finalOutputs,
		NodeQueue:              nodeQueue,
		CompletedNodes:         completedNodes,
		FailedNodes:            failedNodes,
		ExecutionOutputs:       executionOutputs,
		PredecessorNodeID:      pausedNodes[0].PredecessorNodeID,
		PausedNodes:            pausedNodes,
		RequestInputs:          requestInputs,
		ResponseMode:           responseMode,
		SharedVariablePool:     variablePool,
		WorkflowService:        params.WorkflowService,
		WorkflowElapsedTracker: params.WorkflowElapsedTracker,
		ResultChan:             params.ResultChan,
		NodeStartTime:          pausedNodes[0].NodeStartTime,
		AnswerCoordinator:      params.AnswerCoordinator,
	})
}

func workflowGraphPausedSnapshots(snapshots []graph_engine.NodeExecutionSnapshot) []graph_engine.NodeExecutionSnapshot {
	pausedSnapshots := make([]graph_engine.NodeExecutionSnapshot, 0)
	for _, snapshot := range snapshots {
		if snapshot.Status == workflow_shared.PAUSED {
			pausedSnapshots = append(pausedSnapshots, snapshot)
		}
	}
	sort.Slice(pausedSnapshots, func(i, j int) bool {
		left := pausedSnapshots[i]
		right := pausedSnapshots[j]
		if left.StartTime.Equal(right.StartTime) {
			return left.NodeID < right.NodeID
		}
		if left.StartTime.IsZero() {
			return false
		}
		if right.StartTime.IsZero() {
			return true
		}
		return left.StartTime.Before(right.StartTime)
	})
	return pausedSnapshots
}

func workflowGraphPauseExecutorState(snapshots []graph_engine.NodeExecutionSnapshot, pausedNodeIDs []string) ([]string, map[string]bool, map[string]string, map[string]map[string]interface{}) {
	nodeQueueSet := make(map[string]struct{})
	completedNodes := make(map[string]bool)
	failedNodes := make(map[string]string)
	executionOutputs := make(map[string]map[string]interface{})
	pausedNodeSet := make(map[string]struct{}, len(pausedNodeIDs))
	for _, nodeID := range pausedNodeIDs {
		pausedNodeSet[nodeID] = struct{}{}
	}

	for _, snapshot := range snapshots {
		if snapshot.NodeID == "" {
			continue
		}
		if outputs := workflowGraphSnapshotOutputs(snapshot); outputs != nil {
			executionOutputs[snapshot.NodeID] = outputs
		}

		switch snapshot.Status {
		case workflow_shared.PENDING, workflow_shared.RUNNING:
			if _, paused := pausedNodeSet[snapshot.NodeID]; !paused {
				nodeQueueSet[snapshot.NodeID] = struct{}{}
			}
		case workflow_shared.PAUSED:
			continue
		case workflow_shared.SUCCEEDED, workflow_shared.SKIPPED, workflow_shared.FAILED, workflow_shared.EXCEPTION:
			completedNodes[snapshot.NodeID] = true
			if snapshot.Status == workflow_shared.FAILED || snapshot.Status == workflow_shared.EXCEPTION {
				failedNodes[snapshot.NodeID] = workflowGraphSnapshotError(snapshot)
			}
		}
	}

	nodeQueue := make([]string, 0, len(nodeQueueSet))
	for nodeID := range nodeQueueSet {
		nodeQueue = append(nodeQueue, nodeID)
	}
	sort.Strings(nodeQueue)

	return nodeQueue, completedNodes, failedNodes, executionOutputs
}

func workflowGraphSnapshotOutputs(snapshot graph_engine.NodeExecutionSnapshot) map[string]interface{} {
	if snapshot.Outputs == nil {
		return nil
	}
	outputs := copyWorkflowAnyMap(snapshot.Outputs)
	if snapshot.EdgeSourceHandle != "" {
		outputs["__edge_source_handle__"] = snapshot.EdgeSourceHandle
	}
	return outputs
}

func workflowGraphSnapshotError(snapshot graph_engine.NodeExecutionSnapshot) string {
	if snapshot.Error != "" {
		return snapshot.Error
	}
	return string(snapshot.Status)
}

func workflowGraphStreamNodeError(err error) interface{} {
	if err == nil {
		return nil
	}
	return map[string]interface{}{"message": err.Error()}
}

func workflowStreamGraphEdgeMap(streamGraph *workflowStreamGraph) map[string]map[string][]string {
	if streamGraph == nil {
		return nil
	}
	return streamGraph.EdgeMap
}

func workflowStreamFirstPredecessor(reverseEdgeMap map[string][]string, nodeID string) *string {
	upstreams := reverseEdgeMap[nodeID]
	if len(upstreams) == 0 {
		return nil
	}
	predecessor := upstreams[0]
	return &predecessor
}
