package fxapp

import (
	"testing"

	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

func TestProvideApplicationErrorCatalogComposesDomainDefinitions(t *testing.T) {
	t.Parallel()

	productCatalog, err := provideApplicationErrorCatalog()
	if err != nil {
		t.Fatalf("provideApplicationErrorCatalog() error = %v", err)
	}
	presentation, err := productCatalog.Present(
		llmerrors.AppCodeProviderTimeout,
		appcatalog.LocaleChineseSimplified,
		nil,
	)
	if err != nil {
		t.Fatalf("Present(LLM code) error = %v", err)
	}
	if presentation.HTTPStatus != 504 || presentation.Message == "" {
		t.Fatalf("LLM presentation = %#v", presentation)
	}

	sharedCatalog, err := appcatalog.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := sharedCatalog.Definition(llmerrors.AppCodeProviderTimeout); ok {
		t.Fatal("shared catalog unexpectedly owns an LLM definition")
	}
}
