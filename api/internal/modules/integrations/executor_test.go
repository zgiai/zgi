package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestExecutorSuccessRunsPipelineAndAuditsResult(t *testing.T) {
	events := []string{}
	cost := 0.007
	adapter := &testAdapter{
		driverID: "test-driver",
		events:   &events,
		execute: func(_ context.Context, req ActionRequest) (*ActionResult, error) {
			if req.OrganizationID != testOrganizationID || req.Input["query"] != "current events" {
				t.Fatalf("adapter request = %#v", req)
			}
			return &ActionResult{
				Output:            map[string]interface{}{"ok": true},
				ProviderRequestID: "provider-request-1",
				CostUSD:           &cost,
				ResultCount:       3,
				AttemptCount:      1,
			}, nil
		},
	}
	registry := registerTestAction(t, testAction("web.search", "search_web"), adapter)
	quota := &testQuota{events: &events}
	audit := &testAudit{events: &events}
	safety := &testSafety{events: &events}
	executor := NewExecutor(registry, audit, quota, safety, []byte("audit-hmac-key"), time.Second)

	result, err := executor.Execute(context.Background(), validActionRequest("current events"))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result == nil || result.ProviderRequestID != "provider-request-1" || result.ResultCount != 3 {
		t.Fatalf("Execute() result = %#v", result)
	}
	if want := []string{"safety", "quota", "audit.create", "adapter", "audit.complete"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("pipeline events = %#v, want %#v", events, want)
	}
	if audit.created == nil || audit.created.Status != "running" || audit.created.IntegrationID != IntegrationWebSearch || audit.created.DriverID != "test-driver" || audit.created.ActionID != "web.search" {
		t.Fatalf("created audit = %#v", audit.created)
	}
	if audit.created.OrganizationID.String() != testOrganizationID || audit.created.AccountID == nil || audit.created.AccountID.String() != testUserID || audit.created.ConnectionID == nil || audit.created.ConnectionID.String() != testConnectionID {
		t.Fatalf("created audit scope = %#v", audit.created)
	}
	if audit.completion.Status != "succeeded" || audit.completion.ProviderRequestID != "provider-request-1" || audit.completion.ResultCount != 3 || audit.completion.AttemptCount != 1 || audit.completion.ErrorCode != "" {
		t.Fatalf("completion = %#v", audit.completion)
	}
}

func TestCompletionForResultUsesQueryableTimedOutStatus(t *testing.T) {
	completion := completionForResult(nil, 25, NewError(ErrorCodeTimeout, "provider timed out", nil))
	if completion.Status != "timed_out" || completion.ErrorCode != ErrorCodeTimeout {
		t.Fatalf("completion = %#v", completion)
	}
}

func TestCompletionForResultPersistsSafeDiagnosticsWithoutActionResult(t *testing.T) {
	retryAfter := time.Date(2026, 7, 28, 21, 0, 0, 0, time.UTC)
	callErr := NewProviderError(
		ErrorCodeRateLimited,
		"provider rate limited",
		errors.New(`provider response body: {"message":"secret"}`),
		ProviderDiagnostics{
			ErrorCode:    "99991400",
			RequestID:    "feishu-log-123",
			HTTPStatus:   429,
			RetryAfterAt: &retryAfter,
		},
	)
	completion := completionForResult(nil, 25, callErr)
	if completion.Status != "failed" ||
		completion.ErrorCode != ErrorCodeRateLimited ||
		completion.ProviderErrorCode != "99991400" ||
		completion.ProviderRequestID != "feishu-log-123" ||
		completion.ProviderHTTPStatus == nil ||
		*completion.ProviderHTTPStatus != 429 ||
		completion.RetryAfterAt == nil ||
		!completion.RetryAfterAt.Equal(retryAfter) {
		t.Fatalf("completion = %#v", completion)
	}
	encoded, err := json.Marshal(completion)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "provider response body") {
		t.Fatalf("completion leaked provider body: %s", encoded)
	}
}

func TestCompletionForResultKeepsLegacyProviderRequestID(t *testing.T) {
	completion := completionForResult(&ActionResult{ProviderRequestID: "legacy-request-id"}, 10, nil)
	if completion.ProviderRequestID != "legacy-request-id" {
		t.Fatalf("ProviderRequestID = %q", completion.ProviderRequestID)
	}
}

