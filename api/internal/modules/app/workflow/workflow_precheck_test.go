package workflow

import (
	"context"
	"reflect"
	"testing"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
)

type mockWorkflowAppModelPrechecker struct {
	result         *llmclient.AppModelPrecheckResult
	err            error
	called         bool
	receivedModels []string
}

func (m *mockWorkflowAppModelPrechecker) PrecheckAppModels(ctx context.Context, appCtx *llmclient.AppContext, models []string) (*llmclient.AppModelPrecheckResult, error) {
	m.called = true
	m.receivedModels = append([]string(nil), models...)
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

func TestWorkflowServicePrecheckWorkflowRun_UsesRuntimeModelOverride(t *testing.T) {
	svc := &WorkflowService{executor: &WorkflowExecutor{}}
	prechecker := &mockWorkflowAppModelPrechecker{
		result: &llmclient.AppModelPrecheckResult{Status: llmclient.AppModelPrecheckStatusOK},
	}
	svc.executor.SetLLMClient(prechecker)

	workflowMap := map[string]any{
		"graph": map[string]any{
			"nodes": []any{
				map[string]any{"id": "llm-1", "data": map[string]any{"type": "llm", "model": map[string]any{"provider": "openai", "name": "gpt-old"}}},
				map[string]any{"id": "sql-1", "data": map[string]any{"type": "sql-generator", "model": map[string]any{"provider": "openai", "name": "gpt-old"}}},
			},
		},
	}

	result, err := svc.PrecheckWorkflowRun(
		context.Background(),
		workflowMap,
		&llmclient.AppContext{OrganizationID: "org-1", WorkspaceID: "ws-1", BillingSubjectType: llmclient.BillingSubjectTypeWorkspace, AppID: "app-1", AppType: "agent", AccountID: "account-1"},
		map[string]any{"model_config": map[string]any{"provider": "openai", "model": "gpt-override"}},
	)
	if err != nil {
		t.Fatalf("PrecheckWorkflowRun returned error: %v", err)
	}
	if !result.ContainsAICreditNodes {
		t.Fatalf("ContainsAICreditNodes = false, want true")
	}
	if result.Status != WorkflowRunPrecheckStatusOK {
		t.Fatalf("Status = %q, want %q", result.Status, WorkflowRunPrecheckStatusOK)
	}
	if !prechecker.called {
		t.Fatalf("prechecker was not called")
	}
	if !reflect.DeepEqual(prechecker.receivedModels, []string{"gpt-override"}) {
		t.Fatalf("receivedModels = %#v, want %#v", prechecker.receivedModels, []string{"gpt-override"})
	}
}

func TestWorkflowServicePrecheckWorkflowRun_MapsWarningCodes(t *testing.T) {
	svc := &WorkflowService{executor: &WorkflowExecutor{}}
	prechecker := &mockWorkflowAppModelPrechecker{
		result: &llmclient.AppModelPrecheckResult{
			Status: llmclient.AppModelPrecheckStatusWarning,
			Warnings: []llmclient.AppModelPrecheckWarning{
				{Kind: llmclient.AppModelPrecheckWarningOrganizationBalanceLow, CurrentValue: 300, Threshold: 500},
				{Kind: llmclient.AppModelPrecheckWarningPrivateChannelBalanceLow, CurrentValue: 220, Threshold: 500},
			},
		},
	}
	svc.executor.SetLLMClient(prechecker)

	result, err := svc.PrecheckWorkflowRun(
		context.Background(),
		map[string]any{"graph": map[string]any{"nodes": []any{map[string]any{"id": "llm-1", "data": map[string]any{"type": "llm", "model": map[string]any{"provider": "openai", "name": "gpt-4o"}}}}}},
		&llmclient.AppContext{OrganizationID: "org-1", WorkspaceID: "ws-1", AppID: "app-1", AppType: "agent", AccountID: "account-1"},
		nil,
	)
	if err != nil {
		t.Fatalf("PrecheckWorkflowRun returned error: %v", err)
	}
	if result.Status != WorkflowRunPrecheckStatusWarning {
		t.Fatalf("Status = %q, want %q", result.Status, WorkflowRunPrecheckStatusWarning)
	}
	if len(result.Warnings) != 2 {
		t.Fatalf("len(Warnings) = %d, want 2", len(result.Warnings))
	}
	if result.Warnings[0].Code != 207008 {
		t.Fatalf("Warnings[0].Code = %d, want 207008", result.Warnings[0].Code)
	}
	if got := result.Warnings[0].Params["current_value"]; got != int64(300) {
		t.Fatalf("Warnings[0].Params[current_value] = %#v, want %#v", got, int64(300))
	}
	if result.Warnings[1].Code != 207010 {
		t.Fatalf("Warnings[1].Code = %d, want 207010", result.Warnings[1].Code)
	}
}

func TestWorkflowServicePrecheckWorkflowRun_ReturnsUnknownWhenModelCannotBeResolved(t *testing.T) {
	svc := &WorkflowService{executor: &WorkflowExecutor{}}
	prechecker := &mockWorkflowAppModelPrechecker{}
	svc.executor.SetLLMClient(prechecker)

	result, err := svc.PrecheckWorkflowRun(
		context.Background(),
		map[string]any{"graph": map[string]any{"nodes": []any{map[string]any{"id": "image-1", "data": map[string]any{"type": "image-gen", "model": map[string]any{}}}}}},
		&llmclient.AppContext{OrganizationID: "org-1", WorkspaceID: "ws-1", AppID: "app-1", AppType: "agent", AccountID: "account-1"},
		nil,
	)
	if err != nil {
		t.Fatalf("PrecheckWorkflowRun returned error: %v", err)
	}
	if !result.ContainsAICreditNodes {
		t.Fatalf("ContainsAICreditNodes = false, want true")
	}
	if result.Status != WorkflowRunPrecheckStatusUnknown {
		t.Fatalf("Status = %q, want %q", result.Status, WorkflowRunPrecheckStatusUnknown)
	}
	if prechecker.called {
		t.Fatalf("prechecker.called = true, want false")
	}
}

func TestWorkflowServicePrecheckWorkflowRun_CollectsKnowledgeRetrievalModels(t *testing.T) {
	svc := &WorkflowService{executor: &WorkflowExecutor{}}
	prechecker := &mockWorkflowAppModelPrechecker{
		result: &llmclient.AppModelPrecheckResult{Status: llmclient.AppModelPrecheckStatusOK},
	}
	svc.executor.SetLLMClient(prechecker)

	workflowMap := map[string]any{
		"graph": map[string]any{
			"nodes": []any{
				map[string]any{
					"id": "kr-1",
					"data": map[string]any{
						"type":                    "knowledge-retrieval",
						"single_retrieval_config": map[string]any{"provider": "openai", "name": "gpt-4o-mini"},
						"metadata_model_config":   map[string]any{"provider": "openai", "name": "gpt-4.1-mini"},
						"multiple_retrieval_config": map[string]any{
							"reranking_model": map[string]any{"provider": "voyage", "model": "rerank-2"},
							"weights":         map[string]any{"vector_setting": map[string]any{"embedding_provider_name": "openai", "embedding_model_name": "text-embedding-3-large"}},
						},
					},
				},
			},
		},
	}

	result, err := svc.PrecheckWorkflowRun(
		context.Background(),
		workflowMap,
		&llmclient.AppContext{OrganizationID: "org-1", WorkspaceID: "ws-1", AppID: "app-1", AppType: "agent", AccountID: "account-1"},
		nil,
	)
	if err != nil {
		t.Fatalf("PrecheckWorkflowRun returned error: %v", err)
	}
	if result.Status != WorkflowRunPrecheckStatusOK {
		t.Fatalf("Status = %q, want %q", result.Status, WorkflowRunPrecheckStatusOK)
	}
	wantModels := []string{"gpt-4.1-mini", "gpt-4o-mini", "rerank-2", "text-embedding-3-large"}
	if !reflect.DeepEqual(prechecker.receivedModels, wantModels) {
		t.Fatalf("receivedModels = %#v, want %#v", prechecker.receivedModels, wantModels)
	}
}

func TestGraphContainsAICreditNodes(t *testing.T) {
	graph := map[string]any{
		"nodes": []any{
			map[string]any{"id": "answer-1", "data": map[string]any{"type": "answer"}},
			map[string]any{"id": "llm-1", "data": map[string]any{"type": "llm"}},
		},
	}

	if !graphContainsAICreditNodes(graph) {
		t.Fatalf("graphContainsAICreditNodes() = false, want true")
	}
}

func TestGraphContainsAICreditNodes_ReturnsFalseWhenWorkflowDoesNotSpendAICredit(t *testing.T) {
	graph := map[string]any{
		"nodes": []any{
			map[string]any{"id": "start-1", "data": map[string]any{"type": "start"}},
			map[string]any{"id": "answer-1", "data": map[string]any{"type": "answer"}},
		},
	}

	if graphContainsAICreditNodes(graph) {
		t.Fatalf("graphContainsAICreditNodes() = true, want false")
	}
}
