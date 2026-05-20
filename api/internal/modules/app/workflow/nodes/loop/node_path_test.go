package loop

import (
	"testing"

	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/base"
)

func TestNode_initLoopVariables_UsesNestedSelector(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"source", "payload"}, map[string]any{
		"value": "deep",
	})

	node := &Node{
		NodeStruct: base.NodeStruct{
			NodeID:            "loop-node-1",
			GraphRuntimeState: entities.NewGraphRuntimeState(vp),
		},
		nodeData: NodeData{
			LoopVariables: []LoopVariableSpec{
				{
					Label:     "item_name",
					VarType:   "string",
					ValueType: ValueTypeVariable,
					Value:     []string{"source", "payload", "value"},
				},
			},
		},
	}

	got, err := node.initLoopVariables()
	if err != nil {
		t.Fatalf("initLoopVariables() error = %v", err)
	}
	if got["item_name"] != "deep" {
		t.Fatalf("loop variable = %v, want deep", got["item_name"])
	}
}
