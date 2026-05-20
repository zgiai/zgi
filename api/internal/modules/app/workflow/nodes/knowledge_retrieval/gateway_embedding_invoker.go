package knowledgeretrieval

import (
	"context"
	"fmt"
	"strings"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/embedding"
)

// embeddingInvoker mirrors llmInvoker but for embedding calls.
type embeddingInvoker interface {
	Embed(ctx context.Context, accountID, appID, appType string, texts []string, model string) ([][]float64, error)
}

// gatewayEmbeddingInvoker implements embeddingInvoker via LLM client.
type gatewayEmbeddingInvoker struct {
	client             llmclient.LLMClient
	organizationID     string
	workspaceID        string
	billingSubjectType string
}

// NewGatewayEmbeddingInvoker builds an embedding invoker backed by the LLM client.
// The client should be obtained from the DI container (ServiceContainer.GetLLMClient()).
// workspaceID is a required billing subject for workflow-scoped LLM calls.
// organizationID can be empty and will be resolved by llm client from app context.
func NewGatewayEmbeddingInvoker(client llmclient.LLMClient, organizationID string, workspaceID string, billingSubjectType string) (embeddingInvoker, error) {
	if client == nil {
		return nil, nil
	}
	orgID := strings.TrimSpace(organizationID)
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required for workflow gateway invoker")
	}
	return &gatewayEmbeddingInvoker{
		client:             client,
		organizationID:     orgID,
		workspaceID:        workspaceID,
		billingSubjectType: strings.TrimSpace(billingSubjectType),
	}, nil
}

func (g *gatewayEmbeddingInvoker) Embed(ctx context.Context, accountID, appID, appType string, texts []string, model string) ([][]float64, error) {
	if g == nil || g.client == nil {
		return nil, ErrInvokerNotConfigured
	}

	if len(texts) == 0 {
		return nil, fmt.Errorf("no texts to embed")
	}

	// EncodingFormat and Dimensions should use default values during query
	req := &llmadapter.EmbeddingsRequest{
		Input: texts,
		Model: model,
		User:  accountID,
	}

	appCtx := &llmclient.AppContext{
		AppID:              appID,
		AppType:            appType,
		AccountID:          accountID,
		OrganizationID:     g.organizationID,
		WorkspaceID:        g.workspaceID,
		BillingSubjectType: g.billingSubjectType,
	}
	resp, err := g.client.AppEmbed(ctx, appCtx, req)
	if err != nil {
		return nil, err
	}

	embeddings := make([][]float64, 0, len(resp.Data))
	for _, item := range resp.Data {
		embeddings = append(embeddings, float32SliceTo64(item.Embedding))
	}

	return embeddings, nil
}

// gatewayEmbeddingService adapts embeddingInvoker to pkg/embedding.EmbeddingService.
type gatewayEmbeddingService struct {
	invoker   embeddingInvoker
	accountID string
	appID     string
	appType   string
	model     string
}

func newGatewayEmbeddingService(invoker embeddingInvoker, accountID, appID, appType, model string) embedding.EmbeddingService {
	return &gatewayEmbeddingService{
		invoker:   invoker,
		accountID: accountID,
		appID:     appID,
		appType:   appType,
		model:     model,
	}
}

func (s *gatewayEmbeddingService) EmbedText(ctx context.Context, text string) ([]float64, error) {
	vectors, err := s.invoker.Embed(ctx, s.accountID, s.appID, s.appType, []string{text}, s.model)
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, fmt.Errorf("empty embedding result")
	}
	return vectors[0], nil
}

func (s *gatewayEmbeddingService) EmbedTexts(ctx context.Context, texts []string) ([][]float64, error) {
	return s.invoker.Embed(ctx, s.accountID, s.appID, s.appType, texts, s.model)
}

func (s *gatewayEmbeddingService) GetDimension() int {
	// dimension not provided by gateway response; caller uses model metadata if needed
	return 0
}

func (s *gatewayEmbeddingService) GetModel() string {
	return s.model
}

func float32SliceTo64(src []float32) []float64 {
	dst := make([]float64, len(src))
	for i, v := range src {
		dst[i] = float64(v)
	}
	return dst
}
