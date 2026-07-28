package integrations

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestRegistryCatalogIsDefinitionDrivenVersionedAndMutationSafe(t *testing.T) {
	registry := NewRegistry()
	action := testAction("github.issue.list", "list_github_issues")
	action.Name = "List GitHub Issues"
	action.NameI18n = LocalizedText{LocaleSimplifiedChinese: "列出 GitHub 议题"}
	action.Description = "List issues in a repository."
	action.DescriptionI18n = LocalizedText{LocaleSimplifiedChinese: "列出仓库中的议题。"}
	action.RequiredScopes = []string{" Issues:Read ", "issues:read"}
	action.RequiredAnyScopes = []string{" Pulls:Read ", "contents:read", "pulls:read"}
	action.PreferredScopes = []string{" Contents:Read "}
	action.SupportedAuthMethodIDs = []string{" Personal_Access_Token ", "personal_access_token"}
	action.ScopeLabelsI18n = LocalizedLabelMap{
		" Pulls:Read ":   {LocaleEnglishUS: "Read pull requests", LocaleSimplifiedChinese: "读取合并请求"},
		" contents:read": {LocaleEnglishUS: "Read contents", LocaleSimplifiedChinese: "读取内容"},
		" Issues:Read ":  {LocaleEnglishUS: "Read issues", LocaleSimplifiedChinese: "议题读取权限"},
	}
	action.DefaultPolicy = &DefaultActionPolicy{
		Enabled: true, ApprovalPolicy: toolgovernance.ApprovalPolicyNeverAsk, DataEgressAllowed: true,
	}
	action.SupportedCallers = []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat, tools.ToolInvokeFromAgent}
	registration := Registration{
		Definition: ProviderDefinition{
			ID: " GitHub ", DriverID: " GitHub-REST ", Name: "GitHub",
			NameI18n:        LocalizedText{LocaleSimplifiedChinese: "GitHub"},
			Description:     "Work with GitHub repositories.",
			DescriptionI18n: LocalizedText{LocaleSimplifiedChinese: "使用 GitHub 仓库。"},
			Author:          "ZGI", Icon: "github", Tags: []string{"Code", "code"},
			TagLabelsI18n: LocalizedLabelMap{
				" Code ": {LocaleEnglishUS: "Code", LocaleSimplifiedChinese: "代码"},
			},
			Categories: []string{"Development"},
			CategoryLabelsI18n: LocalizedLabelMap{
				" Development ": {LocaleEnglishUS: "Development", LocaleSimplifiedChinese: "软件开发"},
			},
			DocumentationURL: "https://docs.github.com/rest",
			DocumentationURLI18n: LocalizedText{
				LocaleSimplifiedChinese: "https://docs.github.com/zh/rest",
			},
			AuthMethods: []AuthMethodDefinition{{
				ID: "personal_access_token", Type: AuthMethodTypeAPIKey,
				CredentialSource: ConnectionCredentialSourceOrganization, Label: "Personal access token",
				LabelI18n: LocalizedText{LocaleSimplifiedChinese: "个人访问令牌"}, Available: true,
				SetupGuide: &AuthSetupGuideDefinition{
					ConsoleURL:       "https://github.com/settings/tokens",
					DocumentationURL: "https://docs.github.com/authentication",
					Steps: []AuthSetupStepDefinition{{
						ID: "create_token", Title: "Create a token",
						TitleI18n:       LocalizedText{LocaleSimplifiedChinese: "创建令牌"},
						Description:     "Create a token in GitHub.",
						DescriptionI18n: LocalizedText{LocaleSimplifiedChinese: "在 GitHub 中创建令牌。"},
						Action:          AuthSetupStepActionOpenConsole,
					}},
					Notices: []AuthSetupNoticeDefinition{{
						ID: "least_privilege", Level: AuthSetupNoticeLevelInfo, Text: "Use least privilege.",
						TextI18n: LocalizedText{LocaleSimplifiedChinese: "使用最小权限。"},
					}},
				},
				Fields: []CredentialFieldDefinition{{
					Key: "token", Label: "Token", LabelI18n: LocalizedText{LocaleSimplifiedChinese: "令牌"},
					Input: CredentialFieldInputPassword, Required: true, Secret: true,
				}},
			}},
			HealthProbe: HealthProbeDefinition{Supported: true, Description: "Read the authenticated user.", DescriptionI18n: LocalizedText{LocaleSimplifiedChinese: "读取已认证用户。"}},
			Scopes: []ProviderScopeDefinition{{
				ID: "issues:read", Label: "Read issues",
				LabelI18n: LocalizedText{LocaleEnglishUS: "Read issues", LocaleSimplifiedChinese: "璁璇诲彇鏉冮檺"},
				Category:  ProviderScopeCategoryProvider, Access: ProviderScopeAccessRead,
			}, {
				ID: "pulls:read", Label: "Read pull requests",
				LabelI18n: LocalizedText{LocaleEnglishUS: "Read pull requests", LocaleSimplifiedChinese: "读取合并请求"},
				Category:  ProviderScopeCategoryProvider, Access: ProviderScopeAccessRead,
			}, {
				ID: "contents:read", Label: "Read contents",
				LabelI18n: LocalizedText{LocaleEnglishUS: "Read contents", LocaleSimplifiedChinese: "读取内容"},
				Category:  ProviderScopeCategoryProvider, Access: ProviderScopeAccessRead,
			}},
			Actions: []ActionDefinition{action},
		},
		Adapter: &testAdapter{driverID: "github-rest"},
	}
	if err := registry.Register(registration); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	items := registry.Catalog()
	if len(items) != 1 {
		t.Fatalf("Catalog() = %#v", items)
	}
	item := items[0]
	if item.ID != "github" || item.DriverID != "github-rest" || item.Name != "GitHub" || item.Icon != "github" {
		t.Fatalf("Catalog() provider = %#v", item)
	}
	if item.NameI18n[LocaleEnglishUS] != "GitHub" || item.DescriptionI18n[LocaleSimplifiedChinese] != "使用 GitHub 仓库。" {
		t.Fatalf("Catalog() provider localized metadata = %#v / %#v", item.NameI18n, item.DescriptionI18n)
	}
	if item.DocumentationURLI18n[LocaleSimplifiedChinese] != "https://docs.github.com/zh/rest" {
		t.Fatalf("Catalog() localized documentation urls = %#v", item.DocumentationURLI18n)
	}
	if item.TagLabelsI18n["code"][LocaleSimplifiedChinese] != "代码" || item.CategoryLabelsI18n["development"][LocaleSimplifiedChinese] != "软件开发" {
		t.Fatalf("Catalog() localized provider labels = %#v / %#v", item.TagLabelsI18n, item.CategoryLabelsI18n)
	}
	if len(item.Auth) != 1 || item.Auth[0].SetupGuide == nil ||
		item.Auth[0].SetupGuide.Steps[0].TitleI18n[LocaleSimplifiedChinese] != "创建令牌" ||
		item.Auth[0].SetupGuide.Notices[0].TextI18n[LocaleSimplifiedChinese] != "使用最小权限。" {
		t.Fatalf("Catalog() auth setup guide = %#v", item.Auth)
	}
	if item.Actions[0].NameI18n[LocaleSimplifiedChinese] != "列出 GitHub 议题" || item.Actions[0].DescriptionI18n[LocaleEnglishUS] != action.Description {
		t.Fatalf("Catalog() action localized metadata = %#v", item.Actions[0])
	}
	if len(item.CatalogRevision) != 64 || len(item.Actions) != 1 || len(item.Actions[0].SchemaHash) != 64 || item.Actions[0].SchemaRevision != item.Actions[0].SchemaHash {
		t.Fatalf("Catalog() revisions = %#v", item)
	}
	if item.Actions[0].CatalogRevision != item.CatalogRevision || len(item.Actions[0].RequiredScopes) != 1 ||
		item.Actions[0].RequiredScopes[0] != "issues:read" ||
		!slices.Equal(item.Actions[0].RequiredAnyScopes, []string{"contents:read", "pulls:read"}) ||
		!slices.Equal(item.Actions[0].PreferredScopes, []string{"contents:read"}) ||
		!slices.Equal(item.Actions[0].SupportedAuthMethodIDs, []string{"personal_access_token"}) {
		t.Fatalf("Catalog() action metadata = %#v", item.Actions[0])
	}
	if item.Actions[0].ScopeLabelsI18n["issues:read"][LocaleSimplifiedChinese] != "议题读取权限" {
		t.Fatalf("Catalog() localized scope labels = %#v", item.Actions[0].ScopeLabelsI18n)
	}
	if item.Actions[0].DefaultPolicy.ApprovalPolicy != toolgovernance.ApprovalPolicyNeverAsk {
		t.Fatalf("Catalog() default policy = %#v", item.Actions[0].DefaultPolicy)
	}
	for _, source := range item.CredentialSources {
		if source == ConnectionCredentialSourcePlatform {
			t.Fatalf("Catalog() exposed legacy platform credential source: %#v", item)
		}
	}
	for _, authType := range item.AuthTypes {
		if authType == AuthMethodTypePlatform {
			t.Fatalf("Catalog() exposed legacy platform auth type: %#v", item)
		}
	}
	encoded, err := json.Marshal(integrationCatalogTestEnvelope{Items: items})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if strings.Contains(string(encoded), "input_schema") || strings.Contains(string(encoded), "output_schema") ||
		strings.Contains(string(encoded), `"credential_source":"platform"`) ||
		strings.Contains(string(encoded), `"type":"platform"`) ||
		strings.Contains(string(encoded), "platform_credentials_configured") {
		t.Fatalf("catalog summary leaked full schemas: %s", encoded)
	}

	detail, ok := registry.ActionDetail("github", "github.issue.list")
	if !ok || len(detail.InputSchema) == 0 || detail.SchemaHash != item.Actions[0].SchemaHash {
		t.Fatalf("ActionDetail() = %#v, %v", detail, ok)
	}
	detail.InputSchema["type"] = "array"
	detail.RequiredScopes[0] = "mutated"
	detail.RequiredAnyScopes[0] = "mutated"
	detail.PreferredScopes[0] = "mutated"
	detail.SupportedAuthMethodIDs[0] = "mutated"
	definition, ok := registry.ProviderDefinition("github")
	if !ok {
		t.Fatal("ProviderDefinition() missing")
	}
	definition.AuthMethods[0].Fields[0].Label = "mutated"
	definition.AuthMethods[0].Fields[0].LabelI18n[LocaleSimplifiedChinese] = "已修改"
	definition.AuthMethods[0].SetupGuide.Steps[0].TitleI18n[LocaleSimplifiedChinese] = "已修改"
	definition.AuthMethods[0].SetupGuide.Notices[0].TextI18n[LocaleSimplifiedChinese] = "已修改"
	definition.NameI18n[LocaleEnglishUS] = "mutated"
	definition.DocumentationURLI18n[LocaleSimplifiedChinese] = "https://example.invalid"
	definition.TagLabelsI18n["code"][LocaleSimplifiedChinese] = "已修改"
	definition.CategoryLabelsI18n["development"][LocaleSimplifiedChinese] = "已修改"
	detail.NameI18n[LocaleSimplifiedChinese] = "已修改"
	detail.ScopeLabelsI18n["issues:read"][LocaleSimplifiedChinese] = "已修改"
	again, _ := registry.ActionDetail("github", "github.issue.list")
	definitionAgain, _ := registry.ProviderDefinition("github")
	if again.InputSchema["type"] != "object" || again.RequiredScopes[0] != "issues:read" ||
		!slices.Equal(again.RequiredAnyScopes, []string{"contents:read", "pulls:read"}) ||
		!slices.Equal(again.PreferredScopes, []string{"contents:read"}) ||
		!slices.Equal(again.SupportedAuthMethodIDs, []string{"personal_access_token"}) ||
		definitionAgain.AuthMethods[0].Fields[0].Label != "Token" || definitionAgain.AuthMethods[0].Fields[0].LabelI18n[LocaleSimplifiedChinese] != "令牌" || definitionAgain.NameI18n[LocaleEnglishUS] != "GitHub" || definitionAgain.DocumentationURLI18n[LocaleSimplifiedChinese] != "https://docs.github.com/zh/rest" || definitionAgain.TagLabelsI18n["code"][LocaleSimplifiedChinese] != "代码" || definitionAgain.CategoryLabelsI18n["development"][LocaleSimplifiedChinese] != "软件开发" || again.NameI18n[LocaleSimplifiedChinese] != "列出 GitHub 议题" || again.ScopeLabelsI18n["issues:read"][LocaleSimplifiedChinese] != "议题读取权限" {
		t.Fatalf("registry state was mutated: action=%#v definition=%#v", again, definitionAgain)
	}
	if definitionAgain.AuthMethods[0].SetupGuide.Steps[0].TitleI18n[LocaleSimplifiedChinese] != "创建令牌" ||
		definitionAgain.AuthMethods[0].SetupGuide.Notices[0].TextI18n[LocaleSimplifiedChinese] != "使用最小权限。" {
		t.Fatalf("registry setup guide state was mutated: %#v", definitionAgain.AuthMethods[0].SetupGuide)
	}

	found := registry.SearchActionSummaries(ActionSearchRequest{Query: "issues", Caller: tools.ToolInvokeFromAIChat, Limit: 5})
	if len(found) != 1 || found[0].ID != "github.issue.list" {
		t.Fatalf("SearchActionSummaries() = %#v", found)
	}
	if found := registry.SearchActionSummaries(ActionSearchRequest{Query: "issues", Caller: tools.ToolInvokeFromWorkflow, Limit: 5}); len(found) != 0 {
		t.Fatalf("caller-filtered SearchActionSummaries() = %#v", found)
	}
	localized := registry.SearchActionSummaries(ActionSearchRequest{Query: "议题", Caller: tools.ToolInvokeFromAIChat, Limit: 5})
	if len(localized) != 1 || localized[0].ID != "github.issue.list" {
		t.Fatalf("localized SearchActionSummaries() = %#v", localized)
	}
	localizedScope := registry.SearchActionSummaries(ActionSearchRequest{Query: "议题读取权限", Caller: tools.ToolInvokeFromAIChat, Limit: 5})
	if len(localizedScope) != 1 || localizedScope[0].ScopeLabelsI18n["issues:read"][LocaleSimplifiedChinese] != "议题读取权限" {
		t.Fatalf("scope-localized SearchActionSummaries() = %#v", localizedScope)
	}

	secondRegistry := NewRegistry()
	if err := secondRegistry.Register(registration); err != nil {
		t.Fatalf("second Register() error = %v", err)
	}
	secondDefinition, _ := secondRegistry.ProviderDefinition("github")
	if secondDefinition.CatalogRevision != item.CatalogRevision {
		t.Fatalf("catalog revision is not deterministic: %q != %q", secondDefinition.CatalogRevision, item.CatalogRevision)
	}
}

