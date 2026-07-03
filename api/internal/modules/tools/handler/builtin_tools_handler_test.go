package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	pluginmodel "github.com/zgiai/zgi/api/internal/modules/pluginrunner/model"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

type stubAccountInstallationService struct {
	declarations []pluginmodel.PluginDeclaration
}

func (s *stubAccountInstallationService) Install(context.Context, string, string, string, string) (*pluginmodel.AccountPluginInstallation, error) {
	return nil, nil
}

func (s *stubAccountInstallationService) Uninstall(context.Context, string, string) error {
	return nil
}

func (s *stubAccountInstallationService) GetInstallation(context.Context, string, string) (*pluginmodel.AccountPluginInstallation, error) {
	return nil, nil
}

func (s *stubAccountInstallationService) ListByTenant(context.Context, string) ([]pluginmodel.AccountPluginInstallation, error) {
	return nil, nil
}

func (s *stubAccountInstallationService) CountByMarketplaceVersionID(context.Context, string) (int64, error) {
	return 0, nil
}

func (s *stubAccountInstallationService) ListDeclarationsByTenant(_ context.Context, tenantID string) ([]pluginmodel.PluginDeclaration, error) {
	if tenantID != "org-1" {
		return nil, errors.New("unexpected tenant")
	}
	return s.declarations, nil
}

func (s *stubAccountInstallationService) GetDeclarationByProviderName(_ context.Context, tenantID, providerName string) (*pluginmodel.PluginDeclaration, error) {
	if tenantID != "org-1" {
		return nil, errors.New("unexpected tenant")
	}
	for _, decl := range s.declarations {
		if decl.Provider.Name == providerName {
			return &decl, nil
		}
	}
	return nil, nil
}

func (s *stubAccountInstallationService) GetInstalledPluginInfoByProviderName(context.Context, string, string) (*pluginmodel.InstalledPluginInfo, error) {
	return nil, nil
}

func (s *stubAccountInstallationService) InstallFromDirectory(context.Context, string, string, string, string, string) (*pluginmodel.AccountPluginInstallation, error) {
	return nil, nil
}

type failIfCalledMemberSubscriptionService struct {
	t *testing.T
}

type testToolProvider struct {
	entity tools.ToolProviderEntity
}

func (p testToolProvider) GetEntity() tools.ToolProviderEntity {
	return p.entity
}

func (p testToolProvider) GetProviderType() tools.ToolProviderType {
	if p.entity.ProviderType == "" {
		return tools.ToolProviderTypeBuiltin
	}
	return p.entity.ProviderType
}

func (p testToolProvider) GetTool(string) (tools.Tool, error) {
	return nil, tools.ErrToolNotFound
}

func (p testToolProvider) GetTools() []tools.Tool {
	return nil
}

func (p testToolProvider) ValidateCredentials(context.Context, map[string]interface{}) error {
	return nil
}

func (s failIfCalledMemberSubscriptionService) Subscribe(context.Context, string, string, string, string, string, string) (*pluginmodel.OrgPluginSubscription, error) {
	s.t.Fatal("member subscription service should not be used by builtin tools handler")
	return nil, nil
}

func (s failIfCalledMemberSubscriptionService) Unsubscribe(context.Context, string, string, string) error {
	s.t.Fatal("member subscription service should not be used by builtin tools handler")
	return nil
}

func (s failIfCalledMemberSubscriptionService) ListSubscribedPlugins(context.Context, string, string) ([]pluginmodel.OrgPluginSubscription, error) {
	s.t.Fatal("member subscription service should not be used by builtin tools handler")
	return nil, nil
}

func (s failIfCalledMemberSubscriptionService) IsSubscribed(context.Context, string, string, string) (bool, error) {
	s.t.Fatal("member subscription service should not be used by builtin tools handler")
	return false, nil
}

func (s failIfCalledMemberSubscriptionService) ListSubscribedDeclarations(context.Context, string, string) ([]pluginmodel.PluginDeclaration, error) {
	s.t.Fatal("member subscription service should not be used by builtin tools handler")
	return nil, nil
}

func (s failIfCalledMemberSubscriptionService) CanDeleteInstallation(context.Context, string) (bool, error) {
	s.t.Fatal("member subscription service should not be used by builtin tools handler")
	return false, nil
}

