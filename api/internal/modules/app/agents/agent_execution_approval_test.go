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
		Enabled:      true,
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
		{name: "approved controlled HTTPS", approvals: agentExecutionApprovals{Enabled: true, GatewayReady: true, Profiles: map[string]agentExecutionApproval{"standard": {Backend: "local-gvisor", OrganizationIDs: []string{"org-1"}, PolicyIDs: []string{"controlled-http"}, DependencyPreparation: true, DependencyPreparationMode: agentExecutionDependencyPreparationModeControlledHTTP}}}, org: "org-1", cfg: &dto.AgentExecutionRuntime{Enabled: true}},
		{name: "approved standard package installation", approvals: agentExecutionApprovals{Enabled: true, GatewayReady: true, Profiles: map[string]agentExecutionApproval{"standard": {Backend: "local-gvisor", OrganizationIDs: []string{"org-1"}, PolicyIDs: []string{"offline", agentExecutionPackageNetworkMode}, DependencyInstallation: true, PackageNetworkMode: agentExecutionPackageNetworkMode}}}, org: "org-1", cfg: &dto.AgentExecutionRuntime{Enabled: true}},
		{name: "approved E2B package installation", approvals: agentExecutionApprovals{Enabled: true, GatewayReady: true, Profiles: map[string]agentExecutionApproval{"standard": {Backend: "e2b", OrganizationIDs: []string{"org-1"}, PolicyIDs: []string{"offline", agentExecutionPackageNetworkMode}, DependencyInstallation: true, PackageNetworkMode: agentExecutionPackageNetworkMode}}}, org: "org-1", cfg: &dto.AgentExecutionRuntime{Enabled: true}},
		{name: "package mode without installation capability", approvals: agentExecutionApprovals{Enabled: true, GatewayReady: true, Profiles: map[string]agentExecutionApproval{"standard": {Backend: "local-gvisor", OrganizationIDs: []string{"org-1"}, PolicyIDs: []string{agentExecutionPackageNetworkMode}, PackageNetworkMode: agentExecutionPackageNetworkMode}}}, org: "org-1", cfg: &dto.AgentExecutionRuntime{Enabled: true}, wantError: true},
		{name: "gateway absent", org: "org-1", cfg: &dto.AgentExecutionRuntime{Enabled: true, ProfileID: "standard", PolicyID: "offline"}, wantError: true},
		{name: "client profile is ignored", approvals: valid, org: "org-1", cfg: &dto.AgentExecutionRuntime{Enabled: true, ProfileID: "unknown", PolicyID: "online"}},
		{name: "wrong organization", approvals: valid, org: "org-2", cfg: &dto.AgentExecutionRuntime{Enabled: true, ProfileID: "standard", PolicyID: "offline"}, wantError: true},
		{name: "unknown backend", approvals: agentExecutionApprovals{Enabled: true, GatewayReady: true, Profiles: map[string]agentExecutionApproval{"standard": {Backend: "unknown", OrganizationIDs: []string{"org-1"}, PolicyIDs: []string{"offline"}}}}, org: "org-1", cfg: &dto.AgentExecutionRuntime{Enabled: true}, wantError: true},
		{name: "unknown dependency preparation mode", approvals: agentExecutionApprovals{Enabled: true, GatewayReady: true, Profiles: map[string]agentExecutionApproval{"standard": {Backend: "local-gvisor", OrganizationIDs: []string{"org-1"}, PolicyIDs: []string{"offline"}, DependencyPreparation: true, DependencyPreparationMode: "future-mode"}}}, org: "org-1", cfg: &dto.AgentExecutionRuntime{Enabled: true}, wantError: true},
		{name: "dependency mode without capability", approvals: agentExecutionApprovals{Enabled: true, GatewayReady: true, Profiles: map[string]agentExecutionApproval{"standard": {Backend: "local-gvisor", OrganizationIDs: []string{"org-1"}, PolicyIDs: []string{"offline"}, DependencyPreparationMode: agentExecutionDependencyPreparationModeOffline}}}, org: "org-1", cfg: &dto.AgentExecutionRuntime{Enabled: true}, wantError: true},
		{name: "approved snapshot", approvals: func() agentExecutionApprovals { a := valid; a.DefaultHomeSnapshotID = "snapshot-1"; return a }(), org: "org-1", cfg: &dto.AgentExecutionRuntime{Enabled: true}},
		{name: "unowned snapshot", approvals: func() agentExecutionApprovals { a := valid; a.DefaultHomeSnapshotID = "snapshot-1"; return a }(), org: "org-2", cfg: &dto.AgentExecutionRuntime{Enabled: true}, wantError: true},
		{name: "invalid operator snapshot", approvals: func() agentExecutionApprovals { a := valid; a.DefaultHomeSnapshotID = "missing"; return a }(), org: "org-1", cfg: &dto.AgentExecutionRuntime{Enabled: true}, wantError: true},
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

