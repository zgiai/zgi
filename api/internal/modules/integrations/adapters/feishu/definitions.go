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
	ScopeDriveRetrieve = "space:document:retrieve"
	ScopeDriveRead     = "drive:drive:readonly"
	ScopeDriveWrite    = "drive:drive"
	ScopeDocumentRead  = "docx:document:readonly"
	ScopeDocumentWrite = "docx:document"
	ScopeMessage       = "im:message"
	ScopeMessageRead   = "im:message:readonly"
	ScopeMessageLegacy = "im:message.history:readonly"
	ScopeSendAsUser    = "im:message.send_as_user"
	ScopeSendAsBot     = "im:message:send_as_bot"
	ScopeContactSearch = "contact:user:search"
	ScopeChatRead      = "im:chat:read"
	ScopeChatReadOnly  = "im:chat:readonly"
	ScopeChat          = "im:chat"
	ScopeCalendarRead  = "calendar:calendar:read"
	ScopeCalendarRO    = "calendar:calendar:readonly"
	ScopeCalendarWrite = "calendar:calendar"
	ScopeEventRead     = "calendar:calendar.event:read"
	ScopeEventCreate   = "calendar:calendar.event:create"

	ActionGetAccount      = "feishu.account.get"
	ActionListDriveFiles  = "feishu.drive.list"
	ActionReadDocument    = "feishu.document.read"
	ActionSearchContacts  = "feishu.contact.search"
	ActionListChats       = "feishu.chat.list"
	ActionListCalendars   = "feishu.calendar.list"
	ActionListMessages    = "feishu.message.list"
	ActionListEvents      = "feishu.calendar.event.list"
	ActionCreateEvent     = "feishu.calendar.event.create"
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
				ID: ScopeDriveRetrieve, Label: "Read Drive file metadata",
				LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Read Drive file metadata", integrations.LocaleSimplifiedChinese: "读取云盘文件元数据",
				},
				Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessRead,
			},
			{
				ID: ScopeDriveRead, Label: "Read Drive files",
				LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Read Drive files", integrations.LocaleSimplifiedChinese: "读取云盘文件",
				},
				Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessRead,
			},
			{
				ID: ScopeDriveWrite, Label: "Read and manage Drive files",
				LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Read and manage Drive files", integrations.LocaleSimplifiedChinese: "读取和管理云盘文件",
				},
				Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessWrite, Broad: true,
			},
			{
				ID: ScopeDocumentRead, Label: "Read documents",
				LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Read documents", integrations.LocaleSimplifiedChinese: "读取文档",
				},
				Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessRead,
			},
			{
				ID: ScopeDocumentWrite, Label: "Read and edit documents",
				LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Read and edit documents", integrations.LocaleSimplifiedChinese: "读取和编辑文档",
				},
				Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessWrite, Broad: true,
			},
			{
				ID: ScopeMessage, Label: "Read and send messages",
				LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Read and send messages", integrations.LocaleSimplifiedChinese: "读取与发送消息",
				},
				Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessWrite, Broad: true,
			},
			{
				ID: ScopeMessageRead, Label: "Read message history",
				LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Read message history", integrations.LocaleSimplifiedChinese: "读取消息历史",
				},
				Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessRead,
			},
			{
				ID: ScopeMessageLegacy, Label: "Read message history (legacy)",
				LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Read message history (legacy)", integrations.LocaleSimplifiedChinese: "读取消息历史（兼容权限）",
				},
				Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessRead, Broad: true,
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
			{
				ID: ScopeContactSearch, Label: "Search users",
				LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Search users", integrations.LocaleSimplifiedChinese: "搜索用户",
				},
				Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessRead,
			},
			{
				ID: ScopeChatRead, Label: "Read chat information",
				LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Read chat information", integrations.LocaleSimplifiedChinese: "读取群聊信息",
				},
				Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessRead,
			},
			{
				ID: ScopeChatReadOnly, Label: "Read chat information (legacy)",
				LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Read chat information (legacy)", integrations.LocaleSimplifiedChinese: "读取群聊信息（兼容权限）",
				},
				Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessRead,
			},
			{
				ID: ScopeChat, Label: "Read and update chats",
				LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Read and update chats", integrations.LocaleSimplifiedChinese: "读取与更新群聊",
				},
				Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessWrite, Broad: true,
			},
			{
				ID: ScopeCalendarRead, Label: "Read calendar information",
				LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Read calendar information", integrations.LocaleSimplifiedChinese: "读取日历信息",
				},
				Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessRead,
			},
			{
				ID: ScopeCalendarRO, Label: "Read calendars, events, and availability",
				LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Read calendars, events, and availability", integrations.LocaleSimplifiedChinese: "读取日历、日程与忙闲信息",
				},
				Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessRead, Broad: true,
			},
			{
				ID: ScopeCalendarWrite, Label: "Read and manage calendars and events",
				LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Read and manage calendars and events", integrations.LocaleSimplifiedChinese: "读取和管理日历及日程",
				},
				Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessWrite, Broad: true,
			},
			{
				ID: ScopeEventRead, Label: "Read calendar events",
				LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Read calendar events", integrations.LocaleSimplifiedChinese: "读取日程",
				},
				Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessRead,
			},
			{
				ID: ScopeEventCreate, Label: "Create calendar events",
				LabelI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS: "Create calendar events", integrations.LocaleSimplifiedChinese: "创建日程",
				},
				Category: integrations.ProviderScopeCategoryProvider, Access: integrations.ProviderScopeAccessWrite,
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
			{
				ID: "user_message_prerequisites", Level: integrations.AuthSetupNoticeLevelWarning,
				Text: "Sending as a user requires both im:message and im:message.send_as_user. The target must also be visible to the app.",
				TextI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Sending as a user requires both im:message and im:message.send_as_user. The target must also be visible to the app.",
					integrations.LocaleSimplifiedChinese: "以用户身份发消息必须同时开通 im:message 与 im:message.send_as_user，且目标用户或群聊必须在应用可访问范围内。",
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
			{
				ID: "bot_prerequisites", Level: integrations.AuthSetupNoticeLevelWarning,
				Text: "Bot messaging and application-identity calendar access require bot capability, a published app version, and access to the target user, chat, or calendar. A bot must already be in a group before it can send there.",
				TextI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Bot messaging and application-identity calendar access require bot capability, a published app version, and access to the target user, chat, or calendar. A bot must already be in a group before it can send there.",
					integrations.LocaleSimplifiedChinese: "机器人发消息和应用身份日历能力要求启用机器人、发布应用版本，并具备目标用户、群聊或日历的访问权；向群聊发送前机器人必须已在群内。",
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
				ScopeDriveRetrieve: {integrations.LocaleEnglishUS: "Read Drive file metadata", integrations.LocaleSimplifiedChinese: "读取云盘文件元数据"},
				ScopeDriveRead:     {integrations.LocaleEnglishUS: "Read Drive files", integrations.LocaleSimplifiedChinese: "读取云盘文件"},
				ScopeDriveWrite:    {integrations.LocaleEnglishUS: "Read and manage Drive files", integrations.LocaleSimplifiedChinese: "读取和管理云盘文件"},
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
				ScopeDocumentRead:  {integrations.LocaleEnglishUS: "Read documents", integrations.LocaleSimplifiedChinese: "读取文档"},
				ScopeDocumentWrite: {integrations.LocaleEnglishUS: "Read and edit documents", integrations.LocaleSimplifiedChinese: "读取和编辑文档"},
			},
		),
		readAction(
			ActionSearchContacts, "search_feishu_contacts", "Search Feishu users", "搜索飞书用户",
			"Search visible Feishu users by name and return bounded identifiers for a later message action.",
			"按姓名搜索当前账号可见的飞书用户，并返回可用于后续发消息的受限标识。",
			strictObjectSchema(map[string]interface{}{
				"query": localizedSchema(
					map[string]interface{}{"type": "string", "minLength": 1, "maxLength": 128},
					"Name keyword", "姓名关键词",
				),
				"page_size": localizedSchema(
					map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 50, "default": 20},
					"Results per page", "每页数量",
				),
				"page_token": localizedSchema(
					map[string]interface{}{"type": "string", "maxLength": 1024},
					"Next page token", "下一页 Token",
				),
			}, []string{"query"}),
			strictObjectSchema(map[string]interface{}{
				"provider": map[string]interface{}{"const": IntegrationID}, "request_id": boundedStringSchema(128),
				"users": map[string]interface{}{
					"type": "array", "maxItems": 50,
					"items": strictObjectSchema(map[string]interface{}{
						"open_id": boundedStringSchema(128), "user_id": boundedStringSchema(128),
						"name": boundedStringSchema(255),
						"department_ids": map[string]interface{}{
							"type": "array", "maxItems": 50, "items": boundedStringSchema(128),
						},
					}, []string{"open_id", "user_id", "name", "department_ids"}),
				},
				"next_page_token": boundedStringSchema(1024), "has_more": map[string]interface{}{"type": "boolean"},
			}, []string{"provider", "request_id", "users", "next_page_token", "has_more"}),
			[]string{ScopeContactSearch},
			integrations.LocalizedLabelMap{
				ScopeContactSearch: {integrations.LocaleEnglishUS: "Search users", integrations.LocaleSimplifiedChinese: "搜索用户"},
			},
		),
		readAction(
			ActionListChats, "list_feishu_chats", "List Feishu chats", "列出飞书群聊",
			"List chats visible to the connected user or application so a later message action can use a chat ID.",
			"列出当前用户或应用可见的群聊，以便后续发送消息时使用群聊 ID。",
			strictObjectSchema(map[string]interface{}{
				"page_size": localizedSchema(
					map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 50, "default": 20},
					"Results per page", "每页数量",
				),
				"page_token": localizedSchema(
					map[string]interface{}{"type": "string", "maxLength": 1024},
					"Next page token", "下一页 Token",
				),
			}, nil),
			strictObjectSchema(map[string]interface{}{
				"provider": map[string]interface{}{"const": IntegrationID}, "request_id": boundedStringSchema(128),
				"chats": map[string]interface{}{
					"type": "array", "maxItems": 50,
					"items": strictObjectSchema(map[string]interface{}{
						"chat_id": boundedStringSchema(255), "name": boundedStringSchema(500),
						"description": boundedStringSchema(2000), "owner_id": boundedStringSchema(128),
						"chat_mode": boundedStringSchema(32), "chat_type": boundedStringSchema(32),
						"member_count": map[string]interface{}{"type": "integer", "minimum": 0},
					}, []string{"chat_id", "name", "description", "owner_id", "chat_mode", "chat_type", "member_count"}),
				},
				"next_page_token": boundedStringSchema(1024), "has_more": map[string]interface{}{"type": "boolean"},
			}, []string{"provider", "request_id", "chats", "next_page_token", "has_more"}),
			[]string{ScopeChatRead},
			integrations.LocalizedLabelMap{
				ScopeChatRead:     {integrations.LocaleEnglishUS: "Read chat information", integrations.LocaleSimplifiedChinese: "读取群聊信息"},
				ScopeChatReadOnly: {integrations.LocaleEnglishUS: "Read chat information (legacy)", integrations.LocaleSimplifiedChinese: "读取群聊信息（兼容权限）"},
				ScopeChat:         {integrations.LocaleEnglishUS: "Read and update chats", integrations.LocaleSimplifiedChinese: "读取与更新群聊"},
			},
		),
		readAction(
			ActionListMessages, "list_feishu_messages", "List Feishu messages", "读取飞书消息",
			"Read a bounded page of recent messages from one visible Feishu chat.",
			"读取一个当前账号可见群聊中的一页近期消息，返回内容受数量和长度限制。",
			messageListInputSchema(),
			messageListOutputSchema(),
			[]string{ScopeMessageRead},
			integrations.LocalizedLabelMap{
				ScopeMessageRead:   {integrations.LocaleEnglishUS: "Read message history", integrations.LocaleSimplifiedChinese: "读取消息历史"},
				ScopeMessageLegacy: {integrations.LocaleEnglishUS: "Read message history (legacy)", integrations.LocaleSimplifiedChinese: "读取消息历史（兼容权限）"},
				ScopeMessage:       {integrations.LocaleEnglishUS: "Read and send messages", integrations.LocaleSimplifiedChinese: "读取与发送消息"},
			},
		),
		readAction(
			ActionListCalendars, "list_feishu_calendars", "List Feishu calendars", "列出飞书日历",
			"List calendars visible to the connected user or tenant application.",
			"列出当前用户或企业应用可见的日历。",
			calendarListInputSchema(),
			strictObjectSchema(map[string]interface{}{
				"provider": map[string]interface{}{"const": IntegrationID}, "request_id": boundedStringSchema(128),
				"calendars": map[string]interface{}{
					"type": "array", "maxItems": 50,
					"items": strictObjectSchema(map[string]interface{}{
						"calendar_id": boundedStringSchema(512), "summary": boundedStringSchema(255),
						"description": boundedStringSchema(1000), "permissions": boundedStringSchema(64),
						"type": boundedStringSchema(64), "role": boundedStringSchema(64),
						"is_deleted":     map[string]interface{}{"type": "boolean"},
						"is_third_party": map[string]interface{}{"type": "boolean"},
					}, []string{
						"calendar_id", "summary", "description", "permissions", "type", "role",
						"is_deleted", "is_third_party",
					}),
				},
				"next_page_token": boundedStringSchema(1024), "sync_token": boundedStringSchema(1024),
				"has_more": map[string]interface{}{"type": "boolean"},
			}, []string{"provider", "request_id", "calendars", "next_page_token", "sync_token", "has_more"}),
			[]string{ScopeCalendarRead},
			integrations.LocalizedLabelMap{
				ScopeCalendarRead:  {integrations.LocaleEnglishUS: "Read calendar information", integrations.LocaleSimplifiedChinese: "读取日历信息"},
				ScopeCalendarRO:    {integrations.LocaleEnglishUS: "Read calendars, events, and availability", integrations.LocaleSimplifiedChinese: "读取日历、日程与忙闲信息"},
				ScopeCalendarWrite: {integrations.LocaleEnglishUS: "Read and manage calendars and events", integrations.LocaleSimplifiedChinese: "读取和管理日历及日程"},
			},
		),
		readAction(
			ActionListEvents, "list_feishu_calendar_events", "List Feishu calendar events", "列出飞书日程",
			"List a bounded page of events in one calendar and an explicit time range.",
			"列出指定日历和明确时间范围内的一页日程，结果受数量和长度限制。",
			calendarEventListInputSchema(),
			calendarEventListOutputSchema(),
			[]string{ScopeEventRead},
			calendarEventReadScopeLabels(),
		),
		calendarEventCreateAction(),
		sendAction(
			ActionSendUserMessage, "send_feishu_user_message", "Send Feishu message as user", "以用户身份发送飞书消息",
			"Send one text message using delegated user authorization.", "使用用户委托授权发送一条文本消息。",
			[]string{ScopeMessage, ScopeSendAsUser},
			integrations.LocalizedLabelMap{
				ScopeMessage: {
					integrations.LocaleEnglishUS:         "Read and send messages",
					integrations.LocaleSimplifiedChinese: "读取与发送消息",
				},
				ScopeSendAsUser: {
					integrations.LocaleEnglishUS:         "Send messages as the user",
					integrations.LocaleSimplifiedChinese: "以用户身份发送消息",
				},
			},
			true,
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
				ScopeMessage: {
					integrations.LocaleEnglishUS:         "Read and send messages",
					integrations.LocaleSimplifiedChinese: "读取与发送消息",
				},
			},
			false,
		),
	}
	userOAuthMethods := []string{UserOAuthAuthMethodID, OrganizationOAuthAuthMethodID}
	userOrTenantAppMethods := []string{
		UserOAuthAuthMethodID, OrganizationOAuthAuthMethodID, TenantAppAuthMethodID,
	}
	for index := range actions {
		switch actions[index].ID {
		case ActionListDriveFiles:
			actions[index].RequiredScopes = nil
			actions[index].RequiredAnyScopes = []string{ScopeDriveRetrieve, ScopeDriveRead, ScopeDriveWrite}
			actions[index].PreferredScopes = []string{ScopeDriveRetrieve}
			actions[index].SupportedAuthMethodIDs = append([]string(nil), userOrTenantAppMethods...)
		case ActionReadDocument:
			actions[index].RequiredScopes = nil
			actions[index].RequiredAnyScopes = []string{ScopeDocumentRead, ScopeDocumentWrite}
			actions[index].PreferredScopes = []string{ScopeDocumentRead}
			actions[index].SupportedAuthMethodIDs = append([]string(nil), userOrTenantAppMethods...)
			actions[index].PreparationHints = []integrations.ActionPreparationHint{feishuDriveDocumentPreparationHint()}
		case ActionListChats:
			actions[index].RequiredScopes = nil
			actions[index].RequiredAnyScopes = []string{ScopeChatRead, ScopeChatReadOnly, ScopeChat}
			actions[index].PreferredScopes = []string{ScopeChatRead}
			actions[index].SupportedAuthMethodIDs = append([]string(nil), userOrTenantAppMethods...)
		case ActionListCalendars:
			actions[index].RequiredScopes = nil
			actions[index].RequiredAnyScopes = []string{ScopeCalendarRead, ScopeCalendarRO, ScopeCalendarWrite}
			actions[index].PreferredScopes = []string{ScopeCalendarRead}
			actions[index].SupportedAuthMethodIDs = append([]string(nil), userOrTenantAppMethods...)
		case ActionListMessages:
			actions[index].RequiredScopes = nil
			actions[index].RequiredAnyScopes = []string{ScopeMessageRead, ScopeMessageLegacy, ScopeMessage}
			actions[index].PreferredScopes = []string{ScopeMessageRead}
			actions[index].SupportedAuthMethodIDs = append([]string(nil), userOrTenantAppMethods...)
			actions[index].PreparationHints = []integrations.ActionPreparationHint{feishuChatPreparationHint("chat_id")}
		case ActionListEvents:
			actions[index].RequiredScopes = nil
			actions[index].RequiredAnyScopes = []string{ScopeEventRead, ScopeCalendarRO, ScopeCalendarWrite}
			actions[index].PreferredScopes = []string{ScopeEventRead}
			actions[index].SupportedAuthMethodIDs = append([]string(nil), userOrTenantAppMethods...)
			actions[index].PreparationHints = []integrations.ActionPreparationHint{feishuCalendarPreparationHint()}
		case ActionCreateEvent:
			actions[index].RequiredScopes = nil
			actions[index].RequiredAnyScopes = []string{ScopeEventCreate, ScopeCalendarWrite}
			actions[index].PreferredScopes = []string{ScopeEventCreate}
			actions[index].SupportedAuthMethodIDs = append([]string(nil), userOrTenantAppMethods...)
			actions[index].PreparationHints = []integrations.ActionPreparationHint{feishuCalendarPreparationHint()}
		case ActionSendBotMessage:
			actions[index].RequiredScopes = nil
			actions[index].RequiredAnyScopes = []string{ScopeSendAsBot, ScopeMessage}
			actions[index].PreferredScopes = []string{ScopeSendAsBot}
			actions[index].SupportedAuthMethodIDs = []string{TenantAppAuthMethodID}
			actions[index].PreparationHints = feishuMessagePreparationHints(false)
		case ActionGetAccount, ActionSearchContacts, ActionSendUserMessage:
			actions[index].SupportedAuthMethodIDs = append([]string(nil), userOAuthMethods...)
			if actions[index].ID == ActionSendUserMessage {
				actions[index].PreparationHints = feishuMessagePreparationHints(true)
			}
		}
	}
	return actions
}

