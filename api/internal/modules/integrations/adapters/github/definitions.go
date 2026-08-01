package github

import (
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

const (
	IntegrationID = "github"
	DriverID      = "github-rest"

	ActionGetAuthenticatedUser = "github.user.get"
	ActionListRepositories     = "github.repository.list"
	ActionSearchRepositories   = "github.repository.search"
	ActionListIssues           = "github.issue.list"
	ActionGetIssue             = "github.issue.get"
	ActionListIssueComments    = "github.issue.comment.list"
	ActionCreateIssue          = "github.issue.create"
	ActionCreateIssueComment   = "github.issue.comment.create"

	AccountPATAuthMethodID      = "personal_access_token"
	OrganizationPATAuthMethodID = "organization_personal_access_token"
)

func ProviderDefinition() integrations.ProviderDefinition {
	return integrations.ProviderDefinition{
		ID:          IntegrationID,
		DriverID:    DriverID,
		Name:        "GitHub",
		NameI18n:    integrations.LocalizedText{integrations.LocaleEnglishUS: "GitHub", integrations.LocaleSimplifiedChinese: "GitHub"},
		Description: "Search repositories and read or create issues and comments through the GitHub REST API.",
		DescriptionI18n: integrations.LocalizedText{
			integrations.LocaleEnglishUS:         "Search repositories and read or create issues and comments through the GitHub REST API.",
			integrations.LocaleSimplifiedChinese: "通过 GitHub REST API 搜索仓库，并读取或创建议题与评论。",
		},
		Author: "ZGI",
		Icon:   "github",
		Tags:   []string{"code", "repositories", "issues", "external"},
		TagLabelsI18n: integrations.LocalizedLabelMap{
			"code":         {integrations.LocaleEnglishUS: "Code", integrations.LocaleSimplifiedChinese: "代码"},
			"repositories": {integrations.LocaleEnglishUS: "Repositories", integrations.LocaleSimplifiedChinese: "代码仓库"},
			"issues":       {integrations.LocaleEnglishUS: "Issues", integrations.LocaleSimplifiedChinese: "议题"},
			"external":     {integrations.LocaleEnglishUS: "External", integrations.LocaleSimplifiedChinese: "外部服务"},
		},
		Categories: []string{"developer_tools"},
		CategoryLabelsI18n: integrations.LocalizedLabelMap{
			"developer_tools": {integrations.LocaleEnglishUS: "Developer tools", integrations.LocaleSimplifiedChinese: "开发工具"},
		},
		DocumentationURL: "https://docs.github.com/en/rest",
		DocumentationURLI18n: integrations.LocalizedText{
			integrations.LocaleEnglishUS:         "https://docs.github.com/en/rest",
			integrations.LocaleSimplifiedChinese: "https://docs.github.com/zh/rest",
		},
		AuthMethods: []integrations.AuthMethodDefinition{
			githubPATAuthMethod(AccountPATAuthMethodID, integrations.ConnectionCredentialSourceAccount, "Personal access token", "个人访问令牌"),
			githubPATAuthMethod(OrganizationPATAuthMethodID, integrations.ConnectionCredentialSourceOrganization, "Organization personal access token", "组织个人访问令牌"),
		},
		HealthProbe: integrations.HealthProbeDefinition{
			Supported:    true,
			MayIncurCost: false,
			Description:  "Reads the authenticated GitHub user without consuming a paid operation.",
			DescriptionI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS:         "Reads the authenticated GitHub user without consuming a paid operation.",
				integrations.LocaleSimplifiedChinese: "读取已认证的 GitHub 用户，不会产生付费操作。",
			},
		},
		Scopes:  githubProviderScopes(),
		Actions: Actions(),
	}
}

