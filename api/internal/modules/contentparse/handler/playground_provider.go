package handler

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	extractlocal "github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/local"
	"github.com/zgiai/ginext/internal/capabilities/contentparse/routing"
	"github.com/zgiai/ginext/internal/contracts"
	"github.com/zgiai/ginext/internal/modules/contentparse/service"
)

func (h *PlaygroundHandler) catalogForRequest(c *gin.Context) (*contracts.ParseProviderCatalog, string, error) {
	if h == nil {
		return nil, "", errors.New("content parse playground is not initialized")
	}
	if h.catalogs != nil {
		return h.catalogs.Resolve(c.Request.Context(), parseContextUUID(c, "workspace_id", "tenant_id"))
	}
	if h.catalog == nil {
		return nil, "", errors.New("content parse provider catalog is empty")
	}
	return h.catalog, service.ProviderCatalogSourceDefault, nil
}

func (h *PlaygroundHandler) planRequest(req contracts.ParseRequest, provider string, catalog *contracts.ParseProviderCatalog, health *contracts.ParseHealth) (*routing.RoutePlan, contracts.ParseRequest, string, error) {
	if catalog == nil {
		return nil, req, "", errors.New("content parse provider catalog is empty")
	}
	provider = strings.TrimSpace(provider)
	if provider == "" || provider == "auto" {
		plan, err := h.planner.Plan(req, catalog, health)
		if err != nil {
			return nil, req, "", err
		}
		if plan.Primary == nil {
			return nil, req, "", errors.New("content parse route plan has no primary provider")
		}
		if plan.Primary.EngineName != "" {
			req.EngineHint = plan.Primary.EngineName
		}
		return plan, req, plan.Primary.AdapterName, nil
	}

	for _, item := range catalog.Providers {
		if item.Name != provider {
			continue
		}
		if !item.Enabled {
			return nil, req, "", fmt.Errorf("content parse provider %q is not configured or enabled", provider)
		}
		if !adapterAvailable(health, item.Adapter) {
			return nil, req, "", fmt.Errorf("content parse adapter %q for provider %q is unavailable", item.Adapter, provider)
		}
		if item.Engine != "" {
			req.EngineHint = item.Engine
		}
		return forcedRoutePlan(req.Profile, item), req, item.Adapter, nil
	}

	return nil, req, "", fmt.Errorf("unknown content parse provider %q", provider)
}

func forcedRoutePlan(profile contracts.ParseProfile, provider contracts.ParseProviderConfig) *routing.RoutePlan {
	return &routing.RoutePlan{
		Mode:            profile,
		RequestedEngine: provider.Engine,
		Primary: &routing.RouteCandidate{
			ProviderKey:  provider.Name,
			AdapterName:  provider.Adapter,
			EngineName:   provider.Engine,
			Priority:     provider.Priority,
			FallbackOnly: provider.FallbackOnly,
			Reason: map[string]any{
				"selection": "playground_forced_provider",
			},
		},
		Metadata: map[string]any{
			"forced_provider": true,
		},
	}
}

func buildPlaygroundProviderStatuses(catalog *contracts.ParseProviderCatalog, health *contracts.ParseHealth) []playgroundProviderStatus {
	if catalog == nil {
		return []playgroundProviderStatus{
			{
				Key:         "auto",
				DisplayName: "Auto",
				Type:        string(contracts.ParseProviderTypeBuiltin),
				Configured:  false,
				Available:   false,
				Selectable:  false,
				Status:      "unavailable",
				Reason:      "provider catalog is empty",
			},
		}
	}

	statuses := make([]playgroundProviderStatus, 0, len(catalog.Providers)+1)
	selectableCount := 0
	for _, provider := range catalog.Providers {
		status := buildPlaygroundProviderStatus(provider, health)
		if status.Selectable {
			selectableCount++
		}
		statuses = append(statuses, status)
	}

	statuses = append([]playgroundProviderStatus{
		{
			Key:         "auto",
			DisplayName: "Auto Route",
			Type:        string(contracts.ParseProviderTypeBuiltin),
			Enabled:     selectableCount > 0,
			Configured:  selectableCount > 0,
			Available:   selectableCount > 0,
			Selectable:  selectableCount > 0,
			Status:      statusForAutoProvider(selectableCount),
			Reason:      reasonForAutoProvider(selectableCount),
		},
	}, statuses...)
	return statuses
}

func buildPlaygroundProviderStatus(provider contracts.ParseProviderConfig, health *contracts.ParseHealth) playgroundProviderStatus {
	available := provider.Enabled && adapterAvailable(health, provider.Adapter)
	status := playgroundProviderStatus{
		Key:          provider.Name,
		DisplayName:  provider.DisplayName,
		Type:         string(provider.Type),
		AdapterName:  provider.Adapter,
		EngineName:   provider.Engine,
		Enabled:      provider.Enabled,
		Configured:   provider.Enabled,
		Available:    available,
		Selectable:   available,
		FallbackOnly: provider.FallbackOnly,
		Priority:     provider.Priority,
		Status:       "available",
		Reason:       "provider is ready",
	}
	if !provider.Enabled {
		status.Status = "not_configured"
		status.Reason = "provider is not configured"
		return status
	}
	if !available {
		status.Status = "unavailable"
		status.Reason = "provider adapter is unavailable"
		return status
	}
	if provider.FallbackOnly {
		status.Status = "fallback"
		status.Reason = "system fallback provider"
	}
	return status
}

func statusForAutoProvider(selectableCount int) string {
	if selectableCount > 0 {
		return "available"
	}
	return "unavailable"
}

func reasonForAutoProvider(selectableCount int) string {
	if selectableCount > 0 {
		return "routes to the best selectable provider by policy"
	}
	return "no selectable provider is available"
}

func adapterAvailable(health *contracts.ParseHealth, adapterName string) bool {
	if strings.TrimSpace(adapterName) == "" {
		return false
	}
	if health == nil {
		return true
	}
	for _, adapter := range health.Adapters {
		if adapter.Name == adapterName {
			return adapter.Available
		}
	}
	return false
}

func buildPlaygroundOCRStatuses() []playgroundOCRStatus {
	items := extractlocal.OCREngineStatuses()
	out := make([]playgroundOCRStatus, 0, len(items))
	for _, item := range items {
		out = append(out, playgroundOCRStatus{
			Key:       item.Key,
			Provider:  item.Provider,
			Available: item.Available,
			Default:   item.Default,
			Path:      item.Path,
			Reason:    item.Reason,
		})
	}
	return out
}
