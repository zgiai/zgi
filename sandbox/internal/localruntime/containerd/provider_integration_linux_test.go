//go:build linux && agent_runtime_integration

package containerd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
	"github.com/zgiai/zgi-sandbox/internal/localruntime"
	"github.com/zgiai/zgi-sandbox/internal/workspacecheckpoint"
)

func TestContainerdGVisorCheckpointRecovery(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Fatal("checkpoint contract requires a root-owned native Linux worker")
	}
	image := strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_TEST_IMAGE"))
	if image == "" {
		t.Fatal("ZGI_AGENT_RUNTIME_TEST_IMAGE is required")
	}
	root := t.TempDir()
	provider, err := New(Options{
		Address: strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_CONTAINERD_ADDRESS")),
		RootDir: root, Profiles: map[string]Profile{"contract": {Image: image}},
		StartupLimit: 45 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	backend, err := localruntime.New(provider)
	if err != nil {
		t.Fatal(err)
	}
	spec := agentexecution.CreateSpec{BindingID: "checkpoint-binding", WorkspaceID: "checkpoint-workspace", ProfileID: "contract"}
	allocation, err := backend.CreateBinding(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = backend.DestroyBinding(context.Background(), allocation) }()
	const result = "item,qty\napple,5\nbanana,5\n"
	if _, err := backend.UploadFile(context.Background(), allocation, "artifacts/result.csv", strings.NewReader(result), int64(len(result))); err != nil {
		t.Fatal(err)
	}
	store, err := workspacecheckpoint.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	key := strings.Repeat("a", 64)
	archive, err := backend.ExportWorkspace(context.Background(), allocation)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), key, archive); err != nil {
		t.Fatal(err)
	}
	_ = archive.Close()
	if err := backend.SuspendBinding(context.Background(), allocation); err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(root, "workspaces", spec.WorkspaceID)
	relative, err := filepath.Rel(root, workspace)
	if err != nil || relative != filepath.Join("workspaces", spec.WorkspaceID) {
		t.Fatalf("test workspace escaped temporary root: %q", workspace)
	}
	if err := os.RemoveAll(workspace); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.ResumeBinding(context.Background(), spec, allocation); !errors.Is(err, agentexecution.ErrNotFound) {
		t.Fatalf("missing retained workspace error = %v", err)
	}
	allocation, err = backend.CreateBinding(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	archive, err = store.Open(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if err := backend.ImportWorkspace(context.Background(), allocation, archive); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	reader, _, err := backend.DownloadFile(context.Background(), allocation, "artifacts/result.csv")
	if err != nil {
		t.Fatal(err)
	}
	value, readErr := io.ReadAll(reader)
	_ = reader.Close()
	if readErr != nil || string(value) != result {
		t.Fatalf("restored artifact = %q, %v", value, readErr)
	}
}

func TestContainerdGVisorContract(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Fatal("containerd gVisor contract test must run as root on the worker node")
	}
	image := strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_TEST_IMAGE"))
	if image == "" {
		t.Fatal("ZGI_AGENT_RUNTIME_TEST_IMAGE must contain a preloaded digest-qualified image")
	}
	provider, err := New(Options{
		Address: strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_CONTAINERD_ADDRESS")),
		RootDir: t.TempDir(),
		Profiles: map[string]Profile{
			"contract": {Image: image},
		},
		StartupLimit: 45 * time.Second,
		MemoryBytes:  256 * 1024 * 1024,
		PIDs:         64,
		CPUQuota:     100000,
		CPUPeriod:    100000,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	t.Cleanup(func() { _ = provider.Close() })
	backend, err := localruntime.New(provider)
	if err != nil {
		t.Fatalf("new local backend: %v", err)
	}
	spec := agentexecution.CreateSpec{
		BindingID: "contract-binding", WorkspaceID: "contract-workspace",
		OrganizationID: "contract-org", SubjectID: "contract-user", ProfileID: "contract",
	}
	allocation, err := backend.CreateBinding(context.Background(), spec)
	if err != nil {
		t.Fatalf("create binding: %v", err)
	}
	t.Cleanup(func() {
		cleanupContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = backend.DestroyBinding(cleanupContext, allocation)
	})

	capabilities, err := backend.Capabilities(context.Background(), allocation)
	if err != nil {
		t.Fatalf("capabilities: %v", err)
	}
	if !capabilities.Shell || !capabilities.PTY || !capabilities.FileTransfer || capabilities.NetworkMode != "disabled" {
		t.Fatalf("capabilities = %+v", capabilities)
	}

	input := []byte("alpha,beta\n1,2\n")
	if _, err := backend.UploadFile(context.Background(), allocation, "input.csv", bytes.NewReader(input), int64(len(input))); err != nil {
		t.Fatalf("upload input: %v", err)
	}
	result, output := runContractJob(t, backend, allocation, agentexecution.JobSpec{
		JobID: "transform", Command: "python3", Timeout: 10 * time.Second,
		Args: []string{"-c", "from pathlib import Path; value=Path('input.csv').read_text().upper(); Path('result.csv').write_text(value); print(value, end='')"},
	})
	if result.ExitCode != 0 || string(output) != strings.ToUpper(string(input)) {
		t.Fatalf("transform result = %+v, output = %q", result, output)
	}
	reader, info, err := backend.DownloadFile(context.Background(), allocation, "result.csv")
	if err != nil {
		t.Fatalf("download result: %v", err)
	}
	value, readErr := io.ReadAll(reader)
	_ = reader.Close()
	if readErr != nil || info.Size != int64(len(value)) || string(value) != strings.ToUpper(string(input)) {
		t.Fatalf("artifact = %q, info = %+v, error = %v", value, info, readErr)
	}

	for filePath, content := range map[string]string{
		"inputs/sales_a.csv": "item,qty\napple,2\nbanana,1\n",
		"inputs/sales_b.csv": "item,qty\napple,3\nbanana,4\n",
	} {
		if _, err := backend.UploadFile(context.Background(), allocation, filePath, strings.NewReader(content), int64(len(content))); err != nil {
			t.Fatalf("upload %s: %v", filePath, err)
		}
	}
	const aggregateCSV = "import csv\nfrom collections import Counter\nfrom pathlib import Path\ntotals=Counter()\nfor name in ('inputs/sales_a.csv','inputs/sales_b.csv'):\n with open(name, newline='') as source:\n  for row in csv.DictReader(source): totals[row['item']]+=int(row['qty'])\nPath('artifacts').mkdir(exist_ok=True)\nwith open('artifacts/result.csv','w',newline='') as target:\n writer=csv.writer(target, lineterminator='\\n'); writer.writerow(('item','qty')); writer.writerows(sorted(totals.items()))\nprint('rows='+str(len(totals)))"
	aggregateResult, aggregateOutput := runContractJob(t, backend, allocation, agentexecution.JobSpec{
		JobID: "aggregate-csv", Command: "python3", Args: []string{"-c", aggregateCSV}, Timeout: 10 * time.Second,
	})
	if aggregateResult.ExitCode != 0 || string(aggregateOutput) != "rows=2\n" {
		t.Fatalf("aggregate result = %+v, output = %q", aggregateResult, aggregateOutput)
	}
	aggregateReader, aggregateInfo, err := backend.DownloadFile(context.Background(), allocation, "artifacts/result.csv")
	if err != nil {
		t.Fatalf("download aggregate: %v", err)
	}
	aggregateValue, aggregateReadErr := io.ReadAll(aggregateReader)
	_ = aggregateReader.Close()
	const expectedAggregate = "item,qty\napple,5\nbanana,5\n"
	if aggregateReadErr != nil || aggregateInfo.Size != int64(len(aggregateValue)) || string(aggregateValue) != expectedAggregate {
		t.Fatalf("aggregate artifact = %q, info = %+v, error = %v", aggregateValue, aggregateInfo, aggregateReadErr)
	}

	ptyResult, ptyOutput := runContractJob(t, backend, allocation, agentexecution.JobSpec{
		JobID: "pty", Command: "/bin/sh", Args: []string{"-c", "test -t 1 && printf pty-ok"},
		PTY: true, PTYColumns: 80, PTYRows: 24, Timeout: 5 * time.Second,
	})
	if ptyResult.ExitCode != 0 || !bytes.Contains(ptyOutput, []byte("pty-ok")) {
		t.Fatalf("PTY result = %+v, output = %q", ptyResult, ptyOutput)
	}

	networkResult, networkOutput := runContractJob(t, backend, allocation, agentexecution.JobSpec{
		JobID: "network", Command: "python3", Timeout: 5 * time.Second,
		Args: []string{"-c", "import socket; s=socket.socket(); s.settimeout(1);\ntry:\n s.connect(('1.1.1.1',443)); print('connected'); raise SystemExit(42)\nexcept OSError:\n print('blocked')"},
	})
	if networkResult.ExitCode != 0 || !bytes.Contains(networkOutput, []byte("blocked")) {
		t.Fatalf("network result = %+v, output = %q", networkResult, networkOutput)
	}

	if err := backend.SuspendBinding(context.Background(), allocation); err != nil {
		t.Fatalf("suspend binding: %v", err)
	}
	if _, err := backend.Capabilities(context.Background(), allocation); err == nil {
		t.Fatal("suspended environment still accepted data-plane calls")
	}
	allocation, err = backend.ResumeBinding(context.Background(), spec, allocation)
	if err != nil {
		t.Fatalf("resume binding: %v", err)
	}
	reader, _, err = backend.DownloadFile(context.Background(), allocation, "result.csv")
	if err != nil {
		t.Fatalf("download retained result: %v", err)
	}
	retained, readErr := io.ReadAll(reader)
	_ = reader.Close()
	if readErr != nil || !bytes.Equal(retained, value) {
		t.Fatalf("retained artifact = %q, error = %v", retained, readErr)
	}
	aggregateReader, _, err = backend.DownloadFile(context.Background(), allocation, "artifacts/result.csv")
	if err != nil {
		t.Fatalf("download retained aggregate: %v", err)
	}
	retainedAggregate, retainedAggregateErr := io.ReadAll(aggregateReader)
	_ = aggregateReader.Close()
	if retainedAggregateErr != nil || string(retainedAggregate) != expectedAggregate {
		t.Fatalf("retained aggregate = %q, error = %v", retainedAggregate, retainedAggregateErr)
	}

	var outputMu sync.Mutex
	running, err := backend.StartJob(context.Background(), allocation, agentexecution.JobSpec{
		JobID: "cancel", Command: "/bin/sh", Args: []string{"-c", "sleep 60 & wait"}, Timeout: time.Minute,
	}, func(chunk agentexecution.OutputChunk) error {
		outputMu.Lock()
		defer outputMu.Unlock()
		return nil
	})
	if err != nil {
		t.Fatalf("start cancellable job: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := running.Cancel(context.Background()); err != nil {
		t.Fatalf("cancel job: %v", err)
	}
	waitContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	canceled, err := running.Wait(waitContext)
	if err != nil {
		t.Fatalf("wait canceled job: %v", err)
	}
	if !canceled.Canceled {
		t.Fatalf("canceled result = %+v", canceled)
	}
}

func runContractJob(t *testing.T, backend *localruntime.Backend, allocation agentexecution.Allocation, spec agentexecution.JobSpec) (agentexecution.JobResult, []byte) {
	t.Helper()
	var mu sync.Mutex
	var output bytes.Buffer
	running, err := backend.StartJob(context.Background(), allocation, spec, func(chunk agentexecution.OutputChunk) error {
		mu.Lock()
		defer mu.Unlock()
		_, err := output.Write(chunk.Data)
		return err
	})
	if err != nil {
		t.Fatalf("start job %s: %v", spec.JobID, err)
	}
	waitContext, cancel := context.WithTimeout(context.Background(), spec.Timeout+5*time.Second)
	defer cancel()
	result, err := running.Wait(waitContext)
	if err != nil {
		t.Fatalf("wait job %s: %v", spec.JobID, err)
	}
	mu.Lock()
	defer mu.Unlock()
	return result, append([]byte(nil), output.Bytes()...)
}
