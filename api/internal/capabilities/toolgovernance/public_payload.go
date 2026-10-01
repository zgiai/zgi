package toolgovernance

import (
	"encoding/json"
	"net/url"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

const (
	publicToolArgumentRedacted  = "__zgi_redacted__"
	publicToolArgumentTruncated = "__zgi_truncated__"
	publicToolArgumentMaxDepth  = 6
	publicToolArgumentMaxItems  = 20
	publicToolArgumentMaxFields = 40
	publicToolArgumentMaxRunes  = 500
)

var publicToolSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)-----BEGIN(?: [A-Z0-9]+)? PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)\b(?:bearer|basic)\s+[a-z0-9._~+/=-]{8,}`),
	regexp.MustCompile(`(?i)\b(?:proxy-)?authorization\s*[:=]\s*[^\r\n]{4,}`),
	regexp.MustCompile(`(?i)\b(?:set-)?cookie\s*:\s*[^\r\n]{8,}`),
	regexp.MustCompile(`(?i)\b(?:sk|exa)[-_][a-z0-9_-]{12,}`),
	regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,}\b`),
	regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}\b`),
	regexp.MustCompile(`\b(?:AKIA|ASIA)[A-Z0-9]{16}\b`),
	regexp.MustCompile(`\bAIza[A-Za-z0-9_-]{20,}\b`),
	regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{16,}\b`),
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]{5,}\b`),
	regexp.MustCompile(`(?i)\b(?:api[_-]?key|access[_-]?token|refresh[_-]?token|client[_-]?secret|password|passwd|token|secret|access[_-]?key)\s*[:=]\s*["']?[^\s"'&,;}]{8,}`),
	regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^\s/:@]+:[^\s/@]+@`),
	regexp.MustCompile(`(?i)(?:[?&]|\b)(?:access[_-]?token|refresh[_-]?token|api[_-]?key|apikey|client[_-]?secret|password|secret|signature|sig|token|key|googleaccessid|key[_-]?pair[_-]?id|x-amz-credential|x-amz-signature|x-amz-security-token|x-goog-credential|x-goog-signature)=[^&#\s]+`),
}

var publicToolCamelKeyPattern = regexp.MustCompile(`([a-z0-9])([A-Z])`)

// SanitizePublicValue returns a detached, JSON-compatible copy
// suitable for browser-visible tool events and message metadata. Private
// replay state stays on the server; argument previews are bounded and secrets
// are redacted independently of provider identity.
func SanitizePublicValue(value interface{}) interface{} {
	return sanitizePublicValue(value, 0)
}

func sanitizePublicValue(value interface{}, depth int) interface{} {
	if value == nil {
		return nil
	}
	if depth > 32 {
		return publicToolArgumentTruncated
	}
	switch typed := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for key, nested := range typed {
			switch key {
			case "connector_action_instances", "frozen_invocation":
				continue
			case "arguments":
				out[key] = sanitizePublicArgument(nested, 0)
			default:
				if publicToolKeySensitive(key) {
					out[key] = publicToolArgumentRedacted
				} else {
					out[key] = sanitizePublicValue(nested, depth+1)
				}
			}
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(typed))
		for index, nested := range typed {
			out[index] = sanitizePublicValue(nested, depth+1)
		}
		return out
	case []map[string]interface{}:
		out := make([]interface{}, len(typed))
		for index, nested := range typed {
			out[index] = sanitizePublicValue(nested, depth+1)
		}
		return out
	case string:
		if publicToolStringSensitive(typed) {
			return publicToolArgumentRedacted
		}
		return typed
	case bool, float32, float64, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, json.Number:
		return typed
	}

	reflected := reflect.ValueOf(value)
	if reflected.Kind() != reflect.Pointer && reflected.Kind() != reflect.Struct && reflected.Kind() != reflect.Map && reflected.Kind() != reflect.Slice && reflected.Kind() != reflect.Array {
		return value
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var decoded interface{}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return nil
	}
	return sanitizePublicValue(decoded, depth+1)
}

func sanitizePublicArgument(value interface{}, depth int) interface{} {
	if depth > publicToolArgumentMaxDepth {
		return publicToolArgumentTruncated
	}
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		if publicToolStringSensitive(typed) {
			return publicToolArgumentRedacted
		}
		return truncatePublicToolString(typed)
	case bool, float32, float64, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, json.Number:
		return typed
	case []interface{}:
		limit := min(len(typed), publicToolArgumentMaxItems)
		out := make([]interface{}, 0, limit)
		for _, nested := range typed[:limit] {
			out = append(out, sanitizePublicArgument(nested, depth+1))
		}
		return out
	case []string:
		limit := min(len(typed), publicToolArgumentMaxItems)
		out := make([]interface{}, 0, limit)
		for _, nested := range typed[:limit] {
			out = append(out, sanitizePublicArgument(nested, depth+1))
		}
		return out
	case map[string]interface{}:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if len(keys) > publicToolArgumentMaxFields {
			keys = keys[:publicToolArgumentMaxFields]
		}
		out := make(map[string]interface{}, len(keys))
		for _, key := range keys {
			if publicToolKeySensitive(key) {
				out[key] = publicToolArgumentRedacted
				continue
			}
			out[key] = sanitizePublicArgument(typed[key], depth+1)
		}
		return out
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return publicToolArgumentTruncated
	}
	var decoded interface{}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return publicToolArgumentTruncated
	}
	return sanitizePublicArgument(decoded, depth+1)
}

func publicToolKeySensitive(raw string) bool {
	normalized := strings.TrimSpace(raw)
	normalized = publicToolCamelKeyPattern.ReplaceAllString(normalized, `${1}_${2}`)
	normalized = strings.ToLower(strings.NewReplacer("-", "_", ".", "_").Replace(normalized))
	for _, key := range []string{
		"authorization", "proxy_authorization", "cookie", "set_cookie", "credential", "credentials",
		"password", "passwd", "private_key", "secret", "client_secret", "token", "access_token",
		"refresh_token", "api_key", "apikey", "access_key", "key", "googleaccessid", "key_pair_id", "session", "jwt", "signature",
	} {
		if normalized == key || strings.HasSuffix(normalized, "_"+key) {
			return true
		}
	}
	return strings.HasPrefix(normalized, "x_amz_") && (strings.Contains(normalized, "credential") || strings.Contains(normalized, "signature") || strings.Contains(normalized, "security_token")) ||
		strings.HasPrefix(normalized, "x_goog_") && (strings.Contains(normalized, "credential") || strings.Contains(normalized, "signature"))
}

func publicToolStringSensitive(value string) bool {
	for _, pattern := range publicToolSecretPatterns {
		if pattern.MatchString(value) {
			return true
		}
	}
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || !parsed.IsAbs() {
		return false
	}
	if parsed.User != nil {
		return true
	}
	for key, values := range parsed.Query() {
		if publicToolKeySensitive(key) && len(values) > 0 {
			return true
		}
	}
	return false
}

func truncatePublicToolString(value string) string {
	runes := []rune(value)
	if len(runes) <= publicToolArgumentMaxRunes {
		return value
	}
	return string(runes[:publicToolArgumentMaxRunes]) + "…"
}
