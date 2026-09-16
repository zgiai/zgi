package execution

import "github.com/zgiai/zgi/api/internal/modules/connectors/core"

func messagingRiskPolicies() []TrustedOperationRiskPolicy {
	rows := []struct {
		connector, operation, tool string
		risk                       RiskClassification
	}{
		{"dingtalk", "dingtalk.messages.send_direct", "dingtalk_messages_send_direct", RiskHigh},
		{"dingtalk", "dingtalk.messages.send_group", "dingtalk_messages_send_group", RiskHigh},
		{"wecom", "wecom.identity.get", "wecom_identity_get", RiskTrustedLow},
		{"wecom", "wecom.sessions.list", "wecom_sessions_list", RiskTrustedLow},
		{"wecom", "wecom.messages.send", "wecom_messages_send", RiskHigh},
		{"wecom", "wecom.documents.search", "wecom_documents_search", RiskTrustedLow},
		{"wecom", "wecom.documents.get", "wecom_documents_get", RiskTrustedLow},
		{"wecom", "wecom.calendar.list", "wecom_calendar_list", RiskTrustedLow},
		{"wecom", "wecom.calendar.get", "wecom_calendar_get", RiskTrustedLow},
		{"wecom", "wecom.calendar.search", "wecom_calendar_search", RiskTrustedLow},
		{"wecom", "wecom.todos.list", "wecom_todos_list", RiskTrustedLow},
		{"wecom", "wecom.todos.get", "wecom_todos_get", RiskTrustedLow},
		{"weixin", "weixin.messages.pull", "weixin_messages_pull", RiskTrustedLow},
		{"weixin", "weixin.messages.reply", "weixin_messages_reply", RiskHigh},
	}
	result := make([]TrustedOperationRiskPolicy, 0, len(rows))
	for _, r := range rows {
		result = append(result, TrustedOperationRiskPolicy{ConnectorID: core.Identifier(r.connector), ProviderID: core.Identifier(r.connector), DriverID: core.Identifier(r.connector + "-http-v1"), AuthMethodID: core.Identifier(r.connector + ".credentials"), OperationID: core.Identifier(r.operation), ToolName: r.tool, Classification: r.risk})
	}
	return result
}
