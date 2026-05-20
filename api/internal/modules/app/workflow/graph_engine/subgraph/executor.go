package subgraph

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graphconfig"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
)

// Engine defines minimal behaviour required to execute a subgraph.
type Engine interface {
	AddNode(nodeID string, nodeType shared.NodeType, config map[string]any)
	AddDependency(from, to string) error
	AddDependencyWithHandle(from, to, sourceHandle string) error
	SetRuntimeState(state *entities.GraphRuntimeState, graph *entities.Graph)
	SetStreamEventCallback(func(nodeID string, event *shared.RunStreamChunkEvent))
	Execute(ctx context.Context) error
	// SetNodeEventCallbacks sets callbacks for real-time node events during execution.
	// These callbacks are invoked when nodes start and finish, enabling real-time event streaming.
	SetNodeEventCallbacks(
		onStarted func(nodeID string, nodeType string, inputs map[string]any),
		onFinished func(nodeID string, nodeType string, status string, outputs map[string]any, edgeSourceHandle string, err error),
	)
	SetReadyBatchCallback(scope graph_engine.ReadyBatchScope, onReadyBatch func(scope graph_engine.ReadyBatchScope, nodeIDs []string))
}

// EngineFactory constructs an Engine for a subgraph run.
type EngineFactory func(parallelism int) Engine

// Config carries inputs needed for a subgraph execution.
type Config struct {
	NodeID         string
	StartNodeID    *string
	OutputSelector []string
	GraphConfig    map[string]any
	RuntimeState   *entities.GraphRuntimeState
	Parallelism    int
	EventChan      chan *shared.NodeEventCh
	EngineFactory  EngineFactory
}

// Result captures the outcome of a subgraph run.
type Result struct {
	Output               any
	Tokens               int
	ConversationSnapshot map[string]any
}

// Executor executes iteration subgraphs.
type Executor struct {
	cfg Config
}

// New creates a new Executor from config.
func New(cfg Config) *Executor {
	return &Executor{cfg: cfg}
}

