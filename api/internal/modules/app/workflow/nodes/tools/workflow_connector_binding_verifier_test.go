package tools

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/agentbindings"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/pkg/database"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func openWorkflowConnectorVerifierDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
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
	previous := database.GetDB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(previous) })
	return db, mock
}

func workflowConnectorVerifierBindingColumns() []string {
	return []string{
		"id", "agent_id", "consumer_type", "consumer_id", "binding_instance_id",
		"binding_scope", "organization_id", "workspace_id", "published_version_uuid",
		"binding_type", "resource_id", "parent_resource_id", "display_name", "access_mode", "metadata",
		"authorized_by", "authorized_at", "created_at", "updated_at",
	}
}

func expectWorkflowConnectorRuntimeScope(
	mock sqlmock.Sqlmock,
	workflowID uuid.UUID,
	agentID uuid.UUID,
	workspaceID uuid.UUID,
	organizationID uuid.UUID,
	found bool,
) {
	expectation := mock.ExpectQuery(`SELECT workflow.id, workflow.agent_id, workflow.tenant_id AS workspace_id, workflow.version, workflow.version_uuid FROM workflows AS workflow JOIN agents AS owner .* JOIN workspaces AS workspace .* WHERE workflow.id = \$[0-9]+ AND workflow.agent_id = \$[0-9]+ AND workflow.tenant_id = \$[0-9]+ AND workspace.organization_id = \$[0-9]+ LIMIT \$[0-9]+`).
		WithArgs("normal", workflowID, agentID, workspaceID, organizationID, 1)
	rows := sqlmock.NewRows([]string{"id", "agent_id", "workspace_id", "version", "version_uuid"})
	if found {
		rows.AddRow(workflowID, agentID, workspaceID, "draft", nil)
	}
	expectation.WillReturnRows(rows)
}

func expectWorkflowConnectorNodeBinding(
	mock sqlmock.Sqlmock,
	workflowID uuid.UUID,
	agentID uuid.UUID,
	workspaceID uuid.UUID,
	organizationID uuid.UUID,
	nodeID string,
	connectionID uuid.UUID,
	actionIDs string,
	found bool,
) {
	expectation := mock.ExpectQuery(`SELECT \* FROM "agent_resource_bindings" WHERE \(agent_id = \$1 AND consumer_type = \$2 AND consumer_id = \$3 AND binding_instance_id = \$4 AND binding_scope = \$5 AND binding_type = \$6 AND resource_id = \$7 AND parent_resource_id = \$8\) AND published_version_uuid IS NULL AND access_mode = \$9 LIMIT \$10`).
		WithArgs(
			agentID,
			agentbindings.ConsumerTypeWorkflow,
			workflowID,
			nodeID,
			agentbindings.ScopeDraft,
			agentbindings.BindingTypeIntegrationConnection,
			connectionID.String(),
			"wecom",
			"write",
			1,
		)
	rows := sqlmock.NewRows(workflowConnectorVerifierBindingColumns())
	if found {
		now := time.Now()
		rows.AddRow(
			uuid.New(), agentID, agentbindings.ConsumerTypeWorkflow, workflowID, nodeID,
			agentbindings.ScopeDraft, organizationID, workspaceID, nil,
			agentbindings.BindingTypeIntegrationConnection, connectionID.String(), "wecom", "Send message", "write",
			`{"allowed_action_ids":`+actionIDs+`}`,
			uuid.New(), now, now, now,
		)
	}
	expectation.WillReturnRows(rows)
}

