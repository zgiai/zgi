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

func openWorkflowBindingRepository(t *testing.T) (*Repository, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	return NewRepositoryWithTokenSecret(db, "workflow-binding-secret"), mock
}

func workflowBindingColumns() []string {
	return []string{
		"id", "agent_id", "consumer_type", "consumer_id", "binding_instance_id",
		"binding_scope", "organization_id", "workspace_id", "published_version_uuid",
		"binding_type", "resource_id", "parent_resource_id", "display_name", "access_mode", "metadata",
		"authorized_by", "authorized_at", "created_at", "updated_at",
	}
}

func TestAgentScopeQueriesAndReplacementExcludeWorkflowConsumers(t *testing.T) {
	repo, mock := openWorkflowBindingRepository(t)
	agentID := uuid.New()

	mock.ExpectQuery(`SELECT \* FROM "agent_resource_bindings" WHERE \(agent_id = \$1 AND consumer_type = \$2 AND binding_scope = \$3\) AND published_version_uuid IS NULL ORDER BY binding_type ASC, parent_resource_id ASC, resource_id ASC, access_mode ASC`).
		WithArgs(agentID, ConsumerTypeAgent, ScopeDraft).
		WillReturnRows(sqlmock.NewRows(workflowBindingColumns()))
	bindings, err := repo.ListScope(context.Background(), ScopeRef{AgentID: agentID, Scope: ScopeDraft})
	if err != nil {
		t.Fatalf("ListScope() error = %v", err)
	}
	if len(bindings) != 0 {
		t.Fatalf("ListScope() = %#v, want empty result", bindings)
	}

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM "agent_resource_bindings" WHERE \(agent_id = \$1 AND consumer_type = \$2 AND binding_scope = \$3\) AND published_version_uuid IS NULL`).
		WithArgs(agentID, ConsumerTypeAgent, ScopeDraft).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := repo.ReplaceScope(context.Background(), nil, ScopeRef{AgentID: agentID, Scope: ScopeDraft}, nil); err != nil {
		t.Fatalf("ReplaceScope() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("Agent consumer isolation expectations: %v", err)
	}
}

func TestReplaceWorkflowScopeLocksConnectionAndPersistsNodeIdentity(t *testing.T) {
	repo, mock := openWorkflowBindingRepository(t)
	agentID := uuid.New()
	workflowID := uuid.New()
	organizationID := uuid.New()
	workspaceID := uuid.New()
	connectionID := uuid.NewString()
	ref := WorkflowScopeRef{AgentID: agentID, WorkflowID: workflowID, Scope: ScopeDraft}

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "agent_resource_bindings" WHERE \(agent_id = \$1 AND consumer_type = \$2 AND consumer_id = \$3 AND binding_scope = \$4\) AND published_version_uuid IS NULL`).
		WithArgs(agentID, ConsumerTypeWorkflow, workflowID, ScopeDraft).
		WillReturnRows(sqlmock.NewRows(workflowBindingColumns()))
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`)).
		WithArgs(resourceLockKey(organizationID, BindingTypeIntegrationConnection, "wecom", connectionID)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM "agent_resource_bindings" WHERE \(agent_id = \$1 AND consumer_type = \$2 AND consumer_id = \$3 AND binding_scope = \$4\) AND published_version_uuid IS NULL`).
		WithArgs(agentID, ConsumerTypeWorkflow, workflowID, ScopeDraft).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`INSERT INTO "agent_resource_bindings" .*RETURNING "id"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
	mock.ExpectCommit()

	err := repo.ReplaceWorkflowScope(context.Background(), nil, ref, []Binding{{
		OrganizationID:    organizationID,
		WorkspaceID:       workspaceID,
		BindingInstanceID: "  send-message-node  ",
		BindingType:       BindingTypeIntegrationConnection,
		ResourceID:        "  " + connectionID + "  ",
		ParentResourceID:  " wecom ",
		DisplayName:       " Send message ",
		AccessMode:        " WRITE ",
		Metadata: map[string]interface{}{
			IntegrationAllowedActionIDsMetadataKey: []string{"wecom.message.send"},
		},
	}})
	if err != nil {
		t.Fatalf("ReplaceWorkflowScope() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("workflow scope replacement expectations: %v", err)
	}
}

func TestFindWorkflowNodeBindingUsesExactConsumerAndNode(t *testing.T) {
	repo, mock := openWorkflowBindingRepository(t)
	agentID := uuid.New()
	workflowID := uuid.New()
	organizationID := uuid.New()
	workspaceID := uuid.New()
	connectionID := uuid.NewString()
	now := time.Now()

	mock.ExpectQuery(`SELECT \* FROM "agent_resource_bindings" WHERE \(agent_id = \$1 AND consumer_type = \$2 AND consumer_id = \$3 AND binding_instance_id = \$4 AND binding_scope = \$5 AND binding_type = \$6 AND resource_id = \$7 AND parent_resource_id = \$8\) AND published_version_uuid IS NULL AND access_mode = \$9 LIMIT \$10`).
		WithArgs(agentID, ConsumerTypeWorkflow, workflowID, "node-send", ScopeDraft, BindingTypeIntegrationConnection, connectionID, "wecom", "write", 1).
		WillReturnRows(sqlmock.NewRows(workflowBindingColumns()).AddRow(
			uuid.New(), agentID, ConsumerTypeWorkflow, workflowID, "node-send",
			ScopeDraft, organizationID, workspaceID, nil,
			BindingTypeIntegrationConnection, connectionID, "wecom", "Send message", "write", `{"allowed_action_ids":["wecom.message.send"]}`,
			uuid.New(), now, now, now,
		))

	binding, err := repo.FindWorkflowNodeBinding(context.Background(), WorkflowScopeRef{
		AgentID: agentID, WorkflowID: workflowID, Scope: ScopeDraft,
	}, " node-send ", Match{
		BindingType:      BindingTypeIntegrationConnection,
		ResourceID:       connectionID,
		ParentResourceID: "wecom",
		AccessMode:       "write",
	})
	if err != nil {
		t.Fatalf("FindWorkflowNodeBinding() error = %v", err)
	}
	if binding == nil {
		t.Fatal("FindWorkflowNodeBinding() = nil, want binding")
	}
	reference, ok := binding.AsWorkflowConnectionReference()
	if !ok {
		t.Fatalf("AsWorkflowConnectionReference() rejected %#v", binding)
	}
	if reference.WorkflowID != workflowID || reference.NodeID != "node-send" || reference.NodeTitle != "Send message" ||
		reference.ConnectionID != connectionID || reference.IntegrationID != "wecom" ||
		len(reference.ActionIDs) != 1 || reference.ActionIDs[0] != "wecom.message.send" {
		t.Fatalf("workflow connection reference = %#v", reference)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("workflow node lookup expectations: %v", err)
	}
}

func TestReplaceWorkflowPublishedHeadPreservesOtherRunnableVersions(t *testing.T) {
	repo, mock := openWorkflowBindingRepository(t)
	agentID := uuid.New()
	organizationID := uuid.New()
	workspaceID := uuid.New()

	for index, versionID := range []uuid.UUID{uuid.New(), uuid.New()} {
		workflowID := uuid.New()
		connectionID := uuid.NewString()
		ref := WorkflowScopeRef{
			AgentID: agentID, WorkflowID: workflowID, Scope: ScopePublished, PublishedVersionUUID: &versionID,
		}
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT \* FROM "agent_resource_bindings" WHERE \(agent_id = \$1 AND consumer_type = \$2 AND consumer_id = \$3 AND binding_scope = \$4\) AND published_version_uuid = \$5`).
			WithArgs(agentID, ConsumerTypeWorkflow, workflowID, ScopePublished, versionID).
			WillReturnRows(sqlmock.NewRows(workflowBindingColumns()))
		mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`)).
			WithArgs(resourceLockKey(organizationID, BindingTypeIntegrationConnection, "wecom", connectionID)).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`DELETE FROM "agent_resource_bindings" WHERE \(agent_id = \$1 AND consumer_type = \$2 AND consumer_id = \$3 AND binding_scope = \$4\) AND published_version_uuid = \$5`).
			WithArgs(agentID, ConsumerTypeWorkflow, workflowID, ScopePublished, versionID).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`INSERT INTO "agent_resource_bindings" .*RETURNING "id"`).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
		mock.ExpectCommit()

		err := repo.ReplaceWorkflowPublishedHead(context.Background(), nil, ref, []Binding{{
			OrganizationID: organizationID, WorkspaceID: workspaceID,
			BindingInstanceID: "node-" + string(rune('1'+index)),
			BindingType:       BindingTypeIntegrationConnection,
			ResourceID:        connectionID,
			ParentResourceID:  "wecom",
			AccessMode:        "write",
		}})
		if err != nil {
			t.Fatalf("ReplaceWorkflowPublishedHead(version %d) error = %v", index+1, err)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("published Workflow versions were not independently replaced: %v", err)
	}
}

func TestRevokeAgentConsumersRemovesAgentAndWorkflowRowsTogether(t *testing.T) {
	repo, mock := openWorkflowBindingRepository(t)
	agentID := uuid.New()
	workflowID := uuid.New()
	organizationID := uuid.New()
	workspaceID := uuid.New()
	now := time.Now()
	agentConnectionID := uuid.NewString()
	workflowConnectionID := uuid.NewString()

	rows := sqlmock.NewRows(workflowBindingColumns()).
		AddRow(
			uuid.New(), agentID, ConsumerTypeAgent, agentID, "",
			ScopeDraft, organizationID, workspaceID, nil,
			BindingTypeIntegrationConnection, agentConnectionID, "gmail", "Gmail", "read", `{}`,
			uuid.New(), now, now, now,
		).
		AddRow(
			uuid.New(), agentID, ConsumerTypeWorkflow, workflowID, "node-send",
			ScopeDraft, organizationID, workspaceID, nil,
			BindingTypeIntegrationConnection, workflowConnectionID, "wecom", "Send", "write", `{}`,
			uuid.New(), now, now, now,
		)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "agent_resource_bindings" WHERE agent_id = \$1`).
		WithArgs(agentID).
		WillReturnRows(rows)
	lockKeys := []string{
		resourceLockKey(organizationID, BindingTypeIntegrationConnection, "gmail", agentConnectionID),
		resourceLockKey(organizationID, BindingTypeIntegrationConnection, "wecom", workflowConnectionID),
	}
	sort.Strings(lockKeys)
	for _, key := range lockKeys {
		mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`)).
			WithArgs(key).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`)).
		WithArgs(agentScopeLockKey(agentID)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM "agent_resource_bindings" WHERE agent_id = \$1`).
		WithArgs(agentID).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	if err := repo.RevokeAgentConsumers(context.Background(), nil, agentID); err != nil {
		t.Fatalf("RevokeAgentConsumers() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("Agent-owned consumer revocation expectations: %v", err)
	}
}

func TestListImpactCanSelectOneWorkflowNodeAndScope(t *testing.T) {
	repo, mock := openWorkflowBindingRepository(t)
	organizationID := uuid.New()
	workflowID := uuid.New()
	scope := ScopePublished

	mock.ExpectQuery(`SELECT \* FROM "agent_resource_bindings" WHERE organization_id = \$1 AND \(binding_type = \$2 AND resource_id = \$3\) AND consumer_type = \$4 AND consumer_id = \$5 AND binding_instance_id = \$6 AND binding_scope = \$7 ORDER BY agent_id ASC, binding_scope ASC, published_version_uuid ASC`).
		WithArgs(organizationID, BindingTypeIntegrationConnection, "connection-1", ConsumerTypeWorkflow, workflowID, "node-1", scope).
		WillReturnRows(sqlmock.NewRows(workflowBindingColumns()))

	bindings, err := repo.ListImpact(context.Background(), ResourceRef{
		OrganizationID:    organizationID,
		ConsumerType:      ConsumerTypeWorkflow,
		ConsumerID:        &workflowID,
		BindingInstanceID: "node-1",
		Scope:             &scope,
		BindingType:       BindingTypeIntegrationConnection,
		ResourceID:        "connection-1",
	})
	if err != nil {
		t.Fatalf("ListImpact() error = %v", err)
	}
	if len(bindings) != 0 {
		t.Fatalf("ListImpact() = %#v, want empty result", bindings)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("workflow impact filter expectations: %v", err)
	}
}

func TestPrepareWorkflowBindingsRejectsMissingNodeIdentity(t *testing.T) {
	_, err := prepareWorkflowBindings(WorkflowScopeRef{
		AgentID: uuid.New(), WorkflowID: uuid.New(), Scope: ScopeDraft,
	}, []Binding{{
		OrganizationID: uuid.New(), WorkspaceID: uuid.New(),
		BindingType: BindingTypeIntegrationConnection, ResourceID: uuid.NewString(),
	}}, time.Now())
	if err == nil {
		t.Fatal("prepareWorkflowBindings() accepted a binding without node identity")
	}
}

func TestIntegrationConnectionResourceLockDoesNotDependOnProviderID(t *testing.T) {
	organizationID := uuid.New()
	connectionID := uuid.NewString()
	withProvider := resourceLockKey(organizationID, BindingTypeIntegrationConnection, "wecom", connectionID)
	withoutProvider := resourceLockKey(organizationID, BindingTypeIntegrationConnection, "", connectionID)
	if withProvider != withoutProvider {
		t.Fatalf("integration connection lock keys differ: %q != %q", withProvider, withoutProvider)
	}
}
