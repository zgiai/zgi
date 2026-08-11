package x

import (
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

const (
	IntegrationID = "x"
	DriverID      = "x-api-v2"

	ActionGetAccount        = "x.account.get"
	ActionGetUserByUsername = "x.user.get_by_username"
	ActionListOwnPosts      = "x.post.list_own"
	ActionListPostsByUser   = "x.post.list_by_user"
	ActionSearchRecentPosts = "x.post.search_recent"
	ActionCreatePost        = "x.post.create"

	AccountOAuthAuthMethodID      = "x_oauth"
	OrganizationOAuthAuthMethodID = "organization_x_oauth"
	AppBearerAuthMethodID         = "x_app_bearer"

	ScopeUsersRead     = "users.read"
	ScopePostsRead     = "tweet.read"
	ScopePostsWrite    = "tweet.write"
	ScopeOfflineAccess = "offline.access"
)

func ProviderDefinition() integrations.ProviderDefinition {
	return integrations.ProviderDefinition{
		ID:       IntegrationID,
		DriverID: DriverID,
		Name:     "X",
		NameI18n: integrations.LocalizedText{
			integrations.LocaleEnglishUS: "X", integrations.LocaleSimplifiedChinese: "X",
		},
		Description: "Connect an X account to inspect the profile, read posts, and publish only with explicit approval.",
		DescriptionI18n: integrations.LocalizedText{
			integrations.LocaleEnglishUS:         "Connect an X account to inspect the profile, read posts, and publish only with explicit approval.",
			integrations.LocaleSimplifiedChinese: "连接 X 账号以查看资料和帖子，并仅在明确确认后发布内容。",
		},
		Author: "ZGI",
		Icon:   "at-sign",
		Tags:   []string{"social", "communication", "external"},
		TagLabelsI18n: integrations.LocalizedLabelMap{
			"social":        {integrations.LocaleEnglishUS: "Social", integrations.LocaleSimplifiedChinese: "社交媒体"},
			"communication": {integrations.LocaleEnglishUS: "Communication", integrations.LocaleSimplifiedChinese: "沟通"},
			"external":      {integrations.LocaleEnglishUS: "External", integrations.LocaleSimplifiedChinese: "外部服务"},
		},
		Categories: []string{"communication"},
		CategoryLabelsI18n: integrations.LocalizedLabelMap{
			"communication": {integrations.LocaleEnglishUS: "Communication", integrations.LocaleSimplifiedChinese: "沟通协作"},
		},
		DocumentationURL: "https://docs.x.com/x-api",
		DocumentationURLI18n: integrations.LocalizedText{
			integrations.LocaleEnglishUS:         "https://docs.x.com/x-api",
			integrations.LocaleSimplifiedChinese: "https://docs.x.com/x-api",
		},
		AuthMethods: []integrations.AuthMethodDefinition{
			xOAuthMethod(AccountOAuthAuthMethodID, integrations.ConnectionCredentialSourceAccount, "Connect my X account", "连接我的 X 账号"),
			xOAuthMethod(OrganizationOAuthAuthMethodID, integrations.ConnectionCredentialSourceOrganization, "Connect an organization X account", "连接组织 X 账号"),
			xAppBearerMethod(),
		},
		HealthProbe: integrations.HealthProbeDefinition{
			Supported:    true,
			MayIncurCost: false,
			Description:  "Reads the authenticated X profile without publishing a post.",
			DescriptionI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS:         "Reads the authenticated X profile without publishing a post.",
				integrations.LocaleSimplifiedChinese: "读取已认证的 X 账号资料，不会发布帖子。",
			},
		},
		Scopes: []integrations.ProviderScopeDefinition{
			{
				ID: ScopeUsersRead, Label: "Read account profile",
				LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Read account profile", integrations.LocaleSimplifiedChinese: "读取账号资料",
				},
				Category: integrations.ProviderScopeCategoryIdentity, Access: integrations.ProviderScopeAccessRead,
			},
			{
				ID: ScopePostsRead, Label: "Read posts",
				LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Read posts", integrations.LocaleSimplifiedChinese: "读取帖子",
				},
				Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessRead,
			},
			{
				ID: ScopePostsWrite, Label: "Create posts",
				LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Create posts", integrations.LocaleSimplifiedChinese: "发布帖子",
				},
				Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessWrite, Broad: true,
			},
			{
				ID: ScopeOfflineAccess, Label: "Keep the connection signed in",
				LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Keep the connection signed in", integrations.LocaleSimplifiedChinese: "保持连接登录",
				},
				Category: integrations.ProviderScopeCategoryLifecycle, Access: integrations.ProviderScopeAccessSession,
			},
		},
		Actions: Actions(),
	}
}

