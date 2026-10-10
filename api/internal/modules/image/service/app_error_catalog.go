package service

import (
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

// Additional image-owned identities introduced by the error contract. The
// existing task/validation identities live in errors.go.
var (
	AppCodeRequestInvalid  = apperror.MustCode("image.request.invalid")
	AppCodeTaskTimeout     = apperror.MustCode("image.task.timeout")
	AppCodeUpstreamFailed  = apperror.MustCode("image.upstream.failed")
	AppCodeAssetSaveFailed = apperror.MustCode("image.asset.save_failed")
	AppCodeRuntimeFailed   = apperror.MustCode("image.runtime.failed")
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
		imageDefinition(AppCodeRequestInvalid, appcatalog.CategoryValidation, 400, false,
			"The image request is invalid. Check your input and try again.",
			"图片请求内容不正确，请检查后重试。",
			"image.gateway:PROMPT_REQUIRED", "image.gateway:MODEL_NOT_AVAILABLE", "image.gateway:UNSUPPORTED_SIZE"),
		imageDefinition(AppCodeTaskTimeout, appcatalog.CategoryUpstream, 504, true,
			"The image generation task timed out. Try again.",
			"图片生成任务超时，请重试。",
			"image.gateway:IMAGE_TASK_TIMEOUT"),
		imageDefinition(AppCodeUpstreamFailed, appcatalog.CategoryUpstream, 502, true,
			"The image model service failed. Try again or choose another model.",
			"图片模型服务调用失败，请重试或更换模型。",
			"image.gateway:UPSTREAM_FAILED"),
		imageDefinition(AppCodeAssetSaveFailed, appcatalog.CategoryInternal, 502, true,
			"The generated image could not be saved. Try again.",
			"生成图片保存失败，请重试。",
			"image.gateway:IMAGE_SAVE_FAILED"),
		imageDefinition(AppCodeRuntimeFailed, appcatalog.CategoryInternal, 500, true,
			"The image service encountered a problem. Try again later.",
			"图片服务出现问题，请稍后重试。",
			"image.gateway:IMAGE_RUNTIME_FAILED"),
	}
}

func imageDefinition(code apperror.Code, category appcatalog.Category, httpStatus int, retryable bool, english, chinese string, legacy ...string) appcatalog.Definition {
	legacyCodes := make([]appcatalog.LegacyKey, 0, len(legacy))
	for _, value := range legacy {
		legacyCodes = append(legacyCodes, appcatalog.MustLegacyKey(value))
	}
	return appcatalog.Definition{
		Code:       code,
		Category:   category,
		HTTPStatus: httpStatus,
		Retryable:  retryable,
		Messages: map[appcatalog.Locale]string{
			appcatalog.LocaleEnglishUS:         english,
			appcatalog.LocaleChineseSimplified: chinese,
		},
		LegacyCodes: legacyCodes,
	}
}
