package core

import (
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	"github.com/zgiai/zgi/api/pkg/apperror/transport"
	"strings"
	"testing"
)

func TestExecutionPresentationCatalog(t *testing.T) {
	definitions := append(appcatalog.DefaultDefinitions(), CatalogDefinitions()...)
	catalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, definitions...)
	if err != nil {
		t.Fatal(err)
	}
	projector, err := transport.NewProjector(catalog)
	if err != nil {
		t.Fatal(err)
	}
	for _, locale := range []appcatalog.Locale{appcatalog.LocaleEnglishUS, appcatalog.LocaleChineseSimplified} {
		for _, disposition := range []string{"not_submitted", "in_progress", "unknown", ""} {
			result := projector.Project(ExecutionPresentationError(disposition, ErrorCodeConflict), locale)
			if result.Resolution != transport.ResolutionMatched || result.Presentation.Message == "" || result.Presentation.Locale != locale {
				t.Fatalf("catalog missing: %+v", result)
			}
			if disposition != "not_submitted" && (strings.Contains(result.Presentation.Message, "未提交") || strings.Contains(result.Presentation.Message, "not submitted")) {
				t.Fatal("conflict alone implied no submission")
			}
		}
	}
}
