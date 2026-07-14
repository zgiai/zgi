package agents

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/agentbindings"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPublishedSuspendedBindingRemainsIndexedAndCanRecover(t *testing.T) {
	row := agentbindings.Binding{
		AgentID:      uuid.New(),
		BindingType:  agentbindings.BindingTypeSkill,
		ResourceID:   "file-reader",
		AccessMode:   "execute",
		BindingScope: agentbindings.ScopeDraft,
	}
	published := publishedAgentBindingRows([]agentbindings.Binding{row})
	if len(published) != 1 || published[0].ResourceID != row.ResourceID {
		t.Fatalf("published rows = %#v, want suspended binding retained", published)
	}

	snapshot := dto.AgentConfigResponse{EnabledSkillIDs: []string{row.ResourceID}}
	snapshot.BindingHealth = dto.AgentBindingHealth{Items: []dto.AgentBindingHealthItem{{
		BindingType: string(row.BindingType), ResourceID: row.ResourceID, AccessMode: row.AccessMode,
		Status: agentBindingStatusSuspended, Reason: agentBindingReasonOrganizationSkillSuspended,
	}}}
	if got := filterAgentConfigByBindingHealth(snapshot).EnabledSkillIDs; len(got) != 0 {
		t.Fatalf("suspended runtime skills = %v, want filtered", got)
	}

	snapshot.BindingHealth = dto.AgentBindingHealth{ActiveCount: 1, Items: []dto.AgentBindingHealthItem{{
		BindingType: string(row.BindingType), ResourceID: row.ResourceID, AccessMode: row.AccessMode,
		Status: agentBindingStatusActive,
	}}}
	got := filterAgentConfigByBindingHealth(snapshot).EnabledSkillIDs
	if len(got) != 1 || got[0] != row.ResourceID {
		t.Fatalf("re-enabled runtime skills = %v, want %q restored from snapshot", got, row.ResourceID)
	}
}

func TestFilterAgentConfigByBindingHealthPreservesLegacySnapshot(t *testing.T) {
	config := dto.AgentConfigResponse{EnabledSkillIDs: []string{"file-reader"}}
	got := filterAgentConfigByBindingHealth(config)
	if len(got.EnabledSkillIDs) != 1 {
		t.Fatalf("legacy skills = %v, want preserved", got.EnabledSkillIDs)
	}
}

func TestPreserveAgentBindingEvidenceOnlyForUnchangedReference(t *testing.T) {
	authorizedBy := uuid.New()
	existing := []agentbindings.Binding{{
		BindingType: agentbindings.BindingTypeDatabaseTable, ResourceID: "table-1", ParentResourceID: "db-1", AccessMode: "read", AuthorizedBy: &authorizedBy,
	}}
	next := []agentbindings.Binding{
		{BindingType: agentbindings.BindingTypeDatabaseTable, ResourceID: "table-1", ParentResourceID: "db-1", AccessMode: "read"},
		{BindingType: agentbindings.BindingTypeDatabaseTable, ResourceID: "table-2", ParentResourceID: "db-1", AccessMode: "read"},
	}
	got := preserveAgentBindingEvidence(next, existing)
	if got[0].AuthorizedBy == nil || *got[0].AuthorizedBy != authorizedBy {
		t.Fatalf("unchanged binding authorized_by = %v, want %s", got[0].AuthorizedBy, authorizedBy)
	}
	if got[1].AuthorizedBy != nil {
		t.Fatalf("new binding authorized_by = %v, want unchanged new evidence", got[1].AuthorizedBy)
	}
}

