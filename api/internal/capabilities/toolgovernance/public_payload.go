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
	publicExternalArgumentRedacted  = "__zgi_redacted__"
	publicExternalArgumentTruncated = "__zgi_truncated__"
	publicExternalHiddenReference   = "__zgi_hidden_reference__"
	publicExternalArgumentMaxDepth  = 6
	publicExternalArgumentMaxItems  = 20
	publicExternalArgumentMaxFields = 40
	publicExternalArgumentMaxRunes  = 500
)

var publicExternalSecretPatterns = []*regexp.Regexp{
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

var publicExternalCamelKeyPattern = regexp.MustCompile(`([a-z0-9])([A-Z])`)

// SanitizeExternalActionPublicValue returns a detached, JSON-compatible copy
// suitable for browser-visible events and message metadata. Exact arguments
// and internal Connection UUIDs remain in the server-side frozen invocation;
// the public Connected Apps projection keeps only bounded, credential-redacted
// action arguments plus server-owned connection display labels.
func SanitizeExternalActionPublicValue(value interface{}) interface{} {
	return sanitizeExternalPublicValue(value, 0)
}

func sanitizeExternalPublicValue(value interface{}, depth int) interface{} {
	if value == nil {
		return nil
	}
	if depth > 32 {
		return publicExternalArgumentTruncated
	}
	switch typed := value.(type) {
	case map[string]interface{}:
		externalEnvelope := isExternalActionEnvelope(typed)
		connectionID := ""
		connectionName := ""
		connectionDisplayName := ""
		connectionSelection := ""
		if externalEnvelope {
			connectionID = nestedExternalEnvelopeString(typed, "connection_id")
			connectionName = nestedExternalEnvelopeString(typed, "connection_name")
			connectionDisplayName = nestedExternalEnvelopeString(typed, "connection_display_name")
			connectionSelection = nestedExternalEnvelopeString(typed, "connection_selection")
		}
		out := make(map[string]interface{}, len(typed))
		for key, nested := range typed {
			out[key] = sanitizeExternalPublicValue(nested, depth+1)
		}
		sanitizeExternalActionEnvelope(out)
		if externalEnvelope {
			out = sanitizeExternalConnectionPublicMap(out, connectionID)
			if connectionName = publicExternalConnectionLabel(connectionName, connectionID); connectionName != "" {
				out["connection_name"] = connectionName
			}
			if connectionDisplayName = publicExternalConnectionLabel(connectionDisplayName, connectionID); connectionDisplayName != "" {
				out["connection_display_name"] = connectionDisplayName
			}
			if connectionSelection == "preferred" || connectionSelection == "explicit" {
				out["connection_selection"] = connectionSelection
			}
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(typed))
		for index, nested := range typed {
			out[index] = sanitizeExternalPublicValue(nested, depth+1)
		}
		return out
	case []map[string]interface{}:
		out := make([]interface{}, len(typed))
		for index, nested := range typed {
			out[index] = sanitizeExternalPublicValue(nested, depth+1)
		}
		return out
	case string, bool, float32, float64, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, json.Number:
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
	return sanitizeExternalPublicValue(decoded, depth+1)
}

func sanitizeExternalActionEnvelope(value map[string]interface{}) {
	if !isExternalActionEnvelope(value) {
		return
	}
	arguments, ok := value["arguments"].(map[string]interface{})
	if !ok {
		return
	}
	if actionArguments, exists := arguments["arguments"]; exists {
		arguments["arguments"] = sanitizeExternalApprovalArgument(actionArguments, 0)
	}
	if batchItems, exists := arguments["batch_items"]; exists {
		arguments["batch_items"] = sanitizeExternalApprovalArgument(batchItems, 0)
	}
	if batchSummary, exists := arguments["batch_summary"]; exists {
		arguments["batch_summary"] = sanitizeExternalApprovalArgument(batchSummary, 0)
	}
	// Batch IDs, item IDs and digests are internal replay and approval-binding
	// material. Keep them in the sealed invocation, not the public event copy.
	delete(arguments, "operation_batch")
}

func sanitizeExternalConnectionPublicMap(value map[string]interface{}, connectionID string) map[string]interface{} {
	out := make(map[string]interface{}, len(value))
	isConnectionAsset := strings.EqualFold(strings.TrimSpace(publicPayloadString(value["type"])), "integration_connection")
	for key, nested := range value {
		normalizedKey := strings.ToLower(strings.TrimSpace(key))
		if normalizedKey == "connection_id" || normalizedKey == "preferred_connection_id" ||
			(isConnectionAsset && normalizedKey == "id") {
			continue
		}
		publicKey := key
		if connectionID != "" && strings.Contains(strings.ToLower(key), strings.ToLower(connectionID)) {
			publicKey = publicExternalHiddenReference
		}
		out[publicKey] = sanitizeExternalConnectionPublicValue(nested, connectionID)
	}
	return out
}

func sanitizeExternalConnectionPublicValue(value interface{}, connectionID string) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return sanitizeExternalConnectionPublicMap(typed, connectionID)
	case []interface{}:
		out := make([]interface{}, len(typed))
		for index, nested := range typed {
			out[index] = sanitizeExternalConnectionPublicValue(nested, connectionID)
		}
		return out
	case string:
		if connectionID != "" && strings.Contains(strings.ToLower(typed), strings.ToLower(connectionID)) {
			return publicExternalHiddenReference
		}
		return typed
	default:
		return value
	}
}

