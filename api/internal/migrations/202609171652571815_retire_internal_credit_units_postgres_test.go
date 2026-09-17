package migrations

import (
	"os"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestRetireInternalCreditUnitsPrePostgres verifies the dynamic ALTER and JSON
// DataFix against a throwaway PostgreSQL database. Skipped unless
// ZGI_RETIRE_TEST_DSN points at a dedicated empty database.
func TestRetireInternalCreditUnitsPrePostgres(t *testing.T) {
	dsn := os.Getenv("ZGI_RETIRE_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_RETIRE_TEST_DSN to run the PostgreSQL migration verification")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	for _, table := range []string{"channel_wallets", "llm_usage_bills"} {
		require.NoError(t, db.Exec("DROP TABLE IF EXISTS "+table+" CASCADE").Error)
	}
	require.NoError(t, db.Exec("CREATE TABLE channel_wallets (channel_id uuid PRIMARY KEY, balance bigint NOT NULL)").Error)
	require.NoError(t, db.Exec("CREATE TABLE llm_usage_bills (id uuid PRIMARY KEY, official_points bigint, total_points bigint, pricing_snapshot jsonb)").Error)
	require.NoError(t, db.Exec("INSERT INTO channel_wallets (channel_id, balance) VALUES ('00000000-0000-0000-0000-000000000011', 500000000)").Error)
	require.NoError(t, db.Exec("INSERT INTO llm_usage_bills (id, official_points, total_points, pricing_snapshot) VALUES ('00000000-0000-0000-0000-000000000012', 428, 428, '{\"charged_credits\": 500000}'::jsonb)").Error)

	require.NoError(t, up202609171652571815(mschema.New(db)))

	var balance, dataType, scale string
	require.NoError(t, db.Raw("SELECT balance::text FROM channel_wallets WHERE channel_id = '00000000-0000-0000-0000-000000000011'").Scan(&balance).Error)
	require.True(t, decimal.RequireFromString("500000").Equal(decimal.RequireFromString(balance)), "balance = %s", balance)
	require.NoError(t, db.Raw("SELECT data_type FROM information_schema.columns WHERE table_schema='public' AND table_name='channel_wallets' AND column_name='balance'").Scan(&dataType).Error)
	require.Equal(t, "numeric", dataType)
	require.NoError(t, db.Raw("SELECT numeric_scale::text FROM information_schema.columns WHERE table_schema='public' AND table_name='channel_wallets' AND column_name='balance'").Scan(&scale).Error)
	require.Equal(t, "3", scale)

	var charged, unit string
	require.NoError(t, db.Raw("SELECT pricing_snapshot ->> 'charged_credits' FROM llm_usage_bills WHERE id = '00000000-0000-0000-0000-000000000012'").Scan(&charged).Error)
	require.True(t, decimal.RequireFromString("500").Equal(decimal.RequireFromString(charged)), "charged_credits = %s", charged)
	require.NoError(t, db.Raw("SELECT pricing_snapshot ->> 'unit' FROM llm_usage_bills WHERE id = '00000000-0000-0000-0000-000000000012'").Scan(&unit).Error)
	require.Equal(t, "points", unit)

	// Idempotency: a second up must not divide again.
	require.NoError(t, up202609171652571815(mschema.New(db)))
	require.NoError(t, db.Raw("SELECT balance::text FROM channel_wallets WHERE channel_id = '00000000-0000-0000-0000-000000000011'").Scan(&balance).Error)
	require.True(t, decimal.RequireFromString("500000").Equal(decimal.RequireFromString(balance)), "balance after second up = %s", balance)
	require.NoError(t, db.Raw("SELECT pricing_snapshot ->> 'charged_credits' FROM llm_usage_bills WHERE id = '00000000-0000-0000-0000-000000000012'").Scan(&charged).Error)
	require.True(t, decimal.RequireFromString("500").Equal(decimal.RequireFromString(charged)), "charged_credits after second up = %s", charged)

	// Rollback restores bigint values and clears the marker.
	require.NoError(t, down202609171652571815(mschema.New(db).AllowDestructive()))
	require.NoError(t, db.Raw("SELECT balance::text FROM channel_wallets WHERE channel_id = '00000000-0000-0000-0000-000000000011'").Scan(&balance).Error)
	require.True(t, decimal.RequireFromString("500000000").Equal(decimal.RequireFromString(balance)), "restored balance = %s", balance)
	require.NoError(t, db.Raw("SELECT pricing_snapshot ->> 'charged_credits' FROM llm_usage_bills WHERE id = '00000000-0000-0000-0000-000000000012'").Scan(&charged).Error)
	require.True(t, decimal.RequireFromString("500000").Equal(decimal.RequireFromString(charged)), "restored charged_credits = %s", charged)
	require.NoError(t, db.Raw("SELECT COALESCE(pricing_snapshot ->> 'unit', '') FROM llm_usage_bills WHERE id = '00000000-0000-0000-0000-000000000012'").Scan(&unit).Error)
	require.Equal(t, "", unit)
}