func TestAgentExecutionPackageInstallationUsesOperatorNetworkMode(t *testing.T) {
	approvals := agentExecutionApprovals{Enabled: true, GatewayReady: true, Profiles: map[string]agentExecutionApproval{
		"standard": {Backend: "local-gvisor", OrganizationIDs: []string{"org-1"}, PolicyIDs: []string{"offline", agentExecutionPackageNetworkMode}, DependencyInstallation: true, PackageNetworkMode: agentExecutionPackageNetworkMode},
	}}
	resolved, err := approvals.resolve("org-1", &dto.AgentExecutionRuntime{Enabled: true, PolicyID: "user-supplied"})
	if err != nil {
		t.Fatalf("resolve package profile: %v", err)
	}
	if resolved.PolicyID != agentExecutionPackageNetworkMode {
		t.Fatalf("resolved package network policy = %q, want %q", resolved.PolicyID, agentExecutionPackageNetworkMode)
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
		{"http://zgi-agentworker-m6b:2671", true},
		{"http://agent-runtime.internal:2671", true},
		{"http://10.0.0.4:2671", true},
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

func TestAgentExecutionApprovalsReadDeploymentGateAndDefaults(t *testing.T) {
	t.Setenv("ZGI_AGENT_RUNTIME_ENABLED", "true")
	t.Setenv("ZGI_AGENT_RUNTIME_DEFAULT_PROFILE_ID", "standard")
	t.Setenv("ZGI_AGENT_RUNTIME_DEFAULT_POLICY_ID", "offline")
	t.Setenv("ZGI_AGENT_RUNTIME_DEFAULT_HOME_SNAPSHOT_ID", "snapshot-1")
	t.Setenv("ZGI_AGENT_RUNTIME_URL", "https://worker.example")
	t.Setenv("ZGI_AGENT_RUNTIME_API_KEY", "worker-secret")
	t.Setenv("ZGI_AGENT_RUNTIME_APPROVED_PROFILES_JSON", `{"standard":{"backend":"e2b","organization_ids":["org-1"],"policy_ids":["offline"]}}`)
	t.Setenv("ZGI_AGENT_RUNTIME_APPROVED_HOME_SNAPSHOTS_JSON", `{"snapshot-1":{"backend":"e2b","profile_id":"standard","organization_ids":["org-1"]}}`)
	approvals := agentExecutionApprovalsFromEnv()
	if !approvals.Enabled || !approvals.GatewayReady || approvals.DefaultProfileID != "standard" || approvals.DefaultPolicyID != "offline" {
		t.Fatalf("deployment approvals = %+v", approvals)
	}
	resolved, err := approvals.resolve("org-1", &dto.AgentExecutionRuntime{Enabled: true, ProfileID: "user-supplied", PolicyID: "user-supplied"})
	if err != nil || resolved.ProfileID != "standard" || resolved.PolicyID != "offline" || resolved.HomeSnapshotID == nil || *resolved.HomeSnapshotID != "snapshot-1" {
		t.Fatalf("resolved runtime = %+v, error = %v", resolved, err)
	}
	t.Setenv("ZGI_AGENT_RUNTIME_ENABLED", "false")
	if err := agentExecutionApprovalsFromEnv().validate("org-1", &dto.AgentExecutionRuntime{Enabled: true}); !apperror.IsCode(err, AppCodeAgentExecutionRuntimeUnavailable) {
		t.Fatalf("disabled deployment error = %v", err)
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
