package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationCreateAgentMemoriesID = "20260525190000_create_agent_memories"

func init() {
	registerSchemaMigration(
		migrationCreateAgentMemoriesID,
		upCreateAgentMemories,
		downCreateAgentMemories,
	)
}

func upCreateAgentMemories(schema *mschema.Builder) error {
	if err := schema.Create("agent_memory_slots", func(table *mschema.Blueprint) {
		table.ID()
		table.UUID("workspace_id").NotNull()
		table.UUID("agent_id").NotNull()
		table.String("key", 64).NotNull()
		table.Text("description").Default("").NotNull()
		table.Integer("max_chars").Default(1000).NotNull()
		table.Boolean("enabled").Default(true).NotNull()
		table.Integer("sort_order").Default(0).NotNull()
		table.UUID("created_by").Nullable()
		table.UUID("updated_by").Nullable()
		table.TimestampsTz()
		table.Unique("idx_agent_memory_slots_agent_key", "workspace_id", "agent_id", "key")
		table.Index("idx_agent_memory_slots_agent_sort", "workspace_id", "agent_id", "sort_order")
		table.Index("idx_agent_memory_slots_enabled", "enabled")
		table.Foreign("fk_agent_memory_slots_agent", []string{"agent_id"}, "agents", []string{"id"}).CascadeOnDelete()
	}); err != nil {
		return err
	}
	if err := schema.Create("agent_memory_values", func(table *mschema.Blueprint) {
		table.ID()
		table.UUID("workspace_id").NotNull()
		table.UUID("agent_id").NotNull()
		table.String("slot_key", 64).NotNull()
		table.String("user_scope", 32).NotNull()
		table.UUID("user_id").NotNull()
		table.Text("content").Default("").NotNull()
		table.TimestampsTz()
		table.Unique("idx_agent_memory_values_scope", "workspace_id", "agent_id", "slot_key", "user_scope", "user_id")
		table.Index("idx_agent_memory_values_agent_user", "workspace_id", "agent_id", "user_scope", "user_id")
		table.Index("idx_agent_memory_values_slot_key", "slot_key")
		table.Foreign("fk_agent_memory_values_agent", []string{"agent_id"}, "agents", []string{"id"}).CascadeOnDelete()
	}); err != nil {
		return err
	}
	return schema.Create("agent_memory_events", func(table *mschema.Blueprint) {
		table.ID()
		table.UUID("workspace_id").NotNull()
		table.UUID("agent_id").NotNull()
		table.String("slot_key", 64).Default("").NotNull()
		table.String("user_scope", 32).Default("").NotNull()
		table.UUID("user_id").Nullable()
		table.String("action", 32).NotNull()
		table.String("actor_type", 32).Default("system").NotNull()
		table.String("source", 32).Default("api").NotNull()
		table.UUID("source_conversation_id").Nullable()
		table.UUID("source_message_id").Nullable()
		table.JSONB("before_snapshot").Nullable()
		table.JSONB("after_snapshot").Nullable()
		table.TimestampTz("created_at").DefaultSQL("CURRENT_TIMESTAMP").NotNull()
		table.Index("idx_agent_memory_events_agent_created", "workspace_id", "agent_id", "created_at")
		table.Index("idx_agent_memory_events_slot_key", "slot_key")
		table.Index("idx_agent_memory_events_user", "user_scope", "user_id")
		table.Index("idx_agent_memory_events_action", "action")
		table.Index("idx_agent_memory_events_actor_type", "actor_type")
		table.Index("idx_agent_memory_events_source", "source")
		table.Foreign("fk_agent_memory_events_agent", []string{"agent_id"}, "agents", []string{"id"}).CascadeOnDelete()
	})
}

func downCreateAgentMemories(schema *mschema.Builder) error {
	if err := schema.DropIfExists("agent_memory_events"); err != nil {
		return err
	}
	if err := schema.DropIfExists("agent_memory_values"); err != nil {
		return err
	}
	return schema.DropIfExists("agent_memory_slots")
}
