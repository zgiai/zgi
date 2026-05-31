package knowledge

import (
	"context"
	"strings"
	"testing"

	dataset_service "github.com/zgiai/zgi/api/internal/modules/dataset/service"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

type fakeRetrievalService struct {
	listCalled          bool
	retrieveCalled      bool
	agentRetrieveCalled bool
	lastRequest         dataset_service.KnowledgeRetrieveRequest
	lastScope           dataset_service.KnowledgeScope
}

func (f *fakeRetrievalService) ListAccessibleDatasets(ctx context.Context, scope dataset_service.KnowledgeScope, query string, limit int) ([]dataset_service.KnowledgeDatasetSummary, error) {
	f.listCalled = true
	f.lastScope = scope
	return []dataset_service.KnowledgeDatasetSummary{
		{DatasetID: "ds-1", Name: "Policies", Provider: "vendor"},
	}, nil
}

func (f *fakeRetrievalService) Retrieve(ctx context.Context, req dataset_service.KnowledgeRetrieveRequest) (*dataset_service.KnowledgeRetrieveResponse, error) {
	f.retrieveCalled = true
	f.lastRequest = req
	return &dataset_service.KnowledgeRetrieveResponse{
		Query:   req.Query,
		Context: "internal context",
	}, nil
}

func (f *fakeRetrievalService) RetrieveAgentKnowledge(ctx context.Context, req dataset_service.KnowledgeRetrieveRequest) (*dataset_service.KnowledgeRetrieveResponse, error) {
	f.agentRetrieveCalled = true
	f.lastRequest = req
	return &dataset_service.KnowledgeRetrieveResponse{
		Query:   req.Query,
		Context: "agent context",
	}, nil
}

func TestProviderExposesKnowledgeTools(t *testing.T) {
	provider := NewProvider(&fakeRetrievalService{})
	entity := provider.GetEntity()
	if entity.Identity.Name != ProviderID {
		t.Fatalf("provider name = %q, want %q", entity.Identity.Name, ProviderID)
	}
	for _, name := range []string{ToolListAccessibleKnowledge, ToolRetrieveKnowledge, ToolRetrieveAgentKnowledge} {
		if _, err := provider.GetTool(name); err != nil {
			t.Fatalf("provider missing tool %s: %v", name, err)
		}
	}
}

func TestRetrieveKnowledgeRequiresDatasetIDs(t *testing.T) {
	service := &fakeRetrievalService{}
	tool, err := NewProvider(service).GetTool(ToolRetrieveKnowledge)
	if err != nil {
		t.Fatalf("GetTool() error = %v", err)
	}
	tool = tool.ForkToolRuntime(&tools.ToolRuntime{TenantID: "tenant-1", InvokeFrom: tools.ToolInvokeFromAIChat})
	_, err = tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"query": "refund policy",
	}, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "dataset_ids") {
		t.Fatalf("Invoke() error = %v, want dataset_ids validation", err)
	}
	if service.retrieveCalled {
		t.Fatalf("Retrieve should not be called when dataset_ids are missing")
	}
}

func TestListAccessibleKnowledgeUsesOrganizationScope(t *testing.T) {
	service := &fakeRetrievalService{}
	tool, err := NewProvider(service).GetTool(ToolListAccessibleKnowledge)
	if err != nil {
		t.Fatalf("GetTool() error = %v", err)
	}
	tool = tool.ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "workspace-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "workspace-1",
		},
	})
	_, err = tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"query": "policy",
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if !service.listCalled {
		t.Fatalf("ListAccessibleDatasets was not called")
	}
	if service.lastScope.OrganizationID != "org-1" {
		t.Fatalf("OrganizationID = %q, want %q", service.lastScope.OrganizationID, "org-1")
	}
	if service.lastScope.WorkspaceID != "workspace-1" {
		t.Fatalf("WorkspaceID = %q, want %q", service.lastScope.WorkspaceID, "workspace-1")
	}
}

func TestRetrieveAgentKnowledgeIgnoresDatasetIDs(t *testing.T) {
	service := &fakeRetrievalService{}
	tool, err := NewProvider(service).GetTool(ToolRetrieveAgentKnowledge)
	if err != nil {
		t.Fatalf("GetTool() error = %v", err)
	}
	appID := "agent-1"
	tool = tool.ForkToolRuntime(&tools.ToolRuntime{TenantID: "tenant-1", InvokeFrom: tools.ToolInvokeFromAgent})
	messages, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"query":       "refund policy",
		"dataset_ids": []interface{}{"model-supplied-ds"},
		"top_k":       float64(3),
	}, nil, &appID, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if !service.agentRetrieveCalled {
		t.Fatalf("RetrieveAgentKnowledge was not called")
	}
	if len(service.lastRequest.DatasetIDs) != 0 {
		t.Fatalf("DatasetIDs = %v, want ignored empty slice", service.lastRequest.DatasetIDs)
	}
	if service.lastRequest.Scope.AppID != appID {
		t.Fatalf("AppID = %q, want %q", service.lastRequest.Scope.AppID, appID)
	}
	if got := service.lastRequest.TopK; got != 3 {
		t.Fatalf("TopK = %d, want 3", got)
	}
	if len(messages) != 2 || messages[0].Data["context"] != "agent context" {
		t.Fatalf("messages = %#v, want json context and retriever resources", messages)
	}
}

func TestRetrieveAgentKnowledgeUsesBindingActorAccount(t *testing.T) {
	service := &fakeRetrievalService{}
	tool, err := NewProvider(service).GetTool(ToolRetrieveAgentKnowledge)
	if err != nil {
		t.Fatalf("GetTool() error = %v", err)
	}
	appID := "agent-1"
	tool = tool.ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "tenant-1",
		InvokeFrom: tools.ToolInvokeFromAgent,
		RuntimeParameters: map[string]interface{}{
			"knowledge_bound_by_account_id": "binder-1",
			"knowledge_binding_grant":       true,
			"knowledge_dataset_ids":         []string{"ds-1"},
		},
	})

	_, err = tool.Invoke(context.Background(), "caller-1", map[string]interface{}{
		"query": "refund policy",
	}, nil, &appID, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if service.lastRequest.Scope.AccountID != "binder-1" {
		t.Fatalf("Scope.AccountID = %q, want binder-1", service.lastRequest.Scope.AccountID)
	}
	if got := service.lastRequest.DatasetIDs; len(got) != 1 || got[0] != "ds-1" {
		t.Fatalf("DatasetIDs = %v, want [ds-1]", got)
	}
	if !service.lastRequest.AgentBindingGrant {
		t.Fatalf("AgentBindingGrant = false, want true")
	}
}
