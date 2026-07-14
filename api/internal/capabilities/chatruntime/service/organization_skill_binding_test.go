package service

import (
	"context"
	"errors"
	"reflect"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestRemovedOrganizationSkillIDsReturnsStableDisabledSet(t *testing.T) {
	got := removedOrganizationSkillIDs(
		[]string{"custom-b", "custom-a", "custom-a", "calculator"},
		[]string{"calculator", "custom-b"},
	)
	want := []string{"custom-a"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("removedOrganizationSkillIDs() = %#v, want %#v", got, want)
	}
}

func TestPersistOrganizationSkillPolicyRecomputesDisabledSkillsAfterOrganizationLock(t *testing.T) {
	db, mock := newOrganizationSkillPolicyMockDB(t)
	organizationID := uuid.New()
	accountID := uuid.New()
	metadata := []skills.SkillDiscoveryMetadata{
		{ID: "skill-a", Status: skills.SkillStatusActive},
		{ID: "skill-b", Status: skills.SkillStatusActive},
	}
	normalized := []string{"skill-a"}
	configs := organizationSkillConfigRows(organizationID, metadata, normalized)
	impactCheckErr := errors.New("binding impact checked")

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT "id" FROM "organizations" WHERE id = $1 LIMIT $2 FOR UPDATE`)).
		WithArgs(organizationID.String(), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(organizationID.String()))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "chat_runtime_organization_skill_configs" WHERE organization_id = $1 ORDER BY skill_id ASC`)).
		WithArgs(organizationID.String()).
		WillReturnRows(sqlmock.NewRows([]string{"organization_id", "skill_id", "enabled"}).
			AddRow(organizationID, "skill-a", true).
			AddRow(organizationID, "skill-b", true))
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`)).
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT \* FROM "agent_resource_bindings" WHERE organization_id = \$1 AND binding_type = \$2 AND resource_id IN \(\$3\) ORDER BY agent_id ASC, binding_scope ASC, published_version_uuid ASC, resource_id ASC`).
		WithArgs(organizationID, "skill", "skill-b").
		WillReturnError(impactCheckErr)
	mock.ExpectRollback()

	svc := &service{repos: repository.NewRepositories(db)}
	update, err := svc.persistOrganizationSkillPolicy(
		context.Background(),
		Scope{OrganizationID: organizationID, AccountID: accountID},
		runtimedto.UpdateSkillConfigRequest{EnabledSkillIDs: normalized},
		metadata,
		normalized,
		configs,
	)

	require.ErrorContains(t, err, impactCheckErr.Error())
	require.Equal(t, []string{"skill-a", "skill-b"}, update.previous)
	require.Equal(t, []string{"skill-b"}, update.disabledSkillIDs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func newOrganizationSkillPolicyMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{SkipDefaultTransaction: true})
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db, mock
}