func TestRegistryCatalogDoesNotAdvertiseUnavailableNoAuthConnectionFlow(t *testing.T) {
	registry := NewRegistry()
	action := testAction("public.read", "read_public_data")
	registration := localizedTestRegistration("public-data", &testAdapter{driverID: "public-data"}, []ActionDefinition{action})
	registration.Definition.AuthMethods = []AuthMethodDefinition{{
		ID: "public", Type: AuthMethodTypeNone, Label: "Public access", Available: true,
	}}
	localizeTestProviderFixture(&registration.Definition)
	if err := registry.Register(registration); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	items := registry.Catalog()
	if len(items) != 1 {
		t.Fatalf("Catalog() = %#v", items)
	}
	item := items[0]
	if item.Enabled || len(item.CredentialSources) != 0 || len(item.AuthTypes) != 0 {
		t.Fatalf("Catalog() advertised unsupported no-auth connection flow: %#v", item)
	}
	if len(item.Auth) != 1 || item.Auth[0].Available {
		t.Fatalf("Catalog() did not preserve fail-closed auth metadata: %#v", item.Auth)
	}
}

type integrationCatalogTestEnvelope struct {
	Items []ProviderCatalogItem `json:"items"`
}

func TestRegistryDoesNotInventProviderAuthorizationScopes(t *testing.T) {
	action := testAction("github.user.get", "get_github_user")
	action.RequiredScopes = nil
	normalized, err := normalizeActionDefinition("github", action)
	if err != nil {
		t.Fatalf("normalizeActionDefinition() error = %v", err)
	}
	if len(normalized.RequiredScopes) != 0 {
		t.Fatalf("RequiredScopes = %#v, want provider-declared scopes only", normalized.RequiredScopes)
	}
}

