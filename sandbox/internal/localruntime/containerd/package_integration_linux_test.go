//go:build linux && agent_runtime_integration

package containerd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
	"github.com/zgiai/zgi-sandbox/internal/localruntime"
	"github.com/zgiai/zgi-sandbox/internal/packagesource"
	"github.com/zgiai/zgi-sandbox/internal/workspacecheckpoint"
)

// This contract must run on a real gVisor worker with the standard Profile.
// Docker or an unconfined process is not evidence for its network boundary.
// systemPackageVerification reads the root-owned ledger through the authenticated
// data plane, not by relaxing Job file permissions.
func systemPackageVerification(t *testing.T, ctx context.Context, backend agentexecution.ExecutionBackend, allocation agentexecution.Allocation) string {
	t.Helper()
	reader, _, err := backend.DownloadFile(ctx, allocation, ".zgi-runtime-home/package-install-ledger.json")
	if err != nil {
		t.Fatal(err)
	}
	raw, readErr := io.ReadAll(io.LimitReader(reader, (1<<20)+1))
	closeErr := reader.Close()
	if readErr != nil || closeErr != nil || len(raw) > 1<<20 {
		t.Fatalf("read system ledger: bytes=%d read=%v close=%v", len(raw), readErr, closeErr)
	}
	var ledger struct {
		Version  int
		Packages []struct{ Name, Architecture, Version string }
	}
	if err := json.Unmarshal(raw, &ledger); err != nil || ledger.Version != 2 || len(ledger.Packages) == 0 {
		t.Fatalf("invalid locked ledger: version=%d packages=%d error=%v", ledger.Version, len(ledger.Packages), err)
	}
	var rows []string
	for _, pkg := range ledger.Packages {
		rows = append(rows, pkg.Name+"\t"+pkg.Architecture+"\t"+pkg.Version)
	}
	slices.Sort(rows)
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(rows, "\n")+"\n")))
	return " && python3 -c 'import hashlib,subprocess; q=subprocess.check_output([\"dpkg-query\",\"-W\",\"-f=${Package}\\t${Architecture}\\t${Version}\\t${db:Status-Status}\\n\"],text=True); rows=sorted(\"\\t\".join(r[:3]) for r in (x.split(\"\\t\") for x in q.splitlines()) if r[3]==\"installed\"); digest=hashlib.sha256((\"\\n\".join(rows)+\"\\n\").encode()).hexdigest(); assert digest==\"" + digest + "\"; print(\"locked-system-versions-verified\",len(rows))'"
}

