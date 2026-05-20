package runtime

import (
	"testing"
	"time"

	"github.com/zgiai/zgi/runner/internal/plugin"
)

func TestSessionSnapshotMetadataAndLastActivity(t *testing.T) {
	session := NewSession(plugin.Manifest{
		Name:    "demo",
		Version: "0.0.1",
		Runner: plugin.Runner{
			Language:   plugin.LanguagePython,
			Entrypoint: "main",
		},
	}, t.TempDir())

	initial := session.Snapshot()
	if initial.Metadata != nil {
		t.Fatalf("expected metadata to be omitted by default")
	}
	if initial.LastActivityAt == nil {
		t.Fatalf("expected last_activity_at to be set on creation")
	}

	beforeTouch := *initial.LastActivityAt
	time.Sleep(5 * time.Millisecond)

	session.SetMetadata(SessionMetadata{
		WorkflowRunID:             "wf-1",
		SessionPolicy:             string(SessionPolicyReuseWithinRun),
		SessionIdleTTLSeconds:     600,
		SessionMaxLifetimeSeconds: 3600,
	})
	session.TouchActivity()

	snap := session.Snapshot()
	if snap.Metadata == nil {
		t.Fatalf("expected metadata to be present")
	}
	if snap.Metadata.WorkflowRunID != "wf-1" {
		t.Fatalf("unexpected workflow_run_id: %q", snap.Metadata.WorkflowRunID)
	}
	if snap.LastActivityAt == nil || !snap.LastActivityAt.After(beforeTouch) {
		t.Fatalf("expected last_activity_at to move forward")
	}
}
