package workflow

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/agentbindings"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func openWorkflowConnectionReferenceTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
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
	return db, mock
}

func TestEnsureConnectionReferenceIndexReady(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		pending   int64
		wantError bool
	}{
		{name: "complete", pending: 0},
		{name: "pending historical workflow", pending: 2, wantError: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			db, mock := openWorkflowConnectionReferenceTestDB(t)
			organizationID := uuid.New()
			mock.ExpectQuery(`(?s)SELECT count\(\*\) FROM workflow_connector_binding_backfills AS backfill.*workspace\.organization_id = \$1`).
				WithArgs(organizationID).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(testCase.pending))

			service := &WorkflowService{workflowDB: db}
			err := service.EnsureConnectionReferenceIndexReady(context.Background(), organizationID)
			if (err != nil) != testCase.wantError {
				t.Fatalf("EnsureConnectionReferenceIndexReady() error = %v, wantError %v", err, testCase.wantError)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet SQL expectations: %v", err)
			}
		})
	}
}

func workflowReferenceBindingColumns() []string {
	return []string{
		"id", "agent_id", "consumer_type", "consumer_id", "binding_instance_id",
		"binding_scope", "organization_id", "workspace_id", "published_version_uuid",
		"binding_type", "resource_id", "parent_resource_id", "display_name", "access_mode", "metadata",
		"authorized_by", "authorized_at", "created_at", "updated_at",
	}
}

func workflowReferenceRows(
	organizationID uuid.UUID,
	workspaceID uuid.UUID,
	agentID uuid.UUID,
	draftWorkflowID uuid.UUID,
	publishedWorkflowID uuid.UUID,
	publishedVersionID uuid.UUID,
	connectionID uuid.UUID,
) *sqlmock.Rows {
	now := time.Now()
	metadata := `{"allowed_action_ids":["wecom.message.send_user"]}`
	return sqlmock.NewRows(workflowReferenceBindingColumns()).
		AddRow(
			uuid.New(), agentID, agentbindings.ConsumerTypeWorkflow, draftWorkflowID, "send-draft",
			agentbindings.ScopeDraft, organizationID, workspaceID, nil,
			agentbindings.BindingTypeIntegrationConnection, connectionID.String(), "wecom", "Send draft message", "write", metadata,
			uuid.New(), now, now, now,
		).
		AddRow(
			uuid.New(), agentID, agentbindings.ConsumerTypeWorkflow, publishedWorkflowID, "send-published",
			agentbindings.ScopePublished, organizationID, workspaceID, publishedVersionID,
			agentbindings.BindingTypeIntegrationConnection, connectionID.String(), "wecom", "Send published message", "write", metadata,
			uuid.New(), now, now, now,
		)
}

func workflowReferenceImpactQuery() string {
	return `SELECT \* FROM "agent_resource_bindings" WHERE organization_id = \$1 AND \(binding_type = \$2 AND resource_id = \$3\) AND consumer_type = \$4 ORDER BY agent_id ASC, binding_scope ASC, published_version_uuid ASC`
}

func TestPreviewConnectionReferencesReturnsDraftAndPublishedNodes(t *testing.T) {
	db, mock := openWorkflowConnectionReferenceTestDB(t)
	organizationID := uuid.New()
	workspaceID := uuid.New()
	agentID := uuid.New()
	draftWorkflowID := uuid.New()
	publishedWorkflowID := uuid.New()
	publishedVersionID := uuid.New()
	connectionID := uuid.New()

	mock.ExpectQuery(workflowReferenceImpactQuery()).
		WithArgs(organizationID, agentbindings.BindingTypeIntegrationConnection, connectionID.String(), agentbindings.ConsumerTypeWorkflow).
		WillReturnRows(workflowReferenceRows(
			organizationID, workspaceID, agentID, draftWorkflowID, publishedWorkflowID, publishedVersionID, connectionID,
		))

	service := &WorkflowService{workflowBindings: agentbindings.NewRepository(db)}
	references, err := service.PreviewConnectionReferences(context.Background(), organizationID, connectionID)
	if err != nil {
		t.Fatalf("PreviewConnectionReferences() error = %v", err)
	}
	if len(references) != 2 {
		t.Fatalf("PreviewConnectionReferences() returned %d references, want 2", len(references))
	}
	if references[0].Scope != agentbindings.ScopeDraft || references[1].Scope != agentbindings.ScopePublished {
		t.Fatalf("reference scopes = %#v", references)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("preview SQL expectations: %v", err)
	}
}