func feishuMessagePreparationHints(includeContactSearch bool) []integrations.ActionPreparationHint {
	hints := []integrations.ActionPreparationHint{
		feishuChatPreparationHint("recipient_id"),
	}
	if includeContactSearch {
		hints = append(hints, integrations.ActionPreparationHint{
			ActionID: ActionSearchContacts, Relation: integrations.ActionPreparationResolveTarget,
			TargetArguments: []string{"recipient_id"}, ResultPaths: []string{"users[].open_id", "users[].user_id"},
			Description: "Search visible Feishu users when the user provides a person's name, then use one confirmed returned identifier as recipient_id with the matching recipient_type.",
			DescriptionI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS:         "Search visible Feishu users when the user provides a person's name, then use one confirmed returned identifier as recipient_id with the matching recipient_type.",
				integrations.LocaleSimplifiedChinese: "当用户提供人员姓名时，先搜索当前账号可见的飞书用户，再将确认后的返回标识作为 recipient_id，并使用匹配的 recipient_type。",
			},
		})
	}
	return hints
}

func feishuChatPreparationHint(targetArgument string) integrations.ActionPreparationHint {
	description := "List visible chats when the user names a group or conversation, then use one confirmed chat_id."
	chineseDescription := "当用户使用群聊或会话名称时，先列出可见群聊，再使用一个已确认的 chat_id。"
	if targetArgument == "recipient_id" {
		description = "List visible chats when the user names a group or conversation, then use the returned chat_id as recipient_id with recipient_type chat_id."
		chineseDescription = "当用户使用群聊或会话名称时，先列出可见群聊，再将返回的 chat_id 作为 recipient_id，并将 recipient_type 设为 chat_id。"
	}
	return integrations.ActionPreparationHint{
		ActionID: ActionListChats, Relation: integrations.ActionPreparationResolveTarget,
		TargetArguments: []string{targetArgument}, ResultPaths: []string{"chats[].chat_id"},
		Description: description,
		DescriptionI18n: integrations.LocalizedText{
			integrations.LocaleEnglishUS: description, integrations.LocaleSimplifiedChinese: chineseDescription,
		},
	}
}

