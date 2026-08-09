package llmerrors

import (
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

// LLM-owned application error identities. Keeping these beside the domain
// prevents shared infrastructure from becoming the owner of model semantics.
var (
	AppCodeRequestInvalid      = apperror.MustCode("llm.request.invalid")
	AppCodeAPIKeyInvalid       = apperror.MustCode("llm.api_key.invalid")
	AppCodeAPIKeyExpired       = apperror.MustCode("llm.api_key.expired")
	AppCodeAPIKeyInactive      = apperror.MustCode("llm.api_key.inactive")
	AppCodeQuotaExceeded       = apperror.MustCode("llm.quota.exceeded")
	AppCodeBalanceInsufficient = apperror.MustCode("llm.balance.insufficient")
	AppCodeModelNotFound       = apperror.MustCode("llm.model.not_found")
	AppCodeModelForbidden      = apperror.MustCode("llm.model.forbidden")
	AppCodeProviderNotFound    = apperror.MustCode("llm.provider.not_found")
	AppCodeChannelNotFound     = apperror.MustCode("llm.channel.not_found")
	AppCodeRouteNotFound       = apperror.MustCode("llm.route.not_found")
	AppCodeProviderAuthFailed  = apperror.MustCode("llm.provider.auth_failed")
	AppCodeProviderRateLimited = apperror.MustCode("llm.provider.rate_limited")
	AppCodeProviderTimeout     = apperror.MustCode("llm.provider.timeout")
	AppCodeProviderUnavailable = apperror.MustCode("llm.provider.unavailable")
	AppCodeNoProviderAvailable = apperror.MustCode("llm.provider.none_available")
	AppCodeInvocationFailed    = apperror.MustCode("llm.invocation.failed")
)

// CatalogDefinitions returns fresh LLM-owned definitions for composition into
// the process-wide catalog during bootstrap. It does not mutate a registry.
func CatalogDefinitions() []appcatalog.Definition {
	return []appcatalog.Definition{
		llmDefinition(AppCodeRequestInvalid, appcatalog.CategoryValidation, 400, false,
			"The model request is invalid. Check the request and try again.",
			"大模型请求内容不正确，请检查后重试。",
			"llm.gateway:40001", "llm.domain:40001"),
		llmDefinition(AppCodeAPIKeyInvalid, appcatalog.CategoryAuthentication, 401, false,
			"The API key is invalid. Check the key and try again.",
			"API 密钥无效，请检查密钥后重试。",
			"llm.gateway:40101", "llm.domain:40101"),
		llmDefinition(AppCodeAPIKeyExpired, appcatalog.CategoryAuthentication, 401, false,
			"The API key has expired. Create or select an active key.",
			"API 密钥已过期，请创建或选择有效密钥。",
			"llm.gateway:40102", "llm.domain:40103"),
		llmDefinition(AppCodeAPIKeyInactive, appcatalog.CategoryAuthentication, 401, false,
			"The API key is disabled. Enable it or use another key.",
			"API 密钥已停用，请启用后重试或更换密钥。",
			"llm.gateway:40103", "llm.domain:40102"),
		llmDefinition(AppCodeQuotaExceeded, appcatalog.CategoryQuota, 429, false,
			"The API key quota has been reached. Review the quota or use another key.",
			"API 密钥额度已用完，请检查额度或更换密钥。"),
		llmDefinition(AppCodeBalanceInsufficient, appcatalog.CategoryQuota, 402, false,
			"The account balance is insufficient. Add funds or contact an administrator.",
			"账户余额不足，请充值或联系管理员。",
			"llm.domain:40301"),
		llmDefinition(AppCodeModelNotFound, appcatalog.CategoryNotFound, 404, false,
			"The selected model was not found. Choose an available model.",
			"未找到所选大模型，请选择可用模型。",
			"llm.gateway:40401", "llm.domain:40401"),
		llmDefinition(AppCodeModelForbidden, appcatalog.CategoryAuthorization, 403, false,
			"You do not have access to the selected model. Choose another model or contact an administrator.",
			"你没有使用所选大模型的权限，请更换模型或联系管理员。",
			"llm.gateway:40303", "llm.domain:40302"),
		llmDefinition(AppCodeProviderNotFound, appcatalog.CategoryNotFound, 404, false,
			"The model provider was not found. Check the model configuration.",
			"未找到大模型服务商，请检查模型配置。",
			"llm.domain:40402"),
		llmDefinition(AppCodeChannelNotFound, appcatalog.CategoryNotFound, 404, false,
			"No configured model channel was found. Contact an administrator.",
			"未找到已配置的大模型渠道，请联系管理员。",
			"llm.domain:40403"),
		llmDefinition(AppCodeRouteNotFound, appcatalog.CategoryNotFound, 404, false,
			"No route is available for the selected model. Choose another model or contact an administrator.",
			"所选大模型暂无可用路由，请更换模型或联系管理员。",
			"llm.domain:40404"),
		llmDefinition(AppCodeProviderAuthFailed, appcatalog.CategoryUpstream, 502, false,
			"The model provider rejected its credentials. Contact an administrator.",
			"大模型服务商凭据校验失败，请联系管理员。",
			"llm.domain:40501"),
		llmDefinition(AppCodeProviderRateLimited, appcatalog.CategoryRateLimit, 429, true,
			"The model service is busy. Wait a moment and try again.",
			"大模型服务当前繁忙，请稍后重试。",
			"llm.domain:40502", "llm.domain:40901"),
		llmDefinition(AppCodeProviderTimeout, appcatalog.CategoryUpstream, 504, true,
			"The model service took too long to respond. Try again or choose another model.",
			"大模型服务响应超时，请重试或选择其他模型。",
			"llm.domain:40503"),
		llmDefinition(AppCodeProviderUnavailable, appcatalog.CategoryUpstream, 503, true,
			"The model service is temporarily unavailable. Try again later or choose another model.",
			"大模型服务暂时不可用，请稍后重试或选择其他模型。",
			"llm.gateway:50301", "llm.domain:40504"),
		llmDefinition(AppCodeNoProviderAvailable, appcatalog.CategoryUpstream, 503, true,
			"No model provider is currently available. Try another model or contact an administrator.",
			"当前没有可用的大模型服务商，请更换模型或联系管理员。",
			"llm.domain:40506"),
		llmDefinition(AppCodeInvocationFailed, appcatalog.CategoryUpstream, 502, true,
			"The model request could not be completed. Try again or choose another model.",
			"大模型调用未完成，请重试或选择其他模型。",
			"llm.domain:40505"),
	}
}

func llmDefinition(code apperror.Code, category appcatalog.Category, httpStatus int, retryable bool, english, chinese string, legacy ...string) appcatalog.Definition {
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
