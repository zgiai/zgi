package executionruntime

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeplatform"
)

type fakePlatformGateway struct {
	*fakeAgentExecutionGateway
	requests    []runtimeplatform.Request
	responses   []agentExecutionPlatformResponse
	submissions int
	queueError  error
}

func (g *fakePlatformGateway) SubmitPlatformJob(ctx context.Context, scope agentExecutionScope, bindingID, key, command string, args []string, timeout time.Duration) (agentExecutionJob, error) {
	g.submissions++
	return g.SubmitJob(ctx, scope, bindingID, key, command, args, timeout)
}
func (g *fakePlatformGateway) UploadPlatformFile(ctx context.Context, scope agentExecutionScope, jobID, path string, data []byte) error {
	return g.UploadFile(ctx, scope, g.job.BindingID, path, data)
}
func (g *fakePlatformGateway) DownloadPlatformFile(ctx context.Context, scope agentExecutionScope, jobID, path string) ([]byte, error) {
	return g.DownloadFile(ctx, scope, g.job.BindingID, path)
}

func TestPlatformRelayBindsPostSubmissionGeneration(t *testing.T) {
	relay, gateway := newTestPlatformRelay(t)
	// Submission renews activity and increments the repository CAS generation.
	gateway.job.BindingGeneration = 2
	if _, err := relay.SubmitJob(t.Context(), agentExecutionScope{OrganizationID: "org-1", SubjectID: "user-1"}, "binding-1", "tool:message:call", "/bin/sh", nil, time.Minute); err != nil {
		t.Fatal(err)
	}
	gateway.binding.Generation = 2
	if relay.scope.BindingGeneration != 2 || relay.authorizeRequest(t.Context(), relay.scope, runtimeplatform.ReadFile) != nil {
		t.Fatal("activity renewal invalidated new grant")
	}
	// A later external write or lifecycle change must still invalidate it.
	gateway.binding.Generation = 3
	if relay.authorizeRequest(t.Context(), relay.scope, runtimeplatform.ReadFile) == nil {
		t.Fatal("later generation change was accepted")
	}
}
func (g *fakePlatformGateway) PlatformRequests(context.Context, agentExecutionScope, string) ([]runtimeplatform.Request, error) {
	requests := g.requests
	g.requests = nil
	return requests, g.queueError
}

func TestPlatformRelayWaitsForProcessQueueWithoutDispatchOrReplay(t *testing.T) {
	relay, gateway := newTestPlatformRelay(t)
	calls := 0
	relay.services.Model = func(context.Context, string, int) (string, error) { calls++; return "answer", nil }
	scope := agentExecutionScope{OrganizationID: "org-1", SubjectID: "user-1"}
	if _, err := relay.SubmitJob(t.Context(), scope, "binding-1", "tool:message:call", "/bin/sh", nil, time.Minute); err != nil {
		t.Fatal(err)
	}
	gateway.queueError = agentExecutionGatewayHTTPError{statusCode: 404, code: "not_found"}
	if job, err := relay.GetJob(t.Context(), scope, "job-1"); err != nil || job.Status != "running" || calls != 0 || len(gateway.responses) != 0 || gateway.cancelCalls != 0 {
		t.Fatalf("startup gap: job=%+v err=%v calls=%d", job, err, calls)
	}
	gateway.queueError = nil
	request := runtimeplatform.Request{ID: "model", Action: runtimeplatform.CallModel, Arguments: json.RawMessage(`{"prompt":"text","max_tokens":10}`)}
	gateway.requests = []runtimeplatform.Request{request}
	if _, err := relay.GetJob(t.Context(), scope, "job-1"); err != nil || calls != 1 {
		t.Fatalf("ready: calls=%d err=%v", calls, err)
	}
	gateway.queueError = agentExecutionGatewayHTTPError{statusCode: 404, code: "not_found"}
	if _, err := relay.GetJob(t.Context(), scope, "job-1"); err != nil || calls != 1 {
		t.Fatal("missing queue replayed the action or rejected terminal publication gap")
	}
	for _, queueErr := range []error{agentExecutionGatewayHTTPError{statusCode: 409, code: "generation_conflict"}, errors.New("control connection lost")} {
		gateway.queueError = queueErr
		if _, err := relay.GetJob(t.Context(), scope, "job-1"); err == nil {
			t.Fatal("unsafe queue failure was suppressed")
		}
	}
	gateway.job.Status = "succeeded"
	if job, err := relay.GetJob(t.Context(), scope, "job-1"); err != nil || job.Status != "succeeded" || calls != 1 {
		t.Fatal("terminal status lost")
	}
}
func (g *fakePlatformGateway) ResolvePlatformRequest(_ context.Context, _ agentExecutionScope, _ string, response agentExecutionPlatformResponse) error {
	g.responses = append(g.responses, response)
	return nil
}

