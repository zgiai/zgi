package retrieval

import (
	"context"
	"fmt"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

// GatewayRerankResult represents a single rerank result from gateway
type GatewayRerankResult struct {
	Index int
	Score float64
}

// GatewayRerankService provides rerank functionality via LLM client
type GatewayRerankService struct {
	client      llmclient.LLMClient
	accountID   string
	appID       string
	appType     string
	model       string
	workspaceID string
}

// NewGatewayRerankService creates a new gateway rerank service.
// The llmClient should be obtained from the DI container (ServiceContainer.GetLLMClient()).
func NewGatewayRerankService(llmClient llmclient.LLMClient, accountID, appID, appType, model, workspaceID string) (*GatewayRerankService, error) {
	if llmClient == nil {
		return nil, fmt.Errorf("llmClient is nil")
	}
	if model == "" {
		return nil, fmt.Errorf("rerank model is empty")
	}
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}

	return &GatewayRerankService{
		client:      llmClient,
		accountID:   accountID,
		appID:       appID,
		appType:     appType,
		model:       model,
		workspaceID: workspaceID,
	}, nil
}

// Rerank performs reranking using the gateway
func (s *GatewayRerankService) Rerank(ctx context.Context, query string, documents []string, scoreThreshold *float64, topN *int) ([]GatewayRerankResult, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("rerank service not configured")
	}
	if len(documents) == 0 {
		return nil, fmt.Errorf("no documents to rerank")
	}

	req := &llmadapter.RerankRequest{
		Model:          s.model,
		Query:          query,
		Documents:      documents,
		TopN:           topN,
		ScoreThreshold: scoreThreshold,
	}

	appCtx := &llmclient.AppContext{
		AppID:       s.appID,
		AppType:     s.appType,
		AccountID:   s.accountID,
		WorkspaceID: s.workspaceID,
	}
	resp, err := s.client.AppRerank(ctx, appCtx, req)
	if err != nil {
		return nil, err
	}

	results := make([]GatewayRerankResult, 0, len(resp.Results))
	for _, item := range resp.Results {
		results = append(results, GatewayRerankResult{
			Index: item.Index,
			Score: item.RelevanceScore,
		})
	}

	return results, nil
}

// GetModel returns the model name
func (s *GatewayRerankService) GetModel() string {
	return s.model
}
