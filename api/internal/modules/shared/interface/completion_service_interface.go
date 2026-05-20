package interfaces

import "context"

// CompletionService defines the interface for completion-related operations
type CompletionService interface {
	CreateCompletion(ctx context.Context, req interface{}) (interface{}, error)
	GetCompletion(ctx context.Context, id string) (interface{}, error)
	UpdateCompletion(ctx context.Context, id string, req interface{}) (interface{}, error)
	DeleteCompletion(ctx context.Context, id string) error
	GetCompletionsByAppID(ctx context.Context, appID string) (interface{}, error)
	GetPaginatedCompletions(ctx context.Context, filter interface{}, page, limit int) (interface{}, error)
}
