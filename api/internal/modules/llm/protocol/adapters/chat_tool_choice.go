package adapter

import (
	"encoding/json"
	"strings"
)

// NormalizeChatToolChoice rewrites client tool_choice into the OpenAI Chat
// Completions wire form. Named-function aliases (bare name, Anthropic tool,
// Responses function object) become {"type":"function","function":{"name":...}}.
func NormalizeChatToolChoice(value interface{}) interface{} {
	parsed := parseChatToolChoice(value)
	switch parsed.mode {
	case "none", "auto", "required":
		return parsed.mode
	case "function":
		if parsed.name == "" {
			return "required"
		}
		return map[string]interface{}{
			"type": "function",
			"function": map[string]string{
				"name": parsed.name,
			},
		}
	default:
		return value
	}
}

// CompatibleChatToolChoice converts a named-function tool_choice into
// "required". OpenAI-compatible proxies such as Agicto accept the string enum
// but return HTTP 500 for the nested function object.
func CompatibleChatToolChoice(value interface{}) interface{} {
	parsed := parseChatToolChoice(value)
	if parsed.mode == "function" {
		return "required"
	}
	if parsed.mode == "none" || parsed.mode == "auto" || parsed.mode == "required" {
		return parsed.mode
	}
	return value
}

type parsedChatToolChoice struct {
	mode string
	name string
}

func parseChatToolChoice(value interface{}) parsedChatToolChoice {
	if value == nil {
		return parsedChatToolChoice{}
	}
	switch typed := value.(type) {
	case string:
		return parseChatToolChoiceString(typed)
	case json.RawMessage:
		if len(typed) == 0 {
			return parsedChatToolChoice{}
		}
		var decoded interface{}
		if err := json.Unmarshal(typed, &decoded); err != nil {
			return parsedChatToolChoice{}
		}
		return parseChatToolChoice(decoded)
	case map[string]interface{}:
		return parseChatToolChoiceMap(typed)
	case map[string]string:
		converted := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			converted[key] = item
		}
		return parseChatToolChoiceMap(converted)
	default:
		raw, err := json.Marshal(value)
		if err != nil {
			return parsedChatToolChoice{}
		}
		var decoded interface{}
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return parsedChatToolChoice{}
		}
		if decoded == nil {
			return parsedChatToolChoice{}
		}
		return parseChatToolChoice(decoded)
	}
}

func parseChatToolChoiceString(value string) parsedChatToolChoice {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return parsedChatToolChoice{}
	}
	switch strings.ToLower(trimmed) {
	case "none", "auto", "required":
		return parsedChatToolChoice{mode: strings.ToLower(trimmed)}
	case "any":
		return parsedChatToolChoice{mode: "required"}
	default:
		return parsedChatToolChoice{mode: "function", name: trimmed}
	}
}

func parseChatToolChoiceMap(value map[string]interface{}) parsedChatToolChoice {
	if value == nil {
		return parsedChatToolChoice{}
	}
	choiceType := strings.ToLower(strings.TrimSpace(asToolChoiceString(value["type"])))
	switch choiceType {
	case "none", "auto":
		return parsedChatToolChoice{mode: choiceType}
	case "required", "any":
		return parsedChatToolChoice{mode: "required"}
	case "function":
		if name := nestedToolChoiceName(value, "function", "name"); name != "" {
			return parsedChatToolChoice{mode: "function", name: name}
		}
		if name := asToolChoiceString(value["name"]); name != "" {
			return parsedChatToolChoice{mode: "function", name: name}
		}
		return parsedChatToolChoice{mode: "required"}
	case "tool":
		if name := asToolChoiceString(value["name"]); name != "" {
			return parsedChatToolChoice{mode: "function", name: name}
		}
		return parsedChatToolChoice{mode: "required"}
	}
	if name := nestedToolChoiceName(value, "function", "name"); name != "" {
		return parsedChatToolChoice{mode: "function", name: name}
	}
	if name := asToolChoiceString(value["name"]); name != "" {
		return parsedChatToolChoice{mode: "function", name: name}
	}
	return parsedChatToolChoice{}
}

func nestedToolChoiceName(value map[string]interface{}, keys ...string) string {
	current := any(value)
	for _, key := range keys {
		switch typed := current.(type) {
		case map[string]interface{}:
			current = typed[key]
		case map[string]string:
			current = typed[key]
		default:
			return ""
		}
	}
	return asToolChoiceString(current)
}

func asToolChoiceString(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return ""
	}
}
