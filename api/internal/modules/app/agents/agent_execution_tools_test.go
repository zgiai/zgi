package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/dto"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

type fakeAgentExecutionGateway struct {
	binding      agentExecutionBinding
	job          agentExecutionJob
	output       []agentExecutionOutputChunk
	capabilities agentExecutionCapabilities
	command      string
	args         []string
	scope        agentExecutionScope
	uploads      map[string][]byte
	downloads    map[string][]byte
	resumeCalls  int
	setupJobs    int
}

type fakeRuntimeAttachmentReader struct {
	files map[string][]byte
	calls int
}

type fakeAgentExecutionArtifactPersister struct {
	path string
	data []byte
}

func (p *fakeAgentExecutionArtifactPersister) Persist(_ context.Context, _ agentExecutionScope, _, filePath string, data []byte) (map[string]interface{}, error) {
	p.path = filePath
	p.data = append([]byte(nil), data...)
	return map[string]interface{}{
		"artifact_id": "tool_file:file-1", "file_id": "file-1", "tool_file_id": "file-1",
		"filename": "result.csv", "size": len(data), "download_url": "/files/file-1",
	}, nil
}

func (r *fakeRuntimeAttachmentReader) DownloadFile(_ context.Context, fileID string) ([]byte, error) {
	r.calls++
	return append([]byte(nil), r.files[fileID]...), nil
}

func (f *fakeAgentExecutionGateway) CreateBinding(_ context.Context, scope agentExecutionScope, _, _, _, _ string) (agentExecutionBinding, error) {
	f.scope = scope
	return f.binding, nil
}

func (f *fakeAgentExecutionGateway) GetBinding(context.Context, agentExecutionScope, string) (agentExecutionBinding, error) {
	return f.binding, nil
}

func (f *fakeAgentExecutionGateway) ResumeBinding(_ context.Context, _ agentExecutionScope, bindingID string, generation int64) (agentExecutionBinding, error) {
	if bindingID != f.binding.ID || generation != f.binding.Generation {
		return agentExecutionBinding{}, fmt.Errorf("stale binding generation")
	}
	f.resumeCalls++
	f.binding.Status = "active"
	f.binding.Generation++
	return f.binding, nil
}

func (f *fakeAgentExecutionGateway) Capabilities(context.Context, agentExecutionScope, string) (agentExecutionCapabilities, error) {
	if f.capabilities.NetworkMode != "" || f.capabilities.Shell {
		return f.capabilities, nil
	}
	return agentExecutionCapabilities{Shell: true, NetworkMode: "disabled"}, nil
}

func (f *fakeAgentExecutionGateway) SubmitJob(_ context.Context, _ agentExecutionScope, _, _, command string, args []string, _ time.Duration) (agentExecutionJob, error) {
	f.command, f.args = command, append([]string(nil), args...)
	if command == "python3" && len(args) == 7 && args[0] == "-c" && args[1] == agentSkillPackageUnpackProgram {
		f.setupJobs++
		if f.downloads == nil {
			f.downloads = make(map[string][]byte)
		}
		markerPath := strings.TrimPrefix(args[3], "/workspace/") + "/.zgi-ready"
		f.downloads[markerPath] = []byte(args[5] + "\n")
	}
	return f.job, nil
}

func (f *fakeAgentExecutionGateway) GetJob(context.Context, agentExecutionScope, string) (agentExecutionJob, error) {
	return f.job, nil
}

func (f *fakeAgentExecutionGateway) CancelJob(context.Context, agentExecutionScope, string, int64) error {
	return nil
}

func (f *fakeAgentExecutionGateway) JobOutput(_ context.Context, _ agentExecutionScope, _ string, after int64, _ int) ([]agentExecutionOutputChunk, error) {
	if after >= 0 {
		return nil, nil
	}
	return f.output, nil
}

func (f *fakeAgentExecutionGateway) UploadFile(_ context.Context, _ agentExecutionScope, _, filePath string, data []byte) error {
	if f.uploads == nil {
		f.uploads = make(map[string][]byte)
	}
	f.uploads[filePath] = append([]byte(nil), data...)
	return nil
}

