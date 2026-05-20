package agents

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAgentsRepository_ListRunnableWebApps_ReturnsPublishedUndeletedDistinctItems(t *testing.T) {
	ctx := t.Context()
	db := setupRunnableWebAppRepositoryTestDB(t)
	seedRunnableWebAppRepositoryData(t, db)

	repo := NewAgentsRepository(db)
	items, err := repo.ListRunnableWebApps(ctx, []string{"ws-alpha", "ws-beta"}, "")
	require.NoError(t, err)
	require.Equal(t, []runnableWebAppItem{
		{
			AgentID:      "11111111-1111-1111-1111-111111111111",
			WorkspaceID:  "ws-alpha",
			WebAppID:     "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			WebAppStatus: "active",
			AgentName:    "alpha-published",
			AgentIcon:    stringPtr(`{"type":"emoji","value":"🤖"}`),
			AgentDesc:    "published",
			AgentType:    "CONVERSATIONAL_WORKFLOW",
		},
		{
			AgentID:      "44444444-4444-4444-4444-444444444444",
			WorkspaceID:  "ws-beta",
			WebAppID:     "dddddddd-dddd-dddd-dddd-dddddddddddd",
			WebAppStatus: "active",
			AgentName:    "beta-published",
			AgentIcon:    nil,
			AgentDesc:    "published",
			AgentType:    "CONVERSATIONAL_WORKFLOW",
		},
	}, items)
}

func TestAgentsRepository_ListRunnableWebApps_RespectsWorkspaceFilter(t *testing.T) {
	ctx := t.Context()
	db := setupRunnableWebAppRepositoryTestDB(t)
	seedRunnableWebAppRepositoryData(t, db)

	repo := NewAgentsRepository(db)
	items, err := repo.ListRunnableWebApps(ctx, []string{"ws-alpha", "ws-beta"}, "ws-beta")
	require.NoError(t, err)
	require.Equal(t, []runnableWebAppItem{
		{
			AgentID:      "44444444-4444-4444-4444-444444444444",
			WorkspaceID:  "ws-beta",
			WebAppID:     "dddddddd-dddd-dddd-dddd-dddddddddddd",
			WebAppStatus: "active",
			AgentName:    "beta-published",
			AgentIcon:    nil,
			AgentDesc:    "published",
			AgentType:    "CONVERSATIONAL_WORKFLOW",
		},
	}, items)
}

func TestAgentsRepository_UpdateWebAppStatus_WritesAndClearsOfflineAudit(t *testing.T) {
	ctx := t.Context()
	db := setupRunnableWebAppRepositoryTestDB(t)
	seedRunnableWebAppRepositoryData(t, db)

	repo := NewAgentsRepository(db)
	err := repo.UpdateWebAppStatus(
		ctx,
		"11111111-1111-1111-1111-111111111111",
		AgentWebAppStatusInactive,
		"maintenance",
		"99999999-9999-9999-9999-999999999999",
	)
	require.NoError(t, err)

	var inactiveRow struct {
		WebAppStatus        string     `gorm:"column:web_app_status"`
		WebAppOfflinedAt    *time.Time `gorm:"column:web_app_offlined_at"`
		WebAppOfflinedBy    *string    `gorm:"column:web_app_offlined_by"`
		WebAppOfflineReason string     `gorm:"column:web_app_offline_reason"`
	}
	require.NoError(t, db.Table("agents").
		Select("web_app_status, web_app_offlined_at, web_app_offlined_by, web_app_offline_reason").
		Where("id = ?", "11111111-1111-1111-1111-111111111111").
		Scan(&inactiveRow).Error)
	require.Equal(t, "inactive", inactiveRow.WebAppStatus)
	require.NotNil(t, inactiveRow.WebAppOfflinedAt)
	require.NotNil(t, inactiveRow.WebAppOfflinedBy)
	require.Equal(t, "99999999-9999-9999-9999-999999999999", *inactiveRow.WebAppOfflinedBy)
	require.Equal(t, "maintenance", inactiveRow.WebAppOfflineReason)

	err = repo.UpdateWebAppStatus(
		ctx,
		"11111111-1111-1111-1111-111111111111",
		AgentWebAppStatusActive,
		"ignored",
		"99999999-9999-9999-9999-999999999999",
	)
	require.NoError(t, err)

	var activeRow struct {
		WebAppStatus        string     `gorm:"column:web_app_status"`
		WebAppOfflinedAt    *time.Time `gorm:"column:web_app_offlined_at"`
		WebAppOfflinedBy    *string    `gorm:"column:web_app_offlined_by"`
		WebAppOfflineReason string     `gorm:"column:web_app_offline_reason"`
	}
	require.NoError(t, db.Table("agents").
		Select("web_app_status, web_app_offlined_at, web_app_offlined_by, web_app_offline_reason").
		Where("id = ?", "11111111-1111-1111-1111-111111111111").
		Scan(&activeRow).Error)
	require.Equal(t, "active", activeRow.WebAppStatus)
	require.Nil(t, activeRow.WebAppOfflinedAt)
	require.Nil(t, activeRow.WebAppOfflinedBy)
	require.Empty(t, activeRow.WebAppOfflineReason)
}

