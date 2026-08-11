package service

import (
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

var AppCodeContextCompactionUnavailable = apperror.MustCode("aichat.context.compaction_unavailable")

const contextCompactionUnavailablePublicMessage = "We could not safely prepare this conversation's history. All messages are still saved. Try again later to continue from here."

// CatalogDefinitions returns chat-runtime-owned product error definitions.
func CatalogDefinitions() []appcatalog.Definition {
	return []appcatalog.Definition{
		{
			Code:       AppCodeContextCompactionUnavailable,
			Category:   appcatalog.CategoryUpstream,
			HTTPStatus: 503,
			Retryable:  true,
			Messages: map[appcatalog.Locale]string{
				appcatalog.LocaleEnglishUS:         contextCompactionUnavailablePublicMessage,
				appcatalog.LocaleChineseSimplified: "系统暂时无法安全整理这段对话的历史内容。你的全部对话记录均已保留。请稍后重试，恢复后即可从这里继续。",
			},
		},
	}
}

func newContextCompactionUnavailableError(cause error) error {
	if cause == nil {
		return apperror.New(AppCodeContextCompactionUnavailable, apperror.WithOperation("chatruntime.context_compaction"))
	}
	return apperror.Wrap(cause, AppCodeContextCompactionUnavailable, apperror.WithOperation("chatruntime.context_compaction"))
}
