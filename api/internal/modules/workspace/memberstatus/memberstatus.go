// Package memberstatus answers one authorization question shared by every
// path that accepts a workspace or organization from a request, a stored
// credential, or a background task: has an organization administrator
// disabled this account's membership?
//
// A disabled membership is a members row whose status is inactive. A missing
// members row is not disabled: legacy data may grant access through workspace
// membership alone, and callers keep deciding that access themselves.
package memberstatus

import (
	"context"
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	"gorm.io/gorm"
)

var (
	// AppCodeMemberDisabled identifies an account whose membership in the
	// target organization was disabled by an organization administrator.
	AppCodeMemberDisabled = apperror.MustCode("organization.member.disabled")

	// LegacyMemberDisabled is the legacy response code that carries
	// AppCodeMemberDisabled on console HTTP responses.
	LegacyMemberDisabled = appcatalog.MustLegacyKey("workspace.organization_member:403008")
)

// CatalogDefinitions returns the public application errors owned by
// organization member status checks.
func CatalogDefinitions() []appcatalog.Definition {
	return []appcatalog.Definition{{
		Code:       AppCodeMemberDisabled,
		Category:   appcatalog.CategoryAuthorization,
		HTTPStatus: 403,
		Retryable:  false,
		Messages: map[appcatalog.Locale]string{
			appcatalog.LocaleEnglishUS:         "Your access to this organization has been disabled. Contact an organization administrator.",
			appcatalog.LocaleChineseSimplified: "你在该组织的访问权限已被停用，请联系组织管理员。",
		},
		LegacyCodes: []appcatalog.LegacyKey{LegacyMemberDisabled},
	}}
}

// NewMemberDisabledError returns the cataloged error for a disabled
// membership. operation is a stable diagnostic name for the caller.
func NewMemberDisabledError(operation string) error {
	return apperror.New(AppCodeMemberDisabled, apperror.WithOperation(operation))
}

// inactiveStatus is a package constant, so it is safe to inline into SQL.
var inactiveStatus = string(model.OrganizationMemberStatusInactive)

// OrganizationDisabled reports whether accountID has an inactive members row
// in organizationID.
func OrganizationDisabled(ctx context.Context, db *gorm.DB, organizationID, accountID string) (bool, error) {
	organizationID, accountID = strings.TrimSpace(organizationID), strings.TrimSpace(accountID)
	if db == nil || organizationID == "" || accountID == "" {
		return false, nil
	}
	var count int64
	if err := db.WithContext(ctx).
		Table("members").
		Where("organization_id = ? AND account_id = ? AND status = ?", organizationID, accountID, inactiveStatus).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("check organization member status: %w", err)
	}
	return count > 0, nil
}

// WorkspaceOrganizationDisabled reports whether the organization that owns
// workspaceID has an inactive members row for accountID. A workspace without
// an organization, or an account without a members row, is not disabled.
func WorkspaceOrganizationDisabled(ctx context.Context, db *gorm.DB, workspaceID, accountID string) (bool, error) {
	workspaceID, accountID = strings.TrimSpace(workspaceID), strings.TrimSpace(accountID)
	if db == nil || workspaceID == "" || accountID == "" {
		return false, nil
	}
	var count int64
	if err := db.WithContext(ctx).
		Table("members").
		Joins("JOIN workspaces ON workspaces.organization_id = members.organization_id").
		Where("workspaces.id = ? AND members.account_id = ? AND members.status = ?", workspaceID, accountID, inactiveStatus).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("check workspace organization member status: %w", err)
	}
	return count > 0, nil
}

// AccountCanUseWorkspace reports whether accountID may still act in
// workspaceID of organizationID without a person present: the account is
// active (or pending, like the setup administrator), the workspace is normal
// and belongs to the organization, the organization has not disabled the
// account, and the account is either a workspace member or an active
// organization owner or administrator. A missing members row falls back to
// workspace membership for legacy data.
func AccountCanUseWorkspace(ctx context.Context, db *gorm.DB, organizationID, workspaceID, accountID string) (bool, error) {
	organizationID = strings.TrimSpace(organizationID)
	workspaceID = strings.TrimSpace(workspaceID)
	accountID = strings.TrimSpace(accountID)
	if db == nil || organizationID == "" || workspaceID == "" || accountID == "" {
		return false, nil
	}
	var count int64
	err := db.WithContext(ctx).
		Table("workspaces").
		Joins("JOIN accounts ON accounts.id = ?", accountID).
		Where("workspaces.id = ? AND workspaces.organization_id = ? AND workspaces.status = ?",
			workspaceID, organizationID, string(model.WorkspaceStatusNormal)).
		Where("accounts.status IN ? AND accounts.deleted_at IS NULL", []string{"active", "pending"}).
		Where(NotDisabledInWorkspaceOrganizationSQL("workspaces.id", "accounts.id")).
		Where(`(EXISTS (SELECT 1 FROM workspace_members AS access_workspace_members
				WHERE access_workspace_members.workspace_id = workspaces.id AND access_workspace_members.account_id = accounts.id)
			OR EXISTS (SELECT 1 FROM members AS access_org_members
				WHERE access_org_members.organization_id = workspaces.organization_id AND access_org_members.account_id = accounts.id
				AND access_org_members.status = ? AND access_org_members.role IN ?))`,
			string(model.OrganizationMemberStatusActive),
			[]string{string(model.OrganizationRoleOwner), string(model.OrganizationRoleAdmin)}).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("check workspace access for account: %w", err)
	}
	return count > 0, nil
}

// NotDisabledInWorkspaceOrganizationSQL returns a SQL predicate that holds
// when the organization owning the workspace has no inactive members row for
// the account. Both arguments must be trusted SQL column expressions (never
// request input); the predicate uses its own table aliases so it can be added
// to queries that already alias other tables as "members".
func NotDisabledInWorkspaceOrganizationSQL(workspaceIDColumn, accountIDColumn string) string {
	return "NOT EXISTS (SELECT 1 FROM members AS disabled_org_members " +
		"JOIN workspaces AS disabled_org_workspaces ON disabled_org_workspaces.organization_id = disabled_org_members.organization_id " +
		"WHERE disabled_org_workspaces.id = " + workspaceIDColumn +
		" AND disabled_org_members.account_id = " + accountIDColumn +
		" AND disabled_org_members.status = '" + inactiveStatus + "')"
}
