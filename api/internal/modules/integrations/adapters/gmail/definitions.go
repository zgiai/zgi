package gmail

import (
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

const (
	IntegrationID = "gmail"
	DriverID      = "gmail-api"

	ActionGetAccount = "gmail.account.get"
	ActionSendMail   = "gmail.mail.send"

	AccountOAuthAuthMethodID      = "google_oauth"
	OrganizationOAuthAuthMethodID = "organization_google_oauth"

	ScopeOpenID   = "openid"
	ScopeEmail    = "email"
	ScopeMailSend = "https://www.googleapis.com/auth/gmail.send"
)

func ProviderDefinition() integrations.ProviderDefinition {
	return integrations.ProviderDefinition{
		ID:       IntegrationID,
		DriverID: DriverID,
		Name:     "Gmail",
		NameI18n: integrations.LocalizedText{
			integrations.LocaleEnglishUS:         "Gmail",
			integrations.LocaleSimplifiedChinese: "谷歌邮箱",
		},
		Description: "Connect a Google account to inspect its identity and send Gmail messages with explicit approval.",
		DescriptionI18n: integrations.LocalizedText{
			integrations.LocaleEnglishUS:         "Connect a Google account to inspect its identity and send Gmail messages with explicit approval.",
			integrations.LocaleSimplifiedChinese: "连接 Google 账号以读取账号身份，并在明确确认后发送 Gmail 邮件。",
		},
		Author: "ZGI",
		Icon:   "mail",
		Tags:   []string{"email", "communication", "external"},
		TagLabelsI18n: integrations.LocalizedLabelMap{
			"email":         {integrations.LocaleEnglishUS: "Email", integrations.LocaleSimplifiedChinese: "电子邮件"},
			"communication": {integrations.LocaleEnglishUS: "Communication", integrations.LocaleSimplifiedChinese: "沟通协作"},
			"external":      {integrations.LocaleEnglishUS: "External", integrations.LocaleSimplifiedChinese: "外部服务"},
		},
		Categories: []string{"communication"},
		CategoryLabelsI18n: integrations.LocalizedLabelMap{
			"communication": {integrations.LocaleEnglishUS: "Communication", integrations.LocaleSimplifiedChinese: "沟通协作"},
		},
		DocumentationURL: "https://developers.google.com/gmail/api",
		DocumentationURLI18n: integrations.LocalizedText{
			integrations.LocaleEnglishUS:         "https://developers.google.com/gmail/api",
			integrations.LocaleSimplifiedChinese: "https://developers.google.com/gmail/api",
		},
		AuthMethods: []integrations.AuthMethodDefinition{
			gmailOAuthMethod(AccountOAuthAuthMethodID, integrations.ConnectionCredentialSourceAccount, "Connect my Google account", "连接我的 Google 账号"),
			gmailOAuthMethod(OrganizationOAuthAuthMethodID, integrations.ConnectionCredentialSourceOrganization, "Connect an organization Google account", "连接组织 Google 账号"),
		},
		HealthProbe: integrations.HealthProbeDefinition{
			Supported:    true,
			MayIncurCost: false,
			Description:  "Reads the signed-in Google identity without sending email.",
			DescriptionI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS:         "Reads the signed-in Google identity without sending email.",
				integrations.LocaleSimplifiedChinese: "读取已登录的 Google 身份，不会发送邮件。",
			},
		},
		Scopes: []integrations.ProviderScopeDefinition{
			{
				ID: ScopeOpenID, Label: "Verify Google identity",
				LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Verify Google identity", integrations.LocaleSimplifiedChinese: "验证 Google 身份",
				},
				Category: integrations.ProviderScopeCategoryIdentity, Access: integrations.ProviderScopeAccessIdentity,
			},
			{
				ID: ScopeEmail, Label: "Read account email address",
				LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Read account email address", integrations.LocaleSimplifiedChinese: "读取账号邮箱地址",
				},
				Category: integrations.ProviderScopeCategoryIdentity, Access: integrations.ProviderScopeAccessRead,
			},
			{
				ID: "profile", Label: "Read basic Google profile",
				LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Read basic Google profile", integrations.LocaleSimplifiedChinese: "读取基础 Google 资料",
				},
				Category: integrations.ProviderScopeCategoryIdentity, Access: integrations.ProviderScopeAccessRead,
			},
			{
				ID: ScopeMailSend, Label: "Send email on your behalf",
				LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Send email on your behalf", integrations.LocaleSimplifiedChinese: "代表你发送电子邮件",
				},
				Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessWrite, Broad: true,
			},
		},
		Actions: Actions(),
	}
}

