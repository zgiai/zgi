package catalog

import (
	"regexp"
	"strings"
)

// LocalizedText is optional, secret-free presentation copy keyed by BCP 47
// locale tags. Its scalar sibling remains the compatibility fallback; it is
// never an operation identity or an authorization/risk input.
type LocalizedText map[string]string

var localeTagPattern = regexp.MustCompile(`^[A-Za-z]{2,8}(?:-[A-Za-z0-9]{1,8})*$`)

// Clone detaches localized presentation maps from immutable catalog snapshots.
func (values LocalizedText) Clone() LocalizedText {
	if len(values) == 0 {
		return nil
	}
	cloned := make(LocalizedText, len(values))
	for locale, value := range values {
		cloned[locale] = value
	}
	return cloned
}

func validLocalizedText(values LocalizedText) bool {
	if len(values) > 16 {
		return false
	}
	seen := make(map[string]struct{}, len(values))
	for locale, value := range values {
		if len(locale) > 35 || !localeTagPattern.MatchString(locale) || strings.TrimSpace(value) == "" {
			return false
		}
		key := strings.ToLower(locale)
		switch key {
		case "en", "en-us":
			key = "en-us"
		case "zh", "zh-cn", "zh-hans":
			key = "zh-hans"
		}
		if _, exists := seen[key]; exists {
			return false
		}
		seen[key] = struct{}{}
	}
	return true
}
