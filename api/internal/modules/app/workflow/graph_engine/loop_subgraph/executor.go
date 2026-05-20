package loop_subgraph

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine"
	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/subgraph"
	"github.com/zgiai/ginext/internal/modules/app/workflow/graphconfig"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
)

// Config carries inputs needed for a loop subgraph execution.
type Config struct {
	NodeID        string
	StartNodeID   *string
	GraphConfig   map[string]any
	RuntimeState  *entities.GraphRuntimeState
	Parallelism   int
	EventChan     chan *shared.NodeEventCh
	EngineFactory subgraph.EngineFactory
}

// Result captures the outcome of a loop subgraph run.
type Result struct {
	Outputs        map[string]any
	Tokens         int
	ReachedLoopEnd bool
}

// Executor executes loop subgraphs.
type Executor struct {
	cfg Config
}

// New creates a new Executor from config.
func New(cfg Config) *Executor {
	return &Executor{cfg: cfg}
}

// Run executes the loop subgraph for the provided loop index.
func (e *Executor) Run(ctx context.Context, index int) (Result, error) {
	if e.cfg.StartNodeID == nil || len(*e.cfg.StartNodeID) == 0 {
		return Result{}, fmt.Errorf("start_node_id is required")
	}
	if e.cfg.RuntimeState == nil || e.cfg.RuntimeState.VariablePool == nil {
		return Result{}, fmt.Errorf("graph runtime state not initialized")
	}

	engineFactory := e.cfg.EngineFactory
	if engineFactory == nil {
		return Result{}, fmt.Errorf("engine factory is required")
	}

	graph, nodeConfigs, edges, loopNodeIDs, err := e.buildLoopSubgraph(*e.cfg.StartNodeID)
	if err != nil {
		return Result{}, err
	}

	clearLoopVariables(e.cfg.RuntimeState.VariablePool, loopNodeIDs)

	engine := engineFactory(graph_engine.NormalizeMaxConcurrency(e.cfg.Parallelism))

	for nodeID, config := range nodeConfigs {
		nodeType, err := graphconfig.ExtractNodeType(config)
		if err != nil {
			return Result{}, fmt.Errorf("parse node type for %s: %w", nodeID, err)
		}
		engine.AddNode(nodeID, nodeType, config)
	}

	for _, edge := range edges {
		source, _ := edge["source"].(string)
		target, _ := edge["target"].(string)
		if source == "" || target == "" {
			continue
		}
		sourceHandle, _ := edge["sourceHandle"].(string)
		if sourceHandle == "" {
			sourceHandle = "source"
		}
		if err := engine.AddDependencyWithHandle(source, target, sourceHandle); err != nil {
			return Result{}, fmt.Errorf("add dependency %s -[%s]-> %s: %w", source, sourceHandle, target, err)
		}
	}

	reachedLoopEnd := false
	if e.cfg.EventChan != nil {
		nodeStartedAt := make(map[string]time.Time)
		var nodeTimeMu sync.Mutex

		engine.SetStreamEventCallback(func(nodeID string, streamEvent *shared.RunStreamChunkEvent) {
			if streamEvent == nil {
				return
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

		engine.SetNodeEventCallbacks(
			func(nodeID string, nodeType string, inputs map[string]any) {
				if nodeType == string(shared.LoopStart) {
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
								shared.LoopId:    e.cfg.NodeID,
								shared.LoopIndex: index,
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
			func(nodeID string, nodeType string, status string, outputs map[string]any, edgeSourceHandle string, err error) {
				if nodeType == string(shared.LoopStart) {
					return
				}
				if nodeType == string(shared.LoopEnd) && status == string(shared.SUCCEEDED) {
					reachedLoopEnd = true
				}
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
							Status:           shared.WorkflowNodeExecutionStatus(status),
							Outputs:          outputs,
							EdgeSourceHandle: edgeSourceHandle,
							ErrMsg:           errMsg,
							Metadata: map[shared.WorkflowNodeExecutionMetadataKey]any{
								shared.LoopId:    e.cfg.NodeID,
								shared.LoopIndex: index,
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
		VariablePool: e.cfg.RuntimeState.VariablePool,
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

	outputs := runtimeState.OutputsSnapshot()

	return Result{
		Outputs:        outputs,
		Tokens:         runtimeState.TotalTokenCount(),
		ReachedLoopEnd: reachedLoopEnd,
	}, nil
}

func clearLoopVariables(pool *entities.VariablePool, nodeIDs []string) {
	if pool == nil {
		return
	}
	for _, nodeID := range nodeIDs {
		pool.Remove([]string{nodeID})
	}
}

func (e *Executor) buildLoopSubgraph(startNodeID string) (*entities.Graph, map[string]map[string]any, []map[string]any, []string, error) {
	if e.cfg.GraphConfig == nil {
		return nil, nil, nil, nil, fmt.Errorf("graph config not available")
	}

	nodesRaw, ok := e.cfg.GraphConfig["nodes"].([]interface{})
	if !ok {
		return nil, nil, nil, nil, fmt.Errorf("graph config nodes missing or invalid")
	}

	nodeConfigs := make(map[string]map[string]any)
	loopNodeIDs := make([]string, 0)
	for _, raw := range nodesRaw {
		nodeMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		id, _ := nodeMap["id"].(string)
		if id == "" {
			continue
		}

		loopID := ""
		if dataMap := graphconfig.ToMap(nodeMap["data"]); dataMap != nil {
			if v, ok := dataMap["loop_id"].(string); ok {
				loopID = v
			}
		}
		if loopID == "" {
			if v, ok := nodeMap["parentId"].(string); ok {
				loopID = v
			}
		}

		if id == startNodeID || loopID == e.cfg.NodeID {
			nodeConfigs[id] = nodeMap
			loopNodeIDs = append(loopNodeIDs, id)
		}
	}

	if len(nodeConfigs) == 0 {
		return nil, nil, nil, nil, fmt.Errorf("loop subgraph empty for start node %s", startNodeID)
	}
	if _, ok := nodeConfigs[startNodeID]; !ok {
		return nil, nil, nil, nil, fmt.Errorf("start node %s not found in loop subgraph", startNodeID)
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
	// Nodes placed inside the loop container but without edges should not run.
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

	// Also filter loopNodeIDs to exclude unreachable nodes.
	reachableLoopNodeIDs := make([]string, 0, len(loopNodeIDs))
	for _, id := range loopNodeIDs {
		if reachable[id] {
			reachableLoopNodeIDs = append(reachableLoopNodeIDs, id)
		}
	}
	loopNodeIDs = reachableLoopNodeIDs

	nodesSlice := make([]interface{}, 0, len(nodeConfigs))
	graphNodes := make(map[string]interface{}, len(nodeConfigs))
	for id, cfg := range nodeConfigs {
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
		Type:     "loop-subgraph",
		Config:   configCopy,
		Nodes:    graphNodes,
		Edges:    edgesSlice,
		Metadata: map[string]interface{}{},
	}

	return graph, nodeConfigs, filteredEdges, loopNodeIDs, nil
}
