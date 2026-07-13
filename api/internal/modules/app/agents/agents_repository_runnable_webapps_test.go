package agents

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/internal/modules/app/runtimeauth"
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

	result, err := repo.ListRunnableWebApps(t.Context(), []string{workspaceID}, runnableWebAppFilter{})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Equal(t, "Published Agent", result.Items[0].AgentName)
	require.Equal(t, "AGENT", result.Items[0].AgentType)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentsRepository_ListRunnableWebApps_AppliesAuthorizationBeforePagination(t *testing.T) {
	db, mock := newRunnableWebAppsMockDB(t)
	repo := NewAgentsRepository(db)

	organizationID := uuid.New()
	accountID := uuid.New()
	workspaceID := uuid.New()
	agentID := uuid.New()
	webAppID := uuid.New()
	audience := runtimeauth.RuntimeAudience{
		OrganizationID: organizationID,
		AccountID:      accountID,
		WorkspaceIDs:   []uuid.UUID{workspaceID},
	}

	mock.ExpectQuery(`(?s)SELECT count\(\*\) FROM "agents".*published_runtime_surfaces.*published_runtime_surface_grants`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))
	mock.ExpectQuery(`(?s)SELECT agents\.id AS agent_id.*FROM "agents".*published_runtime_surfaces.*published_runtime_surface_grants.*ORDER BY agents\.tenant_id ASC,agents\.created_at DESC LIMIT .* OFFSET .*`).
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
			"Authorized app",
			nil,
			nil,
			"description",
			"AGENT",
		))

	result, err := repo.ListRunnableWebApps(t.Context(), []string{workspaceID.String()}, runnableWebAppFilter{
		Pagination: runnableWebAppPagination{
			enabled:  true,
			page:     2,
			pageSize: 2,
		},
		Authorization: &runnableWebAppAuthorizationFilter{Audience: audience},
	})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Equal(t, 5, result.Pagination.total)
	require.True(t, result.Pagination.hasMore)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentsRepository_ListRunnableWebApps_FiltersByKeyword(t *testing.T) {
	db, mock := newRunnableWebAppsMockDB(t)
	repo := NewAgentsRepository(db)

	workspaceID := "11111111-1111-1111-1111-111111111111"
	pattern := "%report%"
	mock.ExpectQuery(`(?s)SELECT .* FROM "agents".*agents\.name ILIKE .*agents\.description ILIKE.*ORDER BY agents\.tenant_id ASC,agents\.created_at DESC`).
		WithArgs(AgentWebAppStatusActive, workspaceID, "AGENT", "AGENT", "draft", pattern, pattern).
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
		}))

	result, err := repo.ListRunnableWebApps(t.Context(), []string{workspaceID}, runnableWebAppFilter{
		Keyword: " report ",
	})
	require.NoError(t, err)
	require.Empty(t, result.Items)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentsRepository_ListRunnableWebApps_FiltersByWebAppID(t *testing.T) {
	db, mock := newRunnableWebAppsMockDB(t)
	repo := NewAgentsRepository(db)

	workspaceID := "11111111-1111-1111-1111-111111111111"
	webAppID := "22222222-2222-2222-2222-222222222222"
	mock.ExpectQuery(`(?s)SELECT .* FROM "agents".*agents\.web_app_id = .*ORDER BY agents\.tenant_id ASC,agents\.created_at DESC`).
		WithArgs(AgentWebAppStatusActive, workspaceID, "AGENT", "AGENT", "draft", webAppID).
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
		}))

	result, err := repo.ListRunnableWebApps(t.Context(), []string{workspaceID}, runnableWebAppFilter{
		WebAppID: webAppID,
	})
	require.NoError(t, err)
	require.Empty(t, result.Items)
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
