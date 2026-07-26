package end

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
)

func TestGenerateStreamOutputsWhenNodeFinished_UsesNestedSelector(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"answer", "payload"}, map[string]any{
		"text": "nested stream",
	})

	processor := &EndStreamProcessor{
		variablePool: vp,
		endStreamParam: &EndStreamParam{
			EndStreamVariableSelectorMapping: map[string][][]string{
				"end-1": {
					{"answer", "payload", "text"},
				},
			},
		},
		routePosition: map[string]int{"end-1": 0},
		outputNodeIDs: map[string]bool{},
		restNodeIDs:   map[string]bool{},
	}

	event := &NodeRunSucceededEvent{
		BaseNodeEvent: BaseNodeEvent{
			ID:             "evt-1",
			NodeID:         "end-1",
			RouteNodeState: &entities.RouteNodeState{NodeID: "end-1"},
		},
	}

	var events []GraphEngineEvent
	for current := range processor.generateStreamOutputsWhenNodeFinished(event) {
		events = append(events, current)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 stream event, got %d", len(events))
	}

	streamEvent, ok := events[0].(*NodeRunStreamChunkEvent)
	if !ok {
		t.Fatalf("expected NodeRunStreamChunkEvent, got %T", events[0])
	}
	if streamEvent.ChunkContent != "nested stream" {
		t.Fatalf("ChunkContent = %q, want %q", streamEvent.ChunkContent, "nested stream")
	}
}

func TestEndStreamGeneratorRouterIgnoresConstantOutputs(t *testing.T) {
	router := &EndStreamGeneratorRouter{}
	selectors := router.extractStreamVariableSelector(
		map[string]map[string]interface{}{
			"llm-1": {
				"data": map[string]interface{}{"type": "llm"},
			},
		},
		map[string]interface{}{
			"data": map[string]interface{}{
				"outputs": []interface{}{
					map[string]interface{}{
						"variable":   "fixed",
						"value_type": "constant",
						"value":      "done",
					},
					map[string]interface{}{
						"variable":       "generated",
						"value_type":     "variable",
						"value_selector": []interface{}{"llm-1", "text"},
					},
				},
			},
		},
	)

	if len(selectors) != 1 {
		t.Fatalf("selectors = %#v, want one model text selector", selectors)
	}
	if got := selectors[0]; len(got) != 2 || got[0] != "llm-1" || got[1] != "text" {
		t.Fatalf("selector = %#v, want [llm-1 text]", got)
	}
}
