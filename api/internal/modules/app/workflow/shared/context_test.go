package shared

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestIsContextCancellation(t *testing.T) {
	leaseErr := errors.New("renew execution lease")
	tests := []struct {
		name string
		ctx  context.Context
		err  error
		want bool
	}{
		{name: "direct cancellation", ctx: context.Background(), err: context.Canceled, want: true},
		{name: "wrapped cancellation", ctx: context.Background(), err: fmt.Errorf("provider request: %w", context.Canceled), want: true},
		{name: "unwrapped provider error after cancellation", ctx: canceledContext(), err: errors.New("stream closed"), want: true},
		{name: "cancel cause preserves lease failure", ctx: canceledCauseContext(leaseErr), err: context.Canceled, want: false},
		{name: "provider error does not hide cancel cause", ctx: canceledCauseContext(leaseErr), err: errors.New("stream closed"), want: false},
		{name: "successful result after cancellation race", ctx: canceledContext(), err: nil, want: false},
		{name: "deadline is not explicit cancellation", ctx: context.Background(), err: context.DeadlineExceeded, want: false},
		{name: "ordinary error", ctx: context.Background(), err: errors.New("provider unavailable"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsContextCancellation(tt.ctx, tt.err); got != tt.want {
				t.Fatalf("IsContextCancellation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveContextError(t *testing.T) {
	leaseErr := errors.New("renew execution lease")
	if got := ResolveContextError(canceledCauseContext(leaseErr), context.Canceled); !errors.Is(got, leaseErr) {
		t.Fatalf("ResolveContextError() = %v, want lease error", got)
	}
	if got := ResolveContextError(canceledContext(), errors.New("stream closed")); !errors.Is(got, context.Canceled) {
		t.Fatalf("ResolveContextError() = %v, want context.Canceled", got)
	}
	if got := ResolveContextError(canceledCauseContext(leaseErr), nil); got != nil {
		t.Fatalf("ResolveContextError(nil) = %v, want nil", got)
	}
}

func canceledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func canceledCauseContext(cause error) context.Context {
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(cause)
	return ctx
}
