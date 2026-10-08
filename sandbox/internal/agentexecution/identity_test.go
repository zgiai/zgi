package agentexecution

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testExecutionIdentity() *ExecutionIdentity {
	return &ExecutionIdentity{Version: 1, WorkspaceID: "resource-workspace", ApplicationType: "agent",
		ApplicationID: "agent-1", Entry: "webapp", SubjectKind: "account",
		ConversationID: "conversation-1", PublicationRef: "published-v1"}
}

func TestExecutionIdentityRejectsIncompleteOrAmbiguousScope(t *testing.T) {
	mutations := map[string]func(*ExecutionIdentity){
		"future version":       func(i *ExecutionIdentity) { i.Version = 2 },
		"workspace":            func(i *ExecutionIdentity) { i.WorkspaceID = "" },
		"application":          func(i *ExecutionIdentity) { i.ApplicationID = "" },
		"publication":          func(i *ExecutionIdentity) { i.PublicationRef = "" },
		"conversation":         func(i *ExecutionIdentity) { i.ConversationID = "" },
		"runtime workspace":    func(i *ExecutionIdentity) { i.ApplicationType = "runtime_workspace" },
		"task":                 func(i *ExecutionIdentity) { i.Entry = "task" },
		"anonymous":            func(i *ExecutionIdentity) { i.SubjectKind = "anonymous" },
		"api account fallback": func(i *ExecutionIdentity) { i.Entry = "external-api" },
		"spoof console":        func(i *ExecutionIdentity) { i.Entry = "console"; i.SubjectKind = "api_end_user" },
		"control character":    func(i *ExecutionIdentity) { i.PublicationRef = "version\x00other" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			identity := testExecutionIdentity()
			mutate(identity)
			if err := identity.Validate(); !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("validation = %v", err)
			}
		})
	}
	for _, value := range []string{"null", "{}", `{"version":1}`, `{} {}`, strings.Repeat("x", 4097)} {
		if _, err := DecodeExecutionIdentity(value); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("invalid JSON identity accepted: %q", value[:min(32, len(value))])
		}
	}
	encoded, err := json.Marshal(testExecutionIdentity())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeExecutionIdentity(strings.TrimSuffix(string(encoded), "}") + `,"unexpected":true}`); err == nil {
		t.Fatal("unknown identity field accepted")
	}
}

func TestExecutionIdentitySeparatesApplicationsEntriesUsersAndConversations(t *testing.T) {
	service := newTestService(t, newMemoryRepository(), &fakeBackend{})
	request := testCreateRequest()
	request.Scope.Identity = testExecutionIdentity()
	binding, err := service.CreateBinding(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if again, err := service.CreateBinding(context.Background(), request); err != nil || again.ID != binding.ID {
		t.Fatalf("repeat create = %+v, %v", again, err)
	}
	mutations := map[string]func(*Scope){
		"legacy":        func(s *Scope) { s.Identity = nil },
		"user":          func(s *Scope) { s.SubjectID = "other-user" },
		"organization":  func(s *Scope) { s.OrganizationID = "other-org" },
		"workspace":     func(s *Scope) { s.Identity.WorkspaceID = "other-workspace" },
		"application":   func(s *Scope) { s.Identity.ApplicationID = "other-agent" },
		"digital human": func(s *Scope) { s.Identity.ApplicationType = "digital_human" },
		"entry":         func(s *Scope) { s.Identity.Entry = "console" },
		"conversation":  func(s *Scope) { s.Identity.ConversationID = "other-conversation" },
		"publication":   func(s *Scope) { s.Identity.PublicationRef = "published-v2" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			scope := request.Scope
			scope.Identity = cloneExecutionIdentity(scope.Identity)
			mutate(&scope)
			if _, err := service.GetBinding(context.Background(), scope, binding.ID); !errors.Is(err, ErrNotFound) {
				t.Fatalf("cross-identity binding lookup = %v", err)
			}
			if _, err := service.GetBindingByIdempotency(context.Background(), scope, request.IdempotencyKey); !errors.Is(err, ErrNotFound) {
				t.Fatalf("cross-identity idempotency lookup = %v", err)
			}
		})
	}
	// The caller cannot mutate the persisted identity after creation.
	request.Scope.Identity.Entry = "console"
	if binding.Identity.Entry != "webapp" {
		t.Fatal("binding retained a caller-owned identity pointer")
	}
}

