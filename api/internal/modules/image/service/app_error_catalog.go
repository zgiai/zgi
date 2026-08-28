package service

import (
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

// CatalogDefinitions returns the application errors owned by the image runtime domain.
func CatalogDefinitions() []appcatalog.Definition {
	return []appcatalog.Definition{
		imageDefinition(AppCodeTaskNotFound, appcatalog.CategoryNotFound, 404, false,
			"The image generation task was not found or is no longer available.",
			"未找到图片生成任务，它可能已不存在。"),
		imageDefinition(AppCodeTaskConflict, appcatalog.CategoryConflict, 409, false,
			"Too many image generation tasks are running. Wait for one to finish and try again.",
			"当前图片生成任务过多，请等待已有任务结束后重试。"),
		imageDefinition(AppCodeSearchTooLong, appcatalog.CategoryValidation, 400, false,
			"The image task search term is too long. Shorten it and try again.",
			"图片任务搜索内容过长，请缩短后重试。"),
		imageDefinition(AppCodeInvalidCursor, appcatalog.CategoryValidation, 400, false,
			"The image task cursor is invalid. Refresh and try again.",
			"图片任务分页游标无效，请刷新后重试。"),
	}
}

func imageDefinition(code apperror.Code, category appcatalog.Category, httpStatus int, retryable bool, english, chinese string) appcatalog.Definition {
	return appcatalog.Definition{
		Code:       code,
		Category:   category,
		HTTPStatus: httpStatus,
		Retryable:  retryable,
		Messages: map[appcatalog.Locale]string{
			appcatalog.LocaleEnglishUS:         english,
			appcatalog.LocaleChineseSimplified: chinese,
		},
	}
}
