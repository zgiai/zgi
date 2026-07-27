package feishu

import (
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

const (
	IntegrationID = "feishu"
	DriverID      = "feishu-open-api"

	RegionCN     = "cn"
	RegionGlobal = "global"

	UserOAuthAuthMethodID         = "feishu_user_oauth"
	OrganizationOAuthAuthMethodID = "organization_feishu_user_oauth"
	TenantAppAuthMethodID         = "feishu_tenant_app"

	ScopeOfflineAccess = "offline_access"
	ScopeDriveRead     = "drive:drive:readonly"
	ScopeDocumentRead  = "docx:document:readonly"
	ScopeSendAsUser    = "im:message.send_as_user"
	ScopeSendAsBot     = "im:message:send_as_bot"

	ActionGetAccount      = "feishu.account.get"
	ActionListDriveFiles  = "feishu.drive.list"
	ActionReadDocument    = "feishu.document.read"
	ActionSendUserMessage = "feishu.message.send_user"
	ActionSendBotMessage  = "feishu.message.send_bot"
)

func ProviderDefinition() integrations.ProviderDefinition {
	return integrations.ProviderDefinition{
		ID:       IntegrationID,
		DriverID: DriverID,
		Name:     "Feishu",
		NameI18n: integrations.LocalizedText{
			integrations.LocaleEnglishUS:         "Feishu",
			integrations.LocaleSimplifiedChinese: "飞书",
		},
		Description: "Connect Feishu user accounts and tenant apps for documents, Drive, and messaging.",
		DescriptionI18n: integrations.LocalizedText{
			integrations.LocaleEnglishUS:         "Connect Feishu user accounts and tenant apps for documents, Drive, and messaging.",
			integrations.LocaleSimplifiedChinese: "连接飞书用户账号和企业自建应用，以使用云文档、云盘和消息能力。",
		},
		Author: "ZGI",
		Icon:   "message-square",
		Tags:   []string{"communication", "documents", "collaboration", "external"},
		TagLabelsI18n: integrations.LocalizedLabelMap{
			"communication": {integrations.LocaleEnglishUS: "Communication", integrations.LocaleSimplifiedChinese: "沟通"},
			"documents":     {integrations.LocaleEnglishUS: "Documents", integrations.LocaleSimplifiedChinese: "文档"},
			"collaboration": {integrations.LocaleEnglishUS: "Collaboration", integrations.LocaleSimplifiedChinese: "协作"},
			"external":      {integrations.LocaleEnglishUS: "External", integrations.LocaleSimplifiedChinese: "外部服务"},
		},
		Categories: []string{"communication", "knowledge"},
		CategoryLabelsI18n: integrations.LocalizedLabelMap{
			"communication": {integrations.LocaleEnglishUS: "Communication", integrations.LocaleSimplifiedChinese: "沟通协作"},
			"knowledge":     {integrations.LocaleEnglishUS: "Knowledge", integrations.LocaleSimplifiedChinese: "知识管理"},
		},
		DocumentationURL: "https://open.feishu.cn/document/home/index",
		DocumentationURLI18n: integrations.LocalizedText{
			integrations.LocaleEnglishUS:         "https://open.feishu.cn/document/home/index",
			integrations.LocaleSimplifiedChinese: "https://open.feishu.cn/document/home/index",
		},
		AuthMethods: []integrations.AuthMethodDefinition{
			feishuOAuthMethod(UserOAuthAuthMethodID, integrations.ConnectionCredentialSourceAccount, "Connect my Feishu account", "连接我的飞书账号"),
			feishuOAuthMethod(OrganizationOAuthAuthMethodID, integrations.ConnectionCredentialSourceOrganization, "Connect an organization Feishu account", "连接组织飞书账号"),
			tenantAppAuthMethod(),
		},
		HealthProbe: integrations.HealthProbeDefinition{
			Supported:    true,
			MayIncurCost: false,
			Description:  "Reads the authenticated user or tenant identity without sending a message.",
			DescriptionI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS:         "Reads the authenticated user or tenant identity without sending a message.",
				integrations.LocaleSimplifiedChinese: "读取已认证的用户或企业身份，不会发送消息。",
			},
		},
		Scopes: []integrations.ProviderScopeDefinition{
			{
				ID: ScopeOfflineAccess, Label: "Keep the connection signed in",
				LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Keep the connection signed in", integrations.LocaleSimplifiedChinese: "保持连接登录",
				},
				Category: integrations.ProviderScopeCategoryLifecycle, Access: integrations.ProviderScopeAccessSession,
			},
			{
				ID: "auth:user.id:read", Label: "Read user identity",
				LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Read user identity", integrations.LocaleSimplifiedChinese: "读取用户身份",
				},
				Category: integrations.ProviderScopeCategoryIdentity, Access: integrations.ProviderScopeAccessIdentity,
			},
			{
				ID: "user_profile", Label: "Read user profile",
				LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Read user profile", integrations.LocaleSimplifiedChinese: "读取用户资料",
				},
				Category: integrations.ProviderScopeCategoryIdentity, Access: integrations.ProviderScopeAccessRead,
			},
			{
				ID: ScopeDriveRead, Label: "Read Drive files",
				LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Read Drive files", integrations.LocaleSimplifiedChinese: "读取云盘文件",
				},
				Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessRead,
			},
			{
				ID: ScopeDocumentRead, Label: "Read documents",
				LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Read documents", integrations.LocaleSimplifiedChinese: "读取文档",
				},
				Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessRead,
			},
			{
				ID: ScopeSendAsUser, Label: "Send messages as the user",
				LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Send messages as the user", integrations.LocaleSimplifiedChinese: "以用户身份发送消息",
				},
				Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessWrite, Broad: true,
			},
			{
				ID: ScopeSendAsBot, Label: "Send messages as the bot",
				LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Send messages as the bot", integrations.LocaleSimplifiedChinese: "以机器人身份发送消息",
				},
				Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessWrite, Broad: true,
			},
		},
		Actions: Actions(),
	}
}

