package skills

import (
	"strings"
	"testing"
)

// Relevance ranking changes with every question; the tool list must not, or
// the provider's prompt cache misses from the tools onward (ADR 0032).
func TestAppendNativeToolProjectionsKeepsOneOrderWhateverTheRanking(t *testing.T) {
	projection := func(name string) NativeToolProjection {
		return NativeToolProjection{
			Name: name, NameScope: "serverops", Description: "Server operation " + name,
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
			Binding:     NativeToolBinding{SkillID: "serverops", ToolName: name},
		}
	}
	names := func(ranked ...string) string {
		projections := make([]NativeToolProjection, 0, len(ranked))
		for _, name := range ranked {
			projections = append(projections, projection(name))
		}
		toolSet := &NativeToolSet{}
		if added := AppendNativeToolProjections(toolSet, projections, NativeToolProjectionOptions{}); added != len(ranked) {
			t.Fatalf("added = %d, want %d", added, len(ranked))
		}
		out := make([]string, 0, len(toolSet.ProviderTools))
		for _, tool := range toolSet.ProviderTools {
			out = append(out, tool.Function.Name)
		}
		return strings.Join(out, ",")
	}
	statusFirst := names("service_status", "command_run", "journal_tail")
	commandFirst := names("command_run", "journal_tail", "service_status")
	if statusFirst != commandFirst {
		t.Fatalf("tool order follows the ranking:\n%s\n%s", statusFirst, commandFirst)
	}
}
