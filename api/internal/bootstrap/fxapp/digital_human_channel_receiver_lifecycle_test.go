package fxapp

import (
	"context"
	"testing"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

type digitalHumanChannelReceiverTestLifecycle struct {
	hooks []fx.Hook
}

func (lifecycle *digitalHumanChannelReceiverTestLifecycle) Append(hook fx.Hook) {
	lifecycle.hooks = append(lifecycle.hooks, hook)
}

type digitalHumanChannelReceiverLifecycleStub struct {
	started bool
	stopped bool
}

func (stub *digitalHumanChannelReceiverLifecycleStub) Start(context.Context) error {
	stub.started = true
	return nil
}

func (stub *digitalHumanChannelReceiverLifecycleStub) Stop(context.Context) error {
	stub.stopped = true
	return nil
}

func TestRegisterDigitalHumanChannelReceiverLifecycleStartsAndStopsManager(t *testing.T) {
	lifecycle := &digitalHumanChannelReceiverTestLifecycle{}
	manager := &digitalHumanChannelReceiverLifecycleStub{}

	RegisterDigitalHumanChannelReceiverLifecycle(lifecycle, manager, zap.NewNop())
	if len(lifecycle.hooks) != 1 {
		t.Fatalf("hook count = %d, want 1", len(lifecycle.hooks))
	}
	if err := lifecycle.hooks[0].OnStart(t.Context()); err != nil {
		t.Fatalf("OnStart() error = %v", err)
	}
	if !manager.started {
		t.Fatal("channel receiver manager was not started")
	}
	if err := lifecycle.hooks[0].OnStop(t.Context()); err != nil {
		t.Fatalf("OnStop() error = %v", err)
	}
	if !manager.stopped {
		t.Fatal("channel receiver manager was not stopped")
	}
}
