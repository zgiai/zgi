package gateway

import (
	"encoding/json"
	"fmt"
	"strings"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

type nativeUsageBodyFormat string

const (
	nativeUsageBodyFormatResponses nativeUsageBodyFormat = "responses"
	nativeUsageBodyFormatAnthropic nativeUsageBodyFormat = "anthropic"
)

func (s *llmGatewayServiceImpl) estimateMissingChatUsage(req *adapter.ChatRequest, resp *adapter.ChatResponse) *adapter.Usage {
	if resp == nil {
		return nil
	}
	usage, _ := s.completeChatUsageFromText(req, resp.Usage, chatResponseText(resp), 0)
	if hasBillableTokenUsage(usage) {
		resp.Usage = usage
		return usage
	}
	return nil
}

func (s *llmGatewayServiceImpl) estimateUsageFromText(req *adapter.ChatRequest, completionText string, promptTokens int) *adapter.Usage {
	usage, _ := s.completeChatUsageFromText(req, nil, completionText, promptTokens)
	return usage
}

func (s *llmGatewayServiceImpl) completeCreateResponseUsageFromText(req *adapter.CreateResponseRequest, existing *adapter.Usage, completionText string, promptFallback int) (*adapter.Usage, bool) {
	model := ""
	if req != nil {
		model = req.Model
	}
	if promptFallback <= 0 && req != nil {
		promptFallback = s.tokenEstimatorForFallback().EstimateCreateResponsePromptTokens(req)
	}
	return s.completeNativeUsageFromText(model, existing, completionText, promptFallback)
}

func (s *llmGatewayServiceImpl) completeChatUsageFromText(req *adapter.ChatRequest, existing *adapter.Usage, completionText string, promptFallback int) (*adapter.Usage, bool) {
	estimator := s.tokenEstimatorForFallback()
	model := ""
	if req != nil {
		model = req.Model
	}
	textPresent := strings.TrimSpace(completionText) != ""
	usage := adapter.Usage{}
	hadAny := false
	estimated := false
	if existing != nil {
		usage = *existing
		hadAny = usage.PromptTokens > 0 || usage.CompletionTokens > 0 || usage.TotalTokens > 0
	}
	knownTotal := usage.TotalTokens
	if !hadAny && !textPresent {
		return nil, false
	}

	estimatedPrompt := promptFallback
	if estimatedPrompt <= 0 && req != nil {
		estimatedPrompt = estimator.EstimateChatPromptTokens(req)
	}
	estimatedCompletion := 0
	if textPresent {
		estimatedCompletion = estimator.EstimateTextTokensForModel(model, completionText)
	}

	if usage.PromptTokens <= 0 && usage.CompletionTokens <= 0 && usage.TotalTokens > 0 {
		switch {
		case estimatedPrompt > 0 && usage.TotalTokens > estimatedPrompt:
			usage.PromptTokens = estimatedPrompt
			usage.CompletionTokens = usage.TotalTokens - estimatedPrompt
			estimated = true
		case textPresent && estimatedCompletion > 0 && usage.TotalTokens > estimatedCompletion:
			usage.CompletionTokens = estimatedCompletion
			usage.PromptTokens = usage.TotalTokens - estimatedCompletion
			estimated = true
		default:
			usage.PromptTokens = usage.TotalTokens
		}
	}
	if usage.PromptTokens <= 0 && estimatedPrompt > 0 {
		if usage.TotalTokens > usage.CompletionTokens {
			usage.PromptTokens = usage.TotalTokens - usage.CompletionTokens
		} else if usage.TotalTokens <= 0 {
			usage.PromptTokens = estimatedPrompt
			estimated = true
		}
	}
	if textPresent && usage.CompletionTokens <= 0 && estimatedCompletion > 0 {
		if usage.TotalTokens > usage.PromptTokens {
			usage.CompletionTokens = usage.TotalTokens - usage.PromptTokens
		} else if usage.TotalTokens <= 0 {
			usage.CompletionTokens = estimatedCompletion
			estimated = true
		}
	}
	normalizeCompletedUsageTotal(&usage, knownTotal)
	if !hasBillableTokenUsage(&usage) {
		return nil, false
	}
	return &usage, estimated || !hadAny
}

func markEstimatedUsageSource(bc *BillingContext, usage *adapter.Usage) {
	if bc == nil || !hasBillableTokenUsage(usage) {
		return
	}
	bc.UsageSource = usageSourceEstimated
}

func (s *llmGatewayServiceImpl) ensureNativeResponseUsageForSelection(selection *ProviderSelection, bc *BillingContext, resp *adapter.RawResponse, model string, format nativeUsageBodyFormat) (bool, error) {
	if resp == nil {
		return false, nil
	}
	if selection != nil && selection.UseSystemProvider {
		if hasBillableTokenUsage(resp.Usage) {
			normalizeUsage(resp.Usage)
		}
		return false, nil
	}
	if resp.Settlement != nil {
		return false, nil
	}
	usage, estimated := s.completeNativeUsageFromText(model, resp.Usage, nativeResponseText(resp.Body), nativePromptTokens(bc))
	if !hasBillableTokenUsage(usage) {
		return false, nil
	}
	if !nativeRawBodyHasUsage(resp.Body) {
		body, err := setNativeUsageInRawBody(resp.Body, usage, format)
		if err != nil {
			return false, err
		}
		resp.Body = body
	}
	resp.Usage = usage
	if estimated {
		markEstimatedUsageSource(bc, usage)
	}
	return estimated, nil
}

func (s *llmGatewayServiceImpl) estimateNativeUsageFromText(model, completionText string, promptTokens int) *adapter.Usage {
	usage, _ := s.completeNativeUsageFromText(model, nil, completionText, promptTokens)
	return usage
}

func (s *llmGatewayServiceImpl) completeNativeUsageFromText(model string, existing *adapter.Usage, completionText string, promptTokens int) (*adapter.Usage, bool) {
	textPresent := strings.TrimSpace(completionText) != ""
	usage := adapter.Usage{}
	hadAny := false
	estimated := false
	if existing != nil {
		usage = *existing
		hadAny = usage.PromptTokens > 0 || usage.CompletionTokens > 0 || usage.TotalTokens > 0
	}
	knownTotal := usage.TotalTokens
	if !hadAny && !textPresent {
		return nil, false
	}
	completionTokens := s.tokenEstimatorForFallback().EstimateTextTokensForModel(model, completionText)
	if usage.PromptTokens <= 0 && usage.CompletionTokens <= 0 && usage.TotalTokens > 0 {
		switch {
		case promptTokens > 0 && usage.TotalTokens > promptTokens:
			usage.PromptTokens = promptTokens
			usage.CompletionTokens = usage.TotalTokens - promptTokens
			estimated = true
		case textPresent && completionTokens > 0 && usage.TotalTokens > completionTokens:
			usage.CompletionTokens = completionTokens
			usage.PromptTokens = usage.TotalTokens - completionTokens
			estimated = true
		default:
			usage.PromptTokens = usage.TotalTokens
		}
	}
	if usage.PromptTokens <= 0 && promptTokens > 0 {
		if usage.TotalTokens > usage.CompletionTokens {
			usage.PromptTokens = usage.TotalTokens - usage.CompletionTokens
		} else if usage.TotalTokens <= 0 {
			usage.PromptTokens = promptTokens
			estimated = true
		}
	}
	if textPresent && usage.CompletionTokens <= 0 && completionTokens > 0 {
		if usage.TotalTokens > usage.PromptTokens {
			usage.CompletionTokens = usage.TotalTokens - usage.PromptTokens
		} else if usage.TotalTokens <= 0 {
			usage.CompletionTokens = completionTokens
			estimated = true
		}
	}
	normalizeCompletedUsageTotal(&usage, knownTotal)
	if !hasBillableTokenUsage(&usage) {
		return nil, false
	}
	return &usage, estimated || !hadAny
}

func splitTotalOnlyNativeUsage(usage *adapter.Usage, promptTokens int) *adapter.Usage {
	if usage == nil || usage.PromptTokens > 0 || usage.CompletionTokens > 0 || usage.TotalTokens <= 0 {
		return nil
	}
	if promptTokens > 0 && usage.TotalTokens > promptTokens {
		return &adapter.Usage{
			PromptTokens:     promptTokens,
			CompletionTokens: usage.TotalTokens - promptTokens,
			TotalTokens:      usage.TotalTokens,
		}
	}
	return &adapter.Usage{
		PromptTokens: usage.TotalTokens,
		TotalTokens:  usage.TotalTokens,
	}
}

func nativeUsageStreamEvent(model string, usage *adapter.Usage, format nativeUsageBodyFormat) adapter.RawStreamEvent {
	if !hasBillableTokenUsage(usage) {
		return adapter.RawStreamEvent{}
	}
	normalizeUsage(usage)
	switch format {
	case nativeUsageBodyFormatAnthropic:
		return rawJSONStreamEvent("message_delta", map[string]any{
			"type":  "message_delta",
			"delta": map[string]any{},
			"usage": map[string]any{
				"input_tokens":  usage.PromptTokens,
				"output_tokens": usage.CompletionTokens,
			},
		})
	default:
		return rawJSONStreamEvent("response.completed", map[string]any{
			"type": "response.completed",
			"response": map[string]any{
				"model": model,
				"usage": map[string]any{
					"input_tokens":  usage.PromptTokens,
					"output_tokens": usage.CompletionTokens,
					"total_tokens":  usage.TotalTokens,
				},
			},
		})
	}
}

func rawJSONStreamEvent(eventName string, payload map[string]any) adapter.RawStreamEvent {
	data, _ := json.Marshal(payload)
	return adapter.RawStreamEvent{
		Event: eventName,
		Data:  data,
	}
}

func isNativeTerminalStreamEvent(format nativeUsageBodyFormat, event adapter.RawStreamEvent) bool {
	eventName := strings.TrimSpace(event.Event)
	switch format {
	case nativeUsageBodyFormatAnthropic:
		return eventName == "message_stop"
	default:
		return eventName == "response.completed"
	}
}

func injectUsageIntoNativeTerminalEvent(event adapter.RawStreamEvent, model string, usage *adapter.Usage, format nativeUsageBodyFormat) (adapter.RawStreamEvent, bool) {
	if !hasBillableTokenUsage(usage) || len(event.Data) == 0 {
		return event, false
	}
	if format == nativeUsageBodyFormatAnthropic {
		return event, false
	}
	updated, err := setResponsesStreamUsage(event.Data, model, usage)
	if err != nil {
		return event, false
	}
	event.Data = updated
	return event, true
}

func setResponsesStreamUsage(data json.RawMessage, model string, usage *adapter.Usage) (json.RawMessage, error) {
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	response, _ := payload["response"].(map[string]any)
	if response == nil {
		response = map[string]any{}
		payload["response"] = response
	}
	if _, ok := response["usage"]; ok {
		return data, nil
	}
	if strings.TrimSpace(model) != "" {
		if _, ok := response["model"]; !ok {
			response["model"] = model
		}
	}
	response["usage"] = map[string]any{
		"input_tokens":  usage.PromptTokens,
		"output_tokens": usage.CompletionTokens,
		"total_tokens":  usage.TotalTokens,
	}
	return json.Marshal(payload)
}

func nativePromptTokens(bc *BillingContext) int {
	if bc == nil {
		return 0
	}
	return bc.PromptTokens
}

func normalizeUsage(usage *adapter.Usage) {
	if usage == nil {
		return
	}
	if usage.TotalTokens <= 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
}

func normalizeCompletedUsageTotal(usage *adapter.Usage, knownTotal int) {
	if usage == nil {
		return
	}
	if knownTotal > 0 {
		if usage.PromptTokens > knownTotal {
			usage.PromptTokens = knownTotal
		}
		remaining := knownTotal - usage.PromptTokens
		if usage.CompletionTokens > remaining {
			usage.CompletionTokens = remaining
		}
		usage.TotalTokens = knownTotal
		return
	}
	normalizeUsage(usage)
	if sum := usage.PromptTokens + usage.CompletionTokens; sum > usage.TotalTokens {
		usage.TotalTokens = sum
	}
}

func nativeRawBodyHasUsage(body json.RawMessage) bool {
	if len(body) == 0 {
		return false
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}
	_, ok := payload["usage"]
	return ok
}

func ensureEmbeddingUsage(resp *adapter.EmbeddingsResponse, promptTokens int) (actualTokens int, estimated bool) {
	if resp == nil {
		return 0, false
	}
	usage := resp.Usage
	if usage.PromptTokens <= 0 && usage.TotalTokens > 0 {
		usage.PromptTokens = usage.TotalTokens
	}
	if usage.TotalTokens <= 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	if usage.PromptTokens <= 0 && usage.CompletionTokens <= 0 && usage.TotalTokens <= 0 {
		if promptTokens <= 0 {
			return 0, false
		}
		usage = adapter.Usage{
			PromptTokens: promptTokens,
			TotalTokens:  promptTokens,
		}
		estimated = true
	}
	resp.Usage = usage
	return usage.TotalTokens, estimated
}

func ensureEmbeddingUsageForSelection(selection *ProviderSelection, resp *adapter.EmbeddingsResponse, promptTokens int) (actualTokens int, estimated bool) {
	if selection != nil && selection.UseSystemProvider {
		return normalizeEmbeddingUsage(resp), false
	}
	return ensureEmbeddingUsage(resp, promptTokens)
}

func normalizeEmbeddingUsage(resp *adapter.EmbeddingsResponse) int {
	if resp == nil {
		return 0
	}
	usage := resp.Usage
	if usage.PromptTokens <= 0 && usage.TotalTokens > 0 {
		usage.PromptTokens = usage.TotalTokens
	}
	if usage.TotalTokens <= 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	resp.Usage = usage
	return usage.TotalTokens
}

func ensureRerankUsage(resp *adapter.RerankResponse, promptTokens int) (actualTokens int, estimated bool) {
	if resp == nil {
		return 0, false
	}
	if resp.Usage == nil {
		if promptTokens <= 0 {
			return 0, false
		}
		resp.Usage = &adapter.Usage{
			PromptTokens: promptTokens,
			TotalTokens:  promptTokens,
		}
		return promptTokens, true
	}
	usage := *resp.Usage
	if usage.PromptTokens <= 0 && usage.TotalTokens > 0 {
		usage.PromptTokens = usage.TotalTokens
	}
	if usage.TotalTokens <= 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	if usage.PromptTokens <= 0 && usage.CompletionTokens <= 0 && usage.TotalTokens <= 0 {
		if promptTokens <= 0 {
			return 0, false
		}
		usage = adapter.Usage{
			PromptTokens: promptTokens,
			TotalTokens:  promptTokens,
		}
		estimated = true
	}
	resp.Usage = &usage
	return usage.TotalTokens, estimated
}

func ensureRerankUsageForSelection(selection *ProviderSelection, resp *adapter.RerankResponse, promptTokens int) (actualTokens int, estimated bool) {
	if selection != nil && selection.UseSystemProvider {
		return normalizeRerankUsage(resp), false
	}
	return ensureRerankUsage(resp, promptTokens)
}

func normalizeRerankUsage(resp *adapter.RerankResponse) int {
	if resp == nil || resp.Usage == nil {
		return 0
	}
	usage := *resp.Usage
	if usage.PromptTokens <= 0 && usage.TotalTokens > 0 {
		usage.PromptTokens = usage.TotalTokens
	}
	if usage.TotalTokens <= 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	resp.Usage = &usage
	return usage.TotalTokens
}

func (s *llmGatewayServiceImpl) tokenEstimatorForFallback() *TokenEstimator {
	if s != nil && s.tokenEstimator != nil {
		return s.tokenEstimator
	}
	return NewTokenEstimator()
}

func chatResponseText(resp *adapter.ChatResponse) string {
	var text strings.Builder
	for _, choice := range resp.Choices {
		appendText(&text, messageCompletionText(choice.Message))
	}
	return text.String()
}

func messageCompletionText(msg adapter.Message) string {
	var text strings.Builder
	appendText(&text, messageContentText(msg.Content))
	appendText(&text, msg.ReasoningContent)
	if msg.FunctionCall != nil {
		appendText(&text, msg.FunctionCall.Name)
		appendText(&text, msg.FunctionCall.Arguments)
	}
	for _, toolCall := range msg.ToolCalls {
		appendText(&text, toolCall.ID)
		appendText(&text, toolCall.Type)
		appendText(&text, toolCall.Function.Name)
		appendText(&text, toolCall.Function.Arguments)
	}
	return text.String()
}

func createResponseText(resp *adapter.CreateResponseResponse) string {
	if resp == nil {
		return ""
	}
	var text strings.Builder
	for _, output := range resp.Output {
		for _, content := range output.Content {
			appendText(&text, content.Text)
		}
		if output.Message != nil {
			appendText(&text, messageCompletionText(*output.Message))
		}
		if output.RawContent != nil {
			appendText(&text, fmt.Sprint(output.RawContent))
		}
	}
	for _, choice := range resp.Choices {
		appendText(&text, messageCompletionText(choice.Message))
	}
	return text.String()
}

func messageContentText(content any) string {
	switch v := content.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

func appendText(dst *strings.Builder, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	if dst.Len() > 0 {
		dst.WriteByte('\n')
	}
	dst.WriteString(value)
}

func nativeResponseText(body json.RawMessage) string {
	if len(body) == 0 {
		return ""
	}
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return string(body)
	}
	var text strings.Builder
	collectNativeText(&text, value)
	return text.String()
}

func setNativeUsageInRawBody(body json.RawMessage, usage *adapter.Usage, format nativeUsageBodyFormat) (json.RawMessage, error) {
	if len(body) == 0 || !hasBillableTokenUsage(usage) {
		return body, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse native response body for usage fallback: %w", err)
	}
	normalizeUsage(usage)
	switch format {
	case nativeUsageBodyFormatAnthropic:
		payload["usage"] = map[string]any{
			"input_tokens":  usage.PromptTokens,
			"output_tokens": usage.CompletionTokens,
		}
	default:
		payload["usage"] = map[string]any{
			"input_tokens":  usage.PromptTokens,
			"output_tokens": usage.CompletionTokens,
			"total_tokens":  usage.TotalTokens,
		}
	}
	updated, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal native response body for usage fallback: %w", err)
	}
	return updated, nil
}

func collectNativeText(dst *strings.Builder, value any) {
	switch v := value.(type) {
	case []any:
		for _, item := range v {
			collectNativeText(dst, item)
		}
	case map[string]any:
		for _, key := range []string{"text", "delta", "arguments", "name", "content", "output_text"} {
			if text, ok := v[key].(string); ok {
				appendText(dst, text)
			}
		}
		for _, item := range v {
			collectNativeText(dst, item)
		}
	}
}
