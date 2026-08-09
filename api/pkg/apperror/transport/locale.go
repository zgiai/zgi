package transport

import (
	"strings"

	"github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

// LocaleFromAcceptLanguage selects the supported language with the highest
// valid quality value. The zero Locale means the request is unsupported or
// unspecified; Projector.Project then applies the catalog's fallback language.
func LocaleFromAcceptLanguage(value string) catalog.Locale {
	type effectivePreference struct {
		quality     int
		specificity int
		order       int
	}

	supportedLocales := catalog.SupportedLocales()
	effective := make(map[catalog.Locale]effectivePreference, len(supportedLocales))
	candidates := strings.Split(value, ",")

	for order, candidate := range candidates {
		parts := strings.Split(candidate, ";")
		rangeValue := strings.TrimSpace(parts[0])
		quality, valid := languageQuality(parts[1:])
		if !valid {
			continue
		}

		locales := supportedLocales
		specificity := 0
		if rangeValue != "*" {
			locale, supported := catalog.ParseLocale(rangeValue)
			if !supported {
				continue
			}
			locales = []catalog.Locale{locale}
			specificity = strings.Count(strings.ReplaceAll(rangeValue, "_", "-"), "-") + 1
		}

		for _, locale := range locales {
			current, exists := effective[locale]
			if exists && (specificity < current.specificity ||
				(specificity == current.specificity && quality < current.quality) ||
				(specificity == current.specificity && quality == current.quality && order > current.order)) {
				continue
			}
			effective[locale] = effectivePreference{
				quality:     quality,
				specificity: specificity,
				order:       order,
			}
		}
	}

	bestQuality := 0
	bestOrder := len(candidates)
	var best catalog.Locale
	for _, locale := range supportedLocales {
		preference, exists := effective[locale]
		if !exists {
			continue
		}
		if preference.quality > bestQuality ||
			(preference.quality == bestQuality && preference.quality > 0 && preference.order < bestOrder) {
			best = locale
			bestQuality = preference.quality
			bestOrder = preference.order
		}
	}
	return best
}

func languageQuality(parameters []string) (int, bool) {
	quality := 1000
	qualitySeen := false
	for _, parameter := range parameters {
		name, raw, found := strings.Cut(strings.TrimSpace(parameter), "=")
		if !found || !strings.EqualFold(strings.TrimSpace(name), "q") || qualitySeen {
			return 0, false
		}
		parsed, valid := parseQuality(strings.TrimSpace(raw))
		if !valid {
			return 0, false
		}
		quality = parsed
		qualitySeen = true
	}
	return quality, true
}

// parseQuality accepts the HTTP qvalue grammar and returns thousandths. Using
// an integer avoids permissive floating-point forms such as 1e0, +0.5, and .5.
func parseQuality(value string) (int, bool) {
	if value == "" || (value[0] != '0' && value[0] != '1') {
		return 0, false
	}
	whole := int(value[0] - '0')
	if len(value) == 1 {
		return whole * 1000, true
	}
	if value[1] != '.' || len(value) > 5 {
		return 0, false
	}

	fraction := 0
	place := 100
	for index := 2; index < len(value); index++ {
		digit := value[index]
		if digit < '0' || digit > '9' || (whole == 1 && digit != '0') {
			return 0, false
		}
		fraction += int(digit-'0') * place
		place /= 10
	}
	return whole*1000 + fraction, true
}
