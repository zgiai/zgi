package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

// Instance memory (ADR 0006) is business memory in a scope of type instance,
// one per end-user instance.
func init() {
	registerSchemaMigration("202610231100000000_allow_instance_memory_scope", upAllowInstanceMemoryScope, downAllowInstanceMemoryScope)
}

func upAllowInstanceMemoryScope(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.digital_human_memories DROP CONSTRAINT IF EXISTS digital_human_memories_scope_check;
		ALTER TABLE public.digital_human_memories ADD CONSTRAINT digital_human_memories_scope_check CHECK (
			(memory_class = 'self' AND scope_type = '' AND scope_id = '') OR
			(memory_class = 'business' AND scope_type IN ('workspace', 'project', 'customer', 'campaign', 'channel', 'instance') AND scope_id <> '')
		);
		CREATE INDEX IF NOT EXISTS idx_digital_human_memories_instance
			ON public.digital_human_memories (digital_human_id, scope_id, updated_at) WHERE scope_type = 'instance' AND status = 'active';
	`)
}

func downAllowInstanceMemoryScope(schema *mschema.Builder) error {
	return schema.Raw(`
		DROP INDEX IF EXISTS public.idx_digital_human_memories_instance;
		DELETE FROM public.digital_human_memories WHERE scope_type = 'instance';
		ALTER TABLE public.digital_human_memories DROP CONSTRAINT IF EXISTS digital_human_memories_scope_check;
		ALTER TABLE public.digital_human_memories ADD CONSTRAINT digital_human_memories_scope_check CHECK (
			(memory_class = 'self' AND scope_type = '' AND scope_id = '') OR
			(memory_class = 'business' AND scope_type IN ('workspace', 'project', 'customer', 'campaign', 'channel') AND scope_id <> '')
		);
	`)
}
