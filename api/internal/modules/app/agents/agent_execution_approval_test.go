package agents

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

func TestAgentExecutionApprovalFailsClosed(t *testing.T) {
	valid := agentExecutionApprovals{
		GatewayReady: true,
		Profiles: map[string]agentExecutionApproval{
			"standard": {Backend: "local-gvisor", OrganizationIDs: []string{"org-1"}, PolicyIDs: []string{"offline"}},
		},
		HomeSnapshots: map[string]agentExecutionHomeApproval{
			"snapshot-1": {Backend: "local-gvisor", ProfileID: "standard", OrganizationIDs: []string{"org-1"}},
		},
	}
	tests := []struct {
		name      string
		approvals agentExecutionApprovals
		org       string
		cfg       *dto.AgentExecutionRuntime
		wantError bool
	}{
		{name: "legacy config", approvals: valid},
		{name: "explicitly disabled", approvals: valid, cfg: &dto.AgentExecutionRuntime{}},
		{name: "approved", approvals: valid, org: "org-1", cfg: &dto.AgentExecutionRuntime{Enabled: true, ProfileID: "standard", PolicyID: "offline"}},
		{name: "gateway absent", org: "org-1", cfg: &dto.AgentExecutionRuntime{Enabled: true, ProfileID: "standard", PolicyID: "offline"}, wantError: true},
		{name: "unknown profile", approvals: valid, org: "org-1", cfg: &dto.AgentExecutionRuntime{Enabled: true, ProfileID: "unknown", PolicyID: "offline"}, wantError: true},
		{name: "wrong organization", approvals: valid, org: "org-2", cfg: &dto.AgentExecutionRuntime{Enabled: true, ProfileID: "standard", PolicyID: "offline"}, wantError: true},
		{name: "wrong policy", approvals: valid, org: "org-1", cfg: &dto.AgentExecutionRuntime{Enabled: true, ProfileID: "standard", PolicyID: "online"}, wantError: true},
		{name: "unknown backend", approvals: agentExecutionApprovals{GatewayReady: true, Profiles: map[string]agentExecutionApproval{"standard": {Backend: "unknown", OrganizationIDs: []string{"org-1"}, PolicyIDs: []string{"offline"}}}}, org: "org-1", cfg: &dto.AgentExecutionRuntime{Enabled: true, ProfileID: "standard", PolicyID: "offline"}, wantError: true},
		{name: "unknown dependency preparation mode", approvals: agentExecutionApprovals{GatewayReady: true, Profiles: map[string]agentExecutionApproval{"standard": {Backend: "local-gvisor", OrganizationIDs: []string{"org-1"}, PolicyIDs: []string{"offline"}, DependencyPreparation: true, DependencyPreparationMode: "controlled-http"}}}, org: "org-1", cfg: &dto.AgentExecutionRuntime{Enabled: true, ProfileID: "standard", PolicyID: "offline"}, wantError: true},
		{name: "dependency mode without capability", approvals: agentExecutionApprovals{GatewayReady: true, Profiles: map[string]agentExecutionApproval{"standard": {Backend: "local-gvisor", OrganizationIDs: []string{"org-1"}, PolicyIDs: []string{"offline"}, DependencyPreparationMode: agentExecutionDependencyPreparationModeOffline}}}, org: "org-1", cfg: &dto.AgentExecutionRuntime{Enabled: true, ProfileID: "standard", PolicyID: "offline"}, wantError: true},
		{name: "approved snapshot", approvals: valid, org: "org-1", cfg: &dto.AgentExecutionRuntime{Enabled: true, ProfileID: "standard", PolicyID: "offline", HomeSnapshotID: ptrAgentExecutionString("snapshot-1")}},
		{name: "unowned snapshot", approvals: valid, org: "org-2", cfg: &dto.AgentExecutionRuntime{Enabled: true, ProfileID: "standard", PolicyID: "offline", HomeSnapshotID: ptrAgentExecutionString("snapshot-1")}, wantError: true},
		{name: "unknown snapshot", approvals: valid, org: "org-1", cfg: &dto.AgentExecutionRuntime{Enabled: true, ProfileID: "standard", PolicyID: "offline", HomeSnapshotID: ptrAgentExecutionString("missing")}, wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.approvals.validate(tt.org, tt.cfg)
			if (err != nil) != tt.wantError {
				t.Fatalf("validate() error = %v, want error %v", err, tt.wantError)
			}
			if tt.wantError {
				if !apperror.IsCode(err, AppCodeAgentExecutionRuntimeUnavailable) {
					t.Fatalf("error = %v, want execution-runtime API error", err)
				}
			}
		})
	}
}

func TestAgentRuntimeGatewayURL(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  bool
	}{
		{"https://worker.example:8443", true},
		{"http://127.0.0.1:9876", true},
		{"http://[::1]:9876", true},
		{"http://worker.example:9876", false},
		{"https://user:secret@worker.example", false},
		{"https://worker.example/v1", false},
		{"", false},
	} {
		if got := validAgentRuntimeGatewayURL(tc.value); got != tc.want {
			t.Errorf("validAgentRuntimeGatewayURL(%q) = %v, want %v", tc.value, got, tc.want)
		}
	}
}

func TestAgentExecutionErrorCatalogIsLocalized(t *testing.T) {
	catalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, append(appcatalog.DefaultDefinitions(), CatalogDefinitions()...)...)
	if err != nil {
		t.Fatalf("construct product catalog: %v", err)
	}
	for _, locale := range []appcatalog.Locale{appcatalog.LocaleEnglishUS, appcatalog.LocaleChineseSimplified} {
		presentation, err := catalog.Present(AppCodeAgentExecutionRuntimeUnavailable, locale, nil)
		if err != nil || presentation.Message == "" || presentation.Code != AppCodeAgentExecutionRuntimeUnavailable {
			t.Fatalf("presentation for %s = %#v, %v", locale, presentation, err)
		}
	}
}

func TestAgentExecutionErrorHTTPDoesNotExposeOperation(t *testing.T) {
	catalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, append(appcatalog.DefaultDefinitions(), CatalogDefinitions()...)...)
	if err != nil {
		t.Fatalf("construct product catalog: %v", err)
	}
	handler := &AgentsHandler{}
	if err := handler.SetApplicationErrorCatalog(catalog); err != nil {
		t.Fatalf("attach catalog: %v", err)
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/agents", nil)
	c.Request.Header.Set("Accept-Language", "zh-CN")
	handler.failRuntime(c, apperror.New(AppCodeAgentExecutionRuntimeUnavailable, apperror.WithOperation("agent_execution.private_reason")))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), AppCodeAgentExecutionRuntimeUnavailable.String()) || strings.Contains(w.Body.String(), "private_reason") {
		t.Fatalf("unsafe HTTP error: status=%d body=%s", w.Code, w.Body.String())
	}
}

func ptrAgentExecutionString(value string) *string { return &value }
