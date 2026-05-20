package sqlgenerator

import (
	"context"
	"errors"
	"testing"

	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/calldatabase"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
	"github.com/zgiai/ginext/internal/modules/llm/gateway"
	"github.com/zgiai/ginext/internal/prompt"
)

func TestNodeExecuteRunSuccess(t *testing.T) {
	ctx := context.Background()

	state := entities.NewGraphRuntimeStateWithDefaults()
	state.VariablePool.Add([]string{"vars", "test"}, "List VIP users created in the last 7 days")

	mockLLM := &stubLLMInvoker{
		response: "```sql\nSELECT * FROM vip_users;\n```",
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
	}
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
	expectedSQL := "SELECT * FROM vip_users;"
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
			DataSource: DataSourceSection{},
			Prompt: PromptSection{
				User: "List VIP users",
			},
			Execution: ExecutionConfig{
				TimeoutSeconds: 5,
				MaxRetries:     0,
			},
		},
		llmInvoker: &stubLLMInvoker{err: billingErr},
	}
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

func TestNodeExecuteRunUsesEmbeddedSystemPrompt(t *testing.T) {
	ctx := context.Background()
	state := entities.NewGraphRuntimeStateWithDefaults()

	tmpl, err := prompt.GetTemplate(prompt.WorkflowSQLGeneratorSystem)
	if err != nil {
		t.Fatalf("GetTemplate returned error: %v", err)
	}
	expectedSystemPrompt, err := tmpl.Render(struct{}{})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	mockLLM := &stubLLMInvoker{response: "SELECT 1;"}
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
			Prompt: PromptSection{
				User: "List users",
			},
			Execution: ExecutionConfig{TimeoutSeconds: 5},
		},
		llmInvoker: mockLLM,
	}
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
	if got := mockLLM.lastReq.Messages[0].Content; got != expectedSystemPrompt {
		t.Fatalf("system prompt = %#v, want embedded template", got)
	}
	if _, exists := result.Inputs["system_prompt_source"]; exists {
		t.Fatalf("input system_prompt_source should be omitted from frontend input snapshot")
	}
	if got := result.ProcessData["system_prompt_source"]; got != systemPromptSourceEmbeddedDefault {
		t.Fatalf("process system_prompt_source = %#v, want %q", got, systemPromptSourceEmbeddedDefault)
	}
}

func TestNodeExecuteRunPrefersNodeSystemPrompt(t *testing.T) {
	ctx := context.Background()
	state := entities.NewGraphRuntimeStateWithDefaults()

	mockLLM := &stubLLMInvoker{response: "SELECT 1;"}
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
			Prompt: PromptSection{
				System: "Node system prompt",
				User:   "List users",
			},
			Execution: ExecutionConfig{TimeoutSeconds: 5},
		},
		llmInvoker: mockLLM,
	}
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
	if got := mockLLM.lastReq.Messages[0].Content; got != "Node system prompt" {
		t.Fatalf("system prompt = %#v, want node override", got)
	}
	if _, exists := result.Inputs["system_prompt_source"]; exists {
		t.Fatalf("input system_prompt_source should be omitted from frontend input snapshot")
	}
	if got := result.ProcessData["system_prompt_source"]; got != systemPromptSourceNodeOverride {
		t.Fatalf("process system_prompt_source = %#v, want %q", got, systemPromptSourceNodeOverride)
	}
}

type stubLLMInvoker struct {
	response string
	err      error
	lastReq  *InvokeRequest
}

func (s *stubLLMInvoker) Invoke(ctx context.Context, accountID, appID, appType string, req *InvokeRequest) (*InvokeResult, error) {
	s.lastReq = req
	if s.err != nil {
		return nil, s.err
	}
	return &InvokeResult{
		Text:   s.response,
		Finish: "stop",
	}, nil
}