func TestBindingHealthDoesNotTrustHistoricalWorkflowDisplayName(t *testing.T) {
	agentID := uuid.New()
	workspaceID := uuid.New()
	service := &agentsService{}
	health := service.resolveAgentBindingHealth(context.Background(), &Agent{ID: agentID, TenantID: workspaceID}, uuid.NewString(), &dto.AgentConfigResponse{}, []agentbindings.Binding{{
		BindingType:      agentbindings.BindingTypeWorkflow,
		ResourceID:       "binding-1",
		ParentResourceID: "workflow-agent-1",
		DisplayName:      "historical secret label",
		AccessMode:       "execute",
	}})
	if len(health.Items) != 1 || health.Items[0].DisplayName != "" {
		t.Fatalf("workflow display name = %#v, want historical index label redacted", health.Items)
	}
}

func TestWorkflowBindingIndexUsesTargetAgentIDAsParentResource(t *testing.T) {
	targetAgentID := uuid.NewString()
	config := normalizeAgentWorkflowBindings([]dto.AgentWorkflowBinding{{
		BindingID:  "binding-1",
		AgentID:    targetAgentID,
		WorkflowID: uuid.NewString(),
	}})
	if len(config) != 1 {
		t.Fatalf("normalized workflow bindings = %#v", config)
	}
	row := workflowAgentBindingRow(config[0])
	if row.ResourceID != "binding-1" || row.ParentResourceID != targetAgentID {
		t.Fatalf("workflow index row = %#v, want binding id plus target Agent id", row)
	}
}

func TestResolveSkillBindingHealthDistinguishesOrganizationSuspensionAndMissing(t *testing.T) {
	organizationID := uuid.New()
	workspaceID := uuid.New()
	accountID := uuid.New()
	service := &agentsService{chatRuntimeService: &stubAgentBindingChatRuntimeService{skills: []skills.SkillDiscoveryMetadata{{
		ID:               "chart-generator",
		Enabled:          false,
		Status:           skills.SkillStatusActive,
		SupportedCallers: []string{runtimemodel.ConversationCallerAgent},
	}}}}
	agent := &Agent{ID: uuid.New(), TenantID: workspaceID, AgentsType: "AGENT"}
	config := &dto.AgentConfigResponse{EnabledSkillIDs: []string{"chart-generator", "file-reader"}}
	rows := []agentbindings.Binding{
		{BindingType: agentbindings.BindingTypeSkill, ResourceID: "chart-generator", AccessMode: "execute", OrganizationID: organizationID, AuthorizedBy: &accountID},
		{BindingType: agentbindings.BindingTypeSkill, ResourceID: "file-reader", AccessMode: "execute", OrganizationID: organizationID, AuthorizedBy: &accountID},
	}
	health := service.resolveAgentBindingHealth(context.Background(), agent, accountID.String(), config, rows)
	if health.SuspendedCount != 1 || health.UnavailableCount != 1 || health.Status != agentBindingHealthBlocked {
		t.Fatalf("health = %#v, want one suspended and one unavailable", health)
	}
	if health.Items[0].Reason != agentBindingReasonOrganizationSkillSuspended {
		t.Fatalf("disabled skill reason = %q", health.Items[0].Reason)
	}
	if health.Items[1].Reason != agentBindingReasonResourceDeletedOrMissing {
		t.Fatalf("missing skill reason = %q", health.Items[1].Reason)
	}
}

