package runtime

import (
	"context"
	"fmt"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/embedding"
)

// gatewayEmbeddingService adapts LLM client embeddings API to embedding.EmbeddingService.
type gatewayEmbeddingService struct {
	client      llmclient.LLMClient
	accountID   string
	appID       string
	appType     string
	model       string
	workspaceID string
}

// NewGatewayEmbeddingService constructs an embedding service using the injected LLM client.
func NewGatewayEmbeddingService(llmClient llmclient.LLMClient, accountID, appID, appType, model, workspaceID string) (embedding.EmbeddingService, error) {
	if llmClient == nil {
		return nil, fmt.Errorf("llmClient is nil")
	}
	if model == "" {
		return nil, fmt.Errorf("embedding model is empty")
	}
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}

	return &gatewayEmbeddingService{
		client:      llmClient,
		accountID:   accountID,
		appID:       appID,
		appType:     appType,
		model:       model,
		workspaceID: workspaceID,
	}, nil
}

func (s *gatewayEmbeddingService) EmbedText(ctx context.Context, text string) ([]float64, error) {
	vectors, err := s.EmbedTexts(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, fmt.Errorf("empty embedding result")
	}
	return vectors[0], nil
}

func (s *gatewayEmbeddingService) EmbedTexts(ctx context.Context, texts []string) ([][]float64, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("embedding service not configured")
	}
	if len(texts) == 0 {
		return nil, fmt.Errorf("no texts to embed")
	}

	req := &llmadapter.EmbeddingsRequest{
		Input: texts,
		Model: s.model,
		User:  s.accountID,
	}

	appCtx := &llmclient.AppContext{
		AppID:       s.appID,
		AppType:     s.appType,
		AccountID:   s.accountID,
		WorkspaceID: s.workspaceID,
	}
	resp, err := s.client.AppEmbed(ctx, appCtx, req)
	if err != nil {
		return nil, err
	}

	embeddings := make([][]float64, 0, len(resp.Data))
	for _, item := range resp.Data {
		embeddings = append(embeddings, float32SliceTo64(item.Embedding))
	}

	return embeddings, nil
}

func (s *gatewayEmbeddingService) GetDimension() int {
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
