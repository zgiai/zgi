package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	llmcache "github.com/zgiai/zgi/api/internal/modules/llm/cache"
	defaultmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/model"
	defaultmodelrepo "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/repository"
	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	llmmodelrepo "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/repository"
	llmmodelservice "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
	llmsharedtypes "github.com/zgiai/zgi/api/internal/modules/llm/shared/types"
	sharedmodel "github.com/zgiai/zgi/api/internal/modules/shared/model"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

var (
	ErrInvalidUseCase         = errors.New("invalid use case")
	ErrInvalidParams          = errors.New("params must be a JSON object")
	ErrModelUnavailable       = errors.New("model is not available for organization")
	ErrDefaultModelNotFound   = errors.New("default model not found")
	ErrOrganizationIDRequired = errors.New("organization id is required")
)

const (
	SourceExplicit = "explicit"
	SourceAuto     = "auto"
	SourceNone     = "none"
)

type ResolvedModel struct {
	UseCase         string                    `json:"use_case"`
	Provider        string                    `json:"provider"`
	Model           string                    `json:"model"`
	ContextWindow   int                       `json:"context_window,omitempty"`
	MaxOutputTokens int                       `json:"max_output_tokens,omitempty"`
	Params          llmsharedtypes.JSONObject `json:"params"`
	Source          string                    `json:"source"`
}

type DefaultModelResolver interface {
	ResolveModelType(ctx context.Context, organizationID string, explicitProvider, explicitModel *string, modelType sharedmodel.ModelType) (*ResolvedModel, error)
	ResolveUseCase(ctx context.Context, organizationID string, useCase llmmodelmodel.UseCase, explicitProvider, explicitModel *string) (*ResolvedModel, error)
}

type DefaultModelService interface {
	DefaultModelResolver
	ListResolved(ctx context.Context, organizationID uuid.UUID) ([]*ResolvedModel, error)
	Upsert(ctx context.Context, organizationID uuid.UUID, actorID *uuid.UUID, useCase llmmodelmodel.UseCase, provider string, modelName string, params llmsharedtypes.JSONObject) (*defaultmodelmodel.DefaultModel, error)
	Delete(ctx context.Context, organizationID uuid.UUID, useCase llmmodelmodel.UseCase) error
}

type defaultModelService struct {
	repo               defaultmodelrepo.DefaultModelRepository
	availableModelsSvc llmmodelservice.AvailableModelsService
	globalRepo         llmmodelrepo.ModelRepository
	customRepo         llmmodelrepo.CustomModelRepository
	resolvedCacheGroup singleflight.Group
}

type modelSortMetadata struct {
	isRecommended bool
	isFeatured    bool
	sortOrder     int
}

type rankedCandidate struct {
	model    *llmmodelservice.AvailableModel
	metadata modelSortMetadata
}

func NewDefaultModelService(
	repo defaultmodelrepo.DefaultModelRepository,
	availableModelsSvc llmmodelservice.AvailableModelsService,
	globalRepo llmmodelrepo.ModelRepository,
	customRepo llmmodelrepo.CustomModelRepository,
) DefaultModelService {
	return &defaultModelService{
		repo:               repo,
		availableModelsSvc: availableModelsSvc,
		globalRepo:         globalRepo,
		customRepo:         customRepo,
	}
}

func (s *defaultModelService) ListResolved(ctx context.Context, organizationID uuid.UUID) ([]*ResolvedModel, error) {
	generation := llmcache.Generation(ctx, organizationID.String())
	globalGeneration := llmcache.GlobalGeneration(ctx)
	var cached []*ResolvedModel
	if llmcache.GetJSON(ctx, "default", organizationID.String(), generation, []string{globalGeneration}, &cached) {
		return cached, nil
	}
	value, err, _ := s.resolvedCacheGroup.Do(strings.Join([]string{organizationID.String(), generation, globalGeneration}, "\x00"), func() (interface{}, error) {
		fillCtx, cancel := llmcache.FillContext(ctx)
		defer cancel()

		var cached []*ResolvedModel
		if llmcache.GetJSON(fillCtx, "default", organizationID.String(), generation, []string{globalGeneration}, &cached) {
			return cached, nil
		}

		results, err := s.listResolvedUncached(fillCtx, organizationID)
		if err != nil {
			return nil, err
		}
		llmcache.SetJSON(fillCtx, "default", organizationID.String(), generation, []string{globalGeneration}, results)
		return results, nil
	})
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return value.([]*ResolvedModel), nil
}

