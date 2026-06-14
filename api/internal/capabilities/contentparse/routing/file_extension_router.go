package routing

import (
	"path/filepath"
	"strings"

	"github.com/zgiai/zgi/api/internal/contracts"
)

func fileExtensionRouteProviders(fileName string, catalog *contracts.ParseProviderCatalog, health *contracts.ParseHealth) ([]contracts.ParseProviderConfig, string) {
	ext := strings.ToLower(strings.TrimSpace(filepath.Ext(fileName)))
	if ext == "" {
		return nil, ""
	}

	var providers []contracts.ParseProviderConfig
	addProvider := func(name string) {
		if provider, ok := healthyProviderByName(catalog, health, name); ok {
			providers = append(providers, provider)
		}
	}

	switch ext {
	case ".pdf":
		addProvider("mineru")
		addProvider("reducto")
		addProvider("hyperparse_api")
		addProvider("local")
		addProvider("vlm")
	case ".docx", ".doc":
		addProvider("mineru")
		addProvider("hyperparse_api")
		addProvider("local")
		addProvider("reducto")
	case ".xlsx", ".xls", ".csv", ".tsv":
		addProvider("local")
		addProvider("hyperparse_api")
		addProvider("mineru")
		addProvider("reducto")
	case ".png", ".jpg", ".jpeg", ".webp", ".tif", ".tiff":
		addProvider("vlm")
		addProvider("local")
		addProvider("mineru")
	case ".md", ".markdown", ".txt":
		addProvider("local")
		addProvider("hyperparse_api")
	case ".pptx", ".ppt":
		addProvider("mineru")
		addProvider("hyperparse_api")
		addProvider("local")
	default:
		addProvider("local")
		addProvider("mineru")
		addProvider("hyperparse_api")
		addProvider("reducto")
	}

	return providers, ext
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