// Run executes the subgraph for the provided iteration index/item.
func (e *Executor) Run(ctx context.Context, index int, item any) (Result, error) {
	if e.cfg.StartNodeID == nil || len(*e.cfg.StartNodeID) == 0 {
		return Result{}, fmt.Errorf("start_node_id is required")
	}
	if e.cfg.RuntimeState == nil {
		return Result{}, fmt.Errorf("graph runtime state not initialized")
	}

	engineFactory := e.cfg.EngineFactory
	if engineFactory == nil {
		return Result{}, fmt.Errorf("engine factory is required")
	}

	clonedPool, err := cloneVariablePool(e.cfg.RuntimeState.VariablePool)
	if err != nil {
		return Result{}, fmt.Errorf("failed to clone variable pool: %w", err)
	}

	clonedPool.Add([]string{e.cfg.NodeID, "index"}, index)
	clonedPool.Add([]string{e.cfg.NodeID, "item"}, item)

	graph, nodeConfigs, edges, err := e.buildIterationSubgraph(*e.cfg.StartNodeID)
	if err != nil {
		return Result{}, err
	}

	engine := engineFactory(graph_engine.NormalizeMaxConcurrency(e.cfg.Parallelism))

	for nodeID, config := range nodeConfigs {
		nodeType, err := graphconfig.ExtractNodeType(config)
		if err != nil {
			return Result{}, fmt.Errorf("failed to parse node type for %s: %w", nodeID, err)
		}
		engine.AddNode(nodeID, nodeType, config)
	}

	for _, edge := range edges {
		source, _ := edge["source"].(string)
		target, _ := edge["target"].(string)
		if source == "" || target == "" {
			continue
		}

		// Extract sourceHandle for conditional branch routing (if-else, question-classifier, etc.)
		sourceHandle, _ := edge["sourceHandle"].(string)
		if sourceHandle == "" {
			sourceHandle = "source" // Default handle for non-conditional edges
		}

		if err := engine.AddDependencyWithHandle(source, target, sourceHandle); err != nil {
			return Result{}, fmt.Errorf("failed to add dependency %s -[%s]-> %s: %w", source, sourceHandle, target, err)
		}
	}

	if e.cfg.EventChan != nil {
		nodeStartedAt := make(map[string]time.Time)
		var nodeTimeMu sync.Mutex

		engine.SetReadyBatchCallback(graph_engine.ReadyBatchScope{
			Kind:         graph_engine.ReadyBatchScopeIteration,
			ParentNodeID: e.cfg.NodeID,
			Index:        index,
		}, func(scope graph_engine.ReadyBatchScope, nodeIDs []string) {
			event := &shared.NodeEventCh{
				Type: shared.EventTypeInternalReadyBatch,
				Data: &shared.ReadyBatchEvent{
					ScopeKind:    scope.Kind,
					ParentNodeID: scope.ParentNodeID,
					Index:        scope.Index,
					NodeIDs:      append([]string(nil), nodeIDs...),
				},
				Timestamp: time.Now(),
			}
			select {
			case <-ctx.Done():
				return
			case e.cfg.EventChan <- event:
			}
		})

		engine.SetStreamEventCallback(func(nodeID string, streamEvent *shared.RunStreamChunkEvent) {
			if streamEvent == nil {
				return
			}
			streamEvent.Scope = &shared.RunStreamScope{
				Kind:         graph_engine.ReadyBatchScopeIteration,
				ParentNodeID: e.cfg.NodeID,
				Index:        index,
			}
			event := &shared.NodeEventCh{
				Type:      shared.EventTypeRunStreamChunk,
				NodeID:    nodeID,
				Data:      streamEvent,
				Timestamp: time.Now(),
			}
			select {
			case <-ctx.Done():
				return
			case e.cfg.EventChan <- event:
			}
		})

		// Set real-time node event callbacks for iteration subgraph
		// This replaces the batch emission that was causing HTTP/2 connection drops
		engine.SetNodeEventCallbacks(
			// onStarted - called when a node starts execution
			func(nodeID string, nodeType string, inputs map[string]any) {
				// Skip iteration-start nodes - they are internal helper nodes
				if strings.HasSuffix(nodeID, "start") && strings.HasPrefix(nodeID, e.cfg.NodeID) {
					return
				}

				startedAt := time.Now()
				nodeTimeMu.Lock()
				nodeStartedAt[nodeID] = startedAt
				nodeTimeMu.Unlock()

				event := &shared.NodeEventCh{
					Type:   shared.EventTypeInternalNodeStarted,
					NodeID: nodeID,
					Data: &shared.RunCompletedEvent{
						StartedAt: startedAt,
						RunResult: &shared.NodeRunResult{
							Status: shared.RUNNING,
							Inputs: inputs,
							Metadata: map[shared.WorkflowNodeExecutionMetadataKey]any{
								shared.ITERATION_ID:   e.cfg.NodeID,
								shared.IterationIndex: index,
								shared.IterationItem:  item,
							},
						},
					},
					Timestamp: startedAt,
				}
				select {
				case <-ctx.Done():
					return
				case e.cfg.EventChan <- event:
				}
			},
			// onFinished - called when a node finishes execution
			func(nodeID string, nodeType string, status string, outputs map[string]any, edgeSourceHandle string, err error) {
				// Skip iteration-start nodes - they are internal helper nodes
				if strings.HasSuffix(nodeID, "start") && strings.HasPrefix(nodeID, e.cfg.NodeID) {
					return
				}

				execStatus := shared.WorkflowNodeExecutionStatus(status)
				var errMsg string
				if err != nil {
					errMsg = err.Error()
				}

				finishedAt := time.Now()
				nodeTimeMu.Lock()
				startedAt := nodeStartedAt[nodeID]
				delete(nodeStartedAt, nodeID)
				nodeTimeMu.Unlock()
				if startedAt.IsZero() {
					startedAt = finishedAt
				}

				event := &shared.NodeEventCh{
					Type:   shared.EventTypeInternalNodeFinished,
					NodeID: nodeID,
					Data: &shared.RunCompletedEvent{
						StartedAt:  startedAt,
						FinishedAt: finishedAt,
						RunResult: &shared.NodeRunResult{
							Status:           execStatus,
							Outputs:          outputs,
							EdgeSourceHandle: edgeSourceHandle,
							ErrMsg:           errMsg,
							Metadata: map[shared.WorkflowNodeExecutionMetadataKey]any{
								shared.ITERATION_ID:   e.cfg.NodeID,
								shared.IterationIndex: index,
								shared.IterationItem:  item,
							},
						},
					},
					Error:     err,
					Timestamp: finishedAt,
				}
				select {
				case <-ctx.Done():
					return
				case e.cfg.EventChan <- event:
				}
			},
		)
	}

	runtimeState := &entities.GraphRuntimeState{
		VariablePool: clonedPool,
		StartAt:      float64(time.Now().UnixNano()) / 1e9,
		TotalTokens:  0,
		LLMUsage:     entities.EmptyUsage(),
		Outputs:      make(map[string]any),
		NodeRunSteps: 0,
		NodeRunState: entities.NewRuntimeRouteState(),
	}

	engine.SetRuntimeState(runtimeState, graph)

	if err := engine.Execute(ctx); err != nil {
		return Result{}, err
	}

	// Note: applyIterationMetadata is kept for consistency, but events are now sent in real-time
	// via the callbacks above, so emitSubgraphEvents is no longer needed
	applyIterationMetadata(runtimeState, e.cfg.NodeID, index)
	// Removed: emitSubgraphEvents - events are now sent in real-time via SetNodeEventCallbacks

	var output any
	if len(e.cfg.OutputSelector) > 0 {
		if variable := clonedPool.GetWithPath(e.cfg.OutputSelector); variable != nil {
			output = variable.ToObject()
		}
	}

	return Result{
		Output:               output,
		Tokens:               runtimeState.TotalTokenCount(),
		ConversationSnapshot: extractConversationSnapshot(clonedPool),
	}, nil
}

