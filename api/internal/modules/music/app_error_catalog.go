package music

import (
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

// AppCodeTaskNotDeletable identifies a delete request blocked by the task's active state.
var AppCodeTaskNotDeletable = apperror.MustCode("music.task.not_deletable")

// CatalogDefinitions returns the application errors owned by the music domain.
func CatalogDefinitions() []appcatalog.Definition {
	return []appcatalog.Definition{
		{
			Code:       AppCodeTaskNotDeletable,
			Category:   appcatalog.CategoryConflict,
			HTTPStatus: 409,
			Retryable:  false,
			Messages: map[appcatalog.Locale]string{
				appcatalog.LocaleEnglishUS:         "This music task can be deleted after generation completes or fails.",
				appcatalog.LocaleChineseSimplified: "音乐生成完成或失败后才能删除该任务。",
			},
		},
	}
}