func xAppBearerMethod() integrations.AuthMethodDefinition {
	return integrations.AuthMethodDefinition{
		ID:                  AppBearerAuthMethodID,
		Type:                integrations.AuthMethodTypeCustomCredential,
		CredentialSource:    integrations.ConnectionCredentialSourceOrganization,
		IdentityKind:        integrations.AuthIdentityKindApplication,
		AcquisitionStrategy: integrations.AuthAcquisitionStrategyManualForm,
		LifecycleStrategy:   integrations.AuthLifecycleStrategyStatic,
		RequestAuthStrategy: integrations.RequestAuthStrategyBearerHeader,
		ScopeEvidence:       integrations.AuthScopeEvidenceConnectorDeclared,
		Label:               "X app Bearer Token",
		LabelI18n: integrations.LocalizedText{
			integrations.LocaleEnglishUS:         "X app Bearer Token",
			integrations.LocaleSimplifiedChinese: "X 应用 Bearer Token",
		},
		Description: "Use an X app-only Bearer Token for public, read-only data. It cannot act as an X user.",
		DescriptionI18n: integrations.LocalizedText{
			integrations.LocaleEnglishUS:         "Use an X app-only Bearer Token for public, read-only data. It cannot act as an X user.",
			integrations.LocaleSimplifiedChinese: "使用 X 应用级 Bearer Token 读取公开数据，不能代表 X 用户执行操作。",
		},
		Available:  true,
		SetupGuide: xAppBearerSetupGuide(),
		Fields: []integrations.CredentialFieldDefinition{{
			Key:   "bearer_token",
			Label: "Bearer Token",
			LabelI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS:         "Bearer Token",
				integrations.LocaleSimplifiedChinese: "Bearer Token",
			},
			Description: "Copy the app-only Bearer Token from the X Developer Console. It is encrypted before storage and is never returned by the API.",
			DescriptionI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS:         "Copy the app-only Bearer Token from the X Developer Console. It is encrypted before storage and is never returned by the API.",
				integrations.LocaleSimplifiedChinese: "从 X 开发者控制台复制应用级 Bearer Token。令牌会在保存前加密，API 永远不会返回原文。",
			},
			Input:       integrations.CredentialFieldInputPassword,
			Required:    true,
			Secret:      true,
			Placeholder: "Paste the X app Bearer Token",
			PlaceholderI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS:         "Paste the X app Bearer Token",
				integrations.LocaleSimplifiedChinese: "粘贴 X 应用 Bearer Token",
			},
		}},
	}
}

