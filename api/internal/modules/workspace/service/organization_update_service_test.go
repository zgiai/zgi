package service

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	shared_dto "github.com/zgiai/zgi/api/internal/dto"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	workspace_repo "github.com/zgiai/zgi/api/internal/modules/workspace/repository"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestUpdateOrganizationTrimsNameAndUpdatesEditableFields(t *testing.T) {
	t.Parallel()

	db, mock := newOrganizationUpdateMockDB(t)
	expectOrganizationRoleLookup(mock, "org-1", "acc-1", string(model.OrganizationRoleAdmin))
	shortName := "Acme"
	repo := &stubOrganizationUpdateRepo{
		db: db,
		organization: &model.Organization{
			ID:        "org-1",
			Name:      "Old Name",
			Status:    model.OrganizationStatusActive,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}
	svc := &organizationService{organizationRepo: repo}

	updated, err := svc.UpdateOrganization(context.Background(), "org-1", "acc-1", &shared_dto.UpdateOrganizationRequest{
		Name:      "  New Name  ",
		ShortName: &shortName,
	})

	require.NoError(t, err)
	require.Equal(t, "New Name", updated.Name)
	require.NotNil(t, updated.ShortName)
	require.Equal(t, "Acme", *updated.ShortName)
	require.True(t, repo.updated)
	require.True(t, repo.checkedName)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateOrganizationRejectsDuplicateName(t *testing.T) {
	t.Parallel()

	db, mock := newOrganizationUpdateMockDB(t)
	expectOrganizationRoleLookup(mock, "org-1", "acc-1", string(model.OrganizationRoleOwner))
	repo := &stubOrganizationUpdateRepo{
		db: db,
		organization: &model.Organization{
			ID:     "org-1",
			Name:   "Old Name",
			Status: model.OrganizationStatusActive,
		},
		nameExists: true,
	}
	svc := &organizationService{organizationRepo: repo}

	_, err := svc.UpdateOrganization(context.Background(), "org-1", "acc-1", &shared_dto.UpdateOrganizationRequest{Name: "Taken"})

	require.ErrorIs(t, err, ErrOrganizationNameExists)
	require.False(t, repo.updated)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateOrganizationRejectsNormalMember(t *testing.T) {
	t.Parallel()

	db, mock := newOrganizationUpdateMockDB(t)
	expectOrganizationRoleLookup(mock, "org-1", "acc-1", string(model.OrganizationRoleNormal))
	repo := &stubOrganizationUpdateRepo{
		db: db,
		organization: &model.Organization{
			ID:     "org-1",
			Name:   "Old Name",
			Status: model.OrganizationStatusActive,
		},
	}
	svc := &organizationService{organizationRepo: repo}

	_, err := svc.UpdateOrganization(context.Background(), "org-1", "acc-1", &shared_dto.UpdateOrganizationRequest{Name: "New Name"})

	require.ErrorIs(t, err, ErrOrganizationPermissionDenied)
	require.False(t, repo.updated)
	require.False(t, repo.checkedName)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateOrganizationRejectsInvalidAndArchivedOrganizations(t *testing.T) {
	t.Parallel()

	repo := &stubOrganizationUpdateRepo{
		organization: &model.Organization{
			ID:     "org-1",
			Name:   "Old Name",
			Status: model.OrganizationStatusArchived,
		},
	}
	svc := &organizationService{organizationRepo: repo}

	_, err := svc.UpdateOrganization(context.Background(), "org-1", "acc-1", &shared_dto.UpdateOrganizationRequest{Name: "   "})
	require.ErrorIs(t, err, ErrInvalidOrganizationName)

	_, err = svc.UpdateOrganization(context.Background(), "org-1", "acc-1", &shared_dto.UpdateOrganizationRequest{Name: "New Name"})
	require.ErrorIs(t, err, ErrOrganizationNotEditable)
	require.False(t, repo.updated)
}

func TestUpdateCurrentOrganizationMemberRolePromotesNormalMember(t *testing.T) {
	t.Parallel()

	repo := &stubOrganizationUpdateRepo{
		organization: &model.Organization{ID: "org-1", Name: "Org", Status: model.OrganizationStatusActive},
		joins: map[string]*model.OrganizationMember{
			"owner-1":  {OrganizationID: "org-1", AccountID: "owner-1", Role: model.OrganizationRoleOwner, Status: model.OrganizationMemberStatusActive},
			"member-1": {OrganizationID: "org-1", AccountID: "member-1", Role: model.OrganizationRoleNormal, Status: model.OrganizationMemberStatusActive},
		},
	}
	svc := &organizationService{
		organizationRepo: repo,
		accountService:   stubCurrentOrganizationAccountService{organizationID: "org-1"},
	}

	err := svc.UpdateCurrentOrganizationMemberRole(context.Background(), "owner-1", "member-1", model.OrganizationRoleAdmin)

	require.NoError(t, err)
	require.Equal(t, model.OrganizationRoleAdmin, repo.joins["member-1"].Role)
	require.Equal(t, "member-1", repo.updatedJoinAccountID)
}

func TestUpdateCurrentOrganizationMemberRoleDemotesAdminMember(t *testing.T) {
	t.Parallel()

	repo := &stubOrganizationUpdateRepo{
		organization: &model.Organization{ID: "org-1", Name: "Org", Status: model.OrganizationStatusActive},
		joins: map[string]*model.OrganizationMember{
			"owner-1": {OrganizationID: "org-1", AccountID: "owner-1", Role: model.OrganizationRoleOwner, Status: model.OrganizationMemberStatusActive},
			"admin-1": {OrganizationID: "org-1", AccountID: "admin-1", Role: model.OrganizationRoleAdmin, Status: model.OrganizationMemberStatusActive},
		},
	}
	svc := &organizationService{
		organizationRepo: repo,
		accountService:   stubCurrentOrganizationAccountService{organizationID: "org-1"},
	}

	err := svc.UpdateCurrentOrganizationMemberRole(context.Background(), "owner-1", "admin-1", model.OrganizationRoleNormal)

	require.NoError(t, err)
	require.Equal(t, model.OrganizationRoleNormal, repo.joins["admin-1"].Role)
	require.Equal(t, "admin-1", repo.updatedJoinAccountID)
}

func TestUpdateCurrentOrganizationMemberRoleIsIdempotent(t *testing.T) {
	t.Parallel()

	repo := &stubOrganizationUpdateRepo{
		organization: &model.Organization{ID: "org-1", Name: "Org", Status: model.OrganizationStatusActive},
		joins: map[string]*model.OrganizationMember{
			"owner-1": {OrganizationID: "org-1", AccountID: "owner-1", Role: model.OrganizationRoleOwner, Status: model.OrganizationMemberStatusActive},
			"admin-1": {OrganizationID: "org-1", AccountID: "admin-1", Role: model.OrganizationRoleAdmin, Status: model.OrganizationMemberStatusActive},
		},
	}
	svc := &organizationService{
		organizationRepo: repo,
		accountService:   stubCurrentOrganizationAccountService{organizationID: "org-1"},
	}

	err := svc.UpdateCurrentOrganizationMemberRole(context.Background(), "owner-1", "admin-1", model.OrganizationRoleAdmin)

	require.NoError(t, err)
	require.Empty(t, repo.updatedJoinAccountID)
}

func TestUpdateCurrentOrganizationMemberRoleRejectsInvalidOperatorsAndTargets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		operatorID string
		memberID   string
		role       model.OrganizationRole
		joins      map[string]*model.OrganizationMember
		wantErr    error
	}{
		{
			name:       "admin operator denied",
			operatorID: "admin-1",
			memberID:   "member-1",
			role:       model.OrganizationRoleAdmin,
			joins: map[string]*model.OrganizationMember{
				"admin-1":  {OrganizationID: "org-1", AccountID: "admin-1", Role: model.OrganizationRoleAdmin, Status: model.OrganizationMemberStatusActive},
				"member-1": {OrganizationID: "org-1", AccountID: "member-1", Role: model.OrganizationRoleNormal, Status: model.OrganizationMemberStatusActive},
			},
			wantErr: ErrOrganizationPermissionDenied,
		},
		{
			name:       "normal operator denied",
			operatorID: "normal-1",
			memberID:   "member-1",
			role:       model.OrganizationRoleAdmin,
			joins: map[string]*model.OrganizationMember{
				"normal-1": {OrganizationID: "org-1", AccountID: "normal-1", Role: model.OrganizationRoleNormal, Status: model.OrganizationMemberStatusActive},
				"member-1": {OrganizationID: "org-1", AccountID: "member-1", Role: model.OrganizationRoleNormal, Status: model.OrganizationMemberStatusActive},
			},
			wantErr: ErrOrganizationPermissionDenied,
		},
		{
			name:       "target member missing",
			operatorID: "owner-1",
			memberID:   "missing-1",
			role:       model.OrganizationRoleAdmin,
			joins: map[string]*model.OrganizationMember{
				"owner-1": {OrganizationID: "org-1", AccountID: "owner-1", Role: model.OrganizationRoleOwner, Status: model.OrganizationMemberStatusActive},
			},
			wantErr: ErrOrganizationMemberNotFound,
		},
		{
			name:       "target owner immutable",
			operatorID: "owner-1",
			memberID:   "owner-2",
			role:       model.OrganizationRoleNormal,
			joins: map[string]*model.OrganizationMember{
				"owner-1": {OrganizationID: "org-1", AccountID: "owner-1", Role: model.OrganizationRoleOwner, Status: model.OrganizationMemberStatusActive},
				"owner-2": {OrganizationID: "org-1", AccountID: "owner-2", Role: model.OrganizationRoleOwner, Status: model.OrganizationMemberStatusActive},
			},
			wantErr: ErrOrganizationOwnerRoleImmutable,
		},
		{
			name:       "target inactive",
			operatorID: "owner-1",
			memberID:   "member-1",
			role:       model.OrganizationRoleAdmin,
			joins: map[string]*model.OrganizationMember{
				"owner-1":  {OrganizationID: "org-1", AccountID: "owner-1", Role: model.OrganizationRoleOwner, Status: model.OrganizationMemberStatusActive},
				"member-1": {OrganizationID: "org-1", AccountID: "member-1", Role: model.OrganizationRoleNormal, Status: model.OrganizationMemberStatusInactive},
			},
			wantErr: ErrOrganizationMemberNotActive,
		},
		{
			name:       "owner role rejected",
			operatorID: "owner-1",
			memberID:   "member-1",
			role:       model.OrganizationRoleOwner,
			joins: map[string]*model.OrganizationMember{
				"owner-1":  {OrganizationID: "org-1", AccountID: "owner-1", Role: model.OrganizationRoleOwner, Status: model.OrganizationMemberStatusActive},
				"member-1": {OrganizationID: "org-1", AccountID: "member-1", Role: model.OrganizationRoleNormal, Status: model.OrganizationMemberStatusActive},
			},
			wantErr: ErrInvalidOrganizationMemberRole,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := &stubOrganizationUpdateRepo{
				organization: &model.Organization{ID: "org-1", Name: "Org", Status: model.OrganizationStatusActive},
				joins:        tt.joins,
			}
			svc := &organizationService{
				organizationRepo: repo,
				accountService:   stubCurrentOrganizationAccountService{organizationID: "org-1"},
			}

			err := svc.UpdateCurrentOrganizationMemberRole(context.Background(), tt.operatorID, tt.memberID, tt.role)

			require.ErrorIs(t, err, tt.wantErr)
			require.Empty(t, repo.updatedJoinAccountID)
		})
	}
}

func TestUpdateMemberInfoRejectsRoleUpdate(t *testing.T) {
	t.Parallel()

	role := model.OrganizationRoleAdmin
	svc := &organizationService{organizationRepo: &stubOrganizationUpdateRepo{}}

	err := svc.UpdateMemberInfo(context.Background(), &shared_dto.UpdateOrganizationMemberRequest{Role: &role})

	require.ErrorIs(t, err, ErrOrganizationMemberRoleUpdateUnsupported)
}

type stubOrganizationUpdateRepo struct {
	workspace_repo.OrganizationRepository
	db                   *gorm.DB
	organization         *model.Organization
	joins                map[string]*model.OrganizationMember
	nameExists           bool
	checkedName          bool
	updated              bool
	updatedJoinAccountID string
}

func (r *stubOrganizationUpdateRepo) GetByID(ctx context.Context, id string) (*model.Organization, error) {
	if r.organization == nil || r.organization.ID != id {
		return nil, gorm.ErrRecordNotFound
	}
	return r.organization, nil
}

func (r *stubOrganizationUpdateRepo) ExistsByNameExcludingID(ctx context.Context, name, excludeID string) (bool, error) {
	r.checkedName = true
	return r.nameExists, nil
}

func (r *stubOrganizationUpdateRepo) Update(ctx context.Context, organization *model.Organization) error {
	if organization == nil {
		return errors.New("organization is nil")
	}
	r.organization = organization
	r.updated = true
	return nil
}

func (r *stubOrganizationUpdateRepo) GetAccountJoin(ctx context.Context, organizationID, accountID string) (*model.OrganizationMember, error) {
	if r.joins == nil {
		return nil, gorm.ErrRecordNotFound
	}
	join, ok := r.joins[accountID]
	if !ok || join.OrganizationID != organizationID {
		return nil, gorm.ErrRecordNotFound
	}
	return join, nil
}

func (r *stubOrganizationUpdateRepo) UpdateAccountJoin(ctx context.Context, join *model.OrganizationMember) error {
	if join == nil {
		return errors.New("organization member is nil")
	}
	if r.joins == nil {
		r.joins = map[string]*model.OrganizationMember{}
	}
	r.joins[join.AccountID] = join
	r.updatedJoinAccountID = join.AccountID
	return nil
}

func (r *stubOrganizationUpdateRepo) GetDB() *gorm.DB {
	return r.db
}

type stubCurrentOrganizationAccountService struct {
	interfaces.AccountService
	organizationID string
	err            error
}

func (s stubCurrentOrganizationAccountService) EnsureCurrentOrganizationID(ctx context.Context, accountID string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.organizationID, nil
}

func newOrganizationUpdateMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{SkipDefaultTransaction: true})
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	return db, mock
}

func expectOrganizationRoleLookup(mock sqlmock.Sqlmock, organizationID, accountID, role string) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT role FROM "members" WHERE organization_id = $1 AND account_id = $2`)).
		WithArgs(organizationID, accountID).
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow(role))
}
