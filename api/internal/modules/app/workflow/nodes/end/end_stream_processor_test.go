package end

import (
	"testing"

	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
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
