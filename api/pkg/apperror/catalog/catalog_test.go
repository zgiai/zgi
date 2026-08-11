package catalog_test

import (
	"errors"
	"math"
	"sync"
	"testing"

	"github.com/zgiai/zgi/api/pkg/apperror"
	"github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

func TestDefaultCatalogIsCompleteAndFriendly(t *testing.T) {
	t.Parallel()

	productCatalog, err := catalog.NewDefault()
	if err != nil {
		t.Fatalf("NewDefault() error = %v", err)
	}
	definitions := catalog.DefaultDefinitions()
	if len(definitions) < 7 {
		t.Fatalf("default definition count = %d, want baseline product coverage", len(definitions))
	}
	for _, definition := range definitions {
		for _, locale := range catalog.SupportedLocales() {
			presentation, presentErr := productCatalog.Present(definition.Code, locale, nil)
			if presentErr != nil {
				t.Fatalf("Present(%s, %s) error = %v", definition.Code, locale, presentErr)
			}
			if presentation.Message == "" || presentation.Code != definition.Code || presentation.Locale != locale {
				t.Fatalf("Present(%s, %s) = %#v", definition.Code, locale, presentation)
			}
			if presentation.HTTPStatus < 400 || presentation.Category.String() == "unknown" {
				t.Fatalf("Present(%s) has invalid metadata: %#v", definition.Code, presentation)
			}
		}
	}
}

func TestPresentUsesTypedPublicParametersAndIgnoresDiagnostics(t *testing.T) {
	t.Parallel()

	code := apperror.MustCode("rate_limit.retry_after")
	fallbackCode := apperror.MustCode("system.fallback")
	productCatalog, err := catalog.New(catalog.LocaleEnglishUS, fallbackCode,
		testDefinition(fallbackCode, "Try again later.", "请稍后重试。"),
		catalog.Definition{
			Code:       code,
			Category:   catalog.CategoryRateLimit,
			HTTPStatus: 429,
			Retryable:  true,
			Messages: map[catalog.Locale]string{
				catalog.LocaleEnglishUS:         "Try again in {seconds} seconds.",
				catalog.LocaleChineseSimplified: "请在 {seconds} 秒后重试。",
			},
			Parameters: []catalog.Parameter{{Name: "seconds", Type: catalog.ParamUnsigned}},
		},
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	presentation, err := productCatalog.Present(code, catalog.LocaleChineseSimplified, map[string]any{
		"seconds": uint64(30),
		"secret":  "must-not-render",
	})
	if err != nil {
		t.Fatalf("Present() error = %v", err)
	}
	if presentation.Message != "请在 30 秒后重试。" {
		t.Fatalf("message = %q", presentation.Message)
	}
	if _, err := productCatalog.Present(code, catalog.LocaleEnglishUS, nil); !errors.Is(err, catalog.ErrMessageUnavailable) {
		t.Fatalf("missing parameter error = %v", err)
	}
	if _, err := productCatalog.Present(code, catalog.LocaleEnglishUS, map[string]any{"seconds": "30"}); !errors.Is(err, catalog.ErrMessageUnavailable) {
		t.Fatalf("wrong parameter type error = %v", err)
	}
}

func TestPublicStringAndFloatParametersAreBounded(t *testing.T) {
	t.Parallel()

	fallbackCode := apperror.MustCode("system.fallback")
	code := apperror.MustCode("request.public_params")
	definition := catalog.Definition{
		Code:       code,
		Category:   catalog.CategoryValidation,
		HTTPStatus: 400,
		Messages: map[catalog.Locale]string{
			catalog.LocaleEnglishUS:         "Field {field}; ratio {ratio}",
			catalog.LocaleChineseSimplified: "字段 {field}；比例 {ratio}",
		},
		Parameters: []catalog.Parameter{
			{Name: "field", Type: catalog.ParamString},
			{Name: "ratio", Type: catalog.ParamFloat},
		},
	}
	productCatalog, err := catalog.New(catalog.LocaleEnglishUS, fallbackCode,
		testDefinition(fallbackCode, "Fallback", "兜底消息"), definition)
	if err != nil {
		t.Fatal(err)
	}
	for name, params := range map[string]map[string]any{
		"oversized string":  {"field": string(make([]byte, 257)), "ratio": float64(1)},
		"control character": {"field": "bad\nfield", "ratio": float64(1)},
		"non-finite float":  {"field": "name", "ratio": math.Inf(1)},
	} {
		if _, err := productCatalog.Present(code, catalog.LocaleEnglishUS, params); !errors.Is(err, catalog.ErrMessageUnavailable) {
			t.Fatalf("%s error = %v", name, err)
		}
	}
}

func TestUnknownCodeRequiresExplicitSafeFallback(t *testing.T) {
	t.Parallel()

	productCatalog, err := catalog.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	unknown := apperror.MustCode("extension.unknown")
	if _, err := productCatalog.Present(unknown, catalog.LocaleChineseSimplified, nil); !errors.Is(err, catalog.ErrCodeNotCataloged) {
		t.Fatalf("Present(unknown) error = %v", err)
	}
	fallback := productCatalog.Fallback(catalog.LocaleChineseSimplified)
	if fallback.Code != catalog.CodeInternal || fallback.Message == "" || fallback.HTTPStatus != 500 {
		t.Fatalf("Fallback() = %#v", fallback)
	}
}

func TestSharedCatalogDoesNotOwnDomainLegacyMappings(t *testing.T) {
	t.Parallel()

	productCatalog, err := catalog.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	for _, legacy := range []string{"llm.gateway:40001", "llm.gateway:40101", "llm.gateway:50301"} {
		if _, ok := productCatalog.CodeFromLegacy(catalog.MustLegacyKey(legacy)); ok {
			t.Fatalf("shared catalog unexpectedly owns domain alias %q", legacy)
		}
	}
}

func TestCatalogCopiesCallerOwnedDefinitions(t *testing.T) {
	t.Parallel()

	fallbackCode := apperror.MustCode("system.fallback")
	definition := testDefinition(fallbackCode, "Original", "原始消息")
	productCatalog, err := catalog.New(catalog.LocaleEnglishUS, fallbackCode, definition)
	if err != nil {
		t.Fatal(err)
	}
	definition.Messages[catalog.LocaleEnglishUS] = "Mutated"
	copyOfDefinition, ok := productCatalog.Definition(fallbackCode)
	if !ok {
		t.Fatal("Definition() did not find fallback")
	}
	copyOfDefinition.Messages[catalog.LocaleEnglishUS] = "Mutated again"
	presentation, err := productCatalog.Present(fallbackCode, catalog.LocaleEnglishUS, nil)
	if err != nil || presentation.Message != "Original" {
		t.Fatalf("immutable presentation = %#v, %v", presentation, err)
	}
}

func TestDefinitionsAreSortedAndDefensivelyCopied(t *testing.T) {
	t.Parallel()

	productCatalog, err := catalog.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	definitions := productCatalog.Definitions()
	if len(definitions) != len(catalog.DefaultDefinitions()) {
		t.Fatalf("Definitions() count = %d", len(definitions))
	}
	for index := 1; index < len(definitions); index++ {
		if definitions[index-1].Code.String() >= definitions[index].Code.String() {
			t.Fatalf("Definitions() is not sorted at %d", index)
		}
	}
	definitions[0].Messages[catalog.LocaleEnglishUS] = "mutated"
	again := productCatalog.Definitions()
	if again[0].Messages[catalog.LocaleEnglishUS] == "mutated" {
		t.Fatal("Definitions() exposed catalog-owned message map")
	}
}

func TestCatalogRejectsIncompleteOrAmbiguousDefinitions(t *testing.T) {
	t.Parallel()

	fallbackCode := apperror.MustCode("system.fallback")
	validFallback := testDefinition(fallbackCode, "Fallback", "兜底消息")
	tests := []struct {
		name        string
		definitions []catalog.Definition
	}{
		{
			name: "duplicate code",
			definitions: []catalog.Definition{
				validFallback,
				validFallback,
			},
		},
		{
			name: "missing locale",
			definitions: []catalog.Definition{
				validFallback,
				{
					Code:       apperror.MustCode("request.incomplete"),
					Category:   catalog.CategoryValidation,
					HTTPStatus: 400,
					Messages:   map[catalog.Locale]string{catalog.LocaleEnglishUS: "Invalid"},
				},
			},
		},
		{
			name: "unsupported locale typo",
			definitions: []catalog.Definition{
				validFallback,
				{
					Code:       apperror.MustCode("request.locale_typo"),
					Category:   catalog.CategoryValidation,
					HTTPStatus: 400,
					Messages: map[catalog.Locale]string{
						catalog.LocaleEnglishUS:         "Invalid",
						catalog.LocaleChineseSimplified: "无效",
						catalog.Locale("zh-Han"):        "拼写错误",
					},
				},
			},
		},
		{
			name: "undeclared placeholder",
			definitions: []catalog.Definition{
				validFallback,
				{
					Code:       apperror.MustCode("request.placeholder"),
					Category:   catalog.CategoryValidation,
					HTTPStatus: 400,
					Messages: map[catalog.Locale]string{
						catalog.LocaleEnglishUS:         "Invalid {field}",
						catalog.LocaleChineseSimplified: "{field} 无效",
					},
				},
			},
		},
		{
			name: "duplicate legacy alias",
			definitions: []catalog.Definition{
				validFallback,
				withLegacy(testDefinition(apperror.MustCode("request.first"), "First", "第一"), "old.system:1"),
				withLegacy(testDefinition(apperror.MustCode("request.second"), "Second", "第二"), "old.system:1"),
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := catalog.New(catalog.LocaleEnglishUS, fallbackCode, test.definitions...); err == nil {
				t.Fatal("New() succeeded for invalid catalog")
			}
		})
	}
}

func TestCatalogSupportsConcurrentReads(t *testing.T) {
	t.Parallel()

	productCatalog, err := catalog.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	const readers = 64
	var waitGroup sync.WaitGroup
	waitGroup.Add(readers)
	for range readers {
		go func() {
			defer waitGroup.Done()
			for range 1_000 {
				presentation, presentErr := productCatalog.Present(catalog.CodeRateLimitExceeded, catalog.LocaleChineseSimplified, nil)
				if presentErr != nil || presentation.HTTPStatus != 429 {
					t.Errorf("concurrent Present() = %#v, %v", presentation, presentErr)
					return
				}
			}
		}()
	}
	waitGroup.Wait()
}

func BenchmarkPresentStaticMessage(b *testing.B) {
	productCatalog, err := catalog.NewDefault()
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		_, _ = productCatalog.Present(catalog.CodeRateLimitExceeded, catalog.LocaleChineseSimplified, nil)
	}
}

func testDefinition(code apperror.Code, english, chinese string) catalog.Definition {
	return catalog.Definition{
		Code:       code,
		Category:   catalog.CategoryInternal,
		HTTPStatus: 500,
		Messages: map[catalog.Locale]string{
			catalog.LocaleEnglishUS:         english,
			catalog.LocaleChineseSimplified: chinese,
		},
	}
}

func withLegacy(definition catalog.Definition, key string) catalog.Definition {
	definition.LegacyCodes = []catalog.LegacyKey{catalog.MustLegacyKey(key)}
	return definition
}
