package integrations

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const (
	LocaleEnglishUS         = "en-US"
	LocaleSimplifiedChinese = "zh-Hans"
)

// LocalizedText is safe, secret-free catalog copy keyed by BCP 47 language
// tags. The non-localized sibling field remains the compatibility fallback.
type LocalizedText map[string]string

// LocalizedLabelMap attaches provider-owned display labels to stable catalog
// identifiers such as categories, tags, and authorization scopes. The outer
// key must be declared by the corresponding provider or action contract.
type LocalizedLabelMap map[string]LocalizedText

var localeTagPattern = regexp.MustCompile(`^[A-Za-z]{2,8}(?:-[A-Za-z0-9]{1,8})*$`)

func normalizeLocalizedText(values LocalizedText, fallback string, maxRunes int) (LocalizedText, error) {
	if len(values) > 16 {
		return nil, fmt.Errorf("localized text has too many locales")
	}
	out := make(LocalizedText, len(values)+1)
	for rawLocale, rawValue := range values {
		locale := canonicalCatalogLocale(rawLocale)
		value := strings.TrimSpace(rawValue)
		if len(locale) > 35 || !localeTagPattern.MatchString(locale) || value == "" {
			return nil, fmt.Errorf("localized text contains an invalid locale or empty value")
		}
		if len([]rune(value)) > maxRunes {
			return nil, fmt.Errorf("localized text is too large")
		}
		if _, duplicated := out[locale]; duplicated {
			return nil, fmt.Errorf("localized text locale %s is duplicated", locale)
		}
		out[locale] = value
	}
	if fallback = strings.TrimSpace(fallback); fallback != "" {
		if _, exists := out[LocaleEnglishUS]; !exists {
			if len(out) >= 16 {
				return nil, fmt.Errorf("localized text has too many locales after adding the fallback locale")
			}
			out[LocaleEnglishUS] = fallback
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func canonicalCatalogLocale(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "_", "-"))
	switch strings.ToLower(value) {
	case "en", "en-us":
		return LocaleEnglishUS
	case "zh", "zh-cn", "zh-hans":
		return LocaleSimplifiedChinese
	}
	parts := strings.Split(strings.ToLower(value), "-")
	for index := 1; index < len(parts); index++ {
		switch {
		case len(parts[index]) == 4:
			parts[index] = strings.ToUpper(parts[index][:1]) + parts[index][1:]
		case len(parts[index]) == 2:
			parts[index] = strings.ToUpper(parts[index])
		}
	}
	return strings.Join(parts, "-")
}

func cloneLocalizedText(values LocalizedText) LocalizedText {
	if len(values) == 0 {
		return nil
	}
	out := make(LocalizedText, len(values))
	for locale, value := range values {
		out[locale] = value
	}
	return out
}

func normalizeLocalizedLabelMap(values LocalizedLabelMap, allowedValues []string, limit int, maxRunes int) (LocalizedLabelMap, error) {
	if len(values) == 0 {
		return nil, nil
	}
	if limit <= 0 || len(values) > limit {
		return nil, fmt.Errorf("localized label map has too many entries")
	}
	allowed := make(map[string]struct{}, len(allowedValues))
	for _, value := range allowedValues {
		if normalized := strings.ToLower(strings.TrimSpace(value)); normalized != "" {
			allowed[normalized] = struct{}{}
		}
	}
	out := make(LocalizedLabelMap, len(values))
	for rawKey, rawLabels := range values {
		key := strings.ToLower(strings.TrimSpace(rawKey))
		if _, ok := allowed[key]; !ok {
			return nil, fmt.Errorf("localized label map key %q is not declared", rawKey)
		}
		if _, duplicated := out[key]; duplicated {
			return nil, fmt.Errorf("localized label map key %q is duplicated", key)
		}
		labels, err := normalizeLocalizedText(rawLabels, "", maxRunes)
		if err != nil {
			return nil, fmt.Errorf("localized label map key %q: %w", key, err)
		}
		if len(labels) == 0 {
			return nil, fmt.Errorf("localized label map key %q has no labels", key)
		}
		out[key] = labels
	}
	encoded, err := json.Marshal(out)
	if err != nil || len(encoded) > 64*1024 {
		return nil, fmt.Errorf("localized label map is too large")
	}
	return out, nil
}

func cloneLocalizedLabelMap(values LocalizedLabelMap) LocalizedLabelMap {
	if len(values) == 0 {
		return nil
	}
	out := make(LocalizedLabelMap, len(values))
	for key, labels := range values {
		out[key] = cloneLocalizedText(labels)
	}
	return out
}

func localizedLabelMapSearchValues(values LocalizedLabelMap) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var out []string
	for _, key := range keys {
		out = append(out, localizedTextSearchValues(values[key])...)
	}
	return out
}

func localizedTextSearchValues(values LocalizedText) []string {
	if len(values) == 0 {
		return nil
	}
	locales := make([]string, 0, len(values))
	for locale := range values {
		locales = append(locales, locale)
	}
	sort.Strings(locales)
	texts := make([]string, 0, len(locales))
	for _, locale := range locales {
		if value := strings.TrimSpace(values[locale]); value != "" {
			texts = append(texts, value)
		}
	}
	return texts
}