func TestExecutorInvalidInputSchemaStopsAfterSafetyAndBeforeQuota(t *testing.T) {
	adapter := &testAdapter{driverID: "test"}
	registry := registerTestAction(t, testAction("web.search", "search_web"), adapter)
	quota := &testQuota{}
	audit := &testAudit{}
	safety := &testSafety{}
	executor := NewExecutor(registry, audit, quota, safety, []byte("audit-key"), 0)
	req := validActionRequest("valid")
	req.Input = map[string]interface{}{}

	result, err := executor.Execute(context.Background(), req)
	if result != nil || ErrorCode(err) != ErrorCodeInvalidInput {
		t.Fatalf("Execute() result = %#v, error = %v, code = %q", result, err, ErrorCode(err))
	}
	if safety.calls != 1 || quota.calls != 0 || audit.createCalls != 0 || adapter.calls != 0 {
		t.Fatalf("calls after schema failure: safety=%d quota=%d audit=%d adapter=%d", safety.calls, quota.calls, audit.createCalls, adapter.calls)
	}
	feedback := ActionInputValidationFeedback(err)
	if feedback["reason_code"] != ActionValidationReasonSchemaMismatch || feedback["failure_stage"] != ActionValidationStagePreflight || feedback["provider_request_sent"] != false {
		t.Fatalf("schema failure feedback = %#v", feedback)
	}
}

func TestExecutorRejectsInvalidAuditContextBeforePipeline(t *testing.T) {
	adapter := &testAdapter{driverID: "test"}
	registry := registerTestAction(t, testAction("web.search", "search_web"), adapter)
	quota := &testQuota{}
	audit := &testAudit{}
	safety := &testSafety{}
	executor := NewExecutor(registry, audit, quota, safety, []byte("audit-key"), 0)
	req := validActionRequest("current events")
	req.UserID = "not-a-uuid"

	result, err := executor.Execute(context.Background(), req)
	if result != nil || ErrorCode(err) != ErrorCodeInvalidInput {
		t.Fatalf("Execute() result = %#v, error = %v, code = %q", result, err, ErrorCode(err))
	}
	if safety.calls != 0 || quota.calls != 0 || audit.createCalls != 0 || adapter.calls != 0 {
		t.Fatalf("calls after context rejection: safety=%d quota=%d audit=%d adapter=%d", safety.calls, quota.calls, audit.createCalls, adapter.calls)
	}
}

func TestExecutorCanonicalizesUUIDContextBeforeQuotaAndAdapter(t *testing.T) {
	adapter := &testAdapter{
		driverID: "test",
		execute: func(_ context.Context, req ActionRequest) (*ActionResult, error) {
			if req.OrganizationID != testOrganizationID || req.UserID != testUserID {
				t.Fatalf("adapter context was not canonicalized: %#v", req)
			}
			return &ActionResult{Output: map[string]interface{}{"ok": true}}, nil
		},
	}
	registry := registerTestAction(t, testAction("web.search", "search_web"), adapter)
	executor := NewExecutor(registry, &testAudit{}, &testQuota{}, nil, []byte("audit-key"), 0)
	req := validActionRequest("current events")
	req.OrganizationID = "{" + testOrganizationID + "}"
	req.UserID = "urn:uuid:" + testUserID

	if _, err := executor.Execute(context.Background(), req); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestExecutorRejectsUnsupportedCallerBeforePipeline(t *testing.T) {
	action := testAction("web.search", "search_web")
	action.SupportedCallers = []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat, tools.ToolInvokeFromAgent}
	adapter := &testAdapter{driverID: "test"}
	registry := registerTestAction(t, action, adapter)
	quota := &testQuota{}
	audit := &testAudit{}
	safety := &testSafety{}
	executor := NewExecutor(registry, audit, quota, safety, []byte("audit-key"), 0)
	req := validActionRequest("current events")
	req.InvokeFrom = tools.ToolInvokeFromWorkflow

	result, err := executor.Execute(context.Background(), req)
	if result != nil || ErrorCode(err) != ErrorCodeInvalidInput {
		t.Fatalf("Execute() result = %#v, error = %v, code = %q", result, err, ErrorCode(err))
	}
	if safety.calls != 0 || quota.calls != 0 || audit.createCalls != 0 || adapter.calls != 0 {
		t.Fatalf("calls after caller rejection: safety=%d quota=%d audit=%d adapter=%d", safety.calls, quota.calls, audit.createCalls, adapter.calls)
	}
}

