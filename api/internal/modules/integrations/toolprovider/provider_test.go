package toolprovider

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

type providerTestAdapter struct{ request integrations.ActionRequest }

func (a *providerTestAdapter) DriverID() string { return integrations.DriverExa }

func (a *providerTestAdapter) Execute(_ context.Context, req integrations.ActionRequest) (*integrations.ActionResult, error) {
	a.request = req
	return &integrations.ActionResult{
		Output:       map[string]interface{}{"ok": true},
		ResultCount:  1,
		AttemptCount: 1,
	}, nil
}

type namedProviderTestAdapter struct{ driverID string }

func (adapter *namedProviderTestAdapter) DriverID() string { return adapter.driverID }

func (*namedProviderTestAdapter) Execute(context.Context, integrations.ActionRequest) (*integrations.ActionResult, error) {
	return &integrations.ActionResult{Output: map[string]interface{}{"ok": true}, AttemptCount: 1}, nil
}

type providerTestQuota struct{}

func (providerTestQuota) Acquire(context.Context, string) error { return nil }

type providerTestAudit struct{}

func (providerTestAudit) Create(context.Context, *integrations.ExecutionRecord) error { return nil }
func (providerTestAudit) Complete(context.Context, uuid.UUID, integrations.ExecutionCompletion) error {
	return nil
}

func TestToolI18nTextMapsCatalogLocales(t *testing.T) {
	text := toolI18nText("English fallback", integrations.LocalizedText{
		integrations.LocaleSimplifiedChinese: "中文",
	})
	if text.Get("en_US") != "English fallback" || text.Get("zh_Hans") != "中文" {
		t.Fatalf("toolI18nText() = %#v", text)
	}
}

func TestConnectorProviderInvokesIntegrationExecutor(t *testing.T) {
	adapter := &providerTestAdapter{}
	registry := integrations.NewRegistry()
	inputSchema := map[string]interface{}{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]interface{}{"query": map[string]interface{}{
			"type": "string",
			"title_i18n": integrations.LocalizedText{
				integrations.LocaleEnglishUS: "Query", integrations.LocaleSimplifiedChinese: "查询内容",
			},
		}},
		"required": []string{"query"},
	}
	outputSchema := map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{"ok": map[string]interface{}{"type": "boolean"}},
		"required":   []string{"ok"},
	}
	action := integrations.ActionDefinition{
		ID: integrations.ActionWebSearch, ToolName: "search_web", Name: "Search Web",
		NameI18n:    integrations.LocalizedText{integrations.LocaleEnglishUS: "Search Web", integrations.LocaleSimplifiedChinese: "搜索网页"},
		Description: "Search public web content.", InputSchema: inputSchema, OutputSchema: outputSchema,
		DescriptionI18n: integrations.LocalizedText{integrations.LocaleEnglishUS: "Search public web content.", integrations.LocaleSimplifiedChinese: "搜索公开网页内容。"},
		Effect:          toolgovernance.EffectRead, RiskLevel: toolgovernance.RiskLevelLow, DataEgress: true, ExternalDestination: "example.com",
	}
	if err := registry.Register(providerTestRegistration(integrations.IntegrationWebSearch, adapter, []integrations.ActionDefinition{action})); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	executor := integrations.NewExecutor(registry, providerTestAudit{}, providerTestQuota{}, integrations.DefaultSafetyChecker{}, []byte("audit-key"), time.Second)
	provider, err := NewProvider(registry, executor)
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}
	providerTool, err := provider.GetTool("search_web")
	if err != nil {
		t.Fatalf("GetTool() error = %v", err)
	}
	governance := providerTool.GetEntity().Governance
	if governance == nil || governance.Effect != string(toolgovernance.EffectRead) ||
		governance.RiskLevel != string(toolgovernance.RiskLevelLow) ||
		!governance.DataEgress || governance.ExternalDestination != "example.com" ||
		governance.SensitiveDataAllowed {
		t.Fatalf("connector governance metadata = %#v", governance)
	}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(provider); err != nil {
		t.Fatalf("RegisterProvider() error = %v", err)
	}
	engine := tools.NewToolEngine(manager)
	organizationID := uuid.NewString()
	userID := uuid.NewString()
	workspaceID := uuid.NewString()
	connectionID := uuid.NewString()
	result, err := engine.Invoke(context.Background(), tools.InvokeRequest{
		ProviderType: tools.ToolProviderTypeConnector,
		ProviderID:   integrations.IntegrationWebSearch,
		ToolName:     "search_web",
		TenantID:     organizationID,
		UserID:       userID,
		Parameters:   map[string]interface{}{"query": "current ZGI release"},
		InvokeFrom:   tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"workspace_id":  workspaceID,
			"connection_id": connectionID,
		},
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if !result.Success || len(result.Messages) != 1 || result.Messages[0].Data["ok"] != true {
		t.Fatalf("Invoke() result = %#v", result)
	}
	if adapter.request.OrganizationID != organizationID || adapter.request.UserID != userID || adapter.request.WorkspaceID != workspaceID || adapter.request.ConnectionID != connectionID {
		t.Fatalf("adapter request context = %#v", adapter.request)
	}
	if adapter.request.ActionID != integrations.ActionWebSearch || adapter.request.InvokeFrom != tools.ToolInvokeFromAIChat {
		t.Fatalf("adapter action context = %#v", adapter.request)
	}
}

