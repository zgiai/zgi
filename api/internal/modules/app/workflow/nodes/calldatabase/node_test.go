package calldatabase

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/databaseauthorization"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/end"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/start"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	"github.com/zgiai/zgi/api/pkg/sql_base"
	"github.com/zgiai/zgi/api/pkg/sql_base/audit"
)

type fakeSQLBase struct {
	sql_base.SQLBase
	auditCtx      *audit.Context
	auditCtxs     []audit.Context
	executionOpts []sql_base.ExecuteSQLOptions
	params        [][]any
	executeFunc   func(ctx context.Context, query string, params []any, auditCtx *audit.Context) (*sql_base.QueryResult, error)
}

type fakeDatabaseAuthorizer struct {
	err             error
	resolveErr      error
	requests        []databaseauthorization.Request
	resolveRequests []databaseauthorization.Request
	resolvedTables  []databaseauthorization.TableReference
}

func (f *fakeDatabaseAuthorizer) Authorize(_ context.Context, req databaseauthorization.Request) error {
	f.requests = append(f.requests, req)
	return f.err
}

func (f *fakeDatabaseAuthorizer) ResolveTables(_ context.Context, req databaseauthorization.Request) ([]databaseauthorization.TableReference, error) {
	f.resolveRequests = append(f.resolveRequests, req)
	if f.resolveErr != nil {
		return nil, f.resolveErr
	}
	if f.resolvedTables != nil {
		return f.resolvedTables, nil
	}
	result := make([]databaseauthorization.TableReference, 0, len(req.Relations))
	for index, relation := range req.Relations {
		result = append(result, databaseauthorization.TableReference{
			ID: fmt.Sprintf("table-%d", index+1), TableID: index + 1,
			Schema: relation.Schema, Name: relation.Name,
		})
	}
	return result, nil
}

func TestExecuteRunResolvesTablesFromSelectedDataSource(t *testing.T) {
	var executedSQL string
	sqlClient := &fakeSQLBase{executeFunc: func(_ context.Context, query string, _ []any, _ *audit.Context) (*sql_base.QueryResult, error) {
		executedSQL = query
		return &sql_base.QueryResult{}, nil
	}}
	authorizer := &fakeDatabaseAuthorizer{resolvedTables: []databaseauthorization.TableReference{{
		ID: "table-1", TableID: 7, Schema: "public", Name: "zgi_base_users",
	}}}
	node := &Node{
		NodeStruct: base.NodeStruct{
			NodeID: "call-db-node", NodeType: shared.CallDatabase,
			OrganizationID: "organization-1", WorkspaceID: "workspace-1", WorkflowID: "workflow-1",
		},
		NodeData:  NodeData{DataSource: DataSourceConfig{ID: "db-1"}, ManualSQL: "SELECT * FROM users"},
		sqlClient: sqlClient, authorizer: authorizer,
	}

	result, err := node.executeRun(context.Background())

	if err != nil || result == nil || result.Status != shared.SUCCEEDED {
		t.Fatalf("result=%#v error=%v, want success", result, err)
	}
	if executedSQL != `SELECT * FROM "public"."zgi_base_users"` {
		t.Fatalf("executed SQL = %q, want resolved physical table", executedSQL)
	}
	if len(authorizer.resolveRequests) != 1 || len(authorizer.requests) != 1 {
		t.Fatalf("resolve/authorize requests = %d/%d, want 1/1", len(authorizer.resolveRequests), len(authorizer.requests))
	}
	if len(authorizer.requests[0].Tables) != 1 || authorizer.requests[0].Tables[0].ID != "table-1" {
		t.Fatalf("authorization tables = %#v, want resolved table", authorizer.requests[0].Tables)
	}
	if sqlClient.auditCtx == nil || sqlClient.auditCtx.TableID != "table-1" || sqlClient.auditCtx.TableName != "zgi_base_users" {
		t.Fatalf("audit context = %#v, want resolved table", sqlClient.auditCtx)
	}
	tableContext, ok := result.Outputs["table_context"].([]TableRef)
	if !ok || len(tableContext) != 1 || tableContext[0].ID != "table-1" {
		t.Fatalf("table context = %#v, want resolved table", result.Outputs["table_context"])
	}
}

