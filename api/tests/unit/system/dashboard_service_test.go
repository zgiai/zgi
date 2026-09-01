package system_test

import (
	"context"
	"errors"
	"regexp"
	"sync"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	llmmodelservice "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
	systemmodel "github.com/zgiai/zgi/api/internal/modules/system/model"
	"github.com/zgiai/zgi/api/internal/modules/system/service"
	redisutil "github.com/zgiai/zgi/api/pkg/redis"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func setupMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}

	dialector := postgres.New(postgres.Config{
		Conn:       sqlDB,
		DriverName: "postgres",
	})

	db, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open gorm db: %v", err)
	}

	cleanup := func() {
		sqlDB.Close()
	}

	return db, mock, cleanup
}

type fakeAvailableModelsLister struct {
	models    []*llmmodelservice.AvailableModel
	lastOrgID uuid.UUID
	calls     int
	err       error
}

func (f *fakeAvailableModelsLister) ListAvailable(
	_ context.Context,
	organizationID uuid.UUID,
	_ string,
	_ string,
) ([]*llmmodelservice.AvailableModel, error) {
	f.calls++
	f.lastOrgID = organizationID
	return f.models, f.err
}

type blockingAvailableModelsLister struct {
	models  []*llmmodelservice.AvailableModel
	started chan struct{}
	release chan struct{}

	mu    sync.Mutex
	calls int
}

func (f *blockingAvailableModelsLister) ListAvailable(
	_ context.Context,
	_ uuid.UUID,
	_ string,
	_ string,
) ([]*llmmodelservice.AvailableModel, error) {
	f.mu.Lock()
	f.calls++
	if f.calls == 1 {
		close(f.started)
	}
	f.mu.Unlock()

	<-f.release
	return f.models, nil
}

