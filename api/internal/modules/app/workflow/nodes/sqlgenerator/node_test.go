package sqlgenerator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/text2sql"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/calldatabase"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	workspace "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/pkg/sql_base"
)

func TestNodeExecuteRunSuccess(t *testing.T) {
	ctx := context.Background()

	state := entities.NewGraphRuntimeStateWithDefaults()
	state.VariablePool.Add([]string{"vars", "test"}, "List VIP users created in the last 7 days")
	state.VariablePool.UserInputs["data_source"] = map[string]any{"id": "untrusted-runtime-ds"}
	state.VariablePool.UserInputs["table_selection"] = []any{map[string]any{"id": "untrusted-runtime-table"}}

	mockLLM := &stubLLMInvoker{
		response: `{"sql":"SELECT email FROM public.users"}`,
	}

	node := &Node{
		NodeStruct: base.NodeStruct{
			NodeID:            "sql-node",
			NodeType:          shared.SQLGenerator,
			TenantID:          "tenant-1",
			UserID:            "user-1",
			GraphRuntimeState: state,
		},
		NodeData: NodeData{
			Model: ModelSection{
				Provider:   "stub-provider",
				Name:       "stub-model",
				Mode:       "chat",
				Parameters: map[string]any{},
			},
			DataSource: DataSourceSection{
				Source: calldatabase.DataSourceConfig{
					ID:   "ds-1",
					Name: "main",
				},
				Tables: []calldatabase.TableRef{
					{
						TableID: 7,
						Schema:  "public",
						Name:    "users",
						Columns: []string{"id", "email"},
					},
				},
			},
			Prompt: PromptSection{
				User: "User question: {{#vars.test#}}",
			},
			Execution: ExecutionConfig{
				TimeoutSeconds: 5,
				MaxRetries:     0,
			},
		},
		llmInvoker: mockLLM,
		sqlClient: &sqlGeneratorFakeSQLBase{table: &sql_base.Table{
			ID: 7, Schema: "public", Name: "users",
			Columns: []sql_base.Column{{Name: "id", DataType: "bigint"}, {Name: "email", DataType: "text"}},
		}},
	}
	attachText2SQLTestDependencies(node)
	node.NodeData.ensureDefaults()

	result, err := node.executeRun(ctx)
	if err != nil {
		t.Fatalf("executeRun returned error: %v", err)
	}

	if result.Status != shared.SUCCEEDED {
		t.Fatalf("expected status %s, got %s", shared.SUCCEEDED, result.Status)
	}

	sqlOutput, ok := result.Outputs["sql"].(string)
	if !ok {
		t.Fatalf("sql output missing or not string: %#v", result.Outputs["sql"])
	}
	expectedSQL := "SELECT email FROM public.users"
	if sqlOutput != expectedSQL {
		t.Fatalf("unexpected sql output: %s", sqlOutput)
	}

	if attempts, ok := result.ProcessData["attempts"].(int); !ok || attempts != 1 {
		t.Fatalf("expected attempts to be 1, got %#v", result.ProcessData["attempts"])
	}

	if _, exists := result.Inputs["prompt_variables"]; exists {
		t.Fatalf("prompt_variables should be omitted from frontend input snapshot: %#v", result.Inputs)
	}
	prompt, ok := result.Inputs["prompt"].(map[string]any)
	if !ok {
		t.Fatalf("prompt type = %T, want map[string]any", result.Inputs["prompt"])
	}
	if got := prompt["user"]; got != "User question: List VIP users created in the last 7 days" {
		t.Fatalf("prompt.user = %#v, want rendered user prompt", got)
	}
	if node.NodeData.DataSource.Source.ID != "ds-1" || len(node.NodeData.DataSource.Tables) != 1 || node.NodeData.DataSource.Tables[0].Name != "users" {
		t.Fatalf("runtime input overrode persisted database scope: %#v", node.NodeData.DataSource)
	}
	for _, key := range []string{"system_prompt", "rendered_user_prompt", "model", "metadata_context", "data_source", "schema_tables", "table_schema"} {
		if _, exists := result.Inputs[key]; exists {
			t.Fatalf("input %s should be omitted from frontend input snapshot: %#v", key, result.Inputs)
		}
	}
}

