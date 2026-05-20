package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/llm/llmmodel/model"
	"github.com/zgiai/ginext/internal/modules/llm/llmmodel/repository"
)

type privateModelLookupService struct {
	customRepo repository.CustomModelRepository
}

// NewPrivateModelLookupService creates a lookup service for workspace-scoped custom models.
func NewPrivateModelLookupService(customRepo repository.CustomModelRepository) PrivateModelLookupService {
	return &privateModelLookupService{customRepo: customRepo}
}

func (s *privateModelLookupService) ListActiveModelsByNames(ctx context.Context, organizationID uuid.UUID, modelNames []string) ([]*model.CustomModel, error) {
	if s == nil || s.customRepo == nil || organizationID == uuid.Nil || len(modelNames) == 0 {
		return []*model.CustomModel{}, nil
	}

	return s.customRepo.ListByNames(ctx, organizationID, modelNames, boolPtr(true))
}

func (s *privateModelLookupService) ResolveActiveModels(ctx context.Context, organizationID uuid.UUID, modelNames []string) ([]*model.CustomModel, error) {
	records, err := s.ListActiveModelsByNames(ctx, organizationID, modelNames)
	if err != nil {
		return nil, err
	}
	if err := ensureUniquePrivateModels(records); err != nil {
		return nil, err
	}

	return records, nil
}

func (s *privateModelLookupService) ResolveActiveModelsForProvider(ctx context.Context, organizationID uuid.UUID, provider string, modelNames []string) ([]*model.CustomModel, error) {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return s.ResolveActiveModels(ctx, organizationID, modelNames)
	}
	if s == nil || s.customRepo == nil || organizationID == uuid.Nil || len(modelNames) == 0 {
		return []*model.CustomModel{}, nil
	}

	records, err := s.customRepo.ListByNames(ctx, organizationID, modelNames, boolPtr(true))
	if err != nil {
		return nil, err
	}

	filtered := make([]*model.CustomModel, 0, len(records))
	for _, record := range records {
		if record == nil || strings.TrimSpace(record.Provider) != provider {
			continue
		}
		filtered = append(filtered, record)
	}
	if err := ensureUniquePrivateModels(filtered); err != nil {
		return nil, err
	}

	return filtered, nil
}

func (s *privateModelLookupService) ResolveActiveModel(ctx context.Context, organizationID uuid.UUID, modelName string) (*model.CustomModel, error) {
	if strings.TrimSpace(modelName) == "" {
		return nil, nil
	}

	records, err := s.ResolveActiveModels(ctx, organizationID, []string{modelName})
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}

	return records[0], nil
}

func (s *privateModelLookupService) ResolveActiveModelForProvider(ctx context.Context, organizationID uuid.UUID, provider string, modelName string) (*model.CustomModel, error) {
	if strings.TrimSpace(modelName) == "" {
		return nil, nil
	}

	records, err := s.ResolveActiveModelsForProvider(ctx, organizationID, provider, []string{modelName})
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}

	return records[0], nil
}

func (s *privateModelLookupService) LoadActiveModelNameIndexes(ctx context.Context, organizationID uuid.UUID) ([]string, map[string]string, error) {
	if s == nil || s.customRepo == nil || organizationID == uuid.Nil {
		return nil, nil, nil
	}

	records, _, err := s.customRepo.List(ctx, organizationID, nil, "", "", boolPtr(true), 0, 10000)
	if err != nil {
		return nil, nil, err
	}

	exactNames := make([]string, 0, len(records))
	legacyShortNames := make(map[string]string, len(records))
	exactSeen := make(map[string]struct{}, len(records))
	ambiguousShortNames := make(map[string]struct{})
	for _, record := range records {
		if record == nil {
			continue
		}

		modelName := strings.TrimSpace(record.Name)
		if modelName == "" {
			continue
		}

		if _, exists := exactSeen[modelName]; !exists {
			exactNames = append(exactNames, modelName)
			exactSeen[modelName] = struct{}{}
		}
		if strings.Count(modelName, "/") != 1 {
			continue
		}

		parts := strings.SplitN(modelName, "/", 2)
		shortModelName := strings.TrimSpace(parts[1])
		if shortModelName == "" {
			continue
		}
		if _, ambiguous := ambiguousShortNames[shortModelName]; ambiguous {
			continue
		}
		if existing, exists := legacyShortNames[shortModelName]; exists && existing != modelName {
			delete(legacyShortNames, shortModelName)
			ambiguousShortNames[shortModelName] = struct{}{}
			continue
		}
		if _, exists := legacyShortNames[shortModelName]; !exists {
			legacyShortNames[shortModelName] = modelName
		}
	}

	return exactNames, legacyShortNames, nil
}

func ensureUniquePrivateModels(records []*model.CustomModel) error {
	seen := make(map[string]string, len(records))
	for _, record := range records {
		if record == nil {
			continue
		}

		modelName := strings.TrimSpace(record.Name)
		if modelName == "" {
			continue
		}

		providerKey := privateModelProviderKey(record)
		if existingProvider, ok := seen[modelName]; ok {
			if existingProvider != providerKey {
				return fmt.Errorf("private model %q is defined by multiple custom providers (%s, %s)", modelName, existingProvider, providerKey)
			}
			continue
		}

		seen[modelName] = providerKey
	}

	return nil
}

func privateModelProviderKey(record *model.CustomModel) string {
	if record == nil {
		return ""
	}
	provider := strings.TrimSpace(record.Provider)
	if record.ProviderID == uuid.Nil {
		return provider
	}
	if provider == "" {
		return record.ProviderID.String()
	}
	return provider + "/" + record.ProviderID.String()
}
