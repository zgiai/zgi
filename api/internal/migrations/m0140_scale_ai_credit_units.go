package migrations

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	zgiAICreditScaleFactor         int64 = 1000
	zgiAICreditMaxInt64BeforeScale       = int64(9223372036854775)
	zgiTransactionMaxBeforeScale         = "9999999999999.9999"
	zgiLegacyQuotaScaleThreshold         = int64(10000000)
	zgiTransactionDecimalType            = "DECIMAL(20,4)"
)

func M0140_scale_ai_credit_units() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260408000140",
		Migrate: func(tx *gorm.DB) error {
			if err := scaleZGIAPIBigintColumns(tx, "group_ai_credit_accounts", "subscription_credits", "purchased_credits", "total_earned", "total_spent"); err != nil {
				return err
			}
			if err := expandZGIAPICreditTransactionPrecision(tx); err != nil {
				return err
			}
			if err := scaleZGIAPICreditTransactions(tx); err != nil {
				return err
			}
			if err := scaleZGIAIOrderSnapshots(tx); err != nil {
				return err
			}
			if err := scaleZGIAPIBigintColumns(tx, "llm_organization_api_keys", "used_quota", "remain_quota", "quota_limit"); err != nil {
				return err
			}
			if err := scaleZGIAPIBigintColumns(tx, "llm_workspace_quotas", "used_quota", "remain_quota", "quota_limit"); err != nil {
				return err
			}
			if err := scaleZGIAPIBigintColumns(tx, "billing_attempt_entries", "reserved_amount", "actual_amount", "refunded_amount"); err != nil {
				return err
			}
			if err := scaleZGIAPIBigintColumns(tx, "llm_usage_bills", "official_points", "private_points", "total_points"); err != nil {
				return err
			}
			if err := scaleZGIAPISubscriptionQuotas(tx); err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}

func scaleZGIAPIBigintColumns(tx *gorm.DB, table string, columns ...string) error {
	if !tx.Migrator().HasTable(table) {
		return nil
	}

	existing := zgiAPIExistingColumns(tx, table, columns...)
	if len(existing) == 0 {
		return nil
	}

	var overflowCount int64
	if err := tx.Raw(zgiAPIBigintOverflowCountSQL(table, existing...)).Scan(&overflowCount).Error; err != nil {
		return fmt.Errorf("precheck overflow for %s: %w", table, err)
	}
	if overflowCount > 0 {
		return fmt.Errorf("table %s contains values too large to scale by %d", table, zgiAICreditScaleFactor)
	}

	return tx.Exec(zgiAPIScaleColumnsSQL(table, existing...)).Error
}

func scaleZGIAPICreditTransactions(tx *gorm.DB) error {
	if !tx.Migrator().HasTable("transactions") {
		return nil
	}

	var overflowCount int64
	overflowSQL := fmt.Sprintf(`
		SELECT COUNT(1)
		FROM %s
		WHERE currency_type IN ('subscription_credits', 'purchased_credits')
		  AND (
			ABS(COALESCE(amount, 0)) > %s OR
			ABS(COALESCE(balance_before, 0)) > %s OR
			ABS(COALESCE(balance_after, 0)) > %s
		  )
	`, zgiAPIQuoteIdent("transactions"), zgiTransactionMaxBeforeScale, zgiTransactionMaxBeforeScale, zgiTransactionMaxBeforeScale)
	if err := tx.Raw(overflowSQL).Scan(&overflowCount).Error; err != nil {
		return fmt.Errorf("precheck transactions overflow: %w", err)
	}
	if overflowCount > 0 {
		return fmt.Errorf("transactions contains credit values too large to scale by %d", zgiAICreditScaleFactor)
	}

	return tx.Exec(fmt.Sprintf(`
		UPDATE %s
		SET
			amount = amount * %d,
			balance_before = balance_before * %d,
			balance_after = balance_after * %d
		WHERE currency_type IN ('subscription_credits', 'purchased_credits')
	`, zgiAPIQuoteIdent("transactions"), zgiAICreditScaleFactor, zgiAICreditScaleFactor, zgiAICreditScaleFactor)).Error
}

