package channelprovider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

// Validation defaults
const (
	defaultTimeout    = 30 * time.Second
	defaultMaxRetries = 1
	defaultMaxTokens  = 1
)

// Report field keys
const (
	keyProvider             = "provider"
	keyBaseURL              = "base_url"
	keyModels               = "models"
	keyCheckedAt            = "checked_at"
	keyItems                = "items"
	keyModel                = "model"
	keyUseCase              = "use_case"
	keySuccess              = "success"
	keyMessage              = "message"
	keyResponseTimeMs       = "response_time_ms"
	keyValidationMode       = "validation_mode"
	keySampled              = "sampled"
	keySampleSize           = "sample_size"
	keyValidatedCount       = "validated_count"
	keyPassedCount          = "passed_count"
	keyFailedModels         = "failed_models"
	keyUnvalidatedCount     = "unvalidated_count"
	keyWarningMessages      = "warning_messages"
	keyProbedModels         = "probed_models"
	keyModelListingVerified = "model_listing_verified"
)

// TestResult is the normalized validation result used by channels and credentials.
type TestResult struct {
	Success        bool
	Message        string
	ResponseTimeMs int64
	Model          string
	UseCase        string
	TestMethod     string
	Response       string
}

// TestModel validates a single model with the adapter selected by channel_provider.
func TestModel(ctx context.Context, channelProvider, baseURL, apiKey, modelName string) (*TestResult, error) {
	spec, err := Resolve(channelProvider)
	if err != nil {
		return nil, err
	}

	startTime := time.Now()

	config := &adapter.AdapterConfig{
		ProviderName:        spec.AdapterKey,
		APIKey:              apiKey,
		BaseURL:             baseURL,
		Timeout:             defaultTimeout,
		MaxRetries:          defaultMaxRetries,
		GuardOutboundURL:    outboundURLGuardEnabled(),
		GuardOutboundDNS:    outboundDNSGuardEnabled(),
		AllowPrivateBaseURL: AllowsPrivateBaseURL(spec.Name),
	}

	adapterInstance, err := adapter.NewAdapter(config)
	if err != nil {
		return &TestResult{
			Success:        false,
			Message:        fmt.Sprintf("failed to create adapter: %v", err),
			ResponseTimeMs: time.Since(startTime).Milliseconds(),
			Model:          modelName,
			TestMethod:     testMethodChat,
		}, nil
	}

	maxTokens := defaultMaxTokens

	request := &adapter.ChatRequest{
		Model: modelName,
		Messages: []adapter.Message{
			{
				Role:    "user",
				Content: "hi",
			},
		},
		MaxTokens: &maxTokens,
		Stream:    false,
	}

	response, err := adapterInstance.ChatCompletion(ctx, request)
	responseTime := time.Since(startTime).Milliseconds()
	if err != nil {
		errorMsg := normalizeValidationError(err)
		return &TestResult{
			Success:        false,
			Message:        errorMsg,
			ResponseTimeMs: responseTime,
			Model:          modelName,
			TestMethod:     testMethodChat,
		}, nil
	}

	responseContent := ""
	if len(response.Choices) > 0 {
		if content, ok := response.Choices[0].Message.Content.(string); ok {
			responseContent = content
		}
	}

	return &TestResult{
		Success:        true,
		Message:        "ok",
		ResponseTimeMs: responseTime,
		Model:          modelName,
		TestMethod:     testMethodChat,
		Response:       responseContent,
	}, nil
}

// ValidateModels validates all declared models and returns a validation_report payload.
func ValidateModels(ctx context.Context, channelProvider, baseURL, apiKey string, models []string) (map[string]any, error) {
	spec, err := Resolve(channelProvider)
	if err != nil {
		return nil, err
	}

	items := make([]map[string]any, 0, len(models))
	report := map[string]any{
		keyProvider:  spec.Name,
		keyBaseURL:   baseURL,
		keyModels:    append([]string(nil), models...),
		keyCheckedAt: time.Now().Unix(),
		keyItems:     items,
	}

	if len(models) == 0 {
		return report, nil
	}

	for _, modelName := range models {

		result, testErr := TestModel(ctx, spec.Name, baseURL, apiKey, modelName)
		if testErr != nil {
			return nil, testErr
		}

		item := map[string]any{
			keyModel:          modelName,
			keySuccess:        result.Success,
			keyMessage:        result.Message,
			keyResponseTimeMs: result.ResponseTimeMs,
		}
		items = append(items, item)
		report[keyItems] = items

		if !result.Success {
			return report, fmt.Errorf("model %s validation failed: %s", modelName, result.Message)
		}
	}

	return report, nil
}

func normalizeValidationError(err error) string {
	errorMsg := err.Error()
	lowerMsg := strings.ToLower(errorMsg)
	switch {
	case errors.Is(err, adapter.ErrAuthFailed),
		strings.Contains(errorMsg, "401"),
		strings.Contains(lowerMsg, "unauthorized"),
		strings.Contains(lowerMsg, "authentication failed"),
		strings.Contains(lowerMsg, "invalid api key"),
		strings.Contains(lowerMsg, "invalid_api_key"):
		return providerAPIKeyInvalidMessage
	case strings.Contains(errorMsg, "404"):
		return "model not found or endpoint not available"
	case strings.Contains(errorMsg, "429"):
		return "rate limit exceeded"
	case strings.Contains(lowerMsg, "timeout"):
		return "request timeout"
	default:
		return errorMsg
	}
}