func feishuCalendarPreparationHint() integrations.ActionPreparationHint {
	return integrations.ActionPreparationHint{
		ActionID: ActionListCalendars, Relation: integrations.ActionPreparationResolveTarget,
		TargetArguments: []string{"calendar_id"}, ResultPaths: []string{"calendars[].calendar_id"},
		Description: "List visible calendars when the calendar ID is unknown, then use one confirmed writable or readable calendar as required by the target action.",
		DescriptionI18n: integrations.LocalizedText{
			integrations.LocaleEnglishUS:         "List visible calendars when the calendar ID is unknown, then use one confirmed writable or readable calendar as required by the target action.",
			integrations.LocaleSimplifiedChinese: "当日历 ID 未知时，先列出可见日历，再根据目标操作选择一个已确认且具备相应读写权限的日历。",
		},
	}
}

func feishuDriveDocumentPreparationHint() integrations.ActionPreparationHint {
	return integrations.ActionPreparationHint{
		ActionID: ActionListDriveFiles, Relation: integrations.ActionPreparationResolveTarget,
		TargetArguments: []string{"document_id"}, ResultPaths: []string{"files[].token", "files[].type"},
		Description: "List Drive files when the document ID is unknown, confirm that the selected file type is a supported Feishu document, then use its token as document_id.",
		DescriptionI18n: integrations.LocalizedText{
			integrations.LocaleEnglishUS:         "List Drive files when the document ID is unknown, confirm that the selected file type is a supported Feishu document, then use its token as document_id.",
			integrations.LocaleSimplifiedChinese: "当文档 ID 未知时，先列出云盘文件，确认所选文件属于支持的飞书文档类型，再将其 token 作为 document_id。",
		},
	}
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
	allowSelf bool,
) integrations.ActionDefinition {
	return integrations.ActionDefinition{
		ID: id, ToolName: toolName, Name: name,
		NameI18n:        integrations.LocalizedText{integrations.LocaleEnglishUS: name, integrations.LocaleSimplifiedChinese: chineseName},
		Description:     description,
		DescriptionI18n: integrations.LocalizedText{integrations.LocaleEnglishUS: description, integrations.LocaleSimplifiedChinese: chineseDescription},
		InputSchema:     messageInputSchema(allowSelf),
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
		SuccessDeduplication: &integrations.SuccessDeduplicationDefinition{
			TargetArgumentPaths: []string{"recipient_id", "recipient_type"},
		},
		DefaultPolicy: &integrations.DefaultActionPolicy{
			Enabled: false, ApprovalPolicy: toolgovernance.ApprovalPolicyAutoByPermissionTier, DataEgressAllowed: true,
		},
		SupportedCallers: []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat},
	}
}

