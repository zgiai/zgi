package migrations

import (
	"fmt"

	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

const migration20260609090000ID = "20260609090000_add_sql_guard_policy"
const migration20260609090000DefaultPolicy = `{"mode":"warn","readonly":false,"allow_multi_stmt":false,"block_statements":["drop","truncate","alter","create","grant","revoke"],"block_functions":["pg_read_file","pg_read_binary_file","pg_ls_dir","pg_stat_file"],"require_where":true,"max_join_depth":8,"allow_parse_failure":false}`

func init() {
	registerSchemaMigration(migration20260609090000ID, upAddSQLGuardPolicy, nil)
}

func upAddSQLGuardPolicy(schema *mschema.Builder) error {
	queries := []string{
		fmt.Sprintf("ALTER TABLE data_sources ADD COLUMN IF NOT EXISTS guard_policy JSONB NOT NULL DEFAULT '%s'::jsonb", migration20260609090000DefaultPolicy),
		"ALTER TABLE data_source_sql_operations ADD COLUMN IF NOT EXISTS guard_verdict VARCHAR(16)",
		"ALTER TABLE data_source_sql_operations ADD COLUMN IF NOT EXISTS guard_action VARCHAR(16)",
		"ALTER TABLE data_source_sql_operations ADD COLUMN IF NOT EXISTS guard_reasons JSONB",
		"ALTER TABLE data_source_sql_operations ADD COLUMN IF NOT EXISTS guard_policy JSONB",
	}
	for _, query := range queries {
		if err := schema.Raw(query); err != nil {
			return err
		}
	}
	return nil
}
