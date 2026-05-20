package varassign

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
)

func TestResolveVariableInput_UsesNestedSelector(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"source", "payload"}, map[string]any{
		"value": "deep",
	})

	value, wasNil, err := resolveVariableInput(vp, []string{"source", "payload", "value"}, OperationSet)
	if err != nil {
		t.Fatalf("resolveVariableInput() error = %v", err)
	}
	if wasNil {
		t.Fatalf("resolveVariableInput() returned wasNil=true, want false")
	}
	if got, ok := value.(string); !ok || got != "deep" {
		t.Fatalf("value = %#v, want deep", value)
	}
	if shared.SegmentTypeString == "" {
		t.Fatal("unreachable")
	}
}

type mockConversationVariablePersister struct {
	variables map[string]any
}

func (m *mockConversationVariablePersister) Save(_ context.Context, _, _ uuid.UUID, variables map[string]any) error {
	m.variables = variables
	return nil
}

func TestNode_ExecuteRun_UpdatesNestedObjectField(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"source", "profile"}, map[string]any{
		"name": "alice",
		"meta": map[string]any{
			"title": "old",
		},
	})

	node := &Node{
		NodeStruct: base.NodeStruct{
			GraphRuntimeState: entities.NewGraphRuntimeState(vp),
		},
		NodeData: NodeData{
			Items: []VariableOperationItem{
				{
					VariableSelector: []string{"source", "profile", "meta", "title"},
					InputType:        InputTypeConstant,
					Operation:        OperationOverWrite,
					Value:            "new",
				},
			},
		},
	}

	result, err := node.executeRun(context.Background(), nil)
	if err != nil {
		t.Fatalf("executeRun() error = %v", err)
	}

	profileVar := vp.Get([]string{"source", "profile"})
	if profileVar == nil {
		t.Fatal("expected top-level variable to remain in pool")
	}

	profile, ok := profileVar.ToObject().(map[string]any)
	if !ok {
		t.Fatalf("profile type = %T, want map[string]any", profileVar.ToObject())
	}

	meta, ok := profile["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta type = %T, want map[string]any", profile["meta"])
	}

	if got := meta["title"]; got != "new" {
		t.Fatalf("meta[title] = %#v, want %#v", got, "new")
	}
	if got := profile["name"]; got != "alice" {
		t.Fatalf("profile[name] = %#v, want %#v", got, "alice")
	}

	gotProcessData, ok := result.ProcessData["profile"].(map[string]any)
	if !ok {
		t.Fatalf("processData[profile] type = %T, want map[string]any", result.ProcessData["profile"])
	}
	gotProcessMeta, ok := gotProcessData["meta"].(map[string]any)
	if !ok {
		t.Fatalf("processData[profile].meta type = %T, want map[string]any", gotProcessData["meta"])
	}
	if got := gotProcessMeta["title"]; got != "new" {
		t.Fatalf("processData[profile].meta[title] = %#v, want %#v", got, "new")
	}

	updatedVariables, ok := result.ProcessData[updatedVariablesKey].([]map[string]any)
	if !ok {
		t.Fatalf("processData[%s] type = %T, want []map[string]any", updatedVariablesKey, result.ProcessData[updatedVariablesKey])
	}
	if len(updatedVariables) != 1 {
		t.Fatalf("updatedVariables length = %d, want 1", len(updatedVariables))
	}

	entry := updatedVariables[0]
	if got, ok := entry["selector"].([]string); !ok || len(got) != 4 || got[0] != "source" || got[1] != "profile" || got[2] != "meta" || got[3] != "title" {
		t.Fatalf("updated selector = %#v, want full nested selector", entry["selector"])
	}
	if got := entry["new_value"]; got != "new" {
		t.Fatalf("updated new_value = %#v, want %#v", got, "new")
	}
	if got := entry["value_type"]; got != string(shared.SegmentTypeString) {
		t.Fatalf("updated value_type = %#v, want %#v", got, string(shared.SegmentTypeString))
	}
}

