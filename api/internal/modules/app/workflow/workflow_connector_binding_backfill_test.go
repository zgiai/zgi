package workflow

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/agentbindings"
	"github.com/zgiai/zgi/api/internal/dto"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

type workflowBindingBackfillOrganizationService struct {
	interfaces.OrganizationService
	organizationID string
}

func (s workflowBindingBackfillOrganizationService) GetOrganizationByWorkspaceID(context.Context, string) (*workspacemodel.Organization, error) {
	return &workspacemodel.Organization{ID: s.organizationID}, nil
}

type workflowBindingBackfillTool struct {
	entity tools.ToolEntity
}

func (t *workflowBindingBackfillTool) GetEntity() tools.ToolEntity { return t.entity }
func (*workflowBindingBackfillTool) GetProviderType() tools.ToolProviderType {
	return tools.ToolProviderTypeConnector
}
func (*workflowBindingBackfillTool) GetTenantID() string { return "" }
func (*workflowBindingBackfillTool) Invoke(context.Context, string, map[string]interface{}, *string, *string, *string) ([]tools.ToolInvokeMessage, error) {
	return nil, nil
}
func (*workflowBindingBackfillTool) GetRuntimeParameters(context.Context, *string, *string, *string) ([]tools.ToolParameter, error) {
	return nil, nil
}
func (t *workflowBindingBackfillTool) ForkToolRuntime(*tools.ToolRuntime) tools.Tool { return t }
func (*workflowBindingBackfillTool) ValidateCredentials(context.Context, map[string]interface{}) error {
	return nil
}

type workflowBindingBackfillProvider struct {
	tool *workflowBindingBackfillTool
}

func (p *workflowBindingBackfillProvider) GetEntity() tools.ToolProviderEntity {
	return tools.ToolProviderEntity{
		Identity:     tools.ToolProviderIdentity{Name: "wecom"},
		ProviderType: tools.ToolProviderTypeConnector,
		Tools:        []tools.ToolEntity{p.tool.entity},
	}
}
func (*workflowBindingBackfillProvider) GetProviderType() tools.ToolProviderType {
	return tools.ToolProviderTypeConnector
}
func (p *workflowBindingBackfillProvider) GetTool(name string) (tools.Tool, error) {
	if name != p.tool.entity.Identity.Name {
		return nil, tools.ErrToolNotFound
	}
	return p.tool, nil
}
func (p *workflowBindingBackfillProvider) GetTools() []tools.Tool { return []tools.Tool{p.tool} }
func (*workflowBindingBackfillProvider) ValidateCredentials(context.Context, map[string]interface{}) error {
	return nil
}

