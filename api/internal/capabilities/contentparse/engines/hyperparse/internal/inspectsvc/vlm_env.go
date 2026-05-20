package inspectsvc

import (
	"os"
	"strings"
)

// Vision model environment variables. Prefer VLM_*, keep legacy DASHSCOPE_*,
// and remain compatible with GEMINI_* settings used by /api/hyperparse/parse.
const (
	EnvVLMAPIKey    = "VLM_API_KEY"
	EnvLegacyAPIKey = "DASHSCOPE_API_KEY"
	EnvGeminiAPIKey = "GEMINI_API_KEY"

	EnvVLMBaseURL    = "VLM_BASE_URL"
	EnvLegacyBaseURL = "DASHSCOPE_BASE_URL"
	EnvGeminiBaseURL = "GEMINI_BASE_URL"

	EnvVLMModel    = "VLM_MODEL"
	EnvLegacyModel = "DASHSCOPE_VL_MODEL"
	EnvGeminiModel = "GEMINI_MODEL"

	EnvVLMModelFast    = "VLM_MODEL_FAST"
	EnvLegacyModelFast = "DASHSCOPE_VL_MODEL_FAST"
	EnvGeminiModelFast = "GEMINI_FALLBACK_MODEL"
)

const missingVLMAPIKeyMessage = "missing VLM_API_KEY (or legacy DASHSCOPE_API_KEY/GEMINI_API_KEY)"

func getenvFirst(keys ...string) string {
	for _, key := range keys {
		if s := strings.TrimSpace(os.Getenv(key)); s != "" {
			return s
		}
	}
	return ""
}

// VLMAPIKey reads the API key from VLM_API_KEY or legacy provider-specific keys.
func VLMAPIKey() string {
	return getenvFirst(EnvVLMAPIKey, EnvLegacyAPIKey, EnvGeminiAPIKey)
}

func VLMConfigured() bool {
	return strings.TrimSpace(VLMAPIKey()) != "" &&
		strings.TrimSpace(VLMBaseURL()) != "" &&
		strings.TrimSpace(VLMModel()) != ""
}

func VLMUsingGeminiCompatConfig() bool {
	return getenvFirst(EnvVLMAPIKey, EnvLegacyAPIKey) == "" && getenvFirst(EnvGeminiAPIKey) != ""
}

// VLMBaseURL reads the explicitly configured OpenAI-compatible endpoint.
func VLMBaseURL() string {
	return getenvFirst(EnvVLMBaseURL, EnvLegacyBaseURL, EnvGeminiBaseURL)
}

// VLMModel reads the primary model name. Empty means the model is not configured.
func VLMModel() string {
	return getenvFirst(EnvVLMModel, EnvLegacyModel, EnvGeminiModel)
}

// VLMModelFast reads the optional fast model name.
func VLMModelFast() string {
	return getenvFirst(EnvVLMModelFast, EnvLegacyModelFast, EnvGeminiModelFast)
}