func githubProviderScopes() []integrations.ProviderScopeDefinition {
	scope := func(id, english, chinese string, access integrations.ProviderScopeAccess, broad bool) integrations.ProviderScopeDefinition {
		return integrations.ProviderScopeDefinition{
			ID: id, Label: english,
			LabelI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS: english, integrations.LocaleSimplifiedChinese: chinese,
			},
			Category: integrations.ProviderScopeCategoryProvider,
			Access:   access, Broad: broad,
		}
	}
	internal := func(id, english, chinese string) integrations.ProviderScopeDefinition {
		value := scope(id, english, chinese, integrations.ProviderScopeAccessRead, false)
		value.Category = integrations.ProviderScopeCategoryInternal
		return value
	}
	return []integrations.ProviderScopeDefinition{
		scope("repo", "Full repository access", "完整仓库访问", integrations.ProviderScopeAccessManage, true),
		scope("public_repo", "Public repository access", "公开仓库访问", integrations.ProviderScopeAccessWrite, true),
		scope("repo:status", "Repository commit status", "仓库提交状态", integrations.ProviderScopeAccessWrite, false),
		scope("repo_deployment", "Repository deployments", "仓库部署", integrations.ProviderScopeAccessWrite, true),
		scope("repo:invite", "Repository invitations", "仓库邀请", integrations.ProviderScopeAccessManage, true),
		scope("security_events", "Security events", "安全事件", integrations.ProviderScopeAccessManage, true),
		scope("workflow", "GitHub Actions workflows", "GitHub Actions 工作流", integrations.ProviderScopeAccessWrite, true),
		scope("write:packages", "Write packages", "写入软件包", integrations.ProviderScopeAccessWrite, true),
		scope("read:packages", "Read packages", "读取软件包", integrations.ProviderScopeAccessRead, false),
		scope("delete:packages", "Delete packages", "删除软件包", integrations.ProviderScopeAccessManage, true),
		scope("admin:org", "Organization administration", "组织管理", integrations.ProviderScopeAccessManage, true),
		scope("write:org", "Write organization data", "写入组织数据", integrations.ProviderScopeAccessWrite, true),
		scope("read:org", "Read organization data", "读取组织数据", integrations.ProviderScopeAccessRead, false),
		scope("admin:public_key", "Public key administration", "公钥管理", integrations.ProviderScopeAccessManage, true),
		scope("write:public_key", "Write public keys", "写入公钥", integrations.ProviderScopeAccessWrite, true),
		scope("read:public_key", "Read public keys", "读取公钥", integrations.ProviderScopeAccessRead, false),
		scope("admin:repo_hook", "Repository hook administration", "仓库 Webhook 管理", integrations.ProviderScopeAccessManage, true),
		scope("write:repo_hook", "Write repository hooks", "写入仓库 Webhook", integrations.ProviderScopeAccessWrite, true),
		scope("read:repo_hook", "Read repository hooks", "读取仓库 Webhook", integrations.ProviderScopeAccessRead, false),
		scope("gist", "Gists", "Gist 代码片段", integrations.ProviderScopeAccessWrite, true),
		scope("notifications", "Notifications", "通知", integrations.ProviderScopeAccessRead, false),
		scope("user", "User account access", "用户账号访问", integrations.ProviderScopeAccessWrite, true),
		scope("read:user", "Read user profile", "读取用户资料", integrations.ProviderScopeAccessRead, false),
		scope("user:email", "Read user email addresses", "读取用户邮箱地址", integrations.ProviderScopeAccessRead, false),
		scope("user:follow", "Follow and unfollow users", "关注或取消关注用户", integrations.ProviderScopeAccessWrite, true),
		scope("delete_repo", "Delete repositories", "删除仓库", integrations.ProviderScopeAccessManage, true),
		internal("metadata:read", "Read repository metadata", "读取仓库元数据"),
		internal("issues:read", "Read issues", "读取议题"),
		{
			ID: "issues:write", Label: "Write issues",
			LabelI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS: "Write issues", integrations.LocaleSimplifiedChinese: "写入议题",
			},
			Category: integrations.ProviderScopeCategoryInternal,
			Access:   integrations.ProviderScopeAccessWrite,
		},
	}
}

func githubPATAuthMethod(id string, source integrations.ConnectionCredentialSource, label, zhLabel string) integrations.AuthMethodDefinition {
	return integrations.AuthMethodDefinition{
		ID:                  id,
		Type:                integrations.AuthMethodTypeAPIKey,
		CredentialSource:    source,
		IdentityKind:        integrations.AuthIdentityKindUser,
		AcquisitionStrategy: integrations.AuthAcquisitionStrategyManualForm,
		LifecycleStrategy:   integrations.AuthLifecycleStrategyStatic,
		RequestAuthStrategy: integrations.RequestAuthStrategyBearerHeader,
		Label:               label,
		LabelI18n: integrations.LocalizedText{
			integrations.LocaleEnglishUS:         label,
			integrations.LocaleSimplifiedChinese: zhLabel,
		},
		Description: "GitHub recommends a fine-grained personal access token with only the permissions required by the selected actions.",
		DescriptionI18n: integrations.LocalizedText{
			integrations.LocaleEnglishUS:         "GitHub recommends a fine-grained personal access token with only the permissions required by the selected actions.",
			integrations.LocaleSimplifiedChinese: "GitHub 建议使用细粒度个人访问令牌，并只授予所选操作需要的权限。",
		},
		Available:  true,
		SetupGuide: githubPATSetupGuide(),
		Fields: []integrations.CredentialFieldDefinition{{
			Key:   "token",
			Label: "Personal access token",
			LabelI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS:         "Personal access token",
				integrations.LocaleSimplifiedChinese: "个人访问令牌",
			},
			Description: "Fine-grained PAT is recommended. The token is encrypted before storage and is never returned by the API.",
			DescriptionI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS:         "Fine-grained PAT is recommended. The token is encrypted before storage and is never returned by the API.",
				integrations.LocaleSimplifiedChinese: "建议使用细粒度 PAT。令牌会在保存前加密，API 永远不会返回令牌原文。",
			},
			Input:       integrations.CredentialFieldInputPassword,
			Required:    true,
			Secret:      true,
			Placeholder: "github_pat_…",
			PlaceholderI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS:         "github_pat_…",
				integrations.LocaleSimplifiedChinese: "请输入 github_pat_…",
			},
		}},
	}
}

