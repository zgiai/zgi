package fxapp

import (
	"context"
	"testing"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

type fileObjectDeletionLifecycleFixture struct {
	hooks []fx.Hook
}

func (l *fileObjectDeletionLifecycleFixture) Append(hook fx.Hook) {
	l.hooks = append(l.hooks, hook)
}

type fileObjectDeletionLifecycleRunner struct {
	calls chan struct{}
}

func (r *fileObjectDeletionLifecycleRunner) RunOnce(context.Context) error {
	select {
	case r.calls <- struct{}{}:
	default:
	}
	return nil
}

func TestRegisterRuntimeWorkspaceFileObjectDeletionLifecycleStartsAndStops(t *testing.T) {
	lifecycle := &fileObjectDeletionLifecycleFixture{}
	runner := &fileObjectDeletionLifecycleRunner{calls: make(chan struct{}, 1)}
	RegisterRuntimeWorkspaceFileObjectDeletionLifecycle(lifecycle, runner, zap.NewNop())
	if len(lifecycle.hooks) != 1 {
		t.Fatalf("hook count = %d, want 1", len(lifecycle.hooks))
	}
	if err := lifecycle.hooks[0].OnStart(t.Context()); err != nil {
		t.Fatalf("OnStart() error = %v", err)
	}
	select {
	case <-runner.calls:
	default:
		t.Fatal("initial deletion sweep was not run")
	}
	if err := lifecycle.hooks[0].OnStop(t.Context()); err != nil {
		t.Fatalf("OnStop() error = %v", err)
	}
}