func TestDraftBindingStateDoesNotRepairIndexOnRead(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	agentID := uuid.New()
	workspaceID := uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "agent_resource_bindings" WHERE (agent_id = $1 AND binding_scope = $2) AND published_version_uuid IS NULL ORDER BY binding_type ASC, parent_resource_id ASC, resource_id ASC, access_mode ASC`)).
		WithArgs(agentID, agentbindings.ScopeDraft).
		WillReturnRows(sqlmock.NewRows([]string{"id", "agent_id", "binding_scope", "organization_id", "workspace_id", "published_version_uuid", "binding_type", "resource_id", "parent_resource_id", "display_name", "access_mode", "authorized_by", "authorized_at", "created_at", "updated_at"}))
	service := &agentsService{db: db, agentBindings: agentbindings.NewRepositoryWithTokenSecret(db, "test")}
	mode := `{"knowledge_dataset_ids":["dataset-1"]}`
	_, _, _, err = service.draftBindingState(context.Background(), &Agent{ID: agentID, TenantID: workspaceID}, &AgentsConfig{AgentsID: agentID, AgentMode: &mode}, uuid.NewString())
	if err != nil {
		t.Fatalf("draftBindingState() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("read path issued an unexpected index write: %v", err)
	}
}

func TestResolveSkillBindingHealthDoesNotDependOnHistoricalAuthorMembership(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	organizationID := uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "chat_runtime_organization_skill_configs" WHERE organization_id = $1`)).
		WithArgs(organizationID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT "enabled" FROM "chat_runtime_organization_skill_configs" WHERE organization_id = \$1 AND skill_id = \$2.*LIMIT \$3`).
		WithArgs(organizationID, skills.SkillFileReader, 1).
		WillReturnRows(sqlmock.NewRows([]string{"enabled"}).AddRow(true))
	service := &agentsService{db: db}
	status, reason := service.resolveSkillBindingHealth(context.Background(), agentbindings.Binding{
		BindingType: agentbindings.BindingTypeSkill, ResourceID: skills.SkillFileReader, OrganizationID: organizationID,
	}, "historical-author-no-longer-a-member")
	if status != agentBindingStatusActive || reason != "" {
		t.Fatalf("skill health = (%q, %q), want active independent of historical author", status, reason)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected membership-dependent query: %v", err)
	}
}

func TestResolveDefaultSystemSkillRemainsActiveWhenOrganizationConfigRowIsMissing(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	organizationID := uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "chat_runtime_organization_skill_configs" WHERE organization_id = $1`)).
		WithArgs(organizationID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT "enabled" FROM "chat_runtime_organization_skill_configs" WHERE organization_id = \$1 AND skill_id = \$2.*LIMIT \$3`).
		WithArgs(organizationID, skills.SkillFileReader, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	service := &agentsService{db: db}
	status, reason := service.resolveSkillBindingHealth(context.Background(), agentbindings.Binding{
		BindingType: agentbindings.BindingTypeSkill, ResourceID: skills.SkillFileReader, OrganizationID: organizationID,
	}, uuid.NewString())
	if status != agentBindingStatusActive || reason != "" {
		t.Fatalf("default system skill health = (%q, %q), want active", status, reason)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestResolveCustomSkillBindingHealthUsesCatalogRecordAndOrganizationPolicy(t *testing.T) {
	tests := []struct {
		name       string
		status     string
		policy     *bool
		wantStatus string
		wantReason string
	}{
		{name: "active", status: "active", policy: boolPtr(true), wantStatus: agentBindingStatusActive},
		{name: "organization suspended", status: "active", policy: boolPtr(false), wantStatus: agentBindingStatusSuspended, wantReason: agentBindingReasonOrganizationSkillSuspended},
		{name: "invalid", status: "invalid", policy: boolPtr(true), wantStatus: agentBindingStatusUnavailable, wantReason: agentBindingReasonResourceDeletedOrMissing},
		{name: "deleted", status: "", policy: nil, wantStatus: agentBindingStatusUnavailable, wantReason: agentBindingReasonResourceDeletedOrMissing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sqlDB, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New() error = %v", err)
			}
			t.Cleanup(func() { _ = sqlDB.Close() })
			db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
			if err != nil {
				t.Fatalf("gorm.Open() error = %v", err)
			}
			organizationID := uuid.New()
			customQuery := `SELECT "status" FROM "chat_runtime_custom_skills" WHERE .*organization_id = \$1 AND skill_id = \$2 AND deleted_at IS NULL.*LIMIT \$3`
			if tt.status == "" {
				mock.ExpectQuery(customQuery).WithArgs(organizationID, "custom-tool", 1).WillReturnError(gorm.ErrRecordNotFound)
			} else {
				mock.ExpectQuery(customQuery).WithArgs(organizationID, "custom-tool", 1).
					WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow(tt.status))
			}
			if tt.status == "active" {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "chat_runtime_organization_skill_configs" WHERE organization_id = $1`)).
					WithArgs(organizationID).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`SELECT "enabled" FROM "chat_runtime_organization_skill_configs" WHERE organization_id = \$1 AND skill_id = \$2.*LIMIT \$3`).
					WithArgs(organizationID, "custom-tool", 1).
					WillReturnRows(sqlmock.NewRows([]string{"enabled"}).AddRow(*tt.policy))
			}
			service := &agentsService{db: db}
			status, reason := service.resolveSkillBindingHealth(context.Background(), agentbindings.Binding{
				BindingType: agentbindings.BindingTypeSkill, ResourceID: "custom-tool", OrganizationID: organizationID,
			}, uuid.NewString())
			if status != tt.wantStatus || reason != tt.wantReason {
				t.Fatalf("custom skill health = (%q, %q), want (%q, %q)", status, reason, tt.wantStatus, tt.wantReason)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("expectations: %v", err)
			}
		})
	}
}

