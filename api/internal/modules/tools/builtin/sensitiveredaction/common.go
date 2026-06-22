package sensitiveredaction

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	defaultRedactionLevel    = "medium"
	defaultRedactionStrategy = "auto"
	maxRedactionInputBytes   = 512 * 1024
)

var validLevels = map[string]struct{}{
	"low":    {},
	"medium": {},
	"high":   {},
}

var validStrategies = map[string]struct{}{
	"auto":    {},
	"partial": {},
	"full":    {},
	"label":   {},
}

var validEntityTypes = map[string]struct{}{
	"phone":         {},
	"email":         {},
	"id_card":       {},
	"bank_card":     {},
	"address":       {},
	"name":          {},
	"customer_name": {},
	"company":       {},
	"order_id":      {},
	"contract_id":   {},
	"secret":        {},
	"token":         {},
	"password":      {},
	"private_key":   {},
	"ip":            {},
	"url_parameter": {},
}

type redactionOptions struct {
	Level            string
	Strategy         string
	Preserve         preserveRules
	EntityTypes      map[string]struct{}
	Locale           string
	IncludeFieldList bool
}

type preserveRules struct {
	KeepLastDigits  int  `json:"keep_last_digits"`
	KeepEmailDomain bool `json:"keep_email_domain"`
	KeepCity        bool `json:"keep_city"`
	KeepURLDomain   bool `json:"keep_url_domain"`
}

func parseOptions(params map[string]interface{}) (redactionOptions, error) {
	level := normalizeStringParam(params, "level", defaultRedactionLevel)
	if _, ok := validLevels[level]; !ok {
		return redactionOptions{}, fmt.Errorf("level must be low, medium, or high")
	}
	strategy := normalizeStringParam(params, "strategy", defaultRedactionStrategy)
	if _, ok := validStrategies[strategy]; !ok {
		return redactionOptions{}, fmt.Errorf("strategy must be auto, partial, full, or label")
	}
	if strategy == "auto" {
		switch level {
		case "high":
			strategy = "full"
		default:
			strategy = "partial"
		}
	}
	preserve := preserveRules{
		KeepLastDigits:  4,
		KeepEmailDomain: true,
		KeepURLDomain:   true,
	}
	if raw, ok := params["preserve_rules"]; ok {
		if err := decodePreserveRules(raw, &preserve); err != nil {
			return redactionOptions{}, err
		}
	}
	if preserve.KeepLastDigits < 0 || preserve.KeepLastDigits > 8 {
		return redactionOptions{}, fmt.Errorf("preserve_rules.keep_last_digits must be between 0 and 8")
	}
	entityTypes, err := parseEntityTypes(params["entity_types"])
	if err != nil {
		return redactionOptions{}, err
	}
	locale := strings.TrimSpace(rawString(params["locale"]))
	if locale == "" {
		locale = "auto"
	}
	switch strings.ToLower(locale) {
	case "auto":
		locale = "auto"
	case "zh-cn":
		locale = "zh-CN"
	case "en-us":
		locale = "en-US"
	default:
		return redactionOptions{}, fmt.Errorf("locale must be auto, zh-CN, or en-US")
	}
	return redactionOptions{
		Level:            level,
		Strategy:         strategy,
		Preserve:         preserve,
		EntityTypes:      entityTypes,
		Locale:           locale,
		IncludeFieldList: boolParam(params, "include_field_list", true),
	}, nil
}

func normalizeStringParam(params map[string]interface{}, name string, fallback string) string {
	value := strings.TrimSpace(rawString(params[name]))
	if value == "" {
		return fallback
	}
	return strings.ToLower(value)
}

func rawString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	case json.Number:
		return v.String()
	case float64:
		if math.Trunc(v) == v {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case nil:
		return ""
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(encoded)
	}
}

func boolParam(params map[string]interface{}, name string, fallback bool) bool {
	raw, ok := params[name]
	if !ok {
		return fallback
	}
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(v))
		if err != nil {
			return fallback
		}
		return parsed
	default:
		return fallback
	}
}

func decodePreserveRules(raw interface{}, out *preserveRules) error {
	switch value := raw.(type) {
	case map[string]interface{}:
		applyPreserveMap(value, out)
		return nil
	case string:
		text := strings.TrimSpace(value)
		if text == "" {
			return nil
		}
		var decoded map[string]interface{}
		if err := json.Unmarshal([]byte(text), &decoded); err != nil {
			return fmt.Errorf("preserve_rules must be an object or JSON object string")
		}
		applyPreserveMap(decoded, out)
		return nil
	default:
		return fmt.Errorf("preserve_rules must be an object or JSON object string")
	}
}

func applyPreserveMap(values map[string]interface{}, out *preserveRules) {
	for key, value := range values {
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "keep_last_digits":
			if parsed, ok := intFromAny(value); ok {
				out.KeepLastDigits = parsed
			}
		case "keep_email_domain":
			if parsed, ok := boolFromAny(value); ok {
				out.KeepEmailDomain = parsed
			}
		case "keep_city":
			if parsed, ok := boolFromAny(value); ok {
				out.KeepCity = parsed
			}
		case "keep_url_domain":
			if parsed, ok := boolFromAny(value); ok {
				out.KeepURLDomain = parsed
			}
		}
	}
}

func parseEntityTypes(raw interface{}) (map[string]struct{}, error) {
	if raw == nil || strings.TrimSpace(rawString(raw)) == "" {
		return nil, nil
	}
	values := []string{}
	switch typed := raw.(type) {
	case []interface{}:
		for _, item := range typed {
			values = append(values, rawString(item))
		}
	case []string:
		values = append(values, typed...)
	case string:
		text := strings.TrimSpace(typed)
		if strings.HasPrefix(text, "[") {
			if err := json.Unmarshal([]byte(text), &values); err != nil {
				return nil, fmt.Errorf("entity_types must be an array, JSON array string, or comma-separated string")
			}
		} else {
			values = strings.Split(text, ",")
		}
	default:
		return nil, fmt.Errorf("entity_types must be an array, JSON array string, or comma-separated string")
	}
	result := map[string]struct{}{}
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized == "" {
			continue
		}
		if _, ok := validEntityTypes[normalized]; !ok {
			return nil, fmt.Errorf("unsupported entity type: %s", normalized)
		}
		result[normalized] = struct{}{}
	}
	return result, nil
}

func intFromAny(value interface{}) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		if math.Trunc(v) != v {
			return 0, false
		}
		return int(v), true
	case json.Number:
		parsed, err := v.Int64()
		return int(parsed), err == nil
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(v))
		return parsed, err == nil
	default:
		return 0, false
	}
}

func boolFromAny(value interface{}) (bool, bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(v))
		return parsed, err == nil
	default:
		return false, false
	}
}

func (o redactionOptions) wants(entityType string) bool {
	if len(o.EntityTypes) == 0 {
		return true
	}
	_, ok := o.EntityTypes[entityType]
	return ok
}
