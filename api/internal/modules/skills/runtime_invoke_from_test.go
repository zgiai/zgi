package skills

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestNormalizeToolInvokeFromKeepsBoundRuntimeCallers(t *testing.T) {
	for _, caller := range []tools.ToolInvokeFrom{tools.ToolInvokeFromAgent, tools.ToolInvokeFromDigitalHuman, tools.ToolInvokeFromWorkflow, tools.ToolInvokeFromAIChat} {
		if got := normalizeToolInvokeFrom(caller); got != caller {
			t.Fatalf("normalizeToolInvokeFrom(%q) = %q", caller, got)
		}
	}
	if got := normalizeToolInvokeFrom("unknown"); got != tools.ToolInvokeFromAIChat {
		t.Fatalf("unknown caller = %q, want AIChat", got)
	}
}
