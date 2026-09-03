package execution

import "github.com/zgiai/zgi/api/internal/modules/connectors/core"

// BuiltinRiskPolicies is a platform policy, never inferred from plugin hints.
func BuiltinRiskPolicies() []TrustedOperationRiskPolicy {
	operations := []struct {
		connector, driver, auth, id, tool string
		risk                              RiskClassification
	}{
		{"github", "github-rest", "personal_access_token", "github.user.get", "get_github_user", RiskTrustedLow},
		{"github", "github-rest", "personal_access_token", "github.repository.list", "list_github_repositories", RiskTrustedLow},
		{"github", "github-rest", "personal_access_token", "github.repository.search", "search_github_repositories", RiskTrustedLow},
		{"github", "github-rest", "personal_access_token", "github.issue.list", "list_github_issues", RiskTrustedLow},
		{"github", "github-rest", "personal_access_token", "github.issue.get", "get_github_issue", RiskTrustedLow},
		{"github", "github-rest", "personal_access_token", "github.issue.comment.list", "list_github_issue_comments", RiskTrustedLow},
		{"github", "github-rest", "personal_access_token", "github.issue.create", "create_github_issue", RiskHigh},
		{"github", "github-rest", "personal_access_token", "github.issue.comment.create", "create_github_issue_comment", RiskHigh},
		{"web-search", "exa", "personal_api_key", "web.search", "search_web", RiskTrustedLow},
		{"web-search", "exa", "personal_api_key", "web.fetch", "fetch_webpage", RiskTrustedLow},
	}
	result := make([]TrustedOperationRiskPolicy, 0, len(operations))
	for _, op := range operations {
		result = append(result, TrustedOperationRiskPolicy{ConnectorID: core.Identifier(op.connector), ProviderID: core.Identifier(op.connector), DriverID: core.Identifier(op.driver), AuthMethodID: core.Identifier(op.auth), OperationID: core.Identifier(op.id), ToolName: op.tool, Classification: op.risk})
	}
	result = append(result, feishuRiskPolicies()...)
	return append(result, messagingRiskPolicies()...)
}
