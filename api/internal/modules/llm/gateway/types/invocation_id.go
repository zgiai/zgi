package types

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
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

// NewInvocationID keeps the authenticated HTTP request ID as the invocation
// identity when the gateway context carries one. Provider retries keep this
// request ID and have separate attempt IDs within the invocation.
func NewInvocationID(ctx context.Context) string {
	id := ""
	if ctx != nil {
		if requestID, ok := ctx.Value(shared.ContextKeyRequestID).(string); ok {
			id = strings.TrimSpace(requestID)
		}
	}
	if id == "" {
		id = uuid.NewString()
	}
	if observer, ok := ctx.Value(invocationIDObserverKey{}).(func(string)); ok && observer != nil {
		observer(id)
	}
	return id
}
