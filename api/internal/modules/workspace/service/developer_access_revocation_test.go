package service

import (
	"testing"

	"github.com/google/uuid"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	accessmodel "github.com/zgiai/zgi/api/internal/modules/llm/developeraccess/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRevokeWorkspaceDeveloperAccessInvalidatesMembershipAuthority(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&accessmodel.Grant{}, &accessmodel.AccessRequest{}, &apikeymodel.TenantAPIKey{}); err != nil {
		t.Fatal(err)
	}

	organizationID, workspaceID := uuid.NewString(), uuid.NewString()
	otherWorkspaceID, accountID, otherAccountID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	quota := int64(100)
	targetGrant := accessmodel.Grant{
		OrganizationID: organizationID, WorkspaceID: workspaceID,
		PrincipalType: accessmodel.PrincipalTypeUser, PrincipalID: accountID,
		Source: "approved_request", Status: accessmodel.GrantStatusActive,
		QuotaLimit: &quota, RemainQuota: quota, MaxKeys: 3,
		AllowedModels: []string{}, AuthorizationVersion: 7,
	}
	otherGrant := accessmodel.Grant{
		OrganizationID: organizationID, WorkspaceID: otherWorkspaceID,
		PrincipalType: accessmodel.PrincipalTypeUser, PrincipalID: otherAccountID,
		Source: "approved_request", Status: accessmodel.GrantStatusActive,
		QuotaLimit: &quota, RemainQuota: quota, MaxKeys: 3,
		AllowedModels: []string{}, AuthorizationVersion: 4,
	}
	if err := db.Create(&targetGrant).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&otherGrant).Error; err != nil {
		t.Fatal(err)
	}

	principalType := accessmodel.PrincipalTypeUser
	for _, key := range []apikeymodel.TenantAPIKey{
		{
			OrganizationID: organizationID, WorkspaceID: &workspaceID,
			PrincipalType: &principalType, PrincipalID: &accountID, AccessGrantID: &targetGrant.ID,
			KeyHash: "membership-active", Name: "active key", Status: "active", AuthorizationVersion: 7,
		},
		{
			OrganizationID: organizationID, WorkspaceID: &workspaceID,
			PrincipalType: &principalType, PrincipalID: &accountID, AccessGrantID: &targetGrant.ID,
			KeyHash: "membership-inactive", Name: "inactive key", Status: "inactive", AuthorizationVersion: 7,
		},
		{
			OrganizationID: organizationID, WorkspaceID: &otherWorkspaceID,
			PrincipalType: &principalType, PrincipalID: &otherAccountID, AccessGrantID: &otherGrant.ID,
			KeyHash: "membership-other", Name: "other key", Status: "active", AuthorizationVersion: 4,
		},
	} {
		if err := db.Create(&key).Error; err != nil {
			t.Fatal(err)
		}
	}

	pending := accessmodel.AccessRequest{
		OrganizationID: organizationID, WorkspaceID: workspaceID, RequesterAccountID: accountID,
		Purpose: "pending access", Environment: "development", Status: accessmodel.RequestStatusPending,
		RequestedModels: []string{},
	}
	approved := accessmodel.AccessRequest{
		OrganizationID: organizationID, WorkspaceID: workspaceID, RequesterAccountID: accountID,
		Purpose: "historical access", Environment: "development", Status: accessmodel.RequestStatusApproved,
		RequestedModels: []string{},
	}
	if err := db.Create(&pending).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&approved).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		return revokeWorkspaceDeveloperAccess(t.Context(), tx, []string{workspaceID}, accountID)
	}); err != nil {
		t.Fatalf("revokeWorkspaceDeveloperAccess() error = %v", err)
	}
	// Retrying a membership-removal transaction must not keep advancing the
	// authorization version after the grant is already revoked.
	if err := db.Transaction(func(tx *gorm.DB) error {
		return revokeWorkspaceDeveloperAccess(t.Context(), tx, []string{workspaceID}, accountID)
	}); err != nil {
		t.Fatalf("second revokeWorkspaceDeveloperAccess() error = %v", err)
	}

	var reloadedTargetGrant accessmodel.Grant
	if err := db.First(&reloadedTargetGrant, "id = ?", targetGrant.ID).Error; err != nil {
		t.Fatal(err)
	}
	if reloadedTargetGrant.Status != accessmodel.GrantStatusRevoked || reloadedTargetGrant.AuthorizationVersion != 8 {
		t.Fatalf("target grant status/version = %s/%d, want revoked/8", reloadedTargetGrant.Status, reloadedTargetGrant.AuthorizationVersion)
	}
	var reloadedOtherGrant accessmodel.Grant
	if err := db.First(&reloadedOtherGrant, "id = ?", otherGrant.ID).Error; err != nil {
		t.Fatal(err)
	}
	if reloadedOtherGrant.Status != accessmodel.GrantStatusActive || reloadedOtherGrant.AuthorizationVersion != 4 {
		t.Fatalf("unrelated grant changed: %#v", reloadedOtherGrant)
	}

	var targetKeys []apikeymodel.TenantAPIKey
	if err := db.Where("workspace_id = ? AND principal_id = ?", workspaceID, accountID).Find(&targetKeys).Error; err != nil {
		t.Fatal(err)
	}
	if len(targetKeys) != 2 {
		t.Fatalf("target key count = %d, want 2", len(targetKeys))
	}
	for _, key := range targetKeys {
		if key.Status != "revoked" || key.RevokedAt == nil || key.RevokedReason == nil || *key.RevokedReason != workspaceMembershipRemovedReason {
			t.Fatalf("target key was not revoked with membership reason: %#v", key)
		}
	}
	var otherKey apikeymodel.TenantAPIKey
	if err := db.First(&otherKey, "key_hash = ?", "membership-other").Error; err != nil {
		t.Fatal(err)
	}
	if otherKey.Status != "active" || otherKey.RevokedAt != nil {
		t.Fatalf("unrelated key changed: %#v", otherKey)
	}

	var reloadedPending, reloadedApproved accessmodel.AccessRequest
	if err := db.First(&reloadedPending, "id = ?", pending.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&reloadedApproved, "id = ?", approved.ID).Error; err != nil {
		t.Fatal(err)
	}
	if reloadedPending.Status != accessmodel.RequestStatusCancelled {
		t.Fatalf("pending request status = %s, want cancelled", reloadedPending.Status)
	}
	if reloadedApproved.Status != accessmodel.RequestStatusApproved {
		t.Fatalf("approved request status = %s, want unchanged approved", reloadedApproved.Status)
	}
}
