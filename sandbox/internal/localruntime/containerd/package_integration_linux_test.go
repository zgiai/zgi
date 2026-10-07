//go:build linux && agent_runtime_integration

package containerd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
	"github.com/zgiai/zgi-sandbox/internal/localruntime"
	"github.com/zgiai/zgi-sandbox/internal/workspacecheckpoint"
)

// This contract must run on a real gVisor worker with the standard Profile.
// Docker or an unconfined process is not evidence for its network boundary.
func TestGVisorPackageInstallRecovery(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Fatal("package contract requires a root-owned native Linux worker")
	}
	image := strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_TEST_IMAGE"))
	if image == "" {
		t.Fatal("ZGI_AGENT_RUNTIME_TEST_IMAGE must contain the pinned standard Profile")
	}
	root := t.TempDir()
	provider, err := New(Options{
		Address: strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_CONTAINERD_ADDRESS")),
		RootDir: root,
		Profiles: map[string]Profile{"package": {
			Image: image, DependencyInstallation: true, PackageNetworkMode: "package-install",
			EgressAllowedHosts: []string{"pypi.org", "files.pythonhosted.org", "registry.npmjs.org", "deb.debian.org", "security.debian.org"},
		}},
		StartupLimit: 60 * time.Second, MemoryBytes: 768 * 1024 * 1024, PIDs: 256,
		CPUQuota: 100000, CPUPeriod: 100000,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	backend, err := localruntime.New(provider)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Minute)
	defer cancel()
	spec := agentexecution.CreateSpec{BindingID: "package-recovery", WorkspaceID: "package-workspace", ProfileID: "package", NetworkMode: "package-install"}
	allocation, err := backend.CreateBinding(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := backend.DestroyBinding(cleanup, allocation); err != nil {
			t.Errorf("cleanup package environment: %v", err)
		}
	}()
	capabilities, err := backend.Capabilities(ctx, allocation)
	if err != nil || !capabilities.DependencyInstallation || capabilities.EgressProxyAddress == "" || capabilities.NetworkMode != "package-install" {
		t.Fatalf("package capabilities=%+v error=%v", capabilities, err)
	}
	assertJob := func(id, command string, succeeds bool) {
		t.Helper()
		result, output := runContractJob(t, backend, allocation, agentexecution.JobSpec{JobID: id, Command: "sh", Args: []string{"-c", command}, Timeout: 5 * time.Minute})
		t.Logf("%s: exit=%d output=%s", id, result.ExitCode, output)
		if (result.ExitCode == 0) != succeeds {
			t.Fatalf("%s result=%+v output=%s", id, result, output)
		}
	}
	assertJob("nonroot", "test \"$(id -u)\" = 65532", true)
	assertJob("offline-office-resources", "python3 /opt/zgi-profile/resources/verify_xml.py && file --version && test ! -w /opt/zgi-profile/resources/xml/catalog.xml", true)
	assertJob("session-boundary", "grep -Eq '^NoNewPrivs:[[:space:]]+1$' /proc/self/status && python3 -c 'import sys,os; assert sys.prefix == os.environ[\"VIRTUAL_ENV\"]; assert sys.prefix != sys.base_prefix; print(\"session-venv\")'", true)
	assertJob("python-native", "python3 -m pip install --no-cache-dir --no-input --no-binary=ujson ujson==5.10.0 && python3 -c 'import ujson; assert ujson.loads(ujson.dumps([1,2])) == [1,2]'", true)
	assertJob("idle-bridge", "sleep 35", true)
	assertJob("node-native", "npm_config_build_from_source=true npm install --no-save --prefix /workspace bufferutil@4.0.9 && node -e \"require('/workspace/node_modules/bufferutil'); console.log('node-native-ok')\"", true)
	assertJob("apt", "apt-get install -y --no-install-recommends tree && tree --version", true)
	assertJob("network-denied", "python3 -c \"import urllib.request; urllib.request.urlopen('https://example.com',timeout=5)\"", false)
	assertJob("proxy-bypass-denied", "env -u HTTPS_PROXY -u https_proxy -u ALL_PROXY -u all_proxy python3 -c \"import urllib.request; urllib.request.urlopen('https://example.com',timeout=5)\"", false)
	assertJob("private-network-denied", "python3 -c \"import socket; socket.create_connection(('169.254.169.254',80),5)\"", false)
	assertJob("apt-options-denied", "apt-get install -o APT::Update::Pre-Invoke::=id tree", false)
	assertJob("npm-global-denied", "npm install --global --fetch-retries=0 --fetch-timeout=15000 zod@3.25.76", false)
	verify := "python3 -c 'import ujson; print(\"python-retained\")' && node -e \"require('/workspace/node_modules/bufferutil'); console.log('node-retained')\" && tree --version"
	if err := backend.SuspendBinding(ctx, allocation); err != nil {
		t.Fatal(err)
	}
	allocation, err = backend.ResumeBinding(ctx, spec, allocation)
	if err != nil {
		t.Fatal(err)
	}
	assertJob("after-resume", verify, true)
	store, err := workspacecheckpoint.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	archive, err := backend.ExportWorkspace(ctx, allocation)
	if err != nil {
		t.Fatal(err)
	}
	key := strings.Repeat("d", 64)
	if err := store.Save(ctx, key, archive); err != nil {
		_ = archive.Close()
		t.Fatal(err)
	}
	_ = archive.Close()
	if err := backend.DestroyBinding(ctx, allocation); err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(root, "workspaces", spec.WorkspaceID)
	if relative, err := filepath.Rel(root, workspace); err != nil || relative != filepath.Join("workspaces", spec.WorkspaceID) {
		t.Fatalf("test workspace escaped temporary root: %q", workspace)
	}
	if err := os.RemoveAll(workspace); err != nil {
		t.Fatal(err)
	}
	allocation, err = backend.CreateBinding(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	archive, err = store.Open(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := backend.ImportWorkspace(ctx, allocation, archive); err != nil {
		_ = archive.Close()
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	assertJob("after-rebuild", verify, true)
	assertJob("offline-office-resources-after-rebuild", "python3 /opt/zgi-profile/resources/verify_xml.py && file --version", true)
}