func TestNode_ExecuteRun_InputsOnlyIncludeOperationItems(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"source", "profile"}, map[string]any{"name": "alice"})
	items := []VariableOperationItem{
		{
			VariableSelector: []string{"source", "profile", "name"},
			InputType:        InputTypeConstant,
			Operation:        OperationOverWrite,
			Value:            "Alice",
		},
	}

	node := &Node{
		NodeStruct: base.NodeStruct{
			GraphRuntimeState: entities.NewGraphRuntimeState(vp),
		},
		NodeData: NodeData{Items: items},
	}

	result, err := node.executeRun(context.Background(), nil)
	if err != nil {
		t.Fatalf("executeRun() error = %v", err)
	}

	if len(result.Inputs) != 1 {
		t.Fatalf("inputs len = %d, want 1: %#v", len(result.Inputs), result.Inputs)
	}
	if !reflect.DeepEqual(result.Inputs["items"], items) {
		t.Fatalf("inputs[items] = %#v, want operation items", result.Inputs["items"])
	}
	if _, exists := result.Inputs[updatedVariablesKey]; exists {
		t.Fatalf("%s should not be exposed in inputs: %#v", updatedVariablesKey, result.Inputs)
	}
}

func TestNode_ExecuteRun_AutoCreatesMissingNestedObjectPath(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"source", "profile"}, map[string]any{
		"name": "alice",
	})

	node := &Node{
		NodeStruct: base.NodeStruct{
			GraphRuntimeState: entities.NewGraphRuntimeState(vp),
		},
		NodeData: NodeData{
			Items: []VariableOperationItem{
				{
					VariableSelector: []string{"source", "profile", "meta", "title"},
					InputType:        InputTypeConstant,
					Operation:        OperationOverWrite,
					Value:            "new",
				},
			},
		},
	}

	result, err := node.executeRun(context.Background(), nil)
	if err != nil {
		t.Fatalf("executeRun() error = %v", err)
	}

	profileVar := vp.Get([]string{"source", "profile"})
	if profileVar == nil {
		t.Fatal("expected top-level variable to remain in pool")
	}

	profile, ok := profileVar.ToObject().(map[string]any)
	if !ok {
		t.Fatalf("profile type = %T, want map[string]any", profileVar.ToObject())
	}

	meta, ok := profile["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta type = %T, want map[string]any", profile["meta"])
	}

	if got := meta["title"]; got != "new" {
		t.Fatalf("meta[title] = %#v, want %#v", got, "new")
	}
	if got := profile["name"]; got != "alice" {
		t.Fatalf("profile[name] = %#v, want %#v", got, "alice")
	}

	gotProcessData, ok := result.ProcessData["profile"].(map[string]any)
	if !ok {
		t.Fatalf("processData[profile] type = %T, want map[string]any", result.ProcessData["profile"])
	}
	gotProcessMeta, ok := gotProcessData["meta"].(map[string]any)
	if !ok {
		t.Fatalf("processData[profile].meta type = %T, want map[string]any", gotProcessData["meta"])
	}
	if got := gotProcessMeta["title"]; got != "new" {
		t.Fatalf("processData[profile].meta[title] = %#v, want %#v", got, "new")
	}
}

func TestNode_ExecuteRun_ReturnsErrorForMissingTopLevelVariable(t *testing.T) {
	vp := entities.NewVariablePool()

	node := &Node{
		NodeStruct: base.NodeStruct{
			GraphRuntimeState: entities.NewGraphRuntimeState(vp),
		},
		NodeData: NodeData{
			Items: []VariableOperationItem{
				{
					VariableSelector: []string{"source", "profile", "meta", "title"},
					InputType:        InputTypeConstant,
					Operation:        OperationOverWrite,
					Value:            "new",
				},
			},
		},
	}

	_, err := node.executeRun(context.Background(), nil)
	if err == nil {
		t.Fatal("expected executeRun() to fail for missing top-level variable")
	}
	if got := err.Error(); got != "variable [source profile meta title] not found" {
		t.Fatalf("error = %q, want %q", got, "variable [source profile meta title] not found")
	}
}

