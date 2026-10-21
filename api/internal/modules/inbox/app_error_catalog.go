package inbox

import (
	"errors"

	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

var (
	// AppCodeChannelNotFound identifies a missing notification channel.
	AppCodeChannelNotFound = apperror.MustCode("inbox.channel.not_found")
	// AppCodeChannelConflict identifies a duplicate channel or one in the wrong state.
	AppCodeChannelConflict = apperror.MustCode("inbox.channel.conflict")
	// AppCodeVerificationFailed identifies a wrong or expired verification code.
	AppCodeVerificationFailed = apperror.MustCode("inbox.channel.verification_failed")
	// AppCodeRateLimited identifies a verification code requested too soon.
	AppCodeRateLimited = apperror.MustCode("inbox.channel.rate_limited")
	// AppCodeDeliveryUnavailable identifies a channel type this deployment cannot send.
	AppCodeDeliveryUnavailable = apperror.MustCode("inbox.channel.delivery_unavailable")
)

// CatalogDefinitions returns the inbox's public application errors.
func CatalogDefinitions() []appcatalog.Definition {
	definition := func(code apperror.Code, category appcatalog.Category, status int, english, chinese string) appcatalog.Definition {
		return appcatalog.Definition{Code: code, Category: category, HTTPStatus: status, Messages: map[appcatalog.Locale]string{
			appcatalog.LocaleEnglishUS: english, appcatalog.LocaleChineseSimplified: chinese,
		}}
	}
	return []appcatalog.Definition{
		definition(AppCodeChannelNotFound, appcatalog.CategoryNotFound, 404, "The notification channel was not found.", "未找到该通知渠道。"),
		definition(AppCodeChannelConflict, appcatalog.CategoryConflict, 409,
			"This channel already exists, or cannot do that in its current state.", "该通知渠道已存在，或当前状态下不能执行这个操作。"),
		definition(AppCodeVerificationFailed, appcatalog.CategoryValidation, 400,
			"The verification code is wrong or has expired. Request a new code and try again.", "验证码错误或已过期，请重新获取验证码后再试。"),
		definition(AppCodeRateLimited, appcatalog.CategoryRateLimit, 429,
			"A code was sent less than a minute ago. Wait a moment before requesting another.", "验证码发送过于频繁，请稍后再试。"),
		definition(AppCodeDeliveryUnavailable, appcatalog.CategoryConfiguration, 503,
			"This deployment cannot send this kind of notification. Ask an administrator to configure email or credential encryption.",
			"当前部署暂不支持这类通知，请联系管理员配置邮件服务或凭据加密。"),
	}
}

func channelError(err error, operation string) error {
	var code apperror.Code
	switch {
	case errors.Is(err, ErrInvalidRequest):
		code = appcatalog.CodeRequestInvalid
	case errors.Is(err, ErrChannelNotFound):
		code = AppCodeChannelNotFound
	case errors.Is(err, ErrChannelConflict):
		code = AppCodeChannelConflict
	case errors.Is(err, ErrVerificationFailed):
		code = AppCodeVerificationFailed
	case errors.Is(err, ErrRateLimited):
		code = AppCodeRateLimited
	case errors.Is(err, ErrDeliveryUnavailable):
		code = AppCodeDeliveryUnavailable
	default:
		code = appcatalog.CodeInternal
	}
	return apperror.Wrap(err, code, apperror.WithOperation(operation))
}
