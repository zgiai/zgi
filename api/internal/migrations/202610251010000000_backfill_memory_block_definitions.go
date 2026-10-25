package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

// Every existing Agent memory slot becomes a memory block definition, and
// every Agent memory value points at the definition of its slot. Values whose
// slot was removed keep no definition. Both steps are idempotent.
func init() {
	registerSchemaMigration("202610251010000000_backfill_memory_block_definitions", upBackfillMemoryBlockDefinitions, downBackfillMemoryBlockDefinitions)
}

func upBackfillMemoryBlockDefinitions(schema *mschema.Builder) error {
	return schema.DataFix("backfill memory block definitions from agent memory slots", func(db *gorm.DB) error {
		if err := db.Exec(`
			INSERT INTO public.memory_block_definitions
				(id, consumer_type, consumer_id, label, name, description, limit_chars, partition, owner, auto_write, enabled, sort_order, created_at, updated_at)
			SELECT gen_random_uuid(), 'agent', slots.agent_id, slots.key, slots.name, slots.description,
				GREATEST(slots.max_chars, 1), 'person', 'user', true, slots.enabled, slots.sort_order, slots.created_at, slots.updated_at
			FROM public.agent_memory_slots AS slots
			ON CONFLICT (consumer_type, consumer_id, label) DO NOTHING`).Error; err != nil {
			return err
		}
		return db.Exec(`
			UPDATE public.user_memory_items AS items
			SET definition_id = definitions.id
			FROM public.user_memory_relationships AS relationships, public.memory_block_definitions AS definitions
			WHERE items.relationship_id = relationships.id
				AND relationships.consumer_type = 'agent'
				AND definitions.consumer_type = 'agent'
				AND definitions.consumer_id = relationships.consumer_id
				AND definitions.label = items.memory_key
				AND items.definition_id IS NULL`).Error
	})
}

func downBackfillMemoryBlockDefinitions(schema *mschema.Builder) error {
	return schema.Raw(`
		UPDATE public.user_memory_items SET definition_id = NULL WHERE definition_id IS NOT NULL;
		DELETE FROM public.memory_block_definitions WHERE consumer_type = 'agent';
	`)
}
