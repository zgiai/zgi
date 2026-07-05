package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

const migrationAddOrganizationBillingDisplaySettingsID = "202607030900000000_add_organization_billing_display_settings"

func init() {
	registerSchemaMigration(
		migrationAddOrganizationBillingDisplaySettingsID,
		upAddOrganizationBillingDisplaySettings,
		downAddOrganizationBillingDisplaySettings,
	)
}

func upAddOrganizationBillingDisplaySettings(schema *mschema.Builder) error {
	if err := schema.WhenTableDoesntHaveColumn("organizations", "billing_display_currency", func() error {
		return schema.Table("organizations", func(table *mschema.Blueprint) {
			table.String("billing_display_currency", 3).DefaultSQL("'USD'").NotNull()
		})
	}); err != nil {
		return err
	}

	return schema.WhenTableDoesntHaveColumn("organizations", "usd_to_cny_rate", func() error {
		return schema.Table("organizations", func(table *mschema.Blueprint) {
			table.Decimal("usd_to_cny_rate", 18, 6).DefaultSQL("7").NotNull()
		})
	})
}

func downAddOrganizationBillingDisplaySettings(schema *mschema.Builder) error {
	for _, column := range []string{"billing_display_currency", "usd_to_cny_rate"} {
		if err := schema.WhenTableHasColumn("organizations", column, func() error {
			return schema.Table("organizations", func(table *mschema.Blueprint) {
				table.DropColumn(column)
			})
		}); err != nil {
			return err
		}
	}
	return nil
}
