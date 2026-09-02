package service

import (
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

var (
	// AppCodeRegistrationInvitationUnavailable identifies a registration invite
	// that was removed, reset, expired, deactivated, or whose target is no longer
	// available.
	AppCodeRegistrationInvitationUnavailable = apperror.MustCode("auth.registration.invitation.unavailable")
	// AppCodeRegistrationMemberNameConflict identifies a chosen organization
	// member name that is already in use during invited registration.
	AppCodeRegistrationMemberNameConflict = apperror.MustCode("auth.registration.member_name.conflict")
)

// CatalogDefinitions returns public application errors owned by registration.
// HTTP statuses and legacy aliases intentionally retain the existing
// registration handler wire contract while the catalog supplies safe messages.
func CatalogDefinitions() []appcatalog.Definition {
	return []appcatalog.Definition{
		registrationDefinition(
			AppCodeRegistrationInvitationUnavailable,
			appcatalog.CategoryAuthentication,
			401,
			"This invitation is invalid or no longer available. Ask an administrator for a new invitation link.",
			"该邀请已失效或无法使用，请联系管理员获取新的邀请链接。",
			"auth.registration:401002",
		),
		registrationDefinition(
			AppCodeRegistrationMemberNameConflict,
			appcatalog.CategoryConflict,
			400,
			"This member name is already in use in the organization. Choose another name and try again.",
			"该成员名称已在组织中使用，请更换名称后重试。",
			"auth.registration:199001",
		),
	}
}

func registrationDefinition(code apperror.Code, category appcatalog.Category, status int, english, chinese, legacy string) appcatalog.Definition {
	return appcatalog.Definition{
		Code:       code,
		Category:   category,
		HTTPStatus: status,
		Retryable:  false,
		Messages: map[appcatalog.Locale]string{
			appcatalog.LocaleEnglishUS:         english,
			appcatalog.LocaleChineseSimplified: chinese,
		},
		LegacyCodes: []appcatalog.LegacyKey{appcatalog.MustLegacyKey(legacy)},
	}
}