func githubPATSetupGuide() *integrations.AuthSetupGuideDefinition {
	return &integrations.AuthSetupGuideDefinition{
		ConsoleURL:       "https://github.com/settings/personal-access-tokens/new",
		DocumentationURL: "https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens",
		Steps: []integrations.AuthSetupStepDefinition{
			{
				ID: "open_token_settings", Title: "Create a fine-grained personal access token",
				TitleI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Create a fine-grained personal access token",
					integrations.LocaleSimplifiedChinese: "创建细粒度个人访问令牌",
				},
				Description: "Open GitHub token settings and choose Generate new token. Fine-grained tokens are recommended over classic tokens.",
				DescriptionI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Open GitHub token settings and choose Generate new token. Fine-grained tokens are recommended over classic tokens.",
					integrations.LocaleSimplifiedChinese: "打开 GitHub 令牌设置并选择生成新令牌；优先使用细粒度令牌，不建议使用 classic 令牌。",
				},
				Action: integrations.AuthSetupStepActionOpenConsole,
			},
			{
				ID: "choose_owner", Title: "Choose the resource owner and repositories",
				TitleI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Choose the resource owner and repositories",
					integrations.LocaleSimplifiedChinese: "选择资源所有者和仓库范围",
				},
				Description: "Select your account or organization as the resource owner, then limit repository access to only the repositories ZGI should use.",
				DescriptionI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Select your account or organization as the resource owner, then limit repository access to only the repositories ZGI should use.",
					integrations.LocaleSimplifiedChinese: "选择个人账号或组织作为资源所有者，并将仓库访问范围限制为 ZGI 确实需要使用的仓库。",
				},
			},
			{
				ID: "choose_permissions", Title: "Grant the minimum permissions",
				TitleI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Grant the minimum permissions",
					integrations.LocaleSimplifiedChinese: "仅授予最小必要权限",
				},
				Description: "Grant Metadata read for repository discovery, Issues read for issue details and comments, and Issues write only if issue or comment creation will be enabled.",
				DescriptionI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Grant Metadata read for repository discovery, Issues read for issue details and comments, and Issues write only if issue or comment creation will be enabled.",
					integrations.LocaleSimplifiedChinese: "发现仓库授予 Metadata 读取权限；读取议题和评论授予 Issues 读取权限；仅在启用创建议题或评论时授予 Issues 写入权限。",
				},
				Action: integrations.AuthSetupStepActionOpenDocumentation,
			},
			{
				ID: "set_expiration", Title: "Set a short expiration",
				TitleI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Set a short expiration",
					integrations.LocaleSimplifiedChinese: "设置合理的较短有效期",
				},
				Description: "Choose the shortest practical lifetime. GitHub or an organization policy may require approval before organization resources become available.",
				DescriptionI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Choose the shortest practical lifetime. GitHub or an organization policy may require approval before organization resources become available.",
					integrations.LocaleSimplifiedChinese: "选择满足使用需求的最短有效期；访问组织资源前，GitHub 或组织策略可能要求管理员审批。",
				},
			},
			{
				ID: "paste_token", Title: "Generate and paste the token into ZGI",
				TitleI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Generate and paste the token into ZGI",
					integrations.LocaleSimplifiedChinese: "生成令牌并粘贴到 ZGI",
				},
				Description: "Copy the token immediately after generation and paste it below. GitHub will not show the complete token again.",
				DescriptionI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "Copy the token immediately after generation and paste it below. GitHub will not show the complete token again.",
					integrations.LocaleSimplifiedChinese: "令牌生成后立即复制并粘贴到下方；GitHub 不会再次展示完整令牌。",
				},
			},
		},
		Notices: []integrations.AuthSetupNoticeDefinition{
			{
				ID: "user_identity", Level: integrations.AuthSetupNoticeLevelWarning,
				Text: "A PAT acts with the connected GitHub user's identity and cannot exceed that user's own access.",
				TextI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "A PAT acts with the connected GitHub user's identity and cannot exceed that user's own access.",
					integrations.LocaleSimplifiedChinese: "PAT 代表所连接的 GitHub 用户执行操作，其权限不会超过该用户本身拥有的访问权限。",
				},
			},
			{
				ID: "secret_storage", Level: integrations.AuthSetupNoticeLevelInfo,
				Text: "ZGI encrypts the token before storage and never returns the original value after saving.",
				TextI18n: integrations.LocalizedText{
					integrations.LocaleEnglishUS:         "ZGI encrypts the token before storage and never returns the original value after saving.",
					integrations.LocaleSimplifiedChinese: "ZGI 会在保存前加密令牌，保存后不会返回令牌原文。",
				},
			},
		},
	}
}

