package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

func TestRegistrationErrorCatalogIsLocalized(t *testing.T) {
	definitions := append(appcatalog.DefaultDefinitions(), CatalogDefinitions()...)
	productCatalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, definitions...)
	require.NoError(t, err)

	tests := []struct {
		name    string
		code    apperror.Code
		status  int
		legacy  string
		english string
		chinese string
	}{
		{
			name:    "invitation unavailable",
			code:    AppCodeRegistrationInvitationUnavailable,
			status:  401,
			legacy:  "auth.registration:401002",
			english: "This invitation is invalid or no longer available. Ask an administrator for a new invitation link.",
			chinese: "该邀请已失效或无法使用，请联系管理员获取新的邀请链接。",
		},
		{
			name:    "member name conflict",
			code:    AppCodeRegistrationMemberNameConflict,
			status:  400,
			legacy:  "auth.registration:199001",
			english: "This member name is already in use in the organization. Choose another name and try again.",
			chinese: "该成员名称已在组织中使用，请更换名称后重试。",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for locale, wantMessage := range map[appcatalog.Locale]string{
				appcatalog.LocaleEnglishUS:         test.english,
				appcatalog.LocaleChineseSimplified: test.chinese,
			} {
				presentation, err := productCatalog.Present(test.code, locale, nil)
				require.NoError(t, err)
				require.Equal(t, test.status, presentation.HTTPStatus)
				require.Equal(t, wantMessage, presentation.Message)
			}
			mapped, ok := productCatalog.CodeFromLegacy(appcatalog.MustLegacyKey(test.legacy))
			require.True(t, ok)
			require.Equal(t, test.code, mapped)
		})
	}
}
