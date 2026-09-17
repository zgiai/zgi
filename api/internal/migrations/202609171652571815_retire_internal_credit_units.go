package migrations

import (
	"fmt"
	"strings"

	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

const migration202609171652571815ID = "202609171652571815_retire_internal_credit_units"

func init() {
	registerSchemaMigration(migration202609171652571815ID, up202609171652571815, down202609171652571815)
}

type preCreditColumnTarget struct {
	table   string
	columns []string
}

// preRetiredCreditColumns lists every Pre column that stored internal units
// (points x 1000). They become numeric(20,3) points divided by 1000.
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

// up202609171652571815 divides every internal-unit column by 1000 into
// numeric(20,3) points. Column type is the idempotency key: the DO block only
// touches bigint or non-scale-3 numeric columns.
func up202609171652571815(schema *mschema.Builder) error {
	for _, target := range preRetiredCreditColumns() {
		if err := dividePreCreditColumns(schema, target.table, target.columns); err != nil {
			return err
		}
	}

	return schema.DataFix("divide llm_usage_bills.pricing_snapshot.charged_credits by 1000 and mark unit=points", func(db *gorm.DB) error {
		if err := db.Exec("UPDATE llm_usage_bills SET pricing_snapshot = jsonb_set(pricing_snapshot, '{charged_credits}', to_jsonb(((pricing_snapshot->>'charged_credits')::numeric / 1000)::numeric(20,3)), true) WHERE pricing_snapshot IS NOT NULL AND pricing_snapshot ? 'charged_credits' AND COALESCE(pricing_snapshot->>'unit', '') <> 'points'").Error; err != nil {
			return err
		}
		return db.Exec("UPDATE llm_usage_bills SET pricing_snapshot = jsonb_set(pricing_snapshot, '{unit}', to_jsonb('points'::text), true) WHERE pricing_snapshot IS NOT NULL AND pricing_snapshot ? 'charged_credits' AND COALESCE(pricing_snapshot->>'unit', '') <> 'points'").Error
	})
}

func down202609171652571815(schema *mschema.Builder) error {
	for _, target := range preRetiredCreditColumns() {
		if err := multiplyPreCreditColumns(schema, target.table, target.columns); err != nil {
			return err
		}
	}

	return schema.DataFix("multiply llm_usage_bills.pricing_snapshot.charged_credits by 1000 and clear unit marker", func(db *gorm.DB) error {
		if err := db.Exec("UPDATE llm_usage_bills SET pricing_snapshot = jsonb_set(pricing_snapshot, '{charged_credits}', to_jsonb(((pricing_snapshot->>'charged_credits')::numeric * 1000)::numeric(20,3)), true) WHERE pricing_snapshot IS NOT NULL AND pricing_snapshot ? 'charged_credits' AND COALESCE(pricing_snapshot->>'unit', '') = 'points'").Error; err != nil {
			return err
		}
		return db.Exec("UPDATE llm_usage_bills SET pricing_snapshot = (pricing_snapshot - 'unit') WHERE pricing_snapshot IS NOT NULL AND COALESCE(pricing_snapshot->>'unit', '') = 'points'").Error
	})
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
