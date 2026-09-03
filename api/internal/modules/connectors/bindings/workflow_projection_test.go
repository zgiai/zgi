package bindings

import (
	"context"
	"regexp"
	"sort"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

func TestReplaceWorkflowDraftProjectionKeepsBindingIdentityOnConnectionReplacement(t *testing.T) {
	repository, mock := openRepositoryMock(t)
	scope := WorkflowProjectionScope{OrganizationID: uuid.New(), TargetID: uuid.New()}
	ownerID, oldConnectionID, newConnectionID := uuid.New(), uuid.New(), uuid.New()
	connectorID := core.Identifier("github")
	existingID := uuid.New()
	now := time.Now().UTC()
	existing := bindingRecord{
		ID: existingID, OrganizationID: scope.OrganizationID, OwnerAccountID: ownerID,
		ConnectionID: oldConnectionID, ConnectorID: connectorID.String(),
		TargetType: string(core.TargetTypeWorkflow), TargetID: scope.TargetID.String(),
		BindingInstanceID: stringPointer("node-1"), Status: string(core.BindingStatusActive),

		Revision: 3, CreatedAt: now, UpdatedAt: now,
	}

	mock.ExpectBegin()
	expectWorkflowProjectionLock(mock, scope)
	expectWorkflowProjectionRowsUnlocked(mock, scope, workflowProjectionRows(existing))
	expectBindingLifecycleAuthorities(mock, []bindingAuthorityExpectation{
		{binding: workflowProjectionAuthorityBinding(scope, existing.OwnerAccountID, oldConnectionID, connectorID)},
		{binding: workflowProjectionAuthorityBinding(scope, existing.OwnerAccountID, newConnectionID, connectorID)},
	})
	expectWorkflowProjectionRows(mock, scope, workflowProjectionRows(existing))
	expectReadyWorkflowConnection(mock, scope.OrganizationID, ownerID, newConnectionID, connectorID, true)
	mock.ExpectExec(`UPDATE "connector_target_bindings" SET .* WHERE id = \$[0-9]+ AND organization_id = \$[0-9]+ AND revision = \$[0-9]+`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectWorkflowProjectionLifecycleWithoutMigration(mock, existing, oldConnectionID)
	expectWorkflowProjectionLifecycleWithoutMigration(mock, existing, newConnectionID)
	mock.ExpectCommit()

	err := repository.ReplaceWorkflowDraftProjection(context.Background(), nil, scope, []WorkflowPlacement{{
		OwnerAccountID: ownerID, ConnectionID: newConnectionID,
		ConnectorID: connectorID, BindingInstanceID: "node-1",
	}})
	if err != nil {
		t.Fatalf("ReplaceWorkflowDraftProjection() error = %v", err)
	}
	assertMock(t, mock)
}

func TestWorkflowProjectionStableUUIDUsesTargetLineageAndNodePlacement(t *testing.T) {
	organizationID, ownerID := uuid.New(), uuid.New()
	connectorID := core.Identifier("github")
	draftTarget := core.TargetRef{Type: core.TargetTypeWorkflow, ID: uuid.NewString(), BindingInstanceID: "node-1"}
	draftID := stableNativeBindingID(organizationID, ownerID, connectorID, draftTarget)
	if replay := stableNativeBindingID(organizationID, ownerID, connectorID, draftTarget); replay != draftID {
		t.Fatalf("stable draft binding id changed: %s != %s", replay, draftID)
	}
	publishedTarget := draftTarget
	publishedTarget.ID = uuid.NewString()
	if publishedID := stableNativeBindingID(organizationID, ownerID, connectorID, publishedTarget); publishedID == draftID {
		t.Fatalf("draft and published version shared binding id %s", publishedID)
	}
	otherNode := draftTarget
	otherNode.BindingInstanceID = "node-2"
	if otherID := stableNativeBindingID(organizationID, ownerID, connectorID, otherNode); otherID == draftID {
		t.Fatalf("distinct node placements shared binding id %s", otherID)
	}
}

func TestSnapshotWorkflowPublishedProjectionCopiesOwnerAndRejectsHistoricalRewrite(t *testing.T) {
	repository, mock := openRepositoryMock(t)
	organizationID, ownerID, connectionID := uuid.New(), uuid.New(), uuid.New()
	source := WorkflowProjectionScope{OrganizationID: organizationID, TargetID: uuid.New()}
	destination := WorkflowProjectionScope{Scope: core.ScopePublished, OrganizationID: organizationID, TargetID: uuid.New()}
	connectorID := core.Identifier("github")
	now := time.Now().UTC()
	sourceRow := bindingRecord{
		ID: uuid.New(), OrganizationID: organizationID, OwnerAccountID: ownerID,
		ConnectionID: connectionID, ConnectorID: connectorID.String(),
		TargetType: string(core.TargetTypeWorkflow), TargetID: source.TargetID.String(),
		BindingInstanceID: stringPointer("node-1"), Status: string(core.BindingStatusActive),

		Revision: 2, CreatedAt: now, UpdatedAt: now,
	}

	mock.ExpectBegin()
	expectSortedWorkflowProjectionLocks(mock, source, destination)
	expectWorkflowProjectionRowsUnlocked(mock, source, workflowProjectionRows(sourceRow))
	expectWorkflowProjectionRowsUnlocked(mock, destination, workflowProjectionRows())
	expectBindingLifecycleAuthorities(mock, []bindingAuthorityExpectation{{
		binding: workflowProjectionAuthorityBinding(source, ownerID, connectionID, connectorID),
	}})
	expectWorkflowProjectionRows(mock, source, workflowProjectionRows(sourceRow))
	expectWorkflowProjectionRows(mock, destination, workflowProjectionRows())
	expectReadyWorkflowConnection(mock, organizationID, ownerID, connectionID, connectorID, true)
	mock.ExpectExec(`INSERT INTO "connector_target_bindings"`).WillReturnResult(sqlmock.NewResult(0, 1))
	publishedRow := sourceRow
	publishedRow.ID = stableNativeBindingID(organizationID, ownerID, connectorID, core.TargetRef{
		Type: core.TargetTypeWorkflow, ID: destination.TargetID.String(), BindingInstanceID: "node-1",
	})
	publishedRow.TargetID = destination.TargetID.String()
	publishedRow.Revision = 1
	expectWorkflowProjectionLifecycleWithoutMigration(mock, publishedRow, connectionID)
	mock.ExpectCommit()
	if err := repository.SnapshotWorkflowPublishedProjection(context.Background(), nil, source, destination); err != nil {
		t.Fatalf("SnapshotWorkflowPublishedProjection() error = %v", err)
	}
	assertMock(t, mock)

	// A destination with any non-equivalent native history is immutable.
	repository, mock = openRepositoryMock(t)
	historical := sourceRow
	historical.ID = uuid.New()
	historical.TargetID = destination.TargetID.String()
	historical.ConnectionID = uuid.New()
	mock.ExpectBegin()
	expectSortedWorkflowProjectionLocks(mock, source, destination)
	expectWorkflowProjectionRowsUnlocked(mock, source, workflowProjectionRows(sourceRow))
	expectWorkflowProjectionRowsUnlocked(mock, destination, workflowProjectionRows(historical))
	expectBindingLifecycleAuthorities(mock, []bindingAuthorityExpectation{
		{binding: workflowProjectionAuthorityBinding(source, ownerID, sourceRow.ConnectionID, connectorID)},
		{binding: workflowProjectionAuthorityBinding(destination, ownerID, historical.ConnectionID, connectorID)},
	})
	expectWorkflowProjectionRows(mock, source, workflowProjectionRows(sourceRow))
	expectWorkflowProjectionRows(mock, destination, workflowProjectionRows(historical))
	mock.ExpectRollback()
	err := repository.SnapshotWorkflowPublishedProjection(context.Background(), nil, source, destination)
	if !core.IsErrorCode(err, core.ErrorCodeConflict) {
		t.Fatalf("historical rewrite error = %v, want %s", err, core.ErrorCodeConflict)
	}
	assertMock(t, mock)
}

func TestRevokeWorkflowProjectionDisablesNativeRows(t *testing.T) {
	repository, mock := openRepositoryMock(t)
	scope := WorkflowProjectionScope{OrganizationID: uuid.New(), TargetID: uuid.New()}
	now := time.Now().UTC()
	row := bindingRecord{
		ID: uuid.New(), OrganizationID: scope.OrganizationID, OwnerAccountID: uuid.New(),
		ConnectionID: uuid.New(), ConnectorID: "github", TargetType: string(core.TargetTypeWorkflow),
		TargetID: scope.TargetID.String(), BindingInstanceID: stringPointer("node-1"),
		Status: string(core.BindingStatusActive), Revision: 1, CreatedAt: now, UpdatedAt: now,
	}
	mock.ExpectBegin()
	expectWorkflowProjectionLock(mock, scope)
	expectWorkflowProjectionRowsUnlocked(mock, scope, workflowProjectionRows(row))
	expectBindingLifecycleAuthorities(mock, []bindingAuthorityExpectation{{
		binding: workflowProjectionAuthorityBinding(scope, row.OwnerAccountID, row.ConnectionID, core.Identifier(row.ConnectorID)),
	}})
	expectWorkflowProjectionRows(mock, scope, workflowProjectionRows(row))
	mock.ExpectExec(`UPDATE "connector_target_bindings" SET .* WHERE id = \$[0-9]+ AND organization_id = \$[0-9]+ AND revision = \$[0-9]+`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectWorkflowProjectionLifecycleWithoutMigration(mock, row, row.ConnectionID)
	mock.ExpectCommit()
	if err := repository.RevokeWorkflowProjection(context.Background(), nil, scope); err != nil {
		t.Fatalf("RevokeWorkflowProjection() error = %v", err)
	}
	assertMock(t, mock)
}

func TestReplaceWorkflowDraftProjectionRejectsConnectionOutsideOwner(t *testing.T) {
	repository, mock := openRepositoryMock(t)
	scope := WorkflowProjectionScope{OrganizationID: uuid.New(), TargetID: uuid.New()}
	ownerID, connectionID := uuid.New(), uuid.New()
	connectorID := core.Identifier("github")
	mock.ExpectBegin()
	expectWorkflowProjectionLock(mock, scope)
	expectWorkflowProjectionRowsUnlocked(mock, scope, workflowProjectionRows())
	expectBindingLifecycleAuthorities(mock, []bindingAuthorityExpectation{{
		binding: workflowProjectionAuthorityBinding(scope, ownerID, connectionID, connectorID),
	}})
	expectWorkflowProjectionRows(mock, scope, workflowProjectionRows())
	expectReadyWorkflowConnection(mock, scope.OrganizationID, ownerID, connectionID, connectorID, false)
	mock.ExpectRollback()
	err := repository.ReplaceWorkflowDraftProjection(context.Background(), nil, scope, []WorkflowPlacement{{
		OwnerAccountID: ownerID, ConnectionID: connectionID,
		ConnectorID: connectorID, BindingInstanceID: "node-1",
	}})
	if !core.IsErrorCode(err, core.ErrorCodeConnectionNotReady) {
		t.Fatalf("non-owner projection error = %v, want %s", err, core.ErrorCodeConnectionNotReady)
	}
	assertMock(t, mock)
}

func expectWorkflowProjectionLock(mock sqlmock.Sqlmock, scope WorkflowProjectionScope) {
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`)).
		WithArgs("connector-workflow-target:" + scope.OrganizationID.String() + ":" + scope.TargetID.String()).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectSortedWorkflowProjectionLocks(mock sqlmock.Sqlmock, scopes ...WorkflowProjectionScope) {
	sorted := append([]WorkflowProjectionScope(nil), scopes...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].TargetID.String() < sorted[j].TargetID.String()
	})
	for _, scope := range sorted {
		expectWorkflowProjectionLock(mock, scope)
	}
}

func expectWorkflowProjectionRows(mock sqlmock.Sqlmock, scope WorkflowProjectionScope, rows *sqlmock.Rows) {
	mock.ExpectQuery(`SELECT \* FROM "connector_target_bindings" WHERE organization_id = \$1 AND target_type = \$2 AND target_id = \$3 AND target_scope = \$4 AND target_version_id = \$5 ORDER BY owner_account_id ASC, connector_id ASC, binding_instance_id ASC, id ASC FOR UPDATE`).
		WithArgs(scope.OrganizationID, core.TargetTypeWorkflow, scope.TargetID.String(), scope.targetScope(), scope.versionID()).
		WillReturnRows(rows)
}

func expectWorkflowProjectionRowsUnlocked(mock sqlmock.Sqlmock, scope WorkflowProjectionScope, rows *sqlmock.Rows) {
	mock.ExpectQuery(`SELECT \* FROM "connector_target_bindings" WHERE organization_id = \$1 AND target_type = \$2 AND target_id = \$3 AND target_scope = \$4 AND target_version_id = \$5 ORDER BY owner_account_id ASC, connector_id ASC, binding_instance_id ASC, id ASC`).
		WithArgs(scope.OrganizationID, core.TargetTypeWorkflow, scope.TargetID.String(), scope.targetScope(), scope.versionID()).
		WillReturnRows(rows)
}

func workflowProjectionAuthorityBinding(
	scope WorkflowProjectionScope,
	ownerID, connectionID uuid.UUID,
	connectorID core.Identifier,
) core.TargetBinding {
	return core.TargetBinding{
		OrganizationID: scope.OrganizationID,
		OwnerAccountID: ownerID,
		ConnectionID:   connectionID,
		ConnectorID:    connectorID,
	}
}

func expectReadyWorkflowConnection(
	mock sqlmock.Sqlmock,
	organizationID, ownerID, connectionID uuid.UUID,
	connectorID core.Identifier,
	found bool,
) {
	rows := sqlmock.NewRows([]string{"id"})
	if found {
		rows.AddRow(connectionID)
	}
	mock.ExpectQuery(`SELECT "id" FROM "connector_connections" WHERE id = \$1 AND organization_id = \$2 AND owner_account_id = \$3 AND connector_id = \$4 AND state = \$5 AND deleted_at IS NULL LIMIT \$6`).
		WithArgs(connectionID, organizationID, ownerID, connectorID, core.ReadinessReady, 1).
		WillReturnRows(rows)
}

func expectWorkflowProjectionLifecycleWithoutMigration(
	mock sqlmock.Sqlmock,
	row bindingRecord,
	connectionID uuid.UUID,
) {
	mock.ExpectExec(`INSERT INTO "connector_lifecycle_decisions"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectWorkflowProjectionLifecycleWithMigration(
	mock sqlmock.Sqlmock,
	row bindingRecord,
	connectionID uuid.UUID,
	mutationType string,
	maximumSequence int64,
) {
	expectWorkflowProjectionLifecycleWithoutMigration(mock, row, connectionID)
}

func workflowProjectionRows(records ...bindingRecord) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{
		"id", "organization_id", "owner_account_id", "connection_id", "connector_id",
		"target_type", "target_id", "binding_instance_id", "status",
		"revision", "disabled_at", "created_at", "updated_at", "target_scope", "target_version_id", "binding_constraint",
	})
	for _, record := range records {
		if len(record.Constraint) == 0 {
			record.Constraint = mustJSON(NewBindingConstraint(record.OwnerAccountID, core.Identifier(record.ConnectorID)))
		}
		if record.TargetScope == "" {
			record.TargetScope = "draft"
		}
		rows.AddRow(
			record.ID, record.OrganizationID, record.OwnerAccountID, record.ConnectionID, record.ConnectorID,
			record.TargetType, record.TargetID, record.BindingInstanceID, record.Status,
			record.Revision, record.DisabledAt, record.CreatedAt, record.UpdatedAt, record.TargetScope, record.TargetVersionID, record.Constraint,
		)
	}
	return rows
}

func stringPointer(value string) *string { return &value }
