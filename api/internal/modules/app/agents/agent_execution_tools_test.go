package agents

import (
	"context"
	"encoding/json"
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
}

func (f *fakeAgentExecutionGateway) CreateBinding(_ context.Context, scope agentExecutionScope, _, _, _, _ string) (agentExecutionBinding, error) {
	f.scope = scope
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

func TestAgentExecutionShellToolKeepsCommandAndOutputOutOfTrace(t *testing.T) {
	exitCode := 2
	fake := &fakeAgentExecutionGateway{
		binding: agentExecutionBinding{ID: "binding-1", Status: "active"},
		job:     agentExecutionJob{ID: "job-1", Status: "failed", ExitCode: &exitCode},
		output:  []agentExecutionOutputChunk{{Sequence: 0, Stream: "stdout", Data: []byte("private output")}, {Sequence: 1, Stream: "stderr", Data: []byte("private diagnostics")}},
	}
	tool := newAgentExecutionShellTool(fake, agentExecutionScope{OrganizationID: "org-1", SubjectID: "account-1"}, "agent-1", "conversation-1", "message-1", "standard", "local-gvisor")
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
	result := executeAgentShellTool(t.Context(), fake, agentExecutionScope{OrganizationID: "org-1", SubjectID: "account-1"}, "agent-1", "conversation-1", "message-1", "standard", "local-gvisor", adapter.ToolCall{ID: "call-1", Function: adapter.FunctionCall{Arguments: `{"command":"echo test"}`}})
	if result.Status != "failed" || result.Result["error_code"] != "capability_denied" || fake.command != "" {
		t.Fatalf("networked runtime was allowed: %#v", result)
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
	if got := runConfig.RuntimeToolFactory(prepared); len(got) != 1 || got[0].Definition.Function.Name != "shell_exec" {
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