func (e *Executor) buildIterationSubgraph(startNodeID string) (*entities.Graph, map[string]map[string]any, []map[string]any, error) {
	if e.cfg.GraphConfig == nil {
		return nil, nil, nil, fmt.Errorf("graph config not available")
	}

	nodesRaw, ok := e.cfg.GraphConfig["nodes"].([]interface{})
	if !ok {
		return nil, nil, nil, fmt.Errorf("graph config nodes missing or invalid")
	}

	nodeConfigs := make(map[string]map[string]any)
	for _, raw := range nodesRaw {
		nodeMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		id, _ := nodeMap["id"].(string)
		if id == "" {
			continue
		}

		// Check iteration_id from data (legacy) and parentId from node (frontend format)
		iterationID := ""
		if dataMap := graphconfig.ToMap(nodeMap["data"]); dataMap != nil {
			if v, ok := dataMap["iteration_id"].(string); ok {
				iterationID = v
			}
		}
		// Also check parentId at node level (frontend uses this for iteration children)
		if iterationID == "" {
			if v, ok := nodeMap["parentId"].(string); ok {
				iterationID = v
			}
		}

		if id == startNodeID || iterationID == e.cfg.NodeID {
			nodeConfigs[id] = nodeMap
		}
	}

	if len(nodeConfigs) == 0 {
		return nil, nil, nil, fmt.Errorf("iteration subgraph empty for start node %s", startNodeID)
	}
	if _, ok := nodeConfigs[startNodeID]; !ok {
		return nil, nil, nil, fmt.Errorf("start node %s not found in iteration subgraph", startNodeID)
	}

	edgesRaw, _ := e.cfg.GraphConfig["edges"].([]interface{})
	candidateEdges := make([]map[string]any, 0)
	for _, raw := range edgesRaw {
		edgeMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		source, _ := edgeMap["source"].(string)
		target, _ := edgeMap["target"].(string)
		if source == "" || target == "" {
			continue
		}
		if _, ok := nodeConfigs[source]; !ok {
			continue
		}
		if _, ok := nodeConfigs[target]; !ok {
			continue
		}
		candidateEdges = append(candidateEdges, edgeMap)
	}

	// Remove nodes not reachable from the start node.
	// Nodes placed inside the iteration container but without edges should not run.
	reachable := graphconfig.ReachableFromStart(startNodeID, candidateEdges)
	for id := range nodeConfigs {
		if !reachable[id] {
			delete(nodeConfigs, id)
		}
	}

	// Re-filter edges to exclude edges involving removed nodes.
	filteredEdges := make([]map[string]any, 0, len(candidateEdges))
	for _, edge := range candidateEdges {
		source, _ := edge["source"].(string)
		target, _ := edge["target"].(string)
		if _, ok := nodeConfigs[source]; !ok {
			continue
		}
		if _, ok := nodeConfigs[target]; !ok {
			continue
		}
		filteredEdges = append(filteredEdges, edge)
	}

	nodesSlice := make([]interface{}, 0, len(nodeConfigs))
	graphNodes := make(map[string]interface{}, len(nodeConfigs))
	for _, raw := range nodesRaw {
		nodeMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		id, _ := nodeMap["id"].(string)
		cfg, exists := nodeConfigs[id]
		if !exists {
			continue
		}
		graphNodes[id] = cfg
		nodesSlice = append(nodesSlice, cfg)
	}

	edgesSlice := make([]interface{}, len(filteredEdges))
	for i, edge := range filteredEdges {
		edgesSlice[i] = edge
	}

	configCopy := graphconfig.Clone(e.cfg.GraphConfig)
	configCopy["nodes"] = nodesSlice
	configCopy["edges"] = edgesSlice
	configCopy["root_node_id"] = startNodeID

	graph := &entities.Graph{
		ID:       fmt.Sprintf("%s-subgraph", startNodeID),
		Type:     "iteration-subgraph",
		Config:   configCopy,
		Nodes:    graphNodes,
		Edges:    edgesSlice,
		Metadata: map[string]interface{}{},
	}

	return graph, nodeConfigs, filteredEdges, nil
}