func (f *fakeAgentExecutionGateway) DownloadFile(_ context.Context, _ agentExecutionScope, _, filePath string) ([]byte, error) {
	return append([]byte(nil), f.downloads[filePath]...), nil
}

func TestAgentExecutionBindingResumesAfterIdleSuspension(t *testing.T) {
	fake := &fakeAgentExecutionGateway{binding: agentExecutionBinding{ID: "binding-1", Status: "suspended", Generation: 4, Recreated: true}}
	binding, err := readyAgentExecutionBinding(t.Context(), fake, agentExecutionScope{OrganizationID: "org-1", SubjectID: "account-1"}, "agent-1", "conversation-1", "standard", "local-gvisor")
	if err != nil || binding.Status != "active" || binding.Generation != 5 || fake.resumeCalls != 1 || !binding.Recreated {
		t.Fatalf("resumed binding = %#v, calls=%d, err=%v", binding, fake.resumeCalls, err)
	}
	fake.binding.Status = "deleted"
	if _, err := readyAgentExecutionBinding(t.Context(), fake, agentExecutionScope{OrganizationID: "org-1", SubjectID: "account-1"}, "agent-1", "conversation-1", "standard", "local-gvisor"); err == nil {
		t.Fatal("deleted binding with no replacement must not be reused")
	}
}

func TestAgentExecutionHTTPGatewayReplacesDeletedBindingIdempotently(t *testing.T) {
	var keys []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/agent-runtime/bindings" {
			t.Errorf("binding request = %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			IdempotencyKey string `json:"idempotency_key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode binding request: %v", err)
		}
		keys = append(keys, body.IdempotencyKey)
		switch body.IdempotencyKey {
		case "conversation:conversation-1":
			_, _ = io.WriteString(w, `{"data":{"id":"binding-old","status":"deleted","generation":5}}`)
		case "conversation:conversation-1:after:binding-old":
			_, _ = io.WriteString(w, `{"data":{"id":"binding-new","status":"active","generation":2}}`)
		default:
			t.Errorf("unexpected idempotency key %q", body.IdempotencyKey)
		}
	}))
	defer server.Close()
	gateway := &agentExecutionHTTPGateway{baseURL: server.URL, apiKey: "dedicated-worker-key", client: server.Client()}
	for range 2 {
		binding, err := gateway.CreateBinding(t.Context(), agentExecutionScope{OrganizationID: "org-1", SubjectID: "account-1"}, "agent-1", "conversation-1", "standard", "local-gvisor")
		if err != nil || binding.ID != "binding-new" || !binding.Recreated {
			t.Fatalf("replacement binding = %#v, err=%v", binding, err)
		}
	}
	want := []string{"conversation:conversation-1", "conversation:conversation-1:after:binding-old", "conversation:conversation-1", "conversation:conversation-1:after:binding-old"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("binding keys = %v, want %v", keys, want)
	}
}

func TestAgentExecutionShellToolKeepsCommandAndOutputOutOfTrace(t *testing.T) {
	exitCode := 2
	fake := &fakeAgentExecutionGateway{
		binding: agentExecutionBinding{ID: "binding-1", Status: "active"},
		job:     agentExecutionJob{ID: "job-1", Status: "failed", ExitCode: &exitCode},
		output:  []agentExecutionOutputChunk{{Sequence: 0, Stream: "stdout", Data: []byte("private output")}, {Sequence: 1, Stream: "stderr", Data: []byte("private diagnostics")}},
	}
	tool := newAgentExecutionShellTool(fake, agentExecutionScope{OrganizationID: "org-1", SubjectID: "account-1"}, "agent-1", "conversation-1", "message-1", "standard", "local-gvisor", nil, nil)
	result := tool.Handler(t.Context(), adapter.ToolCall{ID: "call-1", Function: adapter.FunctionCall{Arguments: `{"command":"echo private command","timeout_seconds":2}`}})
	if result.Error != nil || result.Status != "success" {
		t.Fatalf("tool result = %#v", result)
	}
	if fake.command != "/bin/sh" || !reflect.DeepEqual(fake.args, []string{"-lc", "echo private command"}) {
		t.Fatalf("submitted command = %q %#v", fake.command, fake.args)
	}
	if fake.scope != (agentExecutionScope{OrganizationID: "org-1", SubjectID: "account-1"}) {
		t.Fatalf("worker scope = %#v", fake.scope)
	}
	trace, _ := json.Marshal(map[string]any{"arguments": result.Arguments, "result": result.Result})
	if strings.Contains(string(trace), "private") {
		t.Fatalf("trace contains private command or output: %s", trace)
	}
	if result.ModelResult["stdout"] != "private output" || result.ModelResult["stderr"] != "private diagnostics" {
		t.Fatalf("model-visible output = %#v", result.ModelResult)
	}
}

