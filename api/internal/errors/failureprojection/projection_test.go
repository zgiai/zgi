package failureprojection

import (
	"fmt"
	"strings"
	"testing"
)

func TestProjectPublicPayloadRedactsNestedDetailsWithoutMutatingInput(t *testing.T) {
	const detail = "private provider route"
	input := map[string]interface{}{
		"status":  "failed",
		"error":   map[string]interface{}{"message": detail},
		"outputs": map[string]interface{}{"failure_reason": detail},
	}

	projected := ProjectPublicPayload(input, "workflow run failed", true)
	if strings.Contains(fmt.Sprint(projected), detail) {
		t.Fatalf("projected payload exposed detail: %#v", projected)
	}
	if outputs, ok := projected["outputs"].(map[string]interface{}); !ok || len(outputs) != 0 {
		t.Fatalf("projected outputs = %#v, want empty", projected["outputs"])
	}
	if !strings.Contains(fmt.Sprint(input), detail) {
		t.Fatalf("input diagnostics were mutated: %#v", input)
	}
}
