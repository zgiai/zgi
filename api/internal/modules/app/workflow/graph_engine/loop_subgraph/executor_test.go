package loop_subgraph

import (
	"context"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/subgraph"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
)

type scopeCapturingEngine struct {
	state          *entities.GraphRuntimeState
	streamCallback func(string, *shared.RunStreamChunkEvent)
	readyScope     graph_engine.ReadyBatchScope
	readyCallback  func(graph_engine.ReadyBatchScope, []string)
}

func (s *scopeCapturingEngine) AddNode(string, shared.NodeType, map[string]any) {}
func (s *scopeCapturingEngine) AddDependency(string, string) error              { return nil }
func (s *scopeCapturingEngine) AddDependencyWithHandle(string, string, string) error {
	return nil
}
func (s *scopeCapturingEngine) SetRuntimeState(state *entities.GraphRuntimeState, _ *entities.Graph) {
	s.state = state
}
func (s *scopeCapturingEngine) SetStreamEventCallback(callback func(string, *shared.RunStreamChunkEvent)) {
	s.streamCallback = callback
}
func (s *scopeCapturingEngine) SetNodeEventCallbacks(
	func(string, string, map[string]any),
	func(string, string, string, map[string]any, string, error),
) {
}
func (s *scopeCapturingEngine) SetReadyBatchCallback(scope graph_engine.ReadyBatchScope, callback func(graph_engine.ReadyBatchScope, []string)) {
	s.readyScope = scope
	s.readyCallback = callback
}
func (s *scopeCapturingEngine) Execute(context.Context) error {
	s.readyCallback(s.readyScope, []string{"answer"})
	s.streamCallback("llm", &shared.RunStreamChunkEvent{
		ChunkContent:         "round output",
		FromVariableSelector: []string{"llm", "text"},
	})
	return nil
}

func TestExecutorRunTagsReadyBatchAndStreamWithLoopScope(t *testing.T) {
	startNodeID := "loop-start"
	eventChan := make(chan *shared.NodeEventCh, 4)
	engine := &scopeCapturingEngine{}
	executor := New(Config{
		NodeID:       "loop-1",
		StartNodeID:  &startNodeID,
		RuntimeState: entities.NewGraphRuntimeState(entities.NewVariablePool()),
		EventChan:    eventChan,
		EngineFactory: func(int) subgraph.Engine {
			return engine
		},
		GraphConfig: map[string]any{
			"nodes": []interface{}{
				map[string]any{"id": startNodeID, "parentId": "loop-1", "data": map[string]any{"type": "loop-start"}},
				map[string]any{"id": "answer", "parentId": "loop-1", "data": map[string]any{"type": "answer"}},
			},
			"edges": []interface{}{
				map[string]any{"source": startNodeID, "target": "answer"},
			},
		},
	})

	if _, err := executor.Run(context.Background(), 2); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	readyEvent := <-eventChan
	ready, ok := readyEvent.Data.(*shared.ReadyBatchEvent)
	if !ok {
		t.Fatalf("ready event data = %T, want *shared.ReadyBatchEvent", readyEvent.Data)
	}
	if ready.ScopeKind != graph_engine.ReadyBatchScopeLoop || ready.ParentNodeID != "loop-1" || ready.Index != 2 {
		t.Fatalf("ready scope = %#v, want loop-1 round 2", ready)
	}

	streamEvent := <-eventChan
	chunk, ok := streamEvent.Data.(*shared.RunStreamChunkEvent)
	if !ok || chunk.Scope == nil {
		t.Fatalf("stream event data = %#v, want scoped chunk", streamEvent.Data)
	}
	if chunk.Scope.Kind != graph_engine.ReadyBatchScopeLoop || chunk.Scope.ParentNodeID != "loop-1" || chunk.Scope.Index != 2 {
		t.Fatalf("stream scope = %#v, want loop-1 round 2", chunk.Scope)
	}
}