func TestExecutorEnforcesProviderDefaultDisabledWithoutPolicyService(t *testing.T) {
	action := testAction("message.send", "send_message")
	action.DefaultPolicy = &DefaultActionPolicy{
		Enabled:           false,
		ApprovalPolicy:    toolgovernance.ApprovalPolicyAlwaysAsk,
		DataEgressAllowed: true,
	}
	adapter := &testAdapter{driverID: "test"}
	registry := registerTestAction(t, action, adapter)
	quota := &testQuota{}
	audit := &testAudit{}
	executor := NewExecutor(registry, audit, quota, nil, []byte("audit-key"), 0)
	request := validActionRequest("current events")
	request.ActionID = action.ID

	result, err := executor.Execute(context.Background(), request)
	if result != nil || ErrorCode(err) != ErrorCodeDisabled {
		t.Fatalf("Execute() result = %#v, error = %v, code = %q", result, err, ErrorCode(err))
	}
	if quota.calls != 0 || audit.createCalls != 0 || adapter.calls != 0 {
		t.Fatalf("calls after provider default rejection: quota=%d audit=%d adapter=%d", quota.calls, audit.createCalls, adapter.calls)
	}
}

func TestExecutorSensitiveInputStopsBeforeQuotaAndOutbound(t *testing.T) {
	action := testAction("web.search", "search_web")
	action.DataEgress = true
	adapter := &testAdapter{driverID: "test"}
	registry := registerTestAction(t, action, adapter)
	quota := &testQuota{}
	audit := &testAudit{}
	executor := NewExecutor(registry, audit, quota, nil, []byte("audit-key"), 0)

	result, err := executor.Execute(context.Background(), validActionRequest("Authorization: Bearer abcdefghijklmnopqrstuvwxyz"))
	if result != nil || ErrorCode(err) != ErrorCodeSensitiveInput {
		t.Fatalf("Execute() result = %#v, error = %v, code = %q", result, err, ErrorCode(err))
	}
	if quota.calls != 0 || audit.createCalls != 0 || adapter.calls != 0 {
		t.Fatalf("calls after sensitive input: quota=%d audit=%d adapter=%d", quota.calls, audit.createCalls, adapter.calls)
	}
}

func TestExecutorSensitiveInputIsBlockedBeforeSchemaErrorCanExposeIt(t *testing.T) {
	action := testAction("web.search", "search_web")
	adapter := &testAdapter{driverID: "test"}
	registry := registerTestAction(t, action, adapter)
	quota := &testQuota{}
	audit := &testAudit{}
	executor := NewExecutor(registry, audit, quota, nil, []byte("audit-key"), 0)
	req := validActionRequest("current events")
	req.Input["unexpected_secret"] = "Authorization: Bearer abcdefghijklmnopqrstuvwxyz"

	result, err := executor.Execute(context.Background(), req)
	if result != nil || ErrorCode(err) != ErrorCodeSensitiveInput {
		t.Fatalf("Execute() result = %#v, error = %v, code = %q", result, err, ErrorCode(err))
	}
	if quota.calls != 0 || audit.createCalls != 0 || adapter.calls != 0 {
		t.Fatalf("calls after sensitive schema-invalid input: quota=%d audit=%d adapter=%d", quota.calls, audit.createCalls, adapter.calls)
	}
}

func TestExecutorTimeoutBoundsSafetyPreflight(t *testing.T) {
	adapter := &testAdapter{driverID: "test"}
	registry := registerTestAction(t, testAction("web.search", "search_web"), adapter)
	quota := &testQuota{}
	audit := &testAudit{}
	executor := NewExecutor(registry, audit, quota, blockingSafety{}, []byte("audit-key"), 20*time.Millisecond)

	startedAt := time.Now()
	result, err := executor.Execute(context.Background(), validActionRequest("current events"))
	if result != nil || ErrorCode(err) != ErrorCodeTimeout {
		t.Fatalf("Execute() result = %#v, error = %v, code = %q", result, err, ErrorCode(err))
	}
	if elapsed := time.Since(startedAt); elapsed > time.Second {
		t.Fatalf("safety preflight exceeded executor timeout: %v", elapsed)
	}
	if quota.calls != 0 || audit.createCalls != 0 || adapter.calls != 0 {
		t.Fatalf("calls after preflight timeout: quota=%d audit=%d adapter=%d", quota.calls, audit.createCalls, adapter.calls)
	}
}