func newTestPlatformRelay(t *testing.T) (*agentPlatformRelay, *fakePlatformGateway) {
	t.Helper()
	gateway := &fakePlatformGateway{fakeAgentExecutionGateway: &fakeAgentExecutionGateway{binding: agentExecutionBinding{ID: "binding-1", Status: "active", Generation: 1}, job: agentExecutionJob{ID: "job-1", BindingID: "binding-1", BindingGeneration: 1, Generation: 1, Status: "running"}, capabilities: agentExecutionCapabilities{Shell: true, NetworkMode: "disabled", PlatformProtocolVersion: 1}}}
	relay := &agentPlatformRelay{agentExecutionGateway: gateway, platform: gateway, scope: platformTestScope(), authorize: func(context.Context) error { return nil }}
	t.Cleanup(func() { relay.Close(t.Context()) })
	return relay, gateway
}

func TestPlatformRelayDispatchesOnceAndFencesChangedBinding(t *testing.T) {
	relay, gateway := newTestPlatformRelay(t)
	modelCalls := 0
	relay.services.Model = func(context.Context, string, int) (string, error) { modelCalls++; return "answer", nil }
	scope := agentExecutionScope{OrganizationID: "org-1", SubjectID: "user-1"}
	if _, err := relay.SubmitJob(t.Context(), scope, "binding-1", "tool:message-1:call-1", "/bin/sh", nil, time.Minute); err != nil {
		t.Fatal(err)
	}
	request := runtimeplatform.Request{ID: "model-1", Action: runtimeplatform.CallModel, Arguments: json.RawMessage(`{"prompt":"text","max_tokens":100}`)}
	for range 2 {
		gateway.requests = []runtimeplatform.Request{request}
		if _, err := relay.GetJob(t.Context(), scope, "job-1"); err != nil {
			t.Fatal(err)
		}
	}
	if modelCalls != 1 || len(gateway.responses) != 2 || gateway.responses[0].Status != runtimeplatform.Completed {
		t.Fatalf("calls=%d responses=%+v", modelCalls, gateway.responses)
	}
	gateway.binding.Generation++
	gateway.requests = []runtimeplatform.Request{{ID: "model-2", Action: request.Action, Arguments: request.Arguments}}
	if _, err := relay.GetJob(t.Context(), scope, "job-1"); err != nil {
		t.Fatal(err)
	}
	if modelCalls != 1 || gateway.responses[2].Status != runtimeplatform.Failed || len(gateway.responses[2].Data) != 0 {
		t.Fatal("stale generation executed or exposed response")
	}
}

func TestPlatformRelayApprovalStopsJobBeforeNextRequest(t *testing.T) {
	relay, gateway := newTestPlatformRelay(t)
	calls := 0
	relay.services.Tool = func(context.Context, string, string, json.RawMessage) (skillloop.RuntimePlatformToolResult, error) {
		calls++
		return skillloop.RuntimePlatformToolResult{PendingGovernance: map[string]interface{}{"correlation_id": "approval-1"}}, nil
	}
	scope := agentExecutionScope{OrganizationID: "org-1", SubjectID: "user-1"}
	if _, err := relay.SubmitJob(t.Context(), scope, "binding-1", "tool:message-1:call-1", "/bin/sh", nil, time.Minute); err != nil {
		t.Fatal(err)
	}
	request := runtimeplatform.Request{ID: "tool-1", Action: runtimeplatform.CallTool, Arguments: json.RawMessage(`{"name":"bound","arguments":{}}`)}
	gateway.requests = []runtimeplatform.Request{request, {ID: "tool-2", Action: request.Action, Arguments: request.Arguments}}
	if _, err := relay.GetJob(t.Context(), scope, "job-1"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || gateway.cancelCalls != 1 || len(gateway.responses) != 1 || gateway.responses[0].Status != runtimeplatform.AwaitingApproval {
		t.Fatalf("calls=%d cancels=%d responses=%+v", calls, gateway.cancelCalls, gateway.responses)
	}
	if _, err := relay.GetJob(t.Context(), scope, "job-1"); err != nil || calls != 1 {
		t.Fatal("approval did not fence later requests")
	}
}