func xAppBearerSetupGuide() *integrations.AuthSetupGuideDefinition {
	return &integrations.AuthSetupGuideDefinition{
		ConsoleURL:       "https://developer.x.com/en/portal/dashboard",
		DocumentationURL: "https://docs.x.com/fundamentals/authentication/oauth-2-0/bearer-tokens",
		Steps: []integrations.AuthSetupStepDefinition{
			{
				ID: "open_app", Title: "Open your X developer app",
				TitleI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Open your X developer app",
					integrations.LocaleSimplifiedChinese: "打开 X 开发者应用",
				},
				Description: "Open the X Developer Portal and select the project and app that will own this connection.",
				DescriptionI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Open the X Developer Portal and select the project and app that will own this connection.",
					integrations.LocaleSimplifiedChinese: "打开 X Developer Portal，选择负责此连接的 Project 和 App。",
				},
				Action: integrations.AuthSetupStepActionOpenConsole,
			},
			{
				ID: "open_keys", Title: "Open Keys and tokens",
				TitleI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Open Keys and tokens",
					integrations.LocaleSimplifiedChinese: "进入 Keys and tokens",
				},
				Description: "In the selected app, open Keys and tokens and locate the app-only Bearer Token.",
				DescriptionI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "In the selected app, open Keys and tokens and locate the app-only Bearer Token.",
					integrations.LocaleSimplifiedChinese: "在所选 App 中进入 Keys and tokens，找到应用级 Bearer Token。",
				},
				Action: integrations.AuthSetupStepActionOpenDocumentation,
			},
			{
				ID: "confirm_usage", Title: "Confirm app-only access is appropriate",
				TitleI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Confirm app-only access is appropriate",
					integrations.LocaleSimplifiedChinese: "确认应用级访问符合使用场景",
				},
				Description: "An app-only Bearer Token can read supported public data. It has no user context and cannot publish posts or act as a user.",
				DescriptionI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "An app-only Bearer Token can read supported public data. It has no user context and cannot publish posts or act as a user.",
					integrations.LocaleSimplifiedChinese: "应用级 Bearer Token 只能读取受支持的公开数据；它没有用户上下文，不能发布帖子或代表用户操作。",
				},
			},
			{
				ID: "paste_token", Title: "Copy and paste the Bearer Token into ZGI",
				TitleI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Copy and paste the Bearer Token into ZGI",
					integrations.LocaleSimplifiedChinese: "复制 Bearer Token 并粘贴到 ZGI",
				},
				Description: "Paste the token value below without adding the word Bearer or quotation marks.",
				DescriptionI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Paste the token value below without adding the word Bearer or quotation marks.",
					integrations.LocaleSimplifiedChinese: "下方只粘贴 Token 本身，不要添加 Bearer 前缀或引号。",
				},
			},
		},
		Notices: []integrations.AuthSetupNoticeDefinition{
			{
				ID: "app_only", Level: integrations.AuthSetupNoticeLevelWarning,
				Text: "Use OAuth user authorization instead when an action must access private account data or act on behalf of an X user.",
				TextI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Use OAuth user authorization instead when an action must access private account data or act on behalf of an X user.",
					integrations.LocaleSimplifiedChinese: "如需访问私有账号数据或代表 X 用户操作，请改用用户 OAuth 授权。",
				},
			},
			{
				ID: "secret_storage", Level: integrations.AuthSetupNoticeLevelInfo,
				Text: "Treat the Bearer Token as a password. ZGI encrypts it before storage and never returns the original value.",
				TextI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Treat the Bearer Token as a password. ZGI encrypts it before storage and never returns the original value.",
					integrations.LocaleSimplifiedChinese: "请将 Bearer Token 视为密码；ZGI 会加密保存且不会返回原文。",
				},
			},
		},
	}
}

func xOAuthMethod(id string, source integrations.ConnectionCredentialSource, label, chineseLabel string) integrations.AuthMethodDefinition {
	return integrations.AuthMethodDefinition{
		ID: id, Type: integrations.AuthMethodTypeOAuth2, CredentialSource: source,
		IdentityKind:        integrations.AuthIdentityKindUser,
		AcquisitionStrategy: integrations.AuthAcquisitionStrategyBrowserRedirect,
		LifecycleStrategy:   integrations.AuthLifecycleStrategyOAuthRefresh,
		RequestAuthStrategy: integrations.RequestAuthStrategyBearerHeader,
		Label:               label,
		LabelI18n: integrations.LocalizedText{
			integrations.LocaleEnglishUS: label, integrations.LocaleSimplifiedChinese: chineseLabel,
		},
		Description: "Opens the official X OAuth 2.0 authorization page with PKCE.",
		DescriptionI18n: integrations.LocalizedText{
			integrations.LocaleEnglishUS:         "Opens the official X OAuth 2.0 authorization page with PKCE.",
			integrations.LocaleSimplifiedChinese: "打开 X 官方 OAuth 2.0 授权页面，并使用 PKCE 保护授权流程。",
		},
		Available:  true,
		SetupGuide: xOAuthSetupGuide(),
		OAuth: &integrations.OAuthMethodMetadata{
			ConnectEnabled: true, ReconnectEnabled: true, ScopeUpgradeEnabled: true,
			ClientConfigID:   IntegrationID,
			ProviderSetupURL: "https://developer.x.com/en/portal/dashboard",
			IdentityScopes:   []string{ScopeUsersRead, ScopePostsRead},
			DefaultActionIDs: []string{ActionGetAccount, ActionListOwnPosts},
			ClientFields: []integrations.CredentialFieldDefinition{
				{
					Key: "client_id", Label: "X OAuth client ID",
					LabelI18n: integrations.LocalizedText{integrations.LocaleEnglishUS: "X OAuth client ID", integrations.LocaleSimplifiedChinese: "X OAuth 客户端 ID"},
					Input:     integrations.CredentialFieldInputText, Required: true, Secret: false,
				},
				{
					Key: "client_secret", Label: "X OAuth client secret",
					LabelI18n: integrations.LocalizedText{integrations.LocaleEnglishUS: "X OAuth client secret", integrations.LocaleSimplifiedChinese: "X OAuth 客户端密钥"},
					Input:     integrations.CredentialFieldInputPassword, Required: false, Secret: true,
				},
			},
		},
	}
}

