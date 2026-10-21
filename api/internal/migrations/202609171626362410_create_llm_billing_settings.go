package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

const migration202609171626362410ID = "202609171626362410_create_llm_billing_settings"

func init() {
	registerSchemaMigration(migration202609171626362410ID, up202609171626362410, down202609171626362410)
}

// up202609171626362410 creates the tiny local settings table that carries the
// Console-authoritative points_per_usd discount rate to Pre. The catalogsync
// synchronizer is the only writer; the row seeded here is the offline default
// used before the first catalog sync or while Console is unreachable.
func up202609171626362410(schema *mschema.Builder) error {
	if err := schema.Create("llm_billing_settings", func(table *mschema.Blueprint) {
		table.String("key", 64).Primary()
		table.Text("value").NotNull()
		table.TimestampTz("updated_at").DefaultSQL("now()").NotNull()
	}); err != nil {
		return err
	}

	return schema.Raw("INSERT INTO public.llm_billing_settings (\"key\", \"value\") VALUES ('points_per_usd', '1000') ON CONFLICT (\"key\") DO NOTHING")
}

func down202609171626362410(schema *mschema.Builder) error {
	return schema.DropIfExists("llm_billing_settings")
}
