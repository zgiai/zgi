package runtime

import (
	"context"
	"fmt"
	"strings"

	llmdefaultservice "github.com/zgiai/ginext/internal/modules/llm/defaultmodel/service"
	shared_model "github.com/zgiai/ginext/internal/modules/shared/model"
)

// DefaultModelGetter resolves explicit or default models using organization-scoped defaults.
type DefaultModelGetter interface {
	ResolveModelType(ctx context.Context, organizationID string, explicitProvider, explicitModel *string, modelType shared_model.ModelType) (*llmdefaultservice.ResolvedModel, error)
}

// ResolvedModel is the minimal runtime model spec needed by gateway-backed callers.
type ResolvedModel = llmdefaultservice.ResolvedModel

// ModelResolver keeps runtime call sites small while delegating default resolution to the LLM default-model service.
type ModelResolver struct {
	defaultModels DefaultModelGetter
}

func NewModelResolver(defaultModels DefaultModelGetter) *ModelResolver {
	return &ModelResolver{defaultModels: defaultModels}
}

func (r *ModelResolver) Resolve(
	ctx context.Context,
	organizationID string,
	explicitProvider string,
	explicitModel string,
	modelType shared_model.ModelType,
) (*ResolvedModel, error) {
	provider := strings.TrimSpace(explicitProvider)
	model := strings.TrimSpace(explicitModel)
	if model != "" {
		return &ResolvedModel{
			Provider: provider,
			Model:    model,
		}, nil
	}
	if r == nil || r.defaultModels == nil {
		return nil, fmt.Errorf("default model resolver is not configured")
	}
	return r.defaultModels.ResolveModelType(ctx, organizationID, &provider, &model, modelType)
}

func (r *ModelResolver) ResolveFromPointers(
	ctx context.Context,
	organizationID string,
	explicitProvider *string,
	explicitModel *string,
	modelType shared_model.ModelType,
) (*ResolvedModel, error) {
	if explicitModel != nil && strings.TrimSpace(*explicitModel) != "" {
		provider := ""
		if explicitProvider != nil {
			provider = strings.TrimSpace(*explicitProvider)
		}
		return &ResolvedModel{
			Provider: provider,
			Model:    strings.TrimSpace(*explicitModel),
		}, nil
	}
	if r == nil || r.defaultModels == nil {
		return nil, fmt.Errorf("default model resolver is not configured")
	}
	return r.defaultModels.ResolveModelType(ctx, organizationID, explicitProvider, explicitModel, modelType)
}

func (r *ModelResolver) ResolveDefault(
	ctx context.Context,
	organizationID string,
	modelType shared_model.ModelType,
) (*ResolvedModel, error) {
	if r == nil || r.defaultModels == nil {
		return nil, fmt.Errorf("default model resolver is not configured")
	}
	return r.defaultModels.ResolveModelType(ctx, organizationID, nil, nil, modelType)
}
