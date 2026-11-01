package service

import (
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
)

func TestModelInvocationMetadataRecordsStreamTermination(t *testing.T) {
	invocation := modelInvocationFromTrace(skillloop.ModelInvocationTrace{
		Phase:              "skill_planning",
		Round:              2,
		Streaming:          true,
		StartedAt:          time.Unix(10, 0),
		FinishReason:       "tool_calls",
		StreamDoneReceived: true,
		TerminatedBy:       "done",
	}, "", true)

	if got := stringFromAny(invocation["finish_reason"]); got != "tool_calls" {
		t.Fatalf("finish_reason = %q, want tool_calls", got)
	}
	if got := stringFromAny(invocation["terminated_by"]); got != "done" {
		t.Fatalf("terminated_by = %q, want done", got)
	}
	if got, ok := invocation["stream_done_received"].(bool); !ok || !got {
		t.Fatalf("stream_done_received = %#v, want true", invocation["stream_done_received"])
	}
}