func Actions() []integrations.ActionDefinition {
	actions := []integrations.ActionDefinition{
		{
			ID:       ActionGetAuthenticatedUser,
			ToolName: "get_github_user",
			Name:     "Get authenticated GitHub user",
			NameI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS:         "Get authenticated GitHub user",
				integrations.LocaleSimplifiedChinese: "获取已认证的 GitHub 用户",
			},
			Description: "Return the bounded public profile for the GitHub account represented by this connection.",
			DescriptionI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS:         "Return the bounded public profile for the GitHub account represented by this connection.",
				integrations.LocaleSimplifiedChinese: "返回此连接所代表 GitHub 账户的受限公开资料。",
			},
			InputSchema: strictObjectSchema(map[string]interface{}{}, nil),
			OutputSchema: strictObjectSchema(map[string]interface{}{
				"provider":   map[string]interface{}{"const": IntegrationID},
				"request_id": boundedStringSchema(128),
				"user": strictObjectSchema(map[string]interface{}{
					"login": boundedStringSchema(128), "name": boundedStringSchema(256),
					"html_url": boundedStringSchema(2048), "company": boundedStringSchema(256),
					"location": boundedStringSchema(256),
				}, []string{"login", "name", "html_url", "company", "location"}),
			}, []string{"provider", "request_id", "user"}),
			Effect: toolgovernance.EffectRead, RiskLevel: toolgovernance.RiskLevelLow,
			DataEgress: true, ExternalDestination: "api.github.com", SensitiveDataAllowed: false,
			Idempotent: true, DefaultPolicy: readOnlyDefaultPolicy(),
			SupportedCallers: []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat, tools.ToolInvokeFromAgent},
		},
		{
			ID:       ActionListRepositories,
			ToolName: "list_github_repositories",
			Name:     "List GitHub repositories",
			NameI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS:         "List GitHub repositories",
				integrations.LocaleSimplifiedChinese: "列出 GitHub 仓库",
			},
			Description: "List repositories the authenticated GitHub user can access, with bounded metadata only.",
			DescriptionI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS:         "List repositories the authenticated GitHub user can access, with bounded metadata only.",
				integrations.LocaleSimplifiedChinese: "列出已认证 GitHub 用户可访问的仓库，仅返回受限元数据。",
			},
			InputSchema: strictObjectSchema(map[string]interface{}{
				"visibility":  localizedInputSchema(enumStringSchema([]string{"all", "public", "private"}, "all"), "Visibility", "可见性", map[string]string{"all": "All", "public": "Public", "private": "Private"}, map[string]string{"all": "全部", "public": "公开", "private": "私有"}),
				"affiliation": localizedInputSchema(enumStringSchema([]string{"owner", "collaborator", "organization_member"}, "owner"), "Affiliation", "关联关系", map[string]string{"owner": "Owner", "collaborator": "Collaborator", "organization_member": "Organization member"}, map[string]string{"owner": "所有者", "collaborator": "协作者", "organization_member": "组织成员"}),
				"sort":        localizedInputSchema(enumStringSchema([]string{"created", "updated", "pushed", "full_name"}, "updated"), "Sort by", "排序字段", map[string]string{"created": "Created time", "updated": "Updated time", "pushed": "Last push", "full_name": "Full repository name"}, map[string]string{"created": "创建时间", "updated": "更新时间", "pushed": "最近推送时间", "full_name": "完整仓库名"}),
				"direction":   localizedInputSchema(enumStringSchema([]string{"asc", "desc"}, "desc"), "Sort direction", "排序方向", map[string]string{"asc": "Ascending", "desc": "Descending"}, map[string]string{"asc": "升序", "desc": "降序"}),
				"per_page":    localizedInputSchema(boundedIntegerSchema(1, 50, 20), "Results per page", "每页数量"),
				"page":        localizedInputSchema(boundedIntegerSchema(1, 1000, 1), "Page", "页码"),
			}, nil),
			OutputSchema: strictObjectSchema(map[string]interface{}{
				"provider": map[string]interface{}{"const": IntegrationID}, "request_id": boundedStringSchema(128),
				"page":         boundedIntegerSchema(1, 1000, 1),
				"repositories": map[string]interface{}{"type": "array", "maxItems": 50, "items": repositoryOutputSchema()},
			}, []string{"provider", "request_id", "page", "repositories"}),
			Effect: toolgovernance.EffectRead, RiskLevel: toolgovernance.RiskLevelLow,
			DataEgress: true, ExternalDestination: "api.github.com", SensitiveDataAllowed: false,
			Idempotent: true, RequiredScopes: []string{"metadata:read"},
			ScopeLabelsI18n: integrations.LocalizedLabelMap{
				"metadata:read": {integrations.LocaleEnglishUS: "Read repository metadata", integrations.LocaleSimplifiedChinese: "读取仓库元数据"},
			},
			DefaultPolicy:    readOnlyDefaultPolicy(),
			SupportedCallers: []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat, tools.ToolInvokeFromAgent},
		},
		{
			ID:       ActionListIssues,
			ToolName: "list_github_issues",
			Name:     "List GitHub repository issues",
			NameI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS:         "List GitHub repository issues",
				integrations.LocaleSimplifiedChinese: "列出 GitHub 仓库议题",
			},
			Description: "List bounded issue metadata for one repository. GitHub issue endpoints can also return pull requests.",
			DescriptionI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS:         "List bounded issue metadata for one repository. GitHub issue endpoints can also return pull requests.",
				integrations.LocaleSimplifiedChinese: "列出指定仓库的受限议题元数据；GitHub 议题接口也可能返回拉取请求。",
			},
			InputSchema: strictObjectSchema(map[string]interface{}{
				"owner":                 localizedInputSchema(identifierSchema("Repository owner."), "Repository owner", "仓库所有者"),
				"repo":                  localizedInputSchema(identifierSchema("Repository name without .git."), "Repository name", "仓库名称"),
				"state":                 localizedInputSchema(enumStringSchema([]string{"open", "closed", "all"}, "open"), "Issue state", "议题状态", map[string]string{"open": "Open", "closed": "Closed", "all": "All"}, map[string]string{"open": "开放", "closed": "已关闭", "all": "全部"}),
				"labels":                localizedInputSchema(map[string]interface{}{"type": "array", "maxItems": 10, "uniqueItems": true, "items": map[string]interface{}{"type": "string", "minLength": 1, "maxLength": 100}}, "Labels", "标签"),
				"sort":                  localizedInputSchema(enumStringSchema([]string{"created", "updated", "comments"}, "updated"), "Sort by", "排序字段", map[string]string{"created": "Created time", "updated": "Updated time", "comments": "Comment count"}, map[string]string{"created": "创建时间", "updated": "更新时间", "comments": "评论数"}),
				"direction":             localizedInputSchema(enumStringSchema([]string{"asc", "desc"}, "desc"), "Sort direction", "排序方向", map[string]string{"asc": "Ascending", "desc": "Descending"}, map[string]string{"asc": "升序", "desc": "降序"}),
				"since":                 localizedInputSchema(map[string]interface{}{"type": "string", "format": "date-time"}, "Updated since", "起始更新时间"),
				"include_pull_requests": localizedInputSchema(map[string]interface{}{"type": "boolean", "default": false}, "Include pull requests", "包含拉取请求"),
				"per_page":              localizedInputSchema(boundedIntegerSchema(1, 50, 20), "Results per page", "每页数量"),
				"page":                  localizedInputSchema(boundedIntegerSchema(1, 1000, 1), "Page", "页码"),
			}, []string{"owner", "repo"}),
			OutputSchema: strictObjectSchema(map[string]interface{}{
				"provider": map[string]interface{}{"const": IntegrationID}, "request_id": boundedStringSchema(128),
				"repository": boundedStringSchema(300), "page": boundedIntegerSchema(1, 1000, 1),
				"issues": map[string]interface{}{"type": "array", "maxItems": 50, "items": issueOutputSchema()},
			}, []string{"provider", "request_id", "repository", "page", "issues"}),
			Effect: toolgovernance.EffectRead, RiskLevel: toolgovernance.RiskLevelLow,
			DataEgress: true, ExternalDestination: "api.github.com", SensitiveDataAllowed: false,
			Idempotent: true, RequiredScopes: []string{"issues:read"},
			ScopeLabelsI18n: integrations.LocalizedLabelMap{
				"issues:read": {integrations.LocaleEnglishUS: "Read issues", integrations.LocaleSimplifiedChinese: "读取议题"},
			},
			DefaultPolicy:    readOnlyDefaultPolicy(),
			SupportedCallers: []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat, tools.ToolInvokeFromAgent},
		},
	}
	actions = append(actions,
		integrations.ActionDefinition{
			ID: ActionSearchRepositories, ToolName: "search_github_repositories",
			Name: "Search GitHub repositories",
			NameI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS: "Search GitHub repositories", integrations.LocaleSimplifiedChinese: "搜索 GitHub 仓库",
			},
			Description: "Search GitHub repositories with a bounded query and bounded repository metadata.",
			DescriptionI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS:         "Search GitHub repositories with a bounded query and bounded repository metadata.",
				integrations.LocaleSimplifiedChinese: "使用受限查询搜索 GitHub 仓库，并返回受限仓库元数据。",
			},
			InputSchema: strictObjectSchema(map[string]interface{}{
				"query": localizedInputSchema(nonBlankStringSchema(256), "Search query", "搜索条件"),
				"sort": localizedInputSchema(enumStringSchema([]string{"best_match", "stars", "forks", "help-wanted-issues", "updated"}, "best_match"), "Sort by", "排序字段",
					map[string]string{"best_match": "Best match", "stars": "Stars", "forks": "Forks", "help-wanted-issues": "Help wanted issues", "updated": "Updated time"},
					map[string]string{"best_match": "最佳匹配", "stars": "星标数", "forks": "复刻数", "help-wanted-issues": "待帮助议题数", "updated": "更新时间"}),
				"order": localizedInputSchema(enumStringSchema([]string{"asc", "desc"}, "desc"), "Sort direction", "排序方向",
					map[string]string{"asc": "Ascending", "desc": "Descending"}, map[string]string{"asc": "升序", "desc": "降序"}),
				"per_page": localizedInputSchema(boundedIntegerSchema(1, 50, 20), "Results per page", "每页数量"),
				"page":     localizedInputSchema(boundedIntegerSchema(1, 20, 1), "Page", "页码"),
			}, []string{"query"}),
			OutputSchema: strictObjectSchema(map[string]interface{}{
				"provider": map[string]interface{}{"const": IntegrationID}, "request_id": boundedStringSchema(128),
				"query": boundedStringSchema(256), "page": boundedIntegerSchema(1, 20, 1),
				"total_count":        map[string]interface{}{"type": "integer", "minimum": 0},
				"incomplete_results": map[string]interface{}{"type": "boolean"},
				"repositories":       map[string]interface{}{"type": "array", "maxItems": 50, "items": repositoryOutputSchema()},
			}, []string{"provider", "request_id", "query", "page", "total_count", "incomplete_results", "repositories"}),
			Effect: toolgovernance.EffectRead, RiskLevel: toolgovernance.RiskLevelLow,
			DataEgress: true, ExternalDestination: "api.github.com", SensitiveDataAllowed: false,
			Idempotent: true, RequiredScopes: []string{"metadata:read"},
			ScopeLabelsI18n:  githubScopeLabels("metadata:read", "Read repository metadata", "读取仓库元数据"),
			DefaultPolicy:    readOnlyDefaultPolicy(),
			SupportedCallers: []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat, tools.ToolInvokeFromAgent},
		},
		integrations.ActionDefinition{
			ID: ActionGetIssue, ToolName: "get_github_issue",
			Name: "Get GitHub issue",
			NameI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS: "Get GitHub issue", integrations.LocaleSimplifiedChinese: "获取 GitHub 议题",
			},
			Description: "Read one GitHub issue or pull request with a bounded body and metadata.",
			DescriptionI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS:         "Read one GitHub issue or pull request with a bounded body and metadata.",
				integrations.LocaleSimplifiedChinese: "读取一个 GitHub 议题或拉取请求，正文和元数据均受平台限制。",
			},
			InputSchema:  issueCoordinatesInputSchema(),
			OutputSchema: issueEnvelopeOutputSchema(),
			Effect:       toolgovernance.EffectRead, RiskLevel: toolgovernance.RiskLevelLow,
			DataEgress: true, ExternalDestination: "api.github.com", SensitiveDataAllowed: false,
			Idempotent: true, RequiredScopes: []string{"issues:read"},
			ScopeLabelsI18n:  githubScopeLabels("issues:read", "Read issues", "读取议题"),
			DefaultPolicy:    readOnlyDefaultPolicy(),
			SupportedCallers: []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat, tools.ToolInvokeFromAgent},
		},
		integrations.ActionDefinition{
			ID: ActionListIssueComments, ToolName: "list_github_issue_comments",
			Name: "List GitHub issue comments",
			NameI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS: "List GitHub issue comments", integrations.LocaleSimplifiedChinese: "列出 GitHub 议题评论",
			},
			Description: "List a bounded page of comments for one GitHub issue or pull request.",
			DescriptionI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS:         "List a bounded page of comments for one GitHub issue or pull request.",
				integrations.LocaleSimplifiedChinese: "分页列出一个 GitHub 议题或拉取请求的受限评论内容。",
			},
			InputSchema: issueCommentsListInputSchema(),
			OutputSchema: strictObjectSchema(map[string]interface{}{
				"provider": map[string]interface{}{"const": IntegrationID}, "request_id": boundedStringSchema(128),
				"repository": boundedStringSchema(300), "issue_number": positiveIntegerSchema(),
				"page":     boundedIntegerSchema(1, 1000, 1),
				"comments": map[string]interface{}{"type": "array", "maxItems": 50, "items": issueCommentOutputSchema()},
			}, []string{"provider", "request_id", "repository", "issue_number", "page", "comments"}),
			Effect: toolgovernance.EffectRead, RiskLevel: toolgovernance.RiskLevelLow,
			DataEgress: true, ExternalDestination: "api.github.com", SensitiveDataAllowed: false,
			Idempotent: true, RequiredScopes: []string{"issues:read"},
			ScopeLabelsI18n:  githubScopeLabels("issues:read", "Read issues", "读取议题"),
			DefaultPolicy:    readOnlyDefaultPolicy(),
			SupportedCallers: []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat, tools.ToolInvokeFromAgent},
		},
		integrations.ActionDefinition{
			ID: ActionCreateIssue, ToolName: "create_github_issue",
			Name: "Create GitHub issue",
			NameI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS: "Create GitHub issue", integrations.LocaleSimplifiedChinese: "创建 GitHub 议题",
			},
			Description: "Create an issue in one repository. This external write is disabled by default and always requires approval when enabled.",
			DescriptionI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS:         "Create an issue in one repository. This external write is disabled by default and always requires approval when enabled.",
				integrations.LocaleSimplifiedChinese: "在指定仓库创建议题。该外部写操作默认关闭，启用后每次执行都需要确认。",
			},
			InputSchema: createIssueInputSchema(), OutputSchema: issueEnvelopeOutputSchema(),
			Effect: toolgovernance.EffectCreate, RiskLevel: toolgovernance.RiskLevelHigh,
			DataEgress: true, ExternalDestination: "api.github.com", SensitiveDataAllowed: false,
			Idempotent: false, RequiredScopes: []string{"issues:write"},
			ScopeLabelsI18n: githubScopeLabels("issues:write", "Write issues", "写入议题"),
			DefaultPolicy:   writeDefaultPolicy(), SupportedCallers: []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat},
		},
		integrations.ActionDefinition{
			ID: ActionCreateIssueComment, ToolName: "create_github_issue_comment",
			Name: "Create GitHub issue comment",
			NameI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS: "Create GitHub issue comment", integrations.LocaleSimplifiedChinese: "创建 GitHub 议题评论",
			},
			Description: "Send a comment to one GitHub issue or pull request. This external send is disabled by default and always requires approval when enabled.",
			DescriptionI18n: integrations.LocalizedText{
				integrations.LocaleEnglishUS:         "Send a comment to one GitHub issue or pull request. This external send is disabled by default and always requires approval when enabled.",
				integrations.LocaleSimplifiedChinese: "向一个 GitHub 议题或拉取请求发送评论。该外发操作默认关闭，启用后每次执行都需要确认。",
			},
			InputSchema: createIssueCommentInputSchema(),
			OutputSchema: strictObjectSchema(map[string]interface{}{
				"provider": map[string]interface{}{"const": IntegrationID}, "request_id": boundedStringSchema(128),
				"repository": boundedStringSchema(300), "issue_number": positiveIntegerSchema(), "comment": issueCommentOutputSchema(),
			}, []string{"provider", "request_id", "repository", "issue_number", "comment"}),
			Effect: toolgovernance.EffectExternalSend, RiskLevel: toolgovernance.RiskLevelHigh,
			DataEgress: true, ExternalDestination: "api.github.com", SensitiveDataAllowed: false,
			Idempotent: false, RequiredScopes: []string{"issues:write"},
			ScopeLabelsI18n: githubScopeLabels("issues:write", "Write issues", "写入议题"),
			DefaultPolicy:   writeDefaultPolicy(), SupportedCallers: []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat},
		},
	)
	for index := range actions {
		actions[index].SupportedAuthMethodIDs = []string{
			AccountPATAuthMethodID,
			OrganizationPATAuthMethodID,
		}
		switch actions[index].ID {
		case ActionListIssues, ActionCreateIssue:
			actions[index].PreparationHints = githubRepositoryPreparationHints()
		case ActionGetIssue, ActionListIssueComments, ActionCreateIssueComment:
			actions[index].PreparationHints = append(
				githubRepositoryPreparationHints(),
				githubIssuePreparationHint(),
			)
		}
	}
	return actions
}