func TestRegistryValidatesAlternativeAndPreferredScopeContract(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*Registration)
		wantError string
	}{
		{
			name: "alternative group requires a preferred member",
			configure: func(registration *Registration) {
				registration.Definition.Actions[0].RequiredAnyScopes = []string{"messages:read", "messages:history"}
				registration.Definition.Actions[0].PreferredScopes = nil
				localizeTestAction(&registration.Definition.Actions[0])
				addTestProviderScopeDefinitions(&registration.Definition)
			},
			wantError: "alternative scope group must declare exactly one preferred scope",
		},
		{
			name: "alternative group rejects multiple preferred members",
			configure: func(registration *Registration) {
				registration.Definition.Actions[0].RequiredAnyScopes = []string{"messages:read", "messages:history"}
				registration.Definition.Actions[0].PreferredScopes = []string{"messages:read", "messages:history"}
				localizeTestAction(&registration.Definition.Actions[0])
				addTestProviderScopeDefinitions(&registration.Definition)
			},
			wantError: "alternative scope group must declare exactly one preferred scope",
		},
		{
			name: "preferred scope must belong to required union",
			configure: func(registration *Registration) {
				registration.Definition.Actions[0].RequiredAnyScopes = []string{"messages:read"}
				registration.Definition.Actions[0].PreferredScopes = []string{"messages:write"}
				localizeTestAction(&registration.Definition.Actions[0])
				addTestProviderScopeDefinitions(&registration.Definition)
			},
			wantError: "preferred scope messages:write is not part of its required scope union",
		},
		{
			name: "alternative scope must be provider declared",
			configure: func(registration *Registration) {
				registration.Definition.Actions[0].RequiredAnyScopes = []string{"messages:read"}
				registration.Definition.Actions[0].PreferredScopes = []string{"messages:read"}
				localizeTestAction(&registration.Definition.Actions[0])
			},
			wantError: "references undeclared scope messages:read",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			action := testAction("messages.list", "list_messages")
			registration := localizedTestRegistration("chat", &testAdapter{driverID: "chat"}, []ActionDefinition{action})
			test.configure(&registration)
			err := NewRegistry().Register(registration)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Register() error = %v, want %q", err, test.wantError)
			}
		})
	}
}

