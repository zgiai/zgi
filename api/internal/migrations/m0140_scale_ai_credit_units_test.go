package migrations

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestM0140_scale_ai_credit_units_PreservesFreshSubscriptionPlanCredits(t *testing.T) {
	db := openScaleAICreditUnitsMigrationTestDB(t)

	require.NoError(t, createSubscriptionPlansTestTable(db))
	require.NoError(t, insertSubscriptionPlanQuotaConfig(db, "plan-free", "free", map[string]interface{}{"monthly_ai_credits": float64(10000000)}))
	require.NoError(t, insertSubscriptionPlanQuotaConfig(db, "plan-team", "team", map[string]interface{}{"monthly_ai_credits": float64(100000000)}))
	require.NoError(t, insertSubscriptionPlanQuotaConfig(db, "plan-business", "business", map[string]interface{}{"monthly_ai_credits": float64(500000000)}))
	require.NoError(t, insertSubscriptionPlanQuotaConfig(db, "plan-enterprise", "enterprise", map[string]interface{}{"monthly_ai_credits": float64(-1)}))

	require.NoError(t, M0140_scale_ai_credit_units().Migrate(db))

	freePlan := loadSubscriptionPlanQuotaConfig(t, db, "free")
	require.Equal(t, float64(10000000), freePlan["monthly_ai_credits"])

	teamPlan := loadSubscriptionPlanQuotaConfig(t, db, "team")
	require.Equal(t, float64(100000000), teamPlan["monthly_ai_credits"])

	businessPlan := loadSubscriptionPlanQuotaConfig(t, db, "business")
	require.Equal(t, float64(500000000), businessPlan["monthly_ai_credits"])

	enterprisePlan := loadSubscriptionPlanQuotaConfig(t, db, "enterprise")
	require.Equal(t, float64(-1), enterprisePlan["monthly_ai_credits"])
}

func TestM0140_scale_ai_credit_units_ScalesLegacySubscriptionPlanCreditsOnce(t *testing.T) {
	db := openScaleAICreditUnitsMigrationTestDB(t)

	require.NoError(t, createSubscriptionPlansTestTable(db))
	require.NoError(t, insertSubscriptionPlanQuotaConfig(db, "plan-legacy", "legacy", map[string]interface{}{
		"monthly_ai_credits": float64(10000),
		"ai_credits":         float64(2000),
	}))

	require.NoError(t, M0140_scale_ai_credit_units().Migrate(db))

	legacyPlan := loadSubscriptionPlanQuotaConfig(t, db, "legacy")
	require.Equal(t, float64(10000000), legacyPlan["monthly_ai_credits"])
	require.Equal(t, float64(2000000), legacyPlan["ai_credits"])
}

func TestAllMigrationsIncludesScaleAICreditUnits(t *testing.T) {
	const targetID = "20260408000140"

	for _, m := range allMigrations() {
		if m.ID == targetID {
			return
		}
	}

	t.Fatalf("expected migration %s to be registered in allMigrations()", targetID)
}

func openScaleAICreditUnitsMigrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:zgi-api-scale-ai-credit-units-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	return db
}

func createSubscriptionPlansTestTable(db *gorm.DB) error {
	return db.Exec(`
		CREATE TABLE subscription_plans (
			id TEXT PRIMARY KEY,
			plan_code TEXT NOT NULL,
			quota_config TEXT NOT NULL
		)
	`).Error
}

func insertSubscriptionPlanQuotaConfig(db *gorm.DB, id, planCode string, quotaConfig map[string]interface{}) error {
	data, err := json.Marshal(quotaConfig)
	if err != nil {
		return err
	}
	return db.Exec(
		`INSERT INTO subscription_plans (id, plan_code, quota_config) VALUES (?, ?, ?)`,
		id,
		planCode,
		string(data),
	).Error
}

func loadSubscriptionPlanQuotaConfig(t *testing.T, db *gorm.DB, planCode string) map[string]interface{} {
	t.Helper()

	var row struct {
		QuotaConfig string `gorm:"column:quota_config"`
	}
	require.NoError(t, db.Table("subscription_plans").Select("quota_config").Where("plan_code = ?", planCode).First(&row).Error)

	var quotaConfig map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(row.QuotaConfig), &quotaConfig))
	return quotaConfig
}
