package agentbindings

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPreviewDatabaseImpactIncludesChildTableBindings(t *testing.T) {
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
	now := time.Unix(1_800_000_000, 0)
	columns := []string{
		"id", "agent_id", "binding_scope", "organization_id", "workspace_id", "published_version_uuid",
		"binding_type", "resource_id", "parent_resource_id", "display_name", "access_mode",
		"authorized_by", "authorized_at", "created_at", "updated_at",
	}
	rows := sqlmock.NewRows(columns).
		AddRow(uuid.New(), agentID, ScopeDraft, organizationID, workspaceID, nil, BindingTypeDatabase, "database-1", "", "database", "read", nil, nil, now, now).
		AddRow(uuid.New(), agentID, ScopeDraft, organizationID, workspaceID, nil, BindingTypeDatabaseTable, "table-1", "database-1", "table", "write", nil, nil, now, now)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "agent_resource_bindings" WHERE organization_id = $1 AND ((binding_type = $2 AND resource_id = $3) OR (binding_type = $4 AND parent_resource_id = $5)) ORDER BY agent_id ASC, binding_scope ASC, published_version_uuid ASC`)).
		WithArgs(organizationID, BindingTypeDatabase, "database-1", BindingTypeDatabaseTable, "database-1").
		WillReturnRows(rows)
	mock.ExpectQuery(`SELECT id, name, description, icon_type, icon FROM "agents" WHERE id IN \(\$1\) AND deleted_at IS NULL`).
		WithArgs(agentID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "description", "icon_type", "icon"}).
			AddRow(agentID, "Sales assistant", "Answers sales questions", "text", `{"icon":"SA","icon_background":"#123456"}`))

	repo := NewRepositoryWithTokenSecret(db, "shared-database-secret")
	impact, err := repo.PreviewImpact(context.Background(), ResourceRef{
		OrganizationID: organizationID,
		BindingType:    BindingTypeDatabase,
		ResourceID:     "database-1",
	}, "delete_database", uuid.New(), now)
	if err != nil {
		t.Fatalf("PreviewImpact() error = %v", err)
	}
	if impact == nil || len(impact.Agents) != 1 || impact.Agents[0].Name != "Sales assistant" || impact.Agents[0].Description != "Answers sales questions" {
		t.Fatalf("impact = %#v, want database and child table bindings", impact)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestPreviewWorkflowImpactMatchesTargetAgentID(t *testing.T) {
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
	consumerAgentID := uuid.New()
	targetAgentID := uuid.NewString()
	now := time.Unix(1_800_000_000, 0)
	columns := []string{
		"id", "agent_id", "binding_scope", "organization_id", "workspace_id", "published_version_uuid",
		"binding_type", "resource_id", "parent_resource_id", "display_name", "access_mode",
		"authorized_by", "authorized_at", "created_at", "updated_at",
	}
	rows := sqlmock.NewRows(columns).
		AddRow(uuid.New(), consumerAgentID, ScopeDraft, organizationID, workspaceID, nil, BindingTypeWorkflow, "stable-binding-id", targetAgentID, "workflow", "execute", nil, nil, now, now)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "agent_resource_bindings" WHERE organization_id = $1 AND (binding_type = $2 AND (resource_id = $3 OR parent_resource_id = $4)) ORDER BY agent_id ASC, binding_scope ASC, published_version_uuid ASC`)).
		WithArgs(organizationID, BindingTypeWorkflow, targetAgentID, targetAgentID).
		WillReturnRows(rows)
	mock.ExpectQuery(`SELECT id, name, description, icon_type, icon FROM "agents" WHERE id IN \(\$1\) AND deleted_at IS NULL`).
		WithArgs(consumerAgentID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "description", "icon_type", "icon"}).
			AddRow(consumerAgentID, "Workflow consumer", "Uses the workflow", "text", `{"icon":"WC"}`))

	repo := NewRepositoryWithTokenSecret(db, "shared-workflow-secret")
	impact, err := repo.PreviewImpact(context.Background(), ResourceRef{
		OrganizationID: organizationID,
		BindingType:    BindingTypeWorkflow,
		ResourceID:     targetAgentID,
	}, "delete_workflow", uuid.New(), now)
	if err != nil {
		t.Fatalf("PreviewImpact() error = %v", err)
	}
	if impact == nil || len(impact.Agents) != 1 || impact.Agents[0].Name != "Workflow consumer" {
		t.Fatalf("impact = %#v, want target workflow Agent binding", impact)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestPreviewMoveDependenciesReturnsPresentationWithoutTargetToken(t *testing.T) {
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
	now := time.Unix(1_800_000_000, 0)
	columns := []string{
		"id", "agent_id", "binding_scope", "organization_id", "workspace_id", "published_version_uuid",
		"binding_type", "resource_id", "parent_resource_id", "display_name", "access_mode",
		"authorized_by", "authorized_at", "created_at", "updated_at",
	}
	mock.ExpectQuery(`SELECT \* FROM "agent_resource_bindings" WHERE organization_id = \$1 AND \(binding_type = \$2 AND resource_id = \$3\)`).
		WithArgs(organizationID, BindingTypeKnowledgeDataset, "dataset-1").
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow(uuid.New(), agentID, ScopeDraft, organizationID, workspaceID, nil, BindingTypeKnowledgeDataset, "dataset-1", "", "Knowledge", "read", nil, nil, now, now))
	mock.ExpectQuery(`SELECT id, name, description, icon_type, icon FROM "agents" WHERE id IN \(\$1\) AND deleted_at IS NULL`).
		WithArgs(agentID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "description", "icon_type", "icon"}).
			AddRow(agentID, "Knowledge Agent", "Uses this knowledge base", "text", `{"icon":"KA"}`))

	repo := NewRepositoryWithTokenSecret(db, "dependency-preview-secret")
	agents, err := repo.PreviewMoveDependencies(context.Background(), MoveDependencyRequest{
		OrganizationID: organizationID,
		ResourceRefs: []ResourceRef{{
			BindingType: BindingTypeKnowledgeDataset,
			ResourceID:  "dataset-1",
		}},
	})
	if err != nil {
		t.Fatalf("PreviewMoveDependencies() error = %v", err)
	}
	if len(agents) != 1 || agents[0].Name != "Knowledge Agent" || agents[0].Description != "Uses this knowledge base" {
		t.Fatalf("Agents = %#v, want impacted Agent presentation", agents)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestRevokeWorkflowResourceMatchesTargetAgentID(t *testing.T) {
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
	targetAgentID := uuid.NewString()
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "agent_resource_bindings" WHERE organization_id = $1 AND (binding_type = $2 AND (resource_id = $3 OR parent_resource_id = $4))`)).
		WithArgs(organizationID, BindingTypeWorkflow, targetAgentID, targetAgentID).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	repo := NewRepositoryWithTokenSecret(db, "shared-workflow-secret")
	err = repo.RevokeResource(context.Background(), nil, ResourceRef{
		OrganizationID: organizationID,
		BindingType:    BindingTypeWorkflow,
		ResourceID:     targetAgentID,
	})
	if err != nil {
		t.Fatalf("RevokeResource() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestReplacePublishedHeadPurgesHistoricalPublishedScopes(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	agentID := uuid.New()
	organizationID := uuid.New()
	workspaceID := uuid.New()
	repo := NewRepositoryWithTokenSecret(db, "published-head-secret")

	for _, versionID := range []uuid.UUID{uuid.New(), uuid.New()} {
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "agent_resource_bindings" WHERE agent_id = $1 AND binding_scope = $2`)).
			WithArgs(agentID, ScopePublished).
			WillReturnResult(sqlmock.NewResult(0, 2))
		mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "agent_resource_bindings" WHERE (agent_id = $1 AND binding_scope = $2) AND published_version_uuid = $3`)).
			WithArgs(agentID, ScopePublished, versionID).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`INSERT INTO "agent_resource_bindings" .*RETURNING "id"`).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
		mock.ExpectCommit()

		err := repo.ReplacePublishedHead(context.Background(), nil, ScopeRef{
			AgentID: agentID, Scope: ScopePublished, PublishedVersionUUID: &versionID,
		}, []Binding{{
			OrganizationID: organizationID,
			WorkspaceID:    workspaceID,
			BindingType:    BindingTypeSkill,
			ResourceID:     "chart-generator",
			AccessMode:     "execute",
		}})
		if err != nil {
			t.Fatalf("ReplacePublishedHead(%s) error = %v", versionID, err)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("consecutive publish did not replace the prior published head: %v", err)
	}
}

func TestWorkflowResourceLocksShareCanonicalTargetKey(t *testing.T) {
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
	targetAgentID := uuid.NewString()
	bindingID := uuid.NewString()
	canonicalKey := resourceLockKey(organizationID, BindingTypeWorkflow, "", targetAgentID)
	specificKey := resourceLockKey(organizationID, BindingTypeWorkflow, targetAgentID, bindingID)
	lockSQL := regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`)
	repo := NewRepositoryWithTokenSecret(db, "lock-secret")

	mock.ExpectBegin()
	orderedKeys := []string{canonicalKey, specificKey}
	sort.Strings(orderedKeys)
	for _, key := range orderedKeys {
		mock.ExpectExec(lockSQL).WithArgs(key).WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectCommit()
	if err := db.Transaction(func(tx *gorm.DB) error {
		return repo.LockResources(context.Background(), tx, []ResourceRef{{
			OrganizationID: organizationID, BindingType: BindingTypeWorkflow, ResourceID: bindingID, ParentResourceID: targetAgentID,
		}})
	}); err != nil {
		t.Fatalf("lock workflow binding row: %v", err)
	}

	mock.ExpectBegin()
	mock.ExpectExec(lockSQL).WithArgs(canonicalKey).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := db.Transaction(func(tx *gorm.DB) error {
		return repo.LockResources(context.Background(), tx, []ResourceRef{{
			OrganizationID: organizationID, BindingType: BindingTypeWorkflow, ResourceID: targetAgentID,
		}})
	}); err != nil {
		t.Fatalf("lock workflow delete target: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("workflow save/delete lock keys diverged: %v", err)
	}
}

func TestRevokeLastDatabaseTableCleansDraftAndPublishedParentBindings(t *testing.T) {
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
	repo := NewRepositoryWithTokenSecret(db, "database-parent-secret")

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM "agent_resource_bindings" WHERE organization_id = \$1.*binding_type = \$2 AND resource_id = \$3.*parent_resource_id = \$4`).
		WithArgs(organizationID, BindingTypeDatabaseTable, "table-1", "database-1").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`(?s)DELETE FROM agent_resource_bindings AS parent WHERE parent.organization_id = \$1.*parent.binding_type = \$2.*parent.resource_id = \$3.*NOT EXISTS.*child.agent_id = parent.agent_id.*child.binding_scope = parent.binding_scope.*child.published_version_uuid IS NOT DISTINCT FROM parent.published_version_uuid.*child.binding_type = \$4.*child.parent_resource_id = parent.resource_id`).
		WithArgs(organizationID, BindingTypeDatabase, "database-1", BindingTypeDatabaseTable).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	err = db.Transaction(func(tx *gorm.DB) error {
		return repo.RevokeResource(context.Background(), tx, ResourceRef{
			OrganizationID:   organizationID,
			BindingType:      BindingTypeDatabaseTable,
			ResourceID:       "table-1",
			ParentResourceID: "database-1",
		})
	})
	if err != nil {
		t.Fatalf("RevokeResource(database_table) error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("last-table parent cleanup did not cover draft/published version identity: %v", err)
	}
}

func TestAgentOnlyMoveLocksAgentScopeWithoutExistingBindings(t *testing.T) {
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
	targetWorkspaceID := uuid.New()
	agentID := uuid.New()
	repo := NewRepositoryWithTokenSecret(db, "agent-move-secret")

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "agent_resource_bindings" WHERE organization_id = \$1 AND agent_id IN \(\$2\) AND binding_type <> \$3`).
		WithArgs(organizationID, agentID, BindingTypeSkill).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`)).
		WithArgs(agentScopeLockKey(agentID)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT \* FROM "agent_resource_bindings" WHERE organization_id = \$1 AND agent_id IN \(\$2\) AND binding_type <> \$3`).
		WithArgs(organizationID, agentID, BindingTypeSkill).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec(`UPDATE "agent_resource_bindings" SET .*workspace_id.*WHERE organization_id = \$[0-9]+ AND agent_id IN \(\$[0-9]+\)`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	err = db.Transaction(func(tx *gorm.DB) error {
		_, moveErr := repo.ApplyMoveImpact(context.Background(), tx, MoveImpactRequest{
			OrganizationID: organizationID, TargetWorkspaceID: targetWorkspaceID, MovingAgentIDs: []uuid.UUID{agentID}, ActorID: uuid.New(),
		}, "", time.Now())
		return moveErr
	})
	if err != nil {
		t.Fatalf("ApplyMoveImpact(agent-only) error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("agent-only move did not lock Agent scope before binding relocation: %v", err)
	}
}

func TestExistingBindingMoveLocksResourceBeforeAgentAndDetectsChange(t *testing.T) {
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
	targetWorkspaceID := uuid.New()
	agentID := uuid.New()
	repo := NewRepositoryWithTokenSecret(db, "existing-binding-move-secret")
	query := `SELECT \* FROM "agent_resource_bindings" WHERE organization_id = \$1 AND agent_id IN \(\$2\) AND binding_type <> \$3`
	columns := []string{
		"id", "agent_id", "binding_scope", "organization_id", "workspace_id", "published_version_uuid",
		"binding_type", "resource_id", "parent_resource_id", "display_name", "access_mode",
		"authorized_by", "authorized_at", "created_at", "updated_at",
	}
	now := time.Now()

	mock.ExpectBegin()
	mock.ExpectQuery(query).
		WithArgs(organizationID, agentID, BindingTypeSkill).
		WillReturnRows(sqlmock.NewRows(columns).AddRow(
			uuid.New(), agentID, ScopeDraft, organizationID, organizationID, nil,
			BindingTypeDatabase, "database-1", "", "Database", "read",
			uuid.New(), now, now, now,
		))
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`)).
		WithArgs(resourceLockKey(organizationID, BindingTypeDatabase, "", "database-1")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`)).
		WithArgs(agentScopeLockKey(agentID)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(query).
		WithArgs(organizationID, agentID, BindingTypeSkill).
		WillReturnRows(sqlmock.NewRows(columns))
	mock.ExpectRollback()

	err = db.Transaction(func(tx *gorm.DB) error {
		_, moveErr := repo.ApplyMoveImpact(context.Background(), tx, MoveImpactRequest{
			OrganizationID: organizationID, TargetWorkspaceID: targetWorkspaceID, MovingAgentIDs: []uuid.UUID{agentID}, ActorID: uuid.New(),
		}, "stale-token", now)
		return moveErr
	})
	if !errors.Is(err, ErrImpactChanged) {
		t.Fatalf("ApplyMoveImpact(existing binding race) error = %v, want ErrImpactChanged", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("existing-binding move must lock resource before Agent and abort on changed bindings: %v", err)
	}
}
