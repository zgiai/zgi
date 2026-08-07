package shared

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestIsContextCancellation(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		err  error
		want bool
	}{
		{name: "direct cancellation", ctx: context.Background(), err: context.Canceled, want: true},
		{name: "wrapped cancellation", ctx: context.Background(), err: fmt.Errorf("provider request: %w", context.Canceled), want: true},
		{name: "unwrapped provider error after cancellation", ctx: canceledContext(), err: errors.New("stream closed"), want: true},
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

func canceledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}
