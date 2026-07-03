package agents

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestAgentsRepository_ListRunnableWebApps_BranchesPublishedChecksByAgentType(t *testing.T) {
	db, mock := newRunnableWebAppsMockDB(t)
	repo := NewAgentsRepository(db)

	workspaceID := "11111111-1111-1111-1111-111111111111"
	agentID := "22222222-2222-2222-2222-222222222222"
	webAppID := "33333333-3333-3333-3333-333333333333"

	mock.ExpectQuery(`(?s)SELECT .* FROM "agents".*agents\.agent_type = .*agent_published_versions.*agent_published_versions\.deleted_at IS NULL.*agents\.agent_type != .*workflows.*workflows\.version !=.*ORDER BY agents\.tenant_id ASC,agents\.created_at DESC`).
		WithArgs(AgentWebAppStatusActive, workspaceID, "AGENT", "AGENT", "draft").
		WillReturnRows(sqlmock.NewRows([]string{
			"agent_id",
			"workspace_id",
			"web_app_id",
			"web_app_status",
			"agent_name",
			"agent_icon",
			"agent_icon_type",
			"agent_desc",
			"agent_type",
		}).AddRow(
			agentID,
			workspaceID,
			webAppID,
			string(AgentWebAppStatusActive),
			"Published Agent",
			nil,
			nil,
			"agent description",
			"AGENT",
		))

	items, err := repo.ListRunnableWebApps(t.Context(), []string{workspaceID}, "")
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "Published Agent", items[0].AgentName)
	require.Equal(t, "AGENT", items[0].AgentType)
	require.NoError(t, mock.ExpectationsWereMet())
}

func newRunnableWebAppsMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{
		DisableAutomaticPing: true,
	})
	require.NoError(t, err)

	return db, mock
}