func gmailOAuthMethod(id string, source integrations.ConnectionCredentialSource, label, chineseLabel string) integrations.AuthMethodDefinition {
	return integrations.AuthMethodDefinition{
		ID:                  id,
		Type:                integrations.AuthMethodTypeOAuth2,
		CredentialSource:    source,
		IdentityKind:        integrations.AuthIdentityKindUser,
		AcquisitionStrategy: integrations.AuthAcquisitionStrategyBrowserRedirect,
		LifecycleStrategy:   integrations.AuthLifecycleStrategyOAuthRefresh,
		RequestAuthStrategy: integrations.RequestAuthStrategyBearerHeader,
		Label:               label,
		LabelI18n: integrations.LocalizedText{
			integrations.LocaleEnglishUS:         label,
			integrations.LocaleSimplifiedChinese: chineseLabel,
		},
		Description: "Google OAuth 2.0 opens a Google authorization page. ZGI never asks for your Google password.",
		DescriptionI18n: integrations.LocalizedText{
			integrations.LocaleEnglishUS:         "Google OAuth 2.0 opens a Google authorization page. ZGI never asks for your Google password.",
			integrations.LocaleSimplifiedChinese: "Google OAuth 2.0 会打开 Google 授权页面，ZGI 不会要求输入 Google 密码。",
		},
		Available:  true,
		SetupGuide: gmailOAuthSetupGuide(),
		OAuth: &integrations.OAuthMethodMetadata{
			ConnectEnabled: true, ReconnectEnabled: true, ScopeUpgradeEnabled: true,
			ClientConfigID:   IntegrationID,
			ProviderSetupURL: "https://console.cloud.google.com/apis/credentials",
			IdentityScopes:   []string{ScopeOpenID, ScopeEmail},
			DefaultActionIDs: []string{ActionGetAccount},
			ClientFields: []integrations.CredentialFieldDefinition{
				{
					Key: "client_id", Label: "Google OAuth client ID",
					LabelI18n: integrations.LocalizedText{integrations.LocaleEnglishUS: "Google OAuth client ID", integrations.LocaleSimplifiedChinese: "Google OAuth 客户端 ID"},
					Input:     integrations.CredentialFieldInputText, Required: true, Secret: false,
				},
				{
					Key: "client_secret", Label: "Google OAuth client secret",
					LabelI18n: integrations.LocalizedText{integrations.LocaleEnglishUS: "Google OAuth client secret", integrations.LocaleSimplifiedChinese: "Google OAuth 客户端密钥"},
					Input:     integrations.CredentialFieldInputPassword, Required: true, Secret: true,
				},
			},
		},
	}
}