func xOAuthSetupGuide() *integrations.AuthSetupGuideDefinition {
	return &integrations.AuthSetupGuideDefinition{
		ConsoleURL:       "https://developer.x.com/en/portal/dashboard",
		DocumentationURL: "https://docs.x.com/fundamentals/authentication/oauth-2-0/authorization-code",
		Steps: []integrations.AuthSetupStepDefinition{
			{
				ID: "select_app", Title: "Create or select an X developer app",
				TitleI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Create or select an X developer app", integrations.LocaleSimplifiedChinese: "创建或选择 X 开发者应用",
				},
				Description: "Open the X Developer Portal and select the project and app that your organization will use.",
				DescriptionI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Open the X Developer Portal and select the project and app that your organization will use.",
					integrations.LocaleSimplifiedChinese: "打开 X Developer Portal，选择组织将使用的 Project 和 App。",
				},
				Action: integrations.AuthSetupStepActionOpenConsole,
			},
			{
				ID: "enable_oauth2", Title: "Enable OAuth 2.0 with PKCE",
				TitleI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Enable OAuth 2.0 with PKCE", integrations.LocaleSimplifiedChinese: "启用 OAuth 2.0 与 PKCE",
				},
				Description: "Open User authentication settings and enable OAuth 2.0 Authorization Code with PKCE.",
				DescriptionI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Open User authentication settings and enable OAuth 2.0 Authorization Code with PKCE.",
					integrations.LocaleSimplifiedChinese: "进入 User authentication settings，启用 OAuth 2.0 Authorization Code with PKCE。",
				},
				Action: integrations.AuthSetupStepActionOpenDocumentation,
			},
			{
				ID: "choose_client_type", Title: "Choose the correct client type",
				TitleI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Choose the correct client type", integrations.LocaleSimplifiedChinese: "选择正确的客户端类型",
				},
				Description: "Use Web App, Automated App, or Bot for a confidential server client. Native App and Single Page App are public clients and do not use a client secret.",
				DescriptionI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Use Web App, Automated App, or Bot for a confidential server client. Native App and Single Page App are public clients and do not use a client secret.",
					integrations.LocaleSimplifiedChinese: "服务端机密客户端请选择 Web App、Automated App 或 Bot；Native App 和 Single Page App 属于公有客户端，不使用 Client Secret。",
				},
			},
			{
				ID: "configure_callback", Title: "Add the callback URI and website URL",
				TitleI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Add the callback URI and website URL", integrations.LocaleSimplifiedChinese: "添加回调 URI 和网站地址",
				},
				Description: "Add the exact callback URL shown by ZGI to the app allowlist and configure the required website URL.",
				DescriptionI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Add the exact callback URL shown by ZGI to the app allowlist and configure the required website URL.",
					integrations.LocaleSimplifiedChinese: "将 ZGI 展示的完整回调地址加入应用允许列表，并配置 X 要求的网站地址。",
				},
				Action: integrations.AuthSetupStepActionCopyCallbackURL,
			},
			{
				ID: "choose_permissions", Title: "Choose the app permissions",
				TitleI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Choose the app permissions", integrations.LocaleSimplifiedChinese: "选择应用权限",
				},
				Description: "Choose read permissions for profile and post access. Enable write permissions only when your organization will use the create-post action.",
				DescriptionI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Choose read permissions for profile and post access. Enable write permissions only when your organization will use the create-post action.",
					integrations.LocaleSimplifiedChinese: "读取资料和帖子时选择只读权限；只有组织需要发布帖子时才启用写入权限。",
				},
			},
			{
				ID: "save_in_zgi", Title: "Copy the OAuth client credentials",
				TitleI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Copy the OAuth client credentials", integrations.LocaleSimplifiedChinese: "返回 ZGI 保存 OAuth 客户端凭据",
				},
				Description: "Copy the Client ID from Keys and Tokens. Add the Client Secret only for a confidential client, then save below.",
				DescriptionI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Copy the Client ID from Keys and Tokens. Add the Client Secret only for a confidential client, then save below.",
					integrations.LocaleSimplifiedChinese: "从 Keys and Tokens 复制 Client ID；只有机密客户端才需要填写 Client Secret，然后在下方保存。",
				},
			},
		},
		Notices: []integrations.AuthSetupNoticeDefinition{
			{
				ID: "secret_optional", Level: integrations.AuthSetupNoticeLevelInfo,
				Text: "Client Secret is optional for public clients and required for confidential clients. ZGI supports both with PKCE.",
				TextI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Client Secret is optional for public clients and required for confidential clients. ZGI supports both with PKCE.",
					integrations.LocaleSimplifiedChinese: "公有客户端不需要 Client Secret，机密客户端需要；ZGI 均使用 PKCE 完成授权。",
				},
			},
			{
				ID: "offline_access", Level: integrations.AuthSetupNoticeLevelWarning,
				Text: "Keep offline.access enabled when the connection must refresh tokens without asking the user to authorize again.",
				TextI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Keep offline.access enabled when the connection must refresh tokens without asking the user to authorize again.",
					integrations.LocaleSimplifiedChinese: "如需在不重复授权的情况下刷新令牌，请保留 offline.access 权限。",
				},
			},
		},
	}
}

