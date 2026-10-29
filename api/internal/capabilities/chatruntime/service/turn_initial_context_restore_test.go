package service

import (
	"strings"
	"testing"
)

func TestContinuationRestoresPageContextOnlyForATurnThatSentIt(t *testing.T) {
	snapshot := func(included bool) map[string]interface{} {
		value := map[string]interface{}{
			"schema": turnInitialContextSnapshotSchema, "surface": "console", "route": "/console/digital-humans/1/chat",
			"operation_ledger": map[string]interface{}{"resource_count": "0"},
		}
		if included {
			value["runtime_context"] = map[string]interface{}{"included": true, "char_count": 120}
		}
		return value
	}

	without := &chatRequestParts{}
	restoreTurnInitialContextFromMetadata(without, map[string]interface{}{turnInitialContextSnapshotKey: snapshot(false)})
	if without.RuntimeContext != "" {
		t.Fatalf("restored page context %q for a turn that sent none", without.RuntimeContext)
	}

	with := &chatRequestParts{}
	restoreTurnInitialContextFromMetadata(with, map[string]interface{}{turnInitialContextSnapshotKey: snapshot(true)})
	if !strings.Contains(with.RuntimeContext, "route=/console/digital-humans/1/chat") {
		t.Fatalf("page context = %q, want the restored route", with.RuntimeContext)
	}
}
