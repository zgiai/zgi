package indexing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

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
		"CREATE TABLE data_source_tables(id uuid PRIMARY KEY,organization_id uuid,data_source_id uuid,table_id integer)",
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
		if err := db.Exec("INSERT INTO data_source_tables VALUES(?,?,?,?)", uuid.NewString(), setting.OrganizationID, setting.DataSourceID, i).Error; err != nil {
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
	if page.Total != 2 || page.Items[0].Trigger != "manual_retry" || page.Items[0].Status != "succeeded" || page.Items[1].Status != "failed" {
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