func githubRepositoryPreparationHints() []integrations.ActionPreparationHint {
	return []integrations.ActionPreparationHint{{
		ActionID: ActionSearchRepositories, Relation: integrations.ActionPreparationResolveTarget,
		TargetArguments: []string{"owner", "repo"}, ResultPaths: []string{"repositories[].full_name"},
		Description: "Search repositories when the owner or repository name is ambiguous, then split one confirmed full_name into the owner and repo arguments.",
		DescriptionI18n: integrations.LocalizedText{
			integrations.LocaleEnglishUS:         "Search repositories when the owner or repository name is ambiguous, then split one confirmed full_name into the owner and repo arguments.",
			integrations.LocaleSimplifiedChinese: "当仓库所有者或仓库名称不明确时，先搜索仓库，再将确认后的 full_name 拆分为 owner 和 repo 参数。",
		},
	}}
}

func githubIssuePreparationHint() integrations.ActionPreparationHint {
	return integrations.ActionPreparationHint{
		ActionID: ActionListIssues, Relation: integrations.ActionPreparationResolveTarget,
		TargetArguments: []string{"issue_number"}, ResultPaths: []string{"issues[].number"},
		Description: "List repository issues when the issue number is unknown, then use one confirmed issue number.",
		DescriptionI18n: integrations.LocalizedText{
			integrations.LocaleEnglishUS:         "List repository issues when the issue number is unknown, then use one confirmed issue number.",
			integrations.LocaleSimplifiedChinese: "当议题编号未知时，先列出仓库议题，再使用确认后的议题编号。",
		},
	}
}

