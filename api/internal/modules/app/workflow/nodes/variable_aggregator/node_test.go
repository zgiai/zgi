package variableaggregator

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
)

func TestExecuteMultiGroupMode_MultiVariableGroupAddsNestedOutputWrapper(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"upstream", "text"}, "question text")
	vp.Add([]string{"upstream", "result"}, "answer result")

	node := &Node{
		NodeStruct: base.NodeStruct{
			NodeID:            "aggregator",
			NodeType:          shared.VariableAggregator,
			GraphRuntimeState: entities.NewGraphRuntimeState(vp),
		},
		NodeData: NodeData{
			AdvancedSettings: &AdvancedSettings{
				GroupEnabled: true,
				Groups: []Group{
					{
						GroupName:  "group1",
						OutputType: shared.SegmentTypeString,
						Variables: [][]string{
							{"upstream", "text"},
							{"upstream", "result"},
						},
					},
				},
			},
		},
	}

	outputs, _, _ := node.executeMultiGroupMode(vp)

	group1, ok := outputs["group1"].(map[string]any)
	if !ok {
		t.Fatalf("outputs[group1] type = %T, want map[string]any", outputs["group1"])
	}
	if len(group1) != 1 {
		t.Fatalf("len(group1) = %d, want 1; group1=%#v", len(group1), group1)
	}

	nestedOutput, ok := group1["output"].(map[string]any)
	if !ok {
		t.Fatalf("group1[output] type = %T, want map[string]any", group1["output"])
	}
	if got := nestedOutput["text"]; got != "question text" {
		t.Fatalf("group1[output][text] = %#v, want %#v", got, "question text")
	}
	if got := nestedOutput["result"]; got != "answer result" {
		t.Fatalf("group1[output][result] = %#v, want %#v", got, "answer result")
	}
}

func TestExecuteMultiGroupMode_NestedOutputWrapperResolvesViaVariablePool(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"upstream", "text"}, "question text")
	vp.Add([]string{"upstream", "result"}, "answer result")

	node := &Node{
		NodeStruct: base.NodeStruct{
			NodeID:            "aggregator",
			NodeType:          shared.VariableAggregator,
			GraphRuntimeState: entities.NewGraphRuntimeState(vp),
		},
		NodeData: NodeData{
			AdvancedSettings: &AdvancedSettings{
				GroupEnabled: true,
				Groups: []Group{
					{
						GroupName:  "group1",
						OutputType: shared.SegmentTypeString,
						Variables: [][]string{
							{"upstream", "text"},
							{"upstream", "result"},
						},
					},
				},
			},
		},
	}

	outputs, _, _ := node.executeMultiGroupMode(vp)
	vp.Add([]string{"aggregator", "group1"}, outputs["group1"])

	groupVar := vp.GetWithPath([]string{"aggregator", "group1"})
	if groupVar == nil {
		t.Fatal("GetWithPath(aggregator, group1) = nil, want value")
	}

	outputVar := vp.GetWithPath([]string{"aggregator", "group1", "output"})
	if outputVar == nil {
		t.Fatal("GetWithPath(aggregator, group1, output) = nil, want nested object")
	}

	nestedOutput, ok := outputVar.ToObject().(map[string]any)
	if !ok {
		t.Fatalf("outputVar.ToObject() type = %T, want map[string]any", outputVar.ToObject())
	}
	if got := nestedOutput["text"]; got != "question text" {
		t.Fatalf("nestedOutput[text] = %#v, want %#v", got, "question text")
	}
	if got := nestedOutput["result"]; got != "answer result" {
		t.Fatalf("nestedOutput[result] = %#v, want %#v", got, "answer result")
	}
}