func TestAgentExecutionShellToolRejectsNetworkedProfile(t *testing.T) {
	fake := &fakeAgentExecutionGateway{
		binding:      agentExecutionBinding{ID: "binding-1", Status: "active"},
		capabilities: agentExecutionCapabilities{Shell: true, NetworkMode: "enabled"},
	}
	result := executeAgentShellTool(t.Context(), fake, agentExecutionScope{OrganizationID: "org-1", SubjectID: "account-1"}, "agent-1", "conversation-1", "message-1", "standard", "local-gvisor", adapter.ToolCall{ID: "call-1", Function: adapter.FunctionCall{Arguments: `{"command":"echo test"}`}}, nil)
	if result.Status != "failed" || result.Result["error_code"] != "capability_denied" || fake.command != "" {
		t.Fatalf("networked runtime was allowed: %#v", result)
	}
}

func TestAgentExecutionShellStagesAuthorizedAttachmentsOnce(t *testing.T) {
	fileID := uuid.NewString()
	reader := &fakeRuntimeAttachmentReader{files: map[string][]byte{fileID: []byte("a,b\n1,2\n")}}
	exitCode := 0
	fake := &fakeAgentExecutionGateway{
		binding:      agentExecutionBinding{ID: "binding-1", Status: "active"},
		job:          agentExecutionJob{ID: "job-1", Status: "succeeded", ExitCode: &exitCode},
		capabilities: agentExecutionCapabilities{Shell: true, FileTransfer: true, NetworkMode: "disabled", MaxUploadBytes: 1024},
	}
	tool := newAgentExecutionShellTool(fake, agentExecutionScope{OrganizationID: "org-1", SubjectID: "account-1"}, "agent-1", "conversation-1", "message-1", "standard", "local-gvisor", []runtimeservice.RuntimeAttachmentRef{{ID: fileID, Name: "sales.csv", Size: 8}}, reader)
	for i := 0; i < 2; i++ {
		result := tool.Handler(t.Context(), adapter.ToolCall{ID: uuid.NewString(), Function: adapter.FunctionCall{Arguments: `{"command":"ls inputs"}`}})
		if result.Error != nil {
			t.Fatalf("shell call %d: %v", i, result.Error)
		}
		paths, ok := result.ModelResult["input_files"].([]string)
		if !ok || len(paths) != 1 || paths[0] != "inputs/"+fileID+".csv" {
			t.Fatalf("staged paths = %#v", result.ModelResult["input_files"])
		}
	}
	if reader.calls != 1 || string(fake.uploads["inputs/"+fileID+".csv"]) != "a,b\n1,2\n" {
		t.Fatalf("attachment staging calls=%d uploads=%#v", reader.calls, fake.uploads)
	}
}

