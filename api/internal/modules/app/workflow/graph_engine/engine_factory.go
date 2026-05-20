package graph_engine

import (
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
)

const (
	DefaultMaxConcurrency = 10
	MaxConcurrencyLimit   = 15
)

// EngineCallbacks are run-scoped event callbacks.
type EngineCallbacks struct {
	Stream               func(nodeID string, event *shared.RunStreamChunkEvent)
	Iteration            func(event *IterationEvent)
	Loop                 func(event *LoopEvent)
	InternalNode         func(event *NodeEvent)
	NodeStarted          func(nodeID string, nodeType string, inputs map[string]any)
	NodeFinished         func(nodeID string, nodeType string, status string, outputs map[string]any, edgeSourceHandle string, err error)
	NodeFinishedDetailed func(event NodeFinishedEvent)
	NodeSkipped          func(nodeID string, nodeType string)
	ReadyBatch           func(scope ReadyBatchScope, nodeIDs []string)
}

// EngineRunConfig contains run-scoped state for a new engine instance.
type EngineRunConfig struct {
	RuntimeState   *entities.GraphRuntimeState
	Graph          *entities.Graph
	Callbacks      EngineCallbacks
	MaxConcurrency int
	ReadyScope     ReadyBatchScope
}

// EngineFactory creates fresh workflow engines while reusing application-scoped dependencies.
type EngineFactory struct {
	maxConcurrency int
	nodeRunner     NodeRunner
	runnerProvider func() NodeRunner
}

func NewEngineFactory(maxConcurrency int, runner NodeRunner) *EngineFactory {
	return &EngineFactory{
		maxConcurrency: NormalizeMaxConcurrency(maxConcurrency),
		nodeRunner:     runner,
	}
}

func NewLazyEngineFactory(maxConcurrency int, runnerProvider func() NodeRunner) *EngineFactory {
	return &EngineFactory{
		maxConcurrency: NormalizeMaxConcurrency(maxConcurrency),
		runnerProvider: runnerProvider,
	}
}

func (f *EngineFactory) New(config EngineRunConfig) *WorkflowEngine {
	maxConcurrency := DefaultMaxConcurrency
	var runner NodeRunner
	if f != nil {
		maxConcurrency = f.maxConcurrency
		if f.runnerProvider != nil {
			runner = f.runnerProvider()
		} else {
			runner = f.nodeRunner
		}
	}
	if config.MaxConcurrency > 0 {
		maxConcurrency = config.MaxConcurrency
	}
	maxConcurrency = NormalizeMaxConcurrency(maxConcurrency)

	engine := NewWorkflowEngine(maxConcurrency)
	engine.SetRuntimeState(config.RuntimeState, config.Graph)
	engine.SetNodeRunner(runner)
	engine.SetStreamEventCallback(config.Callbacks.Stream)
	engine.SetIterationEventCallback(config.Callbacks.Iteration)
	engine.SetLoopEventCallback(config.Callbacks.Loop)
	engine.SetInternalNodeEventCallback(config.Callbacks.InternalNode)
	engine.SetNodeEventCallbacks(config.Callbacks.NodeStarted, config.Callbacks.NodeFinished)
	engine.SetDetailedNodeFinishedCallback(config.Callbacks.NodeFinishedDetailed)
	engine.SetNodeSkippedCallback(config.Callbacks.NodeSkipped)
	engine.SetReadyBatchCallback(config.ReadyScope, config.Callbacks.ReadyBatch)

	return engine
}

func NormalizeMaxConcurrency(maxConcurrency int) int {
	if maxConcurrency <= 0 {
		return DefaultMaxConcurrency
	}
	if maxConcurrency > MaxConcurrencyLimit {
		return MaxConcurrencyLimit
	}
	return maxConcurrency
}