func stringPtr(v string) *string {
	return &v
}

func setupRunnableWebAppRepositoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`
		CREATE TABLE agents (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			name TEXT NOT NULL,
			description TEXT NOT NULL,
			agent_type TEXT NOT NULL,
			icon_type TEXT NULL,
			icon TEXT NULL,
			agents_model_config_id TEXT NULL,
			workflow_id TEXT NULL,
			workflow_config TEXT NULL,
			enable_api BOOLEAN NOT NULL DEFAULT FALSE,
			is_public BOOLEAN NOT NULL DEFAULT FALSE,
			is_universal BOOLEAN NOT NULL DEFAULT FALSE,
			internal BOOLEAN NOT NULL DEFAULT FALSE,
			web_app_id TEXT NOT NULL UNIQUE,
			web_app_status TEXT NOT NULL DEFAULT 'active',
			web_app_offlined_at DATETIME NULL,
			web_app_offlined_by TEXT NULL,
			web_app_offline_reason TEXT NOT NULL DEFAULT '',
			created_by TEXT NULL,
			created_at DATETIME NOT NULL,
			updated_by TEXT NULL,
			updated_at DATETIME NOT NULL,
			deleted_by TEXT NULL,
			deleted_at DATETIME NULL
		)
	`).Error)
	require.NoError(t, db.Exec(`
		CREATE TABLE workflows (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			app_id TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			type TEXT NOT NULL,
			version TEXT NOT NULL,
			graph TEXT NOT NULL DEFAULT '',
			features TEXT NOT NULL DEFAULT '',
			created_by TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_by TEXT NULL,
			updated_at DATETIME NOT NULL,
			environment_variables TEXT NOT NULL DEFAULT '[]',
			conversation_variables TEXT NOT NULL DEFAULT '[]',
			internal BOOLEAN NOT NULL DEFAULT FALSE
		)
	`).Error)

	return db
}

