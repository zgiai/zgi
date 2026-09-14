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
const workspaceOrganizationChangedReason = "workspace_organization_changed"

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

// retireWorkspaceDeveloperAccessForOrganizationChange closes every
// organization-scoped developer-access object before a workspace is detached
// or attached. Policies, requests, and grants belong to the funding
// organization, so carrying them across that boundary would either expose old
// governance state or produce keys that fail organization/grant validation.
func retireWorkspaceDeveloperAccessForOrganizationChange(ctx context.Context, tx *gorm.DB, workspaceID string) error {
	if tx == nil || workspaceID == "" {
		return nil
	}
	// Registration and other focused service tests may intentionally use a
	// reduced schema. Production migrations create the developer-access tables
	// atomically; if the module is absent there is no state to retire.
	if !tx.Migrator().HasTable(&accessmodel.Policy{}) {
		return nil
	}
	now := time.Now()
	if err := tx.WithContext(ctx).Model(&accessmodel.AccessRequest{}).
		Where("workspace_id = ? AND status = ?", workspaceID, accessmodel.RequestStatusPending).
		Updates(map[string]any{"status": accessmodel.RequestStatusCancelled, "updated_at": now}).Error; err != nil {
		return fmt.Errorf("cancel workspace developer access requests: %w", err)
	}
	if err := tx.WithContext(ctx).Model(&accessmodel.Grant{}).
		Where("workspace_id = ? AND status <> ?", workspaceID, accessmodel.GrantStatusRevoked).
		Updates(map[string]any{
			"status": accessmodel.GrantStatusRevoked, "authorization_version": gorm.Expr("authorization_version + 1"), "updated_at": now,
		}).Error; err != nil {
		return fmt.Errorf("revoke workspace developer access grants: %w", err)
	}
	reason := workspaceOrganizationChangedReason
	if err := tx.WithContext(ctx).Model(&apikeymodel.TenantAPIKey{}).
		Where("workspace_id = ? AND principal_type IS NOT NULL AND status <> ?", workspaceID, "revoked").
		Updates(map[string]any{
			"status": "revoked", "revoked_at": now, "revoked_reason": reason, "updated_at": now,
		}).Error; err != nil {
		return fmt.Errorf("revoke workspace personal API keys: %w", err)
	}
	// Soft deletion frees the partial unique indexes so the destination
	// organization can establish a new policy and fresh authorization periods,
	// while Unscoped audit lookups can still identify historical keys.
	if err := tx.WithContext(ctx).Where("workspace_id = ?", workspaceID).Delete(&accessmodel.Policy{}).Error; err != nil {
		return fmt.Errorf("retire workspace developer access policy: %w", err)
	}
	if err := tx.WithContext(ctx).Where("workspace_id = ?", workspaceID).Delete(&accessmodel.AccessRequest{}).Error; err != nil {
		return fmt.Errorf("retire workspace developer access requests: %w", err)
	}
	if err := tx.WithContext(ctx).Where("workspace_id = ?", workspaceID).Delete(&accessmodel.Grant{}).Error; err != nil {
		return fmt.Errorf("retire workspace developer access grants: %w", err)
	}
	return nil
}
