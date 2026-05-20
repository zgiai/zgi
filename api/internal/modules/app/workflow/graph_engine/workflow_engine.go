package graph_engine

import (
	"fmt"
	"sync"
	"time"

	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
)

// NodeEvent represents a node execution event for SSE streaming (used for internal nodes in subgraphs).
type NodeEvent struct {
	ExecutionID string
	Type        string                 `json:"type"` // "started", "finished"
	NodeID      string                 `json:"node_id"`
	NodeType    shared.NodeType        `json:"node_type"`
	Inputs      map[string]interface{} `json:"inputs,omitempty"`
	Outputs     map[string]interface{} `json:"outputs,omitempty"`
	Status      string                 `json:"status,omitempty"`
	Error       string                 `json:"error,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Timestamp   time.Time              `json:"timestamp"`
	StartedAt   time.Time              `json:"started_at,omitempty"`
	FinishedAt  time.Time              `json:"finished_at,omitempty"`
}

// IterationEvent represents an iteration-related event for SSE streaming
type IterationEvent struct {
	Type      string         `json:"type"` // "started", "next", "completed", "failed"
	NodeID    string         `json:"node_id"`
	Index     int            `json:"index,omitempty"`
	Inputs    map[string]any `json:"inputs,omitempty"`
	Outputs   map[string]any `json:"outputs,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Steps     int            `json:"steps,omitempty"`
	Error     string         `json:"error,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
	StartedAt time.Time      `json:"started_at,omitempty"`
}

// LoopEvent represents a loop-related event for SSE streaming
type LoopEvent struct {
	Type          string         `json:"type"` // "started", "next", "completed", "failed"
	NodeID        string         `json:"node_id"`
	Index         int            `json:"index,omitempty"`
	Inputs        map[string]any `json:"inputs,omitempty"`
	Outputs       map[string]any `json:"outputs,omitempty"`
	PreLoopOutput map[string]any `json:"pre_loop_output,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	Steps         int            `json:"steps,omitempty"`
	Error         string         `json:"error,omitempty"`
	Timestamp     time.Time      `json:"timestamp"`
	StartedAt     time.Time      `json:"started_at,omitempty"`
}

// ReadyBatchScope identifies the graph execution scope for a scheduler batch.
type ReadyBatchScope struct {
	Kind         string
	ParentNodeID string
	Index        int
}

const (
	ReadyBatchScopeTop       = "top"
	ReadyBatchScopeIteration = "iteration"
)

// WorkflowEngine is the workflow execution engine
type WorkflowEngine struct {
	// Configuration
	maxConcurrency int
	dontPanic      bool

	// State management
	steps         map[string]*NodeState
	statusMu      sync.RWMutex
	isRunning     bool
	isStopped     bool
	isPaused      bool
	pausedNodeIDs []string
	pausedNodeSet map[string]struct{}

	// Concurrency control
	leaseBucket  chan struct{}
	waitGroup    sync.WaitGroup
	statusChange *sync.Cond

	// Runtime state
	runtimeState *entities.GraphRuntimeState
	graph        *entities.Graph

	// Stream event callback for real-time forwarding
	streamEventCallback func(nodeID string, event *shared.RunStreamChunkEvent)

	// Iteration event callback for iteration node events
	iterationEventCallback func(event *IterationEvent)

	// Loop event callback for loop node events
	loopEventCallback func(event *LoopEvent)

	// Internal node event callback for nodes inside iterations
	internalNodeEventCallback func(event *NodeEvent)

	// Node event callbacks for real-time streaming during subgraph execution
	onNodeStarted          func(nodeID string, nodeType string, inputs map[string]any)
	onNodeFinished         func(nodeID string, nodeType string, status string, outputs map[string]any, edgeSourceHandle string, err error)
	onNodeFinishedDetailed func(event NodeFinishedEvent)
	onNodeSkipped          func(nodeID string, nodeType string)
	onReadyBatch           func(scope ReadyBatchScope, nodeIDs []string)
	readyScope             ReadyBatchScope
	nodeOrder              map[string]int

	// Node runner executes concrete workflow nodes outside of the graph engine.
	nodeRunner NodeRunner
}

