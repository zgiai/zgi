package v1

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/agentbindings"
	"github.com/zgiai/zgi/api/internal/modules/integrations/core"
	"github.com/zgiai/zgi/api/internal/util"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const integrationConnectionImpactQuery = `SELECT \* FROM "agent_resource_bindings" WHERE organization_id = \$1 AND \(binding_type = \$2 AND resource_id = \$3\) ORDER BY agent_id ASC, binding_scope ASC, published_version_uuid ASC`

type integrationReferenceWorkflowService struct {
	readinessErr error
	detachErr    error
	detachCalls  int
	detachTx     *gorm.DB
}

func (service *integrationReferenceWorkflowService) EnsureConnectionReferenceIndexReady(
	context.Context,
	uuid.UUID,
) error {
	return service.readinessErr
}

func (*integrationReferenceWorkflowService) PreviewConnectionReferences(
	context.Context,
	uuid.UUID,
	uuid.UUID,
) ([]agentbindings.WorkflowConnectionReference, error) {
	return []agentbindings.WorkflowConnectionReference{}, nil
}

func (service *integrationReferenceWorkflowService) DetachConnectionReferences(
	_ context.Context,
	tx *gorm.DB,
	_ uuid.UUID,
	_ uuid.UUID,
	_ uuid.UUID,
) ([]agentbindings.WorkflowConnectionReference, error) {
	service.detachCalls++
	service.detachTx = tx
	if service.detachErr != nil {
		return nil, service.detachErr
	}
	return []agentbindings.WorkflowConnectionReference{}, nil
}

func TestIntegrationConnectionDeleteImpactAggregatesAgentAndWorkflowScopes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock := openIntegrationReferenceTestDB(t)
	organizationID := uuid.New()
	workspaceID := uuid.New()
	connectionID := uuid.New()
	actorID := uuid.New()
	agentID := uuid.New()
	workflowAgentID := uuid.New()
	workflowDraftID := uuid.New()
	workflowPublishedID := uuid.New()
	agentVersion := uuid.New()
	workflowVersion := uuid.New()

	bindings := []agentbindings.Binding{
		integrationReferenceBinding(
			organizationID, workspaceID, connectionID, agentID,
			agentbindings.ConsumerTypeAgent, agentID, "", agentbindings.ScopeDraft, nil,
			"Agent connection", []string{"wecom.user.search"},
		),
		integrationReferenceBinding(
			organizationID, workspaceID, connectionID, agentID,
			agentbindings.ConsumerTypeAgent, agentID, "", agentbindings.ScopePublished, &agentVersion,
			"Agent connection", []string{"wecom.user.search"},
		),
		integrationReferenceBinding(
			organizationID, workspaceID, connectionID, workflowAgentID,
			agentbindings.ConsumerTypeWorkflow, workflowDraftID, "node-send", agentbindings.ScopeDraft, nil,
			"Send notification", []string{"wecom.message.send"},
		),
		integrationReferenceBinding(
			organizationID, workspaceID, connectionID, workflowAgentID,
			agentbindings.ConsumerTypeWorkflow, workflowPublishedID, "node-send", agentbindings.ScopePublished, &workflowVersion,
			"Send notification", []string{"wecom.message.send"},
		),
	}

	expectIntegrationImpactBindings(mock, organizationID, connectionID, bindings)
	expectIntegrationImpactAgentPresentations(mock, map[uuid.UUID]string{
		agentID:         "Customer assistant",
		workflowAgentID: "Notification workflow",
	})
	expectIntegrationReferenceAssetNames(mock, organizationID, map[uuid.UUID]string{
		agentID:         "Customer assistant",
		workflowAgentID: "Notification workflow",
	})
	expectIntegrationReferenceWorkspaceNames(mock, organizationID, map[uuid.UUID]string{workspaceID: "Operations"})

	repository := agentbindings.NewRepositoryWithTokenSecret(db, "integration-reference-route-secret")
	service := managedIntegrationConnectionService(organizationID, connectionID)
	handler := &integrationHandler{deps: IntegrationRouteDeps{
		DB: db, Connections: service, AgentBindings: repository,
	}}
	recorder := httptest.NewRecorder()
	ctx := integrationReferenceRouteContext(
		recorder, http.MethodGet,
		"/console/api/integrations/connections/"+connectionID.String()+"/delete-impact",
		"", organizationID, connectionID, actorID,
	)

	handler.connectionDeleteImpact(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	impact := decodeIntegrationReferenceImpact(t, recorder)
	if impact.ConnectionID != connectionID || impact.BoundAgentCount != 1 || impact.BoundWorkflowCount != 1 || impact.CanDelete {
		t.Fatalf("impact summary = %#v", impact)
	}
	if impact.ImpactToken == "" || impact.ExpiresAt == 0 || len(impact.References) != 2 {
		t.Fatalf("impact confirmation/detail = %#v", impact)
	}

	references := make(map[string]integrationConnectionReference, len(impact.References))
	for _, reference := range impact.References {
		references[reference.AssetType] = reference
	}
	agentReference := references[string(agentbindings.ConsumerTypeAgent)]
	if agentReference.AssetID != agentID || agentReference.Name != "Customer assistant" ||
		!agentReference.DraftReferenced || !agentReference.PublishedReferenced ||
		len(agentReference.Actions) != 1 || agentReference.Actions[0] != "wecom.user.search" ||
		len(agentReference.Nodes) != 0 {
		t.Fatalf("Agent reference = %#v", agentReference)
	}
	workflowReference := references[string(agentbindings.ConsumerTypeWorkflow)]
	if workflowReference.AssetID != workflowAgentID || workflowReference.Name != "Notification workflow" ||
		!workflowReference.DraftReferenced || !workflowReference.PublishedReferenced ||
		len(workflowReference.Actions) != 1 || workflowReference.Actions[0] != "wecom.message.send" ||
		len(workflowReference.Nodes) != 1 {
		t.Fatalf("Workflow reference = %#v", workflowReference)
	}
	node := workflowReference.Nodes[0]
	if node.NodeID != "node-send" || node.NodeName != "Send notification" || node.ActionID != "wecom.message.send" ||
		!node.DraftReferenced || !node.PublishedReferenced {
		t.Fatalf("Workflow node = %#v", node)
	}
}

