package service

import (
	"errors"
	"testing"

	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

func TestCatalogDefinitionsLocalizeImageTaskErrors(t *testing.T) {
	t.Parallel()

	definitions := append(appcatalog.DefaultDefinitions(), CatalogDefinitions()...)
	productCatalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, definitions...)
	if err != nil {
		t.Fatalf("compose catalog: %v", err)
	}

	tests := []struct {
		code        apperror.Code
		status      int
		english     string
		chinese     string
		shouldRetry bool
	}{
		{
			code:    AppCodeTaskNotFound,
			status:  404,
			english: "The image generation task was not found or is no longer available.",
			chinese: "未找到图片生成任务，它可能已不存在。",
		},
		{
			code:    AppCodeTaskConflict,
			status:  409,
			english: "Too many image generation tasks are running. Wait for one to finish and try again.",
			chinese: "当前图片生成任务过多，请等待已有任务结束后重试。",
		},
		{
			code:    AppCodeSearchTooLong,
			status:  400,
			english: "The image task search term is too long. Shorten it and try again.",
			chinese: "图片任务搜索内容过长，请缩短后重试。",
		},
		{
			code:    AppCodeInvalidCursor,
			status:  400,
			english: "The image task cursor is invalid. Refresh and try again.",
			chinese: "图片任务分页游标无效，请刷新后重试。",
		},
	}

	for _, test := range tests {
		for locale, wantMessage := range map[appcatalog.Locale]string{
			appcatalog.LocaleEnglishUS:         test.english,
			appcatalog.LocaleChineseSimplified: test.chinese,
		} {
			presentation, presentErr := productCatalog.Present(test.code, locale, nil)
			if presentErr != nil {
				t.Fatalf("Present(%s, %s): %v", test.code, locale, presentErr)
			}
			if presentation.Code != test.code || presentation.HTTPStatus != test.status || presentation.Retryable != test.shouldRetry {
				t.Fatalf("Present(%s, %s) = %#v", test.code, locale, presentation)
			}
			if presentation.Message != wantMessage {
				t.Fatalf("Present(%s, %s) message = %q, want %q", test.code, locale, presentation.Message, wantMessage)
			}
		}
	}
}

func TestImageTaskApplicationErrorsPreserveSentinelMatching(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		base error
		code apperror.Code
	}{
		{name: "task not found", err: imageTaskNotFoundError("image.task.get"), base: ErrTaskNotFound, code: AppCodeTaskNotFound},
		{name: "task conflict", err: imageTaskConflictError("image.task.create"), base: ErrTaskConflict, code: AppCodeTaskConflict},
		{name: "search too long", err: imageSearchTooLongError("image.task.list"), base: ErrSearchTooLong, code: AppCodeSearchTooLong},
		{name: "invalid cursor", err: imageInvalidCursorError("image.task.list"), base: ErrInvalidCursor, code: AppCodeInvalidCursor},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !errors.Is(test.err, test.base) {
				t.Fatalf("errors.Is(%v, %v) = false", test.err, test.base)
			}
			if got, ok := apperror.CodeOf(test.err); !ok || got != test.code {
				t.Fatalf("CodeOf(%v) = %s/%t, want %s/true", test.err, got, ok, test.code)
			}
		})
	}
}
