package knowledgeretrieval

import (
	"context"
	"fmt"
	"strings"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

// rerankInvoker mirrors embeddingInvoker but for rerank calls.
type rerankInvoker interface {
	// Rerank returns ordered results with scores for the provided documents.
	Rerank(ctx context.Context, accountID, appID, appType, query string, documents []string, model string, topN int) ([]llmadapter.RerankResult, error)
}

// gatewayRerankInvoker implements rerankInvoker via LLM client.
type gatewayRerankInvoker struct {
	client             llmclient.LLMClient
	organizationID     string
	workspaceID        string
	billingSubjectType string
}

// NewGatewayRerankInvoker builds a rerank invoker backed by the injected LLM client.
// The llmClient should be obtained from the DI container (ServiceContainer.GetLLMClient()).
func NewGatewayRerankInvoker(llmClient llmclient.LLMClient, organizationID string, workspaceID string, billingSubjectType string) (rerankInvoker, error) {
	if llmClient == nil {
		return nil, nil
	}

	orgID := strings.TrimSpace(organizationID)
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required for workflow gateway invoker")
	}

	return &gatewayRerankInvoker{
		client:             llmClient,
		organizationID:     orgID,
		workspaceID:        workspaceID,
		billingSubjectType: strings.TrimSpace(billingSubjectType),
	}, nil
}

func (g *gatewayRerankInvoker) Rerank(ctx context.Context, accountID, appID, appType, query string, documents []string, model string, topN int) ([]llmadapter.RerankResult, error) {
	if g == nil || g.client == nil {
		return nil, ErrInvokerNotConfigured
	}

	if len(documents) == 0 {
		return nil, fmt.Errorf("no documents to rerank")
	}

	req := &llmadapter.RerankRequest{
		Model:     model,
		Query:     query,
		Documents: documents,
	}

	if topN > 0 {
		req.TopN = &topN
	}

	appCtx := &llmclient.AppContext{
		AppID:              appID,
		AppType:            appType,
		AccountID:          accountID,
		OrganizationID:     g.organizationID,
		WorkspaceID:        g.workspaceID,
		BillingSubjectType: g.billingSubjectType,
	}
	resp, err := g.client.AppRerank(ctx, appCtx, req)
	if err != nil {
		return nil, err
	}

	return resp.Results, nil
}
