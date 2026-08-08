package shared

import (
	"context"
	"errors"
)

// ResolveContextError preserves the cause attached to a canceled context.
// This prevents infrastructure failures propagated through CancelCauseFunc
// from being flattened into context.Canceled by downstream providers.
func ResolveContextError(ctx context.Context, err error) error {
	if err == nil || ctx == nil || ctx.Err() == nil {
		return err
	}
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	return err
}

// IsContextCancellation reports whether err belongs to an execution explicitly
// canceled without a more specific failure cause.
func IsContextCancellation(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(ResolveContextError(ctx, err), context.Canceled)
}