func TestExecuteRunDoesNotResolveReadOnlyCTENamesAsTables(t *testing.T) {
	sqlClient := &fakeSQLBase{executeFunc: func(_ context.Context, query string, _ []any, _ *audit.Context) (*sql_base.QueryResult, error) {
		return &sql_base.QueryResult{}, nil
	}}
	authorizer := &fakeDatabaseAuthorizer{}
	node := &Node{
		NodeStruct: base.NodeStruct{
			NodeID: "call-db-node", NodeType: shared.CallDatabase,
			OrganizationID: "organization-1", WorkspaceID: "workspace-1", WorkflowID: "workflow-1",
		},
		NodeData: NodeData{DataSource: DataSourceConfig{ID: "db-1"}, ManualSQL: `WITH totals AS (
			SELECT city, SUM(amount) AS total_amount FROM public.invoice_table GROUP BY city
		) SELECT * FROM totals`},
		sqlClient: sqlClient, authorizer: authorizer,
	}

	result, err := node.executeRun(context.Background())
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, authorizer.resolveRequests, 1)
	require.Equal(t, []databaseauthorization.SQLRelation{{Schema: "public", Name: "invoice_table"}}, authorizer.resolveRequests[0].Relations)
}

func (f *fakeSQLBase) ExecuteSQL(ctx context.Context, query string, params []any, auditCtx *audit.Context) (*sql_base.QueryResult, error) {
	f.auditCtx = auditCtx
	if auditCtx != nil {
		f.auditCtxs = append(f.auditCtxs, *auditCtx)
	}
	if f.executeFunc != nil {
		return f.executeFunc(ctx, query, params, auditCtx)
	}
	return nil, nil
}

func ExampleNode_executeRun() {
	mockClient := &fakeSQLBase{
		executeFunc: func(ctx context.Context, query string, params []any, auditCtx *audit.Context) (*sql_base.QueryResult, error) {
			return &sql_base.QueryResult{
				RowsAffected: 1,
				Columns:      []string{"id", "status"},
				Rows: [][]any{
					{int64(42), "active"},
				},
			}, nil
		},
	}

	node := &Node{
		NodeStruct: base.NodeStruct{
			InstanceID: "demo-instance",
			NodeID:     "call-db-node",
			NodeType:   shared.CallDatabase,
		},
		NodeData: NodeData{
			DataSource: DataSourceConfig{
				ID:   "pg-main",
				Name: "生产库",
				Type: "postgres",
			},
			TableSelection: []TableRef{
				{Schema: "public", Name: "users", Columns: []string{"id", "status"}},
			},
			ManualSQL: "SELECT id, status FROM public.users WHERE status = 'active' LIMIT 1",
			Execution: ExecutionConfig{
				TimeoutSeconds: 10,
				MaxRetries:     0,
			},
		},
		sqlClient: mockClient,
	}

	result, _ := node.executeRun(context.Background())
	fmt.Printf("rows=%v row_count=%v\n", result.Outputs["rows"], result.Outputs["row_count"])
	// Output: rows=[map[id:42 status:active]] row_count=1
}