func TestExecutorQuotaExceededStopsBeforeAuditAndOutbound(t *testing.T) {
	adapter := &testAdapter{driverID: "test"}
	registry := registerTestAction(t, testAction("web.search", "search_web"), adapter)
	quota := &testQuota{err: ErrQuotaExceeded}
	audit := &testAudit{}
	executor := NewExecutor(registry, audit, quota, nil, []byte("audit-key"), 0)

	result, err := executor.Execute(context.Background(), validActionRequest("current events"))
	if result != nil || ErrorCode(err) != ErrorCodeQuotaExceeded {
		t.Fatalf("Execute() result = %#v, error = %v, code = %q", result, err, ErrorCode(err))
	}
	if !strings.Contains(err.Error(), "external integration daily limit") || strings.Contains(err.Error(), "web search") {
		t.Fatalf("quota error = %q", err.Error())
	}
	if quota.calls != 1 || audit.createCalls != 0 || adapter.calls != 0 {
		t.Fatalf("calls after quota rejection: quota=%d audit=%d adapter=%d", quota.calls, audit.createCalls, adapter.calls)
	}
}

func TestExecutorAuditCreateFailureStopsOutbound(t *testing.T) {
	adapter := &testAdapter{driverID: "test"}
	registry := registerTestAction(t, testAction("web.search", "search_web"), adapter)
	audit := &testAudit{createErr: errors.New("database unavailable")}
	executor := NewExecutor(registry, audit, &testQuota{}, nil, []byte("audit-key"), 0)

	result, err := executor.Execute(context.Background(), validActionRequest("current events"))
	if result != nil || ErrorCode(err) != ErrorCodeAuditFailed {
		t.Fatalf("Execute() result = %#v, error = %v, code = %q", result, err, ErrorCode(err))
	}
	if audit.createCalls != 1 || audit.completeCalls != 0 || adapter.calls != 0 {
		t.Fatalf("calls after audit create failure: create=%d complete=%d adapter=%d", audit.createCalls, audit.completeCalls, adapter.calls)
	}
}

func TestExecutorAdapterFailureFinalizesAudit(t *testing.T) {
	adapter := &testAdapter{
		driverID: "test",
		execute: func(context.Context, ActionRequest) (*ActionResult, error) {
			return &ActionResult{ProviderRequestID: "provider-request-2", AttemptCount: 3}, NewError(ErrorCodeRateLimited, "provider rate limited", nil)
		},
	}
	registry := registerTestAction(t, testAction("web.search", "search_web"), adapter)
	audit := &testAudit{}
	executor := NewExecutor(registry, audit, &testQuota{}, nil, []byte("audit-key"), 0)

	result, err := executor.Execute(context.Background(), validActionRequest("current events"))
	if result != nil || ErrorCode(err) != ErrorCodeRateLimited {
		t.Fatalf("Execute() result = %#v, error = %v, code = %q", result, err, ErrorCode(err))
	}
	if audit.completeCalls != 1 || audit.completion.Status != "failed" || audit.completion.ErrorCode != ErrorCodeRateLimited || audit.completion.ProviderRequestID != "provider-request-2" || audit.completion.AttemptCount != 3 {
		t.Fatalf("completion = %#v", audit.completion)
	}
}

func TestExecutorInvalidOutputSchemaFinalizesAudit(t *testing.T) {
	adapter := &testAdapter{
		driverID: "test",
		execute: func(context.Context, ActionRequest) (*ActionResult, error) {
			return &ActionResult{
				Output:            map[string]interface{}{"ok": "not-a-boolean"},
				ProviderRequestID: "provider-request-3",
				AttemptCount:      1,
			}, nil
		},
	}
	registry := registerTestAction(t, testAction("web.search", "search_web"), adapter)
	audit := &testAudit{}
	executor := NewExecutor(registry, audit, &testQuota{}, nil, []byte("audit-key"), 0)

	result, err := executor.Execute(context.Background(), validActionRequest("current events"))
	if result != nil || ErrorCode(err) != ErrorCodeResponseInvalid {
		t.Fatalf("Execute() result = %#v, error = %v, code = %q", result, err, ErrorCode(err))
	}
	if audit.completion.Status != "failed" || audit.completion.ErrorCode != ErrorCodeResponseInvalid || audit.completion.ProviderRequestID != "provider-request-3" {
		t.Fatalf("completion = %#v", audit.completion)
	}
}

