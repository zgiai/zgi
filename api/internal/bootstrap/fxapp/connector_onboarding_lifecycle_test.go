package fxapp

import (
	"context"
	"errors"
	"go.uber.org/fx"
	"testing"
	"time"
)

type onboardingLifecycleFixture struct{ hooks []fx.Hook }

func (l *onboardingLifecycleFixture) Append(h fx.Hook) { l.hooks = append(l.hooks, h) }

type onboardingWorkerFixture struct{ started, stopped chan struct{} }

func (w onboardingWorkerFixture) Run(ctx context.Context) {
	close(w.started)
	<-ctx.Done()
	close(w.stopped)
}
func TestConnectorOnboardingLifecycleChecksSchemaAndJoinsWorker(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "start stop", true: "missing schema"}[fail], func(t *testing.T) {
			l := &onboardingLifecycleFixture{}
			w := onboardingWorkerFixture{make(chan struct{}), make(chan struct{})}
			RegisterConnectorOnboardingLifecycle(l, w, func(context.Context) error {
				if fail {
					return errors.New("schema missing")
				}
				return nil
			})
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			err := l.hooks[0].OnStart(ctx)
			if fail {
				if err == nil {
					t.Fatal("started without schema")
				}
				select {
				case <-w.started:
					t.Fatal("worker started")
				default:
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-w.started:
			case <-ctx.Done():
				t.Fatal("worker did not start")
			}
			if err = l.hooks[0].OnStop(ctx); err != nil {
				t.Fatal(err)
			}
			select {
			case <-w.stopped:
			default:
				t.Fatal("worker not joined")
			}
		})
	}
}