// NodeState represents the state of a node
type NodeState struct {
	ID          string
	NodeType    shared.NodeType
	Status      shared.WorkflowNodeExecutionStatus
	Error       error
	Inputs      map[string]interface{}
	ProcessData map[string]interface{}
	Outputs     map[string]interface{}
	Metadata    map[shared.WorkflowNodeExecutionMetadataKey]any
	Config      map[string]interface{}

	// Dependencies
	Upstreams   map[string]bool
	Downstreams map[string]bool

	// UpstreamEdges maps upstream nodeID to the set of sourceHandles used for edges from that upstream.
	// This is needed for conditional branch routing (if-else nodes), and supports multiple handles
	// when different branches route into the same target node.
	UpstreamEdges map[string]map[string]struct{}

	// Execution information
	StartTime  time.Time
	EndTime    time.Time
	RetryCount int
	MaxRetries int

	// Edge routing for conditional branching
	EdgeSourceHandle string

	mu sync.RWMutex
}

// NodeFinishedEvent carries internal node completion details for run-scoped coordinators.
type NodeFinishedEvent struct {
	NodeID           string
	NodeType         string
	Status           string
	Outputs          map[string]any
	Err              error
	EdgeSourceHandle string
}

// NewWorkflowEngine creates a new workflow engine
func NewWorkflowEngine(maxConcurrency int) *WorkflowEngine {
	maxConcurrency = NormalizeMaxConcurrency(maxConcurrency)
	engine := &WorkflowEngine{
		maxConcurrency: maxConcurrency,
		dontPanic:      true,
		steps:          make(map[string]*NodeState),
		pausedNodeSet:  make(map[string]struct{}),
	}

	if maxConcurrency > 0 {
		engine.leaseBucket = make(chan struct{}, maxConcurrency)
	}

	engine.statusChange = sync.NewCond(&sync.Mutex{})
	return engine
}

// AddNode adds a node to the workflow
func (e *WorkflowEngine) AddNode(nodeID string, nodeType shared.NodeType, config map[string]interface{}) {
	e.statusMu.Lock()
	defer e.statusMu.Unlock()

	e.steps[nodeID] = &NodeState{
		ID:            nodeID,
		NodeType:      nodeType,
		Status:        shared.PENDING,
		Config:        config,
		Upstreams:     make(map[string]bool),
		Downstreams:   make(map[string]bool),
		UpstreamEdges: make(map[string]map[string]struct{}),
		MaxRetries:    3,
	}
}

// AddDependency adds a dependency relationship between nodes
func (e *WorkflowEngine) AddDependency(from, to string) error {
	return e.AddDependencyWithHandle(from, to, "source")
}

// AddDependencyWithHandle adds a dependency relationship between nodes with explicit sourceHandle.
// The sourceHandle is used for conditional branching (e.g., if-else nodes output "true"/"false" handles).
func (e *WorkflowEngine) AddDependencyWithHandle(from, to, sourceHandle string) error {
	e.statusMu.Lock()
	defer e.statusMu.Unlock()

	fromState, exists := e.steps[from]
	if !exists {
		return fmt.Errorf("source node %s not found", from)
	}

	toState, exists := e.steps[to]
	if !exists {
		return fmt.Errorf("target node %s not found", to)
	}

	fromState.Downstreams[to] = true
	toState.Upstreams[from] = true
	if sourceHandle == "" {
		sourceHandle = "source"
	}
	if _, ok := toState.UpstreamEdges[from]; !ok {
		toState.UpstreamEdges[from] = make(map[string]struct{})
	}
	toState.UpstreamEdges[from][sourceHandle] = struct{}{}

	return nil
}

