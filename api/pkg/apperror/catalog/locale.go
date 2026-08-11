package catalog

import "strings"

// Locale identifies one supported public-message language.
type Locale string

const (
	LocaleEnglishUS         Locale = "en-US"
	LocaleChineseSimplified Locale = "zh-Hans"
)

// SupportedLocales returns the locales every product definition must provide.
func SupportedLocales() []Locale {
	return []Locale{LocaleEnglishUS, LocaleChineseSimplified}
}

// ParseLocale normalizes common language tags. The boolean is false when the
// requested locale is unsupported. Protocol adapters should pass the zero
// Locale to Present in that case; Present selects the catalog's fallback
// language while preserving the original error code and metadata. Fallback is
// reserved for unknown codes or messages that cannot be rendered safely.
func ParseLocale(value string) (Locale, bool) {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "_", "-"))
	switch normalized {
	case "en", "en-us":
		return LocaleEnglishUS, true
	case "zh", "zh-cn", "zh-sg", "zh-hans":
		return LocaleChineseSimplified, true
	default:
		return "", false
	}
}

func isSupportedLocale(locale Locale) bool {
	return locale == LocaleEnglishUS || locale == LocaleChineseSimplified
}
