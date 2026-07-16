package service

import "testing"

func TestRuntimeTimelineKeepsOrderAcrossInvocationUpdates(t *testing.T) {
	metadata := mergeModelInvocationMetadata(nil, map[string]interface{}{
		"kind":          "model_call",
		"runtime_id":    "model-1",
		"created_at_ms": int64(1_000),
		"status":        "success",
	})
	metadata = mergeSkillInvocationMetadata(metadata, []map[string]interface{}{{
		"kind":          "user_input_request",
		"runtime_id":    "input-1",
		"created_at_ms": int64(2_000),
		"status":        "success",
	}})
	metadata = mergeModelInvocationMetadata(metadata, map[string]interface{}{
		"kind":          "model_call",
		"runtime_id":    "model-2",
		"created_at_ms": int64(3_000),
		"status":        "success",
	})

	timeline := runtimeTimelineFromMetadata(metadata[runtimeTimelineMetadataKey])
	if len(timeline) != 3 {
		t.Fatalf("timeline len = %d, want 3: %#v", len(timeline), timeline)
	}
	for index, wantID := range []string{"model-1", "input-1", "model-2"} {
		if got := stringFromAny(timeline[index]["runtime_id"]); got != wantID {
			t.Fatalf("timeline[%d].runtime_id = %q, want %q", index, got, wantID)
		}
		if got := runtimeTimelineInt64(timeline[index]["sequence"]); got != int64(index+1) {
			t.Fatalf("timeline[%d].sequence = %d, want %d", index, got, index+1)
		}
	}

	metadata = mergeSkillInvocationMetadata(metadata, []map[string]interface{}{{
		"kind":          "user_input_request",
		"runtime_id":    "input-1",
		"created_at_ms": int64(9_000),
		"status":        "error",
	}})
	timeline = runtimeTimelineFromMetadata(metadata[runtimeTimelineMetadataKey])
	if got := runtimeTimelineInt64(timeline[1]["sequence"]); got != 2 {
		t.Fatalf("updated sequence = %d, want 2", got)
	}
	if got, _ := unixMillisecondsFromAny(timeline[1]["created_at_ms"]); got != 2_000 {
		t.Fatalf("updated created_at_ms = %d, want 2000", got)
	}

	metadata = mergeModelInvocationMetadata(copyStringAnyMap(metadata), map[string]interface{}{
		"kind":          "model_call",
		"runtime_id":    "model-after-continuation",
		"created_at_ms": int64(10_000),
		"status":        "success",
	})
	timeline = runtimeTimelineFromMetadata(metadata[runtimeTimelineMetadataKey])
	if got := runtimeTimelineInt64(timeline[3]["sequence"]); got != 4 {
		t.Fatalf("continuation sequence = %d, want 4", got)
	}
}

func TestWorkflowRuntimeTimelineReusesRunSequence(t *testing.T) {
	metadata := mergeWorkflowRunMetadata(nil, "workflow_started", map[string]interface{}{
		"workflow_run_id": "run-1",
		"created_at":      int64(10),
	})
	metadata = mergeWorkflowRunMetadata(metadata, "workflow_finished", map[string]interface{}{
		"workflow_run_id": "run-1",
		"created_at":      int64(20),
	})
	timeline := runtimeTimelineFromMetadata(metadata[runtimeTimelineMetadataKey])
	if len(timeline) != 1 {
		t.Fatalf("timeline len = %d, want 1: %#v", len(timeline), timeline)
	}
	if got := runtimeTimelineInt64(timeline[0]["sequence"]); got != 1 {
		t.Fatalf("sequence = %d, want 1", got)
	}
	if got, _ := unixMillisecondsFromAny(timeline[0]["created_at_ms"]); got != 10_000 {
		t.Fatalf("created_at_ms = %d, want 10000", got)
	}
}

func TestRuntimeTimelineBootstrapsLegacyEventsBeforeContinuation(t *testing.T) {
	metadata := map[string]interface{}{
		"model_invocations": []interface{}{
			map[string]interface{}{
				"kind":          "model_call",
				"runtime_id":    "legacy-model-1",
				"created_at_ms": int64(1_000),
				"response":      map[string]interface{}{"tool_call_names": []interface{}{"request_user_input"}},
			},
			map[string]interface{}{
				"kind":          "model_call",
				"runtime_id":    "legacy-model-2",
				"created_at_ms": int64(3_000),
			},
		},
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":       "user_input_request",
				"runtime_id": "legacy-input-1",
			},
		},
	}

	metadata = mergeModelInvocationMetadata(metadata, map[string]interface{}{
		"kind":          "model_call",
		"runtime_id":    "continuation-model",
		"created_at_ms": int64(4_000),
	})
	timeline := runtimeTimelineFromMetadata(metadata[runtimeTimelineMetadataKey])
	if len(timeline) != 4 {
		t.Fatalf("timeline len = %d, want 4: %#v", len(timeline), timeline)
	}
	for index, runtimeID := range []string{"legacy-model-1", "legacy-input-1", "legacy-model-2", "continuation-model"} {
		if got := stringFromAny(timeline[index]["runtime_id"]); got != runtimeID {
			t.Fatalf("timeline[%d].runtime_id = %q, want %q", index, got, runtimeID)
		}
		if got := runtimeTimelineInt64(timeline[index]["sequence"]); got != int64(index+1) {
			t.Fatalf("timeline[%d].sequence = %d, want %d", index, got, index+1)
		}
	}
}
