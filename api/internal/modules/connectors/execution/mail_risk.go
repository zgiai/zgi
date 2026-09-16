package execution

import (
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"strings"
)

func mailRiskPolicies() []TrustedOperationRiskPolicy {
	var result []TrustedOperationRiskPolicy
	for _, id := range []string{"gmail", "qqmail", "netease163"} {
		profile, _ := catalog.MailProfileFor(id)
		for _, op := range []string{"folder.list", "message.list", "message.get", "message.send"} {
			risk := RiskTrustedLow
			if op == "message.send" {
				risk = RiskHigh
			}
			tool := id + "_" + strings.ReplaceAll(op, ".", "_")
			result = append(result, TrustedOperationRiskPolicy{ConnectorID: core.Identifier(id), ProviderID: core.Identifier(id), DriverID: core.Identifier(profile.DriverID), AuthMethodID: core.Identifier(profile.AuthMethodID), OperationID: core.Identifier(id + "." + op), ToolName: tool, Classification: risk})
		}
	}
	return result
}
