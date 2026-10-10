package service

import (
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

// Video-owned application error identities, composed once during bootstrap.
var (
	AppCodeVideoRequestInvalid     = apperror.MustCode("video.request.invalid")
	AppCodeVideoTaskNotFound       = apperror.MustCode("video.task.not_found")
	AppCodeVideoTaskFailed         = apperror.MustCode("video.task.failed")
	AppCodeVideoUpstreamTimeout    = apperror.MustCode("video.upstream.timeout")
	AppCodeVideoRuntimeUnavailable = apperror.MustCode("video.runtime.unavailable")
	AppCodeVideoAssetSaveFailed    = apperror.MustCode("video.asset.save_failed")
)

// CatalogDefinitions returns the video-owned definitions for the process catalog.
func CatalogDefinitions() []appcatalog.Definition {
	return []appcatalog.Definition{
		videoDefinition(AppCodeVideoRequestInvalid, appcatalog.CategoryValidation, 400, false, "The video request is invalid. Check your input and try again.", "视频请求内容不正确，请检查后重试。", "video.gateway:PROMPT_REQUIRED", "video.gateway:PROMPT_TOO_LONG", "video.gateway:MODEL_NOT_AVAILABLE", "video.gateway:INVALID_CURSOR", "video.gateway:SEARCH_TOO_LONG", "video.gateway:BILLING_CONTEXT_REQUIRED"),
		videoDefinition(AppCodeVideoTaskNotFound, appcatalog.CategoryNotFound, 404, false, "The video generation task was not found or is no longer available.", "未找到视频生成任务，它可能已不存在。", "video.gateway:VIDEO_TASK_NOT_FOUND"),
		videoDefinition(AppCodeVideoTaskFailed, appcatalog.CategoryUpstream, 502, true, "The video generation task failed. Try again or choose another model.", "视频生成任务失败，请重试或更换模型。", "video.gateway:UPSTREAM_FAILED", "video.gateway:VIDEO_RUNTIME_FAILED"),
		videoDefinition(AppCodeVideoUpstreamTimeout, appcatalog.CategoryUpstream, 504, true, "The video model service timed out. Try again or choose another model.", "视频模型服务响应超时，请重试或更换模型。"),
		videoDefinition(AppCodeVideoRuntimeUnavailable, appcatalog.CategoryUpstream, 503, true, "The video runtime is temporarily unavailable. Try again later.", "视频运行时暂时不可用，请稍后重试。", "video.gateway:VIDEO_RUNTIME_UNAVAILABLE"),
		videoDefinition(AppCodeVideoAssetSaveFailed, appcatalog.CategoryInternal, 502, true, "The generated video could not be saved. Try again.", "生成视频保存失败，请重试。"),
	}
}

func videoDefinition(code apperror.Code, category appcatalog.Category, httpStatus int, retryable bool, english, chinese string, legacy ...string) appcatalog.Definition {
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
