package executionruntime

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestStrictGatewayReleaseRequiresIdentity(t *testing.T) {
	t.Setenv("ZGI_AGENT_RUNTIME_URL", "http://127.0.0.1:2670")
	t.Setenv("ZGI_AGENT_RUNTIME_API_KEY", "test-key")
	t.Setenv("ZGI_AGENT_RUNTIME_RELEASE_REQUIRED", "true")
	t.Setenv("ZGI_AGENT_RUNTIME_RELEASE_ID", "")
	gateway := agentExecutionGatewayFromEnv().(*agentExecutionHTTPGateway)
	scope := agentExecutionScope{OrganizationID: "org", SubjectID: "subject"}
	if err := gateway.request(t.Context(), scope, http.MethodPost, "bindings", nil, nil); err == nil {
		t.Fatal("strict mode accepted missing identity")
	}
	t.Setenv("ZGI_AGENT_RUNTIME_RELEASE_ID", "release-1")
	if agentExecutionGatewayFromEnv() == nil {
		t.Fatal("strict mode rejected valid identity")
	}
	t.Setenv("ZGI_AGENT_RUNTIME_RELEASE_ID", "bad\nheader")
	gateway = agentExecutionGatewayFromEnv().(*agentExecutionHTTPGateway)
	if err := gateway.request(t.Context(), scope, http.MethodPost, "bindings", nil, nil); err == nil || gateway.releaseID != "" {
		t.Fatal("invalid release header accepted")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"data":{}}`)) }))
	defer server.Close()
	gateway.baseURL, gateway.client = server.URL, server.Client()
	if err := gateway.request(t.Context(), scope, http.MethodGet, "jobs/existing", nil, nil); err != nil {
		t.Fatalf("existing resource query blocked: %v", err)
	}
	if err := gateway.request(t.Context(), scope, http.MethodPost, "jobs/existing/cancel", nil, nil); err != nil {
		t.Fatalf("existing cancellation blocked: %v", err)
	}
}

func TestGatewayReleaseGateRejectsOldAndMismatchedWorker(t *testing.T) {
	for _, test := range []struct {
		name, identity string
		status         int
		admit          bool
	}{
		{"matching", "release-1", 200, true},
		{"mismatch", "release-2", 200, false},
		{"missing", "", 200, false},
		{"old-worker", "", 404, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var admitted atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-ZGI-Runtime-Release") != "release-1" {
					t.Error("missing operator release header")
				}
				if r.URL.Path == "/v1/agent-runtime/release" {
					w.WriteHeader(test.status)
					_, _ = w.Write([]byte(`{"data":{"id":"` + test.identity + `","contract_version":1,"platform_protocol_version":1}}`))
					return
				}
				admitted.Add(1)
				_, _ = w.Write([]byte(`{"data":{}}`))
			}))
			defer server.Close()
			gateway := &agentExecutionHTTPGateway{baseURL: server.URL, apiKey: "test-key", releaseID: "release-1", client: server.Client()}
			scope := agentExecutionScope{OrganizationID: "org", SubjectID: "subject"}
			err := gateway.request(t.Context(), scope, http.MethodPost, "bindings/id/jobs", map[string]string{"command": "true"}, nil)
			if (err == nil) != test.admit || (admitted.Load() == 1) != test.admit {
				t.Fatalf("admitted=%d error=%v", admitted.Load(), err)
			}
			// Recovery/query operations do not require matching release metadata.
			if err := gateway.request(t.Context(), scope, http.MethodGet, "jobs/id", nil, nil); err != nil {
				t.Fatal(err)
			}
		})
	}
}
