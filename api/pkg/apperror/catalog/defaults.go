package catalog

import "github.com/zgiai/zgi/api/pkg/apperror"

// Shared product codes. Domain-specific packages add their own definitions to
// the same startup Catalog rather than creating a second registry.
var (
	CodeRequestInvalid         = apperror.MustCode("request.invalid")
	CodeAuthenticationRequired = apperror.MustCode("auth.unauthenticated")
	CodePermissionDenied       = apperror.MustCode("auth.forbidden")
	CodeAPIKeyInvalid          = apperror.MustCode("auth.api_key.invalid")
	CodeAPIKeyExpired          = apperror.MustCode("auth.api_key.expired")
	CodeAPIKeyInactive         = apperror.MustCode("auth.api_key.inactive")
	CodeResourceNotFound       = apperror.MustCode("resource.not_found")
	CodeResourceConflict       = apperror.MustCode("resource.conflict")
	CodeQuotaExceeded          = apperror.MustCode("quota.exceeded")
	CodeBalanceInsufficient    = apperror.MustCode("billing.balance.insufficient")
	CodeRateLimitExceeded      = apperror.MustCode("rate_limit.exceeded")
	CodeLLMModelNotFound       = apperror.MustCode("llm.model.not_found")
	CodeLLMModelForbidden      = apperror.MustCode("llm.model.forbidden")
	CodeLLMProviderRateLimited = apperror.MustCode("llm.provider.rate_limited")
	CodeLLMProviderTimeout     = apperror.MustCode("llm.provider.timeout")
	CodeLLMProviderUnavailable = apperror.MustCode("llm.provider.unavailable")
	CodeLLMInvocationFailed    = apperror.MustCode("llm.invocation.failed")
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
			"请求内容不正确，请检查后重试。",
			"llm.gateway:40001"),
		definition(CodeAuthenticationRequired, CategoryAuthentication, 401, false,
			"Sign in to continue.",
			"请先登录，再继续操作。"),
		definition(CodePermissionDenied, CategoryAuthorization, 403, false,
			"You do not have permission to perform this action.",
			"你没有执行此操作的权限，请联系管理员。"),
		definition(CodeAPIKeyInvalid, CategoryAuthentication, 401, false,
			"The API key is invalid. Check the key and try again.",
			"API 密钥无效，请检查密钥后重试。",
			"llm.gateway:40101"),
		definition(CodeAPIKeyExpired, CategoryAuthentication, 401, false,
			"The API key has expired. Create or select an active key.",
			"API 密钥已过期，请创建或选择有效密钥。",
			"llm.gateway:40102"),
		definition(CodeAPIKeyInactive, CategoryAuthentication, 401, false,
			"The API key is disabled. Enable it or use another key.",
			"API 密钥已停用，请启用后重试或更换密钥。",
			"llm.gateway:40103"),
		definition(CodeResourceNotFound, CategoryNotFound, 404, false,
			"The requested resource was not found or is no longer available.",
			"未找到请求的内容，它可能已被删除。"),
		definition(CodeResourceConflict, CategoryConflict, 409, false,
			"The resource has changed. Refresh and try again.",
			"内容已发生变化，请刷新后重试。"),
		definition(CodeQuotaExceeded, CategoryQuota, 429, false,
			"The current quota has been reached. Review your quota or try again later.",
			"当前额度已用完，请检查额度或稍后重试。"),
		definition(CodeBalanceInsufficient, CategoryQuota, 402, false,
			"The account balance is insufficient. Add funds or contact an administrator.",
			"账户余额不足，请充值或联系管理员。"),
		definition(CodeRateLimitExceeded, CategoryRateLimit, 429, true,
			"Too many requests were sent. Wait a moment and try again.",
			"请求过于频繁，请稍后重试。"),
		definition(CodeLLMModelNotFound, CategoryNotFound, 404, false,
			"The selected model was not found. Choose an available model.",
			"未找到所选大模型，请选择可用模型。",
			"llm.gateway:40401"),
		definition(CodeLLMModelForbidden, CategoryAuthorization, 403, false,
			"You do not have access to the selected model. Choose another model or contact an administrator.",
			"你没有使用所选大模型的权限，请更换模型或联系管理员。",
			"llm.gateway:40303"),
		definition(CodeLLMProviderRateLimited, CategoryRateLimit, 429, true,
			"The model service is busy. Wait a moment and try again.",
			"大模型服务当前繁忙，请稍后重试。"),
		definition(CodeLLMProviderTimeout, CategoryUpstream, 504, true,
			"The model service took too long to respond. Try again or choose another model.",
			"大模型服务响应超时，请重试或选择其他模型。"),
		definition(CodeLLMProviderUnavailable, CategoryUpstream, 503, true,
			"The model service is temporarily unavailable. Try again later or choose another model.",
			"大模型服务暂时不可用，请稍后重试或选择其他模型。",
			"llm.gateway:50301"),
		definition(CodeLLMInvocationFailed, CategoryUpstream, 502, true,
			"The model request could not be completed. Try again or choose another model.",
			"大模型调用未完成，请重试或选择其他模型。"),
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
