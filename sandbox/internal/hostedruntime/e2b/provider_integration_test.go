//go:build e2b_integration && linux

package e2b

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
	"github.com/zgiai/zgi-sandbox/internal/packagesource"
	"github.com/zgiai/zgi-sandbox/internal/workspacecheckpoint"
)

// This intentionally alters only a disposable contract environment. Recovery
// must block the user command, survive remote error mapping, and permit repair.
func assertLanguageLedgerAndRepair(t *testing.T, ctx context.Context, backend agentexecution.ExecutionBackend, allocation agentexecution.Allocation, corrupt bool) {
	t.Helper()
	read := func(name string) []byte {
		t.Helper()
		reader, _, err := backend.DownloadFile(ctx, allocation, name)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	var ledger struct {
		Version   int                                              `json:"version"`
		Packages  []struct{ Language, Name, Version, Path string } `json:"packages"`
		Artifacts []struct {
			Target, SHA256 string
			Size           int64
		} `json:"artifacts"`
	}
	if err := json.Unmarshal(read(".zgi-runtime-home/language-package-ledger.json"), &ledger); err != nil {
		t.Fatal(err)
	}
	foundPython, foundNode := false, false
	for _, pkg := range ledger.Packages {
		foundPython = foundPython || (pkg.Language == "python" && pkg.Name == "ujson" && pkg.Version == "5.10.0")
		foundNode = foundNode || (pkg.Language == "node" && pkg.Name == "bufferutil" && pkg.Version == "4.0.9")
	}
	if ledger.Version != agentexecution.LanguagePackageLedgerVersion || !foundPython || !foundNode || len(ledger.Artifacts) != 2 {
		t.Fatalf("missing native dependency locks: %+v", ledger)
	}
	t.Logf("retained language ledger verified: %d packages", len(ledger.Packages))
	if !corrupt {
		return
	}
	const metadataPath = "node_modules/bufferutil/package.json"
	artifactPath := ""
	for _, artifact := range ledger.Artifacts {
		if artifact.Target == "node_modules" {
			artifactPath = ".zgi-runtime-home/.dependency-artifacts/" + artifact.SHA256 + ".tar.gz"
		}
	}
	if artifactPath == "" {
		t.Fatal("missing native Node artifact")
	}
	validArtifact := read(artifactPath)
	brokenArtifact := bytes.Clone(validArtifact)
	brokenArtifact[len(brokenArtifact)/2] ^= 0xff
	if _, err := backend.UploadFile(ctx, allocation, artifactPath, bytes.NewReader(brokenArtifact), int64(len(brokenArtifact))); err != nil {
		t.Fatal(err)
	}
	for _, filePath := range []string{".zgi-runtime-home/.venv/bin/python", "node_modules/bufferutil/build/Release/bufferutil.node"} {
		broken := []byte("damaged native file")
		if _, err := backend.UploadFile(ctx, allocation, filePath, bytes.NewReader(broken), int64(len(broken))); err != nil {
			t.Fatal(err)
		}
	}
	invalid := []byte(`{"name":"bufferutil","version":"99.0.0"}`)
	if _, err := backend.UploadFile(ctx, allocation, metadataPath, bytes.NewReader(invalid), int64(len(invalid))); err != nil {
		t.Fatal(err)
	}
	job, err := backend.StartJob(ctx, allocation, agentexecution.JobSpec{JobID: "language-mismatch-must-not-run", Command: "touch", Args: []string{"language-must-not-run.txt"}}, func(agentexecution.OutputChunk) error { return nil })
	if job != nil || !errors.Is(err, agentexecution.ErrBackendUnavailable) || !strings.Contains(err.Error(), "no network") {
		t.Fatalf("damaged language restore accepted: job=%v err=%v", job, err)
	}
	if marker, _, err := backend.DownloadFile(ctx, allocation, "language-must-not-run.txt"); err == nil {
		_ = marker.Close()
		t.Fatal("user command ran before dependency verification")
	} else if !errors.Is(err, agentexecution.ErrNotFound) {
		t.Fatal(err)
	}
	// Repair only the artifact. The next command must recover both native files
	// and the original metadata automatically, without pip/npm or build hooks.
	if _, err := backend.UploadFile(ctx, allocation, artifactPath, bytes.NewReader(validArtifact), int64(len(validArtifact))); err != nil {
		t.Fatal(err)
	}
	t.Log("corrupt artifact blocked user Job; repaired only the archive, leaving Python/Node native files and metadata for automatic restoration")
}

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

func TestE2BContract(t *testing.T) {
	apiKey := strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_E2B_API_KEY"))
	templateID := strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_E2B_TEST_TEMPLATE"))
	if apiKey == "" || templateID == "" {
		t.Fatal("ZGI_AGENT_RUNTIME_E2B_API_KEY and ZGI_AGENT_RUNTIME_E2B_TEST_TEMPLATE are required")
	}
	backend, err := New(Options{
		APIURL: strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_E2B_API_URL")),
		Domain: strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_E2B_DOMAIN")),
		APIKey: apiKey, Profiles: map[string]Profile{"integration": {TemplateID: templateID}},
		TimeoutSeconds: 300, ReadyTimeout: 90 * time.Second,
	})
	if err != nil {
		t.Fatalf("new E2B backend: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	spec := agentexecution.CreateSpec{
		BindingID:   "e2b-integration-" + fmt.Sprint(time.Now().UnixNano()),
		WorkspaceID: "workspace-integration", ProfileID: "integration",
	}
	allocation, err := backend.CreateBinding(ctx, spec)
	if err != nil {
		t.Fatalf("create binding: %v", err)
	}
	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if err := backend.DestroyBinding(cleanupContext, allocation); err != nil {
			t.Errorf("cleanup E2B binding: %v", err)
		}
	})

	content := []byte("managed-runtime-input\n")
	if _, err := backend.UploadFile(ctx, allocation, "input.txt", bytes.NewReader(content), int64(len(content))); err != nil {
		t.Fatalf("upload file: %v", err)
	}
	var outputMu sync.Mutex
	var stdout, stderr bytes.Buffer
	running, err := backend.StartJob(ctx, allocation, agentexecution.JobSpec{
		JobID: "e2b-job", Command: "/bin/sh",
		Args:    []string{"-c", "cat input.txt > output.txt; printf stdout-ok; printf stderr-ok >&2"},
		Timeout: 30 * time.Second,
	}, func(chunk agentexecution.OutputChunk) error {
		outputMu.Lock()
		defer outputMu.Unlock()
		if chunk.Stream == agentexecution.OutputStreamStdout {
			_, _ = stdout.Write(chunk.Data)
		} else {
			_, _ = stderr.Write(chunk.Data)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("start job: %v", err)
	}
	result, err := running.Wait(ctx)
	if err != nil || result.ExitCode != 0 || stdout.String() != "stdout-ok" || stderr.String() != "stderr-ok" {
		t.Fatalf("job result=%+v stdout=%q stderr=%q error=%v", result, stdout.String(), stderr.String(), err)
	}

	for filePath, content := range map[string]string{
		"inputs/sales_a.csv": "item,qty\napple,2\nbanana,1\n",
		"inputs/sales_b.csv": "item,qty\napple,3\nbanana,4\n",
	} {
		if _, err := backend.UploadFile(ctx, allocation, filePath, strings.NewReader(content), int64(len(content))); err != nil {
			t.Fatalf("upload %s: %v", filePath, err)
		}
	}
	const aggregateCSV = "import csv\nfrom collections import Counter\nfrom pathlib import Path\ntotals=Counter()\nfor name in ('inputs/sales_a.csv','inputs/sales_b.csv'):\n with open(name, newline='') as source:\n  for row in csv.DictReader(source): totals[row['item']]+=int(row['qty'])\nPath('artifacts').mkdir(exist_ok=True)\nwith open('artifacts/result.csv','w',newline='') as target:\n writer=csv.writer(target, lineterminator='\\n'); writer.writerow(('item','qty')); writer.writerows(sorted(totals.items()))\nprint('rows='+str(len(totals)))"
	var aggregateOutput bytes.Buffer
	aggregateJob, err := backend.StartJob(ctx, allocation, agentexecution.JobSpec{
		JobID: "e2b-aggregate-csv", Command: "python3", Args: []string{"-c", aggregateCSV}, Timeout: 30 * time.Second,
	}, func(chunk agentexecution.OutputChunk) error {
		if chunk.Stream == agentexecution.OutputStreamStdout {
			outputMu.Lock()
			defer outputMu.Unlock()
			_, err := aggregateOutput.Write(chunk.Data)
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("start aggregate job: %v", err)
	}
	aggregateResult, err := aggregateJob.Wait(ctx)
	if err != nil || aggregateResult.ExitCode != 0 || aggregateOutput.String() != "rows=2\n" {
		t.Fatalf("aggregate result=%+v output=%q error=%v", aggregateResult, aggregateOutput.String(), err)
	}
	aggregateReader, aggregateInfo, err := backend.DownloadFile(ctx, allocation, "artifacts/result.csv")
	if err != nil {
		t.Fatalf("download aggregate: %v", err)
	}
	aggregateValue, aggregateReadErr := io.ReadAll(aggregateReader)
	_ = aggregateReader.Close()
	const expectedAggregate = "item,qty\napple,5\nbanana,5\n"
	if aggregateReadErr != nil || aggregateInfo.Size != int64(len(aggregateValue)) || string(aggregateValue) != expectedAggregate {
		t.Fatalf("aggregate artifact=%q info=%+v error=%v", aggregateValue, aggregateInfo, aggregateReadErr)
	}
	homeJob, err := backend.StartJob(ctx, allocation, agentexecution.JobSpec{
		JobID: "e2b-write-session-home", Command: "python3", Timeout: 30 * time.Second,
		Args: []string{"-c", "from pathlib import Path; home=Path.home(); assert str(home)=='/workspace/.zgi-runtime-home'; (home/'prepared.txt').write_text('prepared-result')"},
	}, func(agentexecution.OutputChunk) error { return nil })
	if err != nil {
		t.Fatalf("start session home job: %v", err)
	}
	if homeResult, err := homeJob.Wait(ctx); err != nil || homeResult.ExitCode != 0 {
		t.Fatalf("write session home result=%+v error=%v", homeResult, err)
	}
	checkpointStore, err := workspacecheckpoint.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer checkpointStore.Close()
	checkpointKey := strings.Repeat("b", 64)
	workspaceArchive, err := backend.ExportWorkspace(ctx, allocation)
	if err != nil {
		t.Fatalf("export workspace: %v", err)
	}
	if err := checkpointStore.Save(ctx, checkpointKey, workspaceArchive); err != nil {
		t.Fatalf("save workspace checkpoint: %v", err)
	}
	_ = workspaceArchive.Close()

	if err := backend.SuspendBinding(ctx, allocation); err != nil {
		t.Fatalf("suspend binding: %v", err)
	}
	allocation, err = backend.ResumeBinding(ctx, spec, allocation)
	if err != nil {
		t.Fatalf("resume binding: %v", err)
	}
	reader, info, err := backend.DownloadFile(ctx, allocation, "output.txt")
	if err != nil {
		t.Fatalf("download retained file: %v", err)
	}
	value, readErr := io.ReadAll(reader)
	_ = reader.Close()
	if readErr != nil || !bytes.Equal(value, content) || info.Size != int64(len(content)) {
		t.Fatalf("retained file=%q info=%+v error=%v", value, info, readErr)
	}
	aggregateReader, _, err = backend.DownloadFile(ctx, allocation, "artifacts/result.csv")
	if err != nil {
		t.Fatalf("download retained aggregate: %v", err)
	}
	retainedAggregate, retainedAggregateErr := io.ReadAll(aggregateReader)
	_ = aggregateReader.Close()
	if retainedAggregateErr != nil || string(retainedAggregate) != expectedAggregate {
		t.Fatalf("retained aggregate=%q error=%v", retainedAggregate, retainedAggregateErr)
	}

	networkJob, err := backend.StartJob(ctx, allocation, agentexecution.JobSpec{
		JobID: "e2b-network-denied", Command: "python3",
		Args:    []string{"-c", "import urllib.request; urllib.request.urlopen('https://example.com', timeout=5)"},
		Timeout: 15 * time.Second,
	}, func(agentexecution.OutputChunk) error { return nil })
	if err != nil {
		t.Fatalf("start network test: %v", err)
	}
	networkResult, err := networkJob.Wait(ctx)
	if err != nil || networkResult.ExitCode == 0 {
		t.Fatalf("network escape result=%+v error=%v", networkResult, err)
	}

	cancelJob, err := backend.StartJob(ctx, allocation, agentexecution.JobSpec{
		JobID: "e2b-cancel", Command: "/bin/sh", Args: []string{"-c", "sleep 60 & wait"}, Timeout: time.Minute,
	}, func(agentexecution.OutputChunk) error { return nil })
	if err != nil {
		t.Fatalf("start cancellation test: %v", err)
	}
	if err := cancelJob.Cancel(ctx); err != nil {
		t.Fatalf("cancel job: %v", err)
	}
	cancelResult, err := cancelJob.Wait(ctx)
	if err != nil || !cancelResult.Canceled {
		t.Fatalf("cancel result=%+v error=%v", cancelResult, err)
	}

	lostAllocation := allocation
	if err := backend.DestroyBinding(ctx, lostAllocation); err != nil {
		t.Fatalf("simulate lost E2B compute: %v", err)
	}
	if _, err := backend.ResumeBinding(ctx, spec, lostAllocation); !errors.Is(err, agentexecution.ErrNotFound) {
		t.Fatalf("lost E2B environment error = %v", err)
	}
	allocation, err = backend.CreateBinding(ctx, spec)
	if err != nil {
		t.Fatalf("create replacement E2B environment: %v", err)
	}
	workspaceArchive, err = checkpointStore.Open(ctx, checkpointKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := backend.ImportWorkspace(ctx, allocation, workspaceArchive); err != nil {
		t.Fatalf("restore workspace into replacement E2B environment: %v", err)
	}
	if err := workspaceArchive.Close(); err != nil {
		t.Fatal(err)
	}
	aggregateReader, _, err = backend.DownloadFile(ctx, allocation, "artifacts/result.csv")
	if err != nil {
		t.Fatal(err)
	}
	restoredAggregate, restoredReadErr := io.ReadAll(aggregateReader)
	_ = aggregateReader.Close()
	if restoredReadErr != nil || string(restoredAggregate) != expectedAggregate {
		t.Fatalf("restored aggregate = %q, %v", restoredAggregate, restoredReadErr)
	}
	var homeOutput bytes.Buffer
	homeJob, err = backend.StartJob(ctx, allocation, agentexecution.JobSpec{
		JobID: "e2b-read-session-home", Command: "python3", Timeout: 30 * time.Second,
		Args: []string{"-c", "from pathlib import Path; print((Path.home()/'prepared.txt').read_text())"},
	}, func(chunk agentexecution.OutputChunk) error {
		if chunk.Stream != agentexecution.OutputStreamStdout {
			return nil
		}
		_, err := homeOutput.Write(chunk.Data)
		return err
	})
	if err != nil {
		t.Fatalf("start restored session home job: %v", err)
	}
	if homeResult, err := homeJob.Wait(ctx); err != nil || homeResult.ExitCode != 0 || homeOutput.String() != "prepared-result\n" {
		t.Fatalf("restored session home result=%+v output=%q error=%v", homeResult, homeOutput.String(), err)
	}
}

// TestE2BPackageInstallContract is intentionally opt-in. It requires a
// template built from Dockerfile.agent-runtime-e2b-package and a disposable E2B
// project whose package Profile has the same registry allowlist as the test.
// Keeping this separate from TestE2BContract prevents an offline template from
// being mistaken for evidence that package-only egress works.
func TestE2BPackageInstallContract(t *testing.T) {
	apiKey := strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_E2B_API_KEY"))
	templateID := strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_E2B_TEST_PACKAGE_TEMPLATE"))
	if apiKey == "" || templateID == "" {
		t.Fatal("ZGI_AGENT_RUNTIME_E2B_API_KEY and ZGI_AGENT_RUNTIME_E2B_TEST_PACKAGE_TEMPLATE are required")
	}
	sources := packagesource.Config{PyPI: strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_TEST_PYPI_INDEX_URL"))}
	allowedHosts := []string{
		"pypi.org", "files.pythonhosted.org", "registry.npmjs.org",
		"deb.debian.org", "security.debian.org",
	}
	for _, host := range sources.Hosts() {
		if !slices.Contains(allowedHosts, host) {
			allowedHosts = append(allowedHosts, host)
		}
	}
	backend, err := New(Options{
		APIURL: strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_E2B_API_URL")),
		Domain: strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_E2B_DOMAIN")),
		APIKey: apiKey,
		Profiles: map[string]Profile{"package": {
			TemplateID: templateID, DependencyInstallation: true,
			PackageNetworkMode: "package-install", EgressAllowedHosts: allowedHosts, PackageSources: sources,
		}},
		TimeoutSeconds: 600, ReadyTimeout: 120 * time.Second,
	})
	if err != nil {
		t.Fatalf("new E2B package backend: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	spec := agentexecution.CreateSpec{
		BindingID:   "e2b-package-integration-" + fmt.Sprint(time.Now().UnixNano()),
		WorkspaceID: "workspace-package-integration", ProfileID: "package",
		NetworkMode: "package-install",
	}
	allocation, err := backend.CreateBinding(ctx, spec)
	if err != nil {
		t.Fatalf("create package binding: %v", err)
	}
	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if err := backend.DestroyBinding(cleanupContext, allocation); err != nil {
			t.Errorf("cleanup E2B package binding: %v", err)
		}
	})

	capabilities, err := backend.Capabilities(ctx, allocation)
	if err != nil || !capabilities.DependencyInstallation || !capabilities.PackageNetworkEnforced || capabilities.NetworkMode != "package-install" {
		t.Fatalf("package capabilities=%+v error=%v", capabilities, err)
	}
	for _, canceled := range []bool{false, true} {
		timeout := 500 * time.Millisecond
		if canceled {
			timeout = time.Minute
		}
		job, err := backend.StartJob(ctx, allocation, agentexecution.JobSpec{JobID: fmt.Sprintf("e2b-termination-%t", canceled), Command: "sh", Args: []string{"-c", "sleep 30 & wait"}, Timeout: timeout}, func(agentexecution.OutputChunk) error { return nil })
		if err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		if canceled {
			if err := job.Cancel(ctx); err != nil {
				t.Fatal(err)
			}
		}
		waitCtx, waitCancel := context.WithTimeout(ctx, 10*time.Second)
		result, waitErr := job.Wait(waitCtx)
		waitCancel()
		if waitErr != nil || result.TerminationFailed || result.Canceled != canceled || (!canceled && result.Error != "job timed out") || time.Since(start) > 5*time.Second {
			t.Fatalf("termination canceled=%t result=%+v err=%v elapsed=%s", canceled, result, waitErr, time.Since(start))
		}
		t.Logf("termination canceled=%t elapsed=%s result=%+v", canceled, time.Since(start), result)
	}

	assertJob := func(jobID, command string, args ...string) agentexecution.JobResult {
		t.Helper()
		var outputMu sync.Mutex
		var output bytes.Buffer
		running, startErr := backend.StartJob(ctx, allocation, agentexecution.JobSpec{
			JobID: jobID, Command: command, Args: args, Timeout: 5 * time.Minute,
		}, func(chunk agentexecution.OutputChunk) error {
			outputMu.Lock()
			defer outputMu.Unlock()
			_, err := output.Write(chunk.Data)
			return err
		})
		if startErr != nil {
			t.Fatalf("start %s: %v", jobID, startErr)
		}
		result, waitErr := running.Wait(ctx)
		if waitErr != nil {
			t.Fatalf("wait %s: %v", jobID, waitErr)
		}
		if strings.Contains(jobID, "cache-hit") && !strings.Contains(output.String(), "Using cached") {
			t.Fatalf("expected pip cache hit: %s", output.String())
		}
		t.Logf("%s: exit=%d output=%s", jobID, result.ExitCode, output.String())
		return result
	}

	if result := assertJob("e2b-package-offline-office-resources", "sh", "-c", "python3 /opt/zgi-profile/resources/verify_xml.py && file --version && test ! -w /opt/zgi-profile/resources/xml/catalog.xml"); result.ExitCode != 0 {
		t.Fatalf("offline Office resources unavailable: %+v", result)
	}
	pythonResult := assertJob("e2b-package-python", "sh", "-c", "python3 -m pip install --disable-pip-version-check --no-input --no-cache-dir tomlkit==0.13.3 && python3 -c 'import tomlkit; print(tomlkit.__version__)'")
	if pythonResult.ExitCode != 0 {
		t.Fatalf("python package install result=%+v", pythonResult)
	}
	nodeResult := assertJob("e2b-package-node", "sh", "-c", "npm install --no-save --prefix /workspace zod@3.25.76 && node -e \"require('/workspace/node_modules/zod')\"")
	if nodeResult.ExitCode != 0 {
		t.Fatalf("node package install result=%+v", nodeResult)
	}
	aptResult := assertJob("e2b-package-apt", "sh", "-c", "apt-get install -y --no-install-recommends tree && tree --version")
	if aptResult.ExitCode != 0 {
		t.Fatalf("apt package install result=%+v", aptResult)
	}
	for _, command := range []struct{ id, script string }{
		{"cache-populate", "python3 -m pip download --no-deps -d /workspace/cache-probe tomlkit==0.13.3 && rm /workspace/cache-probe/*.whl"},
		{"cache-hit", "python3 -m pip download --no-deps -d /workspace/cache-probe tomlkit==0.13.3 && npm cache verify && test -n \"$(find \"$NPM_CONFIG_CACHE/_cacache\" -type f -print -quit)\""},
		{"capabilities-zero", "python3 -c \"from pathlib import Path; s=dict(x.split(':',1) for x in Path('/proc/self/status').read_text().splitlines() if ':' in x); assert all(int(s[k].strip(),16)==0 for k in ('CapEff','CapPrm','CapInh','CapAmb')); print('job-capabilities-zero')\""},
	} {
		if result := assertJob(command.id, "sh", "-c", command.script); result.ExitCode != 0 {
			t.Fatalf("%s result=%+v", command.id, result)
		}
	}

	networkResult := assertJob("e2b-package-network-denied", "python3", "-c", "import urllib.request; urllib.request.urlopen('https://example.com', timeout=5)")
	if networkResult.ExitCode == 0 {
		t.Fatalf("ordinary network unexpectedly succeeded: %+v", networkResult)
	}
	for _, denied := range []struct{ id, command string }{
		{"private", "python3 -c \"import socket; socket.create_connection(('169.254.169.254',80),5)\""},
		{"raw-tcp", "python3 -c \"import socket; socket.create_connection(('1.1.1.1',443),5)\""},
		{"proxy-bypass", "env -u HTTPS_PROXY -u https_proxy -u ALL_PROXY -u all_proxy python3 -c \"import urllib.request; urllib.request.urlopen('https://pypi.org/simple',timeout=5)\""},
		{"envd-loopback", "python3 -c \"import socket; socket.create_connection(('127.0.0.1',49983),5)\""},
		{"control-loopback", "python3 -c \"import socket; socket.create_connection(('127.0.0.1',49984),5)\""},
		{"raw-tcp-data", "python3 -c \"import socket; s=socket.create_connection(('1.1.1.1',443),5); s.sendall(b'GET / HTTP/1.1\\r\\nHost: one.one.one.one\\r\\nConnection: close\\r\\n\\r\\n'); reply=s.recv(256); print(repr(reply)); assert reply, 'no raw TCP response'\""},
		{"ipv6", "python3 -c \"import socket; socket.create_connection(('2606:4700:4700::1111',443),5)\""},
		{"registry-override", "python3 -m pip install --no-input --retries=0 --timeout=5 --index-url=https://example.com/simple zgi-negative-fixture"},
		{"apt-options", "apt-get install -o APT::Update::Pre-Invoke::=id tree"},
		{"npm-global", "npm install --global --fetch-retries=0 --fetch-timeout=15000 zod@3.25.76"},
	} {
		if result := assertJob("e2b-package-denied-"+denied.id, "sh", "-c", denied.command); result.ExitCode == 0 {
			t.Errorf("%s unexpectedly succeeded: %+v", denied.id, result)
		}
	}
	if result := assertJob("e2b-package-nonroot", "sh", "-c", "test \"$(id -u)\" = 65532"); result.ExitCode != 0 {
		t.Fatalf("Job acquired unexpected privileges: %+v", result)
	}
	if result := assertJob("e2b-package-session-boundary", "sh", "-c", "grep -Eq '^NoNewPrivs:[[:space:]]+1$' /proc/self/status && python3 -c 'import sys,os; assert sys.prefix == os.environ[\"VIRTUAL_ENV\"]; assert sys.prefix != sys.base_prefix; assert not os.access(\"/usr/local/lib/python3.13/site-packages\", os.W_OK); print(\"session-venv-readonly-base\")'"); result.ExitCode != 0 {
		t.Fatalf("session virtual environment or base Profile boundary broken: %+v", result)
	}

	pythonNative := assertJob("e2b-package-python-native", "sh", "-c", "python3 -m pip install --disable-pip-version-check --no-input --no-cache-dir --no-binary=ujson ujson==5.10.0 && python3 -c 'import ujson; assert ujson.loads(ujson.dumps([1,2])) == [1,2]; print(ujson.__version__)'")
	if pythonNative.ExitCode != 0 {
		t.Fatalf("Python native build result=%+v", pythonNative)
	}
	nodeNative := assertJob("e2b-package-node-native", "sh", "-c", "npm_config_build_from_source=true npm install --no-save --prefix /workspace bufferutil@4.0.9 && node -e \"require('/workspace/node_modules/bufferutil/build/Release/bufferutil.node'); console.log('node-native-ok')\"")
	if nodeNative.ExitCode != 0 {
		t.Fatalf("Node native build result=%+v", nodeNative)
	}

	if err := backend.SuspendBinding(ctx, allocation); err != nil {
		t.Fatalf("pause installed environment: %v", err)
	}
	allocation, err = backend.ResumeBinding(ctx, spec, allocation)
	if err != nil {
		t.Fatalf("resume installed environment: %v", err)
	}
	verifyInstalled := "python3 -c 'import tomlkit,ujson; import os; from pathlib import Path; assert any(Path(os.environ[\"PIP_CACHE_DIR\"]).rglob(\"*.body\")); assert Path(os.environ[\"NPM_CONFIG_CACHE\"],\"_cacache\").is_dir(); print(\"python-and-caches-retained\")' && node -e \"require('/workspace/node_modules/bufferutil/build/Release/bufferutil.node'); console.log('node-native-retained')\" && tree --version"
	verifyInstalled += systemPackageVerification(t, ctx, backend, allocation)
	if result := assertJob("e2b-package-after-resume", "sh", "-c", verifyInstalled); result.ExitCode != 0 {
		t.Fatalf("installed dependencies lost after resume: %+v", result)
	}

	store, err := workspacecheckpoint.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	archive, err := backend.ExportWorkspace(ctx, allocation)
	if err != nil {
		t.Fatalf("export installed workspace: %v", err)
	}
	checkpointKey := strings.Repeat("c", 64)
	if err := store.Save(ctx, checkpointKey, archive); err != nil {
		_ = archive.Close()
		t.Fatalf("checkpoint installed dependencies: %v", err)
	}
	_ = archive.Close()
	if err := backend.DestroyBinding(ctx, allocation); err != nil {
		t.Fatalf("destroy installed compute: %v", err)
	}
	allocation, err = backend.CreateBinding(ctx, spec)
	if err != nil {
		t.Fatalf("recreate installed environment: %v", err)
	}
	archive, err = store.Open(ctx, checkpointKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := backend.ImportWorkspace(ctx, allocation, archive); err != nil {
		_ = archive.Close()
		t.Fatalf("restore installed workspace: %v", err)
	}
	_ = archive.Close()
	assertLanguageLedgerAndRepair(t, ctx, backend, allocation, true)
	if result := assertJob("e2b-package-after-rebuild", "sh", "-c", verifyInstalled); result.ExitCode != 0 {
		t.Fatalf("installed dependencies lost after compute rebuild: %+v", result)
	}
	if result := assertJob("e2b-package-offline-office-after-rebuild", "sh", "-c", "python3 /opt/zgi-profile/resources/verify_xml.py && file --version"); result.ExitCode != 0 {
		t.Fatalf("offline Office resources unavailable after rebuild: %+v", result)
	}
	// A malformed retained ledger must block execution through the real remote
	// protocol, and remain retryable after authenticated operator repair.
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
	checkpointKey = strings.Repeat("e", 64)
	if err := store.Save(ctx, checkpointKey, archive); err != nil {
		_ = archive.Close()
		t.Fatal(err)
	}
	_ = archive.Close()
	if err := backend.DestroyBinding(ctx, allocation); err != nil {
		t.Fatal(err)
	}
	allocation, err = backend.CreateBinding(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	archive, err = store.Open(ctx, checkpointKey)
	if err != nil {
		t.Fatal(err)
	}
	err = backend.ImportWorkspace(ctx, allocation, archive)
	_ = archive.Close()
	if err != nil {
		t.Fatal(err)
	}
	job, err := backend.StartJob(ctx, allocation, agentexecution.JobSpec{JobID: "e2b-invalid-ledger-must-not-run", Command: "touch", Args: []string{"must-not-run.txt"}}, func(agentexecution.OutputChunk) error { return nil })
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
	if result := assertJob("e2b-package-after-ledger-repair", "sh", "-c", verifyInstalled); result.ExitCode != 0 {
		t.Fatalf("repaired ledger could not retry recovery: %+v", result)
	}
}
