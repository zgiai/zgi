package calldatabase

import (
	"context"
	"fmt"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/end"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/start"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	"github.com/zgiai/zgi/api/pkg/sql_base"
)

type fakeSQLBase struct {
	sql_base.SQLBase
	executeFunc func(ctx context.Context, query string, params []any) (*sql_base.QueryResult, error)
}

func (f *fakeSQLBase) ExecuteSQL(ctx context.Context, query string, params []any) (*sql_base.QueryResult, error) {
	if f.executeFunc != nil {
		return f.executeFunc(ctx, query, params)
	}
	return nil, nil
}

func ExampleNode_executeRun() {
	mockClient := &fakeSQLBase{
		executeFunc: func(ctx context.Context, query string, params []any) (*sql_base.QueryResult, error) {
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
		OrganizationID: "tenant-1",
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
		executeFunc: func(ctx context.Context, query string, params []any) (*sql_base.QueryResult, error) {
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
		executeFunc: func(ctx context.Context, query string, params []any) (*sql_base.QueryResult, error) {
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