func gmailOAuthSetupGuide() *integrations.AuthSetupGuideDefinition {
	return &integrations.AuthSetupGuideDefinition{
		ConsoleURL:       "https://console.cloud.google.com/auth/clients",
		DocumentationURL: "https://developers.google.com/identity/protocols/oauth2/web-server",
		Steps: []integrations.AuthSetupStepDefinition{
			{
				ID: "select_project", Title: "Create or select a Google Cloud project",
				TitleI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Create or select a Google Cloud project", integrations.LocaleSimplifiedChinese: "创建或选择 Google Cloud 项目",
				},
				Description: "Use a project owned by your organization. Keep development and production OAuth applications in separate projects.",
				DescriptionI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Use a project owned by your organization. Keep development and production OAuth applications in separate projects.",
					integrations.LocaleSimplifiedChinese: "使用组织持有的项目，并为开发环境与生产环境分别创建 OAuth 应用。",
				},
				Action: integrations.AuthSetupStepActionOpenConsole,
			},
			{
				ID: "enable_gmail_api", Title: "Enable the Gmail API",
				TitleI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Enable the Gmail API", integrations.LocaleSimplifiedChinese: "启用 Gmail API",
				},
				Description: "Open APIs & Services in the selected project and enable the Gmail API before connecting an account.",
				DescriptionI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Open APIs & Services in the selected project and enable the Gmail API before connecting an account.",
					integrations.LocaleSimplifiedChinese: "在所选项目的“API 和服务”中启用 Gmail API，然后再连接账号。",
				},
			},
			{
				ID: "configure_consent", Title: "Configure the consent screen and audience",
				TitleI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Configure the consent screen and audience", integrations.LocaleSimplifiedChinese: "配置同意屏幕与用户范围",
				},
				Description: "Complete Branding, Audience, and Data Access. When the app is in Testing, add every account that needs to connect as a test user.",
				DescriptionI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Complete Branding, Audience, and Data Access. When the app is in Testing, add every account that needs to connect as a test user.",
					integrations.LocaleSimplifiedChinese: "完成 Branding、Audience 和 Data Access；应用处于测试状态时，将需要连接的账号加入测试用户。",
				},
				Action: integrations.AuthSetupStepActionOpenDocumentation,
			},
			{
				ID: "create_web_client", Title: "Create a Web application OAuth client",
				TitleI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Create a Web application OAuth client", integrations.LocaleSimplifiedChinese: "创建 Web application OAuth 客户端",
				},
				Description: "In Google Auth Platform > Clients, create a client whose application type is Web application.",
				DescriptionI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "In Google Auth Platform > Clients, create a client whose application type is Web application.",
					integrations.LocaleSimplifiedChinese: "在 Google Auth Platform 的 Clients 页面创建客户端，应用类型选择 Web application。",
				},
			},
			{
				ID: "configure_callback", Title: "Add the authorized redirect URI",
				TitleI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Add the authorized redirect URI", integrations.LocaleSimplifiedChinese: "添加已获授权的重定向 URI",
				},
				Description: "Add the exact callback URL shown by ZGI. Scheme, host, path, case, and trailing slash must match.",
				DescriptionI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Add the exact callback URL shown by ZGI. Scheme, host, path, case, and trailing slash must match.",
					integrations.LocaleSimplifiedChinese: "添加 ZGI 展示的完整回调地址；协议、域名、路径、大小写和末尾斜杠必须完全一致。",
				},
				Action: integrations.AuthSetupStepActionCopyCallbackURL,
			},
			{
				ID: "save_in_zgi", Title: "Save the Client ID and Client Secret",
				TitleI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Save the Client ID and Client Secret", integrations.LocaleSimplifiedChinese: "返回 ZGI 保存 Client ID 和 Client Secret",
				},
				Description: "Copy the credentials from the Google client, paste them below, and save the OAuth application.",
				DescriptionI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Copy the credentials from the Google client, paste them below, and save the OAuth application.",
					integrations.LocaleSimplifiedChinese: "从 Google 客户端复制凭据，粘贴到下方并保存 OAuth 应用。",
				},
			},
		},
		Notices: []integrations.AuthSetupNoticeDefinition{
			{
				ID: "testing_users", Level: integrations.AuthSetupNoticeLevelWarning,
				Text: "A Google OAuth application in Testing can only authorize accounts listed as test users.",
				TextI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "A Google OAuth application in Testing can only authorize accounts listed as test users.",
					integrations.LocaleSimplifiedChinese: "处于 Testing 状态的 Google OAuth 应用只能授权已加入测试用户列表的账号。",
				},
			},
			{
				ID: "scope_verification", Level: integrations.AuthSetupNoticeLevelInfo,
				Text: "Production use of sensitive Gmail scopes may require Google OAuth verification. Request only scopes used by enabled actions.",
				TextI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Production use of sensitive Gmail scopes may require Google OAuth verification. Request only scopes used by enabled actions.",
					integrations.LocaleSimplifiedChinese: "生产环境使用 Gmail 敏感权限可能需要 Google OAuth 验证；只申请已启用操作实际需要的权限。",
				},
			},
		},
	}
}