func TestNormalizeAuthMethodInfersGenericStrategyMetadata(t *testing.T) {
	tests := []struct {
		name        string
		method      AuthMethodDefinition
		wantID      AuthIdentityKind
		wantAcquire AuthAcquisitionStrategy
		wantLife    AuthLifecycleStrategy
		wantRequest RequestAuthStrategy
	}{
		{
			name: "legacy api key",
			method: AuthMethodDefinition{
				ID: "token", Type: AuthMethodTypeAPIKey, CredentialSource: ConnectionCredentialSourceOrganization,
				Label: "Token", Available: true,
				Fields: []CredentialFieldDefinition{{Key: "token", Label: "Token", Input: CredentialFieldInputPassword, Required: true, Secret: true}},
			},
			wantID: AuthIdentityKindApplication, wantAcquire: AuthAcquisitionStrategyManualForm,
			wantLife: AuthLifecycleStrategyStatic, wantRequest: RequestAuthStrategyAPIKeyHeader,
		},
		{
			name: "legacy oauth",
			method: AuthMethodDefinition{
				ID: "oauth", Type: AuthMethodTypeOAuth2, CredentialSource: ConnectionCredentialSourceAccount,
				Label: "OAuth", Available: true, OAuth: &OAuthMethodMetadata{
					ConnectEnabled: true,
					ClientFields: []CredentialFieldDefinition{
						{Key: "client_id", Label: "Client ID", Input: CredentialFieldInputText, Required: true},
						{Key: "client_secret", Label: "Client secret", Input: CredentialFieldInputPassword, Required: true, Secret: true},
					},
				},
			},
			wantID: AuthIdentityKindUser, wantAcquire: AuthAcquisitionStrategyBrowserRedirect,
			wantLife: AuthLifecycleStrategyOAuthRefresh, wantRequest: RequestAuthStrategyBearerHeader,
		},
		{
			name: "legacy service account",
			method: AuthMethodDefinition{
				ID: "service", Type: AuthMethodTypeServiceAccount, CredentialSource: ConnectionCredentialSourceOrganization,
				Label: "Service account", Available: true,
				Fields: []CredentialFieldDefinition{{Key: "secret", Label: "Secret", Input: CredentialFieldInputPassword, Required: true, Secret: true}},
			},
			wantID: AuthIdentityKindService, wantAcquire: AuthAcquisitionStrategyManualForm,
			wantLife: AuthLifecycleStrategyExchangeOnDemand, wantRequest: RequestAuthStrategyProviderCustom,
		},
		{
			name: "no auth",
			method: AuthMethodDefinition{
				ID: "public", Type: AuthMethodTypeNone, Label: "Public", Available: true,
			},
			wantID: AuthIdentityKindApplication, wantAcquire: AuthAcquisitionStrategyNone,
			wantLife: AuthLifecycleStrategyStatic, wantRequest: RequestAuthStrategyNone,
		},
		{
			name: "explicit reusable strategies",
			method: AuthMethodDefinition{
				ID: "webhook", Type: AuthMethodTypeCustomCredential, CredentialSource: ConnectionCredentialSourceOrganization,
				IdentityKind: AuthIdentityKindChannel, AcquisitionStrategy: AuthAcquisitionStrategyManualForm,
				LifecycleStrategy: AuthLifecycleStrategyStatic, RequestAuthStrategy: RequestAuthStrategyWebhookURL,
				Label: "Webhook", Available: true,
				Fields: []CredentialFieldDefinition{{Key: "url", Label: "URL", Input: CredentialFieldInputURL, Required: true, Secret: true}},
			},
			wantID: AuthIdentityKindChannel, wantAcquire: AuthAcquisitionStrategyManualForm,
			wantLife: AuthLifecycleStrategyStatic, wantRequest: RequestAuthStrategyWebhookURL,
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := normalizeAuthMethod("example", testCase.method)
			if err != nil {
				t.Fatalf("normalizeAuthMethod() error = %v", err)
			}
			if got.IdentityKind != testCase.wantID || got.AcquisitionStrategy != testCase.wantAcquire ||
				got.LifecycleStrategy != testCase.wantLife || got.RequestAuthStrategy != testCase.wantRequest {
				t.Fatalf("normalizeAuthMethod() = %#v", got)
			}
			if testCase.method.Type == AuthMethodTypeNone && got.Available {
				t.Fatalf("normalizeAuthMethod() advertised unsupported no-auth connection flow: %#v", got)
			}
		})
	}
}

