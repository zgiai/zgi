package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

// Purging a memory version's content now also clears its evidence, reason,
// and former display name, which can quote personal information. Versions
// purged before that change kept them; this clears them once. New purges
// clear them as they happen.
func init() {
	registerSchemaMigration("202610261500000000_purge_reasons_of_purged_memory_versions", upPurgeReasonsOfPurgedMemoryVersions, downPurgeReasonsOfPurgedMemoryVersions)
}

func upPurgeReasonsOfPurgedMemoryVersions(schema *mschema.Builder) error {
	return schema.DataFix("clear what purged memory versions still quote", func(db *gorm.DB) error {
		return db.Exec(`
			UPDATE public.memory_file_versions
			SET before_payload = '', after_payload = '', before_key_id = NULL, after_key_id = NULL,
				evidence = '', reason = '', before_display_name = ''
			WHERE content_purged_at IS NOT NULL
				AND (before_payload <> '' OR after_payload <> '' OR before_key_id IS NOT NULL OR after_key_id IS NOT NULL
					OR evidence <> '' OR reason <> '' OR before_display_name <> '')`).Error
	})
}

// Cleared text cannot be restored.
func downPurgeReasonsOfPurgedMemoryVersions(*mschema.Builder) error {
	return nil
}