func Actions() []integrations.ActionDefinition {
	actions := []integrations.ActionDefinition{
		{
			ID:       ActionGetAccount,
			ToolName: "get_gmail_account",
			Name:     "Get Gmail account",
			NameI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS:         "Get Gmail account",
				integrations.LocaleSimplifiedChinese: "获取 Gmail 账号",
			},
			Description: "Return the bounded Google identity represented by this Gmail connection.",
			DescriptionI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS:         "Return the bounded Google identity represented by this Gmail connection.",
				integrations.LocaleSimplifiedChinese: "返回此 Gmail 连接所代表的受限 Google 身份信息。",
			},
			InputSchema: strictObjectSchema(map[string]interface{}{}, nil),
			OutputSchema: strictObjectSchema(map[string]interface{}{
				"provider": map[string]interface{}{"const": IntegrationID}, "request_id": boundedStringSchema(128),
				"account": strictObjectSchema(map[string]interface{}{
					"id": boundedStringSchema(255), "email": boundedStringSchema(320),
					"name": boundedStringSchema(255), "picture": boundedStringSchema(2048),
					"email_verified": map[string]interface{}{"type": "boolean"},
				}, []string{"id", "email", "name", "picture", "email_verified"}),
			}, []string{"provider", "request_id", "account"}),
			Effect: toolgovernance.EffectRead, RiskLevel: toolgovernance.RiskLevelLow,
			DataEgress: true, ExternalDestination: "openidconnect.googleapis.com",
			SensitiveDataAllowed: false, Idempotent: true,
			RequiredScopes: []string{ScopeOpenID, ScopeEmail},
			ScopeLabelsI18n: integrations.LocalizedLabelMap{
				ScopeOpenID: {integrations.LocaleEnglishUS: "Verify Google identity", integrations.LocaleSimplifiedChinese: "验证 Google 身份"},
				ScopeEmail:  {integrations.LocaleEnglishUS: "Read account email address", integrations.LocaleSimplifiedChinese: "读取账号邮箱地址"},
			},
			DefaultPolicy: readOnlyPolicy(),
			SupportedCallers: []tools.ToolInvokeFrom{
				tools.ToolInvokeFromAIChat, tools.ToolInvokeFromAgent,
			},
		},
		{
			ID:       ActionSendMail,
			ToolName: "send_gmail_message",
			Name:     "Send Gmail message",
			NameI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS:         "Send Gmail message",
				integrations.LocaleSimplifiedChinese: "发送 Gmail 邮件",
			},
			Description: "Send one plain-text email from the connected Gmail account. Every invocation requires explicit approval.",
			DescriptionI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS:         "Send one plain-text email from the connected Gmail account. Every invocation requires explicit approval.",
				integrations.LocaleSimplifiedChinese: "使用已连接的 Gmail 账号发送一封纯文本邮件，每次调用都必须明确确认。",
			},
			InputSchema: strictObjectSchema(map[string]interface{}{
				"to":        localizedSchema(map[string]interface{}{"type": "string", "minLength": 3, "maxLength": 4000}, "Recipients", "收件人"),
				"cc":        localizedSchema(map[string]interface{}{"type": "string", "maxLength": 4000}, "CC recipients", "抄送人"),
				"subject":   localizedSchema(map[string]interface{}{"type": "string", "minLength": 1, "maxLength": 998}, "Subject", "主题"),
				"body_text": localizedSchema(map[string]interface{}{"type": "string", "minLength": 1, "maxLength": 100000}, "Plain-text body", "纯文本正文"),
			}, []string{"to", "subject", "body_text"}),
			OutputSchema: strictObjectSchema(map[string]interface{}{
				"provider": map[string]interface{}{"const": IntegrationID}, "request_id": boundedStringSchema(128),
				"message": strictObjectSchema(map[string]interface{}{
					"id": boundedStringSchema(255), "thread_id": boundedStringSchema(255),
					"label_ids": map[string]interface{}{"type": "array", "maxItems": 20, "items": boundedStringSchema(100)},
				}, []string{"id", "thread_id", "label_ids"}),
			}, []string{"provider", "request_id", "message"}),
			Effect: toolgovernance.EffectExternalSend, RiskLevel: toolgovernance.RiskLevelHigh,
			DataEgress: true, ExternalDestination: "gmail.googleapis.com",
			SensitiveDataAllowed: false, Idempotent: false,
			RequiredScopes: []string{ScopeMailSend},
			ScopeLabelsI18n: integrations.LocalizedLabelMap{
				ScopeMailSend: {integrations.LocaleEnglishUS: "Send email on your behalf", integrations.LocaleSimplifiedChinese: "代表你发送电子邮件"},
			},
			DefaultPolicy: alwaysAskPolicy(),
			SupportedCallers: []tools.ToolInvokeFrom{
				tools.ToolInvokeFromAIChat, tools.ToolInvokeFromAgent,
			},
		},
	}
	for index := range actions {
		actions[index].SupportedAuthMethodIDs = []string{AccountOAuthAuthMethodID, OrganizationOAuthAuthMethodID}
	}
	return actions
}

func readOnlyPolicy() *integrations.DefaultActionPolicy {
	return &integrations.DefaultActionPolicy{
		Enabled: true, ApprovalPolicy: toolgovernance.ApprovalPolicyNeverAsk, DataEgressAllowed: true,
	}
}

func alwaysAskPolicy() *integrations.DefaultActionPolicy {
	return &integrations.DefaultActionPolicy{
		Enabled: false, ApprovalPolicy: toolgovernance.ApprovalPolicyAlwaysAsk, DataEgressAllowed: true,
	}
}

func strictObjectSchema(properties map[string]interface{}, required []string) map[string]interface{} {
	schema := map[string]interface{}{
		"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object",
		"additionalProperties": false, "properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func boundedStringSchema(maxLength int) map[string]interface{} {
	return map[string]interface{}{"type": "string", "maxLength": maxLength}
}

func localizedSchema(schema map[string]interface{}, english, chinese string) map[string]interface{} {
	schema["title_i18n"] = integrations.LocalizedText{
		integrations.LocaleEnglishUS: english, integrations.LocaleSimplifiedChinese: chinese,
	}
	return schema
}
