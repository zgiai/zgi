//go:build linux && agent_runtime_backup_integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
	"github.com/zgiai/zgi-sandbox/internal/config"
	"github.com/zgiai/zgi-sandbox/internal/storage"
	"github.com/zgiai/zgi-sandbox/internal/testutil"
	"github.com/zgiai/zgi-sandbox/internal/workspacecheckpoint"
)

// Run once per real backend with a dedicated PostgreSQL administrator. All
// databases, compute and filesystem roots belong solely to this test invocation.
func TestRuntimeBackupRealBackendRecovery(t *testing.T) {
	if os.Getenv("ZGI_SANDBOX_TEST_POSTGRES_ADMIN_DSN") == "" {
		t.Fatal("isolated test PostgreSQL administrator is required")
	}
	cfg, err := workerConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	profile := os.Getenv("ZGI_RUNTIME_BACKUP_TEST_PROFILE")
	if profile == "" {
		t.Fatal("approved backup-test Profile is required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 12*time.Minute)
	defer cancel()
	newRoot := func() string {
		t.Helper()
		parent := ""
		if cfg.Backend == localBackendName {
			parent = "/var/lib"
		}
		root, err := os.MkdirTemp(parent, "zgir-")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := os.RemoveAll(root); err != nil {
				t.Errorf("remove owned fixture root: %v", err)
			}
		})
		return root
	}
	sourceCfg := cfg
	sourceCfg.RootDir = newRoot()
	sourceCfg.CheckpointDir = filepath.Join(sourceCfg.RootDir, "checkpoints")
	sourceCfg.CheckpointS3 = workspacecheckpoint.S3Config{}
	sourceCfg.DatabaseURL = testutil.CreateTestPostgresDSN(t)
	sourceStore, err := storage.Open(config.Config{DatabaseURL: sourceCfg.DatabaseURL, MaxWorkers: 1, AgentRuntimeAdmission: &cfg.Admission})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sourceStore.Close() })
	sourceCheckpoints, err := workspacecheckpoint.NewFileStore(sourceCfg.CheckpointDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sourceCheckpoints.Close() })
	name, source, closeSource, err := newRuntimeBackend(sourceCfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(closeSource)
	control, err := agentexecution.NewService(sourceStore, map[string]agentexecution.Backend{name: source}, agentexecution.ServiceOptions{Checkpoints: sourceCheckpoints, Jobs: sourceStore})
	if err != nil {
		t.Fatal(err)
	}
	scope := agentexecution.Scope{OrganizationID: "backup-contract", SubjectID: "operator"}
	binding, err := control.CreateBinding(ctx, agentexecution.CreateBindingRequest{Scope: scope, AgentID: "backup-contract", ConversationID: "source", ProfileID: profile, Backend: name, NetworkPolicy: "package-install", IdempotencyKey: "backup-contract"})
	if err != nil {
		t.Fatal(err)
	}
	allocation := agentexecution.Allocation{BindingRef: binding.BackendRef, WorkspaceRef: binding.WorkspaceRef}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := source.DestroyBinding(cleanup, allocation); err != nil {
			t.Errorf("remove own source compute: %v", err)
		}
	})
	runJob := func(backend runtimeBackend, allocation agentexecution.Allocation, id, script string) {
		t.Helper()
		var output bytes.Buffer
		var mu sync.Mutex
		job, err := backend.StartJob(ctx, allocation, agentexecution.JobSpec{JobID: id, Command: "sh", Args: []string{"-ec", script}, Timeout: 5 * time.Minute}, func(chunk agentexecution.OutputChunk) error {
			mu.Lock()
			defer mu.Unlock()
			if output.Len() < 8192 {
				_, _ = output.Write(chunk.Data)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		result, err := job.Wait(ctx)
		mu.Lock()
		diagnostic := output.String()
		mu.Unlock()
		if err != nil || result.ExitCode != 0 || result.Error != "" || result.TerminationFailed {
			t.Fatalf("fixture Job %s failed: %+v error=%v output=%s", id, result, err, diagnostic)
		}
	}
	runJob(source, allocation, "backup-install", "python3 -m pip install --no-cache-dir --no-input ujson==5.10.0 && npm_config_build_from_source=true npm install --no-save --prefix /workspace bufferutil@4.0.9 && python3 -c 'import os;from pathlib import Path;assert os.getuid()==65532;Path(\"backup-result.txt\").write_text(\"independent-recovery-ok\\n\")'")
	binding, err = control.SuspendBinding(ctx, scope, binding.ID, binding.Generation)
	if err != nil {
		t.Fatal(err)
	}
	// Build a strict CLI manifest for this exact test executable/configuration.
	worker, configuration, err := workerReleaseFingerprint(sourceCfg)
	if err != nil {
		t.Fatal(err)
	}
	release := agentexecution.ReleaseManifest{Version: agentexecution.ReleaseContractVersion, ID: "backup-contract", APIImage: "registry.example/api@sha256:" + string(bytes.Repeat([]byte("a"), 64)), WebImage: "registry.example/web@sha256:" + string(bytes.Repeat([]byte("b"), 64)), WorkerDigest: worker, ConfigurationDigest: configuration, PlatformProtocolVersion: agentexecution.PlatformProtocolVersion}
	encoded, err := json.Marshal(release)
	if err != nil {
		t.Fatal(err)
	}
	sourceCfg.ReleaseManifestPath = filepath.Join(sourceCfg.RootDir, "release.json")
	sourceCfg.ReleaseRequired = true
	if err := os.WriteFile(sourceCfg.ReleaseManifestPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(sourceCfg.RootDir, "backup-set")
	if err := runCheckpointBackup(ctx, sourceCfg, "--backup-checkpoints", directory); err != nil {
		t.Fatal(err)
	}
	if err := runCheckpointBackup(ctx, sourceCfg, "--verify-checkpoint-backup", directory); err != nil {
		t.Fatal(err)
	}
	// Destroy only this test's compute and original checkpoint after backup. This
	// proves reconstruction cannot accidentally use the provider's paused disk.
	if err := source.DestroyBinding(ctx, allocation); err != nil {
		t.Fatal(err)
	}
	if err := sourceCheckpoints.Delete(ctx, binding.CheckpointKey); err != nil {
		t.Fatal(err)
	}
	targetCfg := sourceCfg
	targetCfg.RootDir = newRoot()
	targetCfg.CheckpointDir = filepath.Join(targetCfg.RootDir, "checkpoints")
	targetCfg.DatabaseURL = testutil.CreateTestPostgresDSN(t)
	if err := os.Mkdir(targetCfg.CheckpointDir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZGI_AGENT_RUNTIME_RECOVERY_TARGET", "true")
	backupAsTarget := targetCfg
	backupAsTarget.CheckpointDir = filepath.Join(directory, "checkpoints")
	if err := runCheckpointBackup(ctx, backupAsTarget, "--restore-checkpoints", directory); err == nil {
		t.Fatal("backup directory accepted as mutable recovery store")
	}
	if err := runCheckpointBackup(ctx, targetCfg, "--restore-checkpoints", directory); err != nil {
		t.Fatal(err)
	}
	if err := runCheckpointBackup(ctx, targetCfg, "--restore-checkpoints", directory); err == nil {
		t.Fatal("populated target overwritten on retry")
	}
	targetStore, err := storage.Open(config.Config{DatabaseURL: targetCfg.DatabaseURL, MaxWorkers: 1, AgentRuntimeAdmission: &cfg.Admission})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = targetStore.Close() })
	targetCheckpoints, err := workspacecheckpoint.OpenFileStore(targetCfg.CheckpointDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = targetCheckpoints.Close() })
	_, target, closeTarget, err := newRuntimeBackend(targetCfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(closeTarget)
	targetControl, err := agentexecution.NewService(targetStore, map[string]agentexecution.Backend{name: target}, agentexecution.ServiceOptions{Checkpoints: targetCheckpoints, Jobs: targetStore})
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := targetStore.Binding(ctx, binding.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.BackendRef != "" || recovered.WorkspaceRef != "" {
		t.Fatal("source compute reference restored")
	}
	recovered, err = targetControl.ResumeBinding(ctx, scope, recovered.ID, recovered.Generation)
	if err != nil {
		t.Fatal(err)
	}
	recoveredAllocation := agentexecution.Allocation{BindingRef: recovered.BackendRef, WorkspaceRef: recovered.WorkspaceRef}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := target.DestroyBinding(cleanup, recoveredAllocation); err != nil {
			t.Errorf("remove own recovery compute: %v", err)
		}
	})
	// No installation command is allowed here: restored native user packages
	// must work solely from the archived dependency payloads.
	runJob(target, recoveredAllocation, "backup-native-recovery", "python3 -c 'import os,ujson;assert os.getuid()==65532;assert ujson.__version__==\"5.10.0\"' && node -e \"require('/workspace/node_modules/bufferutil/build/Release/bufferutil.node');if(require('/workspace/node_modules/bufferutil/package.json').version!=='4.0.9')process.exit(1)\"")
	reader, _, err := target.DownloadFile(ctx, recoveredAllocation, "backup-result.txt")
	if err != nil {
		t.Fatal(err)
	}
	actual, err := io.ReadAll(reader)
	err = errors.Join(err, reader.Close())
	if err != nil || string(actual) != "independent-recovery-ok\n" {
		t.Fatalf("recovery file=%q error=%v", actual, err)
	}
	if _, err := targetControl.GetBinding(ctx, agentexecution.Scope{OrganizationID: "other", SubjectID: scope.SubjectID}, recovered.ID); !errors.Is(err, agentexecution.ErrNotFound) {
		t.Fatalf("cross-tenant recovery read=%v", err)
	}
	if _, err := targetControl.DestroyBinding(ctx, scope, recovered.ID, recovered.Generation); err != nil {
		t.Fatal(err)
	}
	t.Logf("backend=%s independent recovery, native imports, file content and empty-target guard passed", name)
}
