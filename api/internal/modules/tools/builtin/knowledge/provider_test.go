package knowledge

import (
	"context"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/dto"
	dataset_service "github.com/zgiai/zgi/api/internal/modules/dataset/service"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

type fakeRetrievalService struct {
	listCalled          bool
	retrieveCalled      bool
	agentRetrieveCalled bool
	lastRequest         dataset_service.KnowledgeRetrieveRequest
	lastScope           dataset_service.KnowledgeScope
	agentResponse       *dataset_service.KnowledgeRetrieveResponse
	listResponse        *dataset_service.KnowledgeListResponse
}

func (f *fakeRetrievalService) ListAccessibleDatasets(ctx context.Context, scope dataset_service.KnowledgeScope, query string, limit int) (*dataset_service.KnowledgeListResponse, error) {
	f.listCalled = true
	f.lastScope = scope
	if f.listResponse != nil {
		return f.listResponse, nil
	}
	return &dataset_service.KnowledgeListResponse{
		Query:       query,
		Status:      dataset_service.KnowledgeListStatusSuccess,
		ResultCount: 1,
		Limit:       limit,
		KnowledgeBases: []dataset_service.KnowledgeDatasetSummary{
			{DatasetID: "ds-1", Name: "Policies", Provider: "vendor"},
		},
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
	if f.agentResponse != nil {
		return f.agentResponse, nil
	}
	return &dataset_service.KnowledgeRetrieveResponse{
		Query:       req.Query,
		Status:      dataset_service.KnowledgeRetrieveStatusSuccess,
		Context:     "agent context",
		ResultCount: 1,
		SourceSummary: []dataset_service.KnowledgeSourceSummary{{
			Position:     1,
			DatasetName:  "Policies",
			DocumentName: "Refund Policy",
			MatchType:    "hybrid",
			Score:        0.86,
		}},
		ContextBlocks: []dataset_service.KnowledgeContextBlock{{
			Position: 1,
			Source:   "Policies / Refund Policy",
			Score:    0.86,
		}},
		Resources: []dataset_service.KnowledgeRetrieverResource{{
			Position:     1,
			DatasetID:    "ds-1",
			DatasetName:  "Policies",
			DocumentID:   "doc-1",
			DocumentName: "Refund Policy",
			SegmentID:    "seg-1",
			Score:        0.86,
			Content:      "agent context",
			MatchType:    "hybrid",
		}},
		Records: []dto.HitTestingRecordResponse{},
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
	messages, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
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
	if messages[0].Data["status"] != dataset_service.KnowledgeListStatusSuccess {
		t.Fatalf("status = %#v, want success", messages[0].Data["status"])
	}
	if messages[0].Data["result_count"] != 1 {
		t.Fatalf("result_count = %#v, want 1", messages[0].Data["result_count"])
	}
	if _, ok := messages[0].Data["knowledge_bases"]; !ok {
		t.Fatalf("knowledge_bases missing from message: %#v", messages[0].Data)
	}
}

func TestListAccessibleKnowledgeReturnsFallbackStatus(t *testing.T) {
	service := &fakeRetrievalService{
		listResponse: &dataset_service.KnowledgeListResponse{
			Query:        "refund",
			Status:       dataset_service.KnowledgeListStatusFallback,
			ResultCount:  1,
			FallbackUsed: true,
			Limit:        20,
			Warnings:     []string{"no knowledge bases matched the query; showing recent accessible knowledge bases"},
			KnowledgeBases: []dataset_service.KnowledgeDatasetSummary{
				{DatasetID: "ds-1", Name: "Recent Docs", Provider: "vendor"},
			},
		},
	}
	tool, err := NewProvider(service).GetTool(ToolListAccessibleKnowledge)
	if err != nil {
		t.Fatalf("GetTool() error = %v", err)
	}
	tool = tool.ForkToolRuntime(&tools.ToolRuntime{TenantID: "tenant-1", InvokeFrom: tools.ToolInvokeFromAIChat})
	messages, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"query": "refund",
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if messages[0].Data["status"] != dataset_service.KnowledgeListStatusFallback {
		t.Fatalf("status = %#v, want fallback", messages[0].Data["status"])
	}
	if messages[0].Data["fallback_used"] != true {
		t.Fatalf("fallback_used = %#v, want true", messages[0].Data["fallback_used"])
	}
	if _, ok := messages[0].Data["warnings"]; !ok {
		t.Fatalf("warnings missing from message: %#v", messages[0].Data)
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
	if messages[0].Data["status"] != dataset_service.KnowledgeRetrieveStatusSuccess {
		t.Fatalf("status = %#v, want success", messages[0].Data["status"])
	}
	if messages[0].Data["result_count"] != 1 {
		t.Fatalf("result_count = %#v, want 1", messages[0].Data["result_count"])
	}
	if _, ok := messages[0].Data["source_summary"]; !ok {
		t.Fatalf("source_summary missing from message: %#v", messages[0].Data)
	}
	contextBlocks, ok := messages[0].Data["context_blocks"].([]dataset_service.KnowledgeContextBlock)
	if !ok {
		t.Fatalf("context_blocks missing from message: %#v", messages[0].Data)
	}
	if len(contextBlocks) != 1 || contextBlocks[0].Source != "Policies / Refund Policy" {
		t.Fatalf("context_blocks = %#v, want source summary block", contextBlocks)
	}
}

func TestRetrieveAgentKnowledgeReturnsNoConfigPayload(t *testing.T) {
	service := &fakeRetrievalService{
		agentResponse: &dataset_service.KnowledgeRetrieveResponse{
			Query:       "refund policy",
			Status:      dataset_service.KnowledgeRetrieveStatusNoConfig,
			ResultCount: 0,
			Warnings:    []string{"agent has no configured knowledge datasets"},
			Resources:   []dataset_service.KnowledgeRetrieverResource{},
		},
	}
	tool, err := NewProvider(service).GetTool(ToolRetrieveAgentKnowledge)
	if err != nil {
		t.Fatalf("GetTool() error = %v", err)
	}
	appID := "agent-1"
	tool = tool.ForkToolRuntime(&tools.ToolRuntime{TenantID: "tenant-1", InvokeFrom: tools.ToolInvokeFromAgent})
	messages, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"query": "refund policy",
	}, nil, &appID, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if messages[0].Data["status"] != dataset_service.KnowledgeRetrieveStatusNoConfig {
		t.Fatalf("status = %#v, want no_config", messages[0].Data["status"])
	}
	if messages[0].Data["result_count"] != 0 {
		t.Fatalf("result_count = %#v, want 0", messages[0].Data["result_count"])
	}
	if _, ok := messages[0].Data["warnings"]; !ok {
		t.Fatalf("warnings missing from message: %#v", messages[0].Data)
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
			"knowledge_retrieval_config":    map[string]interface{}{"top_k": float64(8)},
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
	if got := service.lastRequest.RetrievalConfig["top_k"]; got != float64(8) {
		t.Fatalf("RetrievalConfig[top_k] = %#v, want 8", got)
	}
}

func TestRetrieveAgentKnowledgeUsesPersistedResourceAuthorization(t *testing.T) {
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
			"knowledge_binding_grant": true,
			"knowledge_dataset_ids":   []string{"ds-new"},
			"agent_binding_authorizations": []map[string]interface{}{{
				"binding_type": "knowledge_dataset", "resource_id": "ds-new", "access_mode": "read", "bound_by_account_id": "resource-binder", "bound_at_unix": int64(200),
			}},
		},
	})

	_, err = tool.Invoke(context.Background(), "caller-1", map[string]interface{}{
		"query": "refund policy",
	}, nil, &appID, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if service.lastRequest.Scope.AccountID != "resource-binder" {
		t.Fatalf("Scope.AccountID = %q, want resource-binder", service.lastRequest.Scope.AccountID)
	}
}
