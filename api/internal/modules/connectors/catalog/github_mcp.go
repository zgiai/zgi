package catalog

// GitHubMCPDefinition is a separate, read-only trial of GitHub's remote MCP
// service. It intentionally does not change the existing GitHub REST contract.
func GitHubMCPDefinition() Definition {
	text := func(limit int) map[string]any {
		return map[string]any{"type": "string", "minLength": 1, "maxLength": limit}
	}
	issueItem := map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{
		"number": map[string]any{"type": "integer", "minimum": 1}, "title": map[string]any{"type": "string", "maxLength": 500}, "html_url": map[string]any{"type": "string", "maxLength": 2048},
		"state": map[string]any{"type": "string", "maxLength": 32}, "updated_at": map[string]any{"type": "string", "maxLength": 64},
	}, "required": []any{"number", "title", "html_url", "state", "updated_at"}}
	output := func(properties map[string]any, required ...string) map[string]any {
		base := map[string]any{"provider": map[string]any{"const": "github-mcp"}, "transport": map[string]any{"const": "mcp"}, "repository": text(256)}
		for key, value := range properties {
			base[key] = value
		}
		requiredValues := []any{"provider", "transport", "repository"}
		for _, name := range required {
			requiredValues = append(requiredValues, name)
		}
		return map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "additionalProperties": false, "properties": base, "required": requiredValues}
	}
	listInput := map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "additionalProperties": false, "properties": map[string]any{
		"owner": text(100), "repo": text(100), "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 20, "default": 5},
	}, "required": []any{"owner", "repo"}}
	readInput := map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "additionalProperties": false, "properties": map[string]any{
		"owner": text(100), "repo": text(100), "issue_number": map[string]any{"type": "integer", "minimum": 1},
	}, "required": []any{"owner", "repo", "issue_number"}}
	operations := []Operation{
		{ID: "github-mcp.issue.list", ToolName: "list_github_mcp_issues", SkillID: "connector-runtime", Name: "List recent open GitHub issues", NameI18n: LocalizedText{"en-US": "List recent open GitHub issues", "zh-Hans": "列出最近更新的未关闭 GitHub Issue"}, Description: "List recent open issues by updated time using the official GitHub MCP server. Returns titles, issue numbers, and links.", DescriptionI18n: LocalizedText{"en-US": "List recent open issues by updated time using the official GitHub MCP server. Returns titles, issue numbers, and links.", "zh-Hans": "通过 GitHub 官方 MCP 按更新时间列出未关闭的 Issue，返回标题、编号和链接。"}, InputSchema: listInput, OutputSchema: output(map[string]any{"issues": map[string]any{"type": "array", "maxItems": 20, "items": issueItem}, "total_count": map[string]any{"type": "integer", "minimum": 0}, "has_more": map[string]any{"type": "boolean"}}, "issues", "total_count", "has_more"), Effect: OperationEffectRead, DataEgress: true, ExternalDestination: "api.githubcopilot.com", RiskHint: OperationRiskHintLow, Idempotent: true},
		{ID: "github-mcp.issue.get", ToolName: "read_github_mcp_issue", SkillID: "connector-runtime", Name: "Read a GitHub issue", NameI18n: LocalizedText{"en-US": "Read a GitHub issue", "zh-Hans": "读取 GitHub Issue 详情"}, Description: "Read one issue's details through the official GitHub MCP server. Only the get method is allowed.", DescriptionI18n: LocalizedText{"en-US": "Read one issue's details through the official GitHub MCP server. Only the get method is allowed.", "zh-Hans": "通过 GitHub 官方 MCP 读取一个 Issue 的详情，仅允许 get 方法。"}, InputSchema: readInput, OutputSchema: output(map[string]any{"issue": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{
			"number": map[string]any{"type": "integer", "minimum": 1}, "title": map[string]any{"type": "string", "maxLength": 500}, "body": map[string]any{"type": "string", "maxLength": 20000}, "html_url": map[string]any{"type": "string", "maxLength": 2048}, "state": map[string]any{"type": "string", "maxLength": 32}, "updated_at": map[string]any{"type": "string", "maxLength": 64}, "author": map[string]any{"type": "string", "maxLength": 128},
		}, "required": []any{"number", "title", "body", "html_url", "state", "updated_at", "author"}}}, "issue"), Effect: OperationEffectRead, DataEgress: true, ExternalDestination: "api.githubcopilot.com", RiskHint: OperationRiskHintLow, Idempotent: true},
	}
	return Definition{ID: "github-mcp", PluginID: "zgi.github-mcp", PluginVersion: "1.0.0", SourceDialect: SourceDialectAgentPluginsV1, SourceSchemaVersion: SourceSchemaVersion1,
		ProviderID: "github-mcp", DriverID: "github-mcp-remote", AuthMethodID: "personal_access_token", Name: "GitHub · MCP", NameI18n: LocalizedText{"en-US": "GitHub · MCP", "zh-Hans": "GitHub · MCP"},
		Description: "Read GitHub issues using GitHub's official remote MCP server.", DescriptionI18n: LocalizedText{"en-US": "Read GitHub issues using GitHub's official remote MCP server.", "zh-Hans": "通过 GitHub 官方远程 MCP 只读查询 Issue。"},
		Icon: "github", Classification: ClassificationConnector, CapabilityTypes: []CapabilityType{CapabilityTypeAction, CapabilityTypeData}, SetupMethod: SetupMethodKey, Ownership: OwnershipPersonal, Surfaces: FirstReleaseSurfaces(),
		KeyFields: []KeyField{{Name: "token", Label: "Personal access token", LabelI18n: LocalizedText{"en-US": "Personal access token", "zh-Hans": "个人访问令牌"}, Placeholder: "github_pat_…", PlaceholderI18n: LocalizedText{"en-US": "github_pat_…", "zh-Hans": "请输入 github_pat_…"}, HelpURL: "https://github.com/settings/personal-access-tokens/new", Required: true, Secret: true}}, Operations: operations}
}
