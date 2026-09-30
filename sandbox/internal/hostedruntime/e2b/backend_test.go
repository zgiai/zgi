package e2b

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

func TestBackendUsesSecuredOfflineE2BContract(t *testing.T) {
	var mu sync.Mutex
	var created createSandboxRequest
	var agentToken string
	var uploadedToken string
	connectCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.HasPrefix(request.URL.Path, "/v2/") || strings.HasPrefix(request.URL.Path, "/sandboxes/") {
			if request.Header.Get("X-API-KEY") != "provider-secret" {
				http.Error(writer, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/v2/sandboxes":
			if err := json.NewDecoder(request.Body).Decode(&created); err != nil {
				http.Error(writer, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			agentToken = created.Environment[agentTokenEnvironment]
			mu.Unlock()
			writeJSON(writer, http.StatusCreated, sandboxConnection{
				SandboxID: "sandbox-1", Domain: "e2b.test",
				EnvdAccessToken: "envd-create", TrafficAccessToken: "traffic-create",
			})
		case request.Method == http.MethodPost && request.URL.Path == "/files":
			if request.Header.Get("X-Access-Token") != "envd-create" || request.Header.Get("E2b-Sandbox-Port") != "49983" || request.URL.Query().Get("path") != runtimeAgentBootstrapTokenPath || request.URL.Query().Get("username") != runtimeAgentUser {
				http.Error(writer, "bad bootstrap upload", http.StatusUnauthorized)
				return
			}
			value, err := io.ReadAll(request.Body)
			if err != nil {
				http.Error(writer, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			uploadedToken = string(value)
			mu.Unlock()
			writeJSON(writer, http.StatusOK, []map[string]string{{"path": runtimeAgentBootstrapTokenPath}})
		case request.Method == http.MethodPost && request.URL.Path == "/v2/sandboxes/sandbox-1/connect":
			mu.Lock()
			connectCalls++
			mu.Unlock()
			writeJSON(writer, http.StatusOK, sandboxConnection{
				SandboxID: "sandbox-1", Domain: "e2b.test",
				EnvdAccessToken: "envd-connect", TrafficAccessToken: "traffic-connect",
			})
		case request.Method == http.MethodGet && request.URL.Path == "/envs":
			if request.Header.Get("X-Access-Token") != "envd-connect" || request.Header.Get("E2b-Sandbox-Port") != "49983" {
				http.Error(writer, "bad envd authentication", http.StatusUnauthorized)
				return
			}
			mu.Lock()
			token := agentToken
			mu.Unlock()
			writeJSON(writer, http.StatusOK, map[string]string{agentTokenEnvironment: token})
		case request.Method == http.MethodGet && request.URL.Path == "/v1/capabilities":
			mu.Lock()
			token := agentToken
			mu.Unlock()
			if request.Header.Get("Authorization") != "Bearer "+token || request.Header.Get("E2b-Traffic-Access-Token") == "" || request.Header.Get("E2b-Sandbox-Port") != "49984" {
				http.Error(writer, "bad Runtime Agent authentication", http.StatusUnauthorized)
				return
			}
			writeJSON(writer, http.StatusOK, map[string]any{"data": agentexecution.RuntimeCapabilities{
				Shell: true, FileTransfer: true, PTY: true, NetworkMode: "disabled",
			}})
		case request.Method == http.MethodPost && request.URL.Path == "/sandboxes/sandbox-1/pause":
			writeJSON(writer, http.StatusConflict, map[string]string{"message": "already paused"})
		case request.Method == http.MethodDelete && request.URL.Path == "/sandboxes/sandbox-1":
			writer.WriteHeader(http.StatusNotFound)
		default:
			http.Error(writer, fmt.Sprintf("unexpected %s %s", request.Method, request.URL.Path), http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	backend, err := New(Options{
		APIURL: server.URL, APIKey: "provider-secret",
		Profiles:   map[string]Profile{"standard": {TemplateID: "template-zgi-runtime"}},
		HTTPClient: server.Client(), ReadyTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("new backend: %v", err)
	}
	backend.client.sandboxURL = func(uint16, string, string) string { return server.URL }
	spec := agentexecution.CreateSpec{BindingID: "binding-1", WorkspaceID: "workspace-1", ProfileID: "standard"}
	allocation, err := backend.CreateBinding(context.Background(), spec)
	if err != nil {
		t.Fatalf("create binding: %v", err)
	}
	if allocation.BindingRef != "sandbox-1" || allocation.WorkspaceRef != "sandbox-1" {
		t.Fatalf("allocation = %+v", allocation)
	}
	if created.TemplateID != "template-zgi-runtime" || created.AllowInternet || created.Network.AllowPublicTraffic || !created.AutoPause || !created.AutoPauseMemory || created.AutoResume.Enabled || len(created.Network.DenyOut) != 1 || created.Network.DenyOut[0] != "0.0.0.0/0" {
		t.Fatalf("unsafe create request = %+v", created)
	}
	if created.Metadata["zgi.binding_id"] != "binding-1" || created.Metadata["zgi.profile_id"] != "standard" || len(created.Metadata) != 2 {
		t.Fatalf("metadata = %#v", created.Metadata)
	}
	if agentToken == "" || agentToken == "provider-secret" {
		t.Fatalf("agent token was not independently generated")
	}
	if uploadedToken != agentToken {
		t.Fatalf("bootstrap token was not uploaded through envd")
	}
	if capabilities, err := backend.Capabilities(context.Background(), allocation); err != nil || capabilities.NetworkMode != "disabled" {
		t.Fatalf("capabilities=%+v error=%v", capabilities, err)
	}
	if err := backend.SuspendBinding(context.Background(), allocation); err != nil {
		t.Fatalf("suspend idempotently: %v", err)
	}
	resumed, err := backend.ResumeBinding(context.Background(), spec, allocation)
	if err != nil || resumed != allocation {
		t.Fatalf("resume=%+v error=%v", resumed, err)
	}
	if err := backend.DestroyBinding(context.Background(), allocation); err != nil {
		t.Fatalf("destroy idempotently: %v", err)
	}
	mu.Lock()
	gotConnectCalls := connectCalls
	mu.Unlock()
	if gotConnectCalls != 2 {
		t.Fatalf("connect calls = %d, want 2", gotConnectCalls)
	}
}

func TestDiscoverAllocationAttemptIncludesPausedSandboxes(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/v2/sandboxes" || request.Header.Get("X-API-KEY") != "provider-secret" {
			http.Error(writer, "unexpected control request", http.StatusBadRequest)
			return
		}
		if request.URL.Query().Get("metadata") != "zgi.attempt_id=ara-test" {
			http.Error(writer, "unfiltered allocation list", http.StatusBadRequest)
			return
		}
		requests++
		if requests == 1 {
			writer.Header().Set("X-Next-Token", "next")
			writeJSON(writer, http.StatusOK, []map[string]any{{"sandboxID": "paused-1", "state": "paused", "metadata": map[string]string{"zgi.attempt_id": "ara-test"}}})
			return
		}
		if request.URL.Query().Get("nextToken") != "next" {
			http.Error(writer, "wrong pagination token", http.StatusBadRequest)
			return
		}
		writeJSON(writer, http.StatusOK, []map[string]any{{"sandboxID": "running-2", "state": "running", "metadata": map[string]string{"zgi.attempt_id": "ara-test"}}})
	}))
	defer server.Close()
	backend, err := New(Options{APIURL: server.URL, APIKey: "provider-secret", Profiles: map[string]Profile{"standard": {TemplateID: "template"}}, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	allocations, err := backend.FindAllocations(context.Background(), "ara-test")
	if err != nil || len(allocations) != 2 || allocations[0].BindingRef != "paused-1" || allocations[1].BindingRef != "running-2" {
		t.Fatalf("allocations = %+v, %v", allocations, err)
	}
}

func TestBackendClassifiesProviderLimitsWithoutReturningBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusTooManyRequests)
		_, _ = writer.Write([]byte(`{"message":"secret-provider-detail"}`))
	}))
	t.Cleanup(server.Close)
	backend, err := New(Options{
		APIURL: server.URL, APIKey: "provider-secret", HTTPClient: server.Client(),
		Profiles: map[string]Profile{"standard": {TemplateID: "template"}}, ReadyTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("new backend: %v", err)
	}
	_, err = backend.CreateBinding(context.Background(), agentexecution.CreateSpec{BindingID: "binding", ProfileID: "standard"})
	if !errors.Is(err, agentexecution.ErrBackendUnavailable) || strings.Contains(fmt.Sprint(err), "secret-provider-detail") {
		t.Fatalf("create error = %v", err)
	}
}

func TestBackendRejectsUnapprovedProfilesAndInvalidConfiguration(t *testing.T) {
	if _, err := New(Options{}); !errors.Is(err, agentexecution.ErrInvalidRequest) {
		t.Fatalf("empty config error = %v", err)
	}
	backend, err := New(Options{
		APIKey: "provider-secret", Profiles: map[string]Profile{"standard": {TemplateID: "template"}},
	})
	if err != nil {
		t.Fatalf("new backend: %v", err)
	}
	_, err = backend.CreateBinding(context.Background(), agentexecution.CreateSpec{ProfileID: "unknown"})
	if !errors.Is(err, agentexecution.ErrInvalidRequest) {
		t.Fatalf("unapproved profile error = %v", err)
	}
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
