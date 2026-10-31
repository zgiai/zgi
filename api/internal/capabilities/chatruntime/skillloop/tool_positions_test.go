package skillloop

import (
	"reflect"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func toolNamesForTest(tools []adapter.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Function.Name)
	}
	return names
}

func namedToolsForTest(names ...string) []adapter.Tool {
	tools := make([]adapter.Tool, 0, len(names))
	for _, name := range names {
		tools = append(tools, adapter.Tool{Type: "function", Function: adapter.Function{Name: name}})
	}
	return tools
}

// A tool that appears during the turn goes after the tools already sent, so
// the cached prefix covering them stays valid.
func TestKeepToolPositionsAppendsToolsAddedDuringTheTurn(t *testing.T) {
	positions := map[string]int{}
	first := keepToolPositions(namedToolsForTest("ssh_run", "memory_view", "update_plan", "activate_skills"), positions)
	if got := toolNamesForTest(first); !reflect.DeepEqual(got, []string{"ssh_run", "memory_view", "update_plan", "activate_skills"}) {
		t.Fatalf("first round = %v, want the order unchanged", got)
	}

	// The newly active skill's tool is built before the runtime and control
	// tools, and read_skill_reference appears among the control tools.
	second := keepToolPositions(namedToolsForTest("ssh_run", "deploy_site", "memory_view", "update_plan", "activate_skills", "read_skill_reference"), positions)
	want := []string{"ssh_run", "memory_view", "update_plan", "activate_skills", "deploy_site", "read_skill_reference"}
	if got := toolNamesForTest(second); !reflect.DeepEqual(got, want) {
		t.Fatalf("second round = %v, want %v", got, want)
	}

	third := keepToolPositions(namedToolsForTest("read_skill_reference", "deploy_site", "ssh_run", "memory_view", "update_plan", "activate_skills"), positions)
	if got := toolNamesForTest(third); !reflect.DeepEqual(got, want) {
		t.Fatalf("third round = %v, want %v", got, want)
	}
}
