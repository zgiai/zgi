package jsonparser

import (
	"context"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
)

func TestNode_ExecuteRun_UsesNestedSelector(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"source", "payload"}, map[string]any{
		"raw": `{"name":"alice"}`,
	})

	node := &Node{
		NodeStruct: base.NodeStruct{
			NodeID:            "json-node-1",
			GraphRuntimeState: entities.NewGraphRuntimeState(vp),
		},
		NodeData: NodeData{
			InputSelector: []string{"source", "payload", "raw"},
			Outputs: map[string]Output{
				"parsed": {
					Type: shared.SegmentTypeObject,
					Children: map[string]*Output{
						"name": {Type: shared.SegmentTypeString},
					},
				},
			},
		},
	}

	result, err := node.executeRun(context.Background())
	if err != nil {
		t.Fatalf("executeRun() error = %v", err)
	}

	parsed, ok := result.Outputs["parsed"].(map[string]any)
	if !ok {
		t.Fatalf("parsed output type = %T, want map[string]any", result.Outputs["parsed"])
	}
	if parsed["name"] != "alice" {
		t.Fatalf("parsed name = %v, want alice", parsed["name"])
	}
}
