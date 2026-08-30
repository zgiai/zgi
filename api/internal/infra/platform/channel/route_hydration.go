package channel

import (
	"context"
	"sort"
	"strings"

	"github.com/google/uuid"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
)

// HydrateOfficialRoutes replaces the model snapshot on enabled official routes
// with the current platform-channel catalog. Tenant routing settings remain on
// the persisted route; only provider/model availability comes from the platform.
func HydrateOfficialRoutes(
	ctx context.Context,
	provider ChannelProvider,
	organizationID uuid.UUID,
	routes []*channelmodel.LLMRoute,
) ([]*channelmodel.LLMRoute, error) {
	if provider == nil || len(routes) == 0 {
		return routes, nil
	}

	hasOfficialRoute := false
	for _, route := range routes {
		if route != nil && (route.IsOfficial || route.Type == shared.RouteTypeZGICloud) {
			hasOfficialRoute = true
			break
		}
	}
	if !hasOfficialRoute {
		return routes, nil
	}

	channels, err := provider.ListChannels(ctx, organizationID.String())
	if err != nil {
		return nil, err
	}

	models := make([]string, 0)
	providerModels := make([]channelmodel.ProviderModel, 0)
	seenModels := make(map[string]struct{})
	seenProviderModels := make(map[channelmodel.ProviderModel]struct{})
	for _, platformChannel := range channels {
		if platformChannel == nil {
			continue
		}
		channelProvider := strings.TrimSpace(platformChannel.Provider)
		for _, rawModel := range platformChannel.Models {
			modelName := strings.TrimSpace(rawModel)
			if modelName == "" {
				continue
			}
			if _, ok := seenModels[modelName]; !ok {
				seenModels[modelName] = struct{}{}
				models = append(models, modelName)
			}
			if channelProvider == "" {
				continue
			}
			pair := channelmodel.ProviderModel{Provider: channelProvider, Model: modelName}
			if _, ok := seenProviderModels[pair]; ok {
				continue
			}
			seenProviderModels[pair] = struct{}{}
			providerModels = append(providerModels, pair)
		}
	}
	sort.Strings(models)
	sort.Slice(providerModels, func(i, j int) bool {
		if providerModels[i].Provider != providerModels[j].Provider {
			return providerModels[i].Provider < providerModels[j].Provider
		}
		return providerModels[i].Model < providerModels[j].Model
	})

	for _, route := range routes {
		if route == nil || (!route.IsOfficial && route.Type != shared.RouteTypeZGICloud) {
			continue
		}
		route.Models = append([]string(nil), models...)
		route.OfficialProviderModels = append([]channelmodel.ProviderModel(nil), providerModels...)
	}
	return routes, nil
}