func readOnlyDefaultPolicy() *integrations.DefaultActionPolicy {
	return &integrations.DefaultActionPolicy{
		Enabled: true, ApprovalPolicy: toolgovernance.ApprovalPolicyNeverAsk, DataEgressAllowed: true,
	}
}

func writeDefaultPolicy() *integrations.DefaultActionPolicy {
	return &integrations.DefaultActionPolicy{
		Enabled: false, ApprovalPolicy: toolgovernance.ApprovalPolicyAlwaysAsk, DataEgressAllowed: true,
	}
}

func githubScopeLabels(id, english, chinese string) integrations.LocalizedLabelMap {
	return integrations.LocalizedLabelMap{
		id: {integrations.LocaleEnglishUS: english, integrations.LocaleSimplifiedChinese: chinese},
	}
}

func issueCoordinatesInputSchema() map[string]interface{} {
	return strictObjectSchema(map[string]interface{}{
		"owner":        localizedInputSchema(identifierSchema("Repository owner."), "Repository owner", "仓库所有者"),
		"repo":         localizedInputSchema(identifierSchema("Repository name without .git."), "Repository name", "仓库名称"),
		"issue_number": localizedInputSchema(positiveIntegerSchema(), "Issue number", "议题编号"),
	}, []string{"owner", "repo", "issue_number"})
}

