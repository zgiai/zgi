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

func TestCatalogDefinitionsExplainUnavailableCredentialStorage(t *testing.T) {
	t.Parallel()
	definitions := append(appcatalog.DefaultDefinitions(), CatalogDefinitions()...)
	catalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, definitions...)
	if err != nil {
		t.Fatal(err)
	}

	presentation, err := catalog.Present(AppCodeCredentialStorageUnavailable, appcatalog.LocaleChineseSimplified, nil)
	if err != nil {
		t.Fatal(err)
	}
	if presentation.HTTPStatus != 503 || presentation.Message != "安全凭证存储尚未配置，请联系管理员完成配置后，再保存渠道或网页搜索凭证。" {
		t.Fatalf("presentation = %#v", presentation)
	}
}

func TestCatalogDefinitionsHideConsoleFileAccessBoundary(t *testing.T) {
	t.Parallel()
	definitions := append(appcatalog.DefaultDefinitions(), CatalogDefinitions()...)
	catalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, definitions...)
	if err != nil {
		t.Fatal(err)
	}

	presentation, err := catalog.Present(AppCodeConsoleFileNotAccessible, appcatalog.LocaleChineseSimplified, nil)
	if err != nil {
		t.Fatal(err)
	}
	if presentation.HTTPStatus != 404 || presentation.Message != "未找到该文件，或该文件无法用于当前会话。" {
		t.Fatalf("presentation = %#v", presentation)
	}
}
