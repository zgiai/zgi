//go:build e2b_integration && linux

package e2b

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
	"github.com/zgiai/zgi-sandbox/internal/workspacecheckpoint"
)

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
}
