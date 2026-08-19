package fxapp

import (
	"testing"

	digitalhuman "github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	musicmodule "github.com/zgiai/zgi/api/internal/modules/music"
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
	musicPresentation, err := productCatalog.Present(
		musicmodule.AppCodeTaskNotDeletable,
		appcatalog.LocaleChineseSimplified,
		nil,
	)
	if err != nil {
		t.Fatalf("Present(music code) error = %v", err)
	}
	if musicPresentation.HTTPStatus != 409 || musicPresentation.Message != "音乐生成完成或失败后才能删除该任务。" {
		t.Fatalf("music presentation = %#v", musicPresentation)
	}
	digitalHumanPresentation, err := productCatalog.Present(
		digitalhuman.AppCodeRevisionConflict,
		appcatalog.LocaleChineseSimplified,
		nil,
	)
	if err != nil {
		t.Fatalf("Present(DigitalHuman code) error = %v", err)
	}
	if digitalHumanPresentation.HTTPStatus != 409 || digitalHumanPresentation.Message != "数字人配置已发生变化，请刷新后重试。" {
		t.Fatalf("DigitalHuman presentation = %#v", digitalHumanPresentation)
	}

	sharedCatalog, err := appcatalog.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := sharedCatalog.Definition(llmerrors.AppCodeProviderTimeout); ok {
		t.Fatal("shared catalog unexpectedly owns an LLM definition")
	}
}