func TestDetachConnectionReferencesClearsOnlyDraftAndRevokesEveryScope(t *testing.T) {
	db, mock := openWorkflowConnectionReferenceTestDB(t)
	organizationID := uuid.New()
	workspaceID := uuid.New()
	agentID := uuid.New()
	draftWorkflowID := uuid.New()
	publishedWorkflowID := uuid.New()
	publishedVersionID := uuid.New()
	connectionID := uuid.New()
	otherConnectionID := uuid.New()
	actorID := uuid.New()
	now := time.Now()
	draftGraph := `{"nodes":[{"id":"send-draft","data":{"provider_type":"connector","provider_id":"wecom","tool_name":"send_wecom_user_message","connection_id":"` + connectionID.String() + `"}},{"id":"other","data":{"provider_type":"connector","provider_id":"wecom","tool_name":"get_wecom_user","connection_id":"` + otherConnectionID.String() + `"}}]}`

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`)).
		WithArgs("agent-binding-resource:" + organizationID.String() + ":integration_connection::" + connectionID.String()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(workflowReferenceImpactQuery()).
		WithArgs(organizationID, agentbindings.BindingTypeIntegrationConnection, connectionID.String(), agentbindings.ConsumerTypeWorkflow).
		WillReturnRows(workflowReferenceRows(
			organizationID, workspaceID, agentID, draftWorkflowID, publishedWorkflowID, publishedVersionID, connectionID,
		))
	mock.ExpectQuery(`SELECT \* FROM "workflows" WHERE id = \$1 AND agent_id = \$2 AND tenant_id = \$3 AND version = \$4 LIMIT \$5 FOR UPDATE`).
		WithArgs(draftWorkflowID, agentID, workspaceID, "draft", 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "app_id", "agent_id", "type", "version", "version_uuid", "graph", "features",
			"created_by", "created_at", "updated_by", "updated_at", "environment_variables", "conversation_variables", "internal",
		}).AddRow(
			draftWorkflowID, workspaceID, agentID, agentID, "workflow", "draft", nil, draftGraph, `{}`,
			actorID, now, nil, now, `[]`, `[]`, false,
		))
	mock.ExpectExec(`UPDATE "workflows" SET .*"graph"=\$[0-9]+.* WHERE id = \$[0-9]+ AND version = \$[0-9]+`).
		WithArgs(workflowGraphWithoutConnection{removed: connectionID.String(), retained: otherConnectionID.String()}, sqlmock.AnyArg(), actorID.String(), draftWorkflowID, "draft").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM "agent_resource_bindings" WHERE organization_id = \$1 AND \(binding_type = \$2 AND resource_id = \$3\) AND consumer_type = \$4`).
		WithArgs(organizationID, agentbindings.BindingTypeIntegrationConnection, connectionID.String(), agentbindings.ConsumerTypeWorkflow).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin test transaction: %v", tx.Error)
	}
	service := &WorkflowService{workflowBindings: agentbindings.NewRepository(db)}
	references, err := service.DetachConnectionReferences(context.Background(), tx, organizationID, connectionID, actorID)
	if err != nil {
		_ = tx.Rollback()
		t.Fatalf("DetachConnectionReferences() error = %v", err)
	}
	if len(references) != 2 {
		_ = tx.Rollback()
		t.Fatalf("DetachConnectionReferences() returned %d references, want 2", len(references))
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit test transaction: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("detach SQL expectations: %v", err)
	}
}

func TestDetachConnectionReferencesRequiresCallerTransaction(t *testing.T) {
	db, _ := openWorkflowConnectionReferenceTestDB(t)
	service := &WorkflowService{workflowBindings: agentbindings.NewRepository(db)}
	if _, err := service.DetachConnectionReferences(context.Background(), nil, uuid.New(), uuid.New(), uuid.New()); err == nil {
		t.Fatal("DetachConnectionReferences() error = nil, want transaction requirement")
	}
}

type workflowGraphWithoutConnection struct {
	removed  string
	retained string
}

func (matcher workflowGraphWithoutConnection) Match(value driver.Value) bool {
	text, ok := value.(string)
	if !ok {
		return false
	}
	var graph map[string]interface{}
	if err := json.Unmarshal([]byte(text), &graph); err != nil {
		return false
	}
	nodes, ok := graph["nodes"].([]interface{})
	if !ok || len(nodes) != 2 {
		return false
	}
	foundRemoved := false
	foundRetained := false
	for _, rawNode := range nodes {
		node, _ := rawNode.(map[string]interface{})
		data, _ := node["data"].(map[string]interface{})
		switch node["id"] {
		case "send-draft":
			_, foundRemoved = data["connection_id"]
			foundRemoved = !foundRemoved
		case "other":
			foundRetained = data["connection_id"] == matcher.retained
		}
	}
	return foundRemoved && foundRetained
}
