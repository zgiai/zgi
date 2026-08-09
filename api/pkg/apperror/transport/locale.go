package transport

import (
	"math"
	"strconv"
	"strings"

	"github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

// LocaleFromAcceptLanguage selects the supported language with the highest
// valid quality value. The zero Locale means the request is unsupported or
// unspecified; Projector.Project then applies the catalog's fallback language.
func LocaleFromAcceptLanguage(value string) catalog.Locale {
	bestQuality := -1.0
	var best catalog.Locale

	for _, candidate := range strings.Split(value, ",") {
		parts := strings.Split(candidate, ";")
		locale, supported := catalog.ParseLocale(parts[0])
		if !supported {
			continue
		}

		quality, valid := languageQuality(parts[1:])
		if !valid || quality <= 0 || quality <= bestQuality {
			continue
		}
		best = locale
		bestQuality = quality
	}
	return best
}

func languageQuality(parameters []string) (float64, bool) {
	quality := 1.0
	qualitySeen := false
	for _, parameter := range parameters {
		name, raw, found := strings.Cut(strings.TrimSpace(parameter), "=")
		if !found || !strings.EqualFold(strings.TrimSpace(name), "q") || qualitySeen {
			return 0, false
		}
		parsed, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) || parsed < 0 || parsed > 1 {
			return 0, false
		}
		quality = parsed
		qualitySeen = true
	}
	return quality, true
}
