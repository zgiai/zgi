package migrations

const migration202609031410353961ID = "202609031410353961_finalize_connector_detachment"

func init() {
	registerConnectorRetirementMigration(migration202609031410353961ID)
}