func calendarEventCreateAction() integrations.ActionDefinition {
	return integrations.ActionDefinition{
		ID: ActionCreateEvent, ToolName: "create_feishu_calendar_event",
		Name: "Create Feishu calendar event",
		NameI18n: integrations.LocalizedText{
			integrations.LocaleEnglishUS: "Create Feishu calendar event", integrations.LocaleSimplifiedChinese: "创建飞书日程",
		},
		Description: "Create one event in a writable Feishu calendar with a request-scoped idempotency marker.",
		DescriptionI18n: integrations.LocalizedText{
			integrations.LocaleEnglishUS:         "Create one event in a writable Feishu calendar with a request-scoped idempotency marker.",
			integrations.LocaleSimplifiedChinese: "在一个可写飞书日历中创建一条日程，并使用当前请求的幂等标识防止重复创建。",
		},
		InputSchema: calendarEventCreateInputSchema(), OutputSchema: calendarEventCreateOutputSchema(),
		Effect: toolgovernance.EffectCreate, RiskLevel: toolgovernance.RiskLevelHigh,
		DataEgress: true, ExternalDestination: "open.feishu.cn", SensitiveDataAllowed: false,
		Idempotent: false, RequiredScopes: []string{ScopeEventCreate}, ScopeLabelsI18n: calendarEventCreateScopeLabels(),
		SuccessDeduplication: &integrations.SuccessDeduplicationDefinition{
			TargetArgumentPaths: []string{"calendar_id"},
		},
		DefaultPolicy: &integrations.DefaultActionPolicy{
			Enabled: false, ApprovalPolicy: toolgovernance.ApprovalPolicyAlwaysAsk, DataEgressAllowed: true,
		},
		SupportedCallers: []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat},
	}
}

