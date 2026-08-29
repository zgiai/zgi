package workflow

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/agentbindings"
	"github.com/zgiai/zgi/api/internal/dto"
)

func TestPersistWorkflowWithBindingsUpdatesDraftAndLedgerInOneTransaction(t *testing.T) {
	db, mock := openWorkflowConnectionReferenceTestDB(t)
	organizationID := uuid.MustParse("00000000-0000-4000-8000-000000000010")
	workspaceID := uuid.MustParse("00000000-0000-4000-8000-000000000020")
	agentID := uuid.MustParse("00000000-0000-4000-8000-000000000030")
	workflowID := uuid.MustParse("00000000-0000-4000-8000-000000000040")
	actorID := uuid.MustParse("00000000-0000-4000-8000-000000000050")
	oldConnectionID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	newConnectionID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	now := time.Now()
	workflow := &Workflow{
		ID: workflowID.String(), TenantID: workspaceID.String(), AppID: agentID.String(), AgentID: agentID.String(),
		Type: dto.WorkflowType("workflow"), Version: "draft", Graph: `{"nodes":[]}`, Features: `{}`,
		CreatedBy: actorID.String(), CreatedAt: now, UpdatedAt: now,
		EnvironmentVariables: `[]`, ConversationVariables: `[]`,
	}
	binding := agentbindings.Binding{
		OrganizationID: organizationID, WorkspaceID: workspaceID, BindingInstanceID: "send-new",
		BindingType: agentbindings.BindingTypeIntegrationConnection, ResourceID: newConnectionID.String(),
		ParentResourceID: "wecom", DisplayName: "Send message", AccessMode: "write",
		Metadata: map[string]interface{}{agentbindings.IntegrationAllowedActionIDsMetadataKey: []string{"wecom.message.send_user"}},
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "agent_resource_bindings" WHERE \(agent_id = \$1 AND consumer_type = \$2 AND consumer_id = \$3 AND binding_scope = \$4\) AND published_version_uuid IS NULL`).
		WithArgs(agentID, agentbindings.ConsumerTypeWorkflow, workflowID, agentbindings.ScopeDraft).
		WillReturnRows(sqlmock.NewRows(workflowReferenceBindingColumns()).AddRow(
			uuid.New(), agentID, agentbindings.ConsumerTypeWorkflow, workflowID, "send-old",
			agentbindings.ScopeDraft, organizationID, workspaceID, nil,
			agentbindings.BindingTypeIntegrationConnection, oldConnectionID.String(), "wecom", "Old message", "write",
			`{"allowed_action_ids":["wecom.message.send_user"]}`, actorID, now, now, now,
		))
	for _, connectionID := range []uuid.UUID{oldConnectionID, newConnectionID} {
		mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`)).
			WithArgs("agent-binding-resource:" + organizationID.String() + ":integration_connection::" + connectionID.String()).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectExec(`DELETE FROM "agent_resource_bindings" WHERE \(agent_id = \$1 AND consumer_type = \$2 AND consumer_id = \$3 AND binding_scope = \$4\) AND published_version_uuid IS NULL`).
		WithArgs(agentID, agentbindings.ConsumerTypeWorkflow, workflowID, agentbindings.ScopeDraft).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`INSERT INTO "agent_resource_bindings" .*RETURNING "id"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
	mock.ExpectExec(`UPDATE "workflows" SET .* WHERE id = \$[0-9]+`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)UPDATE workflow_connector_binding_backfills.*WHERE workflow_id = \$1`).
		WithArgs(workflowID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	service := &WorkflowService{repo: NewWorkflowRepository(db)}
	service.configureWorkflowBindingIndex()
	if err := service.persistWorkflowWithBindings(context.Background(), workflow, []agentbindings.Binding{binding}, false); err != nil {
		t.Fatalf("persistWorkflowWithBindings() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("draft graph/index transaction SQL expectations: %v", err)
	}
}

func TestPersistWorkflowWithBindingsDoesNotWriteGraphWhenLedgerWriteFails(t *testing.T) {
	db, mock := openWorkflowConnectionReferenceTestDB(t)
	organizationID := uuid.New()
	workspaceID := uuid.New()
	agentID := uuid.New()
	workflowID := uuid.New()
	actorID := uuid.New()
	connectionID := uuid.New()
	now := time.Now()
	workflow := &Workflow{
		ID: workflowID.String(), TenantID: workspaceID.String(), AppID: agentID.String(), AgentID: agentID.String(),
		Type: dto.WorkflowType("workflow"), Version: "draft", Graph: `{"nodes":[]}`, Features: `{}`,
		CreatedBy: actorID.String(), CreatedAt: now, UpdatedAt: now,
		EnvironmentVariables: `[]`, ConversationVariables: `[]`,
	}
	binding := agentbindings.Binding{
		OrganizationID: organizationID, WorkspaceID: workspaceID, BindingInstanceID: "send",
		BindingType: agentbindings.BindingTypeIntegrationConnection, ResourceID: connectionID.String(),
		ParentResourceID: "wecom", AccessMode: "write",
		Metadata: map[string]interface{}{agentbindings.IntegrationAllowedActionIDsMetadataKey: []string{"wecom.message.send_user"}},
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "agent_resource_bindings" WHERE \(agent_id = \$1 AND consumer_type = \$2 AND consumer_id = \$3 AND binding_scope = \$4\) AND published_version_uuid IS NULL`).
		WithArgs(agentID, agentbindings.ConsumerTypeWorkflow, workflowID, agentbindings.ScopeDraft).
		WillReturnRows(sqlmock.NewRows(workflowReferenceBindingColumns()))
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`)).
		WithArgs("agent-binding-resource:" + organizationID.String() + ":integration_connection::" + connectionID.String()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM "agent_resource_bindings" WHERE \(agent_id = \$1 AND consumer_type = \$2 AND consumer_id = \$3 AND binding_scope = \$4\) AND published_version_uuid IS NULL`).
		WithArgs(agentID, agentbindings.ConsumerTypeWorkflow, workflowID, agentbindings.ScopeDraft).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`INSERT INTO "agent_resource_bindings" .*RETURNING "id"`).
		WillReturnError(context.DeadlineExceeded)
	mock.ExpectRollback()

	service := &WorkflowService{repo: NewWorkflowRepository(db)}
	service.configureWorkflowBindingIndex()
	if err := service.persistWorkflowWithBindings(context.Background(), workflow, []agentbindings.Binding{binding}, false); err == nil {
		t.Fatal("persistWorkflowWithBindings() error = nil, want ledger failure")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("draft graph/index rollback SQL expectations: %v", err)
	}
}

func TestPersistWorkflowWithBindingsRollsBackLedgerWhenWorkflowWasDeleted(t *testing.T) {
	db, mock := openWorkflowConnectionReferenceTestDB(t)
	organizationID := uuid.New()
	workspaceID := uuid.New()
	agentID := uuid.New()
	workflowID := uuid.New()
	actorID := uuid.New()
	connectionID := uuid.New()
	now := time.Now()
	workflow := &Workflow{
		ID: workflowID.String(), TenantID: workspaceID.String(), AppID: agentID.String(), AgentID: agentID.String(),
		Type: dto.WorkflowType("workflow"), Version: "draft", Graph: `{"nodes":[]}`, Features: `{}`,
		CreatedBy: actorID.String(), CreatedAt: now, UpdatedAt: now,
		EnvironmentVariables: `[]`, ConversationVariables: `[]`,
	}
	binding := agentbindings.Binding{
		OrganizationID: organizationID, WorkspaceID: workspaceID, BindingInstanceID: "send",
		BindingType: agentbindings.BindingTypeIntegrationConnection, ResourceID: connectionID.String(),
		ParentResourceID: "wecom", AccessMode: "write",
		Metadata: map[string]interface{}{agentbindings.IntegrationAllowedActionIDsMetadataKey: []string{"wecom.message.send_user"}},
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "agent_resource_bindings" WHERE \(agent_id = \$1 AND consumer_type = \$2 AND consumer_id = \$3 AND binding_scope = \$4\) AND published_version_uuid IS NULL`).
		WithArgs(agentID, agentbindings.ConsumerTypeWorkflow, workflowID, agentbindings.ScopeDraft).
		WillReturnRows(sqlmock.NewRows(workflowReferenceBindingColumns()))
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`)).
		WithArgs("agent-binding-resource:" + organizationID.String() + ":integration_connection::" + connectionID.String()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM "agent_resource_bindings" WHERE \(agent_id = \$1 AND consumer_type = \$2 AND consumer_id = \$3 AND binding_scope = \$4\) AND published_version_uuid IS NULL`).
		WithArgs(agentID, agentbindings.ConsumerTypeWorkflow, workflowID, agentbindings.ScopeDraft).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`INSERT INTO "agent_resource_bindings" .*RETURNING "id"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
	mock.ExpectExec(`UPDATE "workflows" SET .* WHERE id = \$[0-9]+`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	service := &WorkflowService{repo: NewWorkflowRepository(db)}
	service.configureWorkflowBindingIndex()
	err := service.persistWorkflowWithBindings(context.Background(), workflow, []agentbindings.Binding{binding}, false)
	if err == nil || err.Error() != "workflow not found" {
		t.Fatalf("persistWorkflowWithBindings() error = %v, want workflow not found", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("deleted Workflow rollback SQL expectations: %v", err)
	}
}
