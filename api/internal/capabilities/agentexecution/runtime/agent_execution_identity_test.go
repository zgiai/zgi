package executionruntime

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
)

func TestExecutionIdentityNegotiatesBeforeQueriesJobsAndFiles(t *testing.T) {
	for _, version := range []int{0, 1, 2} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			var calls atomic.Int32
			identity := &execution.Identity{Version: 1, WorkspaceID: "workspace", ApplicationType: "agent", ApplicationID: "agent",
				Entry: "webapp", SubjectKind: "account", ConversationID: "conversation", PublicationRef: "published-v1"}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/agent-runtime/release" {
					if r.Header.Get(execution.IdentityHeader) != "" {
						t.Error("negotiation requires legacy scope")
					}
					_, _ = fmt.Fprintf(w, `{"data":{"execution_identity_version":%d}}`, version)
					return
				}
				calls.Add(1)
				var received execution.Identity
				if err := json.Unmarshal([]byte(r.Header.Get(execution.IdentityHeader)), &received); err != nil || received != *identity {
					t.Error("identity missing or changed")
				}
				switch r.Method {
				case http.MethodPut:
					w.WriteHeader(http.StatusCreated)
				case http.MethodGet:
					if r.URL.Query().Has("path") {
						w.Header().Set("Content-Length", "2")
						_, _ = w.Write([]byte("ok"))
						return
					}
				}
				_, _ = w.Write([]byte(`{"data":{}}`))
			}))
			defer server.Close()
			gateway := &agentExecutionHTTPGateway{baseURL: server.URL, apiKey: "key", client: server.Client()}
			scope := agentExecutionScope{OrganizationID: "organization", SubjectID: "user", Identity: identity}
			operations := []func() error{
				func() error { return gateway.request(t.Context(), scope, http.MethodGet, "jobs/job", nil, nil) },
				func() error { return gateway.request(t.Context(), scope, http.MethodPost, "bindings/b/jobs", nil, nil) },
				func() error { return gateway.UploadFile(t.Context(), scope, "binding", "input.txt", []byte("input")) },
				func() error { _, err := gateway.DownloadFile(t.Context(), scope, "binding", "output.txt"); return err },
			}
			for _, operation := range operations {
				if err := operation(); (err == nil) != (version == 1) {
					t.Fatalf("version %d: %v", version, err)
				}
			}
			if version != 1 && calls.Load() != 0 {
				t.Fatal("unsupported Worker received identity-scoped operation")
			}
		})
	}
}

func TestBackgroundIdentityRequiresSeparateWorkerNegotiation(t *testing.T) {
	for _, version := range []int{0, 1, 2, 3} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/agent-runtime/release" {
					_, _ = fmt.Fprintf(w, `{"data":{"execution_identity_version":1,"background_execution_identity_version":%d}}`, version)
					return
				}
				calls.Add(1)
				_, _ = w.Write([]byte(`{"data":{}}`))
			}))
			defer server.Close()
			gateway := &agentExecutionHTTPGateway{baseURL: server.URL, apiKey: "key", client: server.Client()}
			scope := agentExecutionScope{OrganizationID: "org", SubjectID: "channel-conversation", Identity: &execution.Identity{Version: execution.BackgroundIdentityVersion,
				WorkspaceID: "workspace", ApplicationType: "digital_human", ApplicationID: "app", Entry: "channel", SubjectKind: execution.SubjectChannelConversation, ConversationID: "conversation", PublicationRef: "published"}}
			err := gateway.request(t.Context(), scope, http.MethodPost, "bindings", nil, nil)
			if (err == nil) != (version == 2) || (version != 2 && calls.Load() != 0) {
				t.Fatalf("capability=%d calls=%d error=%v", version, calls.Load(), err)
			}
		})
	}
}
