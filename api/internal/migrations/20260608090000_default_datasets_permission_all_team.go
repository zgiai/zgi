package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration20260608090000ID = "20260608090000_default_datasets_permission_all_team"

func init() {
	registerSchemaMigration(
		migration20260608090000ID,
		upDefaultDatasetsPermissionAllTeam,
		downDefaultDatasetsPermissionAllTeam,
	)
}

func upDefaultDatasetsPermissionAllTeam(schema *mschema.Builder) error {
	if err := schema.UpdateRowsWhereEqual("datasets", "permission", "all_team", "permission", "only_me"); err != nil {
		return err
	}
	return schema.Raw(`ALTER TABLE public.datasets ALTER COLUMN permission SET DEFAULT 'all_team'`)
}

func downDefaultDatasetsPermissionAllTeam(schema *mschema.Builder) error {
	// Do not convert data back to only_me: rows created after this migration may
	// intentionally be private, and bulk reversal would corrupt user intent.
	return schema.Raw(`ALTER TABLE public.datasets ALTER COLUMN permission SET DEFAULT 'only_me'`)
}