func TestExecutorNormalizesTypedAdapterOutputBeforeSchemaValidation(t *testing.T) {
	action := testAction("folder.list", "list_folders")
	action.OutputSchema = map[string]interface{}{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]interface{}{
			"folders": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": false,
					"properties": map[string]interface{}{
						"name":       map[string]interface{}{"type": "string"},
						"attributes": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
						"ids":        map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "integer"}},
					},
					"required": []string{"name", "attributes", "ids"},
				},
			},
		},
		"required": []string{"folders"},
	}
	adapter := &testAdapter{
		driverID: "test",
		execute: func(context.Context, ActionRequest) (*ActionResult, error) {
			return &ActionResult{Output: map[string]interface{}{
				"folders": []map[string]interface{}{{
					"name": "INBOX", "attributes": []string{"\\HasNoChildren"}, "ids": []int{1, 2},
				}},
			}}, nil
		},
	}
	registry := registerTestAction(t, action, adapter)
	executor := NewExecutor(registry, &testAudit{}, &testQuota{}, nil, []byte("audit-key"), 0)
	req := validActionRequest("current events")
	req.ActionID = action.ID

	result, err := executor.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	folders, ok := result.Output["folders"].([]interface{})
	if !ok || len(folders) != 1 {
		t.Fatalf("folders = %T %#v", result.Output["folders"], result.Output["folders"])
	}
	folder, ok := folders[0].(map[string]interface{})
	if !ok {
		t.Fatalf("folder = %T %#v", folders[0], folders[0])
	}
	if _, ok := folder["attributes"].([]interface{}); !ok {
		t.Fatalf("attributes = %T %#v", folder["attributes"], folder["attributes"])
	}
	if _, ok := folder["ids"].([]interface{}); !ok {
		t.Fatalf("ids = %T %#v", folder["ids"], folder["ids"])
	}
}

func TestExecutorAuditCompleteFailureDoesNotDiscardSuccessfulRead(t *testing.T) {
	adapter := &testAdapter{driverID: "test"}
	registry := registerTestAction(t, testAction("web.search", "search_web"), adapter)
	audit := &testAudit{completeErr: errors.New("database unavailable")}
	outbox := &testCompletionOutbox{}
	executor := NewExecutor(registry, audit, &testQuota{}, nil, []byte("audit-key"), 0).WithCompletionOutbox(outbox)

	result, err := executor.Execute(context.Background(), validActionRequest("current events"))
	if result == nil || err != nil {
		t.Fatalf("Execute() result = %#v, error = %v, code = %q", result, err, ErrorCode(err))
	}
	if adapter.calls != 1 || audit.completeCalls != 3 {
		t.Fatalf("adapter calls = %d, complete calls = %d", adapter.calls, audit.completeCalls)
	}
	if outbox.enqueueCalls != 1 || outbox.enqueued.Completion.Status != "succeeded" {
		t.Fatalf("completion outbox = %#v", outbox)
	}
}

func TestExecutorAuditCompleteAndOutboxFailureFailsClosed(t *testing.T) {
	adapter := &testAdapter{driverID: "test"}
	registry := registerTestAction(t, testAction("web.search", "search_web"), adapter)
	audit := &testAudit{completeErr: errors.New("database unavailable")}
	executor := NewExecutor(registry, audit, &testQuota{}, nil, []byte("audit-key"), 0).
		WithCompletionOutbox(&testCompletionOutbox{enqueueErr: errors.New("redis unavailable")})

	result, err := executor.Execute(context.Background(), validActionRequest("current events"))
	if result != nil || ErrorCode(err) != ErrorCodeAuditFailed {
		t.Fatalf("Execute() result = %#v, error = %v, code = %q", result, err, ErrorCode(err))
	}
}