func TestDigitalHumanExecutionIdentityNeverUsesAgentID(t *testing.T) {
	service := newTestService(t, newMemoryRepository(), &fakeBackend{})
	request := testCreateRequest()
	request.Scope.Identity = testExecutionIdentity()
	request.Scope.Identity.ApplicationType = "digital_human"
	request.Scope.Identity.ApplicationID = "digital-human-1"
	if _, err := service.CreateBinding(context.Background(), request); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("DH impersonating Agent = %v", err)
	}
	request.AgentID = ""
	if binding, err := service.CreateBinding(context.Background(), request); err != nil || binding.AgentID != "" {
		t.Fatalf("DH create = %+v, %v", binding, err)
	}
}

func TestExecutionIdentityHTTPAndJobOwnership(t *testing.T) {
	api := newTestHTTPAPI(t)
	call := func(method, path, body, identity string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, agentRuntimePathPrefix+path, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer runtime-secret")
		request.Header.Set("X-ZGI-Organization-ID", "org-1")
		request.Header.Set("X-ZGI-Subject-ID", "user-1")
		if identity != "" {
			request.Header.Set(ExecutionIdentityHeader, identity)
		}
		response := httptest.NewRecorder()
		api.ServeHTTP(response, request)
		return response
	}
	encoded, err := json.Marshal(testExecutionIdentity())
	if err != nil {
		t.Fatal(err)
	}
	identity := string(encoded)
	created := call(http.MethodPost, "bindings", `{"agent_id":"agent-1","conversation_id":"conversation-1","profile_id":"profile-1","backend":"fake","idempotency_key":"identity-test"}`, identity)
	if created.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", created.Code, created.Body)
	}
	bindingID := responseDataString(t, created.Body.Bytes(), "id")
	submitted := call(http.MethodPost, "bindings/"+bindingID+"/jobs", `{"command":"python3","idempotency_key":"job"}`, identity)
	if submitted.Code != http.StatusAccepted {
		t.Fatalf("submit = %d %s", submitted.Code, submitted.Body)
	}
	jobID := responseDataString(t, submitted.Body.Bytes(), "id")
	for _, other := range []string{"", strings.Replace(identity, "webapp", "console", 1)} {
		for _, path := range []string{"bindings/" + bindingID, "jobs/" + jobID, "jobs/" + jobID + "/output"} {
			if response := call(http.MethodGet, path, "", other); response.Code != http.StatusNotFound {
				t.Fatalf("cross-identity %s = %d %s", path, response.Code, response.Body)
			}
		}
	}
	if response := call(http.MethodGet, "bindings/"+bindingID, "", "null"); response.Code != http.StatusBadRequest {
		t.Fatalf("malformed header = %d", response.Code)
	}
	if response := call(http.MethodGet, "release", "", ""); !strings.Contains(response.Body.String(), `"execution_identity_version":1`) {
		t.Fatalf("identity capability missing: %s", response.Body)
	}
}

func TestLegacyBindingFingerprintAndIdentityIsolation(t *testing.T) {
	request := normalizeCreateRequest(testCreateRequest())
	legacyHash := requestFingerprint(request)
	request.Scope.Identity = testExecutionIdentity()
	if requestFingerprint(request) == legacyHash {
		t.Fatal("new identity was not included in idempotency fingerprint")
	}
	repository := newMemoryRepository()
	service := newTestService(t, repository, &fakeBackend{})
	request.Scope.Identity = nil
	binding, err := service.CreateBinding(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	// Historical offline hashes must continue to resume without renaming files.
	binding.RequestHash = legacyRequestFingerprint(request)
	repository.bindings[binding.ID] = binding
	if _, err := service.CreateBinding(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	request.Scope.Identity = testExecutionIdentity()
	if _, err := service.CreateBinding(context.Background(), request); !errors.Is(err, ErrNotFound) {
		t.Fatalf("new entry adopted legacy environment: %v", err)
	}
}
