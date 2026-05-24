package variableaggregator

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
)

func TestExecuteSingleGroupMode_AggregatesOnlyValidVariables(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"fallback", "result"}, 3)

	node := &Node{
		NodeStruct: base.NodeStruct{
			NodeID:            "aggregator",
			NodeType:          shared.VariableAggregator,
			GraphRuntimeState: entities.NewGraphRuntimeState(vp),
		},
		NodeData: NodeData{
			OutputType: shared.SegmentTypeNumber,
			Variables: [][]string{
				{"missing", "foo"},
				{"fallback", "result"},
			},
		},
	}

	outputs, inputs, processData := node.executeSingleGroupMode(vp)

	output, ok := outputs["output"].(map[string]any)
	if !ok {
		t.Fatalf("outputs[output] type = %T, want map[string]any", outputs["output"])
	}
	if got := output["result"]; got != float64(3) {
		t.Fatalf("output[result] = %#v, want %#v", got, float64(3))
	}
	if _, ok := output["foo"]; ok {
		t.Fatalf("output[foo] exists, want missing; output=%#v", output)
	}
	if _, ok := inputs["fallback.result"]; !ok {
		t.Fatalf("inputs missing fallback.result: %#v", inputs)
	}
	if _, ok := inputs["missing.foo"]; ok {
		t.Fatalf("inputs contains missing.foo, want skipped: %#v", inputs)
	}
	if got := processData["mode"]; got != "aggregate_all" {
		t.Fatalf("mode = %#v, want aggregate_all", got)
	}
}

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
	vp.Add([]string{"upstream", "result"}, 5)

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
						OutputType: shared.SegmentTypeNumber,
						Variables: [][]string{
							{"missing", "foo"},
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
	if got := nestedOutput["result"]; got != float64(5) {
		t.Fatalf("nestedOutput[result] = %#v, want %#v", got, float64(5))
	}
	if _, ok := nestedOutput["foo"]; ok {
		t.Fatalf("nestedOutput[foo] exists, want skipped; nestedOutput=%#v", nestedOutput)
	}
}