func calendarEventReadScopeLabels() integrations.LocalizedLabelMap {
	return integrations.LocalizedLabelMap{
		ScopeEventRead:     {integrations.LocaleEnglishUS: "Read calendar events", integrations.LocaleSimplifiedChinese: "读取日程"},
		ScopeCalendarRO:    {integrations.LocaleEnglishUS: "Read calendars, events, and availability", integrations.LocaleSimplifiedChinese: "读取日历、日程与忙闲信息"},
		ScopeCalendarWrite: {integrations.LocaleEnglishUS: "Read and manage calendars and events", integrations.LocaleSimplifiedChinese: "读取和管理日历及日程"},
	}
}

func calendarEventCreateScopeLabels() integrations.LocalizedLabelMap {
	return integrations.LocalizedLabelMap{
		ScopeEventCreate:   {integrations.LocaleEnglishUS: "Create calendar events", integrations.LocaleSimplifiedChinese: "创建日程"},
		ScopeCalendarWrite: {integrations.LocaleEnglishUS: "Read and manage calendars and events", integrations.LocaleSimplifiedChinese: "读取和管理日历及日程"},
	}
}

func messageListInputSchema() map[string]interface{} {
	schema := strictObjectSchema(map[string]interface{}{
		"chat_id": localizedSchema(
			map[string]interface{}{"type": "string", "minLength": 1, "maxLength": 255, "pattern": `^[A-Za-z0-9_-]+$`},
			"Chat ID", "群聊 ID",
		),
		"start_time": localizedSchema(
			map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 4102444800},
			"Start time (Unix seconds)", "开始时间（Unix 秒）",
		),
		"end_time": localizedSchema(
			map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 4102444800},
			"End time (Unix seconds)", "结束时间（Unix 秒）",
		),
		"sort_type": localizedEnumSchema(
			map[string]interface{}{"type": "string", "enum": []string{"newest_first", "oldest_first"}, "default": "newest_first"},
			"Sort order", "排序方式",
			map[string]string{"newest_first": "Newest first", "oldest_first": "Oldest first"},
			map[string]string{"newest_first": "最新优先", "oldest_first": "最早优先"},
		),
		"page_size": localizedSchema(
			map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 50, "default": 20},
			"Results per page", "每页数量",
		),
		"page_token": localizedSchema(
			map[string]interface{}{"type": "string", "maxLength": 1024}, "Next page token", "下一页 Token",
		),
	}, []string{"chat_id"})
	schema["allOf"] = []interface{}{
		map[string]interface{}{
			"if":   map[string]interface{}{"required": []string{"start_time"}},
			"then": map[string]interface{}{"required": []string{"end_time"}},
		},
		map[string]interface{}{
			"if":   map[string]interface{}{"required": []string{"end_time"}},
			"then": map[string]interface{}{"required": []string{"start_time"}},
		},
	}
	return schema
}