func TestDatabaseBindingValidationDeltaOnlyIncludesNewAccess(t *testing.T) {
	previous := dto.AgentDatabaseBinding{
		DataSourceID:     "db-1",
		TableIDs:         []string{"removed", "kept-read", "kept-write", "upgrade"},
		WritableTableIDs: []string{"kept-write"},
	}
	next := dto.AgentDatabaseBinding{
		DataSourceID:     "db-1",
		TableIDs:         []string{"kept-read", "kept-write", "upgrade", "new-read", "new-write"},
		WritableTableIDs: []string{"kept-write", "upgrade", "new-write"},
	}
	delta := databaseBindingValidationDelta(previous, next, true)
	if got, want := delta.TableIDs, []string{"new-read", "new-write", "upgrade"}; !stringSlicesEqual(got, want) {
		t.Fatalf("delta tables = %v, want %v", got, want)
	}
	if got, want := delta.WritableTableIDs, []string{"new-write", "upgrade"}; !stringSlicesEqual(got, want) {
		t.Fatalf("delta writable tables = %v, want %v", got, want)
	}

	downgrade := databaseBindingValidationDelta(
		dto.AgentDatabaseBinding{DataSourceID: "db-1", TableIDs: []string{"table-1"}, WritableTableIDs: []string{"table-1"}},
		dto.AgentDatabaseBinding{DataSourceID: "db-1", TableIDs: []string{"table-1"}},
		true,
	)
	if len(downgrade.TableIDs) != 0 {
		t.Fatalf("write-to-read downgrade delta = %#v, want empty", downgrade)
	}
}

