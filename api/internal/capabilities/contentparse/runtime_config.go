package contentparse

import (
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/routing"
	"github.com/zgiai/zgi/api/internal/contracts"
)

// RuntimeEnvOverridesForCandidate returns request-scoped provider settings
// embedded by the provider catalog resolver. Keeping this in the capability
// layer lets every routed consumer execute providers with identical settings.
func RuntimeEnvOverridesForCandidate(catalog *contracts.ParseProviderCatalog, candidate routing.RouteCandidate) map[string]string {
	providerKey := strings.ToLower(strings.TrimSpace(candidate.ProviderKey))
	if providerKey == "" || catalog == nil {
		return nil
	}
	for _, provider := range catalog.Providers {
		if strings.ToLower(strings.TrimSpace(provider.Name)) != providerKey {
			continue
		}
		return runtimeStringMap(provider.Metadata["env_overrides"])
	}
	return nil
}

func runtimeStringMap(raw any) map[string]string {
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
			text := strings.TrimSpace(fmt.Sprint(value))
			if text != "" {
				out[key] = text
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}