func feishuOAuthMethod(id string, source integrations.ConnectionCredentialSource, label, chineseLabel string) integrations.AuthMethodDefinition {
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
			integrations.LocaleEnglishUS: label, integrations.LocaleSimplifiedChinese: chineseLabel,
		},
		Description: "Opens the Feishu authorization page and stores short-lived tokens through the encrypted connection vault.",
		DescriptionI18n: integrations.LocalizedText{
			integrations.LocaleEnglishUS:         "Opens the Feishu authorization page and stores short-lived tokens through the encrypted connection vault.",
			integrations.LocaleSimplifiedChinese: "打开飞书授权页面，短期令牌通过加密连接保险库保存。",
		},
		Available:  true,
		SetupGuide: feishuOAuthSetupGuide(),
		OAuth: &integrations.OAuthMethodMetadata{
			ConnectEnabled: true, ReconnectEnabled: true, ScopeUpgradeEnabled: true,
			ClientConfigID:   IntegrationID,
			ProviderSetupURL: "https://open.feishu.cn/app",
			// Feishu returns auth:user.id:read and user_profile as implicit
			// token scopes. They are not requestable application permissions
			// and must never be copied into the authorization URL.
			IdentityScopes:   []string{ScopeOfflineAccess},
			DefaultActionIDs: []string{ActionGetAccount},
			ClientFields: []integrations.CredentialFieldDefinition{
				{
					Key: "client_id", Label: "Feishu App ID",
					LabelI18n: integrations.LocalizedText{integrations.LocaleEnglishUS: "Feishu App ID", integrations.LocaleSimplifiedChinese: "飞书 App ID"},
					Input:     integrations.CredentialFieldInputText, Required: true, Secret: false,
				},
				{
					Key: "client_secret", Label: "Feishu App Secret",
					LabelI18n: integrations.LocalizedText{integrations.LocaleEnglishUS: "Feishu App Secret", integrations.LocaleSimplifiedChinese: "飞书 App Secret"},
					Input:     integrations.CredentialFieldInputPassword, Required: true, Secret: true,
				},
			},
		},
	}
}

