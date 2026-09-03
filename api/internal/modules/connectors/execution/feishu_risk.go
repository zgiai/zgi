package execution

import "github.com/zgiai/zgi/api/internal/modules/connectors/core"

// Explicit platform policy. Manifest hints cannot promote a write or an
// unknown operation to unattended execution.
func feishuRiskPolicies() []TrustedOperationRiskPolicy {
	ops := []struct {
		id, tool string
		risk     RiskClassification
	}{
		{"feishu.resource.get", "feishu_resource_get", RiskTrustedLow},
		{"feishu.resource.upload", "feishu_resource_upload", RiskHigh},
		{"feishu.message.batch_send", "feishu_message_batch_send", RiskHigh},
		{"feishu.message.system_send", "feishu_message_system_send", RiskHigh},
		{"feishu.bot.get", "feishu_bot_get", RiskTrustedLow},
		{"feishu.chat.list", "feishu_chat_list", RiskTrustedLow},
		{"feishu.chat.get", "feishu_chat_get", RiskTrustedLow},
		{"feishu.message.list", "feishu_message_list", RiskTrustedLow},
		{"feishu.message.get", "feishu_message_get", RiskTrustedLow},
		{"feishu.document.read", "feishu_document_read", RiskTrustedLow},
		{"feishu.document.blocks.list", "feishu_document_blocks_list", RiskTrustedLow},
		{"feishu.file.metadata.get", "feishu_file_metadata_get", RiskTrustedLow},
		{"feishu.wiki.node.get", "feishu_wiki_node_get", RiskTrustedLow},
		{"feishu.comment.list", "feishu_comment_list", RiskTrustedLow},
		{"feishu.message.send", "feishu_message_send", RiskHigh},
		{"feishu.message.reply", "feishu_message_reply", RiskHigh},
		{"feishu.document.blocks.append", "feishu_document_blocks_append", RiskHigh},
		{"feishu.document.blocks.update", "feishu_document_blocks_update", RiskHigh},
		{"feishu.comment.create", "feishu_comment_create", RiskHigh},
		{"feishu.comment.reply", "feishu_comment_reply", RiskHigh},
		{"feishu.message.update", "feishu_message_update", RiskHigh},
		{"feishu.comment.update", "feishu_comment_update", RiskHigh},
		{"feishu.comment.delete", "feishu_comment_delete", RiskHigh},
		{"feishu.reaction.list", "feishu_reaction_list", RiskTrustedLow},
		{"feishu.reaction.add", "feishu_reaction_add", RiskHigh},
		{"feishu.reaction.remove", "feishu_reaction_remove", RiskHigh},
		{"feishu.pin.list", "feishu_pin_list", RiskTrustedLow},
		{"feishu.pin.add", "feishu_pin_add", RiskHigh},
		{"feishu.pin.remove", "feishu_pin_remove", RiskHigh},
		{"feishu.chat.create", "feishu_chat_create", RiskHigh},
		{"feishu.chat.update", "feishu_chat_update", RiskHigh},
		{"feishu.card.create", "feishu_card_create", RiskHigh},
		{"feishu.card.update", "feishu_card_update", RiskHigh},
		{"feishu.document.blocks.convert", "feishu_document_blocks_convert", RiskHigh},
		{"feishu.contact.basic.get", "feishu_contact_basic_get", RiskTrustedLow},
	}
	result := make([]TrustedOperationRiskPolicy, 0, len(ops))
	for _, op := range ops {
		result = append(result, TrustedOperationRiskPolicy{ConnectorID: "feishu", ProviderID: "feishu", DriverID: "feishu-openapi-v1", AuthMethodID: "feishu.app_credentials", OperationID: core.Identifier(op.id), ToolName: op.tool, Classification: op.risk})
	}
	return result
}
