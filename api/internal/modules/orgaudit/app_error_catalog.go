package orgaudit

import (
	"errors"

	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

var (
	// AppCodeFilterInvalid identifies an audit log query with a bad filter.
	AppCodeFilterInvalid = apperror.MustCode("org_audit.filter.invalid")
	// AppCodeAccessDenied identifies a caller who is not an owner or admin of
	// the organization.
	AppCodeAccessDenied = apperror.MustCode("org_audit.access.denied")
)

var (
	ErrFilterInvalid = errors.New("org audit filter is invalid")
	ErrAccessDenied  = errors.New("only organization owners and admins can read the audit log")
	ErrNotFound      = errors.New("organization not found")
)

// CatalogDefinitions returns the audit log's public application errors.
func CatalogDefinitions() []appcatalog.Definition {
	definition := func(code apperror.Code, category appcatalog.Category, status int, english, chinese string) appcatalog.Definition {
		return appcatalog.Definition{Code: code, Category: category, HTTPStatus: status, Messages: map[appcatalog.Locale]string{
			appcatalog.LocaleEnglishUS: english, appcatalog.LocaleChineseSimplified: chinese,
		}}
	}
	return []appcatalog.Definition{
		definition(AppCodeFilterInvalid, appcatalog.CategoryValidation, 400,
			"The audit log filter is invalid. Check the account IDs, action and time range.", "审计日志筛选条件无效，请检查账号、操作类型和时间范围。"),
		definition(AppCodeAccessDenied, appcatalog.CategoryAuthorization, 403,
			"Only organization owners and admins can view the audit log.", "只有组织所有者和管理员可以查看审计日志。"),
	}
}

func appError(err error, operation string) error {
	var code apperror.Code
	switch {
	case errors.Is(err, ErrFilterInvalid):
		code = AppCodeFilterInvalid
	case errors.Is(err, ErrAccessDenied):
		code = AppCodeAccessDenied
	case errors.Is(err, ErrNotFound):
		code = appcatalog.CodeResourceNotFound
	default:
		code = appcatalog.CodeInternal
	}
	return apperror.Wrap(err, code, apperror.WithOperation(operation))
}