func feishuOAuthSetupGuide() *integrations.AuthSetupGuideDefinition {
	return &integrations.AuthSetupGuideDefinition{
		ConsoleURL:       "https://open.feishu.cn/app",
		DocumentationURL: "https://open.feishu.cn/document/sso/web-application-end-user-consent/guide",
		Steps: []integrations.AuthSetupStepDefinition{
			{
				ID: "open_console", Title: "Create or select a Feishu app",
				TitleI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Create or select a Feishu app", integrations.LocaleSimplifiedChinese: "创建或选择飞书应用",
				},
				Description: "Open the Feishu developer console and create or select an organization-owned custom app.",
				DescriptionI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Open the Feishu developer console and create or select an organization-owned custom app.",
					integrations.LocaleSimplifiedChinese: "打开飞书开发者后台，创建或选择一个由组织持有的企业自建应用。",
				},
				Action: integrations.AuthSetupStepActionOpenConsole,
			},
			{
				ID: "copy_credentials", Title: "Find the App ID and App Secret",
				TitleI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Find the App ID and App Secret", integrations.LocaleSimplifiedChinese: "获取 App ID 和 App Secret",
				},
				Description: "Open Credentials & Basic Info and copy the App ID and App Secret. Keep the secret private.",
				DescriptionI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Open Credentials & Basic Info and copy the App ID and App Secret. Keep the secret private.",
					integrations.LocaleSimplifiedChinese: "进入“凭证与基础信息”，复制 App ID 和 App Secret，并妥善保管密钥。",
				},
			},
			{
				ID: "configure_callback", Title: "Add the authorization callback URL",
				TitleI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Add the authorization callback URL", integrations.LocaleSimplifiedChinese: "添加授权回调地址",
				},
				Description: "In Development Configuration > Security Settings, add the exact callback URL shown by ZGI.",
				DescriptionI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "In Development Configuration > Security Settings, add the exact callback URL shown by ZGI.",
					integrations.LocaleSimplifiedChinese: "进入“开发配置 → 安全设置”，添加 ZGI 展示的完整回调地址。",
				},
				Action: integrations.AuthSetupStepActionCopyCallbackURL,
			},
			{
				ID: "request_permissions", Title: "Request the required permissions",
				TitleI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Request the required permissions", integrations.LocaleSimplifiedChinese: "申请所需权限",
				},
				Description: "Enable only the permissions needed by the Feishu actions your organization plans to use.",
				DescriptionI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Enable only the permissions needed by the Feishu actions your organization plans to use.",
					integrations.LocaleSimplifiedChinese: "在权限管理中，只申请组织计划使用的飞书操作所需权限。",
				},
				Action: integrations.AuthSetupStepActionOpenDocumentation,
			},
			{
				ID: "publish_app", Title: "Publish the app changes",
				TitleI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Publish the app changes", integrations.LocaleSimplifiedChinese: "发布应用配置",
				},
				Description: "Create and publish an app version so redirect URLs and newly requested permissions take effect.",
				DescriptionI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Create and publish an app version so redirect URLs and newly requested permissions take effect.",
					integrations.LocaleSimplifiedChinese: "创建并发布应用版本，使回调地址和新增权限正式生效。",
				},
			},
			{
				ID: "save_in_zgi", Title: "Save the credentials in ZGI",
				TitleI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Save the credentials in ZGI", integrations.LocaleSimplifiedChinese: "返回 ZGI 保存凭据",
				},
				Description: "Paste the App ID and App Secret below, save the application, and then connect a Feishu account.",
				DescriptionI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Paste the App ID and App Secret below, save the application, and then connect a Feishu account.",
					integrations.LocaleSimplifiedChinese: "在下方粘贴 App ID 和 App Secret，保存应用后再连接飞书账号。",
				},
			},
		},
		Notices: []integrations.AuthSetupNoticeDefinition{
			{
				ID: "implicit_identity_scopes", Level: integrations.AuthSetupNoticeLevelWarning,
				Text: "Do not manually add user_profile or auth:user.id:read to the OAuth request. Feishu returns these identity scopes implicitly.",
				TextI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Do not manually add user_profile or auth:user.id:read to the OAuth request. Feishu returns these identity scopes implicitly.",
					integrations.LocaleSimplifiedChinese: "不要手动把 user_profile 或 auth:user.id:read 加入 OAuth 请求；飞书会隐式返回这些身份范围。",
				},
			},
			{
				ID: "publish_required", Level: integrations.AuthSetupNoticeLevelInfo,
				Text: "Permission and redirect changes may not work until the Feishu app version is published.",
				TextI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Permission and redirect changes may not work until the Feishu app version is published.",
					integrations.LocaleSimplifiedChinese: "权限或回调配置修改后，通常需要发布飞书应用版本才能生效。",
				},
			},
		},
	}
}

