package llmerrors

import (
	"testing"

	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

func TestCatalogDefinitionsComposeWithSharedCatalog(t *testing.T) {
	t.Parallel()

	definitions := append(appcatalog.DefaultDefinitions(), CatalogDefinitions()...)
	productCatalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, definitions...)
	if err != nil {
		t.Fatalf("compose catalog: %v", err)
	}
	for _, definition := range CatalogDefinitions() {
		for _, locale := range appcatalog.SupportedLocales() {
			presentation, presentErr := productCatalog.Present(definition.Code, locale, nil)
			if presentErr != nil {
				t.Fatalf("Present(%s, %s): %v", definition.Code, locale, presentErr)
			}
			if presentation.Message == "" || presentation.Code != definition.Code {
				t.Fatalf("Present(%s, %s) = %#v", definition.Code, locale, presentation)
			}
		}
	}
}

func TestCatalogLegacyNamespacesPreserveConflictingMeanings(t *testing.T) {
	t.Parallel()

	definitions := append(appcatalog.DefaultDefinitions(), CatalogDefinitions()...)
	productCatalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, definitions...)
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]struct {
		want string
	}{
		"llm.gateway:40001": {want: AppCodeRequestInvalid.String()},
		"llm.gateway:40101": {want: AppCodeAPIKeyInvalid.String()},
		"llm.gateway:40102": {want: AppCodeAPIKeyExpired.String()},
		"llm.gateway:40103": {want: AppCodeAPIKeyInactive.String()},
		"llm.domain:40102":  {want: AppCodeAPIKeyInactive.String()},
		"llm.domain:40103":  {want: AppCodeAPIKeyExpired.String()},
		"llm.gateway:40303": {want: AppCodeModelForbidden.String()},
		"llm.gateway:40401": {want: AppCodeModelNotFound.String()},
		"llm.gateway:50301": {want: AppCodeProviderUnavailable.String()},
	}
	for legacy, test := range tests {
		got, ok := productCatalog.CodeFromLegacy(appcatalog.MustLegacyKey(legacy))
		if !ok || got.String() != test.want {
			t.Fatalf("CodeFromLegacy(%q) = %s, %v; want %s", legacy, got, ok, test.want)
		}
	}
	// 114009 has multiple incompatible meanings in the existing Gateway and
	// must be selected from business context instead of a numeric lookup.
	if _, ok := productCatalog.CodeFromLegacy(appcatalog.MustLegacyKey("llm.gateway:114009")); ok {
		t.Fatal("ambiguous legacy code 114009 must not be guessed")
	}
}
