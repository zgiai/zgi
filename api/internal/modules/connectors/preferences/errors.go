package preferences

import (
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

var (
	AppCodeInvalidInput          = apperror.MustCode("connector.preference.invalid_input")
	AppCodeAccessDenied          = apperror.MustCode("connector.preference.access_denied")
	AppCodeConnectionNotFound    = apperror.MustCode("connector.preference.connection_not_found")
	AppCodeConnectionUnavailable = apperror.MustCode("connector.preference.connection_unavailable")
	AppCodeRevisionConflict      = apperror.MustCode("connector.preference.revision_conflict")
	AppCodeIdempotencyConflict   = apperror.MustCode("connector.preference.idempotency_conflict")
	AppCodeInternal              = apperror.MustCode("connector.preference.internal")
)

// CatalogDefinitions contains no public parameters: identifiers and database
// causes remain diagnostic-only. Existing Chat uses the named legacy overlay.
func CatalogDefinitions() []appcatalog.Definition {
	definitions := []appcatalog.Definition{
		preferenceErrorDefinition(AppCodeInvalidInput, appcatalog.CategoryValidation, 400, "Invalid new-chat Connector default request.", "新对话连接器默认设置请求无效。"),
		preferenceErrorDefinition(AppCodeAccessDenied, appcatalog.CategoryAuthentication, 401, "Sign in to manage your new-chat Connector defaults.", "请登录后管理你的新对话连接器默认设置。"),
		preferenceErrorDefinition(AppCodeConnectionNotFound, appcatalog.CategoryNotFound, 404, "This connection is no longer available to you.", "该连接不存在或你无权访问。"),
		preferenceErrorDefinition(AppCodeConnectionUnavailable, appcatalog.CategoryConflict, 409, "This connection cannot be used as a new-chat default. Repair it or choose another connection.", "该连接暂时不能设为新对话默认，请修复或选择其他连接。"),
		preferenceErrorDefinition(AppCodeRevisionConflict, appcatalog.CategoryConflict, 409, "Your new-chat Connector defaults changed. Review the latest selection before sending again.", "新对话连接器默认设置已变化，请核对最新选择后重新发送。"),
		preferenceErrorDefinition(AppCodeIdempotencyConflict, appcatalog.CategoryConflict, 409, "This request has already been used for a different default change. Refresh and try again.", "该请求标识已用于其他默认设置变更，请刷新后重试。"),
		preferenceErrorDefinition(AppCodeInternal, appcatalog.CategoryInternal, 500, "Unable to load or save new-chat Connector defaults. Please try again.", "无法读取或保存新对话连接器默认设置，请重试。"),
	}
	for i := range definitions {
		if definitions[i].Code == AppCodeRevisionConflict {
			definitions[i].LegacyCodes = []appcatalog.LegacyKey{appcatalog.MustLegacyKey("aichat.connector_defaults:199001")}
		}
		if definitions[i].Code == AppCodeInternal {
			definitions[i].LegacyCodes = []appcatalog.LegacyKey{appcatalog.MustLegacyKey("aichat.connector_defaults:399001")}
		}
	}
	return definitions
}

func preferenceErrorDefinition(code apperror.Code, category appcatalog.Category, status int, english, chinese string) appcatalog.Definition {
	return appcatalog.Definition{Code: code, Category: category, HTTPStatus: status, Messages: map[appcatalog.Locale]string{
		appcatalog.LocaleEnglishUS: english, appcatalog.LocaleChineseSimplified: chinese,
	}}
}

func inputError() error {
	return apperror.New(AppCodeInvalidInput, apperror.WithOperation("connector.preference.validate"))
}
func conflictError() error {
	return apperror.New(AppCodeRevisionConflict, apperror.WithOperation("connector.preference.compare_revision"))
}
func storageError(err error) error {
	return apperror.Wrap(err, AppCodeInternal, apperror.WithOperation("connector.preference.store"))
}