func cloneVariablePool(original *entities.VariablePool) (*entities.VariablePool, error) {
	if original == nil {
		return entities.EmptyVariablePool(), nil
	}

	clone := entities.EmptyVariablePool()

	if original.SystemVariables != nil {
		systemCopy := *original.SystemVariables
		clone.SystemVariables = &systemCopy
	}

	for key, value := range original.UserInputs {
		clone.UserInputs[key] = value
	}

	if len(original.EnvironmentVariables) > 0 {
		copyEnv := make([]entities.Variable, len(original.EnvironmentVariables))
		copy(copyEnv, original.EnvironmentVariables)
		clone.EnvironmentVariables = copyEnv
	}
	if len(original.ConversationVariables) > 0 {
		copyConv := make([]entities.Variable, len(original.ConversationVariables))
		copy(copyConv, original.ConversationVariables)
		clone.ConversationVariables = copyConv
	}

	for nodeID, variableMap := range original.VariableDictionary {
		for name, variable := range variableMap {
			if variable == nil {
				continue
			}
			clone.Add([]string{nodeID, name}, variable.ToObject())
		}
	}

	return clone, nil
}

func applyIterationMetadata(state *entities.GraphRuntimeState, iterationID string, iterationIndex int) {
	if state == nil || state.NodeRunState == nil {
		return
	}

	for _, nodeState := range state.NodeRunState.NodeStateMapping {
		if nodeState == nil || nodeState.NodeRunResult == nil {
			continue
		}
		meta := nodeState.NodeRunResult.Metadata
		if meta == nil {
			meta = make(map[shared.WorkflowNodeExecutionMetadataKey]any)
		}

		if _, exists := meta[shared.ITERATION_ID]; !exists {
			meta[shared.ITERATION_ID] = iterationID
		}
		meta[shared.IterationIndex] = iterationIndex
		nodeState.NodeRunResult.Metadata = meta
	}
}

func emitSubgraphEvents(ctx context.Context, eventChan chan *shared.NodeEventCh, state *entities.GraphRuntimeState, iterationID string, iterationIndex int) {
	if eventChan == nil || state == nil || state.NodeRunState == nil {
		logger.DebugContext(ctx, "subgraph event emission skipped",
			zap.Bool("has_event_channel", eventChan != nil),
			zap.Bool("has_runtime_state", state != nil),
			zap.Bool("has_node_run_state", state != nil && state.NodeRunState != nil),
			zap.String("iteration_id", iterationID),
			zap.Int("iteration_index", iterationIndex),
		)
		return
	}

	logger.DebugContext(ctx, "subgraph event emission started",
		zap.Int("node_state_count", len(state.NodeRunState.NodeStateMapping)),
		zap.String("iteration_id", iterationID),
		zap.Int("iteration_index", iterationIndex),
	)

	nodeStates := make([]*entities.RouteNodeState, 0, len(state.NodeRunState.NodeStateMapping))
	for nodeID, ns := range state.NodeRunState.NodeStateMapping {
		if ns != nil {
			logger.DebugContext(ctx, "subgraph node state found",
				zap.String("node_id", nodeID),
				zap.String("node_status", string(ns.Status)),
				zap.String("iteration_id", iterationID),
				zap.Int("iteration_index", iterationIndex),
			)
			nodeStates = append(nodeStates, ns)
		}
	}

	sort.Slice(nodeStates, func(i, j int) bool {
		return nodeStates[i].Index < nodeStates[j].Index
	})

	for _, ns := range nodeStates {
		// Skip iteration-start nodes - they are internal helper nodes
		// Node ID format: {iterationID}start (e.g., "1765537934595start")
		if strings.HasSuffix(ns.NodeID, "start") && strings.HasPrefix(ns.NodeID, iterationID) {
			logger.DebugContext(ctx, "subgraph internal iteration-start node skipped",
				zap.String("node_id", ns.NodeID),
				zap.String("iteration_id", iterationID),
				zap.Int("iteration_index", iterationIndex),
			)
			continue
		}

		// Emit node_started event first
		startedEvent := buildInternalNodeStartedEvent(ns, iterationID, iterationIndex)
		if startedEvent != nil {
			logger.DebugContext(ctx, "subgraph node started event emitted",
				zap.String("node_id", ns.NodeID),
				zap.String("iteration_id", iterationID),
				zap.Int("iteration_index", iterationIndex),
			)
			select {
			case <-ctx.Done():
				return
			case eventChan <- startedEvent:
			}
		}

		// Then emit node_finished event
		finishedEvent := buildSubgraphEvent(ns, iterationID, iterationIndex)
		if finishedEvent != nil {
			logger.DebugContext(ctx, "subgraph node finished event emitted",
				zap.String("node_id", ns.NodeID),
				zap.String("node_status", string(ns.Status)),
				zap.String("iteration_id", iterationID),
				zap.Int("iteration_index", iterationIndex),
			)
			select {
			case <-ctx.Done():
				return
			case eventChan <- finishedEvent:
			}
		}
	}
}

