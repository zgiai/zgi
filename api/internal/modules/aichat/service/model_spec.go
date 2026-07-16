//go:build legacy_aichat_service
// +build legacy_aichat_service

package service

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	llmrepo "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/repository"
	"gorm.io/gorm"
)

type ModelSpec struct {
	ContextWindow    int
	MaxInputTokens   int
	MaxOutputTokens  int
	UseCases         []string
	Vision           bool
	SupportsToolCall bool
}

type ModelSpecResolver interface {
	Resolve(ctx context.Context, organizationID uuid.UUID, provider string, modelName string) (ModelSpec, bool, error)
}

type databaseModelSpecResolver struct {
	globalRepo llmrepo.ModelRepository
	customRepo llmrepo.CustomModelRepository
}

func NewDatabaseModelSpecResolver(db *gorm.DB) ModelSpecResolver {
	return &databaseModelSpecResolver{
		globalRepo: llmrepo.NewModelRepository(db),
		customRepo: llmrepo.NewCustomModelRepository(db),
	}
}

func (r *databaseModelSpecResolver) Resolve(ctx context.Context, organizationID uuid.UUID, provider string, modelName string) (ModelSpec, bool, error) {
	provider = strings.TrimSpace(provider)
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return ModelSpec{}, false, nil
	}
	if r.customRepo != nil {
		spec, ok, err := r.resolveCustom(ctx, organizationID, provider, modelName)
		if err != nil || ok {
			return spec, ok, err
		}
	}
	if r.globalRepo != nil {
		spec, ok, err := r.resolveGlobal(ctx, provider, modelName)
		if err != nil || ok {
			return spec, ok, err
		}
	}
	return ModelSpec{}, false, nil
}

func (r *databaseModelSpecResolver) resolveCustom(ctx context.Context, organizationID uuid.UUID, provider string, modelName string) (ModelSpec, bool, error) {
	var custom *llmmodel.CustomModel
	var err error
	if provider != "" {
		custom, err = r.customRepo.GetByProviderAndModel(ctx, organizationID, provider, modelName)
	} else {
		active := true
		models, listErr := r.customRepo.ListByNames(ctx, organizationID, []string{modelName}, &active)
		err = listErr
		if len(models) > 0 {
			custom = models[0]
		}
	}
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ModelSpec{}, false, nil
		}
		return ModelSpec{}, false, err
	}
	if custom == nil {
		return ModelSpec{}, false, nil
	}
	return ModelSpec{
		ContextWindow:    custom.ContextWindow,
		MaxInputTokens:   custom.MaxInputTokens,
		MaxOutputTokens:  custom.MaxOutputTokens,
		UseCases:         []string(custom.UseCases),
		Vision:           custom.SupportsVision,
		SupportsToolCall: custom.SupportsToolCall,
	}, true, nil
}

func (r *databaseModelSpecResolver) resolveGlobal(ctx context.Context, provider string, modelName string) (ModelSpec, bool, error) {
	var global *llmmodel.LLMModel
	var err error
	if provider != "" {
		global, err = r.globalRepo.GetByProviderAndName(ctx, provider, modelName)
	} else {
		global, err = r.globalRepo.GetByName(ctx, modelName)
	}
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ModelSpec{}, false, nil
		}
		return ModelSpec{}, false, err
	}
	return ModelSpec{
		ContextWindow:    global.ContextWindow,
		MaxInputTokens:   global.MaxInputTokens,
		MaxOutputTokens:  global.MaxOutputTokens,
		UseCases:         []string(global.UseCases),
		Vision:           global.SupportsVision,
		SupportsToolCall: global.SupportsToolCall,
	}, true, nil
}

func (s ModelSpec) SupportsVision() bool {
	if s.Vision {
		return true
	}
	for _, useCase := range s.UseCases {
		if strings.TrimSpace(useCase) == string(llmmodel.UseCaseVision) {
			return true
		}
	}
	return false
}

func (s ModelSpec) SupportsTools() bool {
	return s.SupportsToolCall
}

func (s ModelSpec) SupportsFunctionCalling() bool {
	return s.SupportsToolCall
}
