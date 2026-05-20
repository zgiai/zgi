package knowledgeretrieval

import (
	"context"
	"testing"

	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
)

func TestNode_ExecuteRun_UsesNestedQuerySelector(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"source", "payload"}, map[string]any{
		"value": "",
	})

	node := &Node{
		NodeStruct: base.NodeStruct{
			NodeID:            "kr-node-1",
			GraphRuntimeState: entities.NewGraphRuntimeState(vp),
		},
		NodeData: NodeData{
			QueryVariableSelector: []string{"source", "payload", "value"},
		},
	}

	result, err := node.executeRun(context.Background(), nil)
	if err != nil {
		t.Fatalf("executeRun() error = %v", err)
	}

	if result.Status != shared.FAILED {
		t.Fatalf("status = %v, want FAILED", result.Status)
	}
	if result.ErrMsg != "Query is required." {
		t.Fatalf("err msg = %q, want %q", result.ErrMsg, "Query is required.")
	}
}
