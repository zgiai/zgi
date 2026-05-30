package gateway

import (
	"encoding/json"
	"fmt"
	"strings"

	appconfig "github.com/zgiai/zgi/api/config"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const (
	policyPromptSeparator       = "\n\n"
	policyPromptImagePrefix     = "请严格遵守以下内容安全要求："
	policyPromptImageUserPrefix = "用户图片生成需求："
)

type llmPolicyPromptInjector struct {
	enabled bool
	prompt  string
}

func newLLMPolicyPromptInjector(cfg appconfig.LLMPolicyPromptConfig) llmPolicyPromptInjector {
	return llmPolicyPromptInjector{
		enabled: cfg.Enabled && strings.TrimSpace(cfg.Prompt) != "",
		prompt:  strings.TrimSpace(cfg.Prompt),
	}
}

func (i llmPolicyPromptInjector) enabledPrompt() (string, bool) {
	if !i.enabled || i.prompt == "" {
		return "", false
	}
	return i.prompt, true
}

func (i llmPolicyPromptInjector) injectChatRequest(req *adapter.ChatRequest) *adapter.ChatRequest {
	prompt, ok := i.enabledPrompt()
	if !ok || req == nil {
		return req
	}

	cloned := *req
	cloned.Messages = make([]adapter.Message, 0, len(req.Messages)+1)
	cloned.Messages = append(cloned.Messages, adapter.Message{
		Role:    "system",
		Content: prompt,
	})
	cloned.Messages = append(cloned.Messages, req.Messages...)
	return &cloned
}

func (i llmPolicyPromptInjector) injectCreateResponseRequest(req *adapter.CreateResponseRequest) *adapter.CreateResponseRequest {
	prompt, ok := i.enabledPrompt()
	if !ok || req == nil {
		return req
	}

	cloned := *req
	cloned.Instructions = joinPolicyPrompt(prompt, req.Instructions)
	return &cloned
}

func (i llmPolicyPromptInjector) injectRawResponseRequest(req *adapter.RawResponseRequest) (*adapter.RawResponseRequest, error) {
	if _, ok := i.enabledPrompt(); !ok || req == nil {
		return req, nil
	}
	body, err := i.injectOpenAIResponseBody(req.Body)
	if err != nil {
		return nil, err
	}
	cloned := *req
	cloned.Body = body
	return &cloned, nil
}

func (i llmPolicyPromptInjector) injectAnthropicMessageRequest(req *adapter.AnthropicMessageRequest) (*adapter.AnthropicMessageRequest, error) {
	if _, ok := i.enabledPrompt(); !ok || req == nil {
		return req, nil
	}
	body, err := i.injectAnthropicMessageBody(req.Body)
	if err != nil {
		return nil, err
	}
	cloned := *req
	cloned.Body = body
	return &cloned, nil
}

func (i llmPolicyPromptInjector) injectOpenAIResponseBody(body json.RawMessage) (json.RawMessage, error) {
	prompt, ok := i.enabledPrompt()
	if !ok {
		return body, nil
	}

	payload := map[string]json.RawMessage{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse OpenAI Responses request body for policy prompt injection: %w", err)
	}

	existing, err := optionalJSONString(payload["instructions"], "instructions")
	if err != nil {
		return nil, err
	}
	payload["instructions"] = mustMarshalJSONString(joinPolicyPrompt(prompt, existing))
	return json.Marshal(payload)
}

func (i llmPolicyPromptInjector) injectAnthropicMessageBody(body json.RawMessage) (json.RawMessage, error) {
	prompt, ok := i.enabledPrompt()
	if !ok {
		return body, nil
	}

	payload := map[string]json.RawMessage{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse Anthropic Messages request body for policy prompt injection: %w", err)
	}

	systemRaw, exists := payload["system"]
	if !exists || isJSONNull(systemRaw) {
		payload["system"] = mustMarshalJSONString(prompt)
		return json.Marshal(payload)
	}

	var systemText string
	if err := json.Unmarshal(systemRaw, &systemText); err == nil {
		payload["system"] = mustMarshalJSONString(joinPolicyPrompt(prompt, systemText))
		return json.Marshal(payload)
	}

	var systemBlocks []json.RawMessage
	if err := json.Unmarshal(systemRaw, &systemBlocks); err == nil {
		policyBlock := mustMarshalJSON(map[string]string{
			"type": "text",
			"text": prompt,
		})
		nextBlocks := make([]json.RawMessage, 0, len(systemBlocks)+1)
		nextBlocks = append(nextBlocks, policyBlock)
		nextBlocks = append(nextBlocks, systemBlocks...)
		payload["system"] = mustMarshalJSON(nextBlocks)
		return json.Marshal(payload)
	}

	return nil, fmt.Errorf("anthropic system field must be a string or content block array for policy prompt injection")
}

func (i llmPolicyPromptInjector) injectImageRequest(req *adapter.ImageRequest) *adapter.ImageRequest {
	prompt, ok := i.enabledPrompt()
	if !ok || req == nil {
		return req
	}

	cloned := *req
	cloned.Prompt = strings.Join([]string{
		policyPromptImagePrefix,
		prompt,
		"",
		policyPromptImageUserPrefix,
		req.Prompt,
	}, "\n")
	return &cloned
}

func joinPolicyPrompt(policyPrompt, existing string) string {
	existing = strings.TrimSpace(existing)
	if existing == "" {
		return policyPrompt
	}
	return policyPrompt + policyPromptSeparator + existing
}

func optionalJSONString(raw json.RawMessage, field string) (string, error) {
	if len(raw) == 0 || isJSONNull(raw) {
		return "", nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("%s field must be a string for policy prompt injection", field)
	}
	return value, nil
}

func isJSONNull(raw json.RawMessage) bool {
	return strings.EqualFold(strings.TrimSpace(string(raw)), "null")
}

func mustMarshalJSONString(value string) json.RawMessage {
	return mustMarshalJSON(value)
}

func mustMarshalJSON(value interface{}) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}
