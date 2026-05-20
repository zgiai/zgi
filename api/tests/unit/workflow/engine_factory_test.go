package workflow_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
)

type fakeNodeRunner struct{}

func (fakeNodeRunner) RunNode(ctx context.Context, req graph_engine.NodeRunRequest, eventChan chan<- *shared.NodeEventCh) (*shared.NodeRunResult, error) {
	return &shared.NodeRunResult{
		Status:  shared.SUCCEEDED,
		Inputs:  map[string]any{},
		Outputs: map[string]any{"answer": "ok"},
	}, nil
}

func TestEngineFactoryCreatesFreshEngines(t *testing.T) {
	factory := graph_engine.NewEngineFactory(1, fakeNodeRunner{})

	first := factory.New(graph_engine.EngineRunConfig{
		RuntimeState: entities.NewGraphRuntimeState(entities.NewVariablePool()),
		Graph:        &entities.Graph{Config: map[string]any{}},
	})
	second := factory.New(graph_engine.EngineRunConfig{
		RuntimeState: entities.NewGraphRuntimeState(entities.NewVariablePool()),
		Graph:        &entities.Graph{Config: map[string]any{}},
	})

	first.AddNode("answer", shared.Answer, map[string]any{"id": "answer"})

	_, exists := second.GetNodeStatus("answer")
	require.False(t, exists)
	require.NotSame(t, first.GetRuntimeState(), second.GetRuntimeState())
}

func TestEngineFactoryInstallsCallbacks(t *testing.T) {
	var started bool
	var finished bool
	factory := graph_engine.NewEngineFactory(1, fakeNodeRunner{})
	engine := factory.New(graph_engine.EngineRunConfig{
		RuntimeState: entities.NewGraphRuntimeState(entities.NewVariablePool()),
		Graph:        &entities.Graph{Config: map[string]any{}},
		Callbacks: graph_engine.EngineCallbacks{
			NodeStarted: func(nodeID string, nodeType string, inputs map[string]any) {
				started = nodeID == "answer" && nodeType == string(shared.Answer)
			},
			NodeFinished: func(nodeID string, nodeType string, status string, outputs map[string]any, edgeSourceHandle string, err error) {
				finished = nodeID == "answer" && nodeType == string(shared.Answer) && status == string(shared.SUCCEEDED) && err == nil
			},
		},
	})

	engine.AddNode("answer", shared.Answer, map[string]any{
		"id":   "answer",
		"data": map[string]any{"type": "answer", "answer": "ok"},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, engine.Execute(ctx))
	require.True(t, started)
	require.True(t, finished)
}

func TestLazyEngineFactoryResolvesDependenciesPerNew(t *testing.T) {
	resolved := 0
	factory := graph_engine.NewLazyEngineFactory(0, func() graph_engine.NodeRunner {
		resolved++
		return fakeNodeRunner{}
	})

	first := factory.New(graph_engine.EngineRunConfig{
		RuntimeState: entities.NewGraphRuntimeState(entities.NewVariablePool()),
		Graph:        &entities.Graph{Config: map[string]any{}},
	})
	second := factory.New(graph_engine.EngineRunConfig{
		RuntimeState: entities.NewGraphRuntimeState(entities.NewVariablePool()),
		Graph:        &entities.Graph{Config: map[string]any{}},
	})

	require.NotNil(t, first)
	require.NotNil(t, second)
	require.Equal(t, 2, resolved)
}

func TestNormalizeMaxConcurrency(t *testing.T) {
	require.Equal(t, graph_engine.DefaultMaxConcurrency, graph_engine.NormalizeMaxConcurrency(0))
	require.Equal(t, graph_engine.DefaultMaxConcurrency, graph_engine.NormalizeMaxConcurrency(-1))
	require.Equal(t, 7, graph_engine.NormalizeMaxConcurrency(7))
	require.Equal(t, graph_engine.MaxConcurrencyLimit, graph_engine.NormalizeMaxConcurrency(99))
}
