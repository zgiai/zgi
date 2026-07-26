package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationReconcileAgentConversationDialogueCountID = "20260726100000_reconcile_agent_conversation_dialogue_count"

func init() {
	registerSchemaMigration(
		migrationReconcileAgentConversationDialogueCountID,
		upReconcileAgentConversationDialogueCount,
		nil,
	)
}

func upReconcileAgentConversationDialogueCount(schema *mschema.Builder) error {
	return schema.Raw(`
		WITH workflow_conversations AS (
			SELECT DISTINCT conversation_id
			FROM public.agents_messages
			WHERE workflow_run_id IS NOT NULL
				AND deleted_at IS NULL
		), canonical_counts AS (
			SELECT message.conversation_id, COUNT(*)::integer AS dialogue_count
			FROM public.agents_messages AS message
			JOIN workflow_conversations AS workflow_conversation
				ON workflow_conversation.conversation_id = message.conversation_id
			WHERE message.deleted_at IS NULL
			GROUP BY message.conversation_id
		)
		UPDATE public.agents_conversations AS conversation
		SET dialogue_count = canonical_counts.dialogue_count
		FROM canonical_counts
		WHERE conversation.id = canonical_counts.conversation_id
			AND conversation.deleted_at IS NULL
			AND conversation.dialogue_count IS DISTINCT FROM canonical_counts.dialogue_count;
	`)
}
