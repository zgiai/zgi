//go:build linux

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

func TestAdmissionEnvironmentIsStrict(t *testing.T) {
	for _, input := range []string{"0", "-1", "invalid", "9999999999999999999999"} {
		t.Setenv("ZGI_AGENT_RUNTIME_MAX_JOBS", input)
		var limits agentexecution.AdmissionLimits
		if err := decodeAdmissionLimits(&limits); err == nil {
			t.Fatalf("invalid limit %q accepted", input)
		}
	}
	t.Setenv("ZGI_AGENT_RUNTIME_MAX_JOBS", "2")
	var limits agentexecution.AdmissionLimits
	if err := decodeAdmissionLimits(&limits); err != nil {
		t.Fatal(err)
	}
	if limits.Bindings != 2 || limits.JobsPerBinding != 1 || limits.JobTimeoutSeconds != 300 || limits.BindingsPerEndUser != 2 || limits.JobsPerEndUser != 1 {
		t.Fatalf("defaults=%+v", limits)
	}
}

func TestWorkerReleaseFingerprintAndStrictManifest(t *testing.T) {
	cfg := workerConfig{Backend: localBackendName}
	worker, configuration, err := workerReleaseFingerprint(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg.APIKey, cfg.DatabaseURL, cfg.E2BAPIKey = "not-hashed", "not-hashed", "not-hashed"
	_, same, err := workerReleaseFingerprint(cfg)
	if err != nil || same != configuration {
		t.Fatal("credentials changed public fingerprint")
	}
	cfg.PlatformCLI = true
	_, changed, err := workerReleaseFingerprint(cfg)
	if err != nil || changed == configuration {
		t.Fatal("platform capability missing from fingerprint")
	}
	cfg.PlatformCLI = false
	manifest := agentexecution.ReleaseManifest{Version: 1, ID: "release-1", APIImage: "registry/api@" + worker, WebImage: "registry/web@" + worker,
		WorkerDigest: worker, ConfigurationDigest: configuration, PlatformProtocolVersion: agentexecution.PlatformProtocolVersion}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := agentexecution.VerifyRelease(bytes.NewReader(data), worker, configuration); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "release.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	cfg.ReleaseManifestPath = path
	if id, err := verifyWorkerRelease(cfg); err != nil || id != "release-1" {
		t.Fatalf("verified id=%q err=%v", id, err)
	}
	cfg.PlatformCLI = true
	if _, err := verifyWorkerRelease(cfg); err == nil {
		t.Fatal("configuration mismatch accepted")
	}
	cfg.ReleaseManifestPath += ".missing"
	if _, err := verifyWorkerRelease(cfg); err == nil {
		t.Fatal("missing manifest silently downgraded")
	}
	cfg.ReleaseManifestPath = ""
	if id, err := verifyWorkerRelease(cfg); err != nil || id != "" {
		t.Fatal("legacy opt-out changed")
	}
	cfg.ReleaseRequired = true
	if _, err := verifyWorkerRelease(cfg); err == nil {
		t.Fatal("strict release silently downgraded without a manifest")
	}
}
