package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	"gorm.io/gorm"
)

type modelRepository struct {
	db *gorm.DB
}

const availableModelColumns = "id, provider, name, display_name, use_cases, reasoning, function_calling, structured_output, temperature, top_p, presence_penalty, frequency_penalty, logit_bias, seed, stop, max_stop_sequences, vision, json_mode, streaming, chat_completions, embeddings, image_generation, speech_generation, transcription, moderation, realtime, batch, assistants, responses, system_prompt, logprobs, web_search, file_search, code_interpreter, computer_use, mcp, parallel_tool_calls, reasoning_effort, context_window, max_output_tokens, is_active"

// NewModelRepository creates a new global model repository
func NewModelRepository(db *gorm.DB) ModelRepository {
	return &modelRepository{db: db}
}

func (r *modelRepository) Create(ctx context.Context, m *model.LLMModel) error {
	return r.db.WithContext(ctx).Create(m).Error
}

func (r *modelRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.LLMModel, error) {
	var m model.LLMModel
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&m).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *modelRepository) GetByName(ctx context.Context, name string) (*model.LLMModel, error) {
	var m model.LLMModel
	err := r.db.WithContext(ctx).Where("name = ?", name).First(&m).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *modelRepository) ListByNames(ctx context.Context, names []string) ([]*model.LLMModel, error) {
	if len(names) == 0 {
		return []*model.LLMModel{}, nil
	}

	var models []*model.LLMModel
	err := r.db.WithContext(ctx).
		Where("name IN ?", names).
		Order("name ASC").
		Find(&models).Error
	return models, err
}

func (r *modelRepository) ListAvailableByNames(ctx context.Context, names []string, provider string, useCase string) ([]*model.LLMModel, error) {
	if len(names) == 0 {
		return []*model.LLMModel{}, nil
	}

	query := r.availableModelQuery(ctx, provider, useCase).Where("name IN ?", names)
	var models []*model.LLMModel
	err := query.Order("name ASC").Find(&models).Error
	return models, err
}

func (r *modelRepository) ListAvailableFiltered(ctx context.Context, provider string, useCase string) ([]*model.LLMModel, error) {
	var models []*model.LLMModel
	err := r.availableModelQuery(ctx, provider, useCase).
		Order("sort_order ASC, name ASC").
		Find(&models).Error
	return models, err
}

func (r *modelRepository) GetByProviderAndName(ctx context.Context, provider string, name string) (*model.LLMModel, error) {
	var m model.LLMModel
	err := r.db.WithContext(ctx).Where("provider = ? AND name = ?", provider, name).First(&m).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *modelRepository) List(ctx context.Context, providerID *uuid.UUID, provider string, useCase string, isActive *bool, offset, limit int) ([]*model.LLMModel, int64, error) {
	var models []*model.LLMModel
	var total int64

	query := r.db.WithContext(ctx).Model(&model.LLMModel{})

	if providerID != nil {
		// llm_models does not have provider_id; it references llm_providers via provider name.
		// Use a join to filter models by provider UUID.
		query = query.
			Joins("JOIN llm_providers p ON p.provider = llm_models.provider").
			Where("p.id = ?", *providerID)
	}
	if provider != "" {
		query = query.Where("provider = ?", provider)
	}
	if useCase != "" {
		query = query.Where("? = ANY(use_cases)", useCase)
	}
	if isActive != nil {
		query = query.Where("is_active = ?", *isActive)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Order("sort_order ASC, name ASC").Offset(offset).Limit(limit).Find(&models).Error; err != nil {
		return nil, 0, err
	}

	return models, total, nil
}

func (r *modelRepository) availableModelQuery(ctx context.Context, provider string, useCase string) *gorm.DB {
	query := r.db.WithContext(ctx).
		Model(&model.LLMModel{}).
		Select(availableModelColumns).
		Where("is_active = ?", true)
	if provider != "" {
		query = query.Where("provider = ?", provider)
	}
	if useCase != "" {
		query = query.Where("? = ANY(use_cases)", useCase)
	}
	return query
}

func (r *modelRepository) Update(ctx context.Context, m *model.LLMModel) error {
	return r.db.WithContext(ctx).Save(m).Error
}

func (r *modelRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.LLMModel{}, "id = ?", id).Error
}

func (r *modelRepository) ListByProvider(ctx context.Context, provider string) ([]*model.LLMModel, error) {
	var models []*model.LLMModel
	err := r.db.WithContext(ctx).
		Where("provider = ? AND is_active = true", provider).
		Order("sort_order ASC, name ASC").
		Find(&models).Error
	return models, err
}