func publicExternalConnectionLabel(value string, connectionID string) string {
	value = strings.TrimSpace(value)
	if value == "" || publicExternalStringSensitive(value) ||
		(connectionID != "" && strings.Contains(strings.ToLower(value), strings.ToLower(connectionID))) {
		return ""
	}
	return truncatePublicExternalString(value)
}

func nestedExternalEnvelopeString(value interface{}, key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case map[string]interface{}:
		externalEnvelope := isExternalActionEnvelope(typed)
		if externalEnvelope {
			for candidateKey, nested := range typed {
				if strings.ToLower(strings.TrimSpace(candidateKey)) == key {
					if text := strings.TrimSpace(publicPayloadString(nested)); text != "" {
						return text
					}
				}
			}
			if arguments, ok := typed["arguments"].(map[string]interface{}); ok {
				if text := strings.TrimSpace(publicPayloadString(arguments[key])); text != "" {
					return text
				}
			}
		}
		keys := make([]string, 0, len(typed))
		for candidateKey := range typed {
			keys = append(keys, candidateKey)
		}
		sort.Strings(keys)
		for _, candidateKey := range keys {
			// In a facade envelope the nested action arguments are user/provider
			// data, not server-owned connection labels. Never promote them.
			if externalEnvelope && strings.EqualFold(strings.TrimSpace(candidateKey), "arguments") {
				continue
			}
			if text := nestedExternalEnvelopeString(typed[candidateKey], key); text != "" {
				return text
			}
		}
	case []interface{}:
		for _, nested := range typed {
			if text := nestedExternalEnvelopeString(nested, key); text != "" {
				return text
			}
		}
	default:
		reflected := reflect.ValueOf(value)
		if reflected.Kind() == reflect.Pointer || reflected.Kind() == reflect.Struct || reflected.Kind() == reflect.Map || reflected.Kind() == reflect.Slice || reflected.Kind() == reflect.Array {
			raw, err := json.Marshal(value)
			if err == nil {
				var decoded interface{}
				decoder := json.NewDecoder(strings.NewReader(string(raw)))
				decoder.UseNumber()
				if decoder.Decode(&decoded) == nil {
					return nestedExternalEnvelopeString(decoded, key)
				}
			}
		}
	}
	return ""
}

func isExternalActionEnvelope(value map[string]interface{}) bool {
	skillID := strings.ToLower(strings.TrimSpace(publicPayloadString(value["skill_id"])))
	toolName := strings.ToLower(strings.TrimSpace(publicPayloadString(value["tool_name"])))
	providerID := strings.ToLower(strings.TrimSpace(publicPayloadString(value["provider_id"])))
	return (skillID == "external-apps" && toolName == "execute_action") || providerID == "external-integrations"
}

func sanitizeExternalApprovalArgument(value interface{}, depth int) interface{} {
	if depth > publicExternalArgumentMaxDepth {
		return publicExternalArgumentTruncated
	}
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		if publicExternalStringSensitive(typed) {
			return publicExternalArgumentRedacted
		}
		return truncatePublicExternalString(typed)
	case bool, float32, float64, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, json.Number:
		return typed
	case []interface{}:
		limit := min(len(typed), publicExternalArgumentMaxItems)
		out := make([]interface{}, 0, limit)
		for _, nested := range typed[:limit] {
			out = append(out, sanitizeExternalApprovalArgument(nested, depth+1))
		}
		return out
	case []string:
		limit := min(len(typed), publicExternalArgumentMaxItems)
		out := make([]interface{}, 0, limit)
		for _, nested := range typed[:limit] {
			out = append(out, sanitizeExternalApprovalArgument(nested, depth+1))
		}
		return out
	case map[string]interface{}:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if len(keys) > publicExternalArgumentMaxFields {
			keys = keys[:publicExternalArgumentMaxFields]
		}
		out := make(map[string]interface{}, len(keys))
		for _, key := range keys {
			if publicExternalKeySensitive(key) {
				out[key] = publicExternalArgumentRedacted
				continue
			}
			out[key] = sanitizeExternalApprovalArgument(typed[key], depth+1)
		}
		return out
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return publicExternalArgumentTruncated
	}
	var decoded interface{}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return publicExternalArgumentTruncated
	}
	return sanitizeExternalApprovalArgument(decoded, depth+1)
}

func publicExternalKeySensitive(raw string) bool {
	normalized := strings.TrimSpace(raw)
	normalized = publicExternalCamelKeyPattern.ReplaceAllString(normalized, `${1}_${2}`)
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

func publicExternalStringSensitive(value string) bool {
	for _, pattern := range publicExternalSecretPatterns {
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
		if publicExternalKeySensitive(key) && len(values) > 0 {
			return true
		}
	}
	return false
}

func truncatePublicExternalString(value string) string {
	runes := []rune(value)
	if len(runes) <= publicExternalArgumentMaxRunes {
		return value
	}
	return string(runes[:publicExternalArgumentMaxRunes]) + "…"
}

func publicPayloadString(value interface{}) string {
	text, _ := value.(string)
	return text
}
