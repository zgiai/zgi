package iter

import (
	"context"
	"testing"
	"time"

	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine"
	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/subgraph"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
)

func TestFetchIteratorValues_UsesNestedSelector(t *testing.T) {
	state := entities.NewGraphRuntimeState(entities.NewVariablePool())
	state.VariablePool.Add(
		[]string{"json-parser-node", "result"},
		map[string]any{
			"result": []any{
				map[string]any{"type": "选择", "number": 5},
				map[string]any{"type": "填空", "number": 5},
			},
		},
	)

	node := &Node{
		NodeStruct: base.NodeStruct{
			GraphRuntimeState: state,
		},
		nodeData: NodeData{
			IteratorSelector: []string{"json-parser-node", "result", "result"},
		},
	}

	values, rawValue, err := node.fetchIteratorValues()
	if err != nil {
		t.Fatalf("fetchIteratorValues() error = %v", err)
	}

	items, ok := rawValue.([]any)
	if !ok {
		t.Fatalf("rawValue type = %T, want []any", rawValue)
	}

	if len(values) != 2 {
		t.Fatalf("len(values) = %d, want 2", len(values))
	}

	if len(items) != 2 {
		t.Fatalf("len(rawValue) = %d, want 2", len(items))
	}
}

func TestExecuteRun_InputsIncludeSelectorAndIteratorValue(t *testing.T) {
	state := entities.NewGraphRuntimeState(entities.NewVariablePool())
	state.VariablePool.Add([]string{"start", "filelist"}, []any{"a", "b"})
	state.VariablePool.Add([]string{"answer", "text"}, "done")
	startNodeID := "iter-start"

	node := &Node{
		NodeStruct: base.NodeStruct{
			NodeID:            "iter-1",
			GraphRuntimeState: state,
			GraphConfig: map[string]any{
				"nodes": []any{
					map[string]any{
						"id": startNodeID,
						"data": map[string]any{
							"type": "iteration-start",
						},
					},
				},
			},
		},
		nodeData: NodeData{
			StartNodeID:      &startNodeID,
			IteratorSelector: []string{"start", "filelist"},
			OutputSelector:   []string{"answer", "text"},
		},
		engineFactory: func(parallelism int) subgraph.Engine {
			return &iterationInputStubEngine{}
		},
		now: time.Now,
	}

	eventChan := make(chan *shared.NodeEventCh, 8)
	result, err := node.executeRun(context.Background(), eventChan)
	if err != nil {
		t.Fatalf("executeRun() error = %v", err)
	}

	if got := result.Inputs["filelist"]; !sameAnySlice(got, []any{"a", "b"}) {
		t.Fatalf("filelist = %#v, want iterator value", got)
	}
	if got := result.Inputs["text"]; got != "done" {
		t.Fatalf("text = %#v, want output selector value", got)
	}
	for _, key := range []string{"iterator_selector", "iterator_value", "output_selector"} {
		if _, exists := result.Inputs[key]; exists {
			t.Fatalf("%s should be omitted from iteration inputs: %#v", key, result.Inputs)
		}
	}
}

type iterationInputStubEngine struct {
	state *entities.GraphRuntimeState
}

func (s *iterationInputStubEngine) AddNode(nodeID string, nodeType shared.NodeType, config map[string]any) {
}

func (s *iterationInputStubEngine) AddDependency(from, to string) error { return nil }

func (s *iterationInputStubEngine) AddDependencyWithHandle(from, to, sourceHandle string) error {
	return nil
}

func (s *iterationInputStubEngine) SetRuntimeState(state *entities.GraphRuntimeState, graph *entities.Graph) {
	s.state = state
}

func (s *iterationInputStubEngine) SetStreamEventCallback(func(nodeID string, event *shared.RunStreamChunkEvent)) {
}

func (s *iterationInputStubEngine) Execute(ctx context.Context) error {
	s.state.VariablePool.Add([]string{"answer", "text"}, "ok")
	return nil
}

func (s *iterationInputStubEngine) SetNodeEventCallbacks(
	onStarted func(nodeID string, nodeType string, inputs map[string]any),
	onFinished func(nodeID string, nodeType string, status string, outputs map[string]any, edgeSourceHandle string, err error),
) {
}

func (s *iterationInputStubEngine) SetReadyBatchCallback(scope graph_engine.ReadyBatchScope, onReadyBatch func(scope graph_engine.ReadyBatchScope, nodeIDs []string)) {
}

func sameStringSlice(value any, want []string) bool {
	got, ok := value.([]string)
	if !ok || len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func sameAnySlice(value any, want []any) bool {
	got, ok := value.([]any)
	if !ok || len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