func messageListOutputSchema() map[string]interface{} {
	return strictObjectSchema(map[string]interface{}{
		"provider": map[string]interface{}{"const": IntegrationID}, "request_id": boundedStringSchema(128),
		"messages": map[string]interface{}{
			"type": "array", "maxItems": 50,
			"items": strictObjectSchema(map[string]interface{}{
				"message_id": boundedStringSchema(255), "root_id": boundedStringSchema(255),
				"parent_id": boundedStringSchema(255), "thread_id": boundedStringSchema(255),
				"chat_id": boundedStringSchema(255), "message_type": boundedStringSchema(64),
				"text": boundedStringSchema(4000), "sender_id": boundedStringSchema(128),
				"sender_type": boundedStringSchema(64), "create_time": boundedStringSchema(64),
				"update_time": boundedStringSchema(64), "deleted": map[string]interface{}{"type": "boolean"},
				"updated": map[string]interface{}{"type": "boolean"},
			}, []string{
				"message_id", "root_id", "parent_id", "thread_id", "chat_id", "message_type", "text",
				"sender_id", "sender_type", "create_time", "update_time", "deleted", "updated",
			}),
		},
		"next_page_token": boundedStringSchema(1024), "has_more": map[string]interface{}{"type": "boolean"},
	}, []string{"provider", "request_id", "messages", "next_page_token", "has_more"})
}