func TestNormalizeAuthMethodRejectsUnsafeStrategyMetadata(t *testing.T) {
	valid := func() AuthMethodDefinition {
		return AuthMethodDefinition{
			ID: "token", Type: AuthMethodTypeCustomCredential, CredentialSource: ConnectionCredentialSourceOrganization,
			Label: "Token", Available: true,
			Fields: []CredentialFieldDefinition{{Key: "token", Label: "Token", Input: CredentialFieldInputPassword, Required: true, Secret: true}},
		}
	}
	tests := []struct {
		name   string
		mutate func(*AuthMethodDefinition)
		want   string
	}{
		{name: "unknown identity", mutate: func(method *AuthMethodDefinition) { method.IdentityKind = "robot" }, want: "invalid auth strategy"},
		{name: "unknown acquisition", mutate: func(method *AuthMethodDefinition) { method.AcquisitionStrategy = "paste_magic" }, want: "invalid auth strategy"},
		{name: "unknown lifecycle", mutate: func(method *AuthMethodDefinition) { method.LifecycleStrategy = "forever" }, want: "invalid auth strategy"},
		{name: "unknown request auth", mutate: func(method *AuthMethodDefinition) { method.RequestAuthStrategy = "headerish" }, want: "invalid auth strategy"},
		{name: "browser redirect on non oauth", mutate: func(method *AuthMethodDefinition) {
			method.AcquisitionStrategy = AuthAcquisitionStrategyBrowserRedirect
		}, want: "OAuth-only"},
		{name: "signed lifecycle without signer", mutate: func(method *AuthMethodDefinition) {
			method.LifecycleStrategy = AuthLifecycleStrategySignedRequest
			method.RequestAuthStrategy = RequestAuthStrategyBearerHeader
		}, want: "requires a signing"},
		{name: "channel sent as bearer", mutate: func(method *AuthMethodDefinition) {
			method.IdentityKind = AuthIdentityKindChannel
			method.RequestAuthStrategy = RequestAuthStrategyBearerHeader
		}, want: "channel-compatible"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			method := valid()
			testCase.mutate(&method)
			_, err := normalizeAuthMethod("example", method)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("normalizeAuthMethod() error = %v, want containing %q", err, testCase.want)
			}
		})
	}
}

func TestNormalizeAuthMethodValidatesSetupGuide(t *testing.T) {
	valid := func() AuthMethodDefinition {
		return AuthMethodDefinition{
			ID: "oauth", Type: AuthMethodTypeOAuth2, CredentialSource: ConnectionCredentialSourceOrganization,
			Label: "OAuth", Available: true,
			OAuth: &OAuthMethodMetadata{
				ConnectEnabled: true,
				ClientFields: []CredentialFieldDefinition{
					{Key: "client_id", Label: "Client ID", Input: CredentialFieldInputText, Required: true},
				},
			},
			SetupGuide: &AuthSetupGuideDefinition{
				ConsoleURL:       "https://provider.example/console",
				DocumentationURL: "https://provider.example/docs",
				Steps: []AuthSetupStepDefinition{{
					ID: "configure_callback", Title: "Configure callback",
					TitleI18n: LocalizedText{LocaleSimplifiedChinese: "配置回调"},
					Action:    AuthSetupStepActionCopyCallbackURL,
				}},
			},
		}
	}

	normalized, err := normalizeAuthMethod("example", valid())
	if err != nil {
		t.Fatalf("normalizeAuthMethod() error = %v", err)
	}
	if normalized.SetupGuide == nil ||
		normalized.SetupGuide.Steps[0].TitleI18n[LocaleEnglishUS] != "Configure callback" ||
		normalized.SetupGuide.Steps[0].TitleI18n[LocaleSimplifiedChinese] != "配置回调" {
		t.Fatalf("normalized setup guide = %#v", normalized.SetupGuide)
	}

	tests := []struct {
		name   string
		mutate func(*AuthMethodDefinition)
		want   string
	}{
		{name: "unsafe console URL", mutate: func(method *AuthMethodDefinition) {
			method.SetupGuide.ConsoleURL = "http://provider.example/console"
		}, want: "absolute HTTPS URL"},
		{name: "missing steps", mutate: func(method *AuthMethodDefinition) {
			method.SetupGuide.Steps = nil
		}, want: "between 1 and 8 steps"},
		{name: "duplicate steps", mutate: func(method *AuthMethodDefinition) {
			method.SetupGuide.Steps = append(method.SetupGuide.Steps, method.SetupGuide.Steps[0])
		}, want: "is duplicated"},
		{name: "unknown action", mutate: func(method *AuthMethodDefinition) {
			method.SetupGuide.Steps[0].Action = "run_script"
		}, want: "action is invalid"},
		{name: "callback on non OAuth method", mutate: func(method *AuthMethodDefinition) {
			method.Type = AuthMethodTypeAPIKey
			method.OAuth = nil
			method.Fields = []CredentialFieldDefinition{{
				Key: "token", Label: "Token", Input: CredentialFieldInputPassword, Required: true, Secret: true,
			}}
		}, want: "cannot declare a callback setup step"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			method := valid()
			testCase.mutate(&method)
			_, err := normalizeAuthMethod("example", method)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("normalizeAuthMethod() error = %v, want containing %q", err, testCase.want)
			}
		})
	}
}

