package shared

import (
	"context"
	"errors"
)

// IsContextCancellation reports whether err belongs to an execution whose
// context was explicitly canceled. The context check lets callers recognize
// provider errors that lost the original context.Canceled wrapping.
func IsContextCancellation(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return true
	}
	return ctx != nil && errors.Is(ctx.Err(), context.Canceled)
}
