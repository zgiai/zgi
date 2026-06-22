package sensitiveredaction

import (
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

var highRiskTypes = map[string]struct{}{
	"secret":      {},
	"token":       {},
	"password":    {},
	"private_key": {},
}

func replacementFor(entityType string, value string, options redactionOptions) string {
	if _, highRisk := highRiskTypes[entityType]; highRisk {
		if options.Strategy == "label" {
			return labelReplacement(entityType)
		}
		return fullReplacement(entityType)
	}
	switch options.Strategy {
	case "label":
		return labelReplacement(entityType)
	case "full":
		return fullReplacement(entityType)
	default:
		return partialReplacement(entityType, value, options)
	}
}

func fullReplacement(entityType string) string {
	return "[REDACTED_" + strings.ToUpper(entityType) + "]"
}

func labelReplacement(entityType string) string {
	return "[" + strings.ToUpper(entityType) + "]"
}

func partialReplacement(entityType string, value string, options redactionOptions) string {
	switch entityType {
	case "phone":
		return maskDigits(value, 3, options.Preserve.KeepLastDigits, "*")
	case "email":
		return maskEmail(value, options.Preserve.KeepEmailDomain)
	case "id_card", "bank_card":
		if options.Level == "high" {
			return fullReplacement(entityType)
		}
		return maskDigits(value, 2, options.Preserve.KeepLastDigits, "*")
	case "ip":
		return maskIP(value)
	case "name", "customer_name":
		return maskName(value)
	case "company":
		return labelReplacement(entityType)
	case "address":
		return maskAddress(value, options.Preserve.KeepCity)
	case "order_id", "contract_id":
		return maskIdentifier(value, options.Preserve.KeepLastDigits)
	case "url_parameter":
		return fullReplacement(entityType)
	default:
		return fullReplacement(entityType)
	}
}

func maskDigits(value string, keepPrefix int, keepSuffix int, mask string) string {
	digits := digitsOnly(value)
	if len(digits) <= keepPrefix+keepSuffix {
		return strings.Repeat(mask, maxInt(4, len(digits)))
	}
	if keepSuffix < 0 {
		keepSuffix = 0
	}
	if keepPrefix < 0 {
		keepPrefix = 0
	}
	prefix := digits[:keepPrefix]
	suffix := ""
	if keepSuffix > 0 {
		suffix = digits[len(digits)-keepSuffix:]
	}
	return prefix + strings.Repeat(mask, maxInt(4, len(digits)-keepPrefix-keepSuffix)) + suffix
}

func maskEmail(value string, keepDomain bool) string {
	parts := strings.SplitN(value, "@", 2)
	if len(parts) != 2 {
		return fullReplacement("email")
	}
	local := parts[0]
	domain := parts[1]
	if local == "" {
		local = "*"
	}
	first, _ := utf8.DecodeRuneInString(local)
	maskedLocal := string(first) + strings.Repeat("*", maxInt(3, utf8.RuneCountInString(local)-1))
	if keepDomain {
		return maskedLocal + "@" + domain
	}
	return maskedLocal + "@[REDACTED_DOMAIN]"
}

func maskIP(value string) string {
	if strings.Contains(value, ".") {
		parts := strings.Split(value, ".")
		if len(parts) == 4 {
			return parts[0] + "." + parts[1] + ".*.*"
		}
	}
	return "[REDACTED_IP]"
}

func maskName(value string) string {
	trimmed := strings.TrimSpace(value)
	runes := []rune(trimmed)
	if len(runes) == 0 {
		return fullReplacement("name")
	}
	if len(runes) == 1 {
		return string(runes[0]) + "*"
	}
	return string(runes[0]) + strings.Repeat("*", len(runes)-1)
}

func maskAddress(value string, keepCity bool) string {
	if !keepCity {
		return fullReplacement("address")
	}
	for _, marker := range []string{"市", "City"} {
		if idx := strings.Index(value, marker); idx >= 0 {
			return strings.TrimSpace(value[:idx+len(marker)]) + "[REDACTED_ADDRESS]"
		}
	}
	return fullReplacement("address")
}

func maskIdentifier(value string, keepSuffix int) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "****"
	}
	runes := []rune(trimmed)
	if keepSuffix <= 0 || len(runes) <= keepSuffix {
		return strings.Repeat("*", maxInt(4, len(runes)))
	}
	return strings.Repeat("*", maxInt(4, len(runes)-keepSuffix)) + string(runes[len(runes)-keepSuffix:])
}

func redactURL(raw string, options redactionOptions) (string, int) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.RawQuery == "" {
		return raw, 0
	}
	values, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return raw, 0
	}
	count := 0
	for key, list := range values {
		if !isSensitiveURLParam(key) {
			continue
		}
		for i := range list {
			list[i] = replacementFor("url_parameter", list[i], options)
			count++
		}
		values[key] = list
	}
	if count == 0 {
		return raw, 0
	}
	if !options.Preserve.KeepURLDomain {
		return "[REDACTED_URL]", count
	}
	parsed.RawQuery = values.Encode()
	return parsed.String(), count
}

func isSensitiveURLParam(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	for _, token := range []string{"token", "key", "secret", "password", "passwd", "pwd", "session", "sid", "auth", "credential"} {
		if strings.Contains(normalized, token) {
			return true
		}
	}
	return false
}

func digitsOnly(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}