func buildInternalNodeStartedEvent(ns *entities.RouteNodeState, iterationID string, iterationIndex int) *shared.NodeEventCh {
	if ns == nil {
		return nil
	}

	// Build metadata with iteration info
	meta := make(map[shared.WorkflowNodeExecutionMetadataKey]any)
	meta[shared.ITERATION_ID] = iterationID
	meta[shared.IterationIndex] = iterationIndex

	// Create a minimal run result for started event
	startedResult := &shared.NodeRunResult{
		Status:   shared.RUNNING,
		Metadata: meta,
	}
	if ns.NodeRunResult != nil {
		startedResult.Inputs = ns.NodeRunResult.Inputs
	}
	startedAt := ns.StartAt
	if startedAt.IsZero() {
		startedAt = time.Now()
	}

	return &shared.NodeEventCh{
		Type:      shared.EventTypeInternalNodeStarted,
		NodeID:    ns.NodeID,
		Data:      &shared.RunCompletedEvent{RunResult: startedResult, StartedAt: startedAt},
		Timestamp: startedAt,
	}
}

func buildSubgraphEvent(ns *entities.RouteNodeState, iterationID string, iterationIndex int) *shared.NodeEventCh {
	if ns == nil {
		return nil
	}

	if ns.NodeRunResult == nil {
		ns.NodeRunResult = &shared.NodeRunResult{Status: statusFromRoute(ns.Status)}
	}

	meta := ns.NodeRunResult.Metadata
	if meta == nil {
		meta = make(map[shared.WorkflowNodeExecutionMetadataKey]any)
	}
	if _, ok := meta[shared.ITERATION_ID]; !ok {
		meta[shared.ITERATION_ID] = iterationID
	}
	meta[shared.IterationIndex] = iterationIndex
	ns.NodeRunResult.Metadata = meta

	finishedAt := time.Now()
	if ns.FinishedAt != nil && !ns.FinishedAt.IsZero() {
		finishedAt = *ns.FinishedAt
	}
	startedAt := ns.StartAt
	if startedAt.IsZero() {
		startedAt = finishedAt
	}

	event := &shared.NodeEventCh{
		NodeID:    ns.NodeID,
		Timestamp: finishedAt,
	}
	runCompletedEvent := &shared.RunCompletedEvent{
		RunResult:  ns.NodeRunResult,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
	}

	switch ns.Status {
	case shared.RouteNodeStatusSuccess:
		event.Type = shared.EventTypeInternalNodeFinished
		event.Data = runCompletedEvent
	case shared.RouteNodeStatusFailed, shared.RouteNodeStatusException:
		event.Type = shared.EventTypeInternalNodeFinished
		event.Data = runCompletedEvent
		if ns.FailedReason != nil {
			event.Error = errors.New(*ns.FailedReason)
		}
	default:
		return nil
	}

	return event
}

func statusFromRoute(status shared.RouteNodeStatus) shared.WorkflowNodeExecutionStatus {
	switch status {
	case shared.RouteNodeStatusSuccess:
		return shared.SUCCEEDED
	case shared.RouteNodeStatusFailed:
		return shared.FAILED
	case shared.RouteNodeStatusException:
		return shared.EXCEPTION
	default:
		return shared.PENDING
	}
}

func extractConversationSnapshot(pool *entities.VariablePool) map[string]any {
	if pool == nil {
		return nil
	}
	conversations := pool.VariableDictionary[entities.ConversationVariableNodeId]
	if conversations == nil {
		return nil
	}

	snapshot := make(map[string]any, len(conversations))
	for name, variable := range conversations {
		if variable == nil {
			continue
		}
		snapshot[name] = variable.ToObject()
	}
	return snapshot
}
