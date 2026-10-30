package service

import (
	"fmt"
	"sync/atomic"

	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

// applicationErrorCatalog holds the process-wide catalog composed at bootstrap.
// The chat-runtime stream payload builder is a pure function used from several
// packages, so the catalog is published here once, before the server serves
// traffic, instead of being threaded through every caller.
var applicationErrorCatalog atomic.Pointer[appcatalog.Catalog]

// SetApplicationErrorCatalog publishes the composed application error catalog to
// the chat-runtime package. Bootstrap calls it once; passing nil clears it.
func SetApplicationErrorCatalog(catalog *appcatalog.Catalog) {
	applicationErrorCatalog.Store(catalog)
}

// ApplicationErrorCatalog returns the catalog published at bootstrap, or nil.
func ApplicationErrorCatalog() *appcatalog.Catalog {
	return applicationErrorCatalog.Load()
}

// streamErrorRetryable resolves the catalog-owned retryable flag for a stable
// application error code. It reports false when the code is unknown or the
// catalog has not been published, so callers never invent a retry guidance.
func streamErrorRetryable(code string) (retryable bool, known bool) {
	catalog := applicationErrorCatalog.Load()
	if catalog == nil || code == "" {
		return false, false
	}
	parsed, err := apperror.ParseCode(code)
	if err != nil {
		return false, false
	}
	definition, ok := catalog.Definition(parsed)
	if !ok {
		return false, false
	}
	return definition.Retryable, true
}

// streamErrorCatalogMessage renders the catalog's public message for a stable
// application error code in the given locale. It reports false when the code is
// unknown or the catalog has not been published, so callers keep their own
// message.
func streamErrorCatalogMessage(code string, locale appcatalog.Locale) (string, bool) {
	catalog := applicationErrorCatalog.Load()
	if catalog == nil || code == "" {
		return "", false
	}
	parsed, err := apperror.ParseCode(code)
	if err != nil {
		return "", false
	}
	presentation, err := catalog.Present(parsed, locale, nil)
	if err != nil || presentation.Message == "" {
		return "", false
	}
	return presentation.Message, true
}

// CodeEndUserSuspended refuses a run of an end user an administrator
// suspended (ADR 0010).
var CodeEndUserSuspended = apperror.MustCode("end_user.suspended")

// IsEndUserAdmissionError reports whether err refuses an end user's run:
// spent quota or suspension.
func IsEndUserAdmissionError(err error) bool {
	return apperror.IsCode(err, appcatalog.CodeQuotaExceeded) || apperror.IsCode(err, CodeEndUserSuspended)
}

// CodeQueryTooLong refuses a turn whose input does not fit the model's
// context budget even after history and attachments are trimmed.
var CodeQueryTooLong = apperror.MustCode("aichat.query.too_long")

// LegacyQueryTooLong is the legacy invalid-parameter wire code the HTTP
// handlers keep for CodeQueryTooLong; only the message is overlaid.
var LegacyQueryTooLong = appcatalog.MustLegacyKey("aichat.query_too_long:199001")

// errQueryTooLong is the context-budget rejection. It still matches
// ErrInvalidInput so existing callers keep their legacy handling.
func errQueryTooLong() error {
	return apperror.Wrap(fmt.Errorf("%w: query exceeds model context budget", ErrInvalidInput), CodeQueryTooLong,
		apperror.WithOperation("chatruntime.context_budget"))
}

// CatalogDefinitions returns chat-runtime-owned product error definitions.
func CatalogDefinitions() []appcatalog.Definition {
	return []appcatalog.Definition{{
		Code:       CodeQueryTooLong,
		Category:   appcatalog.CategoryValidation,
		HTTPStatus: 400,
		Messages: map[appcatalog.Locale]string{
			appcatalog.LocaleEnglishUS:         "Your message is too long for this model. Shorten it, split it into several messages, or send long content as a file.",
			appcatalog.LocaleChineseSimplified: "输入内容过长，超出了当前模型可处理的范围。请精简内容、分多条发送，或将长文本作为文件上传。",
		},
		LegacyCodes: []appcatalog.LegacyKey{LegacyQueryTooLong},
	}, {
		Code:       CodeEndUserSuspended,
		Category:   appcatalog.CategoryAuthorization,
		HTTPStatus: 403,
		Messages: map[appcatalog.Locale]string{
			appcatalog.LocaleEnglishUS:         "This user is suspended and cannot start new conversations.",
			appcatalog.LocaleChineseSimplified: "该用户已被停用，无法发起新的对话。",
		},
	}}
}