func TestAgentExecutionAttachmentStagingChecksCapabilityAndSize(t *testing.T) {
	fileID := uuid.NewString()
	fake := &fakeAgentExecutionGateway{}
	reader := &fakeRuntimeAttachmentReader{files: map[string][]byte{fileID: []byte("abcd")}}
	refs := []runtimeservice.RuntimeAttachmentRef{{ID: fileID, Name: "data.csv", Size: 4}}
	if _, err := stageAgentExecutionAttachments(t.Context(), fake, agentExecutionScope{}, "binding-1", agentExecutionCapabilities{FileTransfer: false, MaxUploadBytes: 1024}, refs, reader); err == nil {
		t.Fatal("file transfer without capability was allowed")
	}
	if _, err := stageAgentExecutionAttachments(t.Context(), fake, agentExecutionScope{}, "binding-1", agentExecutionCapabilities{FileTransfer: true, MaxUploadBytes: 3}, refs, reader); err == nil {
		t.Fatal("oversized file transfer was allowed")
	}
	if reader.calls != 0 || len(fake.uploads) != 0 {
		t.Fatal("rejected file was downloaded or uploaded")
	}
}

func TestConsoleAgentExecutionToolFactoryIsCallerScoped(t *testing.T) {
	agentID := uuid.New()
	orgID := uuid.New()
	accountID := uuid.New()
	h := &AgentsHandler{
		executionApprovals: agentExecutionApprovals{GatewayReady: true, Profiles: map[string]agentExecutionApproval{
			"standard": {Backend: "local-gvisor", OrganizationIDs: []string{orgID.String()}, PolicyIDs: []string{"offline"}},
		}},
		executionGateway: &fakeAgentExecutionGateway{},
	}
	scope := runtimeservice.Scope{OrganizationID: orgID, AccountID: accountID}
	cfg := dto.AgentConfigResponse{ExecutionRuntime: &dto.AgentExecutionRuntime{Enabled: true, ProfileID: "standard", PolicyID: "offline"}}
	runConfig := runtimeservice.RunConfig{}
	if err := h.attachConsoleAgentExecutionTools(&runConfig, scope, agentID, cfg); err != nil {
		t.Fatalf("attach console tools: %v", err)
	}
	prepared := &runtimeservice.PreparedChat{
		Scope:        scope,
		Caller:       runtimeservice.Caller{Type: runtimemodel.ConversationCallerAgent, ID: &agentID, Source: runtimemodel.ConversationSourceConsole},
		Conversation: &runtimemodel.Conversation{ID: uuid.New()}, Message: &runtimemodel.Message{ID: uuid.New()},
	}
	if got := runConfig.RuntimeToolFactory(prepared); len(got) != 2 || got[0].Definition.Function.Name != "shell_exec" || got[1].Definition.Function.Name != "workspace_export_file" {
		t.Fatalf("console tools = %#v", got)
	}
	prepared.Caller.Source = runtimemodel.ConversationSourceWebApp
	if got := runConfig.RuntimeToolFactory(prepared); len(got) != 0 {
		t.Fatalf("WebApp received console tools: %#v", got)
	}
	prepared.Caller.Source = runtimemodel.ConversationSourceConsole
	prepared.Scope.AccountID = uuid.New()
	if got := runConfig.RuntimeToolFactory(prepared); len(got) != 0 {
		t.Fatalf("different account received console tools: %#v", got)
	}
}

func TestAgentExecutionExportCreatesDownloadArtifactWithoutTraceContent(t *testing.T) {
	fake := &fakeAgentExecutionGateway{
		binding:      agentExecutionBinding{ID: "binding-1", Status: "active"},
		capabilities: agentExecutionCapabilities{FileTransfer: true, NetworkMode: "disabled", MaxDownloadBytes: 1024},
		downloads:    map[string][]byte{"artifacts/result.csv": []byte("private,output\n")},
	}
	persister := &fakeAgentExecutionArtifactPersister{}
	tool := newAgentExecutionExportTool(fake, persister, agentExecutionScope{OrganizationID: "org-1", SubjectID: "account-1"}, "agent-1", "conversation-1", "message-1", "standard", "local-gvisor")
	result := tool.Handler(t.Context(), adapter.ToolCall{ID: "call-1", Function: adapter.FunctionCall{Arguments: `{"path":"artifacts/result.csv"}`}})
	if result.Error != nil || result.Status != "success" || persister.path != "artifacts/result.csv" || string(persister.data) != "private,output\n" {
		t.Fatalf("export result = %#v, persister=%#v", result, persister)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0]["conversation_id"] != "conversation-1" || result.Artifacts[0]["message_id"] != "message-1" {
		t.Fatalf("artifact event = %#v", result.Artifacts)
	}
	trace, _ := json.Marshal(map[string]interface{}{"arguments": result.Arguments, "result": result.Result})
	if strings.Contains(string(trace), "private") || strings.Contains(string(trace), "result.csv") {
		t.Fatalf("trace contains file content or path: %s", trace)
	}
	for _, unsafe := range []string{"../secret", "/etc/passwd", "..", "artifacts/../secret"} {
		bad := tool.Handler(t.Context(), adapter.ToolCall{Function: adapter.FunctionCall{Arguments: `{"path":"` + unsafe + `"}`}})
		if bad.Result["error_code"] != "invalid_path" {
			t.Fatalf("unsafe path %q accepted: %#v", unsafe, bad)
		}
	}
}

