package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddLLMCredentialUpstreamRuntimeFieldsID = "202607111930000000_add_llm_credential_upstream_runtime_fields"

func init() {
	registerSchemaMigration(
		migrationAddLLMCredentialUpstreamRuntimeFieldsID,
		upAddLLMCredentialUpstreamRuntimeFields,
		downAddLLMCredentialUpstreamRuntimeFields,
	)
}

func upAddLLMCredentialUpstreamRuntimeFields(schema *mschema.Builder) error {
	exists, err := schema.HasTable("llm_credential_upstream_states")
	if err != nil || !exists {
		return err
	}

	if err := schema.WhenTableDoesntHaveColumn("llm_credential_upstream_states", "manual_retry_requested_at", func() error {
		return schema.Table("llm_credential_upstream_states", func(table *mschema.Blueprint) {
			table.TimestampTz("manual_retry_requested_at").Nullable()
		})
	}); err != nil {
		return err
	}
	if err := schema.WhenTableDoesntHaveColumn("llm_credential_upstream_states", "provider_error_code", func() error {
		return schema.Table("llm_credential_upstream_states", func(table *mschema.Blueprint) {
			table.String("provider_error_code", 128).DefaultSQL("''").NotNull()
		})
	}); err != nil {
		return err
	}
	return schema.WhenTableDoesntHaveColumn("llm_credential_upstream_states", "provider_error_status", func() error {
		return schema.Table("llm_credential_upstream_states", func(table *mschema.Blueprint) {
			table.Integer("provider_error_status").DefaultSQL("0").NotNull()
		})
	})
}

func downAddLLMCredentialUpstreamRuntimeFields(schema *mschema.Builder) error {
	for _, column := range []string{"provider_error_status", "provider_error_code", "manual_retry_requested_at"} {
		if err := schema.WhenTableHasColumn("llm_credential_upstream_states", column, func() error {
			return schema.Table("llm_credential_upstream_states", func(table *mschema.Blueprint) {
				table.DropColumn(column)
			})
		}); err != nil {
			return err
		}
	}
	return nil
}
