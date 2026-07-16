package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

const migrationCreateLLMCredentialUpstreamStatesID = "202607100900000000_create_llm_credential_upstream_states"

func init() {
	registerSchemaMigration(
		migrationCreateLLMCredentialUpstreamStatesID,
		upCreateLLMCredentialUpstreamStates,
		downCreateLLMCredentialUpstreamStates,
	)
}

func upCreateLLMCredentialUpstreamStates(schema *mschema.Builder) error {
	exists, err := schema.HasTable("llm_credential_upstream_states")
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	if err := schema.Create("llm_credential_upstream_states", func(table *mschema.Blueprint) {
		table.UUID("credential_id").NotNull().Primary()
		table.UUID("organization_id").NotNull()
		table.BigInteger("generation").DefaultSQL("1").NotNull()

		table.String("balance_capability", 32).DefaultSQL("'unknown'").NotNull()
		table.JSONB("balance_snapshot").Nullable()
		table.TimestampTz("balance_observed_at").Nullable()
		table.JSONB("warning_thresholds").DefaultSQL("'[]'::jsonb").NotNull()

		table.String("availability", 32).DefaultSQL("'unknown'").NotNull()
		table.String("observation_source", 32).DefaultSQL("''").NotNull()
		table.TimestampTz("availability_observed_at").Nullable()

		table.TimestampTz("last_check_at").Nullable()
		table.String("last_check_status", 32).DefaultSQL("'unknown'").NotNull()
		table.String("last_check_error_kind", 64).DefaultSQL("''").NotNull()
		table.TimestampTz("next_check_at").Nullable()
		table.TimestampTz("check_lease_until").Nullable()
		table.Integer("consecutive_failures").DefaultSQL("0").NotNull()

		table.String("block_reason", 32).DefaultSQL("''").NotNull()
		table.TimestampTz("cooldown_until").Nullable()
		table.Integer("guard_strikes").DefaultSQL("0").NotNull()
		table.TimestampTz("half_open_lease_until").Nullable()
		table.TimestampTz("manual_retry_requested_at").Nullable()
		table.String("provider_error_code", 128).DefaultSQL("''").NotNull()
		table.Integer("provider_error_status").DefaultSQL("0").NotNull()

		table.TimestampsTz()
		table.Index("idx_llm_credential_upstream_states_org", "organization_id")
		table.Index("idx_llm_credential_upstream_states_due", "next_check_at", "check_lease_until")
		table.Foreign(
			"fk_llm_credential_upstream_states_credential",
			[]string{"credential_id"},
			"llm_credentials",
			[]string{"id"},
		).CascadeOnDelete()
	}); err != nil {
		return err
	}

	return schema.Raw(`
		INSERT INTO llm_credential_upstream_states (
			credential_id,
			organization_id,
			next_check_at
		)
		SELECT id, organization_id, CURRENT_TIMESTAMP
		FROM llm_credentials
		WHERE deleted_at IS NULL
		ON CONFLICT (credential_id) DO NOTHING
	`)
}

func downCreateLLMCredentialUpstreamStates(schema *mschema.Builder) error {
	return schema.DropIfExists("llm_credential_upstream_states")
}
