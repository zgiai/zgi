package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddAgentMemorySlotNameID = "20260715110000_add_agent_memory_slot_name"

func init() {
	registerSchemaMigration(
		migrationAddAgentMemorySlotNameID,
		upAddAgentMemorySlotName,
		func(schema *mschema.Builder) error {
			return schema.Raw(`ALTER TABLE public.agent_memory_slots DROP COLUMN IF EXISTS name`)
		},
	)
}

func upAddAgentMemorySlotName(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.agent_memory_slots
			ADD COLUMN IF NOT EXISTS name character varying(80) NOT NULL DEFAULT ''
	`)
}