func expandZGIAPICreditTransactionPrecision(tx *gorm.DB) error {
	if !tx.Migrator().HasTable("transactions") {
		return nil
	}

	// SQLite test DBs don't support ALTER COLUMN TYPE and also don't enforce DECIMAL precision.
	if tx.Dialector.Name() == "sqlite" {
		return nil
	}

	existing := zgiAPIExistingColumns(tx, "transactions", "amount", "balance_before", "balance_after")
	if len(existing) == 0 {
		return nil
	}

	clauses := make([]string, 0, len(existing))
	for _, column := range existing {
		quoted := zgiAPIQuoteIdent(column)
		clauses = append(clauses, fmt.Sprintf(
			"ALTER COLUMN %s TYPE %s USING %s::%s",
			quoted,
			zgiTransactionDecimalType,
			quoted,
			zgiTransactionDecimalType,
		))
	}

	return tx.Exec(fmt.Sprintf(
		"ALTER TABLE %s %s",
		zgiAPIQuoteIdent("transactions"),
		strings.Join(clauses, ", "),
	)).Error
}

func scaleZGIAIOrderSnapshots(tx *gorm.DB) error {
	if !tx.Migrator().HasTable("orders") || !tx.Migrator().HasColumn("orders", "product_snapshot") {
		return nil
	}

	var overflowCount int64
	overflowSQL := fmt.Sprintf(`
		SELECT COUNT(1)
		FROM %s
		WHERE order_type = 'credit_purchase'
		  AND product_snapshot IS NOT NULL
		  AND (
			(product_snapshot ? 'credit_amount' AND ABS((product_snapshot->>'credit_amount')::numeric) > %d)
			OR
			(product_snapshot ? 'unit_credits' AND ABS((product_snapshot->>'unit_credits')::numeric) > %d)
		  )
	`, zgiAPIQuoteIdent("orders"), zgiAICreditMaxInt64BeforeScale, zgiAICreditMaxInt64BeforeScale)
	if err := tx.Raw(overflowSQL).Scan(&overflowCount).Error; err != nil {
		return fmt.Errorf("precheck orders.product_snapshot overflow: %w", err)
	}
	if overflowCount > 0 {
		return fmt.Errorf("orders.product_snapshot contains values too large to scale by %d", zgiAICreditScaleFactor)
	}

	return tx.Exec(fmt.Sprintf(`
		UPDATE %s
		SET product_snapshot = CASE
			WHEN product_snapshot ? 'credit_amount' AND product_snapshot ? 'unit_credits' THEN
				jsonb_set(
					jsonb_set(
						product_snapshot,
						'{credit_amount}',
						to_jsonb((((product_snapshot->>'credit_amount')::numeric) * %d)::bigint),
						true
					),
					'{unit_credits}',
					to_jsonb((((product_snapshot->>'unit_credits')::numeric) * %d)::bigint),
					true
				)
			WHEN product_snapshot ? 'credit_amount' THEN
				jsonb_set(
					product_snapshot,
					'{credit_amount}',
					to_jsonb((((product_snapshot->>'credit_amount')::numeric) * %d)::bigint),
					true
				)
			ELSE product_snapshot
		END
		WHERE order_type = 'credit_purchase'
		  AND product_snapshot IS NOT NULL
		  AND product_snapshot ? 'credit_amount'
	`, zgiAPIQuoteIdent("orders"), zgiAICreditScaleFactor, zgiAICreditScaleFactor, zgiAICreditScaleFactor)).Error
}

func scaleZGIAPISubscriptionQuotas(tx *gorm.DB) error {
	if !tx.Migrator().HasTable("subscription_plans") || !tx.Migrator().HasColumn("subscription_plans", "quota_config") {
		return nil
	}

	var plans []zgiSubscriptionPlanQuotaRow
	if err := tx.Table("subscription_plans").Select("id", "quota_config").Find(&plans).Error; err != nil {
		return fmt.Errorf("load subscription plans: %w", err)
	}

	for _, plan := range plans {
		quotaConfig, err := parseSubscriptionPlanQuotaConfig(plan.QuotaConfig)
		if err != nil {
			return fmt.Errorf("parse subscription plan %s quota_config: %w", plan.ID, err)
		}

		scaledConfig, changed, err := scaleLegacySubscriptionQuotaConfig(quotaConfig)
		if err != nil {
			return fmt.Errorf("scale subscription plan %s quota_config: %w", plan.ID, err)
		}
		if !changed {
			continue
		}

		quotaConfigJSON, err := json.Marshal(scaledConfig)
		if err != nil {
			return fmt.Errorf("marshal subscription plan %s quota_config: %w", plan.ID, err)
		}

		if err := tx.Table("subscription_plans").
			Where("id = ?", plan.ID).
			Update("quota_config", datatypes.JSON(quotaConfigJSON)).Error; err != nil {
			return fmt.Errorf("update subscription plan %s quota_config: %w", plan.ID, err)
		}
	}

	return nil
}

