package routing

import (
	"path/filepath"
	"strings"

	"github.com/zgiai/zgi/api/internal/contracts"
)

func fileExtensionRouteProviders(fileName string, catalog *contracts.ParseProviderCatalog, health *contracts.ParseHealth) ([]contracts.ParseProviderConfig, string) {
	names, ext := FileExtensionProviderOrder(fileName)

	var providers []contracts.ParseProviderConfig
	addProvider := func(name string) {
		if provider, ok := healthyProviderByName(catalog, health, name); ok {
			providers = append(providers, provider)
		}
	}
	for _, name := range names {
		addProvider(name)
	}

	return providers, ext
}

func FileExtensionProviderOrder(fileName string) ([]string, string) {
	trimmed := strings.TrimSpace(fileName)
	if trimmed == "" {
		return nil, ""
	}
	ext := strings.ToLower(strings.TrimSpace(filepath.Ext(trimmed)))
	switch ext {
	case ".pdf":
		return []string{"reducto", "mineru", "vlm", "local"}, ext
	case ".docx", ".doc":
		return []string{"reducto", "mineru", "local"}, ext
	case ".xlsx", ".xls", ".csv", ".tsv":
		return []string{"local"}, ext
	case ".png", ".jpg", ".jpeg", ".webp", ".tif", ".tiff":
		return []string{"vlm", "mineru"}, ext
	case ".md", ".markdown", ".txt":
		return []string{"local"}, ext
	case ".pptx", ".ppt":
		return []string{"reducto", "mineru"}, ext
	default:
		return []string{"local"}, ext
	}
}

func FileExtensionAllowsProvider(fileName string, providerName string) bool {
	names, _ := FileExtensionProviderOrder(fileName)
	if len(names) == 0 {
		return true
	}
	providerName = strings.ToLower(strings.TrimSpace(providerName))
	for _, name := range names {
		if name == providerName {
			return true
		}
	}
	return false
}

func healthyProviderByName(catalog *contracts.ParseProviderCatalog, health *contracts.ParseHealth, name string) (contracts.ParseProviderConfig, bool) {
	if catalog == nil {
		return contracts.ParseProviderConfig{}, false
	}
	name = strings.ToLower(strings.TrimSpace(name))
	for _, provider := range catalog.Providers {
		if strings.ToLower(strings.TrimSpace(provider.Name)) != name {
			continue
		}
		if !provider.Enabled || !adapterHealthy(health, provider.Adapter) {
			return contracts.ParseProviderConfig{}, false
		}
		return provider, true
	}
	return contracts.ParseProviderConfig{}, false
}