func tenantAppAuthMethod() integrations.AuthMethodDefinition {
	return integrations.AuthMethodDefinition{
		ID:                  TenantAppAuthMethodID,
		Type:                integrations.AuthMethodTypeServiceAccount,
		CredentialSource:    integrations.ConnectionCredentialSourceOrganization,
		IdentityKind:        integrations.AuthIdentityKindApplication,
		AcquisitionStrategy: integrations.AuthAcquisitionStrategyManualForm,
		LifecycleStrategy:   integrations.AuthLifecycleStrategyExchangeOnDemand,
		RequestAuthStrategy: integrations.RequestAuthStrategyBearerHeader,
		Label:               "Feishu tenant app",
		LabelI18n: integrations.LocalizedText{
			integrations.LocaleEnglishUS:         "Feishu tenant app",
			integrations.LocaleSimplifiedChinese: "飞书企业自建应用",
		},
		Description: "Uses an organization-owned app ID and app secret. It is separate from delegated user OAuth.",
		DescriptionI18n: integrations.LocalizedText{
			integrations.LocaleEnglishUS:         "Uses an organization-owned app ID and app secret. It is separate from delegated user OAuth.",
			integrations.LocaleSimplifiedChinese: "使用组织持有的 App ID 和 App Secret，与用户委托 OAuth 完全分离。",
		},
		Available:  true,
		SetupGuide: feishuTenantAppSetupGuide(),
		Fields: []integrations.CredentialFieldDefinition{
			{
				Key: "app_id", Label: "App ID",
				LabelI18n: integrations.LocalizedText{integrations.LocaleEnglishUS: "App ID", integrations.LocaleSimplifiedChinese: "App ID"},
				Input:     integrations.CredentialFieldInputText, Required: true, Secret: false,
				Placeholder:     "cli_…",
				PlaceholderI18n: integrations.LocalizedText{integrations.LocaleEnglishUS: "cli_…", integrations.LocaleSimplifiedChinese: "请输入 cli_…"},
			},
			{
				Key: "app_secret", Label: "App secret",
				LabelI18n: integrations.LocalizedText{integrations.LocaleEnglishUS: "App secret", integrations.LocaleSimplifiedChinese: "App Secret"},
				Input:     integrations.CredentialFieldInputPassword, Required: true, Secret: true,
				Placeholder:     "Enter the app secret",
				PlaceholderI18n: integrations.LocalizedText{integrations.LocaleEnglishUS: "Enter the app secret", integrations.LocaleSimplifiedChinese: "请输入 App Secret"},
			},
		},
	}
}