func TestWorkflowConnectorBindingVerifierRequiresExactNodeConnectionAndAction(t *testing.T) {
	_, mock := openWorkflowConnectorVerifierDB(t)
	workflowID := uuid.New()
	agentID := uuid.New()
	workspaceID := uuid.New()
	organizationID := uuid.New()
	connectionID := uuid.New()
	nodeID := "send-user"
	node := &Node{nodeData: NodeData{ConnectionID: connectionID.String(), ProviderID: "wecom"}, NodeStruct: base.NodeStruct{
		NodeID: nodeID, WorkflowID: workflowID.String(), APPID: agentID.String(),
		WorkspaceID: workspaceID.String(), TenantID: workspaceID.String(), OrganizationID: organizationID.String(),
	}}

	expectWorkflowConnectorRuntimeScope(mock, workflowID, agentID, workspaceID, organizationID, true)
	expectWorkflowConnectorNodeBinding(
		mock, workflowID, agentID, workspaceID, organizationID, nodeID, connectionID,
		`["wecom.message.send_user"]`, true,
	)
	allowed, err := node.workflowConnectorBindingVerifier()(context.Background(), skills.AgentBindingCheck{
		BindingType:      string(agentbindings.BindingTypeIntegrationConnection),
		ResourceID:       connectionID.String(),
		ParentResourceID: "wecom",
		AccessMode:       "write",
		ActionID:         "wecom.message.send_user",
	})
	if err != nil || !allowed {
		t.Fatalf("exact Workflow connector binding allowed = %v, error = %v", allowed, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("exact verifier SQL expectations: %v", err)
	}
}

func TestWorkflowConnectorBindingVerifierDeniesActionOutsidePersistedAllowlist(t *testing.T) {
	_, mock := openWorkflowConnectorVerifierDB(t)
	workflowID := uuid.New()
	agentID := uuid.New()
	workspaceID := uuid.New()
	organizationID := uuid.New()
	connectionID := uuid.New()
	nodeID := "send-user"
	node := &Node{nodeData: NodeData{ConnectionID: connectionID.String(), ProviderID: "wecom"}, NodeStruct: base.NodeStruct{
		NodeID: nodeID, WorkflowID: workflowID.String(), APPID: agentID.String(),
		WorkspaceID: workspaceID.String(), TenantID: workspaceID.String(), OrganizationID: organizationID.String(),
	}}

	expectWorkflowConnectorRuntimeScope(mock, workflowID, agentID, workspaceID, organizationID, true)
	expectWorkflowConnectorNodeBinding(
		mock, workflowID, agentID, workspaceID, organizationID, nodeID, connectionID,
		`["wecom.user.get"]`, true,
	)
	allowed, err := node.workflowConnectorBindingVerifier()(context.Background(), skills.AgentBindingCheck{
		BindingType:      string(agentbindings.BindingTypeIntegrationConnection),
		ResourceID:       connectionID.String(),
		ParentResourceID: "wecom",
		AccessMode:       "write",
		ActionID:         "wecom.message.send_user",
	})
	if err != nil || allowed {
		t.Fatalf("out-of-allowlist Workflow connector action allowed = %v, error = %v", allowed, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("allowlist verifier SQL expectations: %v", err)
	}
}

func TestWorkflowConnectorBindingVerifierDeniesRevokedLedger(t *testing.T) {
	_, mock := openWorkflowConnectorVerifierDB(t)
	workflowID := uuid.New()
	agentID := uuid.New()
	workspaceID := uuid.New()
	organizationID := uuid.New()
	connectionID := uuid.New()
	nodeID := "send-user"
	node := &Node{nodeData: NodeData{ConnectionID: connectionID.String(), ProviderID: "wecom"}, NodeStruct: base.NodeStruct{
		NodeID: nodeID, WorkflowID: workflowID.String(), APPID: agentID.String(),
		WorkspaceID: workspaceID.String(), TenantID: workspaceID.String(), OrganizationID: organizationID.String(),
	}}

	expectWorkflowConnectorRuntimeScope(mock, workflowID, agentID, workspaceID, organizationID, true)
	expectWorkflowConnectorNodeBinding(
		mock, workflowID, agentID, workspaceID, organizationID, nodeID, connectionID,
		`[]`, false,
	)
	allowed, err := node.workflowConnectorBindingVerifier()(context.Background(), skills.AgentBindingCheck{
		BindingType:      string(agentbindings.BindingTypeIntegrationConnection),
		ResourceID:       connectionID.String(),
		ParentResourceID: "wecom",
		AccessMode:       "write",
		ActionID:         "wecom.message.send_user",
	})
	if err != nil || allowed {
		t.Fatalf("revoked Workflow connector binding allowed = %v, error = %v", allowed, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("revoked verifier SQL expectations: %v", err)
	}
}

func TestWorkflowConnectorBindingVerifierDeniesInactiveOwnerScope(t *testing.T) {
	_, mock := openWorkflowConnectorVerifierDB(t)
	workflowID := uuid.New()
	agentID := uuid.New()
	workspaceID := uuid.New()
	organizationID := uuid.New()
	connectionID := uuid.New()
	node := &Node{nodeData: NodeData{ConnectionID: connectionID.String(), ProviderID: "wecom"}, NodeStruct: base.NodeStruct{
		NodeID: "send-user", WorkflowID: workflowID.String(), APPID: agentID.String(),
		WorkspaceID: workspaceID.String(), TenantID: workspaceID.String(), OrganizationID: organizationID.String(),
	}}

	// The joined query yields no row when the Agent is deleted, the Workspace
	// is archived, or the Workspace belongs to a different organization.
	expectWorkflowConnectorRuntimeScope(mock, workflowID, agentID, workspaceID, organizationID, false)
	allowed, err := node.workflowConnectorBindingVerifier()(context.Background(), skills.AgentBindingCheck{
		BindingType:      string(agentbindings.BindingTypeIntegrationConnection),
		ResourceID:       connectionID.String(),
		ParentResourceID: "wecom",
		AccessMode:       "write",
		ActionID:         "wecom.message.send_user",
	})
	if err != nil || allowed {
		t.Fatalf("inactive Workflow owner scope allowed = %v, error = %v", allowed, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("inactive owner verifier SQL expectations: %v", err)
	}
}

func TestWorkflowConnectorBindingVerifierRejectsGraphAndInvocationMismatchBeforeDatabase(t *testing.T) {
	previous := database.GetDB()
	database.SetDB(nil)
	t.Cleanup(func() { database.SetDB(previous) })
	connectionID := uuid.New()
	node := &Node{nodeData: NodeData{ConnectionID: connectionID.String(), ProviderID: "wecom"}}

	allowed, err := node.workflowConnectorBindingVerifier()(context.Background(), skills.AgentBindingCheck{
		BindingType:      string(agentbindings.BindingTypeIntegrationConnection),
		ResourceID:       uuid.NewString(),
		ParentResourceID: "wecom",
		AccessMode:       "write",
		ActionID:         "wecom.message.send_user",
	})
	if err != nil || allowed {
		t.Fatalf("mismatched graph connection allowed = %v, error = %v", allowed, err)
	}
}

func TestWorkflowConnectorBindingVerifierUsesExactPublishedVersion(t *testing.T) {
	_, mock := openWorkflowConnectorVerifierDB(t)
	workflowID := uuid.New()
	versionID := uuid.New()
	agentID := uuid.New()
	workspaceID := uuid.New()
	organizationID := uuid.New()
	connectionID := uuid.New()
	nodeID := "send-published"
	node := &Node{nodeData: NodeData{ConnectionID: connectionID.String(), ProviderID: "wecom"}, NodeStruct: base.NodeStruct{
		NodeID: nodeID, WorkflowID: workflowID.String(), APPID: agentID.String(),
		WorkspaceID: workspaceID.String(), TenantID: workspaceID.String(), OrganizationID: organizationID.String(),
	}}

	mock.ExpectQuery(`SELECT workflow.id, workflow.agent_id, workflow.tenant_id AS workspace_id, workflow.version, workflow.version_uuid FROM workflows AS workflow JOIN agents AS owner .* JOIN workspaces AS workspace .* WHERE workflow.id = \$[0-9]+ AND workflow.agent_id = \$[0-9]+ AND workflow.tenant_id = \$[0-9]+ AND workspace.organization_id = \$[0-9]+ LIMIT \$[0-9]+`).
		WithArgs("normal", workflowID, agentID, workspaceID, organizationID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "agent_id", "workspace_id", "version", "version_uuid"}).
			AddRow(workflowID, agentID, workspaceID, "20260827120000", versionID))
	mock.ExpectQuery(`SELECT \* FROM "agent_resource_bindings" WHERE \(agent_id = \$1 AND consumer_type = \$2 AND consumer_id = \$3 AND binding_instance_id = \$4 AND binding_scope = \$5 AND binding_type = \$6 AND resource_id = \$7 AND parent_resource_id = \$8\) AND published_version_uuid = \$9 AND access_mode = \$10 LIMIT \$11`).
		WithArgs(
			agentID, agentbindings.ConsumerTypeWorkflow, workflowID, nodeID, agentbindings.ScopePublished,
			agentbindings.BindingTypeIntegrationConnection, connectionID.String(), "wecom", versionID, "write", 1,
		).
		WillReturnRows(func() *sqlmock.Rows {
			now := time.Now()
			return sqlmock.NewRows(workflowConnectorVerifierBindingColumns()).AddRow(
				uuid.New(), agentID, agentbindings.ConsumerTypeWorkflow, workflowID, nodeID,
				agentbindings.ScopePublished, organizationID, workspaceID, versionID,
				agentbindings.BindingTypeIntegrationConnection, connectionID.String(), "wecom", "Send published message", "write",
				`{"allowed_action_ids":["wecom.message.send_user"]}`,
				uuid.New(), now, now, now,
			)
		}())

	allowed, err := node.workflowConnectorBindingVerifier()(context.Background(), skills.AgentBindingCheck{
		BindingType:      string(agentbindings.BindingTypeIntegrationConnection),
		ResourceID:       connectionID.String(),
		ParentResourceID: "wecom",
		AccessMode:       "write",
		ActionID:         "wecom.message.send_user",
	})
	if err != nil || !allowed {
		t.Fatalf("published Workflow connector binding allowed = %v, error = %v", allowed, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("published verifier SQL expectations: %v", err)
	}
}