func calendarEventListInputSchema() map[string]interface{} {
	return strictObjectSchema(map[string]interface{}{
		"calendar_id": localizedSchema(
			map[string]interface{}{"type": "string", "minLength": 1, "maxLength": 512, "pattern": `^[A-Za-z0-9_.@-]+$`},
			"Calendar ID", "日历 ID",
		),
		"start_time": localizedSchema(
			map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 4102444800},
			"Start time (Unix seconds)", "开始时间（Unix 秒）",
		),
		"end_time": localizedSchema(
			map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 4102444800},
			"End time (Unix seconds, at most 40 days after start)", "结束时间（Unix 秒，最多晚于开始时间 40 天）",
		),
		"page_size": localizedSchema(
			map[string]interface{}{"type": "integer", "minimum": 50, "maximum": 50, "default": 50},
			"Results per page", "每页数量",
		),
		"page_token": localizedSchema(
			map[string]interface{}{"type": "string", "maxLength": 1024}, "Next page token", "下一页 Token",
		),
	}, []string{"calendar_id", "start_time", "end_time"})
}

func calendarEventCreateInputSchema() map[string]interface{} {
	return strictObjectSchema(map[string]interface{}{
		"calendar_id": localizedSchema(
			map[string]interface{}{"type": "string", "minLength": 1, "maxLength": 512, "pattern": `^[A-Za-z0-9_.@-]+$`},
			"Writable calendar ID", "可写日历 ID",
		),
		"summary": localizedSchema(
			map[string]interface{}{"type": "string", "minLength": 1, "maxLength": 1000}, "Event title", "日程标题",
		),
		"description": localizedSchema(
			map[string]interface{}{"type": "string", "maxLength": 40960}, "Event description", "日程描述",
		),
		"start_time": localizedSchema(
			map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 4102444800},
			"Start time (Unix seconds)", "开始时间（Unix 秒）",
		),
		"end_time": localizedSchema(
			map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 4102444800},
			"End time (Unix seconds)", "结束时间（Unix 秒）",
		),
		"timezone": localizedSchema(
			map[string]interface{}{"type": "string", "minLength": 1, "maxLength": 64, "pattern": `^[A-Za-z_]+(?:/[A-Za-z0-9_+.-]+)*$`, "default": "Asia/Shanghai"},
			"IANA time zone", "IANA 时区",
		),
		"visibility": localizedEnumSchema(
			map[string]interface{}{"type": "string", "enum": []string{"default", "public", "private"}, "default": "default"},
			"Visibility", "公开范围",
			map[string]string{"default": "Follow calendar", "public": "Public", "private": "Private"},
			map[string]string{"default": "跟随日历", "public": "公开", "private": "私密"},
		),
		"free_busy_status": localizedEnumSchema(
			map[string]interface{}{"type": "string", "enum": []string{"busy", "free"}, "default": "busy"},
			"Availability", "忙闲状态",
			map[string]string{"busy": "Busy", "free": "Free"}, map[string]string{"busy": "忙碌", "free": "空闲"},
		),
		"need_notification": localizedSchema(
			map[string]interface{}{"type": "boolean", "default": true}, "Notify attendees", "通知参与人",
		),
		"location_name": localizedSchema(
			map[string]interface{}{"type": "string", "maxLength": 512}, "Location name", "地点名称",
		),
		"location_address": localizedSchema(
			map[string]interface{}{"type": "string", "maxLength": 1000}, "Location address", "地点地址",
		),
	}, []string{"calendar_id", "summary", "start_time", "end_time"})
}