func feishuTenantAppSetupGuide() *integrations.AuthSetupGuideDefinition {
	return &integrations.AuthSetupGuideDefinition{
		ConsoleURL:       "https://open.feishu.cn/app",
		DocumentationURL: "https://open.feishu.cn/document/ukTMukTMukTMukTM/uMTNz4yM1MjLzUzM",
		Steps: []integrations.AuthSetupStepDefinition{
			{
				ID: "open_app", Title: "Create or select a custom Feishu app",
				TitleI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Create or select a custom Feishu app",
					integrations.LocaleSimplifiedChinese: "创建或选择飞书企业自建应用",
				},
				Description: "Open the Feishu developer console and choose an organization-owned custom app.",
				DescriptionI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Open the Feishu developer console and choose an organization-owned custom app.",
					integrations.LocaleSimplifiedChinese: "打开飞书开发者后台，选择一个由组织持有的企业自建应用。",
				},
				Action: integrations.AuthSetupStepActionOpenConsole,
			},
			{
				ID: "copy_credentials", Title: "Copy the App ID and App Secret",
				TitleI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Copy the App ID and App Secret",
					integrations.LocaleSimplifiedChinese: "复制 App ID 和 App Secret",
				},
				Description: "Open Basic Information > Credentials & Basic Info and copy the application credentials.",
				DescriptionI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Open Basic Information > Credentials & Basic Info and copy the application credentials.",
					integrations.LocaleSimplifiedChinese: "进入“基础信息 → 凭证与基础信息”，复制应用的 App ID 和 App Secret。",
				},
				Action: integrations.AuthSetupStepActionOpenDocumentation,
			},
			{
				ID: "configure_permissions", Title: "Enable only the required app permissions",
				TitleI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Enable only the required app permissions",
					integrations.LocaleSimplifiedChinese: "仅开通所需应用身份权限",
				},
				Description: "Enable the application-identity permissions required by the bot actions you plan to use, then publish the app changes when Feishu requires it.",
				DescriptionI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Enable the application-identity permissions required by the bot actions you plan to use, then publish the app changes when Feishu requires it.",
					integrations.LocaleSimplifiedChinese: "只开通计划使用的机器人操作所需应用身份权限，并在飞书要求时发布应用版本。",
				},
			},
			{
				ID: "paste_credentials", Title: "Paste the credentials into ZGI",
				TitleI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Paste the credentials into ZGI",
					integrations.LocaleSimplifiedChinese: "将应用凭据粘贴到 ZGI",
				},
				Description: "Paste the App ID and App Secret below. ZGI exchanges them for short-lived tenant tokens only at the execution boundary.",
				DescriptionI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Paste the App ID and App Secret below. ZGI exchanges them for short-lived tenant tokens only at the execution boundary.",
					integrations.LocaleSimplifiedChinese: "在下方粘贴 App ID 和 App Secret；ZGI 只在执行边界按需换取短期 tenant token。",
				},
			},
		},
		Notices: []integrations.AuthSetupNoticeDefinition{
			{
				ID: "application_identity", Level: integrations.AuthSetupNoticeLevelWarning,
				Text: "This method acts as the Feishu application, not as an individual Feishu user. Resource access is determined by app permissions and availability.",
				TextI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "This method acts as the Feishu application, not as an individual Feishu user. Resource access is determined by app permissions and availability.",
					integrations.LocaleSimplifiedChinese: "此方式代表飞书应用而非某个飞书用户；资源访问范围由应用权限和可用范围共同决定。",
				},
			},
			{
				ID: "secret_storage", Level: integrations.AuthSetupNoticeLevelInfo,
				Text: "ZGI encrypts the App Secret before storage and never returns the original value after saving.",
				TextI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "ZGI encrypts the App Secret before storage and never returns the original value after saving.",
					integrations.LocaleSimplifiedChinese: "ZGI 会在保存前加密 App Secret，保存后不会返回密钥原文。",
				},
			},
		},
	}
}