func issueCommentsListInputSchema() map[string]interface{} {
	return strictObjectSchema(map[string]interface{}{
		"owner":        localizedInputSchema(identifierSchema("Repository owner."), "Repository owner", "仓库所有者"),
		"repo":         localizedInputSchema(identifierSchema("Repository name without .git."), "Repository name", "仓库名称"),
		"issue_number": localizedInputSchema(positiveIntegerSchema(), "Issue number", "议题编号"),
		"since":        localizedInputSchema(map[string]interface{}{"type": "string", "format": "date-time"}, "Updated since", "起始更新时间"),
		"per_page":     localizedInputSchema(boundedIntegerSchema(1, 50, 20), "Results per page", "每页数量"),
		"page":         localizedInputSchema(boundedIntegerSchema(1, 1000, 1), "Page", "页码"),
	}, []string{"owner", "repo", "issue_number"})
}

func createIssueInputSchema() map[string]interface{} {
	return strictObjectSchema(map[string]interface{}{
		"owner":  localizedInputSchema(identifierSchema("Repository owner."), "Repository owner", "仓库所有者"),
		"repo":   localizedInputSchema(identifierSchema("Repository name without .git."), "Repository name", "仓库名称"),
		"title":  localizedInputSchema(nonBlankStringSchema(256), "Issue title", "议题标题"),
		"body":   localizedInputSchema(boundedStringSchema(20000), "Issue body", "议题正文"),
		"labels": localizedInputSchema(boundedNonBlankStringArraySchema(10, 100), "Labels", "标签"),
		"assignees": localizedInputSchema(map[string]interface{}{
			"type": "array", "maxItems": 10, "uniqueItems": true, "items": identifierSchema("GitHub login."),
		}, "Assignees", "经办人"),
		"milestone": localizedInputSchema(map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 1000000000}, "Milestone number", "里程碑编号"),
	}, []string{"owner", "repo", "title"})
}