func TestCreateAgentPublishedVersionRechecksRevisionAfterConfigLock(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	agentID := uuid.New()
	workspaceID := uuid.New()
	configID := uuid.New()
	versionID := uuid.New()
	now := time.Now()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`)).
		WithArgs("agent-binding-agent:" + agentID.String()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT "tenant_id" FROM "agents" WHERE id = $1 AND deleted_at IS NULL LIMIT $2`)).
		WithArgs(agentID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id"}).AddRow(workspaceID))
	mock.ExpectQuery(`SELECT \* FROM "agents_configs" WHERE deleted_at IS NULL AND id = \$1.*LIMIT \$2 FOR UPDATE`).
		WithArgs(configID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "agents_id", "agent_mode", "created_at", "updated_at"}).
			AddRow(configID, agentID, `{}`, now, now))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "agent_resource_bindings" WHERE (agent_id = $1 AND binding_scope = $2) AND published_version_uuid IS NULL ORDER BY binding_type ASC, parent_resource_id ASC, resource_id ASC, access_mode ASC`)).
		WithArgs(agentID, agentbindings.ScopeDraft).
		WillReturnRows(sqlmock.NewRows([]string{"id", "agent_id", "binding_scope", "organization_id", "workspace_id", "published_version_uuid", "binding_type", "resource_id", "parent_resource_id", "display_name", "access_mode", "authorized_by", "authorized_at", "created_at", "updated_at"}))
	mock.ExpectRollback()

	service := &agentsService{db: db, agentBindings: agentbindings.NewRepositoryWithTokenSecret(db, "test")}
	staleRevision := agentBindingRevision([]agentbindings.Binding{{
		BindingType: agentbindings.BindingTypeSkill,
		ResourceID:  skills.SkillFileReader,
		AccessMode:  "execute",
	}})
	err = service.createAgentPublishedVersion(
		context.Background(),
		&AgentPublishedVersion{AgentID: agentID, WorkspaceID: workspaceID, Version: "v1", VersionUUID: versionID, CreatedAt: now},
		&Agent{ID: agentID, TenantID: workspaceID},
		&AgentsConfig{ID: configID, AgentsID: agentID},
		uuid.NewString(),
		staleRevision,
		false,
		nil,
	)
	var bindingErr *agentBindingAPIError
	if !errors.As(err, &bindingErr) || bindingErr.Code != agentBindingRevisionConflictCode {
		t.Fatalf("createAgentPublishedVersion() error = %#v, want %s", err, agentBindingRevisionConflictCode)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("publish must lock and re-read before any snapshot insert: %v", err)
	}
}

func TestRollbackRechecksImpactAfterResourceAndConfigLocks(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	agentID := uuid.New()
	organizationID := uuid.New()
	configID := uuid.New()
	versionID := uuid.New()
	versionUUID := uuid.New()
	actorID := uuid.New()
	now := time.Now()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`)).
		WithArgs("agent-binding-resource:" + organizationID.String() + ":skill::custom-tool").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`)).
		WithArgs("agent-binding-agent:" + agentID.String()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT "tenant_id" FROM "agents" WHERE id = $1 AND deleted_at IS NULL LIMIT $2`)).
		WithArgs(agentID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id"}).AddRow(organizationID))
	mock.ExpectQuery(`SELECT \* FROM "agents_configs" WHERE deleted_at IS NULL AND id = \$1.*LIMIT \$2 FOR UPDATE`).
		WithArgs(configID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "agents_id", "agent_mode", "created_at", "updated_at"}).
			AddRow(configID, agentID, `{}`, now, now))
	mock.ExpectQuery(`SELECT \* FROM "agent_published_versions" WHERE id = \$1 AND agent_id = \$2 AND deleted_at IS NULL.*LIMIT \$3`).
		WithArgs(versionID.String(), agentID.String(), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "agent_id", "workspace_id", "version", "version_uuid", "config_snapshot", "description", "created_by", "created_at", "deleted_at"}).
			AddRow(versionID, agentID, organizationID, "v1", versionUUID, []byte(`{"enabled_skill_ids":["custom-tool"]}`), "", actorID, now, nil))
	mock.ExpectQuery(`SELECT "status" FROM "chat_runtime_custom_skills" WHERE .*organization_id = \$1 AND skill_id = \$2 AND deleted_at IS NULL.*LIMIT \$3`).
		WithArgs(organizationID, "custom-tool", 1).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectRollback()

	service := &agentsService{db: db, agentBindings: agentbindings.NewRepositoryWithTokenSecret(db, "rollback-secret")}
	_, err = service.rollbackAgentPublishedVersionCAS(
		context.Background(),
		&Agent{ID: agentID, TenantID: organizationID},
		&AgentsConfig{ID: configID, AgentsID: agentID, UpdatedAt: now},
		actorID,
		actorID.String(),
		dto.RollbackAgentPublishedVersionRequest{VersionID: versionID.String(), BindingAction: "remove_all_abnormal", ImpactToken: "stale-token"},
		[]agentbindings.Binding{{OrganizationID: organizationID, BindingType: agentbindings.BindingTypeSkill, ResourceID: "custom-tool", AccessMode: "execute"}},
	)
	var bindingErr *agentBindingAPIError
	if !errors.As(err, &bindingErr) || bindingErr.Code != agentRollbackImpactChangedCode {
		t.Fatalf("rollback error = %#v, want %s", err, agentRollbackImpactChangedCode)
	}
	preview, ok := bindingErr.Data.(*dto.AgentRollbackPreviewResponse)
	if !ok || preview.BindingHealth.UnavailableCount != 1 || len(preview.RemovedBindings) != 1 || preview.ImpactToken == "" {
		t.Fatalf("changed preview = %#v, want fresh unavailable impact", bindingErr.Data)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("rollback must lock then recompute impact before writing: %v", err)
	}
}

