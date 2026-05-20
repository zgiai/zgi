package routing

import (
	"sort"

	"github.com/zgiai/ginext/internal/contracts"
)

func configuredProviders(catalog *contracts.ParseProviderCatalog, health *contracts.ParseHealth) []contracts.ParseProviderConfig {
	items := filterProviders(catalog, func(item contracts.ParseProviderConfig) bool {
		return item.Enabled && !item.FallbackOnly && adapterHealthy(health, item.Adapter)
	})
	sortProviders(items)
	return items
}

func fallbackProviders(catalog *contracts.ParseProviderCatalog, health *contracts.ParseHealth) []contracts.ParseProviderConfig {
	items := filterProviders(catalog, func(item contracts.ParseProviderConfig) bool {
		return item.Enabled && item.FallbackOnly && adapterHealthy(health, item.Adapter)
	})
	sortProviders(items)
	return items
}

func adapterHealthy(health *contracts.ParseHealth, adapterName string) bool {
	if adapterName == "" {
		return false
	}
	if health == nil {
		return true
	}
	for _, item := range health.Adapters {
		if item.Name == adapterName {
			return item.Available
		}
	}
	return false
}

func filterProviders(catalog *contracts.ParseProviderCatalog, keep func(item contracts.ParseProviderConfig) bool) []contracts.ParseProviderConfig {
	if catalog == nil {
		return nil
	}
	items := make([]contracts.ParseProviderConfig, 0, len(catalog.Providers))
	for _, item := range catalog.Providers {
		if keep(item) {
			items = append(items, item)
		}
	}
	return items
}

func sortProviders(items []contracts.ParseProviderConfig) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Priority != items[j].Priority {
			return items[i].Priority < items[j].Priority
		}
		return items[i].Name < items[j].Name
	})
}
