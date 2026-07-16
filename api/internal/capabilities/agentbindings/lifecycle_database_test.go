package agentbindings

import (
	"context"
	"regexp"
	"sort"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestRevokeAndPruneDraftsLocksConfigsInStableOrderBeforeDelete(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	organizationID := uuid.New()
	workspaceID := uuid.New()
	agentIDs := []uuid.UUID{uuid.New(), uuid.New()}
	sort.Slice(agentIDs, func(i, j int) bool { return agentIDs[i].String() < agentIDs[j].String() })
	ref := ResourceRef{OrganizationID: organizationID, BindingType: BindingTypeSkill, ResourceID: "custom-tool"}
	now := time.Now()
	columns := []string{
		"id", "agent_id", "binding_scope", "organization_id", "workspace_id", "published_version_uuid",
		"binding_type", "resource_id", "parent_resource_id", "display_name", "access_mode",
		"authorized_by", "authorized_at", "created_at", "updated_at",
	}
	bindingRows := sqlmock.NewRows(columns)
	for _, agentID := range agentIDs {
		bindingRows.AddRow(uuid.New(), agentID, ScopeDraft, organizationID, workspaceID, nil, BindingTypeSkill, ref.ResourceID, "", "", "execute", nil, nil, now, now)
	}

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`)).
		WithArgs(resourceLockKey(organizationID, BindingTypeSkill, "", ref.ResourceID)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "agent_resource_bindings" WHERE organization_id = $1 AND (binding_type = $2 AND resource_id = $3) ORDER BY agent_id ASC, binding_scope ASC, published_version_uuid ASC`)).
		WithArgs(organizationID, BindingTypeSkill, ref.ResourceID).
		WillReturnRows(bindingRows)
	mock.ExpectQuery(`SELECT id, agents_id, agent_mode FROM "agents_configs" WHERE agents_id IN \(\$1,\$2\) AND deleted_at IS NULL ORDER BY agents_id ASC, id ASC FOR UPDATE`).
		WithArgs(agentIDs[0], agentIDs[1]).
		WillReturnRows(sqlmock.NewRows([]string{"id", "agents_id", "agent_mode"}).
			AddRow(uuid.New(), agentIDs[0], `{}`).
			AddRow(uuid.New(), agentIDs[1], `{}`))
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "agent_resource_bindings" WHERE organization_id = $1 AND (binding_type = $2 AND resource_id = $3)`)).
		WithArgs(organizationID, BindingTypeSkill, ref.ResourceID).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	repo := NewRepositoryWithTokenSecret(db, "lifecycle-lock-secret")
	err = db.Transaction(func(tx *gorm.DB) error {
		_, revokeErr := repo.RevokeAndPruneDrafts(context.Background(), tx, ref, uuid.New())
		return revokeErr
	})
	if err != nil {
		t.Fatalf("RevokeAndPruneDrafts() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("resource/config/delete lock order changed: %v", err)
	}
}

func TestRevokeAndPruneDraftsReturnsPublishedOnlyAffectedAgent(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	organizationID := uuid.New()
	workspaceID := uuid.New()
	agentID := uuid.New()
	versionID := uuid.New()
	ref := ResourceRef{OrganizationID: organizationID, BindingType: BindingTypeSkill, ResourceID: "custom-tool"}
	now := time.Now()
	bindingRows := sqlmock.NewRows([]string{
		"id", "agent_id", "binding_scope", "organization_id", "workspace_id", "published_version_uuid",
		"binding_type", "resource_id", "parent_resource_id", "display_name", "access_mode",
		"authorized_by", "authorized_at", "created_at", "updated_at",
	}).AddRow(uuid.New(), agentID, ScopePublished, organizationID, workspaceID, versionID, BindingTypeSkill, ref.ResourceID, "", "", "execute", nil, nil, now, now)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`)).
		WithArgs(resourceLockKey(organizationID, BindingTypeSkill, "", ref.ResourceID)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "agent_resource_bindings" WHERE organization_id = $1 AND (binding_type = $2 AND resource_id = $3) ORDER BY agent_id ASC, binding_scope ASC, published_version_uuid ASC`)).
		WithArgs(organizationID, BindingTypeSkill, ref.ResourceID).
		WillReturnRows(bindingRows)
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "agent_resource_bindings" WHERE organization_id = $1 AND (binding_type = $2 AND resource_id = $3)`)).
		WithArgs(organizationID, BindingTypeSkill, ref.ResourceID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	repo := NewRepositoryWithTokenSecret(db, "published-audit-secret")
	var affected []uuid.UUID
	err = db.Transaction(func(tx *gorm.DB) error {
		var revokeErr error
		affected, revokeErr = repo.RevokeAndPruneDrafts(context.Background(), tx, ref, uuid.New())
		return revokeErr
	})
	if err != nil {
		t.Fatalf("RevokeAndPruneDrafts() error = %v", err)
	}
	if len(affected) != 1 || affected[0] != agentID {
		t.Fatalf("affected agents = %v, want published-only Agent %s", affected, agentID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("published-only revoke unexpectedly touched draft config: %v", err)
	}
}
