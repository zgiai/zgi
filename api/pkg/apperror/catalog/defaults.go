package catalog

import "github.com/zgiai/zgi/api/pkg/apperror"

// Shared product codes. Domain-specific packages add their own definitions to
// the same startup Catalog rather than creating a second registry.
var (
	CodeRequestInvalid         = apperror.MustCode("request.invalid")
	CodeAuthenticationRequired = apperror.MustCode("auth.unauthenticated")
	CodePermissionDenied       = apperror.MustCode("auth.forbidden")
	CodeResourceNotFound       = apperror.MustCode("resource.not_found")
	CodeResourceConflict       = apperror.MustCode("resource.conflict")
	CodeQuotaExceeded          = apperror.MustCode("quota.exceeded")
	CodeRateLimitExceeded      = apperror.MustCode("rate_limit.exceeded")
	CodeInternal               = apperror.MustCode("system.internal")
)

// NewDefault constructs the open-source baseline catalog. Applications can
// append independently owned domain definitions before building their
// injected process-wide Catalog.
func NewDefault() (*Catalog, error) {
	definitions := DefaultDefinitions()
	return New(LocaleEnglishUS, CodeInternal, definitions...)
}

// DefaultDefinitions returns defensive, mergeable source definitions. The
// list is intentionally explicit so additions receive normal code review.
func DefaultDefinitions() []Definition {
	return []Definition{
		definition(CodeRequestInvalid, CategoryValidation, 400, false,
			"The request is invalid. Check your input and try again.",
			"请求内容不正确，请检查后重试。"),
		definition(CodeAuthenticationRequired, CategoryAuthentication, 401, false,
			"Sign in to continue.",
			"请先登录，再继续操作。"),
		definition(CodePermissionDenied, CategoryAuthorization, 403, false,
			"You do not have permission to perform this action.",
			"你没有执行此操作的权限，请联系管理员。"),
		definition(CodeResourceNotFound, CategoryNotFound, 404, false,
			"The requested resource was not found or is no longer available.",
			"未找到请求的内容，它可能已被删除。"),
		definition(CodeResourceConflict, CategoryConflict, 409, false,
			"The resource has changed. Refresh and try again.",
			"内容已发生变化，请刷新后重试。"),
		definition(CodeQuotaExceeded, CategoryQuota, 429, false,
			"The current quota has been reached. Review your quota or try again later.",
			"当前额度已用完，请检查额度或稍后重试。"),
		definition(CodeRateLimitExceeded, CategoryRateLimit, 429, true,
			"Too many requests were sent. Wait a moment and try again.",
			"请求过于频繁，请稍后重试。"),
		definition(CodeInternal, CategoryInternal, 500, true,
			"The service encountered a problem. Try again later. If it continues, contact an administrator.",
			"服务暂时出现问题，请稍后重试；如持续发生，请联系管理员。"),
	}
}

func definition(code apperror.Code, category Category, httpStatus int, retryable bool, english, chinese string, legacy ...string) Definition {
	legacyCodes := make([]LegacyKey, 0, len(legacy))
	for _, value := range legacy {
		legacyCodes = append(legacyCodes, MustLegacyKey(value))
	}
	return Definition{
		Code:       code,
		Category:   category,
		HTTPStatus: httpStatus,
		Retryable:  retryable,
		Messages: map[Locale]string{
			LocaleEnglishUS:         english,
			LocaleChineseSimplified: chinese,
		},
		LegacyCodes: legacyCodes,
	}
}
