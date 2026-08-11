package transport_test

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/zgiai/zgi/api/pkg/apperror"
	"github.com/zgiai/zgi/api/pkg/apperror/catalog"
	"github.com/zgiai/zgi/api/pkg/apperror/transport"
)

var (
	codeProviderTimeout = apperror.MustCode("llm.provider.timeout")
	codeInvalidField    = apperror.MustCode("request.field.invalid")
)

func TestProjectKnownErrorPreservesPublicMetadataAndCause(t *testing.T) {
	t.Parallel()

	projector := newProjector(t)
	cause := errors.New("provider response contained a private credential")
	err := fmt.Errorf("chat failed: %w", apperror.Wrap(cause, codeProviderTimeout, apperror.WithOperation("gateway.chat")))

	result := projector.Project(err, catalog.LocaleChineseSimplified)
	if result.Resolution != transport.ResolutionMatched {
		t.Fatalf("resolution = %s", result.Resolution)
	}
	if result.Presentation.Code != codeProviderTimeout || result.Presentation.HTTPStatus != 504 || !result.Presentation.Retryable {
		t.Fatalf("presentation = %#v", result.Presentation)
	}
	if result.Presentation.Message != "大模型服务响应超时，请重试。" {
		t.Fatalf("message = %q", result.Presentation.Message)
	}
	if strings.Contains(result.Presentation.Message, "credential") {
		t.Fatal("private cause leaked into public message")
	}
	if !errors.Is(err, cause) {
		t.Fatal("projection changed the caller-owned error chain")
	}
}

func TestProjectUnknownAndUnsafeMessagesUseFallback(t *testing.T) {
	t.Parallel()

	projector := newProjector(t)
	tests := []struct {
		name       string
		err        error
		resolution transport.Resolution
	}{
		{name: "ordinary error", err: errors.New("database password leaked"), resolution: transport.ResolutionUnknownError},
		{name: "unknown app code", err: apperror.New(apperror.MustCode("extension.unregistered")), resolution: transport.ResolutionMessageUnavailable},
		{name: "missing public parameter", err: apperror.New(codeInvalidField), resolution: transport.ResolutionMessageUnavailable},
		{name: "unsafe public parameter", err: apperror.New(codeInvalidField, apperror.WithParams(apperror.StringParam("field", "bad\nfield"))), resolution: transport.ResolutionMessageUnavailable},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := projector.Project(test.err, catalog.LocaleChineseSimplified)
			if result.Resolution != test.resolution {
				t.Fatalf("resolution = %s, want %s", result.Resolution, test.resolution)
			}
			if result.Presentation.Code != catalog.CodeInternal || result.Presentation.HTTPStatus != 500 {
				t.Fatalf("fallback = %#v", result.Presentation)
			}
			if strings.Contains(result.Presentation.Message, "password") || strings.Contains(result.Presentation.Message, "bad") {
				t.Fatalf("unsafe detail leaked: %q", result.Presentation.Message)
			}
		})
	}
}

func TestProjectUnsupportedLocaleKeepsMatchedError(t *testing.T) {
	t.Parallel()

	result := newProjector(t).Project(apperror.New(codeProviderTimeout), catalog.Locale("fr-FR"))
	if result.Resolution != transport.ResolutionMatched || result.Presentation.Code != codeProviderTimeout {
		t.Fatalf("result = %#v", result)
	}
	if result.Presentation.Locale != catalog.LocaleEnglishUS || result.Presentation.Message != "The model service timed out. Try again." {
		t.Fatalf("fallback language = %#v", result.Presentation)
	}
}

func TestProjectLegacyMessageRequiresExactNamespaceMapping(t *testing.T) {
	t.Parallel()

	projector := newProjector(t)
	err := apperror.New(codeProviderTimeout)

	matched := projector.ProjectLegacyMessage(err, catalog.LocaleChineseSimplified, catalog.MustLegacyKey("llm.gateway:40503"))
	if matched.Resolution != transport.ResolutionMatched || matched.AppCode != codeProviderTimeout || matched.Message != "大模型服务响应超时，请重试。" {
		t.Fatalf("matched legacy projection = %#v", matched)
	}

	for _, key := range []string{"llm.domain:40503", "llm.gateway:40502", "other.gateway:40503"} {
		mismatched := projector.ProjectLegacyMessage(err, catalog.LocaleChineseSimplified, catalog.MustLegacyKey(key))
		if mismatched.Resolution != transport.ResolutionLegacyMismatch || mismatched.AppCode != catalog.CodeInternal {
			t.Fatalf("mismatched %s = %#v", key, mismatched)
		}
	}
}

func TestProjectorIsSafeForConcurrentReads(t *testing.T) {
	t.Parallel()

	projector := newProjector(t)
	err := apperror.New(codeProviderTimeout)
	const readers = 64
	var waitGroup sync.WaitGroup
	waitGroup.Add(readers)
	for range readers {
		go func() {
			defer waitGroup.Done()
			for range 1_000 {
				result := projector.Project(err, catalog.LocaleEnglishUS)
				if result.Resolution != transport.ResolutionMatched || result.Presentation.Code != codeProviderTimeout {
					t.Errorf("inconsistent result = %#v", result)
					return
				}
			}
		}()
	}
	waitGroup.Wait()
}

func TestNewProjectorRejectsNilCatalog(t *testing.T) {
	t.Parallel()

	if _, err := transport.NewProjector(nil); !errors.Is(err, transport.ErrCatalogRequired) {
		t.Fatalf("NewProjector(nil) error = %v", err)
	}
}

func BenchmarkProjectKnownStaticError(b *testing.B) {
	projector := newBenchmarkProjector(b)
	err := apperror.New(codeProviderTimeout)
	b.ReportAllocs()
	for b.Loop() {
		_ = projector.Project(err, catalog.LocaleEnglishUS)
	}
}

func newProjector(t *testing.T) *transport.Projector {
	t.Helper()
	projector, err := transport.NewProjector(newCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	return projector
}

func newCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	productCatalog, err := catalog.New(catalog.LocaleEnglishUS, catalog.CodeInternal, testDefinitions()...)
	if err != nil {
		t.Fatal(err)
	}
	return productCatalog
}

func newBenchmarkProjector(b *testing.B) *transport.Projector {
	b.Helper()
	productCatalog, err := catalog.New(catalog.LocaleEnglishUS, catalog.CodeInternal, testDefinitions()...)
	if err != nil {
		b.Fatal(err)
	}
	projector, err := transport.NewProjector(productCatalog)
	if err != nil {
		b.Fatal(err)
	}
	return projector
}

func testDefinitions() []catalog.Definition {
	return append(catalog.DefaultDefinitions(),
		catalog.Definition{
			Code:       codeProviderTimeout,
			Category:   catalog.CategoryUpstream,
			HTTPStatus: 504,
			Retryable:  true,
			Messages: map[catalog.Locale]string{
				catalog.LocaleEnglishUS:         "The model service timed out. Try again.",
				catalog.LocaleChineseSimplified: "大模型服务响应超时，请重试。",
			},
			LegacyCodes: []catalog.LegacyKey{catalog.MustLegacyKey("llm.gateway:40503")},
		},
		catalog.Definition{
			Code:       codeInvalidField,
			Category:   catalog.CategoryValidation,
			HTTPStatus: 400,
			Messages: map[catalog.Locale]string{
				catalog.LocaleEnglishUS:         "Field {field} is invalid.",
				catalog.LocaleChineseSimplified: "字段 {field} 不正确。",
			},
			Parameters: []catalog.Parameter{{Name: "field", Type: catalog.ParamString}},
		},
	)
}