func Actions() []integrations.ActionDefinition {
	actions := []integrations.ActionDefinition{
		readAction(
			ActionGetAccount, "get_feishu_account", "Get Feishu account", "获取飞书账号",
			"Return the authenticated Feishu user profile.", "返回已认证的飞书用户资料。",
			strictObjectSchema(map[string]interface{}{}, nil),
			strictObjectSchema(map[string]interface{}{
				"provider": map[string]interface{}{"const": IntegrationID}, "request_id": boundedStringSchema(128),
				"account": strictObjectSchema(map[string]interface{}{
					"open_id": boundedStringSchema(128), "union_id": boundedStringSchema(128),
					"user_id": boundedStringSchema(128), "name": boundedStringSchema(255),
					"email": boundedStringSchema(320), "tenant_key": boundedStringSchema(128),
					"avatar_url": boundedStringSchema(2048),
				}, []string{"open_id", "union_id", "user_id", "name", "email", "tenant_key", "avatar_url"}),
			}, []string{"provider", "request_id", "account"}),
			nil,
			nil,
		),
		readAction(
			ActionListDriveFiles, "list_feishu_drive_files", "List Feishu Drive files", "列出飞书云盘文件",
			"List bounded file metadata in a Feishu Drive folder.", "列出飞书云盘文件夹中受限的文件元数据。",
			strictObjectSchema(map[string]interface{}{
				"folder_token": localizedSchema(map[string]interface{}{"type": "string", "maxLength": 255}, "Folder token", "文件夹 Token"),
				"page_size":    localizedSchema(map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 50, "default": 20}, "Results per page", "每页数量"),
				"page_token":   localizedSchema(map[string]interface{}{"type": "string", "maxLength": 1024}, "Next page token", "下一页 Token"),
			}, nil),
			strictObjectSchema(map[string]interface{}{
				"provider": map[string]interface{}{"const": IntegrationID}, "request_id": boundedStringSchema(128),
				"files":           map[string]interface{}{"type": "array", "maxItems": 50, "items": driveFileSchema()},
				"next_page_token": boundedStringSchema(1024), "has_more": map[string]interface{}{"type": "boolean"},
			}, []string{"provider", "request_id", "files", "next_page_token", "has_more"}),
			[]string{ScopeDriveRead},
			integrations.LocalizedLabelMap{
				ScopeDriveRead: {integrations.LocaleEnglishUS: "Read Drive files", integrations.LocaleSimplifiedChinese: "读取云盘文件"},
			},
		),
		readAction(
			ActionReadDocument, "read_feishu_document", "Read Feishu document", "读取飞书文档",
			"Read bounded raw text from one Feishu document.", "读取一篇飞书文档中受限长度的原始文本。",
			strictObjectSchema(map[string]interface{}{
				"document_id":    localizedSchema(map[string]interface{}{"type": "string", "minLength": 1, "maxLength": 255, "pattern": `^[A-Za-z0-9_-]+$`}, "Document ID", "文档 ID"),
				"max_characters": localizedSchema(map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 50000, "default": 20000}, "Maximum characters", "最大字符数"),
			}, []string{"document_id"}),
			strictObjectSchema(map[string]interface{}{
				"provider": map[string]interface{}{"const": IntegrationID}, "request_id": boundedStringSchema(128),
				"document_id": boundedStringSchema(255), "content": boundedStringSchema(50000),
				"truncated": map[string]interface{}{"type": "boolean"},
			}, []string{"provider", "request_id", "document_id", "content", "truncated"}),
			[]string{ScopeDocumentRead},
			integrations.LocalizedLabelMap{
				ScopeDocumentRead: {integrations.LocaleEnglishUS: "Read documents", integrations.LocaleSimplifiedChinese: "读取文档"},
			},
		),
		sendAction(
			ActionSendUserMessage, "send_feishu_user_message", "Send Feishu message as user", "以用户身份发送飞书消息",
			"Send one text message using delegated user authorization.", "使用用户委托授权发送一条文本消息。",
			[]string{ScopeSendAsUser},
			integrations.LocalizedLabelMap{
				ScopeSendAsUser: {
					integrations.LocaleEnglishUS:         "Send messages as the user",
					integrations.LocaleSimplifiedChinese: "以用户身份发送消息",
				},
			},
		),
		sendAction(
			ActionSendBotMessage, "send_feishu_bot_message", "Send Feishu bot message", "发送飞书机器人消息",
			"Send one text message using an organization tenant app.", "使用组织企业自建应用发送一条文本消息。",
			[]string{ScopeSendAsBot},
			integrations.LocalizedLabelMap{
				ScopeSendAsBot: {
					integrations.LocaleEnglishUS:         "Send messages as the app bot",
					integrations.LocaleSimplifiedChinese: "以应用机器人身份发送消息",
				},
			},
		),
	}
	userOAuthMethods := []string{UserOAuthAuthMethodID, OrganizationOAuthAuthMethodID}
	for index := range actions {
		switch actions[index].ID {
		case ActionGetAccount, ActionListDriveFiles, ActionReadDocument, ActionSendUserMessage:
			actions[index].SupportedAuthMethodIDs = append([]string(nil), userOAuthMethods...)
		case ActionSendBotMessage:
			actions[index].SupportedAuthMethodIDs = []string{TenantAppAuthMethodID}
		}
	}
	return actions
}

func readAction(id, toolName, name, chineseName, description, chineseDescription string, input, output map[string]interface{}, scopes []string, labels integrations.LocalizedLabelMap) integrations.ActionDefinition {
	return integrations.ActionDefinition{
		ID: id, ToolName: toolName, Name: name,
		NameI18n:        integrations.LocalizedText{integrations.LocaleEnglishUS: name, integrations.LocaleSimplifiedChinese: chineseName},
		Description:     description,
		DescriptionI18n: integrations.LocalizedText{integrations.LocaleEnglishUS: description, integrations.LocaleSimplifiedChinese: chineseDescription},
		InputSchema:     input, OutputSchema: output,
		Effect: toolgovernance.EffectRead, RiskLevel: toolgovernance.RiskLevelLow,
		DataEgress: true, ExternalDestination: "open.feishu.cn", SensitiveDataAllowed: false,
		Idempotent: true, RequiredScopes: scopes, ScopeLabelsI18n: labels,
		DefaultPolicy: &integrations.DefaultActionPolicy{
			Enabled: true, ApprovalPolicy: toolgovernance.ApprovalPolicyNeverAsk, DataEgressAllowed: true,
		},
		SupportedCallers: []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat, tools.ToolInvokeFromAgent},
	}
}

