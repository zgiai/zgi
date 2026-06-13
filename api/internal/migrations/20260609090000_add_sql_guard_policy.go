package migrations

import (
	"fmt"
	"strings"

	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"github.com/zgiai/zgi/api/pkg/sql_base/guard"
)

const migration20260609090000ID = "20260609090000_add_sql_guard_policy"

func init() {
	registerSchemaMigration(migration20260609090000ID, upAddSQLGuardPolicy, nil)
}

func upAddSQLGuardPolicy(schema *mschema.Builder) error {
	defaultPolicy := strings.ReplaceAll(string(guard.DefaultPolicyJSON()), "'", "''")
	queries := []string{
		fmt.Sprintf("ALTER TABLE data_sources ADD COLUMN IF NOT EXISTS guard_policy JSONB NOT NULL DEFAULT '%s'::jsonb", defaultPolicy),
		"ALTER TABLE data_source_sql_operations ADD COLUMN IF NOT EXISTS guard_verdict VARCHAR(16)",
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
