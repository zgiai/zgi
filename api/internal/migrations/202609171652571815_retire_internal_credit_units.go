package migrations

import (
	"fmt"
	"strings"

	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

const migration202609171652571815ID = "202609171652571815_retire_internal_credit_units"

func init() {
	registerSchemaMigration(migration202609171652571815ID, up202609171652571815, down202609171652571815)
}

type preCreditColumnTarget struct {
	table   string
	columns []string
}

// preRetiredCreditColumns lists every Pre column that historically stored
// internal units (points x 1000). They are the columns the retired forward
// migration divided by 1000, and the ones
// 202610011200000000_restore_milli_point_storage multiplies back. The storage
// unit is milli-points: keep this list in sync with
// AssertMilliPointSchema in internal/bootstrap.
func preRetiredCreditColumns() []preCreditColumnTarget {
	return []preCreditColumnTarget{
		{"llm_usage_bills", []string{"official_points", "private_points", "total_points"}},
		{"video_runtime_tasks", []string{"estimated_credits", "actual_credits"}},
		{"group_ai_credit_accounts", []string{"purchased_credits", "total_earned", "total_spent"}},
		{"billing_attempt_entries", []string{"reserved_amount", "actual_amount", "refunded_amount"}},
		{"channel_wallets", []string{"balance"}},
		{"channel_wallet_transactions", []string{"amount", "balance_before", "balance_after"}},
		{"llm_workspace_quotas", []string{"quota_limit", "remain_quota", "used_quota"}},
		{"llm_developer_access_grants", []string{"quota_limit", "remain_quota", "used_quota"}},
		{"llm_developer_access_policies", []string{"default_quota", "max_quota"}},
		{"llm_developer_access_requests", []string{"requested_quota"}},
	}
}

// up202609171652571815 is RETIRED and intentionally a no-op.
//
// It used to divide every credit column by 1000 into numeric(20,3) fractional
// points. That layout is abandoned: the storage unit stays milli-points in
// BIGINT, which is what production already runs and what every reader and
// writer of these columns expects (millipoints.Value returns the raw int64).
//
// The migration ID must stay registered: the current migration plan runs with
// gormigrate's ValidateUnknownMigrations enabled, so dropping the file would
// make any database that already recorded it fail to start. Keeping the ID with
// an empty body means a database that has not applied it records it without
// touching data, and a database that did apply the old body is repaired by
// 202610011200000000_restore_milli_point_storage.
func up202609171652571815(schema *mschema.Builder) error {
	_ = schema
	return nil
}

// down202609171652571815 is retired together with the forward body: there is
// nothing left to roll back.
func down202609171652571815(schema *mschema.Builder) error {
	_ = schema
	return nil
}

func dividePreCreditColumns(schema *mschema.Builder, table string, columns []string) error {
	return schema.Raw(preAlterColumnsStatement(table, columns, true))
}

func multiplyPreCreditColumns(schema *mschema.Builder, table string, columns []string) error {
	return schema.Raw(preAlterColumnsStatement(table, columns, false))
}

// preAlterColumnsStatement builds one idempotent DO block that ALTERs only the
// columns still in internal units (forward) or already in points (reverse).
func preAlterColumnsStatement(table string, columns []string, toPoints bool) string {
	quoted := make([]string, 0, len(columns))
	for _, column := range columns {
		quoted = append(quoted, "'"+column+"'")
	}

	condition := "(data_type = 'bigint' OR (data_type = 'numeric' AND COALESCE(numeric_scale, 0) <> 3))"
	clause := "ALTER COLUMN %I TYPE numeric(20,3) USING (%I::numeric / 1000)"
	if !toPoints {
		condition = "(data_type = 'numeric' AND numeric_scale = 3)"
		clause = "ALTER COLUMN %I TYPE bigint USING ((%I * 1000)::bigint)"
	}

	// Build one ALTER TABLE per table so every column changes in a single
	// statement. Altering them one by one would trip cross-column CHECK
	// constraints (e.g. total_points = official_points + private_points)
	// between column changes.
	return fmt.Sprintf(`
DO $$
DECLARE credit_clauses text;
BEGIN
  SELECT string_agg(format('%s', column_name, column_name), ', ')
  INTO credit_clauses
  FROM information_schema.columns
  WHERE table_schema = 'public'
    AND table_name = '%s'
    AND column_name IN (%s)
    AND %s;
  IF credit_clauses IS NOT NULL AND credit_clauses <> '' THEN
    EXECUTE format('ALTER TABLE public.%%I %%s', '%s', credit_clauses);
  END IF;
END $$;`, clause, table, strings.Join(quoted, ", "), condition, table)
}
