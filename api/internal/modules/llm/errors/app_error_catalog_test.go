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
		wantCode   string
		wantStatus int
	}{
		"llm.gateway:40001": {wantCode: AppCodeRequestInvalid.String(), wantStatus: 400},
		"llm.gateway:40101": {wantCode: AppCodeAPIKeyInvalid.String(), wantStatus: 401},
		"llm.gateway:40102": {wantCode: AppCodeAPIKeyExpired.String(), wantStatus: 401},
		"llm.gateway:40103": {wantCode: AppCodeAPIKeyInactive.String(), wantStatus: 401},
		"llm.domain:40102":  {wantCode: AppCodeAPIKeyInactive.String(), wantStatus: 401},
		"llm.domain:40103":  {wantCode: AppCodeAPIKeyExpired.String(), wantStatus: 401},
		"llm.domain:40301":  {wantCode: AppCodeBalanceInsufficient.String(), wantStatus: 403},
		"llm.gateway:40303": {wantCode: AppCodeModelForbidden.String(), wantStatus: 403},
		"llm.gateway:40401": {wantCode: AppCodeModelNotFound.String(), wantStatus: 404},
		"llm.gateway:50301": {wantCode: AppCodeProviderUnavailable.String(), wantStatus: 503},
	}
	for legacy, test := range tests {
		got, ok := productCatalog.CodeFromLegacy(appcatalog.MustLegacyKey(legacy))
		if !ok || got.String() != test.wantCode {
			t.Fatalf("CodeFromLegacy(%q) = %s, %v; want %s", legacy, got, ok, test.wantCode)
		}
		presentation, presentErr := productCatalog.Present(got, appcatalog.LocaleEnglishUS, nil)
		if presentErr != nil || presentation.HTTPStatus != test.wantStatus {
			t.Fatalf("Present(CodeFromLegacy(%q)) = %#v, %v; want HTTP %d", legacy, presentation, presentErr, test.wantStatus)
		}
	}
	// 114009 has multiple incompatible meanings in the existing Gateway and
	// must be selected from business context instead of a numeric lookup.
	if _, ok := productCatalog.CodeFromLegacy(appcatalog.MustLegacyKey("llm.gateway:114009")); ok {
		t.Fatal("ambiguous legacy code 114009 must not be guessed")
	}
}