// SetRuntimeState sets the runtime state
func (e *WorkflowEngine) SetRuntimeState(state *entities.GraphRuntimeState, graph *entities.Graph) {
	e.runtimeState = state
	e.graph = graph
	e.nodeOrder = graphNodeOrder(graph)
}

// GetRuntimeState gets the runtime state
func (e *WorkflowEngine) GetRuntimeState() *entities.GraphRuntimeState {
	return e.runtimeState
}

// GetGraph returns the parsed workflow graph
func (e *WorkflowEngine) GetGraph() *entities.Graph {
	return e.graph
}

// GetNodeResults returns all node execution states (thread-safe read)
func (e *WorkflowEngine) GetNodeResults() map[string]*NodeState {
	e.statusMu.RLock()
	defer e.statusMu.RUnlock()
	// Return a shallow copy to avoid caller races
	result := make(map[string]*NodeState, len(e.steps))
	for k, v := range e.steps {
		result[k] = v
	}
	return result
}

// SetStreamEventCallback sets the callback function for stream events
func (e *WorkflowEngine) SetStreamEventCallback(callback func(nodeID string, event *shared.RunStreamChunkEvent)) {
	e.streamEventCallback = callback
}

// SetIterationEventCallback sets the callback function for iteration events
func (e *WorkflowEngine) SetIterationEventCallback(callback func(event *IterationEvent)) {
	e.iterationEventCallback = callback
}

// SetLoopEventCallback sets the callback function for loop events
func (e *WorkflowEngine) SetLoopEventCallback(callback func(event *LoopEvent)) {
	e.loopEventCallback = callback
}

// SetInternalNodeEventCallback sets the callback function for internal node events (nodes inside iterations)
func (e *WorkflowEngine) SetInternalNodeEventCallback(callback func(event *NodeEvent)) {
	e.internalNodeEventCallback = callback
}

// SetNodeEventCallbacks sets callbacks for real-time node events during execution.
// These callbacks are invoked when nodes start and finish, enabling real-time event streaming.
func (e *WorkflowEngine) SetNodeEventCallbacks(
	onStarted func(nodeID string, nodeType string, inputs map[string]any),
	onFinished func(nodeID string, nodeType string, status string, outputs map[string]any, edgeSourceHandle string, err error),
) {
	e.onNodeStarted = onStarted
	e.onNodeFinished = onFinished
}

// SetDetailedNodeFinishedCallback sets a callback with internal edge-routing details.
func (e *WorkflowEngine) SetDetailedNodeFinishedCallback(onFinished func(event NodeFinishedEvent)) {
	e.onNodeFinishedDetailed = onFinished
}

// SetNodeSkippedCallback sets an internal callback for scheduler-skipped nodes.
func (e *WorkflowEngine) SetNodeSkippedCallback(onSkipped func(nodeID string, nodeType string)) {
	e.onNodeSkipped = onSkipped
}

// SetReadyBatchCallback sets the callback invoked before one scheduler batch starts.
func (e *WorkflowEngine) SetReadyBatchCallback(scope ReadyBatchScope, onReadyBatch func(scope ReadyBatchScope, nodeIDs []string)) {
	if scope.Kind == "" {
		scope.Kind = ReadyBatchScopeTop
	}
	e.readyScope = scope
	e.onReadyBatch = onReadyBatch
}

// SetNodeRunner sets the concrete node execution boundary.
func (e *WorkflowEngine) SetNodeRunner(runner NodeRunner) {
	e.nodeRunner = runner
}

func graphNodeOrder(graph *entities.Graph) map[string]int {
	order := make(map[string]int)
	if graph == nil || graph.Config == nil {
		return order
	}
	nodes, _ := graph.Config["nodes"].([]interface{})
	for i, raw := range nodes {
		node, _ := raw.(map[string]interface{})
		nodeID, _ := node["id"].(string)
		if nodeID != "" {
			order[nodeID] = i
		}
	}
	return order
}
