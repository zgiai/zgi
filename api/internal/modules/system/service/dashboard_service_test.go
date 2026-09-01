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

	"github.com/zgiai/zgi/api/internal/modules/system/model"
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

func TestDashboardServiceRecentWorkIncludesDirectChatConversations(t *testing.T) {
	db, mock := openDashboardServiceMockDB(t)
	svc := NewDashboardService(db).(*dashboardService)
	svc.tableCache["chat_runtime_conversations"] = true
	svc.tableCache["workspaces"] = true

	updatedAt := time.Unix(1710000000, 0).UTC()
	mock.ExpectQuery(`(?s)SELECT .*FROM "?chat_runtime_conversations"? AS c.*LEFT JOIN workspaces AS w.*c\.organization_id =.*c\.account_id =.*c\.source =.*c\.caller_type =.*c\.conversation_type =.*c\.dialogue_count > 0.*c\.workspace_id IN.*ORDER BY c\.updated_at DESC.*LIMIT`).
		WithArgs("org-1", "acc-1", "console", "aichat", "chat", "ws-1", 5).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"title",
			"resource_id",
			"workspace_id",
			"workspace_name",
			"updated_at",
			"created_at",
		}).AddRow("chat-1", "First direct chat", "chat-1", "ws-1", "Default Workspace", updatedAt, updatedAt))

	resp, err := svc.GetRecentWork(context.Background(), model.RecentWorkRequest{
		OrganizationID: "org-1",
		AccountID:      "acc-1",
		Limit:          5,
		WorkspaceIDs:   []string{"ws-1"},
	})

	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	require.Equal(t, "conversation", resp.Items[0].Type)
	require.Equal(t, "chat-1", resp.Items[0].ResourceID)
	require.Empty(t, resp.Items[0].ParentID)
	require.Equal(t, "ws-1", resp.Items[0].WorkspaceID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardServiceRecentWorkRefreshesBetweenRequests(t *testing.T) {
	db, mock := openDashboardServiceMockDB(t)
	svc := NewDashboardService(db).(*dashboardService)
	svc.tableCache["chat_runtime_conversations"] = true
	svc.tableCache["workspaces"] = true

	updatedAt := time.Unix(1710000000, 0).UTC()
	query := `(?s)SELECT .*FROM "?chat_runtime_conversations"? AS c.*LEFT JOIN workspaces AS w.*c\.organization_id =.*c\.account_id =.*c\.source =.*c\.caller_type =.*c\.conversation_type =.*c\.dialogue_count > 0.*c\.workspace_id IN.*ORDER BY c\.updated_at DESC.*LIMIT`
	columns := []string{"id", "title", "resource_id", "workspace_id", "workspace_name", "updated_at", "created_at"}
	mock.ExpectQuery(query).
		WithArgs("org-1", "acc-1", "console", "aichat", "chat", "ws-1", 5).
		WillReturnRows(sqlmock.NewRows(columns))
	mock.ExpectQuery(query).
		WithArgs("org-1", "acc-1", "console", "aichat", "chat", "ws-1", 5).
		WillReturnRows(sqlmock.NewRows(columns).AddRow("chat-1", "First direct chat", "chat-1", "ws-1", "Default Workspace", updatedAt, updatedAt))

	req := model.RecentWorkRequest{
		OrganizationID: "org-1",
		AccountID:      "acc-1",
		Limit:          5,
		WorkspaceIDs:   []string{"ws-1"},
	}
	first, err := svc.GetRecentWork(context.Background(), req)
	require.NoError(t, err)
	require.Empty(t, first.Items)

	second, err := svc.GetRecentWork(context.Background(), req)
	require.NoError(t, err)
	require.Len(t, second.Items, 1)
	require.Equal(t, "chat-1", second.Items[0].ResourceID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardServiceRecentWorkUsesLogScopedConversationWorkspaces(t *testing.T) {
	db, mock := openDashboardServiceMockDB(t)
	svc := NewDashboardService(db).(*dashboardService)
	svc.tableCache["agents_conversations"] = true
	svc.tableCache["agents"] = true
	svc.tableCache["workspaces"] = true

	newer := time.Unix(1710000002, 0).UTC()
	older := time.Unix(1710000001, 0).UTC()
	mock.ExpectQuery(`(?s)SELECT .*FROM "?agents_conversations"? AS c.*a\.agent_type IN.*c\.from_account_id =.*ORDER BY c\.updated_at DESC.*LIMIT`).
		WithArgs("ws-agent-logs", "AGENT", "acc-1", 5).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"title",
			"resource_id",
			"parent_id",
			"workspace_id",
			"workspace_name",
			"updated_at",
			"created_at",
		}).AddRow("conversation-agent", "Agent Conversation", "conversation-agent", "agent-1", "ws-agent-logs", "Agent Logs Space", older, older))
	mock.ExpectQuery(`(?s)SELECT .*FROM "?agents_conversations"? AS c.*a\.agent_type IN.*c\.from_account_id =.*ORDER BY c\.updated_at DESC.*LIMIT`).
		WithArgs("ws-workflow-logs", "WORKFLOW", "CONVERSATIONAL_WORKFLOW", "acc-1", 5).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"title",
			"resource_id",
			"parent_id",
			"workspace_id",
			"workspace_name",
			"updated_at",
			"created_at",
		}).AddRow("conversation-workflow", "Workflow Conversation", "conversation-workflow", "workflow-1", "ws-workflow-logs", "Workflow Logs Space", newer, newer))

	resp, err := svc.GetRecentWork(context.Background(), model.RecentWorkRequest{
		AccountID:                        "acc-1",
		Limit:                            5,
		AgentConversationWorkspaceIDs:    []string{"ws-agent-logs"},
		WorkflowConversationWorkspaceIDs: []string{"ws-workflow-logs"},
	})

	require.NoError(t, err)
	require.Len(t, resp.Items, 2)
	require.Equal(t, "conversation:conversation-workflow", resp.Items[0].ID)
	require.Equal(t, "workflow-1", resp.Items[0].ParentID)
	require.Equal(t, "ws-workflow-logs", resp.Items[0].WorkspaceID)
	require.Equal(t, "conversation:conversation-agent", resp.Items[1].ID)
	require.Equal(t, "agent-1", resp.Items[1].ParentID)
	require.Equal(t, "ws-agent-logs", resp.Items[1].WorkspaceID)
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