func TestGVisorPackageInstallRecovery(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Fatal("package contract requires a root-owned native Linux worker")
	}
	image := strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_TEST_IMAGE"))
	if image == "" {
		t.Fatal("ZGI_AGENT_RUNTIME_TEST_IMAGE must contain the pinned standard Profile")
	}
	root := t.TempDir()
	sources := packagesource.Config{
		PyPI:        strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_TEST_PYPI_INDEX_URL")),
		APT:         strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_TEST_APT_MIRROR_URL")),
		APTSecurity: strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_TEST_APT_SECURITY_MIRROR_URL")),
	}
	hosts := []string{"pypi.org", "files.pythonhosted.org", "registry.npmjs.org", "deb.debian.org", "security.debian.org"}
	for _, host := range sources.Hosts() {
		if !slices.Contains(hosts, host) {
			hosts = append(hosts, host)
		}
	}
	provider, err := New(Options{
		Address: strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_CONTAINERD_ADDRESS")),
		RootDir: root,
		Profiles: map[string]Profile{"package": {
			Image: image, DependencyInstallation: true, PackageNetworkMode: "package-install",
			EgressAllowedHosts: hosts, PackageSources: sources,
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
		if id == "cached-package-downloads" && !strings.Contains(string(output), "Using cached") {
			t.Fatalf("expected pip cache hit: %s", output)
		}
		t.Logf("%s: exit=%d output=%s", id, result.ExitCode, output)
		if (result.ExitCode == 0) != succeeds {
			t.Fatalf("%s result=%+v output=%s", id, result, output)
		}
	}
	assertJob("nonroot", "test \"$(id -u)\" = 65532", true)
	assertJob("job-capabilities", "python3 -c \"from pathlib import Path; s=dict(x.split(':',1) for x in Path('/proc/self/status').read_text().splitlines() if ':' in x); assert all(int(s[k].strip(),16)==0 for k in ('CapEff','CapPrm','CapInh','CapAmb')); print('job-capabilities-zero')\"", true)
	for _, canceled := range []bool{false, true} {
		timeout := 500 * time.Millisecond
		if canceled {
			timeout = time.Minute
		}
		job, err := backend.StartJob(ctx, allocation, agentexecution.JobSpec{JobID: fmt.Sprintf("termination-%t", canceled), Command: "sh", Args: []string{"-c", "sleep 30 & wait"}, Timeout: timeout}, func(agentexecution.OutputChunk) error { return nil })
		if err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		if canceled {
			if err := job.Cancel(ctx); err != nil {
				t.Fatal(err)
			}
		}
		waitCtx, waitCancel := context.WithTimeout(ctx, 5*time.Second)
		result, waitErr := job.Wait(waitCtx)
		waitCancel()
		if waitErr != nil || result.TerminationFailed || result.Canceled != canceled || (!canceled && result.Error != "job timed out") || time.Since(start) > 3*time.Second {
			t.Fatalf("termination canceled=%t result=%+v err=%v elapsed=%s", canceled, result, waitErr, time.Since(start))
		}
		t.Logf("termination canceled=%t elapsed=%s result=%+v", canceled, time.Since(start), result)
	}
	assertJob("offline-office-resources", "python3 /opt/zgi-profile/resources/verify_xml.py && file --version && test ! -w /opt/zgi-profile/resources/xml/catalog.xml", true)
	assertJob("session-boundary", "grep -Eq '^NoNewPrivs:[[:space:]]+1$' /proc/self/status && python3 -c 'import sys,os; assert sys.prefix == os.environ[\"VIRTUAL_ENV\"]; assert sys.prefix != sys.base_prefix; print(\"session-venv\")'", true)
	assertJob("python-native", "python3 -m pip install --no-cache-dir --no-input --no-binary=ujson ujson==5.10.0 && python3 -c 'import ujson; assert ujson.loads(ujson.dumps([1,2])) == [1,2]'", true)
	assertJob("idle-bridge", "sleep 35", true)
	assertJob("node-native", "npm_config_build_from_source=true npm install --no-save --prefix /workspace bufferutil@4.0.9 && node -e \"require('/workspace/node_modules/bufferutil'); console.log('node-native-ok')\"", true)
	// The guarded installer refreshes omitted metadata before one safe retry.
	assertJob("apt", "apt-get install -y --no-install-recommends tree && tree --version", true)
	assertJob("cached-package-downloads", "python3 -m pip download --no-deps -d /workspace/cache-probe tomlkit==0.13.3 && rm /workspace/cache-probe/*.whl && python3 -m pip download --no-deps -d /workspace/cache-probe tomlkit==0.13.3 && npm cache verify", true)
	assertJob("network-denied", "python3 -c \"import urllib.request; urllib.request.urlopen('https://example.com',timeout=5)\"", false)
	assertJob("proxy-bypass-denied", "env -u HTTPS_PROXY -u https_proxy -u ALL_PROXY -u all_proxy python3 -c \"import urllib.request; urllib.request.urlopen('https://example.com',timeout=5)\"", false)
	assertJob("private-network-denied", "python3 -c \"import socket; socket.create_connection(('169.254.169.254',80),5)\"", false)
	assertJob("apt-options-denied", "apt-get install -o APT::Update::Pre-Invoke::=id tree", false)
	assertJob("npm-global-denied", "npm install --global --fetch-retries=0 --fetch-timeout=15000 zod@3.25.76", false)
	verify := "python3 -c 'import ujson; import os; from pathlib import Path; assert any(Path(os.environ[\"PIP_CACHE_DIR\"]).rglob(\"*.body\")); assert Path(os.environ[\"NPM_CONFIG_CACHE\"],\"_cacache\").is_dir(); print(\"python-and-caches-retained\")' && node -e \"require('/workspace/node_modules/bufferutil'); console.log('node-retained')\" && tree --version"
	verify += systemPackageVerification(t, ctx, backend, allocation)
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
	// Corrupt only this disposable test checkpoint, then prove the remote error
	// survives reconstruction and no user command starts before recovery.
	const ledgerPath = ".zgi-runtime-home/package-install-ledger.json"
	reader, _, err := backend.DownloadFile(ctx, allocation, ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	validLedger, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	invalidLedger := []byte(`{"entries":[{"args":["upgrade"]}]}`)
	if _, err := backend.UploadFile(ctx, allocation, ledgerPath, bytes.NewReader(invalidLedger), int64(len(invalidLedger))); err != nil {
		t.Fatal(err)
	}
	archive, err = backend.ExportWorkspace(ctx, allocation)
	if err != nil {
		t.Fatal(err)
	}
	key = strings.Repeat("e", 64)
	if err := store.Save(ctx, key, archive); err != nil {
		_ = archive.Close()
		t.Fatal(err)
	}
	_ = archive.Close()
	if err := backend.DestroyBinding(ctx, allocation); err != nil {
		t.Fatal(err)
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
	err = backend.ImportWorkspace(ctx, allocation, archive)
	_ = archive.Close()
	if err != nil {
		t.Fatal(err)
	}
	job, err := backend.StartJob(ctx, allocation, agentexecution.JobSpec{JobID: "invalid-ledger-must-not-run", Command: "touch", Args: []string{"must-not-run.txt"}}, func(agentexecution.OutputChunk) error { return nil })
	if job != nil || !errors.Is(err, agentexecution.ErrBackendUnavailable) || !strings.Contains(err.Error(), "no latest-version fallback") {
		t.Fatalf("unsafe recovery accepted or diagnostic lost: job=%v error=%v", job, err)
	}
	if marker, _, err := backend.DownloadFile(ctx, allocation, "must-not-run.txt"); err == nil {
		_ = marker.Close()
		t.Fatal("user command ran before valid recovery")
	} else if !errors.Is(err, agentexecution.ErrNotFound) {
		t.Fatalf("check blocked command: %v", err)
	}
	if _, err := backend.UploadFile(ctx, allocation, ledgerPath, bytes.NewReader(validLedger), int64(len(validLedger))); err != nil {
		t.Fatal(err)
	}
	assertJob("after-ledger-repair", verify, true)
}
