package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	officialmodel "github.com/zgiai/ginext/internal/modules/llm/officialmodel"
	"github.com/zgiai/ginext/internal/modules/llm/shared"
	"gorm.io/gorm"
)

func setupOfficialModelServiceDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:official_model_service_%s?mode=memory&cache=shared", uuid.NewString())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&officialmodel.Snapshot{}))
	require.NoError(t, db.Exec(`
		CREATE TABLE llm_routes (
			id TEXT PRIMARY KEY,
			organization_id TEXT NOT NULL,
			type TEXT NOT NULL,
			name TEXT NOT NULL DEFAULT '',
			models JSON NULL,
			provider TEXT NULL,
			protocol TEXT NULL,
			api_base_url TEXT NULL,
			priority INTEGER NOT NULL DEFAULT 0,
			weight INTEGER NOT NULL DEFAULT 1,
			is_enabled BOOLEAN NOT NULL DEFAULT TRUE,
			is_official BOOLEAN NOT NULL DEFAULT FALSE,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at DATETIME NULL
		)
	`).Error)
	require.NoError(t, db.Exec(`
		CREATE TABLE llm_models (
			id TEXT PRIMARY KEY,
			provider TEXT NOT NULL,
			name TEXT NOT NULL,
			display_name TEXT NOT NULL,
			is_active BOOLEAN NOT NULL DEFAULT TRUE,
			is_system_enabled BOOLEAN NOT NULL DEFAULT TRUE,
			sort_order INTEGER NOT NULL DEFAULT 0,
			deleted_at DATETIME NULL
		)
	`).Error)

	return db
}

func TestGetAvailableModelsFromRoutesIncludesOfficialSnapshotModels(t *testing.T) {
	db := setupOfficialModelServiceDB(t)
	svc := &modelService{db: db}

	orgID := uuid.New()
	require.NoError(t, db.Exec(`
		INSERT INTO llm_routes (id, organization_id, type, name, provider, protocol, is_official, is_enabled, priority, weight)
		VALUES (?, ?, ?, ?, ?, ?, TRUE, TRUE, 200, 100)
	`, uuid.NewString(), orgID.String(), string(shared.RouteTypeZGICloud), "ZGI Cloud", "zgi-cloud", "openai").Error)

	_, err := officialmodel.SyncFromChannels(context.Background(), db, []officialmodel.UpstreamChannel{
		{ID: "ch-1", Models: []string{"gpt-4o", "gpt-4.1"}},
	}, officialmodel.SyncMeta{})
	require.NoError(t, err)

	models := svc.getAvailableModelsFromRoutes(context.Background(), orgID)
	require.True(t, models["gpt-4o"])
	require.True(t, models["gpt-4.1"])
}

func TestListOfficialModelsReadsEffectiveSnapshotModels(t *testing.T) {
	db := setupOfficialModelServiceDB(t)
	svc := &modelService{db: db}

	require.NoError(t, db.Exec(`
		INSERT INTO llm_routes (id, organization_id, type, name, provider, protocol, is_official, is_enabled, priority, weight)
		VALUES (?, ?, ?, ?, ?, ?, TRUE, TRUE, 200, 100)
	`, uuid.NewString(), uuid.NewString(), string(shared.RouteTypeZGICloud), "ZGI Cloud", "zgi-cloud", "openai").Error)

	require.NoError(t, db.Exec(`
		INSERT INTO llm_models (id, provider, name, display_name, is_active, is_system_enabled)
		VALUES (?, 'openai', 'gpt-4o', 'GPT-4o', TRUE, TRUE)
	`, uuid.NewString()).Error)
	require.NoError(t, db.Exec(`
		INSERT INTO llm_models (id, provider, name, display_name, is_active, is_system_enabled)
		VALUES (?, 'openai', 'gpt-4.1', 'GPT-4.1', TRUE, TRUE)
	`, uuid.NewString()).Error)

	_, err := officialmodel.SyncFromChannels(context.Background(), db, []officialmodel.UpstreamChannel{
		{ID: "ch-1", Models: []string{"gpt-4o", "gpt-4.1"}},
	}, officialmodel.SyncMeta{})
	require.NoError(t, err)

	models, err := svc.ListOfficialModels(context.Background())
	require.NoError(t, err)
	require.Len(t, models, 2)
}
