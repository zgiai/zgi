package types

import (
	"context"

	"github.com/google/uuid"
)

// InvocationIDHeader identifies the gateway invocation recorded in usage audit.
// It is distinct from a client-supplied transport X-Request-ID.
const InvocationIDHeader = "X-ZGI-Request-ID"

type invocationIDObserverKey struct{}

// WithInvocationIDObserver observes the next gateway invocation ID synchronously,
// before provider work or streaming starts. It cannot supply a billing identity.
// HTTP callers must not reuse this observer after their response is committed.
func WithInvocationIDObserver(ctx context.Context, observer func(string)) context.Context {
	return context.WithValue(ctx, invocationIDObserverKey{}, observer)
}

// NewInvocationID creates a fresh identity for each invocation, including when
// the parent context or a client request header is reused. Provider retries keep
// this ID and have separate attempt IDs within the invocation.
func NewInvocationID(ctx context.Context) string {
	id := uuid.NewString()
	if observer, ok := ctx.Value(invocationIDObserverKey{}).(func(string)); ok && observer != nil {
		observer(id)
	}
	return id
}
