package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

// Per-block write rules for Agent memory (ADR 0013): a read-only block is
// written only by builders and users, and a manual-only block takes no
// automatic writes. Both default off, which keeps existing behavior.
func init() {
	registerSchemaMigration("202610251100000000_agent_memory_slot_write_rules", upAgentMemorySlotWriteRules, downAgentMemorySlotWriteRules)
}

func upAgentMemorySlotWriteRules(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.agent_memory_slots ADD COLUMN IF NOT EXISTS read_only boolean NOT NULL DEFAULT false;
		ALTER TABLE public.agent_memory_slots ADD COLUMN IF NOT EXISTS manual_only boolean NOT NULL DEFAULT false;
	`)
}

func downAgentMemorySlotWriteRules(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.agent_memory_slots DROP COLUMN IF EXISTS manual_only;
		ALTER TABLE public.agent_memory_slots DROP COLUMN IF EXISTS read_only;
	`)
}
