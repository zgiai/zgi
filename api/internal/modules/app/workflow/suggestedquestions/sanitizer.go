package suggestedquestions

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	defaultTextLimit = 700
	shortTextLimit   = 160
)

var sensitiveKeyPattern = regexp.MustCompile(`(?i)(api[_-]?key|authorization|bearer|secret|token|password|credential|cookie|private[_-]?key)`)

func cleanText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.Join(strings.Fields(value), " ")
	if limit <= 0 || utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return strings.TrimSpace(string(runes[:limit])) + "..."
}

func cleanShortText(value string) string {
	return cleanText(value, shortTextLimit)
}

func isSensitiveKey(key string) bool {
	return sensitiveKeyPattern.MatchString(key)
}

func stringValue(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return ""
	}
}

func boolValue(value interface{}) bool {
	v, ok := value.(bool)
	return ok && v
}

func mapValue(value interface{}) map[string]interface{} {
	if value == nil {
		return nil
	}
	if m, ok := value.(map[string]interface{}); ok {
		return m
	}
	return nil
}

func sliceValue(value interface{}) []interface{} {
	if value == nil {
		return nil
	}
	if items, ok := value.([]interface{}); ok {
		return items
	}
	return nil
}

func firstString(values ...interface{}) string {
	for _, value := range values {
		if text := cleanShortText(stringValue(value)); text != "" {
			return text
		}
	}
	return ""
}

func uniqueTrimmed(values []string, limit int) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		text := cleanShortText(value)
		if text == "" {
			continue
		}
		key := strings.ToLower(text)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, text)
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result
}