func TestReconcileWorkflowConnectorBindingBackfillsIndexesPublishedWorkflow(t *testing.T) {
	db, mock := openWorkflowConnectionReferenceTestDB(t)
	organizationID := uuid.New()
	workspaceID := uuid.New()
	agentID := uuid.New()
	workflowID := uuid.New()
	versionID := uuid.New()
	connectionID := uuid.New()
	actorID := uuid.New()
	now := time.Now()
	graph := `{"nodes":[{"id":"lookup-user","data":{"provider_type":"connector","provider_id":"wecom","tool_name":"get_wecom_user","connection_id":"` + connectionID.String() + `","title":"Lookup user"}}]}`

	tool := &workflowBindingBackfillTool{entity: tools.ToolEntity{
		Identity:         tools.ToolIdentity{Name: "get_wecom_user"},
		InputSchema:      map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		OutputSchema:     map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		SupportedCallers: []tools.ToolInvokeFrom{tools.ToolInvokeFromWorkflow},
		Governance:       &tools.ToolGovernanceMetadata{ToolID: "wecom.user.get", Effect: "read", RiskLevel: "low"},
	}}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(&workflowBindingBackfillProvider{tool: tool}); err != nil {
		t.Fatalf("register connector provider: %v", err)
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT workflow_id.*FROM workflow_connector_binding_backfills.*FOR UPDATE SKIP LOCKED`).
		WillReturnRows(sqlmock.NewRows([]string{"workflow_id"}).AddRow(workflowID))
	mock.ExpectQuery(`SELECT \* FROM "workflows" WHERE id = \$1 LIMIT \$2 FOR UPDATE`).
		WithArgs(workflowID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "app_id", "agent_id", "type", "version", "version_uuid", "graph", "features",
			"created_by", "created_at", "updated_by", "updated_at", "environment_variables", "conversation_variables", "internal",
		}).AddRow(
			workflowID, workspaceID, agentID, agentID, dto.WorkflowTypeWorkflow, "2026-08-27", versionID, graph, `{}`,
			actorID, now, nil, now, `[]`, `[]`, false,
		))
	mock.ExpectExec(`SAVEPOINT workflow_connector_binding_backfill_write`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT \* FROM "agent_resource_bindings" WHERE \(agent_id = \$1 AND consumer_type = \$2 AND consumer_id = \$3 AND binding_scope = \$4\) AND published_version_uuid = \$5`).
		WithArgs(agentID, agentbindings.ConsumerTypeWorkflow, workflowID, agentbindings.ScopePublished, versionID).
		WillReturnRows(sqlmock.NewRows(workflowReferenceBindingColumns()))
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`)).
		WithArgs("agent-binding-resource:" + organizationID.String() + ":integration_connection::" + connectionID.String()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM "agent_resource_bindings" WHERE \(agent_id = \$1 AND consumer_type = \$2 AND consumer_id = \$3 AND binding_scope = \$4\) AND published_version_uuid = \$5`).
		WithArgs(agentID, agentbindings.ConsumerTypeWorkflow, workflowID, agentbindings.ScopePublished, versionID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`INSERT INTO "agent_resource_bindings" .*RETURNING "id"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
	mock.ExpectExec(`(?s)UPDATE workflow_connector_binding_backfills.*WHERE workflow_id = \$1`).
		WithArgs(workflowID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT workflow_id.*FROM workflow_connector_binding_backfills.*FOR UPDATE SKIP LOCKED`).
		WillReturnRows(sqlmock.NewRows([]string{"workflow_id"}))
	mock.ExpectCommit()

	service := &WorkflowService{
		repo:              NewWorkflowRepository(db),
		workflowDB:        db,
		workflowBindings:  agentbindings.NewRepository(db),
		enterpriseService: workflowBindingBackfillOrganizationService{organizationID: organizationID.String()},
		executor: NewWorkflowExecutorWithRuntimeDeps(WorkflowExecutorDeps{
			ToolEngine: tools.NewToolEngine(manager),
		}),
	}
	if err := service.ReconcileWorkflowConnectorBindingBackfills(context.Background()); err != nil {
		t.Fatalf("ReconcileWorkflowConnectorBindingBackfills() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("backfill SQL expectations: %v", err)
	}
}

func TestReconcileWorkflowConnectorBindingBackfillsRetainsFailureForRetry(t *testing.T) {
	db, mock := openWorkflowConnectionReferenceTestDB(t)
	organizationID := uuid.New()
	workspaceID := uuid.New()
	agentID := uuid.New()
	workflowID := uuid.New()
	connectionID := uuid.New()
	actorID := uuid.New()
	now := time.Now()
	graph := `{"nodes":[{"id":"lookup-user","data":{"provider_type":"connector","provider_id":"wecom","tool_name":"get_wecom_user","connection_id":"` + connectionID.String() + `"}}]}`

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT workflow_id.*FROM workflow_connector_binding_backfills.*FOR UPDATE SKIP LOCKED`).
		WillReturnRows(sqlmock.NewRows([]string{"workflow_id"}).AddRow(workflowID))
	mock.ExpectQuery(`SELECT \* FROM "workflows" WHERE id = \$1 LIMIT \$2 FOR UPDATE`).
		WithArgs(workflowID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "app_id", "agent_id", "type", "version", "version_uuid", "graph", "features",
			"created_by", "created_at", "updated_by", "updated_at", "environment_variables", "conversation_variables", "internal",
		}).AddRow(
			workflowID, workspaceID, agentID, agentID, dto.WorkflowTypeWorkflow, "draft", nil, graph, `{}`,
			actorID, now, nil, now, `[]`, `[]`, false,
		))
	mock.ExpectExec(`(?s)UPDATE workflow_connector_binding_backfills.*SET attempts = attempts \+ 1.*WHERE workflow_id = \$2`).
		WithArgs(sqlmock.AnyArg(), workflowID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT workflow_id.*FROM workflow_connector_binding_backfills.*FOR UPDATE SKIP LOCKED`).
		WillReturnRows(sqlmock.NewRows([]string{"workflow_id"}))
	mock.ExpectCommit()

	service := &WorkflowService{
		repo:              NewWorkflowRepository(db),
		workflowDB:        db,
		workflowBindings:  agentbindings.NewRepository(db),
		enterpriseService: workflowBindingBackfillOrganizationService{organizationID: organizationID.String()},
	}
	err := service.ReconcileWorkflowConnectorBindingBackfills(context.Background())
	if err == nil || !strings.Contains(err.Error(), "remain pending") {
		t.Fatalf("ReconcileWorkflowConnectorBindingBackfills() error = %v, want retained pending item", err)
	}
	var incomplete interface {
		error
		WorkflowConnectorBindingBackfillIncomplete() bool
	}
	if !errors.As(err, &incomplete) || !incomplete.WorkflowConnectorBindingBackfillIncomplete() {
		t.Fatalf("ReconcileWorkflowConnectorBindingBackfills() error = %T, want non-fatal incomplete marker", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("failed backfill SQL expectations: %v", err)
	}
}

