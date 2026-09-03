package fxapp

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

type integrationCompletionTestLifecycle struct {
	hooks []fx.Hook
}

func (l *integrationCompletionTestLifecycle) Append(hook fx.Hook) {
	l.hooks = append(l.hooks, hook)
}

type integrationCompletionTestRunner struct {
	started chan struct{}
	stopped chan struct{}
}

type integrationOAuthMaintenanceTestRunner struct {
	started chan struct{}
	stopped chan struct{}
}

type integrationOAuthRecoveryTestRunner struct {
	started chan struct{}
	stopped chan struct{}
}

type workflowBindingBackfillTestRunner struct {
	called bool
	err    error
}

type workflowBindingBackfillIncompleteTestError struct {
	err error
}

func (err *workflowBindingBackfillIncompleteTestError) Error() string {
	return err.err.Error()
}

func (err *workflowBindingBackfillIncompleteTestError) Unwrap() error {
	return err.err
}

func (*workflowBindingBackfillIncompleteTestError) WorkflowConnectorBindingBackfillIncomplete() bool {
	return true
}

func (r *workflowBindingBackfillTestRunner) ReconcileWorkflowConnectorBindingBackfills(context.Context) error {
	r.called = true
	return r.err
}

func TestRegisterWorkflowConnectorBindingBackfillLifecycleRunsBeforeServing(t *testing.T) {
	lifecycle := &integrationCompletionTestLifecycle{}
	runner := &workflowBindingBackfillTestRunner{}
	RegisterWorkflowConnectorBindingBackfillLifecycle(lifecycle, runner, zap.NewNop())
	if len(lifecycle.hooks) != 1 {
		t.Fatalf("hook count = %d, want 1", len(lifecycle.hooks))
	}
	if err := lifecycle.hooks[0].OnStart(context.Background()); err != nil {
		t.Fatalf("OnStart() error = %v", err)
	}
	if !runner.called {
		t.Fatal("workflow binding backfill was not called")
	}
}

func TestRegisterWorkflowConnectorBindingBackfillLifecycleKeepsFailedItemsFailClosed(t *testing.T) {
	lifecycle := &integrationCompletionTestLifecycle{}
	runner := &workflowBindingBackfillTestRunner{err: &workflowBindingBackfillIncompleteTestError{err: errors.New("historical workflow is invalid")}}
	RegisterWorkflowConnectorBindingBackfillLifecycle(lifecycle, runner, zap.NewNop())
	if err := lifecycle.hooks[0].OnStart(context.Background()); err != nil {
		t.Fatalf("OnStart() error = %v, want incomplete item retained without disabling unrelated routes", err)
	}
	if !runner.called {
		t.Fatal("workflow binding backfill was not called")
	}
}

func TestRegisterWorkflowConnectorBindingBackfillLifecycleRejectsMissingMigration(t *testing.T) {
	lifecycle := &integrationCompletionTestLifecycle{}
	runner := &workflowBindingBackfillTestRunner{err: errors.New(`ERROR: column "consumer_type" does not exist (SQLSTATE 42703)`)}
	RegisterWorkflowConnectorBindingBackfillLifecycle(lifecycle, runner, zap.NewNop())
	err := lifecycle.hooks[0].OnStart(context.Background())
	if err == nil || !errors.Is(err, runner.err) {
		t.Fatalf("OnStart() error = %v, want fatal schema initialization error", err)
	}
	if !runner.called {
		t.Fatal("workflow binding backfill was not called")
	}
}

func TestRegisterWorkflowConnectorBindingBackfillLifecycleIgnoresNilRunner(t *testing.T) {
	lifecycle := &integrationCompletionTestLifecycle{}
	RegisterWorkflowConnectorBindingBackfillLifecycle(lifecycle, nil, zap.NewNop())
	if len(lifecycle.hooks) != 0 {
		t.Fatalf("hook count = %d, want 0", len(lifecycle.hooks))
	}
}

func (r *integrationOAuthRecoveryTestRunner) RunOAuthRecovery(ctx context.Context) {
	close(r.started)
	<-ctx.Done()
	close(r.stopped)
}

func (r *integrationOAuthMaintenanceTestRunner) RunOAuthMaintenance(ctx context.Context) {
	close(r.started)
	<-ctx.Done()
	close(r.stopped)
}

func (r *integrationCompletionTestRunner) RunCompletionRecovery(ctx context.Context) {
	close(r.started)
	<-ctx.Done()
	close(r.stopped)
}

