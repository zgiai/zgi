package agentexecution

import (
	"context"
	"fmt"
	"time"
)

type OneShotWorker interface {
	RunOne(context.Context) (bool, error)
}

type OneShotFunc func(context.Context) (bool, error)

func (f OneShotFunc) RunOne(ctx context.Context) (bool, error) {
	return f(ctx)
}

type LoopOptions struct {
	IdleInterval  time.Duration
	ErrorInterval time.Duration
	OnError       func(error)
}

// RunLoop owns the retry and idle wait policy for one background worker. It
// stops only when ctx is canceled; every wait is interruptible.
func RunLoop(ctx context.Context, worker OneShotWorker, options LoopOptions) error {
	if worker == nil {
		return fmt.Errorf("%w: background worker is required", ErrInvalidRequest)
	}
	if options.IdleInterval <= 0 {
		options.IdleInterval = 250 * time.Millisecond
	}
	if options.ErrorInterval <= 0 {
		options.ErrorInterval = time.Second
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		processed, err := worker.RunOne(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if options.OnError != nil {
				options.OnError(err)
			}
			if err := waitLoop(ctx, options.ErrorInterval); err != nil {
				return err
			}
			continue
		}
		if !processed {
			if err := waitLoop(ctx, options.IdleInterval); err != nil {
				return err
			}
		}
	}
}

func waitLoop(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
