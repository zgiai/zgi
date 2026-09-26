package indexing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/capabilities/text2sql"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	defaults "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	workspace "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/pkg/queue"
	"github.com/zgiai/zgi/api/pkg/sql_base"
	"github.com/zgiai/zgi/api/pkg/vectordb"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type testModels struct {
	defaults.DefaultModelService
	model string
}

func (m *testModels) ResolveUseCase(context.Context, string, llmmodel.UseCase, *string, *string) (*defaults.ResolvedModel, error) {
	return &defaults.ResolvedModel{Provider: "test", Model: m.model}, nil
}

type testAccess struct {
	deny     bool
	viewOnly bool
}

func (a *testAccess) CheckWorkspaceOrganizationAnyPermission(_ context.Context, _, _, _ string, permissions ...workspace.WorkspacePermissionCode) (bool, error) {
	if a.deny {
		return false, nil
	}
	if !a.viewOnly {
		return true, nil
	}
	for _, permission := range permissions {
		if permission == workspace.WorkspacePermissionDatabaseSchemaView {
			return true, nil
		}
	}
	return false, nil
}

func TestAuthorizeWorkspaceUsesExistingWorkspacePermissions(t *testing.T) {
	viewer := &Service{access: &testAccess{viewOnly: true}}
	if err := viewer.authorizeWorkspace(context.Background(), "org-1", "workspace-1", "account-1", false); err != nil {
		t.Fatalf("viewer access error = %v, want nil", err)
	}
	if err := viewer.authorizeWorkspace(context.Background(), "org-1", "workspace-1", "account-1", true); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("viewer manage error = %v, want ErrAccessDenied", err)
	}

	missingChecker := &Service{}
	if err := missingChecker.authorizeWorkspace(context.Background(), "org-1", "workspace-1", "account-1", false); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("missing checker error = %v, want ErrAccessDenied", err)
	}
}

type testSQL struct {
	sql_base.SQLBase
	tables map[int]*sql_base.Table
}

func (s testSQL) GetTable(_ context.Context, id int) (*sql_base.Table, error) {
	table, ok := s.tables[id]
	if !ok {
		return nil, errors.New("table gone")
	}
	return table, nil
}

type testLLM struct {
	llmclient.LLMClient
	calls      atomic.Int32
	fail       bool
	badBilling atomic.Bool
}

func (l *testLLM) AppEmbed(_ context.Context, app *llmclient.AppContext, _ *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	l.calls.Add(1)
	if app.BillingSubjectType != llmclient.BillingSubjectTypeWorkspace || app.WorkspaceID == "" || app.AccountID == "" || app.WorkflowID != "" {
		l.badBilling.Store(true)
	}
	if l.fail {
		return nil, context.DeadlineExceeded
	}
	return &adapter.EmbeddingsResponse{Data: []adapter.Embedding{{Embedding: []float32{0.1, 0.2}}}}, nil
}

type fixture struct {
	service *Service
	setting Settings
	llm     *testLLM
	models  *testModels
	access  *testAccess
}

