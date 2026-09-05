package service

import (
	"context"
	"fmt"
	"time"

	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	accessmodel "github.com/zgiai/zgi/api/internal/modules/llm/developeraccess/model"
	"gorm.io/gorm"
)

const workspaceMembershipRemovedReason = "workspace_membership_removed"

// revokeWorkspaceDeveloperAccess invalidates every user credential whose
// authority came from one of the removed workspace memberships. It must run in
// the same transaction that deletes those memberships so a user can never be
// left without membership while an old personal key remains reusable.
func revokeWorkspaceDeveloperAccess(ctx context.Context, tx *gorm.DB, workspaceIDs []string, accountID string) error {
	if tx == nil || len(workspaceIDs) == 0 || accountID == "" {
		return nil
	}

	now := time.Now()
	requestResult := tx.WithContext(ctx).
		Model(&accessmodel.AccessRequest{}).
		Where("workspace_id IN ? AND requester_account_id = ? AND status = ?", workspaceIDs, accountID, accessmodel.RequestStatusPending).
		Updates(map[string]any{
			"status":     accessmodel.RequestStatusCancelled,
			"updated_at": now,
		})
	if requestResult.Error != nil {
		return fmt.Errorf("cancel developer access requests: %w", requestResult.Error)
	}

	grantResult := tx.WithContext(ctx).
		Model(&accessmodel.Grant{}).
		Where("workspace_id IN ? AND principal_type = ? AND principal_id = ? AND status <> ?", workspaceIDs, accessmodel.PrincipalTypeUser, accountID, accessmodel.GrantStatusRevoked).
		Updates(map[string]any{
			"status":                accessmodel.GrantStatusRevoked,
			"authorization_version": gorm.Expr("authorization_version + 1"),
			"updated_at":            now,
		})
	if grantResult.Error != nil {
		return fmt.Errorf("revoke developer access grants: %w", grantResult.Error)
	}

	reason := workspaceMembershipRemovedReason
	keyResult := tx.WithContext(ctx).
		Model(&apikeymodel.TenantAPIKey{}).
		Where("workspace_id IN ? AND principal_type = ? AND principal_id = ? AND status <> ?", workspaceIDs, accessmodel.PrincipalTypeUser, accountID, "revoked").
		Updates(map[string]any{
			"status":         "revoked",
			"revoked_at":     now,
			"revoked_reason": reason,
			"updated_at":     now,
		})
	if keyResult.Error != nil {
		return fmt.Errorf("revoke personal API keys: %w", keyResult.Error)
	}

	return nil
}
