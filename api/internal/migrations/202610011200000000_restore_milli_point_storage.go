package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

// migration202610011200000000ID restores the milli-point storage layout on any
// database that ran the retired
// 202609171652571815_retire_internal_credit_units forward body.
//
// Why a new migration instead of deleting the old one: the current migration
// plan runs with gormigrate's ValidateUnknownMigrations enabled, so a database
// that already recorded 202609171652571815 would refuse to start if the file
// disappeared. The old migration's body is now empty (the layout must not change
// on a database that has not applied it), and this migration repairs the ones
// that did.
//
// The storage unit is milli-points in BIGINT: 1 point = 1000 milli-points and
// 1 USD = 1_000_000 milli-points at the pinned 1000 points/USD rate. That is
// what production already runs, and what the whole charging/reporting chain
// expects (millipoints.Value writes the raw int64; AssertMilliPointSchema
// rejects a fractional column at start-up).
const migration202610011200000000ID = "202610011200000000_restore_milli_point_storage"

func init() {
	registerSchemaMigration(migration202610011200000000ID, up202610011200000000, down202610011200000000)
}

// up202610011200000000 multiplies every credit column back into milli-points.
// The column type is the idempotency key: only numeric(20,3) columns are
// touched, so a database already storing BIGINT is left alone.
func up202610011200000000(schema *mschema.Builder) error {
	for _, target := range preRetiredCreditColumns() {
		if err := multiplyPreCreditColumns(schema, target.table, target.columns); err != nil {
			return err
		}
	}

	return schema.DataFix("restore llm_usage_bills.pricing_snapshot.charged_credits to milli-points and clear the unit marker", func(db *gorm.DB) error {
		if err := db.Exec("UPDATE llm_usage_bills SET pricing_snapshot = jsonb_set(pricing_snapshot, '{charged_credits}', to_jsonb(((pricing_snapshot->>'charged_credits')::numeric * 1000)), true) WHERE pricing_snapshot IS NOT NULL AND pricing_snapshot ? 'charged_credits' AND COALESCE(pricing_snapshot->>'unit', '') = 'points'").Error; err != nil {
			return err
		}
		return db.Exec("UPDATE llm_usage_bills SET pricing_snapshot = (pricing_snapshot - 'unit') WHERE pricing_snapshot IS NOT NULL AND COALESCE(pricing_snapshot->>'unit', '') = 'points'").Error
	})
}

// down202610011200000000 re-applies the fractional-points layout. It exists so
// the migration is reversible; it is not a supported production state, because
// the code asserts the milli-point schema at start-up.
func down202610011200000000(schema *mschema.Builder) error {
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
