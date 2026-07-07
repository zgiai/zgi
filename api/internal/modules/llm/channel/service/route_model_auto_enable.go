package service

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	"gorm.io/gorm"
)

func (s *channelService) autoEnableModelsForRoute(ctx context.Context, organizationID uuid.UUID, modelNames []string) (bool, error) {
	if s == nil || s.modelRepo == nil || s.modelConfigRepo == nil || len(modelNames) == 0 {
		return false, nil
	}

	names := normalizeAutoEnableModelNames(modelNames)
	if len(names) == 0 {
		return false, nil
	}

	models, err := s.modelRepo.ListByNames(ctx, names)
	if err != nil {
		return false, err
	}
	if len(models) == 0 {
		return false, nil
	}

	for _, item := range models {
		if item == nil {
			continue
		}

		_, err := s.modelConfigRepo.GetByModelID(ctx, organizationID, item.ID)
		if err == nil {
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return false, err
		}

		config := &llmmodelmodel.ModelConfig{
			OrganizationID: organizationID,
			ModelID:        item.ID,
			IsEnabled:      true,
			AccessScope:    llmmodelmodel.AccessScopeAll,
		}
		if err := s.modelConfigRepo.Upsert(ctx, config); err != nil {
			return false, err
		}
	}

	if s.availableModels != nil {
		s.availableModels.InvalidateTenantCache(organizationID)
		return true, nil
	}

	return false, nil
}

func normalizeAutoEnableModelNames(modelNames []string) []string {
	if len(modelNames) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(modelNames))
	result := make([]string, 0, len(modelNames))
	for _, item := range modelNames {
		name := strings.TrimSpace(item)
		if name == "" || name == "*" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}
	return result
}
