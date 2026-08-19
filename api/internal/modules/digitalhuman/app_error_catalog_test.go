package digitalhuman

import (
	"testing"

	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

func TestCatalogDefinitionsLocalizeRevisionConflict(t *testing.T) {
	t.Parallel()
	definitions := append(appcatalog.DefaultDefinitions(), CatalogDefinitions()...)
	catalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, definitions...)
	if err != nil {
		t.Fatal(err)
	}

	tests := map[appcatalog.Locale]string{
		appcatalog.LocaleEnglishUS:         "The DigitalHuman has changed. Refresh and try again.",
		appcatalog.LocaleChineseSimplified: "数字人配置已发生变化，请刷新后重试。",
	}
	for locale, want := range tests {
		presentation, presentErr := catalog.Present(AppCodeRevisionConflict, locale, nil)
		if presentErr != nil {
			t.Fatalf("Present(%s): %v", locale, presentErr)
		}
		if presentation.HTTPStatus != 409 || presentation.Message != want {
			t.Fatalf("Present(%s) = %#v, want message %q", locale, presentation, want)
		}
	}
}
