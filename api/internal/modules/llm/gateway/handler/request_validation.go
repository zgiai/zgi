package handler

import (
	"encoding/json"
	"fmt"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func validatePublicChatRequest(req *adapter.ChatRequest) error {
	if req == nil || req.Model == "" {
		return fmt.Errorf("model is required")
	}
	if len(req.Messages) == 0 {
		return fmt.Errorf("messages must contain at least one item")
	}
	if req.MaxTokens != nil && *req.MaxTokens <= 0 {
		return fmt.Errorf("max_tokens must be a positive integer")
	}
	if req.N != nil && *req.N <= 0 {
		return fmt.Errorf("n must be a positive integer")
	}
	if req.Temperature != nil && (*req.Temperature < 0 || *req.Temperature > 2) {
		return fmt.Errorf("temperature must be between 0 and 2")
	}
	if req.TopP != nil && (*req.TopP < 0 || *req.TopP > 1) {
		return fmt.Errorf("top_p must be between 0 and 1")
	}
	if req.PresencePenalty != nil && (*req.PresencePenalty < -2 || *req.PresencePenalty > 2) {
		return fmt.Errorf("presence_penalty must be between -2 and 2")
	}
	if req.FrequencyPenalty != nil && (*req.FrequencyPenalty < -2 || *req.FrequencyPenalty > 2) {
		return fmt.Errorf("frequency_penalty must be between -2 and 2")
	}
	return nil
}

func validatePublicNativeBody(body json.RawMessage, protocol string) error {
	var payload map[string]json.RawMessage
	if len(body) == 0 || json.Unmarshal(body, &payload) != nil {
		return fmt.Errorf("invalid JSON request body")
	}
	for _, key := range []string{"max_tokens", "max_output_tokens"} {
		if raw, ok := payload[key]; ok {
			if err := positiveInteger(raw, key); err != nil {
				return err
			}
		}
	}
	for _, field := range []struct {
		key      string
		min, max float64
	}{{"temperature", 0, 2}, {"top_p", 0, 1}} {
		if raw, ok := payload[field.key]; ok {
			if err := boundedNumber(raw, field.key, field.min, field.max); err != nil {
				return err
			}
		}
	}
	switch protocol {
	case "responses":
		if raw, ok := payload["input"]; !ok || emptyJSONValue(raw) {
			return fmt.Errorf("input is required")
		}
	case "messages":
		if raw, ok := payload["messages"]; !ok || emptyJSONValue(raw) {
			return fmt.Errorf("messages must contain at least one item")
		}
	case "gemini":
		if raw, ok := payload["generationConfig"]; ok {
			var config map[string]json.RawMessage
			if json.Unmarshal(raw, &config) != nil {
				return fmt.Errorf("generationConfig must be an object")
			}
			if value, ok := config["maxOutputTokens"]; ok {
				if err := positiveInteger(value, "maxOutputTokens"); err != nil {
					return err
				}
			}
			for _, field := range []struct {
				key      string
				min, max float64
			}{{"temperature", 0, 2}, {"topP", 0, 1}} {
				if value, ok := config[field.key]; ok {
					if err := boundedNumber(value, field.key, field.min, field.max); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func positiveInteger(raw json.RawMessage, field string) error {
	var value int
	if json.Unmarshal(raw, &value) != nil || value <= 0 {
		return fmt.Errorf("%s must be a positive integer", field)
	}
	return nil
}

func boundedNumber(raw json.RawMessage, field string, min, max float64) error {
	var value float64
	if json.Unmarshal(raw, &value) != nil || value < min || value > max {
		return fmt.Errorf("%s must be between %g and %g", field, min, max)
	}
	return nil
}

func emptyJSONValue(raw json.RawMessage) bool {
	value := string(raw)
	return value == "null" || value == `""` || value == "[]"
}