func (s failIfCalledMemberSubscriptionService) GetSubscriberCount(context.Context, string) (int64, error) {
	s.t.Fatal("member subscription service should not be used by builtin tools handler")
	return 0, nil
}

func TestListBuiltinProvidersIncludesOrganizationPluginsForNormalMember(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewBuiltinToolsHandler(
		tools.NewToolManager(nil),
		&stubAccountInstallationService{declarations: []pluginmodel.PluginDeclaration{emailDeclaration()}},
		failIfCalledMemberSubscriptionService{t: t},
	)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/console/api/tools/builtin", nil)
	ctx.Set("organization_id", "org-1")
	ctx.Set("account_id", "normal-member")

	handler.ListBuiltinProviders(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	var resp struct {
		Code string                    `json:"code"`
		Data []BuiltinProviderResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Code != "0" {
		t.Fatalf("code = %s", resp.Code)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("providers len = %d, data = %+v", len(resp.Data), resp.Data)
	}
	if got := resp.Data[0]; got.Name != "email" || got.Type != "plugin_runner" {
		t.Fatalf("provider = %+v", got)
	}
}

func TestListBuiltinProvidersFiltersBuiltinToolsForWorkflowCaller(t *testing.T) {
	gin.SetMode(gin.TestMode)

	manager := tools.NewToolManager(nil)
	_ = manager.RegisterProvider(testBuiltinProviderEntity("calculator", "calculate", "percentage"))
	_ = manager.RegisterProvider(testBuiltinProviderEntity("chart_generator", "generate_chart"))
	_ = manager.RegisterProvider(testBuiltinProviderEntity("workflow", "run_agent_workflow"))
	_ = manager.RegisterProvider(testBuiltinProviderEntity("knowledge", "retrieve_knowledge"))
	_ = manager.RegisterProvider(testBuiltinProviderEntity("database", "query_table_records"))

	handler := NewBuiltinToolsHandler(
		manager,
		&stubAccountInstallationService{declarations: []pluginmodel.PluginDeclaration{emailDeclaration()}},
		failIfCalledMemberSubscriptionService{t: t},
	)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/console/api/tools/builtin?caller=workflow", nil)
	ctx.Set("organization_id", "org-1")
	ctx.Set("account_id", "normal-member")

	handler.ListBuiltinProviders(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	var resp struct {
		Code string                    `json:"code"`
		Data []BuiltinProviderResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Code != "0" {
		t.Fatalf("code = %s", resp.Code)
	}

	if provider := findBuiltinProviderResponse(resp.Data, "calculator"); provider == nil {
		t.Fatalf("calculator provider missing from workflow-visible response: %+v", resp.Data)
	} else if got := responseToolNames(provider.Tools); !sameStrings(got, []string{"calculate", "percentage"}) {
		t.Fatalf("calculator tools = %v, want calculate and percentage", got)
	}
	if provider := findBuiltinProviderResponse(resp.Data, "email"); provider == nil || provider.Type != "plugin_runner" {
		t.Fatalf("plugin runner provider missing or changed: %+v", resp.Data)
	}
	for _, name := range []string{"chart_generator", "workflow", "knowledge", "database"} {
		if provider := findBuiltinProviderResponse(resp.Data, name); provider != nil {
			t.Fatalf("provider %s should be hidden for workflow caller: %+v", name, provider)
		}
	}
}

func TestGetBuiltinProviderReturnsNotFoundWhenFilteredForWorkflowCaller(t *testing.T) {
	gin.SetMode(gin.TestMode)

	manager := tools.NewToolManager(nil)
	_ = manager.RegisterProvider(testBuiltinProviderEntity("workflow", "run_agent_workflow"))

	handler := NewBuiltinToolsHandler(
		manager,
		&stubAccountInstallationService{},
		failIfCalledMemberSubscriptionService{t: t},
	)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/console/api/tools/builtin/workflow?caller=workflow", nil)
	ctx.Params = gin.Params{{Key: "provider", Value: "workflow"}}
	ctx.Set("organization_id", "org-1")
	ctx.Set("account_id", "normal-member")

	handler.GetBuiltinProvider(ctx)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestGetBuiltinProviderReturnsOrganizationPluginForNormalMember(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewBuiltinToolsHandler(
		tools.NewToolManager(nil),
		&stubAccountInstallationService{declarations: []pluginmodel.PluginDeclaration{emailDeclaration()}},
		failIfCalledMemberSubscriptionService{t: t},
	)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/console/api/tools/builtin/email", nil)
	ctx.Params = gin.Params{{Key: "provider", Value: "email"}}
	ctx.Set("organization_id", "org-1")
	ctx.Set("account_id", "normal-member")

	handler.GetBuiltinProvider(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	var resp struct {
		Code string                  `json:"code"`
		Data BuiltinProviderResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Code != "0" {
		t.Fatalf("code = %s", resp.Code)
	}
	if resp.Data.Name != "email" || resp.Data.Type != "plugin_runner" {
		t.Fatalf("provider = %+v", resp.Data)
	}
}

func TestGetBuiltinProviderReturnsNotFoundForUninstalledPlugin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewBuiltinToolsHandler(
		tools.NewToolManager(nil),
		&stubAccountInstallationService{declarations: []pluginmodel.PluginDeclaration{emailDeclaration()}},
		failIfCalledMemberSubscriptionService{t: t},
	)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/console/api/tools/builtin/not-installed", nil)
	ctx.Params = gin.Params{{Key: "provider", Value: "not-installed"}}
	ctx.Set("organization_id", "org-1")
	ctx.Set("account_id", "normal-member")

	handler.GetBuiltinProvider(ctx)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func testBuiltinProviderEntity(providerName string, toolNames ...string) testToolProvider {
	toolEntities := make([]tools.ToolEntity, 0, len(toolNames))
	for _, toolName := range toolNames {
		toolEntities = append(toolEntities, tools.ToolEntity{
			Identity: tools.ToolIdentity{
				Name:     toolName,
				Provider: providerName,
				Label:    tools.I18nText{"en_US": toolName},
			},
			Description: tools.ToolDescription{Human: tools.I18nText{"en_US": toolName}},
		})
	}
	return testToolProvider{
		entity: tools.ToolProviderEntity{
			ProviderType: tools.ToolProviderTypeBuiltin,
			Identity: tools.ToolProviderIdentity{
				Name:        providerName,
				Label:       tools.I18nText{"en_US": providerName},
				Description: tools.I18nText{"en_US": providerName},
			},
			Tools: toolEntities,
		},
	}
}

func findBuiltinProviderResponse(providers []BuiltinProviderResponse, name string) *BuiltinProviderResponse {
	for idx := range providers {
		if providers[idx].Name == name {
			return &providers[idx]
		}
	}
	return nil
}

func responseToolNames(tools []BuiltinToolResponse) []string {
	out := make([]string, 0, len(tools))
	for _, tool := range tools {
		out = append(out, tool.Name)
	}
	return out
}

func sameStrings(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	seen := make(map[string]int, len(left))
	for _, value := range left {
		seen[value]++
	}
	for _, value := range right {
		if seen[value] == 0 {
			return false
		}
		seen[value]--
	}
	return true
}

func emailDeclaration() pluginmodel.PluginDeclaration {
	return pluginmodel.PluginDeclaration{
		Provider: pluginmodel.ProviderDeclaration{
			Name:        "email",
			Author:      "zgi",
			Label:       map[string]string{"zh_Hans": "电子邮件", "en_US": "Email"},
			Description: map[string]string{"zh_Hans": "发送邮件", "en_US": "Send email"},
		},
		Tools: []pluginmodel.ToolDeclaration{
			{
				Name:        "send_email",
				Label:       map[string]string{"zh_Hans": "发送邮件", "en_US": "Send email"},
				Description: pluginmodel.ToolDescription{Human: map[string]string{"zh_Hans": "发送邮件", "en_US": "Send email"}},
				Parameters: []pluginmodel.ParameterDeclare{
					{
						Name:     "to",
						Type:     "string",
						Required: true,
						Label:    map[string]string{"zh_Hans": "收件人", "en_US": "Recipient"},
						Form:     "llm",
					},
				},
			},
		},
	}
}