func Actions() []integrations.ActionDefinition {
	actions := []integrations.ActionDefinition{
		{
			ID: ActionGetAccount, ToolName: "get_x_account", Name: "Get X account",
			NameI18n:    integrations.LocalizedText{integrations.LocaleEnglishUS: "Get X account", integrations.LocaleSimplifiedChinese: "获取 X 账号"},
			Description: "Return the bounded profile for the authenticated X account.",
			DescriptionI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS:         "Return the bounded profile for the authenticated X account.",
				integrations.LocaleSimplifiedChinese: "返回已认证 X 账号的受限资料。",
			},
			InputSchema: strictObjectSchema(map[string]interface{}{}, nil),
			OutputSchema: strictObjectSchema(map[string]interface{}{
				"provider": map[string]interface{}{"const": IntegrationID}, "request_id": boundedStringSchema(128),
				"account": userOutputSchema(),
			}, []string{"provider", "request_id", "account"}),
			Effect: toolgovernance.EffectRead, RiskLevel: toolgovernance.RiskLevelLow,
			DataEgress: true, ExternalDestination: "api.x.com", SensitiveDataAllowed: false,
			Idempotent: true, RequiredScopes: []string{ScopeUsersRead, ScopePostsRead},
			ScopeLabelsI18n: integrations.LocalizedLabelMap{
				ScopeUsersRead: {integrations.LocaleEnglishUS: "Read account profile", integrations.LocaleSimplifiedChinese: "读取账号资料"},
				ScopePostsRead: {integrations.LocaleEnglishUS: "Read posts", integrations.LocaleSimplifiedChinese: "读取帖子"},
			},
			DefaultPolicy:    readPolicy(true),
			SupportedCallers: []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat, tools.ToolInvokeFromAgent},
		},
		{
			ID: ActionListOwnPosts, ToolName: "list_x_posts", Name: "List my X posts",
			NameI18n:    integrations.LocalizedText{integrations.LocaleEnglishUS: "List my X posts", integrations.LocaleSimplifiedChinese: "列出我的 X 帖子"},
			Description: "List bounded recent posts from the authenticated X account.",
			DescriptionI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS:         "List bounded recent posts from the authenticated X account.",
				integrations.LocaleSimplifiedChinese: "列出已认证 X 账号近期的受限帖子内容。",
			},
			InputSchema:  listInputSchema("Pagination token", "分页 Token"),
			OutputSchema: postsOutputSchema(),
			Effect:       toolgovernance.EffectRead, RiskLevel: toolgovernance.RiskLevelLow,
			DataEgress: true, ExternalDestination: "api.x.com", SensitiveDataAllowed: false,
			Idempotent: true, RequiredScopes: []string{ScopeUsersRead, ScopePostsRead},
			ScopeLabelsI18n: integrations.LocalizedLabelMap{
				ScopeUsersRead: {integrations.LocaleEnglishUS: "Read account profile", integrations.LocaleSimplifiedChinese: "读取账号资料"},
				ScopePostsRead: {integrations.LocaleEnglishUS: "Read posts", integrations.LocaleSimplifiedChinese: "读取帖子"},
			},
			DefaultPolicy:    readPolicy(true),
			SupportedCallers: []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat, tools.ToolInvokeFromAgent},
		},
		{
			ID: ActionSearchRecentPosts, ToolName: "search_recent_x_posts", Name: "Search recent X posts",
			NameI18n:    integrations.LocalizedText{integrations.LocaleEnglishUS: "Search recent X posts", integrations.LocaleSimplifiedChinese: "搜索近期 X 帖子"},
			Description: "Search recent X posts when the connected X developer plan supports recent search. Disabled by default.",
			DescriptionI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS:         "Search recent X posts when the connected X developer plan supports recent search. Disabled by default.",
				integrations.LocaleSimplifiedChinese: "在所连接的 X 开发者套餐支持近期搜索时搜索帖子；默认关闭。",
			},
			InputSchema: strictObjectSchema(map[string]interface{}{
				"query":       localizedSchema(map[string]interface{}{"type": "string", "minLength": 1, "maxLength": 512}, "Search query", "搜索条件"),
				"max_results": localizedSchema(map[string]interface{}{"type": "integer", "minimum": 10, "maximum": 100, "default": 10}, "Maximum results", "最大结果数"),
				"next_token":  localizedSchema(map[string]interface{}{"type": "string", "maxLength": 1024}, "Next page token", "下一页 Token"),
			}, []string{"query"}),
			OutputSchema: postsOutputSchema(),
			Effect:       toolgovernance.EffectRead, RiskLevel: toolgovernance.RiskLevelMedium,
			DataEgress: true, ExternalDestination: "api.x.com", SensitiveDataAllowed: false,
			Idempotent: true, RequiredScopes: []string{ScopeUsersRead, ScopePostsRead},
			ScopeLabelsI18n: integrations.LocalizedLabelMap{
				ScopeUsersRead: {integrations.LocaleEnglishUS: "Read account profile", integrations.LocaleSimplifiedChinese: "读取账号资料"},
				ScopePostsRead: {integrations.LocaleEnglishUS: "Read posts", integrations.LocaleSimplifiedChinese: "读取帖子"},
			},
			DefaultPolicy:    readPolicy(false),
			SupportedCallers: []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat, tools.ToolInvokeFromAgent},
		},
		{
			ID: ActionCreatePost, ToolName: "create_x_post", Name: "Create X post",
			NameI18n:    integrations.LocalizedText{integrations.LocaleEnglishUS: "Create X post", integrations.LocaleSimplifiedChinese: "发布 X 帖子"},
			Description: "Publish one text post from the authenticated X account. Every invocation requires explicit approval.",
			DescriptionI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS:         "Publish one text post from the authenticated X account. Every invocation requires explicit approval.",
				integrations.LocaleSimplifiedChinese: "使用已认证的 X 账号发布一条文本帖子，每次调用都必须明确确认。",
			},
			InputSchema: strictObjectSchema(map[string]interface{}{
				"text": localizedSchema(map[string]interface{}{"type": "string", "minLength": 1, "maxLength": 280}, "Post text", "帖子文本"),
			}, []string{"text"}),
			OutputSchema: strictObjectSchema(map[string]interface{}{
				"provider": map[string]interface{}{"const": IntegrationID}, "request_id": boundedStringSchema(128),
				"post": strictObjectSchema(map[string]interface{}{
					"id": boundedStringSchema(128), "text": boundedStringSchema(1000),
				}, []string{"id", "text"}),
			}, []string{"provider", "request_id", "post"}),
			Effect: toolgovernance.EffectExternalSend, RiskLevel: toolgovernance.RiskLevelHigh,
			DataEgress: true, ExternalDestination: "api.x.com", SensitiveDataAllowed: false,
			Idempotent: false, RequiredScopes: []string{ScopeUsersRead, ScopePostsRead, ScopePostsWrite},
			ScopeLabelsI18n: integrations.LocalizedLabelMap{
				ScopeUsersRead:  {integrations.LocaleEnglishUS: "Read account profile", integrations.LocaleSimplifiedChinese: "读取账号资料"},
				ScopePostsRead:  {integrations.LocaleEnglishUS: "Read posts", integrations.LocaleSimplifiedChinese: "读取帖子"},
				ScopePostsWrite: {integrations.LocaleEnglishUS: "Create posts", integrations.LocaleSimplifiedChinese: "发布帖子"},
			},
			DefaultPolicy: &integrations.DefaultActionPolicy{
				Enabled: false, ApprovalPolicy: toolgovernance.ApprovalPolicyAlwaysAsk, DataEgressAllowed: true,
			},
			SupportedCallers: []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat},
		},
	}
	actions = append(actions,
		integrations.ActionDefinition{
			ID: ActionGetUserByUsername, ToolName: "get_x_user_by_username", Name: "Get X user by username",
			NameI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS: "Get X user by username", integrations.LocaleSimplifiedChinese: "按用户名获取 X 用户",
			},
			Description: "Look up one public X user by username. A leading @ is accepted and normalized by the service.",
			DescriptionI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS:         "Look up one public X user by username. A leading @ is accepted and normalized by the service.",
				integrations.LocaleSimplifiedChinese: "按用户名查询一个公开 X 用户；允许输入开头的 @，服务端会自动规范化。",
			},
			InputSchema: strictObjectSchema(map[string]interface{}{
				"username": localizedSchema(map[string]interface{}{
					"type": "string", "minLength": 1, "maxLength": 16, "pattern": `^@?[A-Za-z0-9_]{1,15}$`,
				}, "X username", "X 用户名"),
			}, []string{"username"}),
			OutputSchema: strictObjectSchema(map[string]interface{}{
				"provider": map[string]interface{}{"const": IntegrationID}, "request_id": boundedStringSchema(128),
				"user": userOutputSchema(),
			}, []string{"provider", "request_id", "user"}),
			Effect: toolgovernance.EffectRead, RiskLevel: toolgovernance.RiskLevelLow,
			DataEgress: true, ExternalDestination: "api.x.com", SensitiveDataAllowed: false,
			Idempotent: true, RequiredScopes: []string{ScopeUsersRead, ScopePostsRead},
			ScopeLabelsI18n: integrations.LocalizedLabelMap{
				ScopeUsersRead: {integrations.LocaleEnglishUS: "Read account profile", integrations.LocaleSimplifiedChinese: "读取账号资料"},
				ScopePostsRead: {integrations.LocaleEnglishUS: "Read posts", integrations.LocaleSimplifiedChinese: "读取帖子"},
			},
			DefaultPolicy:    readPolicy(true),
			SupportedCallers: []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat, tools.ToolInvokeFromAgent},
		},
		integrations.ActionDefinition{
			ID: ActionListPostsByUser, ToolName: "list_x_posts_by_user", Name: "List X posts by user",
			NameI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS: "List X posts by user", integrations.LocaleSimplifiedChinese: "列出指定 X 用户的帖子",
			},
			Description: "List a bounded page of public posts for one X user ID.",
			DescriptionI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS:         "List a bounded page of public posts for one X user ID.",
				integrations.LocaleSimplifiedChinese: "按 X 用户 ID 分页列出受限数量的公开帖子。",
			},
			InputSchema: strictObjectSchema(map[string]interface{}{
				"user_id": localizedSchema(map[string]interface{}{
					"type": "string", "minLength": 1, "maxLength": 32, "pattern": `^[0-9]{1,32}$`,
				}, "X user ID", "X 用户 ID"),
				"max_results": localizedSchema(map[string]interface{}{
					"type": "integer", "minimum": 5, "maximum": 100, "default": 20,
				}, "Maximum results", "最大结果数"),
				"pagination_token": localizedSchema(map[string]interface{}{
					"type": "string", "minLength": 1, "maxLength": 1024, "pattern": `.*\S.*`,
				}, "Pagination token", "分页 Token"),
			}, []string{"user_id"}),
			OutputSchema: postsOutputSchema(),
			Effect:       toolgovernance.EffectRead, RiskLevel: toolgovernance.RiskLevelLow,
			DataEgress: true, ExternalDestination: "api.x.com", SensitiveDataAllowed: false,
			Idempotent: true, RequiredScopes: []string{ScopeUsersRead, ScopePostsRead},
			ScopeLabelsI18n: integrations.LocalizedLabelMap{
				ScopeUsersRead: {integrations.LocaleEnglishUS: "Read account profile", integrations.LocaleSimplifiedChinese: "读取账号资料"},
				ScopePostsRead: {integrations.LocaleEnglishUS: "Read posts", integrations.LocaleSimplifiedChinese: "读取帖子"},
			},
			DefaultPolicy:    readPolicy(true),
			SupportedCallers: []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat, tools.ToolInvokeFromAgent},
		},
	)
	userOAuthMethods := []string{AccountOAuthAuthMethodID, OrganizationOAuthAuthMethodID}
	for index := range actions {
		switch actions[index].ID {
		case ActionGetUserByUsername, ActionListPostsByUser, ActionSearchRecentPosts:
			actions[index].SupportedAuthMethodIDs = append(append([]string(nil), userOAuthMethods...), AppBearerAuthMethodID)
			if actions[index].ID == ActionListPostsByUser {
				actions[index].PreparationHints = []integrations.ActionPreparationHint{{
					ActionID: ActionGetUserByUsername, Relation: integrations.ActionPreparationResolveTarget,
					TargetArguments: []string{"user_id"}, ResultPaths: []string{"user.id"},
					Description: "Look up the public user by username when the numeric user ID is unknown, then use the confirmed returned ID.",
					DescriptionI18n: integrations.LocalizedText{
						integrations.LocaleEnglishUS:         "Look up the public user by username when the numeric user ID is unknown, then use the confirmed returned ID.",
						integrations.LocaleSimplifiedChinese: "当数字用户 ID 未知时，先按用户名查询公开用户，再使用已确认的返回 ID。",
					},
				}}
			}
		default:
			actions[index].SupportedAuthMethodIDs = append([]string(nil), userOAuthMethods...)
		}
	}
	return actions
}