func TestNodeExecuteRunInvocationFailure_PreservesBillingError(t *testing.T) {
	ctx := context.Background()

	state := entities.NewGraphRuntimeStateWithDefaults()
	billingErr := errors.Join(
		errors.New("all providers failed"),
		&gateway.BillingUserError{
			Kind:  gateway.BillingUserErrorKindPrivateChannelBalanceInsufficient,
			Cause: gateway.ErrInsufficientBalance,
		},
	)

	node := &Node{
		NodeStruct: base.NodeStruct{
			NodeID:            "sql-node",
			NodeType:          shared.SQLGenerator,
			TenantID:          "tenant-1",
			UserID:            "user-1",
			GraphRuntimeState: state,
		},
		NodeData: NodeData{
			Model: ModelSection{
				Provider:   "stub-provider",
				Name:       "stub-model",
				Mode:       "chat",
				Parameters: map[string]any{},
			},
			DataSource: DataSourceSection{
				Source: calldatabase.DataSourceConfig{ID: "ds-1"},
				Tables: []calldatabase.TableRef{{TableID: 7, Schema: "public", Name: "users"}},
			},
			Prompt: PromptSection{
				User: "List VIP users",
			},
			Execution: ExecutionConfig{
				TimeoutSeconds: 5,
				MaxRetries:     0,
			},
		},
		llmInvoker: &stubLLMInvoker{err: billingErr},
		sqlClient:  &sqlGeneratorFakeSQLBase{table: &sql_base.Table{ID: 7, Schema: "public", Name: "users", Columns: []sql_base.Column{{Name: "id"}}}},
	}
	attachText2SQLTestDependencies(node)
	node.NodeData.ensureDefaults()

	result, err := node.executeRun(ctx)
	if err != nil {
		t.Fatalf("executeRun returned error: %v", err)
	}
	if result.Status != shared.FAILED {
		t.Fatalf("status = %s, want %s", result.Status, shared.FAILED)
	}

	var userErr *gateway.BillingUserError
	if !errors.As(result.Err, &userErr) {
		t.Fatalf("errors.As(result.Err, *BillingUserError) = false, err = %v", result.Err)
	}
	if userErr.Kind != gateway.BillingUserErrorKindPrivateChannelBalanceInsufficient {
		t.Fatalf("userErr.Kind = %q, want %q", userErr.Kind, gateway.BillingUserErrorKindPrivateChannelBalanceInsufficient)
	}
}

func TestNodeExecuteRunUsesEngineSystemPrompt(t *testing.T) {
	ctx := context.Background()
	state := entities.NewGraphRuntimeStateWithDefaults()

	mockLLM := &stubLLMInvoker{response: `{"sql":"SELECT id FROM public.users"}`}
	node := &Node{
		NodeStruct: base.NodeStruct{
			NodeID:            "sql-node",
			NodeType:          shared.SQLGenerator,
			TenantID:          "tenant-1",
			UserID:            "user-1",
			GraphRuntimeState: state,
		},
		NodeData: NodeData{
			Model: ModelSection{
				Provider:   "stub-provider",
				Name:       "stub-model",
				Mode:       "chat",
				Parameters: map[string]any{},
			},
			DataSource: DataSourceSection{Source: calldatabase.DataSourceConfig{ID: "ds-1"}, Tables: []calldatabase.TableRef{{TableID: 7, Schema: "public", Name: "users"}}},
			Prompt: PromptSection{
				User: "List users",
			},
			Execution: ExecutionConfig{TimeoutSeconds: 5},
		},
		llmInvoker: mockLLM,
		sqlClient:  &sqlGeneratorFakeSQLBase{table: &sql_base.Table{ID: 7, Schema: "public", Name: "users", Columns: []sql_base.Column{{Name: "id"}}}},
	}
	attachText2SQLTestDependencies(node)
	node.NodeData.ensureDefaults()

	result, err := node.executeRun(ctx)
	if err != nil {
		t.Fatalf("executeRun returned error: %v", err)
	}
	if result.Status != shared.SUCCEEDED {
		t.Fatalf("status = %s, want %s", result.Status, shared.SUCCEEDED)
	}
	if mockLLM.lastReq == nil || len(mockLLM.lastReq.Messages) == 0 {
		t.Fatalf("expected LLM request to be captured")
	}
	if got := mockLLM.lastReq.Messages[0].Content; !strings.Contains(got, "Return one JSON object") {
		t.Fatalf("system prompt = %#v, want strict JSON instruction", got)
	}
	if got := mockLLM.lastReq.Parameters["response_format"]; got != "json_object" {
		t.Fatalf("response_format = %#v, want json_object", got)
	}
	if _, exists := mockLLM.lastReq.Parameters["json_schema"]; exists {
		t.Fatalf("json_schema should not be sent: %#v", mockLLM.lastReq.Parameters["json_schema"])
	}
	if len(mockLLM.lastReq.Messages) < 2 || !strings.Contains(mockLLM.lastReq.Messages[1].Content, "Trusted schema:") || !strings.Contains(mockLLM.lastReq.Messages[1].Content, "public.users") {
		t.Fatalf("user prompt should retain trusted database schema: %#v", mockLLM.lastReq.Messages)
	}
}

