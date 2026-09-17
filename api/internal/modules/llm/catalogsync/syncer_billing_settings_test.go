package catalogsync

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/internal/modules/llm/billingsettings"
	"github.com/zgiai/zgi/api/internal/modules/llm/modelmeta"
	pb "github.com/zgiai/zgi/api/pkg/rpc/v1"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCatalogFromResponseCarriesPointsPerUSD(t *testing.T) {
	catalog := catalogFromResponse(&pb.GetPublishedCatalogResponse{Version: 7, PointsPerUsd: 777})

	require.Equal(t, int64(777), catalog.PointsPerUSD)
}

func TestApplyBillingSettingsPersistsCatalogRate(t *testing.T) {
	db := openSyncerTestDB(t)
	syncer := &Synchronizer{db: db}
	syncer.ensureSettings(context.Background())

	syncer.applyBillingSettings(context.Background(), modelmeta.PublishedCatalog{PointsPerUSD: 1234})

	var setting billingsettings.Setting
	require.NoError(t, db.Where("key = ?", billingsettings.KeyPointsPerUSD).First(&setting).Error)
	require.Equal(t, "1234", setting.Value)
	require.Equal(t, int64(1234), syncer.settings.PointsPerUSD())
}

func TestApplyBillingSettingsIgnoresMissingValue(t *testing.T) {
	db := openSyncerTestDB(t)
	syncer := &Synchronizer{db: db}
	syncer.ensureSettings(context.Background())

	syncer.applyBillingSettings(context.Background(), modelmeta.PublishedCatalog{PointsPerUSD: 0})

	var count int64
	require.NoError(t, db.Table("llm_billing_settings").Count(&count).Error)
	require.Zero(t, count)
}

func openSyncerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE TABLE llm_billing_settings (key text PRIMARY KEY, value text NOT NULL, updated_at datetime NOT NULL)").Error)
	return db
}
