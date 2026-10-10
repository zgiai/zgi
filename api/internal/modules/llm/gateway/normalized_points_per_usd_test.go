package gateway

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/internal/modules/llm/billingsettings"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestNormalizedPointsPerUSDIsPinnedRegardlessOfStoredValue locks the pinned
// rate: a stored llm_billing_settings value is recorded but must not move any
// charge, matching main's hardcoded pricingCreditsPerUSD = 1_000_000.
func TestNormalizedPointsPerUSDIsPinnedRegardlessOfStoredValue(t *testing.T) {
	withValue := openGatewaySettingsTestDB(t, "with-value")
	require.NoError(t, withValue.Exec(
		"INSERT INTO llm_billing_settings (key, value, updated_at) VALUES (?, ?, datetime('now'))",
		billingsettings.KeyPointsPerUSD, "1500",
	).Error)

	store := billingsettings.Init(context.Background(), withValue)
	require.Equal(t, billingsettings.DefaultPointsPerUSD, NormalizedPointsPerUSD())
	require.True(t, store.HasValue(), "the stored value must still be observed")

	// Restore the default so later tests in this package see 1000.
	billingsettings.Init(context.Background(), openGatewaySettingsTestDB(t, "empty"))
	require.Equal(t, billingsettings.DefaultPointsPerUSD, NormalizedPointsPerUSD())
}

func openGatewaySettingsTestDB(t *testing.T, name string) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:gateway-"+name+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE TABLE llm_billing_settings (key text PRIMARY KEY, value text NOT NULL, updated_at datetime NOT NULL)").Error)
	return db
}
