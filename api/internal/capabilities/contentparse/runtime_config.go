package contentparse

import (
	"strconv"
	"strings"

	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/routing"
	"github.com/zgiai/zgi/api/internal/contracts"
)

// RuntimeConfigForCandidate returns an isolated configuration snapshot for a
// single provider attempt. Credentials never enter process-global state.
func RuntimeConfigForCandidate(catalog *contracts.ParseProviderCatalog, candidate routing.RouteCandidate) *contracts.ParseProviderRuntimeConfig {
	providerKey := strings.ToLower(strings.TrimSpace(candidate.ProviderKey))
	if providerKey == "" || catalog == nil {
		return nil
	}
	for _, provider := range catalog.Providers {
		if strings.ToLower(strings.TrimSpace(provider.Name)) != providerKey {
			continue
		}
		values := runtimeStringMap(provider.Metadata["env_overrides"])
		config := &contracts.ParseProviderRuntimeConfig{
			ProviderKey:    providerKey,
			BaseURL:        strings.TrimSpace(provider.BaseURL),
			TimeoutSeconds: provider.TimeoutSec,
		}
		switch providerKey {
		case "reducto":
			config.Enabled = parseOptionalBool(values["REDUCTO_ENABLED"])
			config.BaseURL = firstRuntimeValue(values["REDUCTO_BASE_URL"], config.BaseURL)
			config.APIKey = strings.TrimSpace(values["REDUCTO_API_KEY"])
			config.TimeoutSeconds = firstPositiveInt(values["REDUCTO_TIMEOUT_SECONDS"], config.TimeoutSeconds)
		case "mineru":
			config.Mode = strings.ToLower(strings.TrimSpace(values["MINERU_MODE"]))
			if config.Mode == "official" {
				config.BaseURL = firstRuntimeValue(values["MINERU_OFFICIAL_BASE_URL"], config.BaseURL)
				config.APIKey = strings.TrimSpace(values["MINERU_OFFICIAL_TOKEN"])
				config.TimeoutSeconds = firstPositiveInt(values["MINERU_OFFICIAL_TIMEOUT_SECONDS"], config.TimeoutSeconds)
				config.PollIntervalSeconds = firstPositiveInt(values["MINERU_OFFICIAL_POLL_INTERVAL_SECONDS"], 0)
				config.ModelVersion = strings.TrimSpace(values["MINERU_OFFICIAL_MODEL_VERSION"])
			} else {
				config.BaseURL = firstRuntimeValue(values["MINERU_API_URL"], config.BaseURL)
				config.APIKey = strings.TrimSpace(values["MINERU_API_TOKEN"])
				config.TimeoutSeconds = firstPositiveInt(values["MINERU_TIMEOUT_SECONDS"], config.TimeoutSeconds)
			}
		}
		return config
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
			text, _ := value.(string)
			text = strings.TrimSpace(text)
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

func firstRuntimeValue(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func firstPositiveInt(raw string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err == nil && value > 0 {
		return value
	}
	return fallback
}

func parseOptionalBool(raw string) *bool {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	value, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return nil
	}
	return &value
}