func readPolicy(enabled bool) *integrations.DefaultActionPolicy {
	return &integrations.DefaultActionPolicy{
		Enabled: enabled, ApprovalPolicy: toolgovernance.ApprovalPolicyNeverAsk, DataEgressAllowed: true,
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

func listInputSchema(tokenLabel, chineseTokenLabel string) map[string]interface{} {
	return strictObjectSchema(map[string]interface{}{
		"max_results":      localizedSchema(map[string]interface{}{"type": "integer", "minimum": 5, "maximum": 100, "default": 20}, "Maximum results", "最大结果数"),
		"pagination_token": localizedSchema(map[string]interface{}{"type": "string", "maxLength": 1024}, tokenLabel, chineseTokenLabel),
	}, nil)
}

func userOutputSchema() map[string]interface{} {
	metrics := strictObjectSchema(map[string]interface{}{
		"followers_count": nonNegativeInteger(), "following_count": nonNegativeInteger(),
		"tweet_count": nonNegativeInteger(), "listed_count": nonNegativeInteger(),
	}, []string{"followers_count", "following_count", "tweet_count", "listed_count"})
	return strictObjectSchema(map[string]interface{}{
		"id": boundedStringSchema(128), "name": boundedStringSchema(255), "username": boundedStringSchema(128),
		"description": boundedStringSchema(1000), "created_at": boundedStringSchema(64),
		"profile_image_url": boundedStringSchema(2048), "url": boundedStringSchema(2048),
		"verified": map[string]interface{}{"type": "boolean"}, "public_metrics": metrics,
	}, []string{"id", "name", "username", "description", "created_at", "profile_image_url", "url", "verified", "public_metrics"})
}

func postsOutputSchema() map[string]interface{} {
	metrics := strictObjectSchema(map[string]interface{}{
		"retweet_count": nonNegativeInteger(), "reply_count": nonNegativeInteger(),
		"like_count": nonNegativeInteger(), "quote_count": nonNegativeInteger(),
		"bookmark_count": nonNegativeInteger(), "impression_count": nonNegativeInteger(),
	}, []string{"retweet_count", "reply_count", "like_count", "quote_count", "bookmark_count", "impression_count"})
	post := strictObjectSchema(map[string]interface{}{
		"id": boundedStringSchema(128), "text": boundedStringSchema(1000),
		"created_at": boundedStringSchema(64), "lang": boundedStringSchema(32),
		"conversation_id": boundedStringSchema(128), "possibly_sensitive": map[string]interface{}{"type": "boolean"},
		"public_metrics": metrics,
	}, []string{"id", "text", "created_at", "lang", "conversation_id", "possibly_sensitive", "public_metrics"})
	return strictObjectSchema(map[string]interface{}{
		"provider": map[string]interface{}{"const": IntegrationID}, "request_id": boundedStringSchema(128),
		"posts":      map[string]interface{}{"type": "array", "maxItems": 100, "items": post},
		"next_token": boundedStringSchema(1024), "result_count": nonNegativeInteger(),
	}, []string{"provider", "request_id", "posts", "next_token", "result_count"})
}

func nonNegativeInteger() map[string]interface{} {
	return map[string]interface{}{"type": "integer", "minimum": 0}
}
