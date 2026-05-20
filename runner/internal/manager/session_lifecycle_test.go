package manager

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/zgiai/zgi/runner/internal/runtime"
)

type controllableRuntime struct {
	session *runtime.Session
}

func (r *controllableRuntime) Start(_ context.Context, req runtime.StartRequest) (*runtime.Session, error) {
	session := runtime.NewSession(req.Manifest, req.WorkingDir)
	session.MarkRunning(12345)
	r.session = session
	return session, nil
}

func TestManagerLaunch_SetsMetadataAndRemovesSessionOnDone(t *testing.T) {
	cfg := testConfig(t)
	cfg.SessionSweepIntervalSeconds = 0
	cfg.ReuseSessionIdleTTLSeconds = 600
	cfg.ReuseSessionMaxLifetimeSeconds = 3600

	store := newMockStore(t)
	rt := &controllableRuntime{}
	mgr := New(cfg, rt, store, zap.NewNop(), nil, nil, nil)
	defer func() {
		_ = mgr.Close(context.Background())
	}()

	snap, err := mgr.Launch(context.Background(), LaunchRequest{
		Manifest:      regexManifest(),
		WorkflowRunID: "run-123",
		SessionPolicy: string(runtime.SessionPolicyReuseWithinRun),
	})
	if err != nil {
		t.Fatalf("launch failed: %v", err)
	}
	if snap.Metadata == nil {
		t.Fatalf("expected metadata in snapshot")
	}
	if snap.Metadata.WorkflowRunID != "run-123" {
		t.Fatalf("unexpected workflow_run_id: %q", snap.Metadata.WorkflowRunID)
	}
	if snap.Metadata.SessionPolicy != string(runtime.SessionPolicyReuseWithinRun) {
		t.Fatalf("unexpected session_policy: %q", snap.Metadata.SessionPolicy)
	}
	if snap.Metadata.SessionIdleTTLSeconds != 600 {
		t.Fatalf("unexpected idle ttl: %d", snap.Metadata.SessionIdleTTLSeconds)
	}
	if snap.Metadata.SessionMaxLifetimeSeconds != 3600 {
		t.Fatalf("unexpected max lifetime: %d", snap.Metadata.SessionMaxLifetimeSeconds)
	}
	if snap.LastActivityAt == nil {
		t.Fatalf("expected last_activity_at in snapshot")
	}

	if _, ok := mgr.Get(snap.ID); !ok {
		t.Fatalf("session %s not found right after launch", snap.ID)
	}

	rt.session.MarkExited(nil)

	if !waitUntil(2*time.Second, func() bool {
		_, ok := mgr.Get(snap.ID)
		return !ok
	}) {
		t.Fatalf("session %s was not removed after completion", snap.ID)
	}
}

func TestManagerSweepReusableSessions_StopsOnlyExpiredReusableSessions(t *testing.T) {
	cfg := testConfig(t)
	cfg.SessionSweepIntervalSeconds = 0
	cfg.ReuseSessionIdleTTLSeconds = 1
	cfg.ReuseSessionMaxLifetimeSeconds = 60

	store := newMockStore(t)
	mgr := New(cfg, &mockRuntime{}, store, zap.NewNop(), nil, nil, nil)
	defer func() {
		_ = mgr.Close(context.Background())
	}()

	reusable := runtime.NewSession(regexManifest(), store.Workspace(regexManifest()))
	reusable.MarkRunning(111)
	reusable.SetMetadata(runtime.SessionMetadata{
		WorkflowRunID:             "run-reuse",
		SessionPolicy:             string(runtime.SessionPolicyReuseWithinRun),
		SessionIdleTTLSeconds:     1,
		SessionMaxLifetimeSeconds: 60,
	})
	var reusableStops int32
	reusable.SetStopFunc(func(context.Context) error {
		atomic.AddInt32(&reusableStops, 1)
		return nil
	})

	noReuse := runtime.NewSession(regexManifest(), store.Workspace(regexManifest()))
	noReuse.MarkRunning(222)
	noReuse.SetMetadata(runtime.SessionMetadata{
		WorkflowRunID:             "run-no-reuse",
		SessionPolicy:             string(runtime.SessionPolicyNoReuse),
		SessionIdleTTLSeconds:     1,
		SessionMaxLifetimeSeconds: 60,
	})
	var noReuseStops int32
	noReuse.SetStopFunc(func(context.Context) error {
		atomic.AddInt32(&noReuseStops, 1)
		return nil
	})

	mgr.sessions.Store(reusable.ID(), reusable)
	mgr.sessions.Store(noReuse.ID(), noReuse)

	time.Sleep(1100 * time.Millisecond)
	mgr.sweepReusableSessions()

	if got := atomic.LoadInt32(&reusableStops); got != 1 {
		t.Fatalf("expected reusable session to be stopped once, got %d", got)
	}
	if got := atomic.LoadInt32(&noReuseStops); got != 0 {
		t.Fatalf("expected no_reuse session not to be stopped, got %d", got)
	}
}

func waitUntil(timeout time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return cond()
}