func newFixture(t *testing.T, n int) fixture {
	t.Helper()
	dsn := os.Getenv("ZGI_SCHEMA_INDEX_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_SCHEMA_INDEX_TEST_DSN to an isolated PostgreSQL database (keyword DSN)")
	}
	root, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	ns := "schema_index_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := root.Exec("CREATE SCHEMA " + ns).Error; err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(postgres.Open(dsn+" search_path="+ns), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		conn, _ := db.DB()
		conn.Close()
		root.Exec("DROP SCHEMA " + ns + " CASCADE")
		conn, _ = root.DB()
		conn.Close()
	})
	if err := db.AutoMigrate(&Settings{}, &Run{}, &text2sql.SchemaIndex{}); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		"ALTER TABLE text2sql_schema_indexes ADD build_attempts integer NOT NULL DEFAULT 0",
		"CREATE UNIQUE INDEX identity_unique ON text2sql_schema_indexes(workspace_id,data_source_id,asset_table_id,fingerprint,embedding_provider,embedding_model)",
		"CREATE TABLE data_sources(id uuid PRIMARY KEY,organization_id uuid,workspace_id uuid)",
		"CREATE TABLE data_source_tables(id uuid PRIMARY KEY,organization_id uuid,data_source_id uuid,table_id integer,name text NOT NULL,description text NOT NULL)",
	} {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	setting := Settings{DataSourceID: uuid.NewString(), OrganizationID: uuid.NewString(), WorkspaceID: uuid.NewString(), RequestedBy: uuid.NewString()}
	if err := db.Exec("INSERT INTO data_sources VALUES(?,?,?)", setting.DataSourceID, setting.OrganizationID, setting.WorkspaceID).Error; err != nil {
		t.Fatal(err)
	}
	catalog := testSQL{tables: map[int]*sql_base.Table{}}
	for i := 1; i <= n; i++ {
		catalog.tables[i] = &sql_base.Table{ID: i, Schema: "public", Name: fmt.Sprintf("table_%03d", i)}
		if err := db.Exec("INSERT INTO data_source_tables VALUES(?,?,?,?,?,?)", uuid.NewString(), setting.OrganizationID, setting.DataSourceID, i, fmt.Sprintf("Table %03d", i), fmt.Sprintf("Description %03d", i)).Error; err != nil {
			t.Fatal(err)
		}
	}
	redis := miniredis.RunT(t)
	host, port, _ := net.SplitHostPort(redis.Addr())
	portNumber, _ := strconv.Atoi(port)
	cfg := &config.Config{}
	cfg.Redis.Host = host
	cfg.Redis.Port = portNumber
	cfg.TaskQueue.Concurrency = 2
	manager, err := queue.NewTaskManager(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { manager.Close() })
	llm, models, access := &testLLM{}, &testModels{model: "embedding-v1"}, &testAccess{}
	service := NewService(db, catalog, models, llm, &vectordb.MockVectorDB{}, manager, access, zap.NewNop())
	return fixture{service, setting, llm, models, access}
}
func (f fixture) start(t *testing.T) {
	t.Helper()
	if err := f.service.Start(context.Background(), f.setting.OrganizationID, f.setting.DataSourceID, f.setting.RequestedBy); err != nil {
		t.Fatal(err)
	}
	if err := f.service.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
}
func (f fixture) rows(t *testing.T) []text2sql.SchemaIndex {
	t.Helper()
	var setting Settings
	if err := f.service.db.First(&setting).Error; err != nil {
		t.Fatal(err)
	}
	var ids []string
	json.Unmarshal(setting.TargetIDs, &ids)
	var rows []text2sql.SchemaIndex
	if len(ids) > 0 {
		if err := f.service.db.Where("id IN ?", ids).Find(&rows).Error; err != nil {
			t.Fatal(err)
		}
	}
	return rows
}
func indexTask(row text2sql.SchemaIndex) *asynq.Task {
	data, _ := json.Marshal(taskPayload{row.ID, row.AssetTableID})
	return asynq.NewTask(taskType, data)
}
func (f fixture) status(t *testing.T) *Status {
	t.Helper()
	status, err := f.service.Status(context.Background(), f.setting.OrganizationID, f.setting.DataSourceID, f.setting.RequestedBy)
	if err != nil {
		t.Fatal(err)
	}
	return status
}

func TestAutomaticPendingTriggerPreservesInitialBuildAndDistinguishesLaterCreates(t *testing.T) {
	got, err := automaticPendingTrigger(Settings{TargetIDs: []byte("[]")}, TriggerTableCreated)
	if err != nil {
		t.Fatal(err)
	}
	if got != TriggerInitial {
		t.Fatalf("first table trigger = %q, want %q", got, TriggerInitial)
	}

	pending := TriggerTableCreated
	got, err = automaticPendingTrigger(Settings{TargetIDs: []byte(`["existing"]`), PendingTrigger: &pending}, TriggerSchemaChanged)
	if err != nil {
		t.Fatal(err)
	}
	if got != TriggerTableCreated {
		t.Fatalf("coalesced trigger = %q, want %q", got, TriggerTableCreated)
	}

	got, err = automaticPendingTrigger(Settings{TargetIDs: []byte(`["existing"]`)}, TriggerTableCreated)
	if err != nil {
		t.Fatal(err)
	}
	if got != TriggerTableCreated {
		t.Fatalf("later table trigger = %q, want %q", got, TriggerTableCreated)
	}
}