func (s *defaultModelService) listResolvedUncached(ctx context.Context, organizationID uuid.UUID) ([]*ResolvedModel, error) {
	defaults, err := s.repo.ListByOrganization(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	explicitByUseCase := make(map[string]*defaultmodelmodel.DefaultModel, len(defaults))
	for _, item := range defaults {
		explicitByUseCase[item.UseCase] = item
	}

	availableModels, err := s.availableModelsSvc.ListAvailable(ctx, organizationID, "", "")
	if err != nil {
		return nil, err
	}

	rankedCandidates, err := s.buildRankedCandidates(ctx, organizationID, availableModels)
	if err != nil {
		return nil, err
	}

	results := make([]*ResolvedModel, 0, len(llmmodelmodel.ValidUseCases()))
	for _, useCase := range llmmodelmodel.ValidUseCases() {
		resolved := s.resolveFromCandidates(useCase, explicitByUseCase[string(useCase)], rankedCandidates)
		results = append(results, resolved)
	}

	return results, nil
}

func (s *defaultModelService) Upsert(ctx context.Context, organizationID uuid.UUID, actorID *uuid.UUID, useCase llmmodelmodel.UseCase, provider string, modelName string, params llmsharedtypes.JSONObject) (*defaultmodelmodel.DefaultModel, error) {
	if !isValidUseCase(useCase) {
		return nil, ErrInvalidUseCase
	}
	provider = strings.TrimSpace(provider)
	modelName = strings.TrimSpace(modelName)
	if provider == "" || modelName == "" {
		return nil, ErrModelUnavailable
	}
	params = normalizeParams(params)

	resolved, err := s.resolveExplicitUseCase(ctx, organizationID, useCase, provider, modelName)
	if err != nil {
		return nil, err
	}

	item := &defaultmodelmodel.DefaultModel{
		OrganizationID: organizationID,
		UseCase:        string(useCase),
		Provider:       resolved.Provider,
		Model:          resolved.Model,
		Params:         params,
		CreatedBy:      actorID,
		UpdatedBy:      actorID,
	}
	if err := s.repo.Upsert(ctx, item); err != nil {
		return nil, err
	}
	llmcache.Invalidate(context.Background(), organizationID.String())
	return item, nil
}

func (s *defaultModelService) Delete(ctx context.Context, organizationID uuid.UUID, useCase llmmodelmodel.UseCase) error {
	if !isValidUseCase(useCase) {
		return ErrInvalidUseCase
	}
	if err := s.repo.DeleteByOrganizationAndUseCase(ctx, organizationID, string(useCase)); err != nil {
		return err
	}
	llmcache.Invalidate(context.Background(), organizationID.String())
	return nil
}

func (s *defaultModelService) ResolveModelType(ctx context.Context, organizationID string, explicitProvider, explicitModel *string, modelType sharedmodel.ModelType) (*ResolvedModel, error) {
	useCase, err := mapModelTypeToUseCase(modelType)
	if err != nil {
		return nil, err
	}
	return s.ResolveUseCase(ctx, organizationID, useCase, explicitProvider, explicitModel)
}

func (s *defaultModelService) ResolveUseCase(ctx context.Context, organizationID string, useCase llmmodelmodel.UseCase, explicitProvider, explicitModel *string) (*ResolvedModel, error) {
	if !isValidUseCase(useCase) {
		return nil, ErrInvalidUseCase
	}
	organizationUUID, err := parseOrganizationID(organizationID)
	if err != nil {
		return nil, err
	}

	if explicitModel != nil && strings.TrimSpace(*explicitModel) != "" {
		provider := ""
		if explicitProvider != nil {
			provider = *explicitProvider
		}
		return s.resolveExplicitUseCase(ctx, organizationUUID, useCase, provider, *explicitModel)
	}

	item, err := s.repo.GetByOrganizationAndUseCase(ctx, organizationUUID, string(useCase))
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if item != nil {
		resolved, resolveErr := s.resolveExplicitUseCase(ctx, organizationUUID, useCase, item.Provider, item.Model)
		if resolveErr == nil {
			resolved.Params = normalizeParams(item.Params)
			resolved.Source = SourceExplicit
			return resolved, nil
		}
	}

	return s.resolveAutoUseCase(ctx, organizationUUID, useCase)
}

func (s *defaultModelService) resolveAutoUseCase(ctx context.Context, organizationID uuid.UUID, useCase llmmodelmodel.UseCase) (*ResolvedModel, error) {
	availableModels, err := s.availableModelsSvc.ListAvailable(ctx, organizationID, "", string(useCase))
	if err != nil {
		return nil, err
	}

	rankedCandidates, err := s.buildRankedCandidates(ctx, organizationID, availableModels)
	if err != nil {
		return nil, err
	}

	candidate := s.selectBestCandidate(rankedCandidates)
	if candidate == nil {
		return &ResolvedModel{
			UseCase: string(useCase),
			Params:  llmsharedtypes.JSONObject{},
			Source:  SourceNone,
		}, nil
	}

	return &ResolvedModel{
		UseCase:         string(useCase),
		Provider:        candidate.Provider,
		Model:           candidate.Name,
		ContextWindow:   candidate.ContextWindow,
		MaxOutputTokens: candidate.MaxOutputTokens,
		Params:          llmsharedtypes.JSONObject{},
		Source:          SourceAuto,
	}, nil
}

func (s *defaultModelService) resolveExplicitUseCase(ctx context.Context, organizationID uuid.UUID, useCase llmmodelmodel.UseCase, provider string, modelName string) (*ResolvedModel, error) {
	modelName = strings.TrimSpace(modelName)
	provider = strings.TrimSpace(provider)
	if modelName == "" {
		return nil, ErrModelUnavailable
	}

	availableModels, err := s.availableModelsSvc.ListAvailable(ctx, organizationID, provider, string(useCase))
	if err != nil {
		return nil, err
	}

	sort.SliceStable(availableModels, func(i, j int) bool {
		if availableModels[i].Provider != availableModels[j].Provider {
			return availableModels[i].Provider < availableModels[j].Provider
		}
		return availableModels[i].Name < availableModels[j].Name
	})

	for _, item := range availableModels {
		if item == nil || strings.TrimSpace(item.Name) != modelName {
			continue
		}
		if provider != "" && strings.TrimSpace(item.Provider) != provider {
			continue
		}
		return &ResolvedModel{
			UseCase:         string(useCase),
			Provider:        strings.TrimSpace(item.Provider),
			Model:           strings.TrimSpace(item.Name),
			ContextWindow:   item.ContextWindow,
			MaxOutputTokens: item.MaxOutputTokens,
			Params:          llmsharedtypes.JSONObject{},
			Source:          SourceExplicit,
		}, nil
	}

	return nil, ErrModelUnavailable
}

func (s *defaultModelService) resolveFromCandidates(useCase llmmodelmodel.UseCase, explicit *defaultmodelmodel.DefaultModel, rankedCandidates []*rankedCandidate) *ResolvedModel {
	if explicit != nil {
		for _, candidate := range rankedCandidates {
			if candidate == nil || candidate.model == nil {
				continue
			}
			if candidate.model.Provider == explicit.Provider && candidate.model.Name == explicit.Model && containsUseCase(candidate.model.UseCases, string(useCase)) {
				return &ResolvedModel{
					UseCase:         string(useCase),
					Provider:        explicit.Provider,
					Model:           explicit.Model,
					ContextWindow:   candidate.model.ContextWindow,
					MaxOutputTokens: candidate.model.MaxOutputTokens,
					Params:          normalizeParams(explicit.Params),
					Source:          SourceExplicit,
				}
			}
		}
	}

	candidate := s.selectBestCandidate(filterRankedCandidatesByUseCase(rankedCandidates, string(useCase)))
	if candidate == nil {
		return &ResolvedModel{
			UseCase: string(useCase),
			Params:  llmsharedtypes.JSONObject{},
			Source:  SourceNone,
		}
	}

	return &ResolvedModel{
		UseCase:         string(useCase),
		Provider:        candidate.Provider,
		Model:           candidate.Name,
		ContextWindow:   candidate.ContextWindow,
		MaxOutputTokens: candidate.MaxOutputTokens,
		Params:          llmsharedtypes.JSONObject{},
		Source:          SourceAuto,
	}
}

func (s *defaultModelService) buildRankedCandidates(ctx context.Context, organizationID uuid.UUID, availableModels []*llmmodelservice.AvailableModel) ([]*rankedCandidate, error) {
	if len(availableModels) == 0 {
		return []*rankedCandidate{}, nil
	}

	globalModels, _, err := s.globalRepo.List(ctx, nil, "", "", "", nil, 0, 5000)
	if err != nil {
		return nil, err
	}
	customModels, _, err := s.customRepo.List(ctx, organizationID, nil, "", "", nil, 0, 5000)
	if err != nil {
		return nil, err
	}

	globalMeta := make(map[string]modelSortMetadata, len(globalModels))
	for _, item := range globalModels {
		if item == nil {
			continue
		}
		globalMeta[candidateKey(item.Provider, item.Model)] = modelSortMetadata{
			isRecommended: item.IsRecommended,
			isFeatured:    item.IsFeatured,
			sortOrder:     item.SortOrder,
		}
	}

	customMeta := make(map[string]modelSortMetadata, len(customModels))
	for _, item := range customModels {
		if item == nil {
			continue
		}
		customMeta[candidateKey(item.Provider, item.Name)] = modelSortMetadata{
			isRecommended: false,
			isFeatured:    item.IsFeatured,
			sortOrder:     item.SortOrder,
		}
	}

	candidates := make([]*rankedCandidate, 0, len(availableModels))
	for _, item := range availableModels {
		if item == nil {
			continue
		}
		meta, ok := globalMeta[candidateKey(item.Provider, item.Name)]
		if !ok {
			meta, ok = customMeta[candidateKey(item.Provider, item.Name)]
		}
		if !ok {
			meta = modelSortMetadata{sortOrder: 999999}
		}
		candidates = append(candidates, &rankedCandidate{
			model:    item,
			metadata: meta,
		})
	}

	return candidates, nil
}

func (s *defaultModelService) selectBestCandidate(candidates []*rankedCandidate) *llmmodelservice.AvailableModel {
	if len(candidates) == 0 {
		return nil
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		left := candidates[i]
		right := candidates[j]
		if left.metadata.isRecommended != right.metadata.isRecommended {
			return left.metadata.isRecommended
		}
		if left.metadata.isFeatured != right.metadata.isFeatured {
			return left.metadata.isFeatured
		}
		if left.metadata.sortOrder != right.metadata.sortOrder {
			return left.metadata.sortOrder < right.metadata.sortOrder
		}
		if left.model.Provider != right.model.Provider {
			return left.model.Provider < right.model.Provider
		}
		return left.model.Name < right.model.Name
	})

	return candidates[0].model
}

