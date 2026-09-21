package channelprovider

import (
	"strings"
	"unicode"
)

// identitySuffixes are trailing identity words folded away so a commercial
// supplier spelling such as "agicto supplier" matches the bare key "agicto".
// Both separated and concatenated forms are handled by trimming the suffix
// string directly from the normalized key.
var identitySuffixes = []string{
	"supplier", "suppliers",
	"vendor", "vendors",
	"provider", "providers",
	"channel", "channels",
	"供应商", "提供商", "渠道", "厂商", "官方",
}

// NormalizeIdentityKey folds a provider or vendor spelling into the stable
// identity key used by metered pre-deduct and reconciliation matching.
//
// Rules, in order:
//  1. lowercase and trim;
//  2. collapse known channel-provider aliases to their canonical spec name;
//  3. replace every non letter/digit rune with a single space;
//  4. strip trailing supplier/vendor/channel identity words (English and CJK).
//
// Examples:
//
//	"agicto"            -> "agicto"
//	"Agicto Supplier"   -> "agicto"
//	"agicto-supplier"   -> "agicto"
//	"agicto 供应商"      -> "agicto"
//	"zhipu"             -> "glm"
func NormalizeIdentityKey(raw string) string {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" {
		return ""
	}
	if spec, ok := specs[trimmed]; ok {
		return spec.Name
	}

	normalized := normalizeIdentityText(trimmed)
	if normalized == "" {
		return ""
	}
	if spec, ok := specs[strings.ReplaceAll(normalized, " ", "")]; ok {
		return spec.Name
	}

	key := normalized
	for {
		next := key
		for _, suffix := range identitySuffixes {
			if strings.HasSuffix(next, suffix) {
				next = strings.TrimSpace(strings.TrimSuffix(next, suffix))
			}
		}
		if next == key {
			break
		}
		key = next
	}
	if key == "" {
		return normalized
	}
	return key
}

// normalizeIdentityText lowercases via the caller and folds every rune that is
// not a letter or digit into a single separating space.
func normalizeIdentityText(value string) string {
	var builder strings.Builder
	pendingSpace := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if pendingSpace && builder.Len() > 0 {
				builder.WriteByte(' ')
			}
			pendingSpace = false
			builder.WriteRune(r)
			continue
		}
		pendingSpace = true
	}
	return strings.TrimSpace(builder.String())
}
