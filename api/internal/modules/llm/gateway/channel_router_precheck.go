package gateway

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/google/uuid"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func (r *ChannelRouter) CandidateRoutesForModel(
	ctx context.Context,
	organizationID uuid.UUID,
	modelName string,
	maxSelections int,
) ([]*channelmodel.LLMRoute, error) {
	if r == nil {
		return nil, fmt.Errorf("channel router is nil")
	}

	modelName = normalizeRequestedModelName(modelName)
	llmModel, privateModel, err := r.resolveCandidateModel(ctx, organizationID, modelName)
	if err != nil {
		return nil, err
	}

	routes, err := r.organizationIDRouteRepo.GetEnabledRoutes(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get enabled routes: %w", err)
	}
	return r.candidateRoutesForResolvedModel(ctx, organizationID, modelName, maxSelections, routes, llmModel, privateModel)
}

func (r *ChannelRouter) resolveCandidateModel(
	ctx context.Context,
	organizationID uuid.UUID,
	modelName string,
) (*llmmodel.LLMModel, *llmmodel.CustomModel, error) {
	llmModel, privateModel, err := r.resolveSelectionModel(ctx, organizationID, "", modelName)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, fmt.Errorf("failed to resolve LLM model %q: %w", modelName, err)
		}
		knownGlobalModel, knownErr := r.globalModelExists(ctx, "", modelName)
		if knownErr != nil {
			return nil, nil, fmt.Errorf("failed to check LLM model %q: %w", modelName, knownErr)
		}
		if knownGlobalModel {
			return nil, nil, llmerrors.NewModelNotFoundErrorWithName(modelName)
		}
		logger.DebugContext(ctx, "candidate LLM route model not found in local registries, using passthrough mode",
			zap.String("organization_id", organizationID.String()),
			zap.String("model", modelName),
		)
		return nil, nil, nil
	}
	return llmModel, privateModel, nil
}

func (r *ChannelRouter) CandidateRoutesForModels(
	ctx context.Context,
	organizationID uuid.UUID,
	modelNames []string,
	maxSelections int,
) (map[string][]*channelmodel.LLMRoute, error) {
	if r == nil {
		return nil, fmt.Errorf("channel router is nil")
	}

	routes, err := r.organizationIDRouteRepo.GetEnabledRoutes(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get enabled routes: %w", err)
	}
	if len(routes) == 0 && len(modelNames) > 0 {
		return nil, fmt.Errorf("no enabled routes found for organizationID %s", organizationID)
	}

	result := make(map[string][]*channelmodel.LLMRoute, len(modelNames))
	for _, modelName := range modelNames {
		normalizedModelName := normalizeRequestedModelName(modelName)
		if normalizedModelName == "" {
			continue
		}
		llmModel, privateModel, err := r.resolveCandidateModel(ctx, organizationID, normalizedModelName)
		if err != nil {
			if !errors.Is(err, llmerrors.DomainErrModelNotFound) {
				return nil, err
			}
			result[normalizedModelName] = nil
			continue
		}
		candidates, err := r.candidateRoutesForResolvedModel(ctx, organizationID, normalizedModelName, maxSelections, routes, llmModel, privateModel)
		if err != nil {
			if !errors.Is(err, llmerrors.DomainErrModelNotFound) {
				return nil, err
			}
			result[normalizedModelName] = nil
			continue
		}
		result[normalizedModelName] = candidates
	}
	return result, nil
}

func (r *ChannelRouter) candidateRoutesForResolvedModel(
	ctx context.Context,
	organizationID uuid.UUID,
	modelName string,
	maxSelections int,
	routes []*channelmodel.LLMRoute,
	llmModel *llmmodel.LLMModel,
	privateModel *llmmodel.CustomModel,
) ([]*channelmodel.LLMRoute, error) {
	modelName = normalizeRequestedModelName(modelName)
	isPrivateCustomModel := privateModel != nil
	isPassthroughMode := llmModel == nil && privateModel == nil
	modelProvider := ""
	if llmModel != nil {
		modelProvider = llmModel.Provider
	}

	if len(routes) == 0 {
		return nil, fmt.Errorf("no enabled routes found for organizationID %s", organizationID)
	}

	validRoutes := r.filterRoutesForSelection(routes, modelName, modelProvider, isPrivateCustomModel)
	modelCategory, _ := ctx.Value(shared.ContextKeyModelCategory).(string)
	validRoutes = filterRoutesForNativeProtocol(validRoutes, llmModel, modelCategory)
	if len(validRoutes) == 0 {
		if isPassthroughMode {
			return nil, llmerrors.NewModelNotFoundErrorWithName(modelName)
		}
		return nil, llmerrors.NewModelNotFoundErrorWithName(modelName)
	}

	return r.selectCandidateRoutesForAttemptWindow(validRoutes, maxSelections), nil
}

func (r *ChannelRouter) selectCandidateRoutesForAttemptWindow(routes []*channelmodel.LLMRoute, maxSelections int) []*channelmodel.LLMRoute {
	if len(routes) == 0 || maxSelections <= 0 {
		return nil
	}

	sortedRoutes := append([]*channelmodel.LLMRoute(nil), routes...)
	sort.Slice(sortedRoutes, func(i, j int) bool {
		if sortedRoutes[i].Priority == sortedRoutes[j].Priority {
			return sortedRoutes[i].ID.String() < sortedRoutes[j].ID.String()
		}
		return sortedRoutes[i].Priority > sortedRoutes[j].Priority
	})

	priorityGroups := make(map[int][]*channelmodel.LLMRoute)
	priorities := make([]int, 0)
	for _, route := range sortedRoutes {
		if _, ok := priorityGroups[route.Priority]; !ok {
			priorities = append(priorities, route.Priority)
		}
		priorityGroups[route.Priority] = append(priorityGroups[route.Priority], route)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(priorities)))

	remaining := maxSelections
	selected := make([]*channelmodel.LLMRoute, 0)
	for _, priority := range priorities {
		if remaining <= 0 {
			break
		}
		group := priorityGroups[priority]
		selected = append(selected, group...)
		remaining -= len(group)
	}

	return selected
}

func isOfficialRoute(route *channelmodel.LLMRoute) bool {
	if route == nil {
		return false
	}
	return route.IsOfficial || route.Type == shared.RouteTypeZGICloud
}
