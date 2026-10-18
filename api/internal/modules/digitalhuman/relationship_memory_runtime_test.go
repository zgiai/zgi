package digitalhuman

import (
	"context"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/relationshipmemory"
)

type staticMemoryRuntimeProvider struct {
	preparation *MemoryRuntimePreparation
}

func TestRelationshipMemoryToolRejectsProactiveClearBeforeStorage(t *testing.T) {
	provider := &RelationshipMemoryRuntimeProvider{}
	tool := provider.mutationTool(
		&Run{Query: "Please remember that I prefer concise answers."},
		&relationshipmemory.UserMemoryRelationship{MemoryEnabled: true, AutoCaptureEnabled: true},
		relationshipmemory.ConsumerMemoryContract{Enabled: true, DefinitionMode: "dynamic", AllowedCategories: []string{"preference"}, AutoCaptureAllowed: true},
	)
	result := tool.Handler(t.Context(), adapter.ToolCall{Function: adapter.FunctionCall{
		Name:      userRelationshipMemoryToolName,
		Arguments: `{"action":"clear","mode":"proactive","key":"answer_style","category":"preference","evidence":"I prefer concise answers","expected_revision":1}`,
	}})
	if result.Status != "failed" || result.Result["error_code"] != relationshipmemory.ErrorCodePolicyRejected {
		t.Fatalf("proactive clear result = %#v", result)
	}
}

func (p staticMemoryRuntimeProvider) Prepare(context.Context, *Run, *ResolvedExecution, invocation.Event) (*MemoryRuntimePreparation, error) {
	return p.preparation, nil
}

func TestCompositeMemoryRuntimeProviderKeepsOwnedAndRelationshipMemorySeparate(t *testing.T) {
	owned := staticMemoryRuntimeProvider{preparation: &MemoryRuntimePreparation{
		SystemPromptAddendum: "digital-human-owned-memory",
		Tools: []skillloop.RuntimeTool{{Definition: adapter.Tool{
			Type:     "function",
			Function: adapter.Function{Name: digitalHumanMemoryToolName},
		}}},
	}}
	relationship := staticMemoryRuntimeProvider{preparation: &MemoryRuntimePreparation{
		SystemPromptAddendum: "user-relationship-memory",
		Tools: []skillloop.RuntimeTool{{Definition: adapter.Tool{
			Type:     "function",
			Function: adapter.Function{Name: userRelationshipMemoryToolName},
		}}},
	}}

	prepared, err := NewCompositeMemoryRuntimeProvider(owned, relationship).Prepare(
		context.Background(), &Run{}, &ResolvedExecution{}, invocation.Event{},
	)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if prepared.SystemPromptAddendum != "digital-human-owned-memory\n\nuser-relationship-memory" {
		t.Fatalf("SystemPromptAddendum = %q", prepared.SystemPromptAddendum)
	}
	if len(prepared.Tools) != 2 {
		t.Fatalf("len(Tools) = %d, want 2", len(prepared.Tools))
	}
	if got := prepared.Tools[0].Definition.Function.Name; got != digitalHumanMemoryToolName {
		t.Fatalf("first tool = %q, want %q", got, digitalHumanMemoryToolName)
	}
	if got := prepared.Tools[1].Definition.Function.Name; got != userRelationshipMemoryToolName {
		t.Fatalf("second tool = %q, want %q", got, userRelationshipMemoryToolName)
	}
}
