package integrations

import (
	"fmt"
	"testing"
)

func TestNormalizeLocalizedTextCanonicalizesAliasesAndAddsFallback(t *testing.T) {
	localized, err := normalizeLocalizedText(LocalizedText{
		"zh_CN": " 中文 ",
	}, "English", 32)
	if err != nil {
		t.Fatalf("normalizeLocalizedText() error = %v", err)
	}
	if localized[LocaleSimplifiedChinese] != "中文" || localized[LocaleEnglishUS] != "English" {
		t.Fatalf("normalizeLocalizedText() = %#v", localized)
	}
}

func TestNormalizeLocalizedTextRejectsDuplicateCanonicalLocale(t *testing.T) {
	_, err := normalizeLocalizedText(LocalizedText{
		"zh-CN":   "中文一",
		"zh_Hans": "中文二",
	}, "English", 32)
	if err == nil {
		t.Fatal("normalizeLocalizedText() accepted duplicate canonical locale")
	}
}

func TestNormalizeLocalizedTextCanonicalizesGeneralBCP47Tags(t *testing.T) {
	localized, err := normalizeLocalizedText(LocalizedText{
		"fr_fr":      "Français",
		"zh_hant_tw": "繁體中文",
	}, "English", 32)
	if err != nil {
		t.Fatalf("normalizeLocalizedText() error = %v", err)
	}
	if localized["fr-FR"] != "Français" || localized["zh-Hant-TW"] != "繁體中文" {
		t.Fatalf("normalizeLocalizedText() = %#v", localized)
	}
	if _, err := normalizeLocalizedText(LocalizedText{"fr-FR": "un", "fr-fr": "deux"}, "English", 32); err == nil {
		t.Fatal("normalizeLocalizedText() accepted case-only duplicate locales")
	}
}

func TestNormalizeLocalizedTextRejectsLocaleKeysOutsideWireContract(t *testing.T) {
	_, err := normalizeLocalizedText(LocalizedText{
		"en-abcdefghijklmnopqrstuvwxyz-abcdef": "too long",
	}, "English", 32)
	if err == nil {
		t.Fatal("normalizeLocalizedText() accepted a locale key longer than 35 characters")
	}
}

func TestNormalizeLocalizedTextCountsFallbackLocaleInLimit(t *testing.T) {
	values := make(LocalizedText, 16)
	for index := 0; index < 16; index++ {
		values[fmt.Sprintf("qz-%02d", index)] = fmt.Sprintf("value %d", index)
	}
	if _, err := normalizeLocalizedText(values, "English", 32); err == nil {
		t.Fatal("normalizeLocalizedText() accepted 16 explicit locales plus the fallback locale")
	}
}
