package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration202606130635416578ID = "202606130635416578_add_llm_model_lifecycle_fields"

func init() {
	registerSchemaMigration(migration202606130635416578ID, up202606130635416578, down202606130635416578)
}

func up202606130635416578(schema *mschema.Builder) error {
	if err := schema.WhenTableDoesntHaveColumn("llm_models", "replacement_provider", func() error {
		return schema.Table("llm_models", func(table *mschema.Blueprint) {
			table.String("replacement_provider", 100).Nullable()
		})
	}); err != nil {
		return err
	}
	if err := schema.WhenTableDoesntHaveColumn("llm_models", "replacement_model", func() error {
		return schema.Table("llm_models", func(table *mschema.Blueprint) {
			table.String("replacement_model", 100).Nullable()
		})
	}); err != nil {
		return err
	}
	return schema.WhenTableDoesntHaveColumn("llm_models", "deprecation_reason", func() error {
		return schema.Table("llm_models", func(table *mschema.Blueprint) {
			table.Text("deprecation_reason").Nullable()
		})
	})
}

func down202606130635416578(schema *mschema.Builder) error {
	if err := schema.WhenTableHasColumn("llm_models", "deprecation_reason", func() error {
		return schema.Table("llm_models", func(table *mschema.Blueprint) {
			table.DropColumn("deprecation_reason")
		})
	}); err != nil {
		return err
	}
	if err := schema.WhenTableHasColumn("llm_models", "replacement_model", func() error {
		return schema.Table("llm_models", func(table *mschema.Blueprint) {
			table.DropColumn("replacement_model")
		})
	}); err != nil {
		return err
	}
	return schema.WhenTableHasColumn("llm_models", "replacement_provider", func() error {
		return schema.Table("llm_models", func(table *mschema.Blueprint) {
			table.DropColumn("replacement_provider")
		})
	})
}
