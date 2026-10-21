package gateway

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
)

var publicUpstreamModelMentionPattern = regexp.MustCompile(`(?i)(\bmodel(?:[_ ](?:id|name))?\b\s*(?::|=|is)?\s*[` + "`" + `'\"]?)([A-Za-z0-9._:/-]*[0-9._:/-][A-Za-z0-9._:/-]*)`)

// sanitizePublicNativeBody preserves protocol payloads while restoring the
// requested logical model and removing internal translation accounting data.
func sanitizePublicNativeBody(body json.RawMessage, logicalModel string) json.RawMessage {
	if len(body) == 0 {
		return body
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return body
	}
	if !sanitizePublicNativeObject(payload, logicalModel, true) {
		return body
	}
	updated, err := json.Marshal(payload)
	if err != nil {
		return body
	}
	return updated
}

func sanitizePublicNativeObject(payload map[string]any, logicalModel string, rewriteEnvelope bool) bool {
	changed := false
	if _, ok := payload["billing_usage"]; ok {
		delete(payload, "billing_usage")
		changed = true
	}
	if rewriteEnvelope {
		for _, key := range []string{"model", "modelVersion"} {
			if _, ok := payload[key]; ok && strings.TrimSpace(logicalModel) != "" {
				payload[key] = logicalModel
				changed = true
			}
		}
	}
	for key, value := range payload {
		switch child := value.(type) {
		case map[string]any:
			if sanitizePublicNativeObject(child, logicalModel, key == "message" || key == "response") {
				changed = true
			}
		case []any:
			if sanitizePublicNativeArray(child, logicalModel) {
				changed = true
			}
		}
	}
	return changed
}

func sanitizePublicNativeArray(values []any, logicalModel string) bool {
	changed := false
	for _, value := range values {
		switch child := value.(type) {
		case map[string]any:
			if sanitizePublicNativeObject(child, logicalModel, false) {
				changed = true
			}
		case []any:
			if sanitizePublicNativeArray(child, logicalModel) {
				changed = true
			}
		}
	}
	return changed
}

// SanitizePublicNativeErrorBody removes provider-routing details from a native
// error while preserving the provider's public protocol error structure.
func SanitizePublicNativeErrorBody(body json.RawMessage, logicalModel string) json.RawMessage {
	if len(body) == 0 || !json.Valid(body) {
		return nil
	}
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}
	replacements := collectPublicErrorReplacements(payload, logicalModel)
	sort.SliceStable(replacements, func(i, j int) bool { return len(replacements[i][0]) > len(replacements[j][0]) })
	payload = sanitizePublicErrorValue(payload, logicalModel, replacements)
	updated, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return updated
}

func collectPublicErrorReplacements(value any, logicalModel string) [][2]string {
	seen := make(map[string]struct{})
	result := make([][2]string, 0, 4)
	var visit func(any)
	visit = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			for key, child := range typed {
				if text, ok := child.(string); ok {
					replacement := ""
					switch publicErrorFieldKind(key) {
					case "model":
						replacement = logicalModel
					case "provider":
						replacement = "zgi"
					case "channel":
						replacement = "upstream"
					}
					if strings.TrimSpace(text) != "" && replacement != "" && text != replacement {
						if _, exists := seen[text]; !exists {
							seen[text] = struct{}{}
							result = append(result, [2]string{text, replacement})
						}
					}
				}
				visit(child)
			}
		case []any:
			for _, child := range typed {
				visit(child)
			}
		}
	}
	visit(value)
	return result
}

func sanitizePublicErrorValue(value any, logicalModel string, replacements [][2]string) any {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			switch publicErrorFieldKind(key) {
			case "model":
				if strings.TrimSpace(logicalModel) == "" || strings.Contains(strings.ToLower(strings.ReplaceAll(key, "-", "_")), "upstream") {
					delete(typed, key)
				} else {
					typed[key] = logicalModel
				}
				continue
			case "provider", "channel":
				delete(typed, key)
				continue
			}
			typed[key] = sanitizePublicErrorValue(child, logicalModel, replacements)
		}
		return typed
	case []any:
		for index, child := range typed {
			typed[index] = sanitizePublicErrorValue(child, logicalModel, replacements)
		}
		return typed
	case string:
		for _, replacement := range replacements {
			typed = strings.ReplaceAll(typed, replacement[0], replacement[1])
		}
		if strings.TrimSpace(logicalModel) != "" {
			typed = publicUpstreamModelMentionPattern.ReplaceAllString(typed, "${1}"+strings.ReplaceAll(logicalModel, "$", "$$"))
		}
		return typed
	default:
		return value
	}
}

func publicErrorFieldKind(key string) string {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "-", "_"))
	switch normalized {
	case "model", "model_id", "model_name", "modelversion", "model_version", "upstream_model", "actual_upstream_model":
		return "model"
	case "provider", "provider_id", "provider_name", "upstream_provider", "actual_provider":
		return "provider"
	case "channel", "channel_id", "channel_name", "actual_channel_id", "actual_channel_name", "actual_protocol":
		return "channel"
	default:
		return ""
	}
}