func TestAgentExecutionExportRejectsPreviousFileAfterWorkspaceRecreation(t *testing.T) {
	fake := &fakeAgentExecutionGateway{binding: agentExecutionBinding{ID: "binding-new", Status: "active", Recreated: true}}
	persister := &fakeAgentExecutionArtifactPersister{}
	tool := newAgentExecutionExportTool(fake, persister, agentExecutionScope{OrganizationID: "org-1", SubjectID: "account-1"}, "agent-1", "conversation-1", "message-1", "standard", "e2b")
	result := tool.Handler(t.Context(), adapter.ToolCall{Function: adapter.FunctionCall{Arguments: `{"path":"artifacts/result.csv"}`}})
	if result.Result["error_code"] != "workspace_recreated" || persister.path != "" {
		t.Fatalf("export after recreation = %#v, persister=%#v", result, persister)
	}
}

func TestAgentExecutionHTTPGatewayUsesDedicatedScopeAndCredential(t *testing.T) {
	const key = "dedicated-worker-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+key || r.Header.Get("X-ZGI-Organization-ID") != "org-1" || r.Header.Get("X-ZGI-Subject-ID") != "account-1" {
			t.Errorf("missing dedicated auth or scope")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/v1/agent-runtime/bindings" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode binding request: %v", err)
		}
		if body["profile_id"] != "standard" || body["backend"] != "local-gvisor" || body["idempotency_key"] != "conversation:conversation-1" {
			t.Errorf("binding body = %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":"binding-1","status":"active","generation":1}}`))
	}))
	defer server.Close()
	gateway := &agentExecutionHTTPGateway{baseURL: server.URL, apiKey: key, client: server.Client()}
	binding, err := gateway.CreateBinding(t.Context(), agentExecutionScope{OrganizationID: "org-1", SubjectID: "account-1"}, "agent-1", "conversation-1", "standard", "local-gvisor")
	if err != nil || binding.ID != "binding-1" {
		t.Fatalf("CreateBinding = %#v, %v", binding, err)
	}
}

func TestAgentExecutionHTTPGatewayDecodesWorkerOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/agent-runtime/jobs/job-1/output" || r.URL.Query().Get("after") != "-1" {
			t.Errorf("output request = %s %s", r.Method, r.URL.String())
		}
		if r.Header.Get("Authorization") != "Bearer dedicated-worker-key" {
			t.Error("dedicated credential missing")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"sequence":0,"stream":"stdout","data":"aGVsbG8="},{"sequence":1,"stream":"stderr","data":"d2FybmluZw=="}]}`))
	}))
	defer server.Close()
	gateway := &agentExecutionHTTPGateway{baseURL: server.URL, apiKey: "dedicated-worker-key", client: server.Client()}
	chunks, err := gateway.JobOutput(t.Context(), agentExecutionScope{OrganizationID: "org-1", SubjectID: "account-1"}, "job-1", -1, 32)
	if err != nil || len(chunks) != 2 || string(chunks[0].Data) != "hello" || string(chunks[1].Data) != "warning" {
		t.Fatalf("JobOutput = %#v, %v", chunks, err)
	}
}