func createIssueCommentInputSchema() map[string]interface{} {
	return strictObjectSchema(map[string]interface{}{
		"owner":        localizedInputSchema(identifierSchema("Repository owner."), "Repository owner", "仓库所有者"),
		"repo":         localizedInputSchema(identifierSchema("Repository name without .git."), "Repository name", "仓库名称"),
		"issue_number": localizedInputSchema(positiveIntegerSchema(), "Issue number", "议题编号"),
		"body":         localizedInputSchema(nonBlankStringSchema(20000), "Comment body", "评论内容"),
	}, []string{"owner", "repo", "issue_number", "body"})
}

func issueEnvelopeOutputSchema() map[string]interface{} {
	return strictObjectSchema(map[string]interface{}{
		"provider": map[string]interface{}{"const": IntegrationID}, "request_id": boundedStringSchema(128),
		"repository": boundedStringSchema(300), "issue": issueDetailOutputSchema(),
	}, []string{"provider", "request_id", "repository", "issue"})
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

func nonBlankStringSchema(maxLength int) map[string]interface{} {
	return map[string]interface{}{
		"type": "string", "minLength": 1, "maxLength": maxLength, "pattern": `.*\S.*`,
	}
}

func boundedNonBlankStringArraySchema(maxItems, maxLength int) map[string]interface{} {
	return map[string]interface{}{
		"type": "array", "maxItems": maxItems, "uniqueItems": true,
		"items": nonBlankStringSchema(maxLength),
	}
}

func positiveIntegerSchema() map[string]interface{} {
	return map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 1000000000}
}

func identifierSchema(description string) map[string]interface{} {
	return map[string]interface{}{
		"type": "string", "minLength": 1, "maxLength": 100, "pattern": `^[A-Za-z0-9_.-]+$`, "description": description,
	}
}

func enumStringSchema(values []string, defaultValue string) map[string]interface{} {
	return map[string]interface{}{"type": "string", "enum": values, "default": defaultValue}
}

func boundedIntegerSchema(minimum, maximum, defaultValue int) map[string]interface{} {
	return map[string]interface{}{"type": "integer", "minimum": minimum, "maximum": maximum, "default": defaultValue}
}

func localizedInputSchema(schema map[string]interface{}, english, simplifiedChinese string, enumLabels ...map[string]string) map[string]interface{} {
	schema["title_i18n"] = integrations.LocalizedText{
		integrations.LocaleEnglishUS:         english,
		integrations.LocaleSimplifiedChinese: simplifiedChinese,
	}
	if len(enumLabels) == 2 {
		schema["enum_labels_i18n"] = map[string]map[string]string{
			integrations.LocaleEnglishUS:         enumLabels[0],
			integrations.LocaleSimplifiedChinese: enumLabels[1],
		}
	}
	return schema
}

func repositoryOutputSchema() map[string]interface{} {
	return strictObjectSchema(map[string]interface{}{
		"full_name": boundedStringSchema(300), "html_url": boundedStringSchema(2048),
		"description": boundedStringSchema(1000), "visibility": boundedStringSchema(32),
		"private": map[string]interface{}{"type": "boolean"}, "archived": map[string]interface{}{"type": "boolean"},
		"default_branch": boundedStringSchema(255), "language": boundedStringSchema(100),
		"updated_at": boundedStringSchema(64), "pushed_at": boundedStringSchema(64),
		"open_issues_count": map[string]interface{}{"type": "integer", "minimum": 0},
	}, []string{"full_name", "html_url", "description", "visibility", "private", "archived", "default_branch", "language", "updated_at", "pushed_at", "open_issues_count"})
}

func issueOutputSchema() map[string]interface{} {
	return strictObjectSchema(map[string]interface{}{
		"number": map[string]interface{}{"type": "integer", "minimum": 1}, "title": boundedStringSchema(500),
		"state": boundedStringSchema(32), "kind": map[string]interface{}{"type": "string", "enum": []string{"issue", "pull_request"}},
		"html_url": boundedStringSchema(2048), "author": boundedStringSchema(128),
		"labels":     map[string]interface{}{"type": "array", "maxItems": 20, "items": boundedStringSchema(100)},
		"comments":   map[string]interface{}{"type": "integer", "minimum": 0},
		"created_at": boundedStringSchema(64), "updated_at": boundedStringSchema(64),
	}, []string{"number", "title", "state", "kind", "html_url", "author", "labels", "comments", "created_at", "updated_at"})
}

func issueDetailOutputSchema() map[string]interface{} {
	schema := issueOutputSchema()
	properties := schema["properties"].(map[string]interface{})
	properties["body"] = boundedStringSchema(20000)
	properties["locked"] = map[string]interface{}{"type": "boolean"}
	required := schema["required"].([]string)
	schema["required"] = append(required, "body", "locked")
	return schema
}

func issueCommentOutputSchema() map[string]interface{} {
	return strictObjectSchema(map[string]interface{}{
		"id": map[string]interface{}{"type": "integer", "minimum": 1}, "body": boundedStringSchema(20000),
		"html_url": boundedStringSchema(2048), "author": boundedStringSchema(128),
		"created_at": boundedStringSchema(64), "updated_at": boundedStringSchema(64),
	}, []string{"id", "body", "html_url", "author", "created_at", "updated_at"})
}
