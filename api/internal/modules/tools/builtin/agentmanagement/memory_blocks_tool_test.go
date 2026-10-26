package agentmanagement

import "testing"

// ADR 0016 renamed the tool; the former name still resolves but is not listed.
func TestReplaceAgentMemoryBlocksKeepsTheSlotsNameAsAlias(t *testing.T) {
	provider := NewProvider(nil, nil, nil)
	canonical, err := provider.GetTool(ToolReplaceAgentMemoryBlocks)
	if err != nil {
		t.Fatal(err)
	}
	alias, err := provider.GetTool(ToolReplaceAgentMemorySlots)
	if err != nil || alias != canonical {
		t.Fatalf("alias = %v, %v; want the memory blocks tool", alias, err)
	}
	for _, tool := range provider.GetTools() {
		if tool.GetEntity().Identity.Name == ToolReplaceAgentMemorySlots {
			t.Fatal("the former name is listed as its own tool")
		}
	}
}
