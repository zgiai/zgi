package agents

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestAgentsRepositoryApplyFiltersMultipleTenants_FiltersPublishedStatus(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectClose()
	t.Cleanup(func() {
		require.NoError(t, sqlDB.Close())
	})

	db, err := gorm.Open(
		postgres.New(postgres.Config{Conn: sqlDB}),
		&gorm.Config{DryRun: true},
	)
	require.NoError(t, err)

	repo := &agentsRepository{db: db}
	isPublished := true
	publishedQuery := repo.applyFiltersMultipleTenants(
		db.Model(&Agent{}),
		AgentsFilter{IsPublished: &isPublished},
	).Find(&[]Agent{})
	publishedSQL := compactSQL(publishedQuery.Statement.SQL.String())

	require.Contains(t, publishedSQL, "EXISTS ( SELECT 1 FROM agent_published_versions")
	require.Contains(t, publishedSQL, "EXISTS ( SELECT 1 FROM workflows")
	require.Contains(t, publishedSQL, "workflows.version !=")
	require.NotContains(t, publishedSQL, "NOT (")
	require.Equal(t, []any{"AGENT", "AGENT", "draft"}, publishedQuery.Statement.Vars)

	isPublished = false
	draftQuery := repo.applyFiltersMultipleTenants(
		db.Model(&Agent{}),
		AgentsFilter{IsPublished: &isPublished},
	).Find(&[]Agent{})
	draftSQL := compactSQL(draftQuery.Statement.SQL.String())

	require.Contains(t, draftSQL, "NOT (")
	require.Contains(t, draftSQL, "EXISTS ( SELECT 1 FROM agent_published_versions")
	require.Contains(t, draftSQL, "EXISTS ( SELECT 1 FROM workflows")
	require.Equal(t, []any{"AGENT", "AGENT", "draft"}, draftQuery.Statement.Vars)

	offlineQuery := repo.applyFiltersMultipleTenants(
		db.Model(&Agent{}),
		AgentsFilter{WebAppStatus: string(AgentWebAppStatusInactive)},
	).Find(&[]Agent{})
	offlineSQL := compactSQL(offlineQuery.Statement.SQL.String())

	require.Contains(t, offlineSQL, "web_app_status =")
	require.Equal(t, []any{string(AgentWebAppStatusInactive)}, offlineQuery.Statement.Vars)
}

func compactSQL(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