func TestNodeExecuteRunTellsModelToUseStoredEncodingForValues(t *testing.T) {
	ctx := context.Background()
	state := entities.NewGraphRuntimeStateWithDefaults()

	mockLLM := &stubLLMInvoker{response: `{"sql":"SELECT COUNT(*) FROM public.users WHERE status = 'completed'"}`}
	node := &Node{
		NodeStruct: base.NodeStruct{
			NodeID:            "sql-node",
			NodeType:          shared.SQLGenerator,
			TenantID:          "tenant-1",
			UserID:            "user-1",
			GraphRuntimeState: state,
		},
		NodeData: NodeData{
			Model: ModelSection{
				Provider:   "stub-provider",
				Name:       "stub-model",
				Mode:       "chat",
				Parameters: map[string]any{},
			},
			DataSource: DataSourceSection{Source: calldatabase.DataSourceConfig{ID: "ds-1"}, Tables: []calldatabase.TableRef{{TableID: 7, Schema: "public", Name: "users"}}},
			Prompt: PromptSection{
				User: "已经完成的用户有多少？",
			},
			Execution: ExecutionConfig{TimeoutSeconds: 5},
		},
		llmInvoker: mockLLM,
		sqlClient:  &sqlGeneratorFakeSQLBase{table: &sql_base.Table{ID: 7, Schema: "public", Name: "users", Columns: []sql_base.Column{{Name: "id"}, {Name: "status"}}}},
	}
	attachText2SQLTestDependencies(node)
	node.NodeData.ensureDefaults()

	result, err := node.executeRun(ctx)
	if err != nil {
		t.Fatalf("executeRun returned error: %v", err)
	}
	if result.Status != shared.SUCCEEDED {
		t.Fatalf("status = %s, want %s", result.Status, shared.SUCCEEDED)
	}
	system := mockLLM.lastReq.Messages[0].Content
	if !strings.Contains(system, "encoding actually stored in the database") {
		t.Fatalf("system prompt = %#v, want the stored-encoding rule", system)
	}
	if !strings.Contains(system, "never copy a non-English word from the question into a value") {
		t.Fatalf("system prompt = %#v, want the no-translated-values rule", system)
	}
	// The rule must not weaken the pre-existing contracts.
	if !strings.Contains(system, "Return one JSON object") || !strings.Contains(system, "Use only the supplied database schema") {
		t.Fatalf("system prompt = %#v, want existing contracts preserved", system)
	}
}