func (f *blockingAvailableModelsLister) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func TestDashboardService_CachesStatsResponses(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	redisServer := miniredis.RunT(t)
	redisClient := goredis.NewClient(&goredis.Options{Addr: redisServer.Addr()})
	defer redisClient.Close()
	previousRedis := redisutil.GetClient()
	redisutil.SetClient(redisClient)
	defer redisutil.SetClient(previousRedis)

	orgID := uuid.New()
	availableModels := &fakeAvailableModelsLister{models: []*llmmodelservice.AvailableModel{{
		ID:       uuid.New(),
		Name:     "gpt-4o",
		Provider: "openai",
		UseCases: []string{"text-chat"},
	}}}
	svc := service.NewDashboardServiceWithAvailableModels(db, availableModels)
	scopes := systemmodel.DashboardWorkspaceScopes{WorkspaceIDs: []string{"workspace-1"}}
	tableExistsQuery := `SELECT EXISTS`
	mock.ExpectQuery(tableExistsQuery).
		WithArgs("chat_runtime_conversations").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`(?s)SELECT count\(\*\) FROM "?chat_runtime_conversations"?.*dialogue_count > 0.*workspace_id IN.*workspace_id IS NULL`).
		WithArgs(orgID.String(), "account-1", "console", "aichat", "chat", "workspace-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(tableExistsQuery).
		WithArgs("agents").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(tableExistsQuery).
		WithArgs("datasets").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`(?s)SELECT count\(\*\) FROM "?chat_runtime_conversations"?.*dialogue_count > 0.*workspace_id IN.*workspace_id IS NULL`).
		WithArgs(orgID.String(), "account-1", "console", "aichat", "chat", "workspace-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	first, err := svc.GetDashboardStats(context.Background(), orgID.String(), "account-1", scopes)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), first.Models.Total)
	assert.Equal(t, int64(1), first.Activity.DirectConversations)

	second, err := svc.GetDashboardStats(context.Background(), orgID.String(), "account-1", scopes)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), second.Models.Total)
	assert.Equal(t, int64(2), second.Activity.DirectConversations)
	assert.Equal(t, 1, availableModels.calls)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardService_RecentWorkDoesNotUseRedisCache(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	redisServer := miniredis.RunT(t)
	redisClient := goredis.NewClient(&goredis.Options{Addr: redisServer.Addr()})
	defer redisClient.Close()
	previousRedis := redisutil.GetClient()
	redisutil.SetClient(redisClient)
	defer redisutil.SetClient(previousRedis)

	svc := service.NewDashboardService(db)
	tableExistsQuery := `SELECT EXISTS`
	mock.ExpectQuery(tableExistsQuery).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	request := systemmodel.RecentWorkRequest{OrganizationID: "organization-1", AccountID: "account-1", Limit: 10, AgentWorkspaceIDs: []string{"workspace-1"}}
	first, err := svc.GetRecentWork(context.Background(), request)
	assert.NoError(t, err)
	assert.Empty(t, first.Items)

	second, err := svc.GetRecentWork(context.Background(), request)
	assert.NoError(t, err)
	assert.Empty(t, second.Items)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardService_DoesNotCacheDegradedStats(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	redisServer := miniredis.RunT(t)
	redisClient := goredis.NewClient(&goredis.Options{Addr: redisServer.Addr()})
	defer redisClient.Close()
	previousRedis := redisutil.GetClient()
	redisutil.SetClient(redisClient)
	defer redisutil.SetClient(previousRedis)

	orgID := uuid.New()
	availableModels := &fakeAvailableModelsLister{err: errors.New("available models unavailable")}
	svc := service.NewDashboardServiceWithAvailableModels(db, availableModels)
	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs("chat_runtime_conversations").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	first, err := svc.GetDashboardStats(context.Background(), orgID.String(), "account-1", systemmodel.DashboardWorkspaceScopes{})
	assert.NoError(t, err)
	assert.Zero(t, first.Models.Total)

	availableModels.err = nil
	availableModels.models = []*llmmodelservice.AvailableModel{{
		ID:       uuid.New(),
		Name:     "gpt-4o",
		Provider: "openai",
		UseCases: []string{"text-chat"},
	}}
	second, err := svc.GetDashboardStats(context.Background(), orgID.String(), "account-1", systemmodel.DashboardWorkspaceScopes{})
	assert.NoError(t, err)
	assert.Equal(t, int64(1), second.Models.Total)

	_, err = svc.GetDashboardStats(context.Background(), orgID.String(), "account-1", systemmodel.DashboardWorkspaceScopes{})
	assert.NoError(t, err)
	assert.Equal(t, 2, availableModels.calls)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardService_DoesNotCacheDegradedResourceStats(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	redisServer := miniredis.RunT(t)
	redisClient := goredis.NewClient(&goredis.Options{Addr: redisServer.Addr()})
	defer redisClient.Close()
	previousRedis := redisutil.GetClient()
	redisutil.SetClient(redisClient)
	defer redisutil.SetClient(previousRedis)

	orgID := uuid.New()
	availableModels := &fakeAvailableModelsLister{models: []*llmmodelservice.AvailableModel{{
		ID:       uuid.New(),
		Name:     "gpt-4o",
		Provider: "openai",
		UseCases: []string{"text-chat"},
	}}}
	svc := service.NewDashboardServiceWithAvailableModels(db, availableModels)
	scopes := systemmodel.DashboardWorkspaceScopes{AgentWorkspaceIDs: []string{"workspace-1"}}

	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs("agents").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`(?s)SELECT count\(\*\) FROM "?agents"?.*tenant_id IN.*agent_type`).
		WithArgs(false, false, "workspace-1", "AGENT").
		WillReturnError(errors.New("agents count unavailable"))
	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs("chat_runtime_conversations").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`(?s)SELECT count\(\*\) FROM "?agents"?.*tenant_id IN.*agent_type`).
		WithArgs(false, false, "workspace-1", "AGENT").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(4))

	first, err := svc.GetDashboardStats(context.Background(), orgID.String(), "account-1", scopes)
	assert.NoError(t, err)
	assert.Zero(t, first.Resources.Agents)

	second, err := svc.GetDashboardStats(context.Background(), orgID.String(), "account-1", scopes)
	assert.NoError(t, err)
	assert.Equal(t, int64(4), second.Resources.Agents)

	third, err := svc.GetDashboardStats(context.Background(), orgID.String(), "account-1", scopes)
	assert.NoError(t, err)
	assert.Equal(t, int64(4), third.Resources.Agents)
	assert.Equal(t, 2, availableModels.calls)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardService_RetriesFailedResourceTableProbeBeforeCaching(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	redisServer := miniredis.RunT(t)
	redisClient := goredis.NewClient(&goredis.Options{Addr: redisServer.Addr()})
	defer redisClient.Close()
	previousRedis := redisutil.GetClient()
	redisutil.SetClient(redisClient)
	defer redisutil.SetClient(previousRedis)

	orgID := uuid.New()
	availableModels := &fakeAvailableModelsLister{models: []*llmmodelservice.AvailableModel{{
		ID:       uuid.New(),
		Name:     "gpt-4o",
		Provider: "openai",
		UseCases: []string{"text-chat"},
	}}}
	svc := service.NewDashboardServiceWithAvailableModels(db, availableModels)
	scopes := systemmodel.DashboardWorkspaceScopes{AgentWorkspaceIDs: []string{"workspace-1"}}

	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs("agents").
		WillReturnError(errors.New("table probe unavailable"))
	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs("chat_runtime_conversations").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs("agents").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`(?s)SELECT count\(\*\) FROM "?agents"?.*tenant_id IN.*agent_type`).
		WithArgs(false, false, "workspace-1", "AGENT").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	first, err := svc.GetDashboardStats(context.Background(), orgID.String(), "account-1", scopes)
	assert.NoError(t, err)
	assert.Zero(t, first.Resources.Agents)

	second, err := svc.GetDashboardStats(context.Background(), orgID.String(), "account-1", scopes)
	assert.NoError(t, err)
	assert.Equal(t, int64(3), second.Resources.Agents)

	third, err := svc.GetDashboardStats(context.Background(), orgID.String(), "account-1", scopes)
	assert.NoError(t, err)
	assert.Equal(t, int64(3), third.Resources.Agents)
	assert.Equal(t, 2, availableModels.calls)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardService_CachesMissingResourceTableAsHealthyZero(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	redisServer := miniredis.RunT(t)
	redisClient := goredis.NewClient(&goredis.Options{Addr: redisServer.Addr()})
	defer redisClient.Close()
	previousRedis := redisutil.GetClient()
	redisutil.SetClient(redisClient)
	defer redisutil.SetClient(previousRedis)

	orgID := uuid.New()
	availableModels := &fakeAvailableModelsLister{models: []*llmmodelservice.AvailableModel{{
		ID:       uuid.New(),
		Name:     "gpt-4o",
		Provider: "openai",
		UseCases: []string{"text-chat"},
	}}}
	svc := service.NewDashboardServiceWithAvailableModels(db, availableModels)
	scopes := systemmodel.DashboardWorkspaceScopes{AgentWorkspaceIDs: []string{"workspace-1"}}

	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs("agents").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs("chat_runtime_conversations").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	first, err := svc.GetDashboardStats(context.Background(), orgID.String(), "account-1", scopes)
	assert.NoError(t, err)
	assert.Zero(t, first.Resources.Agents)

	second, err := svc.GetDashboardStats(context.Background(), orgID.String(), "account-1", scopes)
	assert.NoError(t, err)
	assert.Zero(t, second.Resources.Agents)
	assert.Equal(t, 1, availableModels.calls)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardService_RetriesDegradedGlobalModelStatsBeforeCaching(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	redisServer := miniredis.RunT(t)
	redisClient := goredis.NewClient(&goredis.Options{Addr: redisServer.Addr()})
	defer redisClient.Close()
	previousRedis := redisutil.GetClient()
	redisutil.SetClient(redisClient)
	defer redisutil.SetClient(previousRedis)

	svc := service.NewDashboardService(db)
	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs("llm_models").
		WillReturnError(errors.New("model table probe unavailable"))
	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs("chat_runtime_conversations").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs("llm_models").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "llm_models" WHERE is_active = $1 AND deleted_at IS NULL`)).
		WithArgs(true).
		WillReturnError(errors.New("model count unavailable"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "llm_models" WHERE is_active = $1 AND deleted_at IS NULL`)).
		WithArgs(true).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))
	mock.ExpectQuery(`SELECT unnest.*use_cases.*AS use_case`).
		WillReturnRows(sqlmock.NewRows([]string{"use_case", "count"}).AddRow("text-chat", 5))

	first, err := svc.GetDashboardStats(context.Background(), "organization-1", "account-1", systemmodel.DashboardWorkspaceScopes{})
	assert.NoError(t, err)
	assert.Zero(t, first.Models.Total)

	second, err := svc.GetDashboardStats(context.Background(), "organization-1", "account-1", systemmodel.DashboardWorkspaceScopes{})
	assert.NoError(t, err)
	assert.Zero(t, second.Models.Total)

	third, err := svc.GetDashboardStats(context.Background(), "organization-1", "account-1", systemmodel.DashboardWorkspaceScopes{})
	assert.NoError(t, err)
	assert.Equal(t, int64(5), third.Models.Total)

	fourth, err := svc.GetDashboardStats(context.Background(), "organization-1", "account-1", systemmodel.DashboardWorkspaceScopes{})
	assert.NoError(t, err)
	assert.Equal(t, int64(5), fourth.Models.Total)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardService_CoalescesConcurrentStatsCacheMisses(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	redisServer := miniredis.RunT(t)
	redisClient := goredis.NewClient(&goredis.Options{Addr: redisServer.Addr()})
	defer redisClient.Close()
	previousRedis := redisutil.GetClient()
	redisutil.SetClient(redisClient)
	defer redisutil.SetClient(previousRedis)

	lister := &blockingAvailableModelsLister{
		models: []*llmmodelservice.AvailableModel{{
			ID:       uuid.New(),
			Name:     "gpt-4o",
			Provider: "openai",
			UseCases: []string{"text-chat"},
		}},
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	svc := service.NewDashboardServiceWithAvailableModels(db, lister)
	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs("chat_runtime_conversations").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	const callers = 6
	results := make(chan error, callers)
	for range callers {
		go func() {
			_, err := svc.GetDashboardStats(context.Background(), uuid.Nil.String(), "account-1", systemmodel.DashboardWorkspaceScopes{})
			results <- err
		}()
	}

	<-lister.started
	close(lister.release)
	for range callers {
		assert.NoError(t, <-results)
	}

	assert.Equal(t, 1, lister.Calls())
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardService_GetDashboardStats_UsesAvailableModels(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	orgID := uuid.New()
	availableModels := &fakeAvailableModelsLister{
		models: []*llmmodelservice.AvailableModel{
			{
				ID:       uuid.New(),
				Name:     "gpt-4o",
				Provider: "openai",
				UseCases: []string{"text-chat", "vision"},
			},
			{
				ID:       uuid.New(),
				Name:     "tenant-custom-chat",
				Provider: "tenant-openai",
				UseCases: []string{"text-chat"},
			},
		},
	}
	svc := service.NewDashboardServiceWithAvailableModels(db, availableModels)
	ctx := context.Background()
	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs("chat_runtime_conversations").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	stats, err := svc.GetDashboardStats(ctx, orgID.String(), "account-1", systemmodel.DashboardWorkspaceScopes{})

	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Equal(t, orgID, availableModels.lastOrgID)
	assert.Equal(t, int64(2), stats.Models.Total)
	assert.Equal(t, int64(2), stats.Models.ByUseCase["text-chat"])
	assert.Equal(t, int64(1), stats.Models.ByUseCase["vision"])
	assert.Equal(t, int64(0), stats.Resources.Agents)
	assert.Equal(t, int64(0), stats.Resources.Datasets)
	assert.Equal(t, int64(0), stats.Resources.DataSources)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardService_GetDashboardStats(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	svc := service.NewDashboardService(db)
	ctx := context.Background()
	orgID := "test-org-id"

	tableExistsQuery := `SELECT EXISTS`

	// tableExists: llm_models
	mock.ExpectQuery(tableExistsQuery).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	// 1. Model stats - total (safeCount also calls tableExists, but it's cached)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "llm_models" WHERE is_active = $1 AND deleted_at IS NULL`)).
		WithArgs(true).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(88))

	// 2. Model stats - by use_case (unnest)
	mock.ExpectQuery(`SELECT unnest.*use_cases.*AS use_case`).
		WillReturnRows(sqlmock.NewRows([]string{"use_case", "count"}).
			AddRow("text-chat", 70).
			AddRow("embedding", 12).
			AddRow("rerank", 6))

	// tableExists: agents
	mock.ExpectQuery(tableExistsQuery).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	// 4. Agents count
	mock.ExpectQuery(`SELECT count\(\*\) FROM "agents" WHERE .*tenant_id IN \(\$3,\$4\) AND agent_type = \$5`).
		WithArgs(false, false, "ws-1", "ws-2", "AGENT").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))

	// tableExists: datasets
	mock.ExpectQuery(tableExistsQuery).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	// 5. Datasets count
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "datasets" WHERE workspace_id IN ($1,$2)`)).
		WithArgs("ws-1", "ws-2").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	// tableExists: data_sources
	mock.ExpectQuery(tableExistsQuery).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	// 6. Data sources count
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "data_sources" WHERE organization_id = $1 AND workspace_id IN ($2,$3)`)).
		WithArgs(orgID, "ws-1", "ws-2").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	// Activity is refreshed separately from the cached model/resource stats.
	mock.ExpectQuery(tableExistsQuery).
		WithArgs("chat_runtime_conversations").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`(?s)SELECT count\(\*\) FROM "agents" WHERE .*created_by`).
		WithArgs("ws-1", "ws-2", "account-1", "AGENT", false, false).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`(?s)SELECT count\(\*\) FROM "datasets" WHERE .*created_by`).
		WithArgs("ws-1", "ws-2", "account-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	// Execute
	stats, err := svc.GetDashboardStats(ctx, orgID, "account-1", systemmodel.DashboardWorkspaceScopes{
		WorkspaceIDs:           []string{"ws-1", "ws-2"},
		AgentWorkspaceIDs:      []string{"ws-1", "ws-2"},
		DatasetWorkspaceIDs:    []string{"ws-1", "ws-2"},
		DataSourceWorkspaceIDs: []string{"ws-1", "ws-2"},
	})

	assert.NoError(t, err)
	assert.NotNil(t, stats)

	// Models
	assert.Equal(t, int64(88), stats.Models.Total)
	assert.Equal(t, int64(70), stats.Models.ByUseCase["text-chat"])
	assert.Equal(t, int64(12), stats.Models.ByUseCase["embedding"])
	assert.Equal(t, int64(6), stats.Models.ByUseCase["rerank"])

	// Resources
	assert.Equal(t, int64(5), stats.Resources.Agents)
	assert.Equal(t, int64(3), stats.Resources.Datasets)
	assert.Equal(t, int64(2), stats.Resources.DataSources)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardService_GetDashboardStats_EmptyOrg(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	svc := service.NewDashboardService(db)
	ctx := context.Background()
	orgID := "empty-org-id"

	tableExistsQuery := `SELECT EXISTS`

	// tableExists: llm_models
	mock.ExpectQuery(tableExistsQuery).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	// Models - total
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "llm_models" WHERE is_active = $1 AND deleted_at IS NULL`)).
		WithArgs(true).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	// Models - by use_case (empty)
	mock.ExpectQuery(`SELECT unnest.*use_cases.*AS use_case`).
		WillReturnRows(sqlmock.NewRows([]string{"use_case", "count"}))
	mock.ExpectQuery(tableExistsQuery).
		WithArgs("chat_runtime_conversations").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	stats, err := svc.GetDashboardStats(ctx, orgID, "account-1", systemmodel.DashboardWorkspaceScopes{})

	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Equal(t, int64(0), stats.Models.Total)
	assert.Equal(t, int64(0), stats.Resources.Agents)
	assert.Equal(t, int64(0), stats.Resources.Datasets)
	assert.Equal(t, int64(0), stats.Resources.DataSources)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardService_GetDashboardStats_DatabaseError(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	svc := service.NewDashboardService(db)
	ctx := context.Background()
	orgID := "error-org-id"

	tableExistsQuery := `SELECT EXISTS`

	// tableExists: llm_models
	mock.ExpectQuery(tableExistsQuery).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	// Model stats - total
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "llm_models" WHERE is_active = $1 AND deleted_at IS NULL`)).
		WithArgs(true).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	// Model stats - by use_case (empty)
	mock.ExpectQuery(`SELECT unnest.*use_cases.*AS use_case`).
		WillReturnRows(sqlmock.NewRows([]string{"use_case", "count"}))
	mock.ExpectQuery(tableExistsQuery).
		WithArgs("chat_runtime_conversations").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	stats, err := svc.GetDashboardStats(ctx, orgID, "account-1", systemmodel.DashboardWorkspaceScopes{})

	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Equal(t, int64(0), stats.Models.Total)
	assert.NoError(t, mock.ExpectationsWereMet())
}