func TestIntegrationConnectionLifecycleFailsClosedWhileWorkflowReferenceIndexIsIncomplete(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, _ := openIntegrationReferenceTestDB(t)
	organizationID := uuid.New()
	connectionID := uuid.New()
	actorID := uuid.New()
	service := managedIntegrationConnectionService(organizationID, connectionID)
	handler := &integrationHandler{deps: IntegrationRouteDeps{
		DB: db, Connections: service, AgentBindings: agentbindings.NewRepository(db),
		WorkflowReferences: &integrationReferenceWorkflowService{
			readinessErr: errors.New("workflow connection reference index is incomplete for 1 workflow(s)"),
		},
	}}
	recorder := httptest.NewRecorder()
	ctx := integrationReferenceRouteContext(
		recorder, http.MethodDelete,
		"/console/api/integrations/connections/"+connectionID.String(),
		"", organizationID, connectionID, actorID,
	)

	handler.deleteConnection(ctx)

	if recorder.Code != http.StatusServiceUnavailable || service.deleted != uuid.Nil {
		t.Fatalf("status = %d, deleted = %s, body = %s", recorder.Code, service.deleted, recorder.Body.String())
	}
}

func TestIntegrationConnectionDeleteFailsClosedForAgentAndWorkflowReferences(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, testCase := range []struct {
		name         string
		consumerType agentbindings.ConsumerType
		nodeID       string
	}{
		{name: "Agent reference", consumerType: agentbindings.ConsumerTypeAgent},
		{name: "Workflow reference", consumerType: agentbindings.ConsumerTypeWorkflow, nodeID: "node-send"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			db, mock := openIntegrationReferenceTestDB(t)
			organizationID := uuid.New()
			workspaceID := uuid.New()
			connectionID := uuid.New()
			actorID := uuid.New()
			assetID := uuid.New()
			consumerID := assetID
			if testCase.consumerType == agentbindings.ConsumerTypeWorkflow {
				consumerID = uuid.New()
			}
			binding := integrationReferenceBinding(
				organizationID, workspaceID, connectionID, assetID,
				testCase.consumerType, consumerID, testCase.nodeID, agentbindings.ScopeDraft, nil,
				"Bound consumer", []string{"wecom.message.send"},
			)
			expectIntegrationImpactBindings(mock, organizationID, connectionID, []agentbindings.Binding{binding})

			service := managedIntegrationConnectionService(organizationID, connectionID)
			handler := &integrationHandler{deps: IntegrationRouteDeps{
				DB:            db,
				Connections:   service,
				AgentBindings: agentbindings.NewRepositoryWithTokenSecret(db, "integration-reference-route-secret"),
			}}
			recorder := httptest.NewRecorder()
			ctx := integrationReferenceRouteContext(
				recorder, http.MethodDelete,
				"/console/api/integrations/connections/"+connectionID.String(),
				"", organizationID, connectionID, actorID,
			)

			handler.deleteConnection(ctx)

			if recorder.Code != http.StatusConflict || service.deleted != uuid.Nil {
				t.Fatalf("status = %d, deleted = %s, body = %s", recorder.Code, service.deleted, recorder.Body.String())
			}
			var payload struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if payload.Code != "integration_connection_in_use" {
				t.Fatalf("code = %q, body = %s", payload.Code, recorder.Body.String())
			}
		})
	}
}

