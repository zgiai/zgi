package core

import (
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

var (
	AppCodeInvalidAppCredentials = apperror.MustCode("connector.app_credentials.invalid")
	AppCodePermissionMissing     = apperror.MustCode("connector.provider.permission_missing")
	AppCodeResourceAccessDenied  = apperror.MustCode("connector.resource.access_denied")
	AppCodeApplicationNotReady   = apperror.MustCode("connector.application.not_ready")
	AppCodeRateLimited           = apperror.MustCode("connector.onboarding.rate_limited")
	AppCodeFlowExpired           = apperror.MustCode("connector.onboarding.expired")
	// AppCodeSessionGrantUnsupported keeps the legacy route's 403 and
	// connector_session_grant_unsupported code; only its message is localized.
	AppCodeSessionGrantUnsupported = apperror.MustCode("connector.confirmation.session_grant_unsupported")
)

// CatalogDefinitions owns new Connector errors. Existing Key error wire values
// remain in their legacy transport adapter; no raw provider response is public.
func CatalogDefinitions() []appcatalog.Definition {
	definitions := append([]appcatalog.Definition{
		appDefinition(AppCodeInvalidAppCredentials, appcatalog.CategoryValidation, 422, "The App ID and App Secret could not be verified. Check that they belong to the same application.", "无法验证 App ID 和 App Secret，请确认它们属于同一个应用。"),
		appDefinition(AppCodePermissionMissing, appcatalog.CategoryAuthorization, 403, "The application is missing a required permission. Grant it and verify the connection again.", "应用缺少所需权限，请授权后重新验证连接。"),
		appDefinition(AppCodeResourceAccessDenied, appcatalog.CategoryAuthorization, 403, "The application cannot access this resource. Check its sharing and access restrictions.", "应用无法访问此资源，请检查共享范围与访问限制。"),
		appDefinition(AppCodeApplicationNotReady, appcatalog.CategoryConflict, 409, "The application is not available. Check its activation, publication and authorization status.", "应用尚不可用，请检查启用、发布与授权状态。"),
		appDefinition(AppCodeRateLimited, appcatalog.CategoryRateLimit, 429, "Too many connection requests. Wait before trying again.", "连接请求过于频繁，请稍后再试。"),
		appDefinition(AppCodeFlowExpired, appcatalog.CategoryConflict, 410, "This registration has expired. Start a new registration when ready.", "本次扫码流程已过期，请重新开始。"),
		appDefinition(AppCodeSessionGrantUnsupported, appcatalog.CategoryAuthorization, 403, "This confirmation applies to one operation only and cannot be remembered for the session. Confirm it again without remembering.", "此确认仅对本次操作有效，不能在本会话中记住。请取消“记住”后重新确认。"),
	}, authCatalogDefinitions()...)
	return append(definitions, executionPresentationDefinitions()...)
}

func appDefinition(code apperror.Code, category appcatalog.Category, status int, en, zh string) appcatalog.Definition {
	return appcatalog.Definition{Code: code, Category: category, HTTPStatus: status, Messages: map[appcatalog.Locale]string{
		appcatalog.LocaleEnglishUS: en, appcatalog.LocaleChineseSimplified: zh,
	}}
}

// ApplicationError projects only declared new codes. It deliberately does not
// wrap the cause, which may contain credentials returned by a provider.
func ApplicationError(err error) (error, bool) {
	var code apperror.Code
	switch ErrorCodeOf(err) {
	case ErrorCodeInvalidAppCredentials:
		code = AppCodeInvalidAppCredentials
	case ErrorCodePermissionMissing:
		code = AppCodePermissionMissing
	case ErrorCodeResourceAccessDenied:
		code = AppCodeResourceAccessDenied
	case ErrorCodeApplicationNotReady:
		code = AppCodeApplicationNotReady
	case ErrorCodeRateLimited:
		code = AppCodeRateLimited
	case ErrorCodeFlowExpired:
		code = AppCodeFlowExpired
	case ErrorCodeSessionGrantUnsupported:
		code = AppCodeSessionGrantUnsupported
	default:
		return authApplicationError(ErrorCodeOf(err))
	}
	return apperror.New(code), true
}
