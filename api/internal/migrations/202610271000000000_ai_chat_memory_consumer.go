package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

// AI chat remembers each person as a memory consumer of its own (ADR 0021),
// so the relationship store accepts it beside Agents and DigitalHumans.
func init() {
	registerSchemaMigration("202610271000000000_ai_chat_memory_consumer", upAIChatMemoryConsumer, downAIChatMemoryConsumer)
}

func upAIChatMemoryConsumer(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.user_memory_relationships DROP CONSTRAINT IF EXISTS ck_user_memory_consumer_type;
		ALTER TABLE public.user_memory_relationships ADD CONSTRAINT ck_user_memory_consumer_type
			CHECK (consumer_type IN ('agent', 'digital_human', 'ai_chat'));
	`)
}

// The check is restored only when no AI chat memory remains; otherwise it
// stays wide, because restoring it would mean deleting people's memory.
func downAIChatMemoryConsumer(schema *mschema.Builder) error {
	return schema.Raw(`
		DO $$
		BEGIN
			IF NOT EXISTS (SELECT 1 FROM public.user_memory_relationships WHERE consumer_type = 'ai_chat') THEN
				ALTER TABLE public.user_memory_relationships DROP CONSTRAINT IF EXISTS ck_user_memory_consumer_type;
				ALTER TABLE public.user_memory_relationships ADD CONSTRAINT ck_user_memory_consumer_type
					CHECK (consumer_type IN ('agent', 'digital_human'));
			END IF;
		END $$;
	`)
}