func TestRegisterIntegrationAuditCompletionLifecycleStartsAndStopsWorker(t *testing.T) {
	lifecycle := &integrationCompletionTestLifecycle{}
	runner := &integrationCompletionTestRunner{
		started: make(chan struct{}),
		stopped: make(chan struct{}),
	}
	RegisterIntegrationAuditCompletionLifecycle(lifecycle, runner, zap.NewNop())
	if len(lifecycle.hooks) != 1 {
		t.Fatalf("hook count = %d, want 1", len(lifecycle.hooks))
	}

	if err := lifecycle.hooks[0].OnStart(context.Background()); err != nil {
		t.Fatalf("OnStart() error = %v", err)
	}
	select {
	case <-runner.started:
	case <-time.After(time.Second):
		t.Fatal("completion recovery worker did not start")
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := lifecycle.hooks[0].OnStop(stopCtx); err != nil {
		t.Fatalf("OnStop() error = %v", err)
	}
	select {
	case <-runner.stopped:
	default:
		t.Fatal("completion recovery worker did not stop")
	}
}

func TestRegisterIntegrationAuditCompletionLifecycleIgnoresNilRunner(t *testing.T) {
	lifecycle := &integrationCompletionTestLifecycle{}
	RegisterIntegrationAuditCompletionLifecycle(lifecycle, nil, zap.NewNop())
	if len(lifecycle.hooks) != 0 {
		t.Fatalf("hook count = %d, want 0", len(lifecycle.hooks))
	}
}

func TestRegisterIntegrationOAuthMaintenanceLifecycleStartsAndStopsWorker(t *testing.T) {
	lifecycle := &integrationCompletionTestLifecycle{}
	runner := &integrationOAuthMaintenanceTestRunner{
		started: make(chan struct{}),
		stopped: make(chan struct{}),
	}
	RegisterIntegrationOAuthMaintenanceLifecycle(lifecycle, runner, zap.NewNop())
	if len(lifecycle.hooks) != 1 {
		t.Fatalf("hook count = %d, want 1", len(lifecycle.hooks))
	}
	if err := lifecycle.hooks[0].OnStart(context.Background()); err != nil {
		t.Fatalf("OnStart() error = %v", err)
	}
	select {
	case <-runner.started:
	case <-time.After(time.Second):
		t.Fatal("OAuth maintenance worker did not start")
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := lifecycle.hooks[0].OnStop(stopCtx); err != nil {
		t.Fatalf("OnStop() error = %v", err)
	}
	select {
	case <-runner.stopped:
	default:
		t.Fatal("OAuth maintenance worker did not stop")
	}
}

func TestRegisterIntegrationOAuthMaintenanceLifecycleIgnoresNilRunner(t *testing.T) {
	lifecycle := &integrationCompletionTestLifecycle{}
	RegisterIntegrationOAuthMaintenanceLifecycle(lifecycle, nil, zap.NewNop())
	if len(lifecycle.hooks) != 0 {
		t.Fatalf("hook count = %d, want 0", len(lifecycle.hooks))
	}
}

func TestRegisterIntegrationOAuthRecoveryLifecycleStartsAndStopsWorker(t *testing.T) {
	lifecycle := &integrationCompletionTestLifecycle{}
	runner := &integrationOAuthRecoveryTestRunner{
		started: make(chan struct{}),
		stopped: make(chan struct{}),
	}
	RegisterIntegrationOAuthRecoveryLifecycle(lifecycle, runner, zap.NewNop())
	if len(lifecycle.hooks) != 1 {
		t.Fatalf("hook count = %d, want 1", len(lifecycle.hooks))
	}
	if err := lifecycle.hooks[0].OnStart(context.Background()); err != nil {
		t.Fatalf("OnStart() error = %v", err)
	}
	select {
	case <-runner.started:
	case <-time.After(time.Second):
		t.Fatal("OAuth recovery worker did not start")
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := lifecycle.hooks[0].OnStop(stopCtx); err != nil {
		t.Fatalf("OnStop() error = %v", err)
	}
	select {
	case <-runner.stopped:
	default:
		t.Fatal("OAuth recovery worker did not stop")
	}
}

func TestRegisterIntegrationOAuthRecoveryLifecycleIgnoresNilRunner(t *testing.T) {
	lifecycle := &integrationCompletionTestLifecycle{}
	RegisterIntegrationOAuthRecoveryLifecycle(lifecycle, nil, zap.NewNop())
	if len(lifecycle.hooks) != 0 {
		t.Fatalf("hook count = %d, want 0", len(lifecycle.hooks))
	}
}
