package service

import (
	"strings"

	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/routing"
	"github.com/zgiai/zgi/api/internal/contracts"
)

func RuntimeEnvOverridesForCandidate(catalog *contracts.ParseProviderCatalog, candidate routing.RouteCandidate) map[string]string {
	providerKey := strings.ToLower(strings.TrimSpace(candidate.ProviderKey))
	if providerKey == "" || catalog == nil {
		return nil
	}
	for _, provider := range catalog.Providers {
		if strings.ToLower(strings.TrimSpace(provider.Name)) != providerKey {
			continue
		}
		raw, ok := provider.Metadata["env_overrides"]
		if !ok {
			return nil
		}
		return stringMap(raw)
	}
	return nil
}

func stringMap(raw any) map[string]string {
	switch typed := raw.(type) {
	case map[string]string:
		if len(typed) == 0 {
			return nil
		}
		out := make(map[string]string, len(typed))
		for key, value := range typed {
			out[key] = value
		}
		return out
	case map[string]any:
		out := make(map[string]string, len(typed))
		for key, value := range typed {
			text := strings.TrimSpace(metadataString(map[string]any{key: value}, key))
			if text != "" {
				out[key] = text
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	default:
		return nil
	}
}