func TestRegistryDispatchesRegistrationOwnedCapabilities(t *testing.T) {
	tester := &registryDispatchTester{}
	validatorCalled := false
	probe := &registryDispatchProbe{}
	dynamic := &registryDynamicGovernanceResolver{}
	registry := NewRegistry()
	action := testAction("github.issue.create", "create_github_issue")
	action.Effect = toolgovernance.EffectCreate
	action.RiskLevel = toolgovernance.RiskLevelHigh
	action.DefaultPolicy = &DefaultActionPolicy{
		Enabled: true, ApprovalPolicy: toolgovernance.ApprovalPolicyAlwaysAsk, DataEgressAllowed: true,
	}
	definition := ProviderDefinition{
		ID: "github", DriverID: "github-rest", Name: "GitHub",
		AuthMethods: []AuthMethodDefinition{{
			ID: "pat", Type: AuthMethodTypeAPIKey, CredentialSource: ConnectionCredentialSourceOrganization,
			Label: "PAT", Available: true,
			Fields: []CredentialFieldDefinition{{Key: "token", Label: "Token", Input: CredentialFieldInputPassword, Required: true, Secret: true}},
		}},
		HealthProbe: HealthProbeDefinition{Supported: true}, Actions: []ActionDefinition{action},
	}
	localizeTestProviderFixture(&definition)
	if err := registry.Register(Registration{
		Definition:       definition,
		Adapter:          &testAdapter{driverID: "github-rest"},
		ConnectionTester: tester,
		CredentialValidator: CredentialValidatorFunc(func(_ context.Context, request CredentialValidationRequest) error {
			validatorCalled = request.Credentials["token"] == "secret"
			return nil
		}),
		HealthProbe:        probe,
		GovernanceResolver: dynamic,
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	connection := &ResolvedConnection{IntegrationID: "github", DriverID: "github-rest"}
	profile, err := registry.ValidateConnection(context.Background(), connection)
	if err != nil || profile == nil || profile.AccountID != "octocat" || !tester.called {
		t.Fatalf("ValidateConnection() = %#v, %v", profile, err)
	}
	if err := registry.ValidateProviderCredentials(context.Background(), CredentialValidationRequest{
		IntegrationID: "github", DriverID: "github-rest", AuthMethodID: "pat", Credentials: map[string]string{"token": "secret"},
	}); err != nil || !validatorCalled {
		t.Fatalf("ValidateProviderCredentials() error = %v, called = %v", err, validatorCalled)
	}
	validatorCalled = false
	if err := registry.ValidateProviderCredentials(context.Background(), CredentialValidationRequest{
		IntegrationID: "github", DriverID: "github-rest", AuthMethodID: "pat", Credentials: map[string]string{"unknown": "secret"},
	}); ErrorCode(err) != ErrorCodeInvalidInput || validatorCalled {
		t.Fatalf("invalid credentials error = %v, validator called = %v", err, validatorCalled)
	}
	report, err := registry.ProbeConnection(context.Background(), connection)
	if err != nil || report == nil || report.Status != HealthProbeStatusHealthy || !probe.called {
		t.Fatalf("ProbeConnection() = %#v, %v", report, err)
	}
	governance, err := registry.ResolveDynamicActionGovernance(context.Background(), ActionGovernanceRequest{
		IntegrationID: "github", ActionID: "github.issue.create", InvokeFrom: tools.ToolInvokeFromAIChat,
	})
	if err != nil {
		t.Fatalf("ResolveDynamicActionGovernance() error = %v", err)
	}
	if governance.RiskLevel != toolgovernance.RiskLevelHigh || governance.Effect != toolgovernance.EffectCreate ||
		!governance.DataEgress || governance.ExternalDestination != "example.com" || governance.SensitiveDataAllowed ||
		governance.DefaultPolicy.ApprovalPolicy != toolgovernance.ApprovalPolicyAlwaysAsk {
		t.Fatalf("dynamic governance weakened baseline: %#v", governance)
	}
}

func TestRegistryRejectsInvalidProviderDefinitions(t *testing.T) {
	validRegistration := func() Registration {
		action := testAction("github.issue.list", "list_github_issues")
		definition := ProviderDefinition{
			ID: "github", DriverID: "github-rest", Name: "GitHub",
			AuthMethods: []AuthMethodDefinition{{
				ID: "pat", Type: AuthMethodTypeAPIKey, CredentialSource: ConnectionCredentialSourceOrganization,
				Label: "PAT", Available: true,
				Fields: []CredentialFieldDefinition{{Key: "token", Label: "Token", Input: CredentialFieldInputPassword, Required: true, Secret: true}},
			}},
			HealthProbe: HealthProbeDefinition{Supported: true}, Actions: []ActionDefinition{action},
		}
		localizeTestProviderFixture(&definition)
		return Registration{
			Definition: definition,
			Adapter:    &testAdapter{driverID: "github-rest"},
		}
	}
	tests := []struct {
		name   string
		mutate func(*Registration)
		want   string
	}{
		{name: "driver mismatch", mutate: func(value *Registration) { value.Definition.DriverID = "graphql" }, want: "does not match adapter driver"},
		{name: "missing name", mutate: func(value *Registration) { value.Definition.Name = "" }, want: "provider name"},
		{name: "missing auth", mutate: func(value *Registration) { value.Definition.AuthMethods = nil }, want: "auth method"},
		{name: "multiple auth methods without explicit action matrix", mutate: func(value *Registration) {
			second := value.Definition.AuthMethods[0]
			second.ID = "second_pat"
			second.Label = "Second PAT"
			second.LabelI18n = nil
			value.Definition.AuthMethods = append(value.Definition.AuthMethods, second)
		}, want: "must explicitly declare supported auth methods"},
		{name: "invalid localized documentation url", mutate: func(value *Registration) {
			value.Definition.DocumentationURLI18n = LocalizedText{LocaleSimplifiedChinese: "file:///private/docs"}
		}, want: "documentation url for locale"},
		{name: "undeclared localized tag label", mutate: func(value *Registration) {
			value.Definition.TagLabelsI18n = LocalizedLabelMap{"undeclared": {LocaleEnglishUS: "Undeclared"}}
		}, want: "localized tag labels"},
		{name: "undeclared localized category label", mutate: func(value *Registration) {
			value.Definition.CategoryLabelsI18n = LocalizedLabelMap{"undeclared": {LocaleEnglishUS: "Undeclared"}}
		}, want: "localized category labels"},
		{name: "undeclared localized scope label", mutate: func(value *Registration) {
			value.Definition.Actions[0].ScopeLabelsI18n = LocalizedLabelMap{"undeclared:read": {LocaleEnglishUS: "Undeclared"}}
		}, want: "localized scope labels"},
		{name: "duplicate auth", mutate: func(value *Registration) {
			value.Definition.AuthMethods = append(value.Definition.AuthMethods, value.Definition.AuthMethods[0])
		}, want: "is duplicated"},
		{name: "invalid credential source", mutate: func(value *Registration) {
			value.Definition.AuthMethods[0].CredentialSource = ConnectionCredentialSourcePlatform
		}, want: "must use organization or account credentials"},
		{name: "legacy platform auth", mutate: func(value *Registration) {
			value.Definition.AuthMethods[0].Type = AuthMethodTypePlatform
			value.Definition.AuthMethods[0].CredentialSource = ConnectionCredentialSourcePlatform
		}, want: "id, type, and label"},
		{name: "missing credential fields", mutate: func(value *Registration) { value.Definition.AuthMethods[0].Fields = nil }, want: "must declare credential fields"},
		{name: "costly unsupported probe", mutate: func(value *Registration) {
			value.Definition.HealthProbe = HealthProbeDefinition{MayIncurCost: true}
		}, want: "cannot incur cost"},
		{name: "false schema hash", mutate: func(value *Registration) { value.Definition.Actions[0].SchemaHash = strings.Repeat("0", 64) }, want: "schema hash does not match"},
		{name: "false catalog revision", mutate: func(value *Registration) { value.Definition.CatalogRevision = strings.Repeat("0", 64) }, want: "catalog revision does not match"},
		{name: "invalid caller", mutate: func(value *Registration) {
			value.Definition.Actions[0].SupportedCallers = []tools.ToolInvokeFrom{"unknown"}
		}, want: "supported caller"},
		{name: "unknown action auth method", mutate: func(value *Registration) {
			value.Definition.Actions[0].SupportedAuthMethodIDs = []string{"unknown_auth"}
		}, want: "references unknown auth method"},
		{name: "invalid default policy", mutate: func(value *Registration) {
			value.Definition.Actions[0].DefaultPolicy = &DefaultActionPolicy{Enabled: true, ApprovalPolicy: "sometimes", DataEgressAllowed: true}
		}, want: "default approval policy"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			registration := validRegistration()
			testCase.mutate(&registration)
			err := NewRegistry().Register(registration)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("Register() error = %v, want containing %q", err, testCase.want)
			}
		})
	}
}