func TestWorkflowChainStartCallDatabaseEnd(t *testing.T) {
	ctx := context.Background()

	state := entities.NewGraphRuntimeStateWithDefaults()
	state.VariablePool.UserInputs["request"] = "fetch active users"

	initParams := entities.GraphInitParams{
		TenantID:       "workspace-1",
		OrganizationID: "organization-1",
		AppID:          "app-1",
		WorkflowType:   entities.WorkflowTypeWorkflow,
		WorkflowID:     "workflow-1",
		GraphConfig:    map[string]any{},
		UserID:         "user-1",
		UserFrom:       entities.UserFromAccount,
		InvokeFrom:     entities.InvokeFromDebugger,
		CallDepth:      1,
	}
	graph := &entities.Graph{}

	startConfig := map[string]any{
		"id": "start-node",
		"data": map[string]any{
			"variables": []any{},
		},
	}
	startNode, err := start.New("start-instance", startConfig, initParams, graph, state, nil)
	if err != nil {
		t.Fatalf("start.New error: %v", err)
	}
	startResult := runNode(t, ctx, startNode)
	addOutputsToPool(state, "start-node", startResult.Outputs)

	rawSQL := "SELECT id, email FROM public.users LIMIT 1"
	expectedSQL := `SELECT id, email FROM "public"."zgi_base_users" LIMIT 1`
	mockClient := &fakeSQLBase{
		executeFunc: func(ctx context.Context, query string, params []any, auditCtx *audit.Context) (*sql_base.QueryResult, error) {
			if query != expectedSQL {
				t.Fatalf("unexpected query: %s", query)
			}
			return &sql_base.QueryResult{
				RowsAffected: 1,
				Columns:      []string{"id", "email"},
				Rows: [][]any{
					{int64(1), "alice@example.com"},
				},
			}, nil
		},
	}

	callConfig := map[string]any{
		"id": "call-db-node",
		"data": map[string]any{
			"data_source": map[string]any{
				"id":   "pg-main",
				"name": "main",
				"type": "postgres",
			},
			"table_selection": []any{
				map[string]any{
					"schema":  "public",
					"name":    "users",
					"label":   "Users",
					"id":      "table-uuid-1",
					"columns": []any{"id", "email"},
				},
			},
			"manual_sql": rawSQL,
			"execution": map[string]any{
				"timeout_seconds": 5,
				"max_retries":     0,
			},
		},
	}

	prev := "start-node"
	callNode, err := New("call-instance", callConfig, initParams, graph, state, &prev, mockClient)
	if err != nil {
		t.Fatalf("call.New error: %v", err)
	}

	// calldatabase node
	callResult := runNode(t, ctx, callNode)
	addOutputsToPool(state, "call-db-node", callResult.Outputs)

	if callResult.Status != shared.SUCCEEDED {
		t.Fatalf("call node status: %s", callResult.Status)
	}
	if callResult.Outputs["row_count"] != 1 {
		t.Fatalf("unexpected row_count: %v", callResult.Outputs["row_count"])
	}
	if mockClient.auditCtx == nil {
		t.Fatal("audit context should be passed to sql base")
	}
	if mockClient.auditCtx.ClientType != audit.ClientTypeWorkflow {
		t.Fatalf("client type = %s, want %s", mockClient.auditCtx.ClientType, audit.ClientTypeWorkflow)
	}
	if mockClient.auditCtx.OrganizationID != "organization-1" {
		t.Fatalf("organization_id = %s, want organization-1", mockClient.auditCtx.OrganizationID)
	}
	if mockClient.auditCtx.WorkspaceID != "workspace-1" {
		t.Fatalf("workspace_id = %s, want workspace-1", mockClient.auditCtx.WorkspaceID)
	}
	if mockClient.auditCtx.WorkflowRunID != state.VariablePool.SystemVariables.WorkflowRunID {
		t.Fatalf("workflow_run_id = %s, want %s", mockClient.auditCtx.WorkflowRunID, state.VariablePool.SystemVariables.WorkflowRunID)
	}
	if mockClient.auditCtx.NodeID != "call-db-node" {
		t.Fatalf("node_id = %s, want call-db-node", mockClient.auditCtx.NodeID)
	}
	if mockClient.auditCtx.RequestID != "call-instance:call-db-node" {
		t.Fatalf("request_id = %s, want node instance and node id", mockClient.auditCtx.RequestID)
	}
	if mockClient.auditCtx.TableID != "table-uuid-1" {
		t.Fatalf("table_id = %s, want table-uuid-1", mockClient.auditCtx.TableID)
	}
	if mockClient.auditCtx.TableName != "Users" {
		t.Fatalf("table_name = %s, want Users", mockClient.auditCtx.TableName)
	}
	if got := callResult.Inputs["sql"]; got != expectedSQL {
		t.Fatalf("call database input sql = %#v, want executed sql", got)
	}
	if _, exists := callResult.Inputs["data_source"]; exists {
		t.Fatalf("call database inputs should omit data_source: %#v", callResult.Inputs)
	}
	if _, exists := callResult.Inputs["table_selection"]; exists {
		t.Fatalf("call database inputs should omit table_selection: %#v", callResult.Inputs)
	}
	if _, exists := callResult.Inputs["schema_tables"]; exists {
		t.Fatalf("call database inputs should omit schema_tables: %#v", callResult.Inputs)
	}
	if _, exists := callResult.Inputs["execution"]; exists {
		t.Fatalf("call database inputs should omit execution config: %#v", callResult.Inputs)
	}

	endConfig := map[string]any{
		"id": "end-node",
		"data": map[string]any{
			"outputs": []any{
				map[string]any{
					"variable":       "final_rows",
					"value_selector": []any{"call-db-node", "rows"},
				},
				map[string]any{
					"variable":       "final_count",
					"value_selector": []any{"call-db-node", "row_count"},
				},
			},
		},
	}
	prevEnd := "call-db-node"
	endNode, err := end.New("end-instance", endConfig, initParams, graph, state, &prevEnd)
	if err != nil {
		t.Fatalf("end.New error: %v", err)
	}
	endResult := runNode(t, ctx, endNode)

	rows, ok := endResult.Outputs["final_rows"].([]map[string]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("unexpected final_rows: %#v", endResult.Outputs["final_rows"])
	}
	if rows[0]["email"] != "alice@example.com" {
		t.Fatalf("unexpected email: %v", rows[0])
	}
	callRowCount, _ := callResult.Outputs["row_count"].(int)
	switch v := endResult.Outputs["final_count"].(type) {
	case float64:
		if int(v) != callRowCount {
			t.Fatalf("final_count mismatch: %v vs %v", v, callRowCount)
		}
	case int:
		if v != callRowCount {
			t.Fatalf("final_count mismatch: %v vs %v", v, callRowCount)
		}
	default:
		t.Fatalf("unexpected final_count type: %T", v)
	}
}

