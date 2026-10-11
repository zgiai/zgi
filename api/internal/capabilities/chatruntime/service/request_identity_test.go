package service

import (
	"context"
	"testing"
)

// TestStreamRequestIDSurvivesWithoutCancel pins the transport request identity
// plumbing: the runtime derives its persistence context with
// context.WithoutCancel, so the id recorded at the HTTP edge must survive into
// the error finalization path that builds the SSE error payload.
func TestStreamRequestIDSurvivesWithoutCancel(t *testing.T) {
	if got := StreamRequestID(context.Background()); got != "" {
		t.Fatalf("StreamRequestID(background) = %q, want empty", got)
	}
	if got := StreamRequestID(WithStreamRequestID(context.Background(), "   ")); got != "" {
		t.Fatalf("blank request id = %q, want empty", got)
	}

	ctx := WithStreamRequestID(context.Background(), " req-aichat-1 ")
	persistCtx := context.WithoutCancel(ctx)
	if got := StreamRequestID(persistCtx); got != "req-aichat-1" {
		t.Fatalf("StreamRequestID(persistCtx) = %q, want req-aichat-1", got)
	}
}
