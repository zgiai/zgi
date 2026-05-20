package embedding

import (
	"context"
)

// EmbeddingService defines the interface for text embedding services
type EmbeddingService interface {
	// EmbedText converts a single text to vector
	EmbedText(ctx context.Context, text string) ([]float64, error)

	// EmbedTexts converts multiple texts to vectors
	EmbedTexts(ctx context.Context, texts []string) ([][]float64, error)

	// GetDimension returns the dimension of the embedding vectors
	GetDimension() int

	// GetModel returns the model name used for embedding
	GetModel() string
}

// EmbeddingResult represents the result of embedding operation
type EmbeddingResult struct {
	Text       string    `json:"text"`
	Vector     []float64 `json:"vector"`
	Dimension  int       `json:"dimension"`
	Model      string    `json:"model"`
	TokensUsed int       `json:"tokens_used,omitempty"`
}

// BatchEmbeddingResult represents batch embedding results
type BatchEmbeddingResult struct {
	Results     []EmbeddingResult `json:"results"`
	TotalTokens int               `json:"total_tokens,omitempty"`
}