func TestReconcileWorkflowConnectorBindingBackfillsTreatsMissingSchemaAsFatal(t *testing.T) {
	db, mock := openWorkflowConnectionReferenceTestDB(t)
	schemaErr := errors.New(`ERROR: relation "workflow_connector_binding_backfills" does not exist (SQLSTATE 42P01)`)

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT workflow_id.*FROM workflow_connector_binding_backfills.*FOR UPDATE SKIP LOCKED`).
		WillReturnError(schemaErr)
	mock.ExpectRollback()

	service := &WorkflowService{
		workflowDB:       db,
		workflowBindings: agentbindings.NewRepository(db),
	}
	err := service.ReconcileWorkflowConnectorBindingBackfills(context.Background())
	if err == nil || !errors.Is(err, schemaErr) {
		t.Fatalf("ReconcileWorkflowConnectorBindingBackfills() error = %v, want schema failure", err)
	}
	var incomplete interface {
		error
		WorkflowConnectorBindingBackfillIncomplete() bool
	}
	if errors.As(err, &incomplete) {
		t.Fatalf("ReconcileWorkflowConnectorBindingBackfills() error = %T, schema failure must abort startup", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("schema failure SQL expectations: %v", err)
	}
}

func TestReconcileWorkflowConnectorBindingBackfillsTreatsBindingRepositoryFailureAsFatal(t *testing.T) {
	db, mock := openWorkflowConnectionReferenceTestDB(t)
	workflowID := uuid.New()
	workspaceID := uuid.New()
	agentID := uuid.New()
	actorID := uuid.New()
	now := time.Now()
	schemaErr := errors.New(`ERROR: column "consumer_type" does not exist (SQLSTATE 42703)`)

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT workflow_id.*FROM workflow_connector_binding_backfills.*FOR UPDATE SKIP LOCKED`).
		WillReturnRows(sqlmock.NewRows([]string{"workflow_id"}).AddRow(workflowID))
	mock.ExpectQuery(`SELECT \* FROM "workflows" WHERE id = \$1 LIMIT \$2 FOR UPDATE`).
		WithArgs(workflowID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "app_id", "agent_id", "type", "version", "version_uuid", "graph", "features",
			"created_by", "created_at", "updated_by", "updated_at", "environment_variables", "conversation_variables", "internal",
		}).AddRow(
			workflowID, workspaceID, agentID, agentID, dto.WorkflowTypeWorkflow, "draft", nil, `{"nodes":[]}`, `{}`,
			actorID, now, nil, now, `[]`, `[]`, false,
		))
	mock.ExpectExec(`SAVEPOINT workflow_connector_binding_backfill_write`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT \* FROM "agent_resource_bindings" WHERE \(agent_id = \$1 AND consumer_type = \$2 AND consumer_id = \$3 AND binding_scope = \$4\) AND published_version_uuid IS NULL`).
		WithArgs(agentID, agentbindings.ConsumerTypeWorkflow, workflowID, agentbindings.ScopeDraft).
		WillReturnError(schemaErr)
	mock.ExpectExec(`ROLLBACK TO SAVEPOINT workflow_connector_binding_backfill_write`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	service := &WorkflowService{
		workflowDB:       db,
		workflowBindings: agentbindings.NewRepository(db),
	}
	err := service.ReconcileWorkflowConnectorBindingBackfills(context.Background())
	if err == nil || !errors.Is(err, schemaErr) {
		t.Fatalf("ReconcileWorkflowConnectorBindingBackfills() error = %v, want binding repository schema failure", err)
	}
	var incomplete interface {
		error
		WorkflowConnectorBindingBackfillIncomplete() bool
	}
	if errors.As(err, &incomplete) {
		t.Fatalf("ReconcileWorkflowConnectorBindingBackfills() error = %T, binding repository failure must abort startup", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("binding repository failure SQL expectations: %v", err)
	}
}
