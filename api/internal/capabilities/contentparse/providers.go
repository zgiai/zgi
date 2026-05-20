package contentparse

import (
	"os"
	"strconv"
	"strings"

	extractmineru "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/mineru"
	"github.com/zgiai/zgi/api/internal/contracts"
)

func DefaultProviderCatalog() *contracts.ParseProviderCatalog {
	return &contracts.ParseProviderCatalog{
		Providers: []contracts.ParseProviderConfig{
			{
				Name:         "local",
				DisplayName:  "Local Parse",
				Type:         contracts.ParseProviderTypeBuiltin,
				Enabled:      true,
				Priority:     1000,
				FallbackOnly: true,
				Adapter:      "hyperparse_sdk",
				Engine:       contracts.ParseEngineLocal,
				Metadata: map[string]any{
					"tier": "builtin",
				},
			},
			{
				Name:        "mineru",
				DisplayName: "MinerU",
				Type:        contracts.ParseProviderTypeBuiltin,
				Enabled:     extractmineru.Configured(),
				Priority:    200,
				Adapter:     "hyperparse_sdk",
				Engine:      contracts.ParseEngineMineru,
				BaseURL:     extractmineru.ConfiguredBaseURL(),
				APIKeyEnv:   extractmineru.APIKeyEnv(),
				TimeoutSec:  extractmineru.TimeoutSeconds(),
				Metadata: map[string]any{
					"tier": "vendor",
					"mode": extractmineru.Mode(),
				},
			},
			{
				Name:        "reducto",
				DisplayName: "Reducto",
				Type:        contracts.ParseProviderTypeBuiltin,
				Enabled:     strings.TrimSpace(os.Getenv("REDUCTO_API_KEY")) != "",
				Priority:    100,
				Adapter:     "hyperparse_sdk",
				Engine:      contracts.ParseEngineReducto,
				BaseURL:     strings.TrimSpace(os.Getenv("REDUCTO_BASE_URL")),
				APIKeyEnv:   "REDUCTO_API_KEY",
				TimeoutSec:  intFromEnv("REDUCTO_TIMEOUT_SECONDS"),
				Metadata: map[string]any{
					"tier": "vendor",
				},
			},
			SystemVLMProviderConfig(false),
			{
				Name:        "hyperparse_api",
				DisplayName: "Hyperparse API",
				Type:        contracts.ParseProviderTypeRemote,
				Enabled:     strings.TrimSpace(os.Getenv("CONTENT_PARSE_HYPERPARSE_API_BASE_URL")) != "",
				Priority:    300,
				Adapter:     "hyperparse_api",
				BaseURL:     strings.TrimSpace(os.Getenv("CONTENT_PARSE_HYPERPARSE_API_BASE_URL")),
				APIKeyEnv:   firstNonEmptyEnvName("CONTENT_PARSE_HYPERPARSE_API_KEY", "HYPERPARSE_API_KEY"),
				TimeoutSec:  intFromEnv("CONTENT_PARSE_HYPERPARSE_API_TIMEOUT_SECONDS"),
				Metadata: map[string]any{
					"tier": "remote",
				},
			},
		},
	}
}

func SystemVLMProviderConfig(enabled bool) contracts.ParseProviderConfig {
	return contracts.ParseProviderConfig{
		Name:         "vlm",
		DisplayName:  "Vision LLM",
		Type:         contracts.ParseProviderTypeBuiltin,
		Enabled:      enabled,
		Priority:     1100,
		FallbackOnly: true,
		Adapter:      "system_vlm",
		Engine:       contracts.ParseEngineVLM,
		Metadata: map[string]any{
			"tier":                 "system",
			"source":               "default_vision_model",
			"uses_env_base_url":    false,
			"requires_model_route": true,
		},
	}
}

func intFromEnv(key string) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return value
}

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func firstNonEmptyEnvName(keys ...string) string {
	for _, key := range keys {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			return key
		}
	}
	return ""
}