func TestExecutorAuditHMACContainsNoRawInput(t *testing.T) {
	const rawInput = "confidential-search-phrase"
	adapter := &testAdapter{driverID: "test"}
	registry := registerTestAction(t, testAction("web.search", "search_web"), adapter)
	audit := &testAudit{}
	executor := NewExecutor(registry, audit, &testQuota{}, nil, []byte("audit-key"), 0)

	if _, err := executor.Execute(context.Background(), validActionRequest(rawInput)); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if audit.created == nil || audit.created.InputHMAC == nil || len(*audit.created.InputHMAC) != 64 {
		t.Fatalf("audit input HMAC = %#v", audit.created)
	}
	if strings.Contains(*audit.created.InputHMAC, rawInput) {
		t.Fatalf("input HMAC %q contains raw input", *audit.created.InputHMAC)
	}
	encoded, err := json.Marshal(audit.created)
	if err != nil {
		t.Fatalf("json.Marshal(audit) error = %v", err)
	}
	if strings.Contains(string(encoded), rawInput) {
		t.Fatalf("audit record contains raw input: %s", encoded)
	}
	want, err := inputFingerprint([]byte("audit-key"), map[string]interface{}{"query": rawInput})
	if err != nil {
		t.Fatalf("inputFingerprint() error = %v", err)
	}
	if *audit.created.InputHMAC != want {
		t.Fatalf("input HMAC = %q, want deterministic %q", *audit.created.InputHMAC, want)
	}
}

func registerTestAction(t *testing.T, action ActionDefinition, adapter Adapter) *Registry {
	t.Helper()
	registry := NewRegistry()
	if err := registry.Register(localizedTestRegistration(IntegrationWebSearch, adapter, []ActionDefinition{action})); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	return registry
}

const (
	testOrganizationID = "11111111-1111-4111-8111-111111111111"
	testUserID         = "22222222-2222-4222-8222-222222222222"
	testWorkspaceID    = "33333333-3333-4333-8333-333333333333"
	testConnectionID   = "44444444-4444-4444-8444-444444444444"
)

func validActionRequest(query string) ActionRequest {
	return ActionRequest{
		OrganizationID: testOrganizationID,
		WorkspaceID:    testWorkspaceID,
		UserID:         testUserID,
		ConnectionID:   testConnectionID,
		InvokeFrom:     tools.ToolInvokeFromAIChat,
		IntegrationID:  IntegrationWebSearch,
		ActionID:       ActionWebSearch,
		Input:          map[string]interface{}{"query": query},
	}
}

type testQuota struct {
	calls  int
	err    error
	events *[]string
}

func (q *testQuota) Acquire(_ context.Context, organizationID string) error {
	q.calls++
	appendTestEvent(q.events, "quota")
	if organizationID != testOrganizationID {
		return errors.New("unexpected organization")
	}
	return q.err
}

type testSafety struct {
	calls  int
	err    error
	events *[]string
}

type blockingSafety struct{}

func (blockingSafety) Check(ctx context.Context, _ ActionDefinition, _ map[string]interface{}) error {
	<-ctx.Done()
	return ctx.Err()
}

func (s *testSafety) Check(_ context.Context, _ ActionDefinition, _ map[string]interface{}) error {
	s.calls++
	appendTestEvent(s.events, "safety")
	return s.err
}

type testAudit struct {
	createCalls   int
	completeCalls int
	createErr     error
	completeErr   error
	created       *ExecutionRecord
	completedID   uuid.UUID
	completion    ExecutionCompletion
	events        *[]string
}

type testCompletionOutbox struct {
	enqueueCalls int
	enqueued     PendingExecutionCompletion
	enqueueErr   error
}

func (o *testCompletionOutbox) Enqueue(_ context.Context, pending PendingExecutionCompletion) error {
	o.enqueueCalls++
	o.enqueued = pending
	return o.enqueueErr
}

func (o *testCompletionOutbox) Claim(context.Context, int64) (ExecutionCompletionClaim, error) {
	return ExecutionCompletionClaim{}, nil
}

func (o *testCompletionOutbox) Delete(context.Context, uuid.UUID) error { return nil }

func (a *testAudit) Create(_ context.Context, record *ExecutionRecord) error {
	a.createCalls++
	appendTestEvent(a.events, "audit.create")
	if record != nil {
		copy := *record
		a.created = &copy
	}
	return a.createErr
}

func (a *testAudit) Complete(_ context.Context, id uuid.UUID, completion ExecutionCompletion) error {
	a.completeCalls++
	appendTestEvent(a.events, "audit.complete")
	a.completedID = id
	a.completion = completion
	return a.completeErr
}
