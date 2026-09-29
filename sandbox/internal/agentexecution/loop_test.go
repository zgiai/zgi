package agentexecution

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunLoopStopsDuringIdleWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	worker := &fakeOneShotWorker{}
	done := make(chan error, 1)
	go func() {
		done <- RunLoop(ctx, worker, LoopOptions{IdleInterval: time.Hour})
	}()
	deadline := time.Now().Add(time.Second)
	for worker.calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("loop error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("loop did not stop")
	}
}

func TestRunLoopReportsErrorAndBacksOff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	worker := &fakeOneShotWorker{err: errors.New("injected")}
	reported := make(chan error, 1)
	done := make(chan error, 1)
	go func() {
		done <- RunLoop(ctx, worker, LoopOptions{
			ErrorInterval: time.Hour,
			OnError: func(err error) {
				reported <- err
				cancel()
			},
		})
	}()
	if err := <-reported; err.Error() != "injected" {
		t.Fatalf("reported error = %v", err)
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("loop error = %v", err)
	}
	if worker.calls.Load() != 1 {
		t.Fatalf("worker calls = %d, want 1", worker.calls.Load())
	}
}

type fakeOneShotWorker struct {
	calls atomic.Int64
	err   error
}

func (w *fakeOneShotWorker) RunOne(context.Context) (bool, error) {
	w.calls.Add(1)
	return false, w.err
}