type registryDispatchTester struct{ called bool }

func (tester *registryDispatchTester) ValidateConnection(context.Context, *ResolvedConnection) (*ConnectionProfile, error) {
	tester.called = true
	return &ConnectionProfile{AccountID: "octocat"}, nil
}

type registryDispatchProbe struct{ called bool }

func (probe *registryDispatchProbe) ProbeConnection(context.Context, *ResolvedConnection) (*HealthProbeReport, error) {
	probe.called = true
	return &HealthProbeReport{Status: HealthProbeStatusHealthy}, nil
}

type registryDynamicGovernanceResolver struct{}

func (*registryDynamicGovernanceResolver) ResolveActionGovernance(_ context.Context, request ActionGovernanceRequest) (ActionDefinition, error) {
	return ActionDefinition{
		ID: request.ActionID, Effect: request.Baseline.Effect, RiskLevel: toolgovernance.RiskLevelLow,
		SensitiveDataAllowed: true,
		DefaultPolicy: &DefaultActionPolicy{
			Enabled: true, ApprovalPolicy: toolgovernance.ApprovalPolicyNeverAsk, DataEgressAllowed: true,
		},
	}, nil
}

func TestRegistryRegisterResolveAndSortActions(t *testing.T) {
	registry := NewRegistry()
	adapter := &testAdapter{driverID: "test"}
	actions := []ActionDefinition{
		testAction(" WEB.FETCH ", "fetch_webpage"),
		testAction(" WEB.SEARCH ", "search_web"),
	}

	if err := registry.Register(localizedTestRegistration(" WEB-SEARCH ", adapter, actions)); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if !registry.Configured("web-search") || !registry.Configured(" WEB-SEARCH ") {
		t.Fatal("Configured() should normalize integration ids")
	}

	resolved, err := registry.Resolve(" WEB-SEARCH ", " WEB.SEARCH ")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.IntegrationID != "web-search" || resolved.Definition.ID != "web.search" || resolved.Adapter != adapter {
		t.Fatalf("Resolve() = %#v", resolved)
	}
	if resolved.Definition.Effect != toolgovernance.EffectRead ||
		resolved.Definition.RiskLevel != toolgovernance.RiskLevelLow ||
		!resolved.Definition.DataEgress || resolved.Definition.ExternalDestination != "example.com" ||
		resolved.Definition.SensitiveDataAllowed {
		t.Fatalf("Resolve() governance = %#v, want normalized action governance", resolved.Definition)
	}

	listed := registry.Actions("web-search")
	if len(listed) != 2 || listed[0].ID != "web.fetch" || listed[1].ID != "web.search" {
		t.Fatalf("Actions() = %#v, want sorted normalized actions", listed)
	}

	_, err = registry.Resolve("web-search", "missing")
	if ErrorCode(err) != ErrorCodeInvalidInput {
		t.Fatalf("Resolve() unknown action error = %v, code = %q", err, ErrorCode(err))
	}
	_, err = registry.Resolve("missing", "web.search")
	if ErrorCode(err) != ErrorCodeDisabled {
		t.Fatalf("Resolve() unknown integration error = %v, code = %q", err, ErrorCode(err))
	}
}

func TestRegistryHasActionUsesNormalizedExactRegistration(t *testing.T) {
	registry := NewRegistry()
	adapter := &testAdapter{driverID: "test"}
	action := testAction("Web.Search", "search_web")
	action.InputSchema = map[string]any{"type": "object"}
	action.OutputSchema = map[string]any{"type": "object"}
	err := registry.Register(localizedTestRegistration("Web-Search", adapter, []ActionDefinition{action}))
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if !registry.HasAction(" WEB-SEARCH ", " WEB.SEARCH ") {
		t.Fatal("HasAction() = false for registered action")
	}
	if registry.HasAction("web-search", "web.fetch") {
		t.Fatal("HasAction() = true for unknown action")
	}
}