func TestNode_ExecuteRun_ReturnsErrorWhenIntermediatePathIsNotObject(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"source", "profile"}, map[string]any{
		"meta": "not-object",
	})

	node := &Node{
		NodeStruct: base.NodeStruct{
			GraphRuntimeState: entities.NewGraphRuntimeState(vp),
		},
		NodeData: NodeData{
			Items: []VariableOperationItem{
				{
					VariableSelector: []string{"source", "profile", "meta", "title"},
					InputType:        InputTypeConstant,
					Operation:        OperationOverWrite,
					Value:            "new",
				},
			},
		},
	}

	_, err := node.executeRun(context.Background(), nil)
	if err == nil {
		t.Fatal("expected executeRun() to fail for non-object intermediate path")
	}
	if got := err.Error(); got != "nested path \"meta.title\" is not object" {
		t.Fatalf("error = %q, want %q", got, "nested path \"meta.title\" is not object")
	}
}

func TestNode_ExecuteRun_ReturnsErrorWhenIntermediatePathIsFile(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"source", "profile"}, map[string]any{
		"asset": map[string]any{
			"type":            "document",
			"id":              "file-1",
			"transfer_method": "local_file",
			"filename":        "old.pdf",
		},
	})

	node := &Node{
		NodeStruct: base.NodeStruct{
			GraphRuntimeState: entities.NewGraphRuntimeState(vp),
		},
		NodeData: NodeData{
			Items: []VariableOperationItem{
				{
					VariableSelector: []string{"source", "profile", "asset", "filename"},
					InputType:        InputTypeConstant,
					Operation:        OperationOverWrite,
					Value:            "new.pdf",
				},
			},
		},
	}

	_, err := node.executeRun(context.Background(), nil)
	if err == nil {
		t.Fatal("expected executeRun() to fail for file attribute path")
	}
	if got := err.Error(); got != "nested path \"asset.filename\" is not object" {
		t.Fatalf("error = %q, want %q", got, "nested path \"asset.filename\" is not object")
	}
}

func TestNode_ExecuteRun_PersistsTopLevelConversationVariableForNestedUpdate(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{entities.ConversationVariableNodeId, "profile"}, map[string]any{
		"name": "alice",
	})
	vp.SystemVariables.ConversationID = uuid.NewString()
	vp.SystemVariables.AppID = uuid.NewString()

	persister := &mockConversationVariablePersister{}
	node := &Node{
		NodeStruct: base.NodeStruct{
			GraphRuntimeState: entities.NewGraphRuntimeState(vp),
		},
		NodeData: NodeData{
			Items: []VariableOperationItem{
				{
					VariableSelector: []string{entities.ConversationVariableNodeId, "profile", "meta", "title"},
					InputType:        InputTypeConstant,
					Operation:        OperationOverWrite,
					Value:            "new",
				},
			},
		},
		conversationSaver: persister,
	}

	_, err := node.executeRun(context.Background(), nil)
	if err != nil {
		t.Fatalf("executeRun() error = %v", err)
	}

	savedProfile, ok := persister.variables["profile"].(map[string]any)
	if !ok {
		t.Fatalf("saved profile type = %T, want map[string]any", persister.variables["profile"])
	}
	savedMeta, ok := savedProfile["meta"].(map[string]any)
	if !ok {
		t.Fatalf("saved profile.meta type = %T, want map[string]any", savedProfile["meta"])
	}
	if got := savedMeta["title"]; got != "new" {
		t.Fatalf("saved profile.meta[title] = %#v, want %#v", got, "new")
	}
}
