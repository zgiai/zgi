package service

import (
	"context"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestDashboardServiceTableExistsCacheIsConcurrentSafe(t *testing.T) {
	db, mock := openDashboardServiceMockDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = $1)`)).
		WithArgs("agents").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	svc := NewDashboardService(db).(*dashboardService)
	start := make(chan struct{})
	results := make(chan bool, 32)

	var wg sync.WaitGroup
	for i := 0; i < cap(results); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- svc.tableExists(context.Background(), "agents")
		}()
	}

	close(start)
	wg.Wait()
	close(results)

	for result := range results {
		require.True(t, result)
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardServiceRecentAgentsBranchesByRuntimeType(t *testing.T) {
	db, mock := openDashboardServiceMockDB(t)
	svc := NewDashboardService(db).(*dashboardService)
	svc.tableCache["agents"] = true
	svc.tableCache["workspaces"] = true

	updatedAt := time.Unix(1710000000, 0).UTC()
	mock.ExpectQuery(`(?s)SELECT .*FROM "?agents"? AS a.*a\.agent_type IN.*ORDER BY a\.updated_at DESC.*LIMIT`).
		WithArgs("ws-workflow", "WORKFLOW", "CONVERSATIONAL_WORKFLOW", false, false, 3).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"title",
			"resource_id",
			"workspace_id",
			"workspace_name",
			"updated_at",
			"created_at",
		}).AddRow("workflow-1", "Workflow One", "workflow-1", "ws-workflow", "Workflow Space", updatedAt, updatedAt))

	items := svc.getRecentAgents(context.Background(), []string{"ws-workflow"}, dashboardWorkflowAgentTypes, "workflow", 3)

	require.Len(t, items, 1)
	require.Equal(t, "workflow", items[0].Type)
	require.Equal(t, "workflow:workflow-1", items[0].ID)
	require.Equal(t, "workflow-1", items[0].ResourceID)
	require.Equal(t, "ws-workflow", items[0].WorkspaceID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardServiceRecentConversationsBranchesByRuntimeType(t *testing.T) {
	db, mock := openDashboardServiceMockDB(t)
	svc := NewDashboardService(db).(*dashboardService)
	svc.tableCache["agents_conversations"] = true
	svc.tableCache["agents"] = true
	svc.tableCache["workspaces"] = true

	updatedAt := time.Unix(1710000000, 0).UTC()
	mock.ExpectQuery(`(?s)SELECT .*FROM "?agents_conversations"? AS c.*a\.agent_type IN.*c\.from_account_id =.*ORDER BY c\.updated_at DESC.*LIMIT`).
		WithArgs("ws-workflow", "WORKFLOW", "CONVERSATIONAL_WORKFLOW", "acc-1", 3).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"title",
			"resource_id",
			"parent_id",
			"workspace_id",
			"workspace_name",
			"updated_at",
			"created_at",
		}).AddRow("conversation-1", "Conversation One", "conversation-1", "workflow-1", "ws-workflow", "Workflow Space", updatedAt, updatedAt))

	items := svc.getRecentAgentConversations(context.Background(), []string{"ws-workflow"}, dashboardWorkflowAgentTypes, "acc-1", 3)

	require.Len(t, items, 1)
	require.Equal(t, "conversation", items[0].Type)
	require.Equal(t, "workflow-1", items[0].ParentID)
	require.Equal(t, "ws-workflow", items[0].WorkspaceID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardServiceCountAgentAssetsSplitsWorkspaceScopes(t *testing.T) {
	db, mock := openDashboardServiceMockDB(t)
	svc := NewDashboardService(db).(*dashboardService)
	svc.tableCache["agents"] = true

	mock.ExpectQuery(`(?s)SELECT count\(\*\) FROM "?agents"? WHERE .*agent_type =.*agent_type IN`).
		WithArgs(false, false, "ws-agent", "AGENT", "ws-workflow", "WORKFLOW", "CONVERSATIONAL_WORKFLOW").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	count := svc.countAgentAssets(context.Background(), []string{"ws-agent"}, []string{"ws-workflow"})

	require.Equal(t, int64(2), count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func openDashboardServiceMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{})
	require.NoError(t, err)
	return db, mock
}
