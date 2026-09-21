package execution

import (
	"github.com/zgiai/zgi/api/internal/capabilities/serverops"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

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
	result = append(result, mailRiskPolicies()...)
	result = append(result, messagingRiskPolicies()...)
	return append(result, serverOpsSSHRiskPolicies()...)
}

// serverOpsSSHRiskPolicies classifies exactly the three read-only SSH Server
// Operations actions as RiskHigh, pinned to the complete stable leaf identity
// frozen by catalog.ServerOpsSSHDefinition.
//
// Read-only is the operation Effect, not a risk claim. An SSH read exposes
// sensitive remote host state (kernel and service inventory, unit state, and
// journal content that routinely contains secrets), so the platform does not
// present it as a trusted low-risk read. RiskHigh makes the DigitalHuman surface
// require a mandatory per-call approval for every use; an operation, tool,
// driver, auth method, or connector that does not match this exact identity
// stays unclassified and therefore fails closed.
func serverOpsSSHRiskPolicies() []TrustedOperationRiskPolicy {
	rows := []struct {
		operation, tool string
	}{
		{serverops.ActionSystemSnapshot, "serverops_ssh_system_snapshot"},
		{serverops.ActionServiceStatus, "serverops_ssh_service_status"},
		{serverops.ActionJournalTail, "serverops_ssh_journal_tail"},
	}
	result := make([]TrustedOperationRiskPolicy, 0, len(rows))
	for _, row := range rows {
		result = append(result, TrustedOperationRiskPolicy{
			ConnectorID:    core.Identifier(catalog.ServerOpsSSHConnectorID),
			ProviderID:     core.Identifier(catalog.ServerOpsSSHProviderID),
			DriverID:       core.Identifier(catalog.ServerOpsSSHDriverID),
			AuthMethodID:   core.Identifier(catalog.ServerOpsSSHAuthMethodID),
			OperationID:    core.Identifier(row.operation),
			ToolName:       row.tool,
			Classification: RiskHigh,
		})
	}
	return result
}
