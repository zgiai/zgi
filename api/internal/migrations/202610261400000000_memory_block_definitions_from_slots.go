package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

// Memory block definitions become the draft source of an Agent's blocks (ADR
// 0016). Until now agent_memory_slots was authoritative and definitions
// mirrored it, but a definition first created disabled or without automatic
// writes was stored with the column defaults instead. This copies the slots
// onto their definitions once, and creates any definition that is missing.
func init() {
	registerSchemaMigration("202610261400000000_memory_block_definitions_from_slots", upMemoryBlockDefinitionsFromSlots, downMemoryBlockDefinitionsFromSlots)
}

func upMemoryBlockDefinitionsFromSlots(schema *mschema.Builder) error {
	return schema.DataFix("copy agent memory slots onto memory block definitions", func(db *gorm.DB) error {
		if err := db.Exec(`
			UPDATE public.memory_block_definitions AS definitions
			SET name = slots.name, description = slots.description, limit_chars = GREATEST(slots.max_chars, 1),
				enabled = slots.enabled, sort_order = slots.sort_order, read_only = slots.read_only,
				auto_write = NOT slots.manual_only, deleted_at = NULL, updated_at = now()
			FROM public.agent_memory_slots AS slots
			WHERE definitions.consumer_type = 'agent'
				AND definitions.consumer_id = slots.agent_id
				AND definitions.label = slots.key
				AND (definitions.name <> slots.name OR definitions.description <> slots.description
					OR definitions.limit_chars <> GREATEST(slots.max_chars, 1) OR definitions.enabled <> slots.enabled
					OR definitions.sort_order <> slots.sort_order OR definitions.read_only <> slots.read_only
					OR definitions.auto_write <> NOT slots.manual_only OR definitions.deleted_at IS NOT NULL)`).Error; err != nil {
			return err
		}
		return db.Exec(`
			INSERT INTO public.memory_block_definitions
				(id, consumer_type, consumer_id, label, name, description, limit_chars, partition, owner,
				 read_only, auto_write, enabled, sort_order, created_at, updated_at)
			SELECT gen_random_uuid(), 'agent', slots.agent_id, slots.key, slots.name, slots.description,
				GREATEST(slots.max_chars, 1), 'person', 'user', slots.read_only, NOT slots.manual_only,
				slots.enabled, slots.sort_order, slots.created_at, slots.updated_at
			FROM public.agent_memory_slots AS slots
			ON CONFLICT (consumer_type, consumer_id, label) DO NOTHING`).Error
	})
}

// The copy cannot be told apart from later edits, so down keeps it.
func downMemoryBlockDefinitionsFromSlots(*mschema.Builder) error {
	return nil
}