func TestAgentExecutionHTTPGatewayUploadsRawFileToWorkspace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/v1/agent-runtime/bindings/binding-1/files" || r.URL.Query().Get("path") != "inputs/data.csv" {
			t.Errorf("upload request = %s %s", r.Method, r.URL.String())
		}
		if r.Header.Get("Authorization") != "Bearer dedicated-worker-key" || r.Header.Get("X-ZGI-Organization-ID") != "org-1" || r.Header.Get("X-ZGI-Subject-ID") != "account-1" {
			t.Error("upload authentication or scope missing")
		}
		var body []byte
		body, _ = io.ReadAll(r.Body)
		if string(body) != "raw,csv\n" || r.ContentLength != int64(len(body)) {
			t.Errorf("upload payload = %q, content-length=%d", body, r.ContentLength)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	gateway := &agentExecutionHTTPGateway{baseURL: server.URL, apiKey: "dedicated-worker-key", client: server.Client()}
	if err := gateway.UploadFile(t.Context(), agentExecutionScope{OrganizationID: "org-1", SubjectID: "account-1"}, "binding-1", "inputs/data.csv", []byte("raw,csv\n")); err != nil {
		t.Fatalf("UploadFile: %v", err)
	}
}

func TestAgentExecutionHTTPGatewayDownloadsBoundedFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/agent-runtime/bindings/binding-1/files" || r.URL.Query().Get("path") != "artifacts/result.csv" {
			t.Errorf("download request = %s %s", r.Method, r.URL.String())
		}
		if r.Header.Get("Authorization") != "Bearer dedicated-worker-key" || r.Header.Get("X-ZGI-Organization-ID") != "org-1" || r.Header.Get("X-ZGI-Subject-ID") != "account-1" {
			t.Error("download authentication or scope missing")
		}
		w.Header().Set("Content-Length", "8")
		_, _ = w.Write([]byte("raw,csv\n"))
	}))
	defer server.Close()
	gateway := &agentExecutionHTTPGateway{baseURL: server.URL, apiKey: "dedicated-worker-key", client: server.Client()}
	data, err := gateway.DownloadFile(t.Context(), agentExecutionScope{OrganizationID: "org-1", SubjectID: "account-1"}, "binding-1", "artifacts/result.csv")
	if err != nil || string(data) != "raw,csv\n" {
		t.Fatalf("DownloadFile = %q, %v", data, err)
	}
}

func TestAgentExecutionHTTPGatewayRejectsUnboundedFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "67108865")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	gateway := &agentExecutionHTTPGateway{baseURL: server.URL, apiKey: "dedicated-worker-key", client: server.Client()}
	_, err := gateway.DownloadFile(t.Context(), agentExecutionScope{OrganizationID: "org-1", SubjectID: "account-1"}, "binding-1", "artifacts/result.csv")
	if err == nil || !strings.Contains(err.Error(), "exceeds the limit") {
		t.Fatalf("expected file size limit, got %v", err)
	}
}

func TestAgentExecutionHTTPGatewayResumesBindingWithGeneration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/agent-runtime/bindings/binding-1/resume" {
			t.Errorf("resume request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer dedicated-worker-key" || r.Header.Get("X-ZGI-Organization-ID") != "org-1" || r.Header.Get("X-ZGI-Subject-ID") != "account-1" {
			t.Error("resume authentication or scope missing")
		}
		var body struct {
			Generation int64 `json:"generation"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Generation != 4 {
			t.Errorf("resume generation = %d, err=%v", body.Generation, err)
		}
		_, _ = io.WriteString(w, `{"data":{"id":"binding-1","status":"active","generation":5}}`)
	}))
	defer server.Close()
	gateway := &agentExecutionHTTPGateway{baseURL: server.URL, apiKey: "dedicated-worker-key", client: server.Client()}
	binding, err := gateway.ResumeBinding(t.Context(), agentExecutionScope{OrganizationID: "org-1", SubjectID: "account-1"}, "binding-1", 4)
	if err != nil || binding.Status != "active" || binding.Generation != 5 {
		t.Fatalf("ResumeBinding = %#v, %v", binding, err)
	}
}
