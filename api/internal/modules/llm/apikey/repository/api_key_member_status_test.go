package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	accessmodel "github.com/zgiai/zgi/api/internal/modules/llm/developeraccess/model"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestValidatePrincipalAccessRejectsDisabledOrganizationMember(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.TenantAPIKey{},
		&workspacemodel.Organization{},
		&workspacemodel.OrganizationMember{},
		&workspacemodel.Workspace{},
		&workspacemodel.WorkspaceMember{},
		&accessmodel.Grant{},
		&accessmodel.Policy{},
	))
	organizationID, workspaceID := uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Create(&workspacemodel.Organization{ID: organizationID, Name: "Org", Status: workspacemodel.OrganizationStatusActive}).Error)
	require.NoError(t, db.Create(&workspacemodel.Workspace{ID: workspaceID, OrganizationID: &organizationID, Name: "Workspace", Status: workspacemodel.WorkspaceStatusNormal}).Error)

	newPersonalKey := func(principalID string) *model.TenantAPIKey {
		require.NoError(t, db.Create(&workspacemodel.WorkspaceMember{
			ID: uuid.NewString(), WorkspaceID: workspaceID, AccountID: principalID, Role: workspacemodel.WorkspaceRoleMember,
		}).Error)
		grant := accessmodel.Grant{
			OrganizationID: organizationID, WorkspaceID: workspaceID,
			PrincipalType: accessmodel.PrincipalTypeUser, PrincipalID: principalID,
			Source: "approved_request", Status: accessmodel.GrantStatusActive,
			MaxKeys: 1, AllowedModels: []string{}, AuthorizationVersion: 1,
		}
		require.NoError(t, db.Create(&grant).Error)
		principalType := accessmodel.PrincipalTypeUser
		key := &model.TenantAPIKey{
			ID: uuid.NewString(), OrganizationID: organizationID, WorkspaceID: &workspaceID,
			PrincipalType: &principalType, PrincipalID: &principalID, AccessGrantID: &grant.ID,
			KeyHash: uuid.NewString(), Name: "personal", Status: "active", AuthorizationVersion: 1,
		}
		require.NoError(t, db.Create(key).Error)
		return key
	}

	repo := NewAPIKeyRepository(db)
	ctx := context.Background()

	memberID := uuid.NewString()
	require.NoError(t, db.Create(&workspacemodel.OrganizationMember{
		OrganizationID: organizationID, AccountID: memberID, Role: workspacemodel.OrganizationRoleNormal,
		Status: workspacemodel.OrganizationMemberStatusActive,
	}).Error)
	memberKey := newPersonalKey(memberID)
	require.NoError(t, repo.ValidatePrincipalAccess(ctx, memberKey))

	require.NoError(t, db.Model(&workspacemodel.OrganizationMember{}).
		Where("organization_id = ? AND account_id = ?", organizationID, memberID).
		Update("status", workspacemodel.OrganizationMemberStatusInactive).Error)
	require.ErrorContains(t, repo.ValidatePrincipalAccess(ctx, memberKey), "organization membership is disabled")

	legacyKey := newPersonalKey(uuid.NewString())
	require.NoError(t, repo.ValidatePrincipalAccess(ctx, legacyKey), "a principal without a members row keeps workspace-membership access")
}