func TestNodeExecuteRunIgnoresLegacyNodeSystemPrompt(t *testing.T) {
	ctx := context.Background()
	state := entities.NewGraphRuntimeStateWithDefaults()

	mockLLM := &stubLLMInvoker{response: `{"sql":"SELECT id FROM public.users"}`}
	node := &Node{
		NodeStruct: base.NodeStruct{
			NodeID:            "sql-node",
			NodeType:          shared.SQLGenerator,
			TenantID:          "tenant-1",
			UserID:            "user-1",
			GraphRuntimeState: state,
		},
		NodeData: NodeData{
			Model: ModelSection{
				Provider:   "stub-provider",
				Name:       "stub-model",
				Mode:       "chat",
				Parameters: map[string]any{},
			},
			DataSource: DataSourceSection{Source: calldatabase.DataSourceConfig{ID: "ds-1"}, Tables: []calldatabase.TableRef{{TableID: 7, Schema: "public", Name: "users"}}},
			Prompt: PromptSection{
				System: "Node system prompt",
				User:   "List users",
			},
			Execution: ExecutionConfig{TimeoutSeconds: 5},
		},
		llmInvoker: mockLLM,
		sqlClient:  &sqlGeneratorFakeSQLBase{table: &sql_base.Table{ID: 7, Schema: "public", Name: "users", Columns: []sql_base.Column{{Name: "id"}}}},
	}
	attachText2SQLTestDependencies(node)
	node.NodeData.ensureDefaults()

	result, err := node.executeRun(ctx)
	if err != nil {
		t.Fatalf("executeRun returned error: %v", err)
	}
	if result.Status != shared.SUCCEEDED {
		t.Fatalf("status = %s, want %s", result.Status, shared.SUCCEEDED)
	}
	if mockLLM.lastReq == nil || len(mockLLM.lastReq.Messages) == 0 {
		t.Fatalf("expected LLM request to be captured")
	}
	if got := mockLLM.lastReq.Messages[0].Content; got == "Node system prompt" || !strings.Contains(got, "Return one JSON object") {
		t.Fatalf("system prompt = %#v, want engine-owned strict prompt", got)
	}
}

func TestEmbeddingAppContextIncludesRequiredWorkflowIdentity(t *testing.T) {
	node := &Node{NodeStruct: base.NodeStruct{
		OrganizationID: "org-1", WorkspaceID: "workspace-1", UserID: "account-1", APPID: "app-1",
		WorkflowID: "workflow-1", NodeID: "node-1", NodeType: shared.SQLGenerator,
	}, billingSubjectType: llmclient.BillingSubjectTypeWorkspace}
	ctx := node.embeddingAppContext()
	if err := ctx.Validate(); err != nil {
		t.Fatalf("embedding AppContext.Validate() error = %v", err)
	}
	if ctx.AccountID != node.UserID || ctx.NodeID != node.NodeID || ctx.WorkflowID != node.WorkflowID {
		t.Fatalf("embedding AppContext lost workflow identity: %#v", ctx)
	}
}

func TestBuildChatRequestUsesJSONModeWithoutSchema(t *testing.T) {
	req := buildChatRequest(&InvokeRequest{
		ModelSlug: "qwen-plus",
		Parameters: map[string]any{
			"response_format": "json_object",
		},
	})
	if req.ResponseFormat == nil || req.ResponseFormat.Type != "json_object" {
		t.Fatalf("response format = %#v, want json_object", req.ResponseFormat)
	}
	if req.ResponseFormat.Schema != nil {
		t.Fatalf("response schema = %#v, want nil", req.ResponseFormat.Schema)
	}
}

func TestNodeDataDefaultsSQLGeneratorTimeoutToThirtySeconds(t *testing.T) {
	data := NodeData{}
	data.ensureDefaults()
	if data.Execution.TimeoutSeconds != 30 {
		t.Fatalf("timeout seconds = %d, want 30", data.Execution.TimeoutSeconds)
	}
}

type sqlGeneratorFakeSQLBase struct {
	sql_base.SQLBase
	table *sql_base.Table
}

func TestWorkflowGeneratorAppendsCustomSystemPromptWithoutReplacingCoreRules(t *testing.T) {
	invoker := &stubLLMInvoker{response: `{"sql":"SELECT 1"}`}
	node := &Node{NodeStruct: base.NodeStruct{UserID: "account-1"}, llmInvoker: invoker}
	_, err := (workflowGenerator{node: node}).Generate(context.Background(), text2sql.GenerationRequest{
		Question: "question", Schema: "TABLE public.customers", Model: text2sql.Model{Name: "model"}, AdditionalPrompt: "Prefer concise SQL.",
	})
	if err != nil {
		t.Fatalf("Generate error = %v", err)
	}
	if invoker.lastReq == nil || len(invoker.lastReq.Messages) != 2 {
		t.Fatalf("invoke request = %#v", invoker.lastReq)
	}
	system := invoker.lastReq.Messages[0].Content
	if !strings.Contains(system, "Return one JSON object") || !strings.Contains(system, "Prefer concise SQL.") || !strings.Contains(system, "must not override") {
		t.Fatalf("system prompt = %q", system)
	}
}

