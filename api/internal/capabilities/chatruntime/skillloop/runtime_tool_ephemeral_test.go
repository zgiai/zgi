package skillloop

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestRuntimeToolModelResultIsNotPersistedInTrace(t *testing.T) {
	tool := RuntimeTool{
		Definition: adapter.Tool{Type: "function", Function: adapter.Function{Name: "ephemeral"}},
		Handler: func(context.Context, adapter.ToolCall) RuntimeToolResult {
			return RuntimeToolResult{
				Result:      map[string]interface{}{"message_count": 1},
				ModelResult: map[string]interface{}{"context": "secret channel content"},
			}
		},
	}
	step := handleRuntimeToolCall(t.Context(), adapter.ToolCall{ID: "call-1", Function: adapter.FunctionCall{Name: "ephemeral"}}, tool)
	if encoded := toTestJSON(step.trace.Result); strings.Contains(encoded, "secret channel content") {
		t.Fatalf("ephemeral model result leaked into trace: %s", encoded)
	}
	if !strings.Contains(fmt.Sprint(step.toolMessage.Content), "secret channel content") {
		t.Fatalf("model did not receive ephemeral result: %#v", step.toolMessage)
	}
}

func toTestJSON(value interface{}) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
