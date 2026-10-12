package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

func init() {
	registerSchemaMigration("202610121000000000_drop_digital_human_scope_name_index", upDropDigitalHumanScopeNameIndex, nil)
}

// DigitalHuman names are labels, not identifiers.  Keep the historical index
// removal forward-only so old databases stop rejecting otherwise valid names.
func upDropDigitalHumanScopeNameIndex(schema *mschema.Builder) error {
	return schema.Raw(`DROP INDEX IF EXISTS public.idx_digital_humans_scope_name;`)
}