func TestExecuteWithRetryUsesSingleAuditRequestIDAcrossAttempts(t *testing.T) {
	var calls int
	mockClient := &fakeSQLBase{
		executeFunc: func(ctx context.Context, query string, params []any, auditCtx *audit.Context) (*sql_base.QueryResult, error) {
			calls++
			if calls < 3 {
				return nil, fmt.Errorf("temporary database error")
			}
			return &sql_base.QueryResult{
				RowsAffected: 0,
				Columns:      []string{},
				Rows:         [][]any{},
			}, nil
		},
	}

	node := &Node{
		NodeStruct: base.NodeStruct{
			NodeID:   "call-db-node",
			NodeType: shared.CallDatabase,
		},
		NodeData: NodeData{
			Execution: ExecutionConfig{MaxRetries: 2},
		},
		sqlClient: mockClient,
	}

	auditCtx := &audit.Context{RequestID: "run-1:call-db-node"}
	_, attempts, err := node.executeWithRetry(context.Background(), "SELECT 1", auditCtx)
	if err != nil {
		t.Fatalf("executeWithRetry returned error: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
	if len(mockClient.auditCtxs) != 3 {
		t.Fatalf("audit ctx count = %d, want 3", len(mockClient.auditCtxs))
	}
	for i, got := range mockClient.auditCtxs {
		if got.RequestID != "run-1:call-db-node" {
			t.Fatalf("attempt %d request_id = %s, want shared request id", i+1, got.RequestID)
		}
		if got.Attempt != i+1 {
			t.Fatalf("attempt %d audit attempt = %d, want %d", i+1, got.Attempt, i+1)
		}
	}
	if len(mockClient.executionOpts) != 3 {
		t.Fatalf("execution option count = %d, want 3", len(mockClient.executionOpts))
	}
	for i, opts := range mockClient.executionOpts {
		if !opts.ReadOnly {
			t.Fatalf("attempt %d read-only = false, want true", i+1)
		}
		if !opts.CatalogOnly {
			t.Fatalf("attempt %d catalog-only = false, want true", i+1)
		}
	}
	if reauthorizationCount != 2 {
		t.Fatalf("reauthorization count = %d, want one before each retry", reauthorizationCount)
	}
}

func TestExecuteWithRetryDoesNotRetryWrites(t *testing.T) {
	executionCount := 0
	mockClient := &fakeSQLBase{executeFunc: func(context.Context, string, []any, *audit.Context) (*sql_base.QueryResult, error) {
		executionCount++
		return nil, errors.New("write failed")
	}}
	node := &Node{
		NodeData:  NodeData{Execution: ExecutionConfig{MaxRetries: 3}},
		sqlClient: mockClient,
	}

	_, attempts, err := node.executeWithRetry(context.Background(), "UPDATE users SET active = false", nil, nil, sql_base.ExecuteSQLOptions{CatalogOnly: true}, nil)
	if err == nil {
		t.Fatal("executeWithRetry error = nil, want write failure")
	}
	if attempts != 1 || executionCount != 1 {
		t.Fatalf("attempts=%d executions=%d, want one non-retried write", attempts, executionCount)
	}
}

func TestExecuteRunEnforcesReadOnlyTransactionForSelect(t *testing.T) {
	var executedSQL string
	mockClient := &fakeSQLBase{executeFunc: func(_ context.Context, query string, _ []any, _ *audit.Context) (*sql_base.QueryResult, error) {
		executedSQL = query
		return &sql_base.QueryResult{Columns: []string{"value"}, Rows: [][]any{{int64(1)}}}, nil
	}}
	node := &Node{
		NodeStruct: base.NodeStruct{
			InstanceID: "test-instance", NodeID: "call-db-node", NodeType: shared.CallDatabase,
			OrganizationID: "organization-1", WorkspaceID: "workspace-1", WorkflowID: "workflow-1",
		},
		NodeData:  NodeData{DataSource: DataSourceConfig{ID: "db-1"}, ManualSQL: "SELECT count(*) AS value FROM users"},
		sqlClient: mockClient, authorizer: &fakeDatabaseAuthorizer{},
	}

	result, err := node.executeRun(context.Background())

	if err != nil {
		t.Fatalf("executeRun returned error: %v", err)
	}
	if result == nil || result.Status != shared.SUCCEEDED {
		t.Fatalf("result = %#v, want successful result", result)
	}
	if len(mockClient.executionOpts) != 1 || !mockClient.executionOpts[0].ReadOnly || !mockClient.executionOpts[0].CatalogOnly {
		t.Fatalf("execution options = %#v, want one read-only catalog-only execution", mockClient.executionOpts)
	}
	if executedSQL != `SELECT count(*) AS value FROM "public"."zgi_base_users"` {
		t.Fatalf("executed SQL = %q, want resolved physical table", executedSQL)
	}
}

func TestExecuteRunDoesNotExecuteWhenTableResolutionFails(t *testing.T) {
	executed := false
	sqlClient := &fakeSQLBase{executeFunc: func(_ context.Context, _ string, _ []any, _ *audit.Context) (*sql_base.QueryResult, error) {
		executed = true
		return &sql_base.QueryResult{}, nil
	}}
	node := &Node{
		NodeStruct: base.NodeStruct{
			NodeID: "call-db-node", NodeType: shared.CallDatabase,
			OrganizationID: "organization-1", WorkspaceID: "workspace-1", WorkflowID: "workflow-1",
		},
		NodeData:   NodeData{DataSource: DataSourceConfig{ID: "db-1"}, ManualSQL: "SELECT * FROM missing_table"},
		sqlClient:  sqlClient,
		authorizer: &fakeDatabaseAuthorizer{resolveErr: databaseauthorization.ErrAccessDenied},
	}

	result, err := node.executeRun(context.Background())

	if result != nil || !errors.Is(err, databaseauthorization.ErrAccessDenied) {
		t.Fatalf("result=%#v error=%v, want access denied", result, err)
	}
	if executed {
		t.Fatal("SQL must not execute after table resolution denial")
	}
}

func TestExecuteRunRejectsLockingSelect(t *testing.T) {
	mockClient := &fakeSQLBase{executeFunc: func(context.Context, string, []any, *audit.Context) (*sql_base.QueryResult, error) {
		return &sql_base.QueryResult{Columns: []string{"id"}, Rows: [][]any{{int64(1)}}}, nil
	}}
	authorizer := &fakeDatabaseAuthorizer{}
	node := &Node{
		NodeStruct: base.NodeStruct{
			InstanceID: "test-instance", NodeID: "call-db-node", NodeType: shared.CallDatabase,
			OrganizationID: "organization-1", WorkspaceID: "workspace-1", WorkflowID: "workflow-1",
		},
		NodeData:  NodeData{DataSource: DataSourceConfig{ID: "db-1"}, ManualSQL: "SELECT id FROM users FOR UPDATE"},
		sqlClient: mockClient, authorizer: authorizer,
	}

	result, err := node.executeRun(context.Background())
	if err == nil || !strings.Contains(err.Error(), "locking SELECT") {
		t.Fatalf("error = %v, want locking SELECT rejection", err)
	}
	if result != nil || len(authorizer.requests) != 0 || len(mockClient.executionOpts) != 0 {
		t.Fatalf("result=%#v authorizations=%d executions=%d, want rejection before authorization", result, len(authorizer.requests), len(mockClient.executionOpts))
	}
}

func TestExecuteRunRejectsDDL(t *testing.T) {
	var executedSQL string
	mockClient := &fakeSQLBase{executeFunc: func(_ context.Context, query string, _ []any, _ *audit.Context) (*sql_base.QueryResult, error) {
		executedSQL = query
		return &sql_base.QueryResult{}, nil
	}}
	node := &Node{
		NodeStruct: base.NodeStruct{
			InstanceID: "test-instance", NodeID: "call-db-node", NodeType: shared.CallDatabase,
			OrganizationID: "organization-1", WorkspaceID: "workspace-1", WorkflowID: "workflow-1",
		},
		NodeData:  NodeData{DataSource: DataSourceConfig{ID: "db-1"}, ManualSQL: "CREATE TABLE workflow_probe (value text)"},
		sqlClient: mockClient, authorizer: &fakeDatabaseAuthorizer{},
	}

	result, err := node.executeRun(context.Background())
	if err == nil || !strings.Contains(err.Error(), "only SELECT, INSERT, UPDATE, and DELETE") {
		t.Fatalf("error = %v, want DDL rejection", err)
	}
	if result != nil || executedSQL != "" {
		t.Fatalf("result=%#v executed SQL=%q, want rejection before execution", result, executedSQL)
	}
}

func TestExecuteRunRejectsFunctionSideEffectsBeforeAuthorization(t *testing.T) {
	for _, sql := range []string{
		"SELECT pg_advisory_lock(42)",
		"INSERT INTO users(name) VALUES ('a') RETURNING mutate_other_rows()",
		"SELECT public.lower(1)",
	} {
		t.Run(sql, func(t *testing.T) {
			mockClient := &fakeSQLBase{}
			authorizer := &fakeDatabaseAuthorizer{}
			node := &Node{
				NodeStruct: base.NodeStruct{
					InstanceID: "test-instance", NodeID: "call-db-node", NodeType: shared.CallDatabase,
					OrganizationID: "organization-1", WorkspaceID: "workspace-1", WorkflowID: "workflow-1",
				},
				NodeData:  NodeData{DataSource: DataSourceConfig{ID: "db-1"}, ManualSQL: sql},
				sqlClient: mockClient, authorizer: authorizer,
			}

			result, err := node.executeRun(context.Background())
			if err == nil || !strings.Contains(err.Error(), "is not allowed") {
				t.Fatalf("error = %v, want unsafe function rejection", err)
			}
			if result != nil {
				t.Fatalf("result = %#v, want nil", result)
			}
			if len(mockClient.executionOpts) != 0 {
				t.Fatalf("execution options = %#v, want no SQL execution", mockClient.executionOpts)
			}
		})
	}
}

func TestExecuteRunFailsClosedWhenReadOnlyExecutionIsUnavailable(t *testing.T) {
	type legacySQLBase struct{ sql_base.SQLBase }
	node := &Node{
		NodeStruct: base.NodeStruct{
			InstanceID: "test-instance", NodeID: "call-db-node", NodeType: shared.CallDatabase,
			OrganizationID: "organization-1", WorkspaceID: "workspace-1", WorkflowID: "workflow-1",
		},
		NodeData:  NodeData{DataSource: DataSourceConfig{ID: "db-1"}, ManualSQL: "SELECT 1"},
		sqlClient: legacySQLBase{}, authorizer: &fakeDatabaseAuthorizer{},
	}

	result, err := node.executeRun(context.Background())

	if err != nil {
		t.Fatalf("executeRun returned transport error: %v", err)
	}
	if result == nil || result.Status != shared.FAILED {
		t.Fatalf("result = %#v, want failed result", result)
	}
	if !strings.Contains(result.ErrMsg, sql_base.ErrCatalogOnlyExecutionUnsupported.Error()) {
		t.Fatalf("error = %q, want catalog-only unsupported error", result.ErrMsg)
	}
}

func TestNewPrefersCanonicalWorkspaceIDForAuditScope(t *testing.T) {
	mockClient := &fakeSQLBase{
		executeFunc: func(ctx context.Context, query string, params []any, auditCtx *audit.Context) (*sql_base.QueryResult, error) {
			return &sql_base.QueryResult{
				RowsAffected: 0,
				Columns:      []string{},
				Rows:         [][]any{},
			}, nil
		},
	}

	nodeConfig := map[string]any{
		"id": "call-db-node",
		"data": map[string]any{
			"data_source": map[string]any{
				"id":   "pg-main",
				"name": "main",
				"type": "postgres",
			},
			"manual_sql": "SELECT 1",
			"execution": map[string]any{
				"timeout_seconds": 5,
				"max_retries":     0,
			},
		},
	}
	initParams := entities.GraphInitParams{
		TenantID:       "legacy-workspace",
		WorkspaceID:    "canonical-workspace",
		OrganizationID: "organization-1",
		AppID:          "app-1",
		WorkflowType:   entities.WorkflowTypeWorkflow,
		WorkflowID:     "workflow-1",
		GraphConfig:    map[string]any{},
		UserID:         "user-1",
		UserFrom:       entities.UserFromAccount,
		InvokeFrom:     entities.InvokeFromDebugger,
	}

	node, err := New("call-instance", nodeConfig, initParams, &entities.Graph{}, entities.NewGraphRuntimeStateWithDefaults(), nil, mockClient)
	if err != nil {
		t.Fatalf("New error: %v", err)
	}

	runNode(t, context.Background(), node)

	if mockClient.auditCtx == nil {
		t.Fatal("audit context should be passed to sql base")
	}
	if mockClient.auditCtx.WorkspaceID != "canonical-workspace" {
		t.Fatalf("workspace_id = %s, want canonical-workspace", mockClient.auditCtx.WorkspaceID)
	}
}

func TestEnsureQuotedIdentifiers(t *testing.T) {
	tableName := "zgi_base_tbl_183d2908-7e0c-4d14-81a0-d95e7c14fc09"
	testCases := []struct {
		name   string
		sql    string
		tables []TableRef
		want   string
	}{
		{
			name: "AddPrefixWithSchema",
			sql:  "SELECT * FROM public.users LIMIT 10",
			tables: []TableRef{
				{Schema: "public", Name: "users"},
			},
			want: `SELECT * FROM "public"."zgi_base_users" LIMIT 10`,
		},
		{
			name: "AddPrefixWithoutSchema",
			sql:  "SELECT * FROM orders ORDER BY created_time DESC",
			tables: []TableRef{
				{Name: "orders"},
			},
			want: `SELECT * FROM "zgi_base_orders" ORDER BY created_time DESC`,
		},
		{
			name: "HyphenatedWithSchema",
			sql:  fmt.Sprintf("SELECT user_name FROM public.%s WHERE is_vip_customer=true", tableName),
			tables: []TableRef{
				{Schema: "public", Name: tableName},
			},
			want: fmt.Sprintf(`SELECT user_name FROM "public"."%s" WHERE is_vip_customer=true`, tableName),
		},
		{
			name: "HyphenatedWithoutSchema",
			sql:  fmt.Sprintf("SELECT * FROM %s ORDER BY created_time DESC", tableName),
			tables: []TableRef{
				{Name: tableName},
			},
			want: fmt.Sprintf(`SELECT * FROM "%s" ORDER BY created_time DESC`, tableName),
		},
		{
			name: "AlreadyQuoted",
			sql:  fmt.Sprintf(`SELECT * FROM "public"."%s" WHERE id = 1`, tableName),
			tables: []TableRef{
				{Schema: "public", Name: tableName},
			},
			want: fmt.Sprintf(`SELECT * FROM "public"."%s" WHERE id = 1`, tableName),
		},
		{
			name: "ColumnQualifiedUsage",
			sql:  "SELECT orders.total_amount FROM public.orders AS orders",
			tables: []TableRef{
				{Schema: "public", Name: "orders"},
			},
			want: `SELECT "zgi_base_orders".total_amount FROM "public"."zgi_base_orders" AS "zgi_base_orders"`,
		},
		{
			name: "StringLiteralIgnored",
			sql:  fmt.Sprintf("SELECT * FROM public.%s WHERE note = 'public.%s'", tableName, tableName),
			tables: []TableRef{
				{Schema: "public", Name: tableName},
			},
			want: fmt.Sprintf(`SELECT * FROM "public"."%s" WHERE note = 'public.%s'`, tableName, tableName),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := ensureQuotedIdentifiers(tc.sql, tc.tables)
			if got != tc.want {
				t.Fatalf("unexpected sql:\nwant: %s\ngot:  %s", tc.want, got)
			}
		})
	}
}

func TestExecuteRunAppliesIdentifierQuoting(t *testing.T) {
	const tableName = "zgi_base_tbl_183d2908-7e0c-4d14-81a0-d95e7c14fc09"
	const rawSQL = "SELECT user_name FROM public." + tableName + " WHERE is_vip_customer=true"
	const expectedSQL = `SELECT user_name FROM "public"."` + tableName + `" WHERE is_vip_customer=true`

	var executedSQL string
	mockClient := &fakeSQLBase{
		executeFunc: func(ctx context.Context, query string, params []any, auditCtx *audit.Context) (*sql_base.QueryResult, error) {
			executedSQL = query
			return &sql_base.QueryResult{
				RowsAffected: 0,
				Columns:      []string{"user_name"},
				Rows: [][]any{
					{"alice"},
				},
			}, nil
		},
	}

	node := &Node{
		NodeStruct: base.NodeStruct{
			InstanceID: "test-instance",
			NodeID:     "call-db-node",
			NodeType:   shared.CallDatabase,
		},
		NodeData: NodeData{
			DataSource: DataSourceConfig{
				ID:   "ds-1",
				Name: "测试数据库1",
				Type: "postgres",
			},
			TableSelection: []TableRef{
				{Schema: "public", Name: tableName},
			},
			ManualSQL: rawSQL,
			Execution: ExecutionConfig{
				TimeoutSeconds: 5,
				MaxRetries:     0,
			},
		},
		sqlClient: mockClient,
	}

	_, err := node.executeRun(context.Background())
	if err != nil {
		t.Fatalf("executeRun returned error: %v", err)
	}
	if executedSQL != expectedSQL {
		t.Fatalf("unexpected executed sql:\nwant: %s\ngot:  %s", expectedSQL, executedSQL)
	}
}

func runNode(t *testing.T, ctx context.Context, node shared.NodeInterface) *shared.NodeRunResult {
	t.Helper()
	eventCh := make(chan *shared.NodeEventCh, 10)
	errCh := make(chan error, 1)
	go func() {
		err := node.Run(ctx, eventCh)
		errCh <- err
		close(eventCh)
	}()

	var result *shared.NodeRunResult
	for event := range eventCh {
		switch event.Type {
		case shared.EventTypeRunCompleted:
			if data, ok := event.Data.(*shared.RunCompletedEvent); ok {
				result = data.RunResult
			}
		case shared.EventTypeRunFailed:
			t.Fatalf("node run failed: %v", event.Error)
		}
	}

	if err := <-errCh; err != nil {
		t.Fatalf("node.Run returned error: %v", err)
	}
	if result == nil {
		t.Fatalf("no RunCompletedEvent received")
	}
	return result
}

func addOutputsToPool(state *entities.GraphRuntimeState, nodeID string, outputs map[string]any) {
	for key, value := range outputs {
		state.VariablePool.Add([]string{nodeID, key}, value)
	}
}
