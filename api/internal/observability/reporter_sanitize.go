package observability

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	reporterRedactedValue  = "[REDACTED]"
	reporterMaxStringBytes = 8192
	reporterMaxDepth       = 6
	reporterMaxItems       = 64
)

var reporterSensitiveKeys = []string{
	"password",
	"passwd",
	"secret",
	"api_key",
	"apikey",
	"authorization",
	"cookie",
	"credential",
	"private_key",
	"access_token",
	"refresh_token",
	"ip_address",
	"ipaddress",
	"client_ip",
	"x_forwarded_for",
	"x_real_ip",
	"remote_addr",
	"node_inputs",
	"prompt",
	"request_body",
	"response_body",
	"response_text",
	"raw_sql",
}

var reporterSensitiveValuePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(?:bearer|basic)\s+[a-z0-9._~+/=-]+`),
	regexp.MustCompile(`\bsk-[a-zA-Z0-9_-]{8,}\b`),
	regexp.MustCompile(`(?i)\b(?:api[_-]?key|access_token|refresh_token|password|secret)=([^\s&]+)`),
}

var reporterAbsoluteURLPattern = regexp.MustCompile(`https?://[^\s]+`)

func sanitizeReporterTags(tags map[string]string) map[string]string {
	if len(tags) == 0 {
		return nil
	}
	result := make(map[string]string, len(tags))
	for key, value := range tags {
		key = sanitizeReporterString(key)
		if key == "" {
			continue
		}
		if isReporterSensitiveKey(key) {
			result[key] = reporterRedactedValue
			continue
		}
		if isReporterURLKey(key) {
			result[key] = sanitizeReporterURL(value)
		} else {
			result[key] = sanitizeReporterString(value)
		}
	}
	return result
}

// SanitizeReporterAttributes removes known credential/content fields and
// bounds diagnostic payloads before they reach any platform adapter.
func SanitizeReporterAttributes(attributes map[string]any) map[string]any {
	if len(attributes) == 0 {
		return nil
	}
	return sanitizeReporterMap(attributes, 0)
}

func sanitizeReporterMap(values map[string]any, depth int) map[string]any {
	if depth >= reporterMaxDepth {
		return map[string]any{"truncated": true}
	}
	result := make(map[string]any, len(values))
	itemCount := 0
	for key, value := range values {
		if itemCount >= reporterMaxItems {
			result["zgi.truncated"] = true
			break
		}
		key = sanitizeReporterString(key)
		if key == "" {
			continue
		}
		if isReporterSensitiveKey(key) {
			result[key] = reporterRedactedValue
			itemCount++
			continue
		}
		if stringValue, ok := value.(string); ok && isReporterURLKey(key) {
			result[key] = sanitizeReporterURL(stringValue)
		} else {
			result[key] = sanitizeReporterValue(value, depth+1)
		}
		itemCount++
	}
	return result
}

func sanitizeReporterValue(value any, depth int) any {
	if depth >= reporterMaxDepth {
		return "[TRUNCATED]"
	}
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		return sanitizeReporterString(typed)
	case error:
		return sanitizeReporterString(typed.Error())
	case map[string]any:
		return sanitizeReporterMap(typed, depth)
	case map[string]string:
		converted := make(map[string]any, len(typed))
		for key, item := range typed {
			converted[key] = item
		}
		return sanitizeReporterMap(converted, depth)
	case []any:
		limit := min(len(typed), reporterMaxItems)
		result := make([]any, limit)
		for i, item := range typed[:limit] {
			result[i] = sanitizeReporterValue(item, depth+1)
		}
		return result
	case []string:
		limit := min(len(typed), reporterMaxItems)
		result := make([]string, limit)
		for i, item := range typed[:limit] {
			result[i] = sanitizeReporterString(item)
		}
		return result
	case bool, int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64, float32, float64:
		return typed
	default:
		return sanitizeReporterString(fmt.Sprint(typed))
	}
}

func isReporterSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.NewReplacer("-", "_", ".", "_").Replace(key))
	if normalized == "sql" {
		return true
	}
	for _, sensitive := range reporterSensitiveKeys {
		if normalized == sensitive || strings.Contains(normalized, sensitive) {
			return true
		}
	}
	return false
}

func isReporterURLKey(key string) bool {
	normalized := strings.ToLower(strings.NewReplacer("-", "_", ".", "_").Replace(key))
	return normalized == "url" || strings.HasSuffix(normalized, "_url")
}

func sanitizeReporterURL(value string) string {
	value = sanitizeReporterSecrets(value)
	if index := strings.IndexAny(value, "?#"); index >= 0 {
		return value[:index]
	}
	return value
}

func sanitizeReporterString(value string) string {
	value = sanitizeReporterSecrets(value)
	value = reporterAbsoluteURLPattern.ReplaceAllStringFunc(value, stripReporterURLQuery)
	if len(value) <= reporterMaxStringBytes {
		return value
	}
	return strings.ToValidUTF8(value[:reporterMaxStringBytes], invalidUTF8Replacement) + "...[TRUNCATED]"
}

func sanitizeReporterSecrets(value string) string {
	value = SanitizeString(value)
	for _, pattern := range reporterSensitiveValuePatterns {
		value = pattern.ReplaceAllString(value, reporterRedactedValue)
	}
	if len(value) <= reporterMaxStringBytes {
		return value
	}
	return strings.ToValidUTF8(value[:reporterMaxStringBytes], invalidUTF8Replacement) + "...[TRUNCATED]"
}

func stripReporterURLQuery(value string) string {
	if index := strings.IndexAny(value, "?#"); index >= 0 {
		return value[:index]
	}
	return value
}
