package agents

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

func TestAgentExecutionGatewayReadsOnlySafeJobFailureCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/agent-runtime/jobs/job-1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":"job-1","status":"failed","exit_code":-1,"failure_code":"language_package_recovery_failed","failure_reason":"https://user:password@private"}}`))
	}))
	defer server.Close()
	gateway := &agentExecutionHTTPGateway{baseURL: server.URL, apiKey: "test-key", client: server.Client()}
	job, err := gateway.GetJob(context.Background(), agentExecutionScope{OrganizationID: "org", SubjectID: "subject"}, "job-1")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(job)
	if err != nil {
		t.Fatal(err)
	}
	if job.FailureCode != "language_package_recovery_failed" || strings.Contains(string(encoded), "password") || strings.Contains(string(encoded), "failure_reason") {
		t.Fatalf("unsafe job transport: %s", encoded)
	}
}

func TestAgentShellFailedJobsHaveSafeCatalogedErrors(t *testing.T) {
	catalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, append(appcatalog.DefaultDefinitions(), CatalogDefinitions()...)...)
	if err != nil {
		t.Fatal(err)
	}
	projector, err := apptransport.NewProjector(catalog)
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"language_package_recovery_failed", "package_recovery_failed", "", "https://user:password@private"} {
		fake := &fakeAgentExecutionGateway{binding: agentExecutionBinding{ID: "binding-1", Status: "active"}, job: agentExecutionJob{ID: "job-1", Status: "failed", FailureCode: code}}
		tool := newAgentExecutionShellTool(fake, agentExecutionScope{}, "agent", "conversation", "message", "standard", "e2b", nil, nil)
		result := tool.Handler(t.Context(), adapter.ToolCall{Function: adapter.FunctionCall{Arguments: `{"command":"python -c 'import ujson'"}`}})
		wantCode, _ := agentExecutionJobFailure(code)
		if result.Status != "failed" || result.ModelResult["error_code"] != wantCode || result.Result["error_code"] != wantCode {
			t.Fatalf("tool result = %+v", result)
		}
		if _, ok := apperror.As(result.Error); !ok {
			t.Fatal("missing application error")
		}
		result = projectAgentExecutionToolFailure(result, projector)
		message, ok := result.ModelResult["message"].(string)
		if !ok || message == "" || strings.Contains(message, "password") || strings.Contains(message, "private") {
			t.Fatalf("unsafe model message = %#v", result.ModelResult)
		}
		if code == "language_package_recovery_failed" && !strings.Contains(message, "no network or latest-version fallback") {
			t.Fatal("model did not receive the recovery boundary")
		}
		if appCode, ok := result.ModelResult["app_error_code"].(string); !ok || appCode == "" {
			t.Fatalf("application error identity must be JSON text: %#v", result.ModelResult)
		}
		for _, locale := range []appcatalog.Locale{appcatalog.LocaleEnglishUS, appcatalog.LocaleChineseSimplified} {
			p := projector.Project(result.Error, locale)
			if p.Resolution != apptransport.ResolutionMatched || p.Presentation.Message == "" || strings.Contains(p.Presentation.Message, "password") || strings.Contains(p.Presentation.Message, "private") {
				t.Fatalf("unsafe projection = %+v", p)
			}
		}
	}
}
