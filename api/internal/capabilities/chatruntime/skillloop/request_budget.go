package skillloop

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const (
	budgetComponentTurnEvidence       = "turn_evidence_state"
	budgetComponentRestoredSkills     = "non_target_restored_skills"
	budgetComponentHistoricalTools    = "historical_tool_payloads"
	budgetComponentIntermediateAnswer = "historical_intermediate_answers"
)

type planningRequestBudget struct {
	safeContextLimit       int
	promptBudget           int
	outputTokenLimit       int
	estimateScale          float64
	preferredRestoredSkill string
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
	safeLimit := numericValue(control["safe_context_limit"])
	promptBudget := numericValue(control["prompt_budget"])
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
		safeContextLimit:       safeLimit,
		promptBudget:           promptBudget,
		outputTokenLimit:       req.PlanningOutputTokenLimit,
		estimateScale:          reusablePromptEstimateScale(metadata, provider, model),
		preferredRestoredSkill: strings.TrimSpace(req.PreferredRestoredSkillID),
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
		var parsed json.Number = json.Number(strings.TrimSpace(typed))
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

func (r *Runner) applyFinalPlanningRequestBudget(request *adapter.ChatRequest, sourceMessages []adapter.Message) error {
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
	defer func() {
		r.diagnostics.requestBudget = diagnostics
	}()

	estimate := chatRequestPromptEstimate(request)
	scaledEstimateTokens := scaledPromptTokens(estimate.Tokens, estimateScale)
	diagnostics.originalPromptTokens = scaledEstimateTokens
	diagnostics.finalPromptTokens = scaledEstimateTokens
	if r.requestBudget.promptBudget <= 0 && r.requestBudget.safeContextLimit <= 0 {
		return nil
	}

	messages := append([]adapter.Message(nil), sourceMessages...)
	if len(messages) == 0 {
		messages = append([]adapter.Message(nil), request.Messages...)
	}
	stages := []struct {
		name  string
		apply func([]adapter.Message) ([]adapter.Message, bool)
	}{
		{name: budgetComponentTurnEvidence, apply: compactRepeatedTurnEvidenceState},
		{name: budgetComponentRestoredSkills, apply: func(input []adapter.Message) ([]adapter.Message, bool) {
			return compactNonTargetRestoredSkills(input, r.requestBudget.preferredRestoredSkill)
		}},
		{name: budgetComponentHistoricalTools, apply: compactHistoricalToolPayloads},
		{name: budgetComponentIntermediateAnswer, apply: compactHistoricalIntermediateAnswers},
	}
	for _, stage := range stages {
		if scaledEstimateTokens <= r.requestBudget.promptBudget {
			break
		}
		candidateMessages, changed := stage.apply(messages)
		if !changed {
			continue
		}
		candidate := cloneChatRequest(request)
		candidate.Messages = adapter.NormalizeSystemMessages(candidateMessages)
		candidateEstimate := chatRequestPromptEstimate(candidate)
		if candidateEstimate.Tokens >= estimate.Tokens && candidateEstimate.Characters >= estimate.Characters {
			continue
		}
		savedChars := estimate.Characters - candidateEstimate.Characters
		if savedChars > 0 {
			diagnostics.compressionChars[stage.name] += savedChars
			diagnostics.savedChars += savedChars
		}
		messages = candidateMessages
		request.Messages = candidate.Messages
		estimate = candidateEstimate
		scaledEstimateTokens = scaledPromptTokens(estimate.Tokens, estimateScale)
	}

	diagnostics.finalPromptTokens = scaledEstimateTokens
	if r.requestBudget.safeContextLimit > 0 && scaledEstimateTokens >= r.requestBudget.safeContextLimit {
		return fmt.Errorf("%w: final planning request exceeds safe context limit", ErrInvalidInput)
	}
	if r.requestBudget.safeContextLimit <= 0 {
		return nil
	}
	remaining := r.requestBudget.safeContextLimit - scaledEstimateTokens
	if remaining <= 0 {
		return fmt.Errorf("%w: final planning request leaves no output budget", ErrInvalidInput)
	}
	desiredOutputTokens := diagnostics.originalMaxTokens
	if desiredOutputTokens <= 0 {
		desiredOutputTokens = r.requestBudget.outputTokenLimit
	}
	outputWasClamped := desiredOutputTokens <= 0 || desiredOutputTokens > remaining
	if desiredOutputTokens <= 0 || desiredOutputTokens > remaining {
		desiredOutputTokens = remaining
	}
	if request.MaxTokens == nil || *request.MaxTokens != desiredOutputTokens {
		request.MaxTokens = intPointer(desiredOutputTokens)
	}
	diagnostics.maxTokensClamped = outputWasClamped
	if request.MaxTokens != nil {
		diagnostics.effectiveMaxTokens = *request.MaxTokens
	}
	return nil
}

func intPointer(value int) *int {
	return &value
}

func compactRepeatedTurnEvidenceState(messages []adapter.Message) ([]adapter.Message, bool) {
	out := cloneMessagesForProvider(messages)
	changed := false
	seen := map[string]struct{}{}
	for index := len(out) - 1; index >= 0; index-- {
		message := out[index]
		if !strings.EqualFold(strings.TrimSpace(message.Role), "system") {
			continue
		}
		content, ok := message.Content.(string)
		if !ok || !isTurnEvidenceStateMessage(content) {
			continue
		}
		key := strings.TrimSpace(content)
		if _, duplicate := seen[key]; duplicate {
			out = append(out[:index], out[index+1:]...)
			changed = true
			continue
		}
		seen[key] = struct{}{}
	}
	latestToolIndex, latestToolCallID := latestToolEvidence(out)
	toolNames := toolCallNamesByID(out)
	for index := range out {
		if index == latestToolIndex {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(out[index].Role)) {
		case "assistant":
			for callIndex := range out[index].ToolCalls {
				call := &out[index].ToolCalls[callIndex]
				if call.ID == latestToolCallID || !isTurnEvidenceTool(call.Function.Name) {
					continue
				}
				call.Function.Arguments = budgetCompactedToolArguments()
				changed = true
			}
		case "tool":
			if out[index].ToolCallID == latestToolCallID || !isTurnEvidenceTool(toolNames[out[index].ToolCallID]) {
				continue
			}
			out[index].Content = budgetCompactedToolResult("historical turn/evidence state omitted")
			changed = true
		}
	}
	return out, changed
}

func isTurnEvidenceStateMessage(content string) bool {
	lower := strings.ToLower(content)
	for _, marker := range []string{
		"turn_start_context",
		"current_page_context",
		"current turn structured state",
		"current assistant turn task contract:",
		"operation_plan",
		"evidence_ledger",
		"execution_ledger",
		"turn_state",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func isTurnEvidenceTool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case strings.ToLower(skills.MetaToolTurnState), strings.ToLower(skills.MetaToolUpdatePlan):
		return true
	default:
		return false
	}
}

func compactNonTargetRestoredSkills(messages []adapter.Message, preferredSkillID string) ([]adapter.Message, bool) {
	out := cloneMessagesForProvider(messages)
	changed := false
	for index := range out {
		if !strings.EqualFold(strings.TrimSpace(out[index].Role), "system") {
			continue
		}
		content, ok := out[index].Content.(string)
		if !ok || !strings.HasPrefix(strings.TrimSpace(content), "The following skill instructions were loaded earlier") || !strings.Contains(content, "Restored skill: ") {
			continue
		}
		compacted, ok := compactRestoredSkillInstructions(content, preferredSkillID)
		if !ok {
			continue
		}
		out[index].Content = compacted
		changed = true
	}
	return out, changed
}

func compactRestoredSkillInstructions(content string, preferredSkillID string) (string, bool) {
	const marker = "Restored skill: "
	first := strings.Index(content, marker)
	if first < 0 {
		return content, false
	}
	prefix := strings.TrimSpace(content[:first])
	remainder := content[first:]
	suffix := ""
	if suffixIndex := strings.Index(remainder, "\n\nSkills requiring full reload before use:"); suffixIndex >= 0 {
		suffix = strings.TrimSpace(remainder[suffixIndex:])
		remainder = remainder[:suffixIndex]
	}
	starts := []int{0}
	for searchFrom := len(marker); searchFrom < len(remainder); {
		next := strings.Index(remainder[searchFrom:], "\n\n"+marker)
		if next < 0 {
			break
		}
		start := searchFrom + next + 2
		starts = append(starts, start)
		searchFrom = start + len(marker)
	}
	sections := make([]string, 0, len(starts))
	changed := false
	for index, start := range starts {
		end := len(remainder)
		if index+1 < len(starts) {
			end = starts[index+1] - 2
		}
		section := strings.TrimSpace(remainder[start:end])
		firstLineEnd := strings.IndexByte(section, '\n')
		firstLine := section
		if firstLineEnd >= 0 {
			firstLine = section[:firstLineEnd]
		}
		skillID := strings.TrimSpace(strings.TrimPrefix(firstLine, marker))
		if strings.EqualFold(skillID, strings.TrimSpace(preferredSkillID)) {
			sections = append(sections, section)
			continue
		}
		instructionMarker := "\nInstructions:\n"
		instructionIndex := strings.Index(section, instructionMarker)
		if instructionIndex < 0 {
			sections = append(sections, section)
			continue
		}
		sections = append(sections, strings.TrimSpace(section[:instructionIndex])+"\nInstructions omitted by the final request budget; call load_skill before using this skill.")
		changed = true
	}
	if !changed {
		return content, false
	}
	parts := []string{}
	if prefix != "" {
		parts = append(parts, prefix)
	}
	parts = append(parts, sections...)
	if suffix != "" {
		parts = append(parts, suffix)
	}
	return strings.Join(parts, "\n\n"), true
}

func compactHistoricalToolPayloads(messages []adapter.Message) ([]adapter.Message, bool) {
	return compactHistoricalToolMessages(messages, func(name string) bool {
		return !isTurnEvidenceTool(name) && !strings.EqualFold(strings.TrimSpace(name), skills.MetaToolIntermediateAnswer)
	})
}

func compactHistoricalIntermediateAnswers(messages []adapter.Message) ([]adapter.Message, bool) {
	return compactHistoricalToolMessages(messages, func(name string) bool {
		return strings.EqualFold(strings.TrimSpace(name), skills.MetaToolIntermediateAnswer)
	})
}

func compactHistoricalToolMessages(messages []adapter.Message, selected func(string) bool) ([]adapter.Message, bool) {
	out := cloneMessagesForProvider(messages)
	latestToolIndex, latestToolCallID := latestToolEvidence(out)
	toolNames := toolCallNamesByID(out)
	changed := false
	for index := range out {
		switch strings.ToLower(strings.TrimSpace(out[index].Role)) {
		case "assistant":
			for callIndex := range out[index].ToolCalls {
				call := &out[index].ToolCalls[callIndex]
				if call.ID == latestToolCallID || !selected(call.Function.Name) {
					continue
				}
				call.Function.Arguments = budgetCompactedToolArguments()
				changed = true
			}
		case "tool":
			if index == latestToolIndex || out[index].ToolCallID == latestToolCallID || !selected(toolNames[out[index].ToolCallID]) {
				continue
			}
			out[index].Content = budgetCompactedToolResult("historical tool payload omitted")
			changed = true
		}
	}
	return out, changed
}

func latestToolEvidence(messages []adapter.Message) (int, string) {
	for index := len(messages) - 1; index >= 0; index-- {
		if strings.EqualFold(strings.TrimSpace(messages[index].Role), "tool") {
			return index, strings.TrimSpace(messages[index].ToolCallID)
		}
	}
	return -1, ""
}

func toolCallNamesByID(messages []adapter.Message) map[string]string {
	out := map[string]string{}
	for _, message := range messages {
		for _, call := range message.ToolCalls {
			if id := strings.TrimSpace(call.ID); id != "" {
				out[id] = strings.TrimSpace(call.Function.Name)
			}
		}
	}
	return out
}

func budgetCompactedToolArguments() string {
	encoded, _ := json.Marshal(map[string]interface{}{
		"budget_compacted": true,
		"detail":           "historical arguments omitted",
	})
	return string(encoded)
}

func budgetCompactedToolResult(detail string) string {
	encoded, _ := json.Marshal(map[string]interface{}{
		"budget_compacted": true,
		"status":           "historical_evidence_compacted",
		"detail":           detail,
	})
	return string(encoded)
}