func sendAction(
	id, toolName, name, chineseName, description, chineseDescription string,
	scopes []string,
	scopeLabels integrations.LocalizedLabelMap,
) integrations.ActionDefinition {
	return integrations.ActionDefinition{
		ID: id, ToolName: toolName, Name: name,
		NameI18n:        integrations.LocalizedText{integrations.LocaleEnglishUS: name, integrations.LocaleSimplifiedChinese: chineseName},
		Description:     description,
		DescriptionI18n: integrations.LocalizedText{integrations.LocaleEnglishUS: description, integrations.LocaleSimplifiedChinese: chineseDescription},
		InputSchema: strictObjectSchema(map[string]interface{}{
			"receive_id": localizedSchema(map[string]interface{}{"type": "string", "minLength": 1, "maxLength": 255, "pattern": `^[A-Za-z0-9_-]+$`}, "Recipient ID", "接收者 ID"),
			"receive_id_type": localizedEnumSchema(
				map[string]interface{}{"type": "string", "enum": []string{"open_id", "union_id", "chat_id"}, "default": "open_id"},
				"Recipient ID type", "接收者 ID 类型",
				map[string]string{"open_id": "Open ID", "union_id": "Union ID", "chat_id": "Chat ID"},
				map[string]string{"open_id": "Open ID", "union_id": "Union ID", "chat_id": "群聊 ID"},
			),
			"text": localizedSchema(map[string]interface{}{"type": "string", "minLength": 1, "maxLength": 10000}, "Message text", "消息文本"),
		}, []string{"receive_id", "text"}),
		OutputSchema: strictObjectSchema(map[string]interface{}{
			"provider": map[string]interface{}{"const": IntegrationID}, "request_id": boundedStringSchema(128),
			"message": strictObjectSchema(map[string]interface{}{
				"message_id": boundedStringSchema(255), "root_id": boundedStringSchema(255),
				"parent_id": boundedStringSchema(255), "create_time": boundedStringSchema(64),
			}, []string{"message_id", "root_id", "parent_id", "create_time"}),
		}, []string{"provider", "request_id", "message"}),
		Effect: toolgovernance.EffectExternalSend, RiskLevel: toolgovernance.RiskLevelHigh,
		DataEgress: true, ExternalDestination: "open.feishu.cn", SensitiveDataAllowed: false,
		Idempotent: false, RequiredScopes: scopes,
		ScopeLabelsI18n: scopeLabels,
		DefaultPolicy: &integrations.DefaultActionPolicy{
			Enabled: false, ApprovalPolicy: toolgovernance.ApprovalPolicyAlwaysAsk, DataEgressAllowed: true,
		},
		SupportedCallers: []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat, tools.ToolInvokeFromAgent},
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

func localizedEnumSchema(schema map[string]interface{}, english, chinese string, englishLabels, chineseLabels map[string]string) map[string]interface{} {
	schema = localizedSchema(schema, english, chinese)
	schema["enum_labels_i18n"] = map[string]map[string]string{
		integrations.LocaleEnglishUS: englishLabels, integrations.LocaleSimplifiedChinese: chineseLabels,
	}
	return schema
}

func driveFileSchema() map[string]interface{} {
	return strictObjectSchema(map[string]interface{}{
		"token": boundedStringSchema(255), "name": boundedStringSchema(500),
		"type": boundedStringSchema(64), "parent_token": boundedStringSchema(255),
		"url": boundedStringSchema(2048), "created_time": boundedStringSchema(64),
		"modified_time": boundedStringSchema(64), "owner_id": boundedStringSchema(128),
	}, []string{"token", "name", "type", "parent_token", "url", "created_time", "modified_time", "owner_id"})
}
