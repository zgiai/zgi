package retrieval

import "context"

// Embedding defines the dataset embedding contract used by indexing and dataset services.
type Embedding interface {
	EmbedQuery(ctx context.Context, text string) ([]float64, error)
	EmbedDocuments(ctx context.Context, texts []string) ([][]float64, error)
	GetModel() string
	GetDimension() int
}