type zgiSubscriptionPlanQuotaRow struct {
	ID          string         `gorm:"column:id"`
	QuotaConfig datatypes.JSON `gorm:"column:quota_config"`
}

func parseSubscriptionPlanQuotaConfig(raw datatypes.JSON) (map[string]interface{}, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	var quotaConfig map[string]interface{}
	if err := json.Unmarshal(raw, &quotaConfig); err != nil {
		return nil, err
	}

	return quotaConfig, nil
}

func zgiAPIExistingColumns(tx *gorm.DB, table string, columns ...string) []string {
	existing := make([]string, 0, len(columns))
	for _, column := range columns {
		if tx.Migrator().HasColumn(table, column) {
			existing = append(existing, column)
		}
	}
	return existing
}

func zgiAPIBigintOverflowCountSQL(table string, columns ...string) string {
	parts := make([]string, 0, len(columns))
	for _, column := range columns {
		quoted := zgiAPIQuoteIdent(column)
		parts = append(parts, fmt.Sprintf("ABS(COALESCE(%s, 0)) > %d", quoted, zgiAICreditMaxInt64BeforeScale))
	}
	return fmt.Sprintf(
		"SELECT COUNT(1) FROM %s WHERE %s",
		zgiAPIQuoteIdent(table),
		strings.Join(parts, " OR "),
	)
}

func zgiAPIScaleColumnsSQL(table string, columns ...string) string {
	assignments := make([]string, 0, len(columns))
	for _, column := range columns {
		quoted := zgiAPIQuoteIdent(column)
		assignments = append(assignments, fmt.Sprintf("%s = %s * %d", quoted, quoted, zgiAICreditScaleFactor))
	}
	return fmt.Sprintf("UPDATE %s SET %s", zgiAPIQuoteIdent(table), strings.Join(assignments, ", "))
}

func zgiAPIQuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func scaleLegacySubscriptionQuotaConfig(quotaConfig map[string]interface{}) (map[string]interface{}, bool, error) {
	if quotaConfig == nil {
		return nil, false, nil
	}

	cloned := make(map[string]interface{}, len(quotaConfig))
	for key, value := range quotaConfig {
		cloned[key] = value
	}

	changed := false
	for _, key := range []string{"monthly_ai_credits", "ai_credits"} {
		value, ok := quotaConfig[key]
		if !ok {
			continue
		}

		scaled, shouldScale, err := scaleLegacyQuotaValue(value)
		if err != nil {
			return nil, false, fmt.Errorf("%s: %w", key, err)
		}
		if !shouldScale {
			continue
		}

		cloned[key] = scaled
		changed = true
	}

	if !changed {
		return quotaConfig, false, nil
	}

	return cloned, true, nil
}

func scaleLegacyQuotaValue(raw interface{}) (int64, bool, error) {
	value, ok, err := quotaValueToInt64(raw)
	if err != nil {
		return 0, false, err
	}
	if !ok {
		return 0, false, nil
	}

	if value <= 0 || value >= zgiLegacyQuotaScaleThreshold {
		return value, false, nil
	}

	return value * zgiAICreditScaleFactor, true, nil
}

func quotaValueToInt64(raw interface{}) (int64, bool, error) {
	switch v := raw.(type) {
	case int:
		return int64(v), true, nil
	case int64:
		return v, true, nil
	case float64:
		return int64(v), true, nil
	case json.Number:
		parsed, err := v.Int64()
		if err != nil {
			return 0, false, err
		}
		return parsed, true, nil
	default:
		return 0, false, nil
	}
}
