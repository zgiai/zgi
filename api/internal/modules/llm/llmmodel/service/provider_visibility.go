package service

import (
	"context"
	"strings"

	"github.com/google/uuid"
	providerrepo "github.com/zgiai/ginext/internal/modules/llm/provider/repository"
)

type providerVisibility struct {
	global      map[string]bool
	custom      map[string]bool
	constrained bool
}

func loadProviderVisibility(
	ctx context.Context,
	organizationID uuid.UUID,
	globalProviderRepo providerrepo.ProviderRepository,
	providerConfigRepo providerrepo.ProviderConfigRepository,
	customProviderRepo providerrepo.CustomProviderRepository,
) (*providerVisibility, error) {
	visibility := &providerVisibility{
		global: make(map[string]bool),
		custom: make(map[string]bool),
	}

	if globalProviderRepo != nil && providerConfigRepo != nil {
		visibility.constrained = true

		providers, _, err := globalProviderRepo.List(ctx, boolPtr(true), 0, 1000)
		if err != nil {
			return nil, err
		}

		providerNamesByID := make(map[uuid.UUID]string, len(providers))
		for _, provider := range providers {
			name := normalizeProviderKey(provider.Provider)
			if name == "" {
				continue
			}
			providerNamesByID[provider.ID] = name
			visibility.global[name] = true
		}

		configs, _, err := providerConfigRepo.List(ctx, organizationID, nil, 0, 1000)
		if err != nil {
			return nil, err
		}
		for _, cfg := range configs {
			name, ok := providerNamesByID[cfg.ProviderID]
			if !ok {
				continue
			}
			visibility.global[name] = cfg.IsEnabled
		}
	}

	if customProviderRepo != nil {
		visibility.constrained = true

		customProviders, _, err := customProviderRepo.List(ctx, organizationID, nil, 0, 1000)
		if err != nil {
			return nil, err
		}
		for _, provider := range customProviders {
			name := normalizeProviderKey(provider.Provider)
			if name == "" {
				continue
			}
			visibility.custom[name] = provider.IsActive
		}
	}

	return visibility, nil
}

func (v *providerVisibility) Allows(provider string) bool {
	name := normalizeProviderKey(provider)
	if name == "" {
		return false
	}
	if v == nil || !v.constrained {
		return true
	}
	if enabled, ok := v.custom[name]; ok {
		return enabled
	}
	if enabled, ok := v.global[name]; ok {
		return enabled
	}
	return false
}

func (v *providerVisibility) ShouldQueryGlobal(provider string) bool {
	name := normalizeProviderKey(provider)
	if name == "" {
		return true
	}
	if v == nil || !v.constrained {
		return true
	}
	_, ok := v.global[name]
	return ok
}

func (v *providerVisibility) ShouldQueryCustom(provider string) bool {
	name := normalizeProviderKey(provider)
	if name == "" {
		return true
	}
	if v == nil || !v.constrained {
		return true
	}
	_, ok := v.custom[name]
	return ok
}

func normalizeProviderKey(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}