func TestConnectorProviderFactoryBuildsEveryRegisteredIntegration(t *testing.T) {
	registry := integrations.NewRegistry()
	for _, registration := range []integrations.Registration{
		providerTestRegistration("github", &namedProviderTestAdapter{driverID: "github-rest"}, []integrations.ActionDefinition{providerFactoryTestAction("github.issue.list", "list_github_issues")}),
		providerTestRegistration("feishu", &namedProviderTestAdapter{driverID: "feishu-openapi"}, []integrations.ActionDefinition{providerFactoryTestAction("feishu.document.get", "get_feishu_document")}),
	} {
		if err := registry.Register(registration); err != nil {
			t.Fatalf("Register(%s) error = %v", registration.IntegrationID, err)
		}
	}
	executor := integrations.NewExecutor(registry, providerTestAudit{}, providerTestQuota{}, integrations.DefaultSafetyChecker{}, []byte("audit-key"), time.Second)
	providers, err := NewProviders(registry, executor)
	if err != nil {
		t.Fatalf("NewProviders() error = %v", err)
	}
	if len(providers) != 2 || providers[0].IntegrationID() != "feishu" || providers[1].IntegrationID() != "github" {
		t.Fatalf("NewProviders() = %#v", providers)
	}
	if providers[0].GetEntity().Identity.Name != "feishu" || providers[0].GetEntity().Identity.Label.Get("en_US") != "Feishu" {
		t.Fatalf("Feishu provider entity = %#v", providers[0].GetEntity())
	}
	if _, err := providers[1].GetTool("list_github_issues"); err != nil {
		t.Fatalf("GitHub tool was not generated: %v", err)
	}
	if _, err := NewProvider(registry, executor); err == nil {
		t.Fatal("NewProvider() should reject an ambiguous multi-provider registry")
	}
	provider, err := NewProviderForIntegration(registry, executor, " GITHUB ")
	if err != nil || provider.IntegrationID() != "github" {
		t.Fatalf("NewProviderForIntegration() = %#v, %v", provider, err)
	}
}

func providerFactoryTestAction(id, toolName string) integrations.ActionDefinition {
	return integrations.ActionDefinition{
		ID: id, ToolName: toolName, Name: "Test action", Description: "Execute a test action.",
		NameI18n:        integrations.LocalizedText{integrations.LocaleEnglishUS: "Test action", integrations.LocaleSimplifiedChinese: "测试操作"},
		DescriptionI18n: integrations.LocalizedText{integrations.LocaleEnglishUS: "Execute a test action.", integrations.LocaleSimplifiedChinese: "执行测试操作。"},
		InputSchema:     map[string]interface{}{"type": "object", "additionalProperties": false},
		OutputSchema:    map[string]interface{}{"type": "object", "properties": map[string]interface{}{"ok": map[string]interface{}{"type": "boolean"}}, "required": []string{"ok"}},
		Effect:          toolgovernance.EffectRead, RiskLevel: toolgovernance.RiskLevelLow,
	}
}

func providerTestRegistration(integrationID string, adapter integrations.Adapter, actions []integrations.ActionDefinition) integrations.Registration {
	name := "Test external application"
	zhName := "测试外部应用"
	switch integrationID {
	case "github":
		name, zhName = "GitHub", "GitHub"
	case "feishu":
		name, zhName = "Feishu", "飞书"
	case integrations.IntegrationWebSearch:
		name, zhName = "Web Search", "网页搜索"
	}
	return integrations.Registration{
		IntegrationID: integrationID,
		Definition: integrations.ProviderDefinition{
			ID: integrationID, DriverID: adapter.DriverID(), Name: name,
			NameI18n:    integrations.LocalizedText{integrations.LocaleEnglishUS: name, integrations.LocaleSimplifiedChinese: zhName},
			Description: "External application used by connector tests.",
			DescriptionI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS: "External application used by connector tests.", integrations.LocaleSimplifiedChinese: "连接器测试使用的外部应用。",
			},
			AuthMethods: []integrations.AuthMethodDefinition{{
				ID: string(integrations.AuthMethodTypeNone), Type: integrations.AuthMethodTypeNone,
				CredentialSource: integrations.ConnectionCredentialSourceOrganization,
				Label:            "No authentication", LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "No authentication", integrations.LocaleSimplifiedChinese: "无需认证",
				},
				Available: true,
			}},
			Actions: actions,
		},
		Adapter: adapter,
		Actions: actions,
	}
}
