package integrations

import "github.com/zgiai/zgi/api/internal/modules/integrations/localization"

const (
	LocaleEnglishUS         = localization.LocaleEnglishUS
	LocaleSimplifiedChinese = localization.LocaleSimplifiedChinese
)

type LocalizedText = localization.LocalizedText

type LocalizedLabelMap = localization.LocalizedLabelMap

func normalizeLocalizedText(values LocalizedText, fallback string, maxRunes int) (LocalizedText, error) {
	return localization.NormalizeText(values, fallback, maxRunes)
}

func cloneLocalizedText(values LocalizedText) LocalizedText {
	return localization.CloneText(values)
}

func normalizeLocalizedLabelMap(values LocalizedLabelMap, allowedValues []string, limit int, maxRunes int) (LocalizedLabelMap, error) {
	return localization.NormalizeLabelMap(values, allowedValues, limit, maxRunes)
}

func cloneLocalizedLabelMap(values LocalizedLabelMap) LocalizedLabelMap {
	return localization.CloneLabelMap(values)
}

func localizedLabelMapSearchValues(values LocalizedLabelMap) []string {
	return localization.LabelMapSearchValues(values)
}

func localizedTextSearchValues(values LocalizedText) []string {
	return localization.TextSearchValues(values)
}
