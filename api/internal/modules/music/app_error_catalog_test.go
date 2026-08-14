package music

import (
	"testing"

	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

func TestCatalogDefinitionsLocalizeTaskNotDeletable(t *testing.T) {
	t.Parallel()

	definitions := append(appcatalog.DefaultDefinitions(), CatalogDefinitions()...)
	productCatalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, definitions...)
	if err != nil {
		t.Fatalf("compose catalog: %v", err)
	}
	tests := map[appcatalog.Locale]string{
		appcatalog.LocaleEnglishUS:         "This music task can be deleted after generation completes or fails.",
		appcatalog.LocaleChineseSimplified: "音乐生成完成或失败后才能删除该任务。",
	}
	for locale, wantMessage := range tests {
		presentation, presentErr := productCatalog.Present(AppCodeTaskNotDeletable, locale, nil)
		if presentErr != nil {
			t.Fatalf("Present(%s): %v", locale, presentErr)
		}
		if presentation.Code != AppCodeTaskNotDeletable || presentation.HTTPStatus != 409 || presentation.Retryable {
			t.Fatalf("Present(%s) = %#v", locale, presentation)
		}
		if presentation.Message != wantMessage {
			t.Fatalf("Present(%s) message = %q, want %q", locale, presentation.Message, wantMessage)
		}
	}
}
