package interfaces

import "context"

// ModelConfigService defines the interface for model configuration operations
type ModelConfigService interface {
	CreateModelConfig(ctx context.Context, req interface{}) (interface{}, error)
	GetModelConfig(ctx context.Context, id string) (interface{}, error)
	UpdateModelConfig(ctx context.Context, id string, req interface{}) (interface{}, error)
	DeleteModelConfig(ctx context.Context, id string) error
	GetModelConfigsByAppID(ctx context.Context, appID string) (interface{}, error)
	GetPaginatedModelConfigs(ctx context.Context, filter interface{}, page, limit int) (interface{}, error)
}
