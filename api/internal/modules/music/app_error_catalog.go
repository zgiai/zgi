package music

import (
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

var (
	// AppCodeTaskNotDeletable identifies a delete request blocked by the task's active state.
	AppCodeTaskNotDeletable = apperror.MustCode("music.task.not_deletable")
	AppCodeTaskNotFound     = apperror.MustCode("music.task.not_found")
	AppCodeTaskConflict     = apperror.MustCode("music.task.conflict")
	AppCodeModelUnavailable = apperror.MustCode("music.model.unavailable")
	AppCodeTaskFailed       = apperror.MustCode("music.task.failed")
)

// CatalogDefinitions returns the application errors owned by the music domain.
func CatalogDefinitions() []appcatalog.Definition {
	return []appcatalog.Definition{
		musicDefinition(AppCodeTaskNotDeletable, appcatalog.CategoryConflict, 409, false,
			"This music task can be deleted after generation completes or fails.",
			"音乐生成完成或失败后才能删除该任务。"),
		musicDefinition(AppCodeTaskNotFound, appcatalog.CategoryNotFound, 404, false,
			"The music task was not found.",
			"未找到音乐任务。",
			"music.gateway:TASK_NOT_FOUND"),
		musicDefinition(AppCodeTaskConflict, appcatalog.CategoryConflict, 409, false,
			"A music task with this request ID already exists.",
			"该请求 ID 的音乐任务已存在。",
			"music.gateway:TASK_CONFLICT"),
		musicDefinition(AppCodeModelUnavailable, appcatalog.CategoryUpstream, 503, false,
			"The music model is unavailable. Choose another model.",
			"音乐模型不可用，请更换模型。",
			"music.gateway:MODEL_UNAVAILABLE"),
		musicDefinition(AppCodeTaskFailed, appcatalog.CategoryInternal, 500, true,
			"The music task could not be processed. Try again later.",
			"音乐任务处理失败，请稍后重试。",
			"music.gateway:MUSIC_TASK_FAILED"),
	}
}

func musicDefinition(code apperror.Code, category appcatalog.Category, httpStatus int, retryable bool, english, chinese string, legacy ...string) appcatalog.Definition {
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
