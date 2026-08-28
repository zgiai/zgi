package skillloop

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

// planningRequestBudget is retained only for callers that do not install the
// run-scoped ContextManager (mainly isolated unit tests and legacy embedding
// points). It validates/clamps output capacity but never mutates transcript
// content. All transcript governance belongs to contextmgr.
type planningRequestBudget struct {
	safeContextLimit      int
	promptBudget          int
	initialOutputTokens   int
	outputTokenLimit      int
	providerManagedOutput bool
	estimateScale         float64
}

type planningRequestBudgetDiagnostics struct {
	safeContextLimit     int
	promptBudget         int
	originalPromptTokens int
	finalPromptTokens    int
	compressionChars     map[string]int
	savedChars           int
	maxTokensClamped     bool
	originalMaxTokens    int
	effectiveMaxTokens   int
	estimateScale        float64
}

func planningRequestBudgetForRun(req RunRequest) planningRequestBudget {
	metadata := currentMetadataForRun(req)
	control := evidenceMapFromAny(metadata["context_control"])
	safeLimit := numericValue(control["agent_context_window"])
	promptBudget := numericValue(control["prompt_budget"])
	if safeLimit <= 0 {
		safeLimit = numericValue(control["safe_context_limit"])
	}
	if safeLimit <= 0 {
		safeLimit = promptBudget
	}
	if promptBudget <= 0 {
		promptBudget = safeLimit
	}
	if safeLimit > 0 && promptBudget > safeLimit {
		promptBudget = safeLimit
	}
	provider, model := planningRequestProviderModel(req)
	return planningRequestBudget{
		safeContextLimit:      safeLimit,
		promptBudget:          promptBudget,
		initialOutputTokens:   numericValue(control["reserved_output_tokens"]),
		outputTokenLimit:      req.PlanningOutputTokenLimit,
		providerManagedOutput: req.NativeAgentLoop,
		estimateScale:         reusablePromptEstimateScale(metadata, provider, model),
	}
}

func planningRequestProviderModel(req RunRequest) (string, string) {
	if req.Prepared == nil || req.Prepared.LLMRequest == nil {
		return "", ""
	}
	provider := strings.TrimSpace(req.Prepared.LLMRequest.Provider)
	if provider == "" && req.Prepared.parts != nil {
		provider = strings.TrimSpace(req.Prepared.parts.Provider)
	}
	return provider, strings.TrimSpace(req.Prepared.LLMRequest.Model)
}

func reusablePromptEstimateScale(metadata map[string]interface{}, provider string, model string) float64 {
	provider = strings.ToLower(strings.TrimSpace(provider))
	model = strings.ToLower(strings.TrimSpace(model))
	if provider == "" || model == "" {
		return 1
	}
	key := provider + "/" + model
	calibrations := evidenceMapFromAny(metadata["prompt_usage_calibration"])
	record := evidenceMapFromAny(calibrations[key])
	if !strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(record["estimate_version"])), "chat_request.v1") {
		return 1
	}
	scale := numericFloatValue(record["prompt_estimate_scale"])
	if scale < 0.25 || scale > 20 {
		return 1
	}
	return scale
}

func numericFloatValue(value interface{}) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		parsed, _ := typed.Float64()
		return parsed
	case string:
		parsed := json.Number(strings.TrimSpace(typed))
		result, _ := parsed.Float64()
		return result
	default:
		return 0
	}
}

func scaledPromptTokens(tokens int, scale float64) int {
	if tokens <= 0 {
		return 0
	}
	if scale <= 0 {
		scale = 1
	}
	return int(math.Ceil(float64(tokens) * scale))
}

func (r *Runner) applyFinalPlanningRequestBudget(request *adapter.ChatRequest, _ []adapter.Message) error {
	if r == nil || request == nil {
		return nil
	}
	estimateScale := r.requestBudget.estimateScale
	if estimateScale <= 0 {
		estimateScale = 1
	}
	diagnostics := planningRequestBudgetDiagnostics{
		safeContextLimit: r.requestBudget.safeContextLimit,
		promptBudget:     r.requestBudget.promptBudget,
		compressionChars: map[string]int{},
		estimateScale:    estimateScale,
	}
	if request.MaxTokens != nil && *request.MaxTokens > 0 {
		diagnostics.originalMaxTokens = *request.MaxTokens
	}
	defer func() { r.diagnostics.requestBudget = diagnostics }()

	estimate := chatRequestPromptEstimate(request)
	promptTokens := scaledPromptTokens(estimate.Tokens, estimateScale)
	diagnostics.originalPromptTokens = promptTokens
	diagnostics.finalPromptTokens = promptTokens
	if r.requestBudget.safeContextLimit <= 0 {
		return nil
	}
	if promptTokens >= r.requestBudget.safeContextLimit {
		return fmt.Errorf("%w: final planning request exceeds safe context limit", ErrInvalidInput)
	}
	remaining := r.requestBudget.safeContextLimit - promptTokens
	if r.requestBudget.providerManagedOutput && diagnostics.originalMaxTokens <= 0 {
		request.MaxTokens = nil
		return nil
	}
	desiredOutputTokens := diagnostics.originalMaxTokens
	if desiredOutputTokens <= 0 {
		desiredOutputTokens = r.requestBudget.initialOutputTokens
	}
	if desiredOutputTokens <= 0 {
		desiredOutputTokens = r.requestBudget.outputTokenLimit
	}
	outputWasClamped := desiredOutputTokens <= 0 || desiredOutputTokens > remaining
	if outputWasClamped {
		desiredOutputTokens = remaining
	}
	if request.MaxTokens == nil || *request.MaxTokens != desiredOutputTokens {
		request.MaxTokens = intPointer(desiredOutputTokens)
	}
	diagnostics.maxTokensClamped = outputWasClamped
	diagnostics.effectiveMaxTokens = desiredOutputTokens
	return nil
}

func intPointer(value int) *int { return &value }