func filterRankedCandidatesByUseCase(candidates []*rankedCandidate, useCase string) []*rankedCandidate {
	filtered := make([]*rankedCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate == nil || candidate.model == nil {
			continue
		}
		if containsUseCase(candidate.model.UseCases, useCase) {
			filtered = append(filtered, candidate)
		}
	}
	return filtered
}

func normalizeParams(params llmsharedtypes.JSONObject) llmsharedtypes.JSONObject {
	if params == nil {
		return llmsharedtypes.JSONObject{}
	}
	return params
}

func parseOrganizationID(organizationID string) (uuid.UUID, error) {
	organizationID = strings.TrimSpace(organizationID)
	if organizationID == "" {
		return uuid.Nil, ErrOrganizationIDRequired
	}
	return uuid.Parse(organizationID)
}

func mapModelTypeToUseCase(modelType sharedmodel.ModelType) (llmmodelmodel.UseCase, error) {
	switch modelType {
	case sharedmodel.ModelTypeLLM:
		return llmmodelmodel.UseCaseTextChat, nil
	case sharedmodel.ModelTypeEmbedding:
		return llmmodelmodel.UseCaseEmbedding, nil
	case sharedmodel.ModelTypeRerank:
		return llmmodelmodel.UseCaseRerank, nil
	case sharedmodel.ModelTypeModeration:
		return llmmodelmodel.UseCaseModeration, nil
	case sharedmodel.ModelTypeSpeech2Text:
		return llmmodelmodel.UseCaseSpeechToText, nil
	case sharedmodel.ModelTypeTTS:
		return llmmodelmodel.UseCaseTextToSpeech, nil
	default:
		return "", fmt.Errorf("unsupported model type %s", modelType)
	}
}

func isValidUseCase(useCase llmmodelmodel.UseCase) bool {
	for _, item := range llmmodelmodel.ValidUseCases() {
		if item == useCase {
			return true
		}
	}
	return false
}

func containsUseCase(useCases []string, useCase string) bool {
	for _, item := range useCases {
		if item == useCase {
			return true
		}
	}
	return false
}

func candidateKey(provider string, model string) string {
	return strings.TrimSpace(provider) + "::" + strings.TrimSpace(model)
}