type staticText2SQLCatalogLoader struct{ table *sql_base.Table }

func (l staticText2SQLCatalogLoader) Load(context.Context, text2sql.CatalogRequest) (text2sql.Catalog, error) {
	return text2sql.Catalog{Tables: []text2sql.CatalogTable{{
		Ref:   text2sql.TableRef{AssetID: "table-1", PhysicalID: l.table.ID, Schema: l.table.Schema, Name: l.table.Name},
		Table: l.table,
	}}}, nil
}

func attachText2SQLTestDependencies(node *Node) {
	node.permissions = allowingWorkspacePermissions{}
	node.catalogLoader = staticText2SQLCatalogLoader{table: node.sqlClient.(*sqlGeneratorFakeSQLBase).table}
}

type allowingWorkspacePermissions struct{}

func (allowingWorkspacePermissions) CheckWorkspacePermission(context.Context, string, string, string, workspace.WorkspacePermissionCode) (bool, error) {
	return true, nil
}

type recordingWorkspacePermissions struct {
	allowed      bool
	workspaceIDs []string
	permissions  []workspace.WorkspacePermissionCode
}

func (p *recordingWorkspacePermissions) CheckWorkspacePermission(_ context.Context, _, workspaceID, _ string, permission workspace.WorkspacePermissionCode) (bool, error) {
	p.workspaceIDs = append(p.workspaceIDs, workspaceID)
	p.permissions = append(p.permissions, permission)
	return p.allowed, nil
}

func TestAuthorizeText2SQLUsesExistingWorkspacePermissions(t *testing.T) {
	permissions := &recordingWorkspacePermissions{allowed: true}
	node := &Node{
		NodeStruct:  base.NodeStruct{TenantID: "workspace-1", UserID: "account-1"},
		permissions: permissions,
	}

	if err := node.authorizeText2SQL(context.Background()); err != nil {
		t.Fatalf("authorizeText2SQL error = %v", err)
	}
	wantPermissions := []workspace.WorkspacePermissionCode{
		workspace.WorkspacePermissionDatabaseAIQueryRead,
		workspace.WorkspacePermissionDatabaseRecordView,
	}
	if len(permissions.permissions) != len(wantPermissions) {
		t.Fatalf("permission checks = %v", permissions.permissions)
	}
	for index, permission := range wantPermissions {
		if permissions.permissions[index] != permission || permissions.workspaceIDs[index] != "workspace-1" {
			t.Fatalf("permission check %d = (%s, %s)", index, permissions.workspaceIDs[index], permissions.permissions[index])
		}
	}
}

func TestAuthorizeText2SQLFailsClosedWithoutWorkspacePermissions(t *testing.T) {
	node := &Node{NodeStruct: base.NodeStruct{TenantID: "workspace-1", UserID: "account-1"}}
	if err := node.authorizeText2SQL(context.Background()); !errors.Is(err, errDatabaseAccessDenied) {
		t.Fatalf("authorizeText2SQL error = %v, want %v", err, errDatabaseAccessDenied)
	}
}

type recordingEmbeddingClient struct {
	llmclient.LLMClient
	request *llmadapter.EmbeddingsRequest
}

func (c *recordingEmbeddingClient) AppEmbed(_ context.Context, _ *llmclient.AppContext, request *llmadapter.EmbeddingsRequest) (*llmadapter.EmbeddingsResponse, error) {
	c.request = request
	return &llmadapter.EmbeddingsResponse{Data: []llmadapter.Embedding{{Embedding: []float32{0.1}}}}, nil
}

func TestWorkflowEmbedderPreservesConfiguredProvider(t *testing.T) {
	client := &recordingEmbeddingClient{}
	node := &Node{
		NodeStruct: base.NodeStruct{
			OrganizationID: "org-1", WorkspaceID: "workspace-1", TenantID: "workspace-1", UserID: "account-1",
			APPID: "app-1", WorkflowID: "workflow-1", NodeID: "node-1", NodeType: shared.SQLGenerator,
		},
		llmClient: client,
	}
	if _, err := (workflowEmbedder{node: node}).Embed(context.Background(), "provider-a", "shared-model", "question"); err != nil {
		t.Fatal(err)
	}
	if client.request == nil || client.request.Provider != "provider-a" {
		t.Fatalf("embedding request = %#v, want provider-a", client.request)
	}
}

