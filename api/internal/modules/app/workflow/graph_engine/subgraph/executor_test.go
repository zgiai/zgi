package subgraph

import (
	"context"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
)

func TestApplyIterationMetadata(t *testing.T) {
	state := &entities.GraphRuntimeState{NodeRunState: entities.NewRuntimeRouteState()}
	state.NodeRunState.NodeStateMapping["node-success"] = &entities.RouteNodeState{
		NodeID:        "node-success",
		Status:        shared.RouteNodeStatusSuccess,
		Index:         2,
		NodeRunResult: &shared.NodeRunResult{},
	}
	state.NodeRunState.NodeStateMapping["node-failed"] = &entities.RouteNodeState{
		NodeID:        "node-failed",
		Status:        shared.RouteNodeStatusFailed,
		Index:         3,
		NodeRunResult: &shared.NodeRunResult{},
		FailedReason:  ptr("failure"),
	}

	applyIterationMetadata(state, "iter-node", 5)

	for key, nodeState := range state.NodeRunState.NodeStateMapping {
		if nodeState.NodeRunResult == nil {
			t.Fatalf("node %s missing run result metadata", key)
		}
		meta := nodeState.NodeRunResult.Metadata
		if meta == nil {
			t.Fatalf("node %s metadata not initialised", key)
		}
		if meta[shared.ITERATION_ID] != "iter-node" {
			t.Fatalf("node %s iteration id mismatch", key)
		}
		if meta[shared.IterationIndex] != 5 {
			t.Fatalf("node %s iteration index mismatch", key)
		}
	}
}

func TestEmitSubgraphEvents(t *testing.T) {
	state := &entities.GraphRuntimeState{NodeRunState: entities.NewRuntimeRouteState()}
	startedAt := time.Unix(1700000000, 100000000)
	failedFinishedAt := time.Unix(1700000001, 350000000)
	successFinishedAt := time.Unix(1700000002, 600000000)
	state.NodeRunState.NodeStateMapping["B"] = &entities.RouteNodeState{
		NodeID:        "B",
		Status:        shared.RouteNodeStatusSuccess,
		Index:         2,
		NodeRunResult: &shared.NodeRunResult{Status: shared.SUCCEEDED},
		StartAt:       startedAt,
		FinishedAt:    &successFinishedAt,
	}
	state.NodeRunState.NodeStateMapping["A"] = &entities.RouteNodeState{
		NodeID:        "A",
		Status:        shared.RouteNodeStatusFailed,
		Index:         1,
		NodeRunResult: &shared.NodeRunResult{Status: shared.FAILED, ErrMsg: "boom"},
		FailedReason:  ptr("boom"),
		StartAt:       startedAt,
		FinishedAt:    &failedFinishedAt,
	}

	eventChan := make(chan *shared.NodeEventCh, 8)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	applyIterationMetadata(state, "iter-x", 7)
	emitSubgraphEvents(ctx, eventChan, state, "iter-x", 7)
	close(eventChan)

	var events []*shared.NodeEventCh
	for evt := range eventChan {
		events = append(events, evt)
	}

	// Each node emits 2 events: node_started + node_finished = 4 total
	if len(events) != 4 {
		t.Fatalf("expected 4 events (started+finished for each node), got %d", len(events))
	}

	// Events are sorted by node Index, so A (index=1) comes before B (index=2)
	// Event 0: A started
	if events[0].NodeID != "A" || events[0].Type != shared.EventTypeInternalNodeStarted {
		t.Fatalf("expected first event to be node_started for node A, got %+v", events[0])
	}

	// Event 1: A finished
	if events[1].NodeID != "A" || events[1].Type != shared.EventTypeInternalNodeFinished {
		t.Fatalf("expected second event to be node_finished for node A, got %+v", events[1])
	}
	failedData, ok := events[1].Data.(*shared.RunCompletedEvent)
	if !ok {
		t.Fatalf("unexpected data type for finished event: %T", events[1].Data)
	}
	if failedData.RunResult.Metadata[shared.ITERATION_ID] != "iter-x" ||
		failedData.RunResult.Metadata[shared.IterationIndex] != 7 {
		t.Fatalf("iteration metadata missing on failed event")
	}
	if !failedData.StartedAt.Equal(startedAt) || !failedData.FinishedAt.Equal(failedFinishedAt) {
		t.Fatalf("failed event times = (%s, %s), want (%s, %s)", failedData.StartedAt, failedData.FinishedAt, startedAt, failedFinishedAt)
	}

	// Event 2: B started
	if events[2].NodeID != "B" || events[2].Type != shared.EventTypeInternalNodeStarted {
		t.Fatalf("expected third event to be node_started for node B, got %+v", events[2])
	}

	// Event 3: B finished
	if events[3].NodeID != "B" || events[3].Type != shared.EventTypeInternalNodeFinished {
		t.Fatalf("expected fourth event to be node_finished for node B, got %+v", events[3])
	}
	successData, ok := events[3].Data.(*shared.RunCompletedEvent)
	if !ok {
		t.Fatalf("unexpected data type for success event: %T", events[3].Data)
	}
	if successData.RunResult.Metadata[shared.ITERATION_ID] != "iter-x" ||
		successData.RunResult.Metadata[shared.IterationIndex] != 7 {
		t.Fatalf("iteration metadata missing on success event")
	}
	if !successData.StartedAt.Equal(startedAt) || !successData.FinishedAt.Equal(successFinishedAt) {
		t.Fatalf("success event times = (%s, %s), want (%s, %s)", successData.StartedAt, successData.FinishedAt, startedAt, successFinishedAt)
	}
}

