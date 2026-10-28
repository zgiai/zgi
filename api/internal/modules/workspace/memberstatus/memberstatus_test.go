package memberstatus

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type memberStatusDB struct {
	db             *gorm.DB
	organizationID string
	workspaceID    string
}

func newMemberStatusDB(t *testing.T) *memberStatusDB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&auth_model.Account{},
		&model.Organization{},
		&model.OrganizationMember{},
		&model.Workspace{},
		&model.WorkspaceMember{},
	))
	fixture := &memberStatusDB{db: db, organizationID: uuid.NewString(), workspaceID: uuid.NewString()}
	require.NoError(t, db.Create(&model.Organization{ID: fixture.organizationID, Name: "Org", Status: model.OrganizationStatusActive}).Error)
	require.NoError(t, db.Create(&model.Workspace{ID: fixture.workspaceID, Name: "Workspace", Status: model.WorkspaceStatusNormal, OrganizationID: &fixture.organizationID}).Error)
	return fixture
}

func (f *memberStatusDB) account(t *testing.T, status auth_model.AccountStatus) string {
	t.Helper()
	id := uuid.NewString()
	require.NoError(t, f.db.Create(&auth_model.Account{ID: id, Name: id, Email: id + "@example.com", Status: status}).Error)
	return id
}

func (f *memberStatusDB) member(t *testing.T, accountID string, role model.OrganizationRole, status model.OrganizationMemberStatus) {
	t.Helper()
	require.NoError(t, f.db.Create(&model.OrganizationMember{
		OrganizationID: f.organizationID, AccountID: accountID, Role: role, Status: status,
	}).Error)
}

func (f *memberStatusDB) workspaceMember(t *testing.T, accountID string) {
	t.Helper()
	join := &model.WorkspaceMember{ID: uuid.NewString(), WorkspaceID: f.workspaceID, AccountID: accountID, Role: model.WorkspaceRoleNormal}
	model.ApplyWorkspaceMemberDefaults(join)
	require.NoError(t, f.db.Create(join).Error)
}

func TestDisabledChecksDistinguishInactiveFromMissingRows(t *testing.T) {
	fixture := newMemberStatusDB(t)
	ctx := context.Background()
	active := uuid.NewString()
	disabled := uuid.NewString()
	legacy := uuid.NewString()
	fixture.member(t, active, model.OrganizationRoleNormal, model.OrganizationMemberStatusActive)
	fixture.member(t, disabled, model.OrganizationRoleNormal, model.OrganizationMemberStatusInactive)

	for _, test := range []struct {
		accountID string
		disabled  bool
	}{{active, false}, {disabled, true}, {legacy, false}} {
		got, err := OrganizationDisabled(ctx, fixture.db, fixture.organizationID, test.accountID)
		require.NoError(t, err)
		require.Equal(t, test.disabled, got)

		got, err = WorkspaceOrganizationDisabled(ctx, fixture.db, fixture.workspaceID, test.accountID)
		require.NoError(t, err)
		require.Equal(t, test.disabled, got)
	}

	got, err := WorkspaceOrganizationDisabled(ctx, fixture.db, uuid.NewString(), disabled)
	require.NoError(t, err)
	require.False(t, got, "an unknown workspace has no organization to disable the account")
}

func TestNotDisabledPredicateCoexistsWithMembersAlias(t *testing.T) {
	fixture := newMemberStatusDB(t)
	active := uuid.NewString()
	disabled := uuid.NewString()
	legacy := uuid.NewString()
	fixture.member(t, active, model.OrganizationRoleNormal, model.OrganizationMemberStatusActive)
	fixture.member(t, disabled, model.OrganizationRoleNormal, model.OrganizationMemberStatusInactive)
	for _, accountID := range []string{active, disabled, legacy} {
		fixture.workspaceMember(t, accountID)
	}

	var accountIDs []string
	require.NoError(t, fixture.db.Table("workspace_members AS members").
		Where("members.workspace_id = ?", fixture.workspaceID).
		Where(NotDisabledInWorkspaceOrganizationSQL("members.workspace_id", "members.account_id")).
		Pluck("members.account_id", &accountIDs).Error)
	require.ElementsMatch(t, []string{active, legacy}, accountIDs)
}

func TestAccountCanUseWorkspace(t *testing.T) {
	fixture := newMemberStatusDB(t)
	ctx := context.Background()

	activeMember := fixture.account(t, auth_model.AccountStatusActive)
	fixture.member(t, activeMember, model.OrganizationRoleNormal, model.OrganizationMemberStatusActive)
	fixture.workspaceMember(t, activeMember)

	disabledMember := fixture.account(t, auth_model.AccountStatusActive)
	fixture.member(t, disabledMember, model.OrganizationRoleNormal, model.OrganizationMemberStatusInactive)
	fixture.workspaceMember(t, disabledMember)

	legacyMember := fixture.account(t, auth_model.AccountStatusActive)
	fixture.workspaceMember(t, legacyMember)

	organizationAdmin := fixture.account(t, auth_model.AccountStatusActive)
	fixture.member(t, organizationAdmin, model.OrganizationRoleAdmin, model.OrganizationMemberStatusActive)

	organizationOnly := fixture.account(t, auth_model.AccountStatusActive)
	fixture.member(t, organizationOnly, model.OrganizationRoleNormal, model.OrganizationMemberStatusActive)

	bannedMember := fixture.account(t, auth_model.AccountStatusBanned)
	fixture.member(t, bannedMember, model.OrganizationRoleNormal, model.OrganizationMemberStatusActive)
	fixture.workspaceMember(t, bannedMember)

	for name, test := range map[string]struct {
		accountID string
		allowed   bool
	}{
		"active workspace member":               {activeMember, true},
		"disabled organization member":          {disabledMember, false},
		"legacy member without members row":     {legacyMember, true},
		"active organization administrator":     {organizationAdmin, true},
		"organization member outside workspace": {organizationOnly, false},
		"banned account":                        {bannedMember, false},
	} {
		t.Run(name, func(t *testing.T) {
			allowed, err := AccountCanUseWorkspace(ctx, fixture.db, fixture.organizationID, fixture.workspaceID, test.accountID)
			require.NoError(t, err)
			require.Equal(t, test.allowed, allowed)
		})
	}

	allowed, err := AccountCanUseWorkspace(ctx, fixture.db, uuid.NewString(), fixture.workspaceID, activeMember)
	require.NoError(t, err)
	require.False(t, allowed, "the workspace must belong to the task organization")
}

func TestMemberDisabledErrorIsCataloged(t *testing.T) {
	productCatalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal,
		append(appcatalog.DefaultDefinitions(), CatalogDefinitions()...)...)
	require.NoError(t, err)

	err = NewMemberDisabledError("test.operation")
	require.True(t, apperror.IsCode(err, AppCodeMemberDisabled))
	for _, locale := range []appcatalog.Locale{appcatalog.LocaleEnglishUS, appcatalog.LocaleChineseSimplified} {
		presentation, presentErr := productCatalog.Present(AppCodeMemberDisabled, locale, nil)
		require.NoError(t, presentErr)
		require.Equal(t, 403, presentation.HTTPStatus)
		require.NotEmpty(t, presentation.Message)
	}
}