func TestReconcileDataSourceRecordsFailureBeforeUnlock(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, PreferSimpleProtocol: true}), &gorm.Config{SkipDefaultTransaction: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}

	runID := uuid.NewString()
	dataSourceID := uuid.NewString()
	pendingTrigger := TriggerTableMetadataChanged
	now := time.Now()
	reconcileErr := errors.New("database unavailable")
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_lock(hashtextextended($1, 0))`)).
		WithArgs("schema-index:database:" + dataSourceID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "database_schema_index_settings"`)).WillReturnRows(sqlmock.NewRows([]string{
		"data_source_id", "organization_id", "workspace_id", "requested_by", "embedding_provider", "embedding_model", "target_ids", "pending_run_id", "pending_trigger", "created_at", "updated_at",
	}).AddRow(dataSourceID, uuid.NewString(), uuid.NewString(), uuid.NewString(), "test", "embedding-v1", []byte("[]"), runID, pendingTrigger, now, now))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "data_sources"`)).WillReturnError(reconcileErr)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "database_schema_index_runs"`)).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "database_schema_index_runs"`)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "database_schema_index_settings" SET .*"pending_run_id"=.*"pending_trigger"=`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_unlock(hashtextextended($1, 0))`)).
		WithArgs("schema-index:database:" + dataSourceID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	service := &Service{db: db, log: zap.NewNop()}
	if err := service.ReconcileDataSource(context.Background(), dataSourceID); !errors.Is(err, reconcileErr) {
		t.Fatalf("ReconcileDataSource() error = %v, want %v", err, reconcileErr)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateTaskUsesMetadataEnrichedTableSnapshot(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, PreferSimpleProtocol: true}), &gorm.Config{SkipDefaultTransaction: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}

	setting := Settings{
		DataSourceID: uuid.NewString(), OrganizationID: uuid.NewString(), WorkspaceID: uuid.NewString(), RequestedBy: uuid.NewString(),
		EmbeddingProvider: "test", EmbeddingModel: "embedding-v1",
	}
	assetID := uuid.NewString()
	physical := &sql_base.Table{ID: 1, Schema: "public", Name: "physical_table"}
	catalog := testSQL{tables: map[int]*sql_base.Table{1: physical}}
	assetRows := func() *sqlmock.Rows {
		return sqlmock.NewRows([]string{"id", "table_id", "name", "description"}).AddRow(assetID, 1, "Business name", "Business description")
	}
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, table_id, name, description FROM "data_source_tables"`)).WillReturnRows(assetRows())
	snapshot, err := text2sql.NewIndexRepository(db).SnapshotTable(context.Background(), assetID, catalog)
	if err != nil {
		t.Fatal(err)
	}
	row := text2sql.NewSchemaIndex(setting.OrganizationID, setting.WorkspaceID, setting.DataSourceID, assetID, snapshot.Table, setting.EmbeddingProvider, setting.EmbeddingModel)
	setting.TargetIDs, err = json.Marshal([]string{row.ID})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "database_schema_index_settings"`)).WillReturnRows(sqlmock.NewRows([]string{
		"data_source_id", "organization_id", "workspace_id", "requested_by", "embedding_provider", "embedding_model", "target_ids", "created_at", "updated_at",
	}).AddRow(setting.DataSourceID, setting.OrganizationID, setting.WorkspaceID, setting.RequestedBy, setting.EmbeddingProvider, setting.EmbeddingModel, setting.TargetIDs, now, now))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "data_sources"`)).WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "workspace_id"}).AddRow(setting.DataSourceID, setting.OrganizationID, setting.WorkspaceID))
	mock.ExpectQuery(`SELECT (\*|id, table_id, name, description) FROM "data_source_tables"`).WillReturnRows(assetRows())

	service := NewService(db, catalog, &testModels{model: setting.EmbeddingModel}, nil, nil, nil, &testAccess{}, zap.NewNop())
	if err := service.validateTask(context.Background(), setting, row); err != nil {
		t.Fatalf("validateTask() error = %v, want nil", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresTwoHundredTablesResumeAndReuse(t *testing.T) {
	f := newFixture(t, 200)
	f.start(t)
	rows := f.rows(t)
	if len(rows) != 200 {
		t.Fatalf("got %d targets", len(rows))
	}
	for _, row := range rows[:80] {
		if err := f.service.handleTask(context.Background(), indexTask(row)); err != nil {
			t.Fatal(err)
		}
	}
	if status := f.status(t); status.Ready != 80 || status.State == "ready" {
		t.Fatalf("partial state: %+v", status)
	}
	// A fresh service object has no in-memory progress or ownership.
	if err := f.service.db.Table("text2sql_schema_indexes").Where("id = ?", rows[80].ID).Updates(map[string]any{"status": text2sql.IndexIndexing, "build_attempts": 1}).Error; err != nil {
		t.Fatal(err)
	}
	restarted := *f.service
	f.service = &restarted
	for _, row := range rows {
		if err := f.service.handleTask(context.Background(), indexTask(row)); err != nil {
			t.Fatal(err)
		}
	}
	if status := f.status(t); status.Ready != 200 || status.State != "ready" {
		t.Fatalf("complete state: %+v", status)
	}
	if f.llm.calls.Load() != 200 || f.llm.badBilling.Load() {
		t.Fatalf("calls=%d invalid billing=%v", f.llm.calls.Load(), f.llm.badBilling.Load())
	}
	page, err := f.service.History(context.Background(), f.setting.OrganizationID, f.setting.DataSourceID, f.setting.RequestedBy, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].Status != "succeeded" || page.Items[0].TableCount != 200 {
		t.Fatalf("unexpected history: %+v", page)
	}
}

func TestPostgresDuplicateDeliveryBuildsOnce(t *testing.T) {
	f := newFixture(t, 1)
	f.start(t)
	row := f.rows(t)[0]
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- f.service.handleTask(context.Background(), indexTask(row)) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if f.llm.calls.Load() != 1 {
		t.Fatalf("duplicate calls: %d", f.llm.calls.Load())
	}
}

func TestPostgresTimeoutRetryBudgetAndManualRetry(t *testing.T) {
	f := newFixture(t, 1)
	f.start(t)
	row := f.rows(t)[0]
	f.llm.fail = true
	for range MaxAttempts {
		if err := f.service.handleTask(context.Background(), indexTask(row)); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(err)
		}
	}
	if err := f.service.handleTask(context.Background(), indexTask(row)); err != nil {
		t.Fatal(err)
	}
	if f.llm.calls.Load() != MaxAttempts {
		t.Fatal("retry budget ignored")
	}
	if status := f.status(t); status.State != "failed" || status.Failed[0].Code != "timeout" {
		t.Fatalf("%+v", status)
	}
	f.llm.fail = false
	f.start(t)
	if err := f.service.handleTask(context.Background(), indexTask(row)); err != nil {
		t.Fatal(err)
	}
	if f.status(t).State != "ready" {
		t.Fatal("manual retry did not finish")
	}
	page, err := f.service.History(context.Background(), f.setting.OrganizationID, f.setting.DataSourceID, f.setting.RequestedBy, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || page.Items[0].Trigger != TriggerManualRetry || page.Items[0].Status != "succeeded" || page.Items[1].Status != "failed" {
		t.Fatalf("unexpected retry history: %+v", page.Items)
	}
	firstPage, err := f.service.History(context.Background(), f.setting.OrganizationID, f.setting.DataSourceID, f.setting.RequestedBy, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(firstPage.Items) != 1 || !firstPage.HasMore {
		t.Fatalf("unexpected pagination: %+v", firstPage)
	}
}

func TestPostgresPermissionModelChangeAndMove(t *testing.T) {
	f := newFixture(t, 1)
	f.access.viewOnly = true
	if err := f.service.Start(context.Background(), f.setting.OrganizationID, f.setting.DataSourceID, f.setting.RequestedBy); !errors.Is(err, ErrAccessDenied) {
		t.Fatal("viewer can build")
	}
	if f.status(t).Enabled {
		t.Fatal("viewer implicitly enabled indexing")
	}
	f.access.viewOnly = false
	f.start(t)
	old := f.rows(t)[0]
	f.models.model = "embedding-v2"
	if err := f.service.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	current := f.rows(t)[0]
	if current.ID == old.ID {
		t.Fatal("model identity reused")
	}
	if err := f.service.handleTask(context.Background(), indexTask(old)); err != nil {
		t.Fatal(err)
	}
	if f.llm.calls.Load() != 0 {
		t.Fatal("obsolete task billed")
	}
	f.service.db.Exec("UPDATE data_sources SET workspace_id=?", uuid.NewString())
	if err := f.service.handleTask(context.Background(), indexTask(current)); !errors.Is(err, errTaskScope) {
		t.Fatal(err)
	}
	if err := f.service.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	var count int64
	f.service.db.Model(&Settings{}).Count(&count)
	if count != 0 {
		t.Fatal("moved database remains enabled in old workspace")
	}
}

func TestPostgresTableMetadataChangeRebuildsIndexAndKeepsTrigger(t *testing.T) {
	f := newFixture(t, 1)
	f.start(t)
	before := f.rows(t)[0]

	if err := f.service.db.Table("data_source_tables").Where("data_source_id = ?", f.setting.DataSourceID).Updates(map[string]any{
		"name": "Renamed table", "description": "Updated description",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.service.NotifySchemaChanged(context.Background(), f.setting.DataSourceID, TriggerTableMetadataChanged); err != nil {
		t.Fatal(err)
	}
	if err := f.service.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	after := f.rows(t)[0]
	if after.ID == before.ID {
		t.Fatal("table metadata change reused the previous index")
	}
	if err := f.service.handleTask(context.Background(), indexTask(after)); err != nil {
		t.Fatal(err)
	}
	var run Run
	if err := f.service.db.Where("data_source_id = ? AND trigger = ?", f.setting.DataSourceID, TriggerTableMetadataChanged).Take(&run).Error; err != nil {
		t.Fatal(err)
	}
}

func TestPostgresTableCreateAndDeleteKeepDistinctTriggers(t *testing.T) {
	f := newFixture(t, 1)
	f.start(t)
	assetID := uuid.NewString()
	f.service.sql.(testSQL).tables[2] = &sql_base.Table{ID: 2, Schema: "public", Name: "table_002"}
	if err := f.service.db.Exec("INSERT INTO data_source_tables VALUES(?,?,?,?,?,?)", assetID, f.setting.OrganizationID, f.setting.DataSourceID, 2, "Second table", "Created later").Error; err != nil {
		t.Fatal(err)
	}
	if err := f.service.NotifySchemaChanged(context.Background(), f.setting.DataSourceID, TriggerTableCreated); err != nil {
		t.Fatal(err)
	}
	if err := f.service.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	var created Run
	if err := f.service.db.Where("data_source_id = ? AND trigger = ?", f.setting.DataSourceID, TriggerTableCreated).Take(&created).Error; err != nil {
		t.Fatal(err)
	}
	if created.TableCount != 1 {
		t.Fatalf("created table count = %d, want 1", created.TableCount)
	}

	if err := f.service.db.Exec("DELETE FROM data_source_tables WHERE id = ?", assetID).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.service.NotifySchemaChanged(context.Background(), f.setting.DataSourceID, TriggerTableDeleted); err != nil {
		t.Fatal(err)
	}
	if err := f.service.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	var deleted Run
	if err := f.service.db.Where("data_source_id = ? AND trigger = ?", f.setting.DataSourceID, TriggerTableDeleted).Take(&deleted).Error; err != nil {
		t.Fatal(err)
	}
	if deleted.TableCount != 1 {
		t.Fatalf("deleted table count = %d, want 1", deleted.TableCount)
	}
}