func seedRunnableWebAppRepositoryData(t *testing.T, db *gorm.DB) {
	t.Helper()

	now := time.Now().UTC()
	require.NoError(t, db.Create(&Agent{
		ID:          uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		TenantID:    uuid.Nil,
		Name:        "alpha-published",
		Description: "published",
		AgentsType:  "CONVERSATIONAL_WORKFLOW",
		Icon:        stringPtr(`{"type":"emoji","value":"🤖"}`),
		WebAppID:    uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"),
		CreatedAt:   now,
		UpdatedAt:   now,
	}).Error)
	require.NoError(t, db.Exec(`UPDATE agents SET tenant_id = ? WHERE id = ?`, "ws-alpha", "11111111-1111-1111-1111-111111111111").Error)
	require.NoError(t, db.Create(&Agent{
		ID:          uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		TenantID:    uuid.Nil,
		Name:        "alpha-deleted",
		Description: "deleted",
		AgentsType:  "CONVERSATIONAL_WORKFLOW",
		WebAppID:    uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"),
		CreatedAt:   now.Add(time.Second),
		UpdatedAt:   now.Add(time.Second),
	}).Error)
	require.NoError(t, db.Exec(`UPDATE agents SET tenant_id = ?, deleted_at = ? WHERE id = ?`, "ws-alpha", now.Add(10*time.Second), "22222222-2222-2222-2222-222222222222").Error)
	require.NoError(t, db.Create(&Agent{
		ID:          uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		TenantID:    uuid.Nil,
		Name:        "beta-draft-only",
		Description: "draft only",
		AgentsType:  "CONVERSATIONAL_WORKFLOW",
		WebAppID:    uuid.MustParse("cccccccc-cccc-cccc-cccc-cccccccccccc"),
		CreatedAt:   now.Add(2 * time.Second),
		UpdatedAt:   now.Add(2 * time.Second),
	}).Error)
	require.NoError(t, db.Exec(`UPDATE agents SET tenant_id = ? WHERE id = ?`, "ws-beta", "33333333-3333-3333-3333-333333333333").Error)
	require.NoError(t, db.Create(&Agent{
		ID:          uuid.MustParse("44444444-4444-4444-4444-444444444444"),
		TenantID:    uuid.Nil,
		Name:        "beta-published",
		Description: "published",
		AgentsType:  "CONVERSATIONAL_WORKFLOW",
		WebAppID:    uuid.MustParse("dddddddd-dddd-dddd-dddd-dddddddddddd"),
		CreatedAt:   now.Add(3 * time.Second),
		UpdatedAt:   now.Add(3 * time.Second),
	}).Error)
	require.NoError(t, db.Exec(`UPDATE agents SET tenant_id = ? WHERE id = ?`, "ws-beta", "44444444-4444-4444-4444-444444444444").Error)
	require.NoError(t, db.Create(&Agent{
		ID:          uuid.MustParse("55555555-5555-5555-5555-555555555555"),
		TenantID:    uuid.Nil,
		Name:        "gamma-published",
		Description: "outside scope",
		AgentsType:  "CONVERSATIONAL_WORKFLOW",
		WebAppID:    uuid.MustParse("eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"),
		CreatedAt:   now.Add(4 * time.Second),
		UpdatedAt:   now.Add(4 * time.Second),
	}).Error)
	require.NoError(t, db.Exec(`UPDATE agents SET tenant_id = ? WHERE id = ?`, "ws-gamma", "55555555-5555-5555-5555-555555555555").Error)
	require.NoError(t, db.Create(&Agent{
		ID:           uuid.MustParse("66666666-6666-6666-6666-666666666666"),
		TenantID:     uuid.Nil,
		Name:         "alpha-inactive",
		Description:  "inactive",
		AgentsType:   "CONVERSATIONAL_WORKFLOW",
		WebAppID:     uuid.MustParse("ffffffff-ffff-ffff-ffff-ffffffffffff"),
		WebAppStatus: AgentWebAppStatusInactive,
		CreatedAt:    now.Add(5 * time.Second),
		UpdatedAt:    now.Add(5 * time.Second),
	}).Error)
	require.NoError(t, db.Exec(`UPDATE agents SET tenant_id = ? WHERE id = ?`, "ws-alpha", "66666666-6666-6666-6666-666666666666").Error)

	require.NoError(t, db.Exec(`
		INSERT INTO workflows (id, tenant_id, app_id, agent_id, type, version, created_by, created_at, updated_at)
		VALUES
			('wf-1', 'ws-alpha', 'app-1', '11111111-1111-1111-1111-111111111111', 'WORKFLOW', '202603180001', 'account-1', ?, ?),
			('wf-2', 'ws-alpha', 'app-1', '11111111-1111-1111-1111-111111111111', 'WORKFLOW', '202603180002', 'account-1', ?, ?),
			('wf-3', 'ws-alpha', 'app-2', '22222222-2222-2222-2222-222222222222', 'WORKFLOW', '202603180001', 'account-1', ?, ?),
			('wf-4', 'ws-beta', 'app-3', '33333333-3333-3333-3333-333333333333', 'WORKFLOW', 'draft', 'account-1', ?, ?),
			('wf-5', 'ws-beta', 'app-4', '44444444-4444-4444-4444-444444444444', 'WORKFLOW', '202603180001', 'account-1', ?, ?),
			('wf-6', 'ws-gamma', 'app-5', '55555555-5555-5555-5555-555555555555', 'WORKFLOW', '202603180001', 'account-1', ?, ?),
			('wf-7', 'ws-alpha', 'app-6', '66666666-6666-6666-6666-666666666666', 'WORKFLOW', '202603180001', 'account-1', ?, ?)
	`, now, now, now, now, now, now, now, now, now, now, now, now, now, now).Error)
}