func TestRegistryRejectsInvalidRegistration(t *testing.T) {
	validAdapter := &testAdapter{driverID: "test"}
	validAction := testAction("web.search", "search_web")
	invalidSchema := map[string]interface{}{"type": "not-a-json-schema-type"}

	tests := []struct {
		name         string
		registration Registration
		want         string
	}{
		{name: "missing integration id", registration: Registration{Adapter: validAdapter, Actions: []ActionDefinition{validAction}}, want: "integration id is required"},
		{name: "missing adapter", registration: Registration{IntegrationID: "web-search", Actions: []ActionDefinition{validAction}}, want: "adapter is required"},
		{name: "missing driver id", registration: Registration{IntegrationID: "web-search", Adapter: &testAdapter{}, Actions: []ActionDefinition{validAction}}, want: "adapter is required"},
		{name: "missing actions", registration: Registration{IntegrationID: "web-search", Adapter: validAdapter}, want: "at least one action"},
		{name: "missing action id", registration: Registration{IntegrationID: "web-search", Adapter: validAdapter, Actions: []ActionDefinition{testAction("", "search_web")}}, want: "action id and tool name are required"},
		{name: "missing tool name", registration: Registration{IntegrationID: "web-search", Adapter: validAdapter, Actions: []ActionDefinition{testAction("web.search", "")}}, want: "action id and tool name are required"},
		{name: "duplicate action id", registration: Registration{IntegrationID: "web-search", Adapter: validAdapter, Actions: []ActionDefinition{testAction("web.search", "search_web"), testAction(" WEB.SEARCH ", "search_again")}}, want: "action web.search is duplicated"},
		{name: "duplicate tool name", registration: Registration{IntegrationID: "web-search", Adapter: validAdapter, Actions: []ActionDefinition{testAction("web.search", "search_web"), testAction("web.fetch", "search_web")}}, want: "tool search_web is duplicated"},
		{name: "invalid input schema", registration: Registration{IntegrationID: "web-search", Adapter: validAdapter, Actions: []ActionDefinition{func() ActionDefinition { action := validAction; action.InputSchema = invalidSchema; return action }()}}, want: "input schema"},
		{name: "invalid output schema", registration: Registration{IntegrationID: "web-search", Adapter: validAdapter, Actions: []ActionDefinition{func() ActionDefinition { action := validAction; action.OutputSchema = invalidSchema; return action }()}}, want: "output schema"},
		{name: "missing governance effect", registration: Registration{IntegrationID: "web-search", Adapter: validAdapter, Actions: []ActionDefinition{func() ActionDefinition { action := validAction; action.Effect = ""; return action }()}}, want: "governance effect"},
		{name: "invalid governance effect", registration: Registration{IntegrationID: "web-search", Adapter: validAdapter, Actions: []ActionDefinition{func() ActionDefinition { action := validAction; action.Effect = "unsafe"; return action }()}}, want: "governance effect"},
		{name: "missing governance risk", registration: Registration{IntegrationID: "web-search", Adapter: validAdapter, Actions: []ActionDefinition{func() ActionDefinition { action := validAction; action.RiskLevel = ""; return action }()}}, want: "governance risk level"},
		{name: "invalid governance risk", registration: Registration{IntegrationID: "web-search", Adapter: validAdapter, Actions: []ActionDefinition{func() ActionDefinition { action := validAction; action.RiskLevel = "unsafe"; return action }()}}, want: "governance risk level"},
		{name: "missing egress destination", registration: Registration{IntegrationID: "web-search", Adapter: validAdapter, Actions: []ActionDefinition{func() ActionDefinition {
			action := validAction
			action.DataEgress = true
			action.ExternalDestination = ""
			return action
		}()}}, want: "data-egress destination"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewRegistry()
			err := registry.Register(tt.registration)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Register() error = %v, want containing %q", err, tt.want)
			}
		})
	}

	var nilRegistry *Registry
	if err := nilRegistry.Register(Registration{}); err == nil {
		t.Fatal("nil Registry.Register() should fail")
	}
}

func TestRegistryRejectsDuplicateIntegration(t *testing.T) {
	registry := NewRegistry()
	adapter := &testAdapter{driverID: "test"}
	registration := localizedTestRegistration("web-search", adapter, []ActionDefinition{testAction("web.search", "search_web")})
	if err := registry.Register(registration); err != nil {
		t.Fatalf("first Register() error = %v", err)
	}
	if err := registry.Register(registration); err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("duplicate Register() error = %v", err)
	}
}

type testAdapter struct {
	driverID string
	calls    int
	execute  func(context.Context, ActionRequest) (*ActionResult, error)
	events   *[]string
}

func (a *testAdapter) DriverID() string { return a.driverID }

func (a *testAdapter) Execute(ctx context.Context, req ActionRequest) (*ActionResult, error) {
	a.calls++
	appendTestEvent(a.events, "adapter")
	if a.execute != nil {
		return a.execute(ctx, req)
	}
	return &ActionResult{Output: map[string]interface{}{"ok": true}, AttemptCount: 1}, nil
}

func testAction(id, toolName string) ActionDefinition {
	return ActionDefinition{
		ID:          id,
		ToolName:    toolName,
		Name:        "Test action",
		NameI18n:    LocalizedText{LocaleEnglishUS: "Test action", LocaleSimplifiedChinese: "测试操作"},
		Description: "Execute an integration test action.",
		DescriptionI18n: LocalizedText{
			LocaleEnglishUS:         "Execute an integration test action.",
			LocaleSimplifiedChinese: "执行集成测试操作。",
		},
		Effect:              toolgovernance.EffectRead,
		RiskLevel:           toolgovernance.RiskLevelLow,
		DataEgress:          true,
		ExternalDestination: " Example.COM ",
		InputSchema: map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type": "string", "minLength": 1,
					"title_i18n": LocalizedText{LocaleEnglishUS: "Query", LocaleSimplifiedChinese: "查询内容"},
				},
			},
			"required": []string{"query"},
		},
		OutputSchema: map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]interface{}{
				"ok": map[string]interface{}{"type": "boolean"},
			},
			"required": []string{"ok"},
		},
	}
}

func appendTestEvent(events *[]string, event string) {
	if events != nil {
		*events = append(*events, event)
	}
}
