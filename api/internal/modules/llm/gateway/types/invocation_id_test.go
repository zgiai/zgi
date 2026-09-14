package types

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestInvocationIDIsFreshAndObservedSynchronously(t *testing.T) {
	var observed []string
	parent, cancel := context.WithCancel(t.Context())
	ctx := WithInvocationIDObserver(parent, func(id string) { observed = append(observed, id) })
	first, second := NewInvocationID(ctx), NewInvocationID(ctx)
	if first == second || len(observed) != 2 || observed[0] != first || observed[1] != second {
		t.Fatalf("invocation IDs were reused or not published: %q %q %v", first, second, observed)
	}
	for _, id := range []string{first, second, NewInvocationID(t.Context())} {
		if _, err := uuid.Parse(id); err != nil {
			t.Fatalf("invalid invocation ID %q: %v", id, err)
		}
	}
	cancel()
	if ctx.Err() != context.Canceled {
		t.Fatal("observer lost parent cancellation")
	}
}