func TestIntegrationConnectionDetachRejectsChangedAndExpiredImpactTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, testCase := range []struct {
		name     string
		expired  bool
		issuedAt func() time.Time
	}{
		{
			name:     "changed impact",
			issuedAt: func() time.Time { return time.Now().UTC() },
		},
		{
			name:    "expired confirmation",
			expired: true,
			issuedAt: func() time.Time {
				return time.Now().UTC().Add(-2 * agentbindings.ImpactTokenTTL)
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			db, mock := openIntegrationReferenceTestDB(t)
			organizationID := uuid.New()
			workspaceID := uuid.New()
			connectionID := uuid.New()
			actorID := uuid.New()
			agentID := uuid.New()
			oldBinding := integrationReferenceBinding(
				organizationID, workspaceID, connectionID, agentID,
				agentbindings.ConsumerTypeAgent, agentID, "", agentbindings.ScopeDraft, nil,
				"Old reference", []string{"wecom.user.get"},
			)
			latestBinding := integrationReferenceBinding(
				organizationID, workspaceID, connectionID, agentID,
				agentbindings.ConsumerTypeAgent, agentID, "", agentbindings.ScopeDraft, nil,
				"Latest reference", []string{"wecom.user.search"},
			)
			repository := agentbindings.NewRepositoryWithTokenSecret(db, "integration-reference-route-secret")
			oldToken := issueIntegrationReferenceImpactToken(
				t, mock, repository, organizationID, connectionID, actorID,
				testCase.issuedAt(), []agentbindings.Binding{oldBinding},
				map[uuid.UUID]string{agentID: "Customer assistant"},
			)

			mock.ExpectBegin()
			expectIntegrationReferenceResourceLock(mock, organizationID, connectionID)
			if !testCase.expired {
				expectIntegrationImpactBindings(mock, organizationID, connectionID, []agentbindings.Binding{latestBinding})
			}
			mock.ExpectRollback()
			expectIntegrationImpactBindings(mock, organizationID, connectionID, []agentbindings.Binding{latestBinding})
			expectIntegrationImpactAgentPresentations(mock, map[uuid.UUID]string{agentID: "Customer assistant"})
			expectIntegrationReferenceAssetNames(mock, organizationID, map[uuid.UUID]string{agentID: "Customer assistant"})
			expectIntegrationReferenceWorkspaceNames(mock, organizationID, map[uuid.UUID]string{workspaceID: "Operations"})

			workflowReferences := &integrationReferenceWorkflowService{}
			handler := &integrationHandler{deps: IntegrationRouteDeps{
				DB: db, Connections: managedIntegrationConnectionService(organizationID, connectionID),
				AgentBindings: repository, WorkflowReferences: workflowReferences,
			}}
			recorder := httptest.NewRecorder()
			body := `{"impact_token":` + mustIntegrationReferenceJSON(t, oldToken) + `}`
			ctx := integrationReferenceRouteContext(
				recorder, http.MethodPost,
				"/console/api/integrations/connections/"+connectionID.String()+"/detach-references",
				body, organizationID, connectionID, actorID,
			)

			handler.detachConnectionReferences(ctx)

			if recorder.Code != http.StatusConflict {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			var payload struct {
				Code string                                    `json:"code"`
				Data integrationConnectionDeleteImpactResponse `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if payload.Code != "integration_connection_reference_impact_changed" {
				t.Fatalf("code = %q, body = %s", payload.Code, recorder.Body.String())
			}
			if payload.Data.ImpactToken == "" || payload.Data.ImpactToken == oldToken ||
				payload.Data.CanDelete || len(payload.Data.References) != 1 ||
				payload.Data.References[0].Name != "Customer assistant" ||
				len(payload.Data.References[0].Actions) != 1 ||
				payload.Data.References[0].Actions[0] != "wecom.user.search" {
				t.Fatalf("latest impact = %#v", payload.Data)
			}
			if workflowReferences.detachCalls != 0 {
				t.Fatalf("detach calls = %d, want 0", workflowReferences.detachCalls)
			}
		})
	}
}

func TestIntegrationConnectionDetachRollsBackWhenWorkflowDetachFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock := openIntegrationReferenceTestDB(t)
	organizationID := uuid.New()
	workspaceID := uuid.New()
	connectionID := uuid.New()
	actorID := uuid.New()
	workflowAgentID := uuid.New()
	workflowVersion := uuid.New()
	binding := integrationReferenceBinding(
		organizationID, workspaceID, connectionID, workflowAgentID,
		agentbindings.ConsumerTypeWorkflow, uuid.New(), "node-send", agentbindings.ScopePublished, &workflowVersion,
		"Send notification", []string{"wecom.message.send"},
	)
	repository := agentbindings.NewRepositoryWithTokenSecret(db, "integration-reference-route-secret")
	token := issueIntegrationReferenceImpactToken(
		t, mock, repository, organizationID, connectionID, actorID, time.Now().UTC(),
		[]agentbindings.Binding{binding}, map[uuid.UUID]string{workflowAgentID: "Notification workflow"},
	)

	mock.ExpectBegin()
	expectIntegrationReferenceResourceLock(mock, organizationID, connectionID)
	expectIntegrationImpactBindings(mock, organizationID, connectionID, []agentbindings.Binding{binding})
	mock.ExpectRollback()

	workflowReferences := &integrationReferenceWorkflowService{detachErr: errors.New("workflow graph update failed")}
	handler := &integrationHandler{deps: IntegrationRouteDeps{
		DB: db, Connections: managedIntegrationConnectionService(organizationID, connectionID),
		AgentBindings: repository, WorkflowReferences: workflowReferences,
	}}
	recorder := httptest.NewRecorder()
	body := `{"impact_token":` + mustIntegrationReferenceJSON(t, token) + `}`
	ctx := integrationReferenceRouteContext(
		recorder, http.MethodPost,
		"/console/api/integrations/connections/"+connectionID.String()+"/detach-references",
		body, organizationID, connectionID, actorID,
	)

	handler.detachConnectionReferences(ctx)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Code != integrations.ErrorCodeUpstream {
		t.Fatalf("code = %q, body = %s", payload.Code, recorder.Body.String())
	}
	if workflowReferences.detachCalls != 1 {
		t.Fatalf("detach calls = %d, want 1", workflowReferences.detachCalls)
	}
	if workflowReferences.detachTx == nil {
		t.Fatal("Workflow detach did not receive the active transaction")
	}
}

func TestIntegrationConnectionDetachSuccessReturnsDeletableImpact(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock := openIntegrationReferenceTestDB(t)
	organizationID := uuid.New()
	workspaceID := uuid.New()
	connectionID := uuid.New()
	actorID := uuid.New()
	agentID := uuid.New()
	workflowAgentID := uuid.New()
	agentVersion := uuid.New()
	workflowVersion := uuid.New()
	bindings := []agentbindings.Binding{
		integrationReferenceBinding(
			organizationID, workspaceID, connectionID, agentID,
			agentbindings.ConsumerTypeAgent, agentID, "", agentbindings.ScopePublished, &agentVersion,
			"Agent connection", []string{"wecom.user.search"},
		),
		integrationReferenceBinding(
			organizationID, workspaceID, connectionID, workflowAgentID,
			agentbindings.ConsumerTypeWorkflow, uuid.New(), "node-send", agentbindings.ScopePublished, &workflowVersion,
			"Send notification", []string{"wecom.message.send"},
		),
	}
	repository := agentbindings.NewRepositoryWithTokenSecret(db, "integration-reference-route-secret")
	token := issueIntegrationReferenceImpactToken(
		t, mock, repository, organizationID, connectionID, actorID, time.Now().UTC(), bindings,
		map[uuid.UUID]string{agentID: "Customer assistant", workflowAgentID: "Notification workflow"},
	)

	mock.ExpectBegin()
	expectIntegrationReferenceResourceLock(mock, organizationID, connectionID)
	expectIntegrationImpactBindings(mock, organizationID, connectionID, bindings)
	expectIntegrationReferenceResourceLock(mock, organizationID, connectionID)
	expectIntegrationImpactBindings(mock, organizationID, connectionID, bindings)
	mock.ExpectExec(`DELETE FROM "agent_resource_bindings" WHERE organization_id = \$1 AND \(binding_type = \$2 AND resource_id = \$3\)`).
		WithArgs(organizationID, string(agentbindings.BindingTypeIntegrationConnection), connectionID.String()).
		WillReturnResult(sqlmock.NewResult(0, int64(len(bindings))))
	expectIntegrationImpactBindings(mock, organizationID, connectionID, nil)
	mock.ExpectCommit()

	workflowReferences := &integrationReferenceWorkflowService{}
	handler := &integrationHandler{deps: IntegrationRouteDeps{
		DB: db, Connections: managedIntegrationConnectionService(organizationID, connectionID),
		AgentBindings: repository, WorkflowReferences: workflowReferences,
	}}
	recorder := httptest.NewRecorder()
	body := `{"impact_token":` + mustIntegrationReferenceJSON(t, token) + `}`
	ctx := integrationReferenceRouteContext(
		recorder, http.MethodPost,
		"/console/api/integrations/connections/"+connectionID.String()+"/detach-references",
		body, organizationID, connectionID, actorID,
	)

	handler.detachConnectionReferences(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	impact := decodeIntegrationReferenceImpact(t, recorder)
	if impact.ConnectionID != connectionID || !impact.CanDelete || impact.BoundAgentCount != 0 ||
		impact.BoundWorkflowCount != 0 || impact.ImpactToken != "" || len(impact.References) != 0 {
		t.Fatalf("detached impact = %#v", impact)
	}
	if workflowReferences.detachCalls != 1 {
		t.Fatalf("detach calls = %d, want 1", workflowReferences.detachCalls)
	}
	if workflowReferences.detachTx == nil {
		t.Fatal("Workflow detach did not receive the active transaction")
	}
}

func openIntegrationReferenceTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatalf("gorm.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet SQL expectations: %v", err)
		}
		_ = sqlDB.Close()
	})
	return db, mock
}

func integrationReferenceBinding(
	organizationID uuid.UUID,
	workspaceID uuid.UUID,
	connectionID uuid.UUID,
	agentID uuid.UUID,
	consumerType agentbindings.ConsumerType,
	consumerID uuid.UUID,
	bindingInstanceID string,
	scope agentbindings.Scope,
	publishedVersionUUID *uuid.UUID,
	displayName string,
	actionIDs []string,
) agentbindings.Binding {
	now := time.Unix(1_800_000_000, 0).UTC()
	return agentbindings.Binding{
		ID:                   uuid.New(),
		AgentID:              agentID,
		ConsumerType:         consumerType,
		ConsumerID:           consumerID,
		BindingInstanceID:    bindingInstanceID,
		BindingScope:         scope,
		OrganizationID:       organizationID,
		WorkspaceID:          workspaceID,
		PublishedVersionUUID: publishedVersionUUID,
		BindingType:          agentbindings.BindingTypeIntegrationConnection,
		ResourceID:           connectionID.String(),
		ParentResourceID:     "wecom",
		DisplayName:          displayName,
		AccessMode:           "write",
		Metadata: map[string]interface{}{
			agentbindings.IntegrationAllowedActionIDsMetadataKey: actionIDs,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func integrationReferenceBindingColumns() []string {
	return []string{
		"id", "agent_id", "consumer_type", "consumer_id", "binding_instance_id",
		"binding_scope", "organization_id", "workspace_id", "published_version_uuid",
		"binding_type", "resource_id", "parent_resource_id", "display_name", "access_mode", "metadata",
		"authorized_by", "authorized_at", "created_at", "updated_at",
	}
}

func expectIntegrationImpactBindings(
	mock sqlmock.Sqlmock,
	organizationID uuid.UUID,
	connectionID uuid.UUID,
	bindings []agentbindings.Binding,
) {
	mock.ExpectQuery(integrationConnectionImpactQuery).
		WithArgs(organizationID, string(agentbindings.BindingTypeIntegrationConnection), connectionID.String()).
		WillReturnRows(integrationReferenceBindingRowsWithoutTest(bindings))
}

func integrationReferenceBindingRowsWithoutTest(bindings []agentbindings.Binding) *sqlmock.Rows {
	rows := sqlmock.NewRows(integrationReferenceBindingColumns())
	for _, binding := range bindings {
		metadata, _ := json.Marshal(binding.Metadata)
		var publishedVersion interface{}
		if binding.PublishedVersionUUID != nil {
			publishedVersion = *binding.PublishedVersionUUID
		}
		var authorizedBy interface{}
		if binding.AuthorizedBy != nil {
			authorizedBy = *binding.AuthorizedBy
		}
		var authorizedAt interface{}
		if binding.AuthorizedAt != nil {
			authorizedAt = *binding.AuthorizedAt
		}
		rows.AddRow(
			binding.ID, binding.AgentID, binding.ConsumerType, binding.ConsumerID, binding.BindingInstanceID,
			binding.BindingScope, binding.OrganizationID, binding.WorkspaceID, publishedVersion,
			binding.BindingType, binding.ResourceID, binding.ParentResourceID, binding.DisplayName, binding.AccessMode, string(metadata),
			authorizedBy, authorizedAt, binding.CreatedAt, binding.UpdatedAt,
		)
	}
	return rows
}

func expectIntegrationImpactAgentPresentations(mock sqlmock.Sqlmock, names map[uuid.UUID]string) {
	rows := sqlmock.NewRows([]string{"id", "name", "description", "icon_type", "icon"})
	for id, name := range names {
		rows.AddRow(id, name, "", nil, nil)
	}
	mock.ExpectQuery(`SELECT id, name, description, icon_type, icon FROM "agents" WHERE id IN \(.+\) AND deleted_at IS NULL`).
		WillReturnRows(rows)
}

func expectIntegrationReferenceAssetNames(mock sqlmock.Sqlmock, organizationID uuid.UUID, names map[uuid.UUID]string) {
	rows := sqlmock.NewRows([]string{"id", "name"})
	for id, name := range names {
		rows.AddRow(id, name)
	}
	arguments := integrationReferenceOrganizationScopedArguments(len(names), organizationID)
	mock.ExpectQuery(`SELECT asset.id, asset.name FROM agents AS asset JOIN workspaces AS workspace ON workspace.id = asset.tenant_id WHERE asset.id IN \(.+\) AND workspace.organization_id = \$[0-9]+`).
		WithArgs(arguments...).
		WillReturnRows(rows)
}

func expectIntegrationReferenceWorkspaceNames(mock sqlmock.Sqlmock, organizationID uuid.UUID, names map[uuid.UUID]string) {
	rows := sqlmock.NewRows([]string{"id", "name"})
	for id, name := range names {
		rows.AddRow(id, name)
	}
	arguments := integrationReferenceOrganizationScopedArguments(len(names), organizationID)
	mock.ExpectQuery(`SELECT id, name FROM "workspaces" WHERE id IN \(.+\) AND organization_id = \$[0-9]+`).
		WithArgs(arguments...).
		WillReturnRows(rows)
}

func integrationReferenceOrganizationScopedArguments(idCount int, organizationID uuid.UUID) []driver.Value {
	arguments := make([]driver.Value, 0, idCount+1)
	for index := 0; index < idCount; index++ {
		arguments = append(arguments, sqlmock.AnyArg())
	}
	return append(arguments, organizationID)
}

func expectIntegrationReferenceResourceLock(mock sqlmock.Sqlmock, organizationID, connectionID uuid.UUID) {
	lockKey := strings.Join([]string{
		"agent-binding-resource",
		organizationID.String(),
		string(agentbindings.BindingTypeIntegrationConnection),
		"",
		connectionID.String(),
	}, ":")
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`)).
		WithArgs(lockKey).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func issueIntegrationReferenceImpactToken(
	t *testing.T,
	mock sqlmock.Sqlmock,
	repository *agentbindings.Repository,
	organizationID uuid.UUID,
	connectionID uuid.UUID,
	actorID uuid.UUID,
	issuedAt time.Time,
	bindings []agentbindings.Binding,
	names map[uuid.UUID]string,
) string {
	t.Helper()
	expectIntegrationImpactBindings(mock, organizationID, connectionID, bindings)
	expectIntegrationImpactAgentPresentations(mock, names)
	impact, err := repository.PreviewImpact(
		context.Background(),
		agentbindings.ResourceRef{
			OrganizationID: organizationID,
			BindingType:    agentbindings.BindingTypeIntegrationConnection,
			ResourceID:     connectionID.String(),
		},
		detachIntegrationConnectionReferencesOperation,
		actorID,
		issuedAt,
	)
	if err != nil {
		t.Fatalf("PreviewImpact() error = %v", err)
	}
	if impact == nil || impact.ImpactToken == "" {
		t.Fatalf("PreviewImpact() = %#v", impact)
	}
	return impact.ImpactToken
}

func managedIntegrationConnectionService(organizationID, connectionID uuid.UUID) *integrationRouteConnectionService {
	return &integrationRouteConnectionService{getItem: integrations.ConnectionView{
		ID:               connectionID,
		OrganizationID:   organizationID,
		CredentialSource: integrations.ConnectionCredentialSourceOrganization,
	}}
}

func integrationReferenceRouteContext(
	recorder *httptest.ResponseRecorder,
	method string,
	path string,
	body string,
	organizationID uuid.UUID,
	connectionID uuid.UUID,
	actorID uuid.UUID,
) *gin.Context {
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		ctx.Request.Header.Set("Content-Type", "application/json")
	}
	ctx.Params = gin.Params{{Key: "id", Value: connectionID.String()}}
	util.SetOrganizationID(ctx, organizationID.String())
	ctx.Set("account_id", actorID.String())
	return ctx
}

func decodeIntegrationReferenceImpact(t *testing.T, recorder *httptest.ResponseRecorder) integrationConnectionDeleteImpactResponse {
	t.Helper()
	var payload struct {
		Code string                                    `json:"code"`
		Data integrationConnectionDeleteImpactResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Code != "0" {
		t.Fatalf("code = %q, body = %s", payload.Code, recorder.Body.String())
	}
	return payload.Data
}

func mustIntegrationReferenceJSON(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON value: %v", err)
	}
	return string(encoded)
}