func (f *sqlGeneratorFakeSQLBase) GetTable(context.Context, int) (*sql_base.Table, error) {
	return f.table, nil
}

type stubLLMInvoker struct {
	response string
	err      error
	lastReq  *InvokeRequest
	usage    *llmadapter.Usage
}

func (s *stubLLMInvoker) Invoke(ctx context.Context, accountID, appID, appType string, req *InvokeRequest) (*InvokeResult, error) {
	s.lastReq = req
	if s.err != nil {
		return nil, s.err
	}
	return &InvokeResult{
		Text:   s.response,
		Finish: "stop",
		Usage:  s.usage,
	}, nil
}

func TestFailedSQLGenerationStillReportsItsTokens(t *testing.T) {
	state := entities.NewGraphRuntimeStateWithDefaults()
	state.VariablePool.Add([]string{"vars", "test"}, "List VIP users")
	// Every attempt, the repair included, returns something that is not SQL.
	invoker := &stubLLMInvoker{response: "not json", usage: &llmadapter.Usage{PromptTokens: 20, CompletionTokens: 10, TotalTokens: 30}}
	node := &Node{
		NodeStruct: base.NodeStruct{NodeID: "sql-node", NodeType: shared.SQLGenerator, TenantID: "tenant-1", UserID: "user-1", GraphRuntimeState: state},
		NodeData: NodeData{
			Model: ModelSection{Provider: "stub-provider", Name: "stub-model", Mode: "chat", Parameters: map[string]any{}},
			DataSource: DataSourceSection{
				Source: calldatabase.DataSourceConfig{ID: "ds-1", Name: "main"},
				Tables: []calldatabase.TableRef{{TableID: 7, Schema: "public", Name: "users", Columns: []string{"id", "email"}}},
			},
			Prompt:    PromptSection{User: "User question: {{#vars.test#}}"},
			Execution: ExecutionConfig{TimeoutSeconds: 5},
		},
		llmInvoker: invoker,
		sqlClient: &sqlGeneratorFakeSQLBase{table: &sql_base.Table{
			ID: 7, Schema: "public", Name: "users",
			Columns: []sql_base.Column{{Name: "id", DataType: "bigint"}, {Name: "email", DataType: "text"}},
		}},
	}
	attachText2SQLTestDependencies(node)
	node.NodeData.ensureDefaults()

	result, err := node.executeRun(context.Background())
	if err != nil || result.Status != shared.FAILED {
		t.Fatalf("result = %#v, %v", result, err)
	}
	usage, ok := result.Outputs["usage"].(map[string]any)
	if total, _ := usage["total_tokens"].(int); !ok || total == 0 || total%30 != 0 {
		t.Fatalf("failed generation usage = %#v", result.Outputs["usage"])
	}
}

func TestWorkflowGeneratorAddsUpTheTokensOfEveryAttempt(t *testing.T) {
	invoker := &stubLLMInvoker{response: `{"sql":"SELECT 1"}`, usage: &llmadapter.Usage{PromptTokens: 145, CompletionTokens: 351, TotalTokens: 496}}
	usage := &generationUsage{}
	generator := workflowGenerator{node: &Node{llmInvoker: invoker}, usage: usage}
	request := text2sql.GenerationRequest{Question: "question", Schema: "TABLE public.customers", Model: text2sql.Model{Name: "model"}}
	if _, err := generator.Generate(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	// A repair is a second call; a gateway that leaves out the total still counts.
	invoker.usage = &llmadapter.Usage{PromptTokens: 100, CompletionTokens: 28}
	request.Repair = &text2sql.RepairRequest{PreviousResponse: "SELECT", Failure: "invalid"}
	if _, err := generator.Generate(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if got := usage.output(); got["prompt_tokens"] != 245 || got["completion_tokens"] != 379 || got["total_tokens"] != 624 {
		t.Fatalf("usage = %#v", got)
	}
}
