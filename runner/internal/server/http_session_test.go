package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/zgiai/zgi/runner/internal/runtime"
)

func TestAPI_LaunchSession_MetadataHeaders(t *testing.T) {
	_, ts := setupTestServer(t)

	payload := map[string]any{
		"name":       "demo-plugin",
		"version":    "0.0.1",
		"language":   "python",
		"entrypoint": "main",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/sessions", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "test-admin-key")
	req.Header.Set("X-Workflow-Run-ID", "run-from-header")
	req.Header.Set("X-Session-Policy", string(runtime.SessionPolicyReuseWithinRun))
	req.Header.Set("X-Session-Idle-TTL-Seconds", "321")
	req.Header.Set("X-Session-Max-Lifetime-Seconds", "654")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, string(b))
	}

	var snap runtime.Snapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if snap.Metadata == nil {
		t.Fatalf("expected metadata in launch response")
	}
	if snap.Metadata.WorkflowRunID != "run-from-header" {
		t.Fatalf("unexpected workflow_run_id: %q", snap.Metadata.WorkflowRunID)
	}
	if snap.Metadata.SessionPolicy != string(runtime.SessionPolicyReuseWithinRun) {
		t.Fatalf("unexpected session_policy: %q", snap.Metadata.SessionPolicy)
	}
	if snap.Metadata.SessionIdleTTLSeconds != 321 {
		t.Fatalf("unexpected session_idle_ttl_seconds: %d", snap.Metadata.SessionIdleTTLSeconds)
	}
	if snap.Metadata.SessionMaxLifetimeSeconds != 654 {
		t.Fatalf("unexpected session_max_lifetime_seconds: %d", snap.Metadata.SessionMaxLifetimeSeconds)
	}
	if snap.LastActivityAt == nil {
		t.Fatalf("expected last_activity_at in launch response")
	}
}
