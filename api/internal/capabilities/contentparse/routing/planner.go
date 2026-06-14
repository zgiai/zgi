package routing

import (
	"fmt"

	"github.com/zgiai/zgi/api/internal/contracts"
)

type Planner interface {
	Plan(req contracts.ParseRequest, catalog *contracts.ParseProviderCatalog, health *contracts.ParseHealth) (*RoutePlan, error)
}

type DefaultPlanner struct{}

func NewDefaultPlanner() *DefaultPlanner {
	return &DefaultPlanner{}
}

func (p *DefaultPlanner) Plan(req contracts.ParseRequest, catalog *contracts.ParseProviderCatalog, health *contracts.ParseHealth) (*RoutePlan, error) {
	profile := normalizeProfile(req.Profile)
	plan := &RoutePlan{
		Mode:            profile,
		RequestedEngine: req.EngineHint,
		Metadata: map[string]any{
			"allow_remote": profileAllowsRemote(profile),
			"health_count": healthCount(health),
		},
	}

	if catalog == nil || len(catalog.Providers) == 0 {
		return nil, fmt.Errorf("content parse provider catalog is empty")
	}

	if req.EngineHint != "" {
		for _, provider := range catalog.Providers {
			if !provider.Enabled || provider.Engine != req.EngineHint || !adapterHealthy(health, provider.Adapter) {
				continue
			}
			plan.Primary = candidateFromProvider(provider, map[string]any{
				"selection": "explicit_engine_hint",
			})
			if provider.FallbackOnly {
				plan.Metadata["fallback_primary"] = true
			}
			return plan, nil
		}
	}

	if profileAllowsRemote(profile) {
		if routeProviders, ext := fileExtensionRouteProviders(req.FileName, catalog, health); len(routeProviders) > 0 {
			plan.Primary = candidateFromProvider(routeProviders[0], map[string]any{
				"selection":  "file_extension_auto_route",
				"file_ext":   ext,
				"route_rank": 0,
			})
			if len(routeProviders) > 1 {
				plan.FallbackCandidates = buildCandidates(routeProviders[1:], map[string]any{
					"selection": "file_extension_auto_fallback",
					"file_ext":  ext,
				})
			}
			plan.Metadata["file_ext"] = ext
			plan.Metadata["selection"] = "file_extension_auto_route"
			return plan, nil
		}
	}

	configured := configuredProviders(catalog, health)
	fallbacks := fallbackProviders(catalog, health)

	if !profileAllowsRemote(profile) {
		if len(fallbacks) == 0 {
			return nil, fmt.Errorf("content parse local-first profile requires enabled fallback providers")
		}
		plan.Primary = candidateFromProvider(fallbacks[0], map[string]any{
			"selection": "local_first_primary",
		})
		if len(fallbacks) > 1 {
			plan.FallbackCandidates = buildCandidates(fallbacks[1:], map[string]any{
				"selection": "local_first_fallback",
			})
		}
		return plan, nil
	}

	if len(configured) > 0 {
		plan.Primary = candidateFromProvider(preferredConfiguredProvider(profile, configured), map[string]any{
			"selection": "configured_provider_primary",
		})
		remainingConfigured := configured
		if plan.Primary != nil {
			remainingConfigured = removeProviderByKey(configured, plan.Primary.ProviderKey)
		}
		plan.FallbackCandidates = append(
			plan.FallbackCandidates,
			buildCandidates(remainingConfigured, map[string]any{"selection": "configured_provider_fallback"})...,
		)
		plan.FallbackCandidates = append(
			plan.FallbackCandidates,
			buildCandidates(fallbacks, map[string]any{"selection": "system_fallback"})...,
		)
		return plan, nil
	}

	if len(fallbacks) == 0 {
		return nil, fmt.Errorf("content parse has no enabled providers for profile %q", profile)
	}

	plan.Primary = candidateFromProvider(fallbacks[0], map[string]any{
		"selection": "fallback_only_primary",
	})
	if len(fallbacks) > 1 {
		plan.FallbackCandidates = buildCandidates(fallbacks[1:], map[string]any{
			"selection": "fallback_only_secondary",
		})
	}
	return plan, nil
}

func preferredConfiguredProvider(profile contracts.ParseProfile, items []contracts.ParseProviderConfig) contracts.ParseProviderConfig {
	for _, target := range providerPreference(profile) {
		for _, item := range items {
			if item.Name == target {
				return item
			}
		}
	}
	return items[0]
}

func providerPreference(profile contracts.ParseProfile) []string {
	switch normalizeProfile(profile) {
	case contracts.ParseProfileHighQuality, contracts.ParseProfileLayoutFirst, contracts.ParseProfileDatasetIndex:
		return []string{"reducto", "mineru", "hyperparse_api"}
	case contracts.ParseProfileFast, contracts.ParseProfileFastPreview, contracts.ParseProfileTextFirst:
		return []string{"mineru", "hyperparse_api", "reducto"}
	default:
		return []string{"reducto", "mineru", "hyperparse_api"}
	}
}

func removeProviderByKey(items []contracts.ParseProviderConfig, providerKey string) []contracts.ParseProviderConfig {
	out := make([]contracts.ParseProviderConfig, 0, len(items))
	for _, item := range items {
		if item.Name == providerKey {
			continue
		}
		out = append(out, item)
	}
	return out
}

func healthCount(health *contracts.ParseHealth) int {
	if health == nil {
		return 0
	}
	return len(health.Adapters)
}