func TestDashboardServiceDatasetStatsUsesWorkspaceScopeOnly(t *testing.T) {
	db, mock := openDashboardServiceMockDB(t)
	svc := NewDashboardService(db).(*dashboardService)
	svc.tableCache["datasets"] = true

	mock.ExpectQuery(`(?s)SELECT count\(\*\) FROM "?datasets"? WHERE workspace_id IN`).
		WithArgs("ws-knowledge").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	stats := svc.getResourceStats(context.Background(), "org-1", "acc-1", model.DashboardWorkspaceScopes{
		DatasetWorkspaceIDs: []string{"ws-knowledge"},
	})

	require.Equal(t, int64(2), stats.Datasets)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardServiceFileStatsUsesWorkspaceScopeOnly(t *testing.T) {
	db, mock := openDashboardServiceMockDB(t)
	svc := NewDashboardService(db).(*dashboardService)
	svc.tableCache["upload_files"] = true

	mock.ExpectQuery(`(?s)SELECT count\(\*\) FROM "?upload_files"? WHERE workspace_id IN`).
		WithArgs("ws-files").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	stats := svc.getResourceStats(context.Background(), "org-1", "acc-1", model.DashboardWorkspaceScopes{
		FileWorkspaceIDs: []string{"ws-files"},
	})

	require.Equal(t, int64(3), stats.Files)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardServiceActivityStatsCountsStableDirectChatHistory(t *testing.T) {
	db, mock := openDashboardServiceMockDB(t)
	svc := NewDashboardService(db).(*dashboardService)
	svc.tableCache["chat_runtime_conversations"] = true
	svc.tableCache["agents"] = true
	svc.tableCache["datasets"] = true

	mock.ExpectQuery(`(?s)SELECT count\(\*\) FROM "?chat_runtime_conversations"? WHERE .*organization_id =.*account_id =.*workspace_id IN.*source =.*caller_type =.*conversation_type =.*dialogue_count > 0`).
		WithArgs("org-1", "acc-1", "ws-1", "console", "aichat", "chat").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`(?s)SELECT count\(\*\) FROM "?agents"? WHERE .*tenant_id IN.*created_by =.*agent_type =.*is_universal =.*internal`).
		WithArgs("ws-1", "acc-1", "AGENT", false, false).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`(?s)SELECT count\(\*\) FROM "?datasets"? WHERE .*workspace_id IN.*created_by =`).
		WithArgs("ws-1", "acc-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	stats := svc.getActivityStats(context.Background(), "org-1", "acc-1", model.DashboardWorkspaceScopes{
		WorkspaceIDs: []string{"ws-1"},
	})

	require.Equal(t, int64(1), stats.DirectConversations)
	require.Equal(t, int64(1), stats.CreatedAgents)
	require.Equal(t, int64(1), stats.CreatedDatasets)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardServiceStatsRefreshesOnboardingActivityBetweenRequests(t *testing.T) {
	db, mock := openDashboardServiceMockDB(t)
	svc := NewDashboardService(db).(*dashboardService)
	svc.tableCache["llm_models"] = false
	svc.tableCache["chat_runtime_conversations"] = true
	svc.tableCache["agents"] = false
	svc.tableCache["datasets"] = false

	query := `(?s)SELECT count\(\*\) FROM "?chat_runtime_conversations"? WHERE .*organization_id =.*account_id =.*workspace_id IN.*source =.*caller_type =.*conversation_type =.*dialogue_count > 0`
	mock.ExpectQuery(query).
		WithArgs("org-1", "acc-1", "ws-1", "console", "aichat", "chat").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(query).
		WithArgs("org-1", "acc-1", "ws-1", "console", "aichat", "chat").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	scopes := model.DashboardWorkspaceScopes{WorkspaceIDs: []string{"ws-1"}}
	first, err := svc.GetDashboardStats(context.Background(), "org-1", "acc-1", scopes)
	require.NoError(t, err)
	require.Equal(t, int64(1), first.Activity.DirectConversations)

	second, err := svc.GetDashboardStats(context.Background(), "org-1", "acc-1", scopes)
	require.NoError(t, err)
	require.Equal(t, int64(2), second.Activity.DirectConversations)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardServiceRecentDatasetsUsesWorkspaceScopeOnly(t *testing.T) {
	db, mock := openDashboardServiceMockDB(t)
	svc := NewDashboardService(db).(*dashboardService)
	svc.tableCache["datasets"] = true
	svc.tableCache["workspaces"] = true

	updatedAt := time.Unix(1710000000, 0).UTC()
	mock.ExpectQuery(`(?s)SELECT .*FROM "?datasets"? AS d.*d\.workspace_id IN.*ORDER BY d\.updated_at DESC.*LIMIT`).
		WithArgs("ws-knowledge", 3).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"title",
			"resource_id",
			"workspace_id",
			"workspace_name",
			"updated_at",
			"created_at",
		}).AddRow("dataset-1", "Dataset One", "dataset-1", "ws-knowledge", "Knowledge Space", updatedAt, updatedAt))

	items := svc.getRecentDatasets(context.Background(), []string{"ws-knowledge"}, "acc-1", 3)

	require.Len(t, items, 1)
	require.Equal(t, "dataset", items[0].Type)
	require.Equal(t, "dataset:dataset-1", items[0].ID)
	require.Equal(t, "dataset-1", items[0].ResourceID)
	require.Equal(t, "ws-knowledge", items[0].WorkspaceID)
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