type stubEngine struct {
	state *entities.GraphRuntimeState
}

func (s *stubEngine) AddNode(nodeID string, nodeType shared.NodeType, config map[string]any) {}

func (s *stubEngine) AddDependency(from, to string) error { return nil }

func (s *stubEngine) AddDependencyWithHandle(from, to, sourceHandle string) error { return nil }

func (s *stubEngine) SetRuntimeState(state *entities.GraphRuntimeState, graph *entities.Graph) {
	s.state = state
}

func (s *stubEngine) SetStreamEventCallback(func(nodeID string, event *shared.RunStreamChunkEvent)) {}

func (s *stubEngine) SetLLMClient(llmClient interface{}) {}

func (s *stubEngine) SetContentExtractor(contentExtractor interface{}) {}

func (s *stubEngine) SetToolEngine(toolEngine interface{}) {}

func (s *stubEngine) Execute(ctx context.Context) error {
	s.state.VariablePool.Add([]string{"answer", "payload"}, map[string]any{
		"text": "nested output",
	})
	return nil
}

func (s *stubEngine) SetNodeEventCallbacks(
	onStarted func(nodeID string, nodeType string, inputs map[string]any),
	onFinished func(nodeID string, nodeType string, status string, outputs map[string]any, edgeSourceHandle string, err error),
) {
}

func (s *stubEngine) SetReadyBatchCallback(scope graph_engine.ReadyBatchScope, onReadyBatch func(scope graph_engine.ReadyBatchScope, nodeIDs []string)) {
}

func TestExecutorRun_UsesNestedOutputSelector(t *testing.T) {
	startNodeID := "iter-start"
	executor := New(Config{
		NodeID:         "iter-1",
		StartNodeID:    &startNodeID,
		OutputSelector: []string{"answer", "payload", "text"},
		GraphConfig: map[string]any{
			"nodes": []any{
				map[string]any{
					"id": startNodeID,
					"data": map[string]any{
						"type": "start",
					},
				},
				map[string]any{
					"id":       "answer",
					"parentId": "iter-1",
					"data": map[string]any{
						"type": "answer",
					},
				},
			},
			"edges": []any{
				map[string]any{
					"source": startNodeID,
					"target": "answer",
				},
			},
		},
		RuntimeState:  entities.NewGraphRuntimeState(entities.NewVariablePool()),
		EngineFactory: func(parallelism int) Engine { return &stubEngine{} },
	})

	result, err := executor.Run(context.Background(), 0, "item")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.Output != "nested output" {
		t.Fatalf("result.Output = %#v, want %q", result.Output, "nested output")
	}
}

func ptr[T any](v T) *T {
	return &v
}
