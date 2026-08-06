package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

const migration202608040452036859ID = "202608040452036859_standardize_graphflow_llm_extraction"

func init() {
	registerSchemaMigration(migration202608040452036859ID, up202608040452036859, down202608040452036859)
}

func up202608040452036859(schema *mschema.Builder) error {
	if err := schema.Raw(`
		ALTER TABLE public.datasets
			ALTER COLUMN extraction_strategy SET DEFAULT 'llm';
	`); err != nil {
		return err
	}
	return schema.DataFix("standardize dataset graph extraction strategy to llm", func(db *gorm.DB) error {
		return db.Table("datasets").
			Where("extraction_strategy IS NULL OR extraction_strategy <> ?", "llm").
			Update("extraction_strategy", "llm").Error
	})
}

func down202608040452036859(schema *mschema.Builder) error {
	// Restore the old default only. Existing datasets are deliberately not
	// rewritten because their previous per-row strategy cannot be recovered.
	return schema.Raw(`
		ALTER TABLE public.datasets
			ALTER COLUMN extraction_strategy SET DEFAULT 'openie';
	`)
}