func TestUpdateLocksResourceBeforeRevalidatingNewBinding(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	agentID := uuid.New()
	organizationID := uuid.New()
	configID := uuid.New()
	accountID := uuid.New()
	now := time.Now()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`)).
		WithArgs("agent-binding-resource:" + organizationID.String() + ":skill::" + skills.SkillFileReader).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`)).
		WithArgs("agent-binding-agent:" + agentID.String()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT "tenant_id" FROM "agents" WHERE id = $1 AND deleted_at IS NULL LIMIT $2`)).
		WithArgs(agentID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id"}).AddRow(organizationID))
	mock.ExpectQuery(`SELECT \* FROM "agents_configs" WHERE deleted_at IS NULL AND id = \$1.*LIMIT \$2 FOR UPDATE`).
		WithArgs(configID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "agents_id", "agent_mode", "created_at", "updated_at"}).
			AddRow(configID, agentID, `{}`, now, now))
	mock.ExpectRollback()

	service := &agentsService{
		db:                 db,
		agentBindings:      agentbindings.NewRepositoryWithTokenSecret(db, "update-secret"),
		chatRuntimeService: &stubAgentBindingChatRuntimeService{skills: []skills.SkillDiscoveryMetadata{}},
	}
	_, _, err = service.updateAgentConfigCAS(
		context.Background(),
		&Agent{ID: agentID, TenantID: organizationID},
		&AgentsConfig{ID: configID, AgentsID: agentID, UpdatedAt: now},
		agentBindingRevision(nil),
		dto.AgentConfigRequest{EnabledSkillIDs: []string{skills.SkillFileReader}},
		accountID.String(),
	)
	if err == nil || !regexp.MustCompile(`skill .* is not available`).MatchString(err.Error()) {
		t.Fatalf("updateAgentConfigCAS() error = %v, want locked post-delete validation failure", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("new binding must take resource lock before revalidation and write: %v", err)
	}
}

func TestUpdateRejectsBindingSaveAfterAgentOnlyMove(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	agentID := uuid.New()
	oldWorkspaceID := uuid.New()
	newWorkspaceID := uuid.New()
	configID := uuid.New()
	now := time.Now()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`)).
		WithArgs("agent-binding-agent:" + agentID.String()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT "tenant_id" FROM "agents" WHERE id = $1 AND deleted_at IS NULL LIMIT $2`)).
		WithArgs(agentID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id"}).AddRow(newWorkspaceID))
	mock.ExpectRollback()

	service := &agentsService{
		db:            db,
		agentBindings: agentbindings.NewRepositoryWithTokenSecret(db, "move-race-secret"),
	}
	_, _, err = service.updateAgentConfigCAS(
		context.Background(),
		&Agent{ID: agentID, TenantID: oldWorkspaceID},
		&AgentsConfig{ID: configID, AgentsID: agentID, UpdatedAt: now},
		agentBindingRevision(nil),
		dto.AgentConfigRequest{},
		uuid.NewString(),
	)
	var bindingErr *agentBindingAPIError
	if !errors.As(err, &bindingErr) || bindingErr.Code != agentBindingRevisionConflictCode {
		t.Fatalf("updateAgentConfigCAS() error = %#v, want workspace revision conflict", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("binding save must stop after an Agent-only move changes the workspace: %v", err)
	}
}

func boolPtr(value bool) *bool {
	return &value
}

func stringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}