func calendarEventListOutputSchema() map[string]interface{} {
	return strictObjectSchema(map[string]interface{}{
		"provider": map[string]interface{}{"const": IntegrationID}, "request_id": boundedStringSchema(128),
		"events":          map[string]interface{}{"type": "array", "maxItems": 50, "items": calendarEventSchema()},
		"next_page_token": boundedStringSchema(1024), "has_more": map[string]interface{}{"type": "boolean"},
	}, []string{"provider", "request_id", "events", "next_page_token", "has_more"})
}

func calendarEventCreateOutputSchema() map[string]interface{} {
	return strictObjectSchema(map[string]interface{}{
		"provider": map[string]interface{}{"const": IntegrationID}, "request_id": boundedStringSchema(128),
		"event": calendarEventSchema(),
	}, []string{"provider", "request_id", "event"})
}

func calendarEventSchema() map[string]interface{} {
	timeInfo := strictObjectSchema(map[string]interface{}{
		"timestamp": boundedStringSchema(32), "date": boundedStringSchema(32), "timezone": boundedStringSchema(64),
	}, []string{"timestamp", "date", "timezone"})
	return strictObjectSchema(map[string]interface{}{
		"event_id": boundedStringSchema(255), "organizer_calendar_id": boundedStringSchema(512),
		"summary": boundedStringSchema(1000), "description": boundedStringSchema(4000),
		"start_time": timeInfo, "end_time": timeInfo, "status": boundedStringSchema(64),
		"visibility": boundedStringSchema(32), "free_busy_status": boundedStringSchema(32),
		"location_name": boundedStringSchema(512), "location_address": boundedStringSchema(1000),
		"app_link": boundedStringSchema(2048), "recurrence": boundedStringSchema(2000),
		"is_exception": map[string]interface{}{"type": "boolean"},
	}, []string{
		"event_id", "organizer_calendar_id", "summary", "description", "start_time", "end_time", "status",
		"visibility", "free_busy_status", "location_name", "location_address", "app_link", "recurrence", "is_exception",
	})
}

func messageInputSchema(allowSelf bool) map[string]interface{} {
	recipientTypes := []string{"open_id", "user_id", "union_id", "chat_id"}
	englishLabels := map[string]string{
		"open_id": "Open ID", "user_id": "User ID", "union_id": "Union ID", "chat_id": "Chat ID",
	}
	chineseLabels := map[string]string{
		"open_id": "Open ID", "user_id": "用户 ID", "union_id": "Union ID", "chat_id": "群聊 ID",
	}
	defaultType := "open_id"
	if allowSelf {
		recipientTypes = append([]string{"self"}, recipientTypes...)
		englishLabels["self"] = "Myself"
		chineseLabels["self"] = "我自己"
		defaultType = "self"
	}
	recipientIDEnglish := "Recipient ID"
	recipientIDChinese := "接收者 ID"
	if allowSelf {
		recipientIDEnglish = "Recipient ID (not required when sending to yourself)"
		recipientIDChinese = "接收者 ID（发送给自己时无需填写）"
	}
	schema := strictObjectSchema(map[string]interface{}{
		"recipient_type": localizedEnumSchema(
			map[string]interface{}{"type": "string", "enum": recipientTypes, "default": defaultType},
			"Recipient type", "接收者类型", englishLabels, chineseLabels,
		),
		"recipient_id": localizedSchema(
			map[string]interface{}{"type": "string", "minLength": 1, "maxLength": 255, "pattern": `^[A-Za-z0-9_-]+$`},
			recipientIDEnglish, recipientIDChinese,
		),
		"text": localizedSchema(
			map[string]interface{}{"type": "string", "minLength": 1, "maxLength": 10000},
			"Message text", "消息文本",
		),
	}, []string{"recipient_type", "text"})
	if !allowSelf {
		schema["required"] = []string{"recipient_type", "recipient_id", "text"}
	} else {
		schema["allOf"] = []interface{}{
			map[string]interface{}{
				"if": map[string]interface{}{
					"properties": map[string]interface{}{
						"recipient_type": localizedSchema(
							map[string]interface{}{"const": "self"},
							"Recipient type",
							"接收者类型",
						),
					},
					"required": []string{"recipient_type"},
				},
				"else": map[string]interface{}{"required": []string{"recipient_id"}},
			},
		}
	}
	return schema
}

func calendarListInputSchema() map[string]interface{} {
	schema := strictObjectSchema(map[string]interface{}{
		"page_token": localizedSchema(
			map[string]interface{}{"type": "string", "maxLength": 1024},
			"Next page token", "下一页 Token",
		),
		"sync_token": localizedSchema(
			map[string]interface{}{"type": "string", "maxLength": 1024},
			"Incremental synchronization token", "增量同步 Token",
		),
	}, nil)
	schema["allOf"] = []interface{}{
		map[string]interface{}{
			"not": map[string]interface{}{"required": []string{"page_token", "sync_token"}},
		},
	}
	return schema
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
