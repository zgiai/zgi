//go:build agent_runtime_integration

package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

type workerCSVAttachmentReader struct {
	files map[string][]byte
	reads int
}

func (r *workerCSVAttachmentReader) DownloadFile(_ context.Context, fileID string) ([]byte, error) {
	r.reads++
	data, exists := r.files[fileID]
	if !exists {
		return nil, fmt.Errorf("test attachment %s is missing", fileID)
	}
	return append([]byte(nil), data...), nil
}

type workerCSVArtifactStore struct{ data []byte }

func (s *workerCSVArtifactStore) Persist(_ context.Context, _ agentExecutionScope, _, _ string, data []byte) (map[string]interface{}, error) {
	s.data = append([]byte(nil), data...)
	return map[string]interface{}{"artifact_id": "tool_file:test-file", "file_id": "test-file", "size": len(data)}, nil
}

func TestAgentExecutionWorkerCSVContract(t *testing.T) {
	baseURL := strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_TEST_URL"))
	apiKey := strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_TEST_API_KEY"))
	profileID := strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_TEST_PROFILE"))
	backend := strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_TEST_BACKEND"))
	if baseURL == "" || apiKey == "" || profileID == "" || backend == "" {
		t.Fatal("ZGI_AGENT_RUNTIME_TEST_URL, _API_KEY, _PROFILE, and _BACKEND are required")
	}
	gateway := &agentExecutionHTTPGateway{baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey, client: &http.Client{Timeout: 90 * time.Second}}
	scope := agentExecutionScope{OrganizationID: uuid.NewString(), SubjectID: uuid.NewString()}
	agentID, conversationID, messageID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	binding, err := gateway.CreateBinding(ctx, scope, agentID, conversationID, profileID, backend)
	if err != nil || binding.Status != "active" {
		t.Fatalf("create worker binding = %#v, error = %v", binding, err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cleanupCancel()
		latest, err := gateway.GetBinding(cleanupCtx, scope, binding.ID)
		if err != nil {
			t.Errorf("read binding for cleanup: %v", err)
			return
		}
		var deleted agentExecutionBinding
		requestPath := fmt.Sprintf("bindings/%s?generation=%d", binding.ID, latest.Generation)
		if err := gateway.request(cleanupCtx, scope, http.MethodDelete, requestPath, nil, &deleted); err != nil {
			t.Errorf("destroy test binding: %v", err)
		}
	})

	firstID, secondID := uuid.NewString(), uuid.NewString()
	first := []byte("item,qty\napple,2\nbanana,1\n")
	second := []byte("item,qty\napple,3\nbanana,4\n")
	reader := &workerCSVAttachmentReader{files: map[string][]byte{firstID: first, secondID: second}}
	attachments := []runtimeservice.RuntimeAttachmentRef{
		{ID: firstID, Name: "sales_a.csv", Size: int64(len(first))},
		{ID: secondID, Name: "sales_b.csv", Size: int64(len(second))},
	}
	shell := newAgentExecutionShellTool(gateway, scope, agentID, conversationID, messageID, profileID, backend, attachments, reader)
	const aggregateCommand = `python3 - <<'PY'
import csv
from collections import Counter
from pathlib import Path
totals = Counter()
for source_path in sorted(Path('inputs').glob('*.csv')):
    with source_path.open(newline='') as source:
        for row in csv.DictReader(source):
            totals[row['item']] += int(row['qty'])
Path('artifacts').mkdir(exist_ok=True)
with Path('artifacts/result.csv').open('w', newline='') as target:
    writer = csv.writer(target, lineterminator='\n')
    writer.writerow(('item', 'qty'))
    writer.writerows(sorted(totals.items()))
print('rows=' + str(len(totals)))
PY`
	arguments, _ := json.Marshal(map[string]any{"command": aggregateCommand, "timeout_seconds": 60})
	result := shell.Handler(ctx, adapter.ToolCall{ID: "aggregate", Function: adapter.FunctionCall{Arguments: string(arguments)}})
	if result.Error != nil || result.Status != "success" || result.ModelResult["stdout"] != "rows=2\n" || result.ModelResult["input_files"] == nil {
		t.Fatalf("CSV shell result = %#v", result)
	}
	if exitCode, ok := result.ModelResult["exit_code"].(*int); !ok || exitCode == nil || *exitCode != 0 {
		t.Fatalf("CSV shell exit code = %#v", result.ModelResult["exit_code"])
	}
	if reader.reads != 2 {
		t.Fatalf("authorized attachment reads = %d, want 2", reader.reads)
	}

	current, err := gateway.GetBinding(ctx, scope, binding.ID)
	if err != nil {
		t.Fatalf("read active binding: %v", err)
	}
	var suspended agentExecutionBinding
	if err := gateway.request(ctx, scope, http.MethodPost, "bindings/"+binding.ID+"/suspend", map[string]any{"generation": current.Generation}, &suspended); err != nil || suspended.Status != "suspended" {
		t.Fatalf("suspend worker binding = %#v, error = %v", suspended, err)
	}
	secondTurn := newAgentExecutionShellTool(gateway, scope, agentID, conversationID, uuid.NewString(), profileID, backend, nil, nil)
	verifyArgs, _ := json.Marshal(map[string]any{"command": "cat artifacts/result.csv", "timeout_seconds": 30})
	continued := secondTurn.Handler(ctx, adapter.ToolCall{ID: "verify", Function: adapter.FunctionCall{Arguments: string(verifyArgs)}})
	const expected = "item,qty\napple,5\nbanana,5\n"
	if continued.Error != nil || continued.ModelResult["stdout"] != expected {
		t.Fatalf("resumed shell result = %#v", continued)
	}

	persister := &workerCSVArtifactStore{}
	export := newAgentExecutionExportTool(gateway, persister, scope, agentID, conversationID, messageID, profileID, backend)
	exported := export.Handler(ctx, adapter.ToolCall{ID: "export", Function: adapter.FunctionCall{Arguments: `{"path":"artifacts/result.csv"}`}})
	if exported.Error != nil || exported.Status != "success" || string(persister.data) != expected || len(exported.Artifacts) != 1 {
		t.Fatalf("exported file = %#v, persisted = %q", exported, persister.data)
	}
}
