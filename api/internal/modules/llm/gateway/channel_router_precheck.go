package gateway

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"
	channelmodel "github.com/zgiai/ginext/internal/modules/llm/channel/model"
	llmerrors "github.com/zgiai/ginext/internal/modules/llm/errors"
	"github.com/zgiai/ginext/internal/modules/llm/shared"
	"github.com/zgiai/ginext/pkg/logger"
	"go.uber.org/zap"
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

	llmModel, privateModel, err := r.resolveSelectionModel(ctx, organizationID, "", modelName)
	isPrivateCustomModel := privateModel != nil
	isPassthroughMode := false
	modelProvider := ""
	if err != nil {
		logger.DebugContext(ctx, "candidate LLM route model not found in local registries, using passthrough mode",
			zap.String("organization_id", organizationID.String()),
			zap.String("model", modelName),
		)
		isPassthroughMode = true
	} else {
		modelProvider = llmModel.Provider
	}

	routes, err := r.organizationIDRouteRepo.GetEnabledRoutes(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get enabled routes: %w", err)
	}
	if len(routes) == 0 {
		return nil, fmt.Errorf("no enabled routes found for organizationID %s", organizationID)
	}

	validRoutes := r.filterRoutesForSelection(routes, modelName, modelProvider, isPrivateCustomModel)
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
