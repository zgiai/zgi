package bindings

import (
	"context"
	"database/sql/driver"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func TestDetachLifecycleCommitsBindingAndAuditTogether(t *testing.T) {
	repository, mock := openRepositoryMock(t)
	binding := nativeBinding(core.TargetTypeWorkflow, "workflow-1", "node-1")
	binding.Revision = 7
	createdAt := time.Date(2026, 9, 2, 1, 2, 3, 0, time.UTC)
	mutation := BindingLifecycleMutation{
		ID: uuid.New(), MutationType: MutationBindingDetached,
		CorrelationID: "binding-detach-atomic-0001", CreatedAt: createdAt,
	}

	mock.ExpectBegin()
	expectBindingByIDUnlocked(mock, binding, bindingRows(binding))
	expectBindingLifecycleAuthorities(mock, []bindingAuthorityExpectation{{binding: binding}})
	expectBindingByID(mock, binding, bindingRows(binding))
	mock.ExpectExec(`UPDATE "connector_target_bindings" SET .* WHERE organization_id = \$[0-9]+ AND owner_account_id = \$[0-9]+ AND id = \$[0-9]+ AND revision = \$[0-9]+`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO "connector_lifecycle_decisions"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	stored, err := repository.SetBindingStatusWithLifecycle(
		context.Background(), binding.OrganizationID, binding.OwnerAccountID, binding.ID,
		core.BindingStatusDisabled, binding.Revision, mutation,
	)
	if err != nil {
		t.Fatalf("SetBindingStatusWithLifecycle() error = %v; expectations = %v", err, mock.ExpectationsWereMet())
	}
	if stored.Status != core.BindingStatusDisabled || stored.Revision != binding.Revision+1 || stored.DisabledAt == nil {
		t.Fatalf("SetBindingStatusWithLifecycle() = %#v", stored)
	}
	assertMock(t, mock)
}

func TestDetachLifecycleRollsBackBindingWhenAuditCannotPersist(t *testing.T) {
	repository, mock := openRepositoryMock(t)
	binding := nativeBinding(core.TargetTypeChat, "chat-1", "")
	mutation := BindingLifecycleMutation{
		ID: uuid.New(), MutationType: MutationBindingDetached,
		CorrelationID: "binding-detach-atomic-rollback", CreatedAt: time.Now().UTC(),
	}

	mock.ExpectBegin()
	expectBindingByIDUnlocked(mock, binding, bindingRows(binding))
	expectBindingLifecycleAuthorities(mock, []bindingAuthorityExpectation{{binding: binding}})
	expectBindingByID(mock, binding, bindingRows(binding))
	mock.ExpectExec(`UPDATE "connector_target_bindings" SET .* WHERE organization_id = \$[0-9]+ AND owner_account_id = \$[0-9]+ AND id = \$[0-9]+ AND revision = \$[0-9]+`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO "connector_lifecycle_decisions"`).
		WillReturnError(errors.New("audit unavailable"))
	mock.ExpectRollback()

	_, err := repository.SetBindingStatusWithLifecycle(
		context.Background(), binding.OrganizationID, binding.OwnerAccountID, binding.ID,
		core.BindingStatusDisabled, binding.Revision, mutation,
	)
	if !core.IsErrorCode(err, core.ErrorCodeInternal) {
		t.Fatalf("SetBindingStatusWithLifecycle() error = %v, want internal", err)
	}
	assertMock(t, mock)
}

func TestBindingLifecycleAuditReplayRejectsDifferentContentForSameCorrelation(t *testing.T) {
	repository, mock := openRepositoryMock(t)
	createdAt := time.Date(2026, 9, 2, 10, 11, 12, 654321000, time.UTC)
	desired := bindingLifecycleDecisionRecord{
		ID: uuid.New(), OrganizationID: uuid.New(), OwnerAccountID: uuid.New(), ActingPrincipalID: uuid.New(),
		ConnectorID: "github", ConnectionID: uuid.New(), TargetType: string(core.TargetTypeWorkflow),
		TargetID: uuid.NewString(), BindingInstanceID: stringPointer("node-1"), Decision: MutationBindingUpserted,
		Status: bindingLifecycleDecisionSuccessful, CorrelationID: "binding-audit-replay-conflict",
		Details: datatypes.JSON(`{"binding_id":"expected"}`), CreatedAt: createdAt,
	}
	existing := desired
	existing.ID = uuid.New()
	existing.Details = datatypes.JSON(`{"binding_id":"different"}`)

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "connector_lifecycle_decisions".*ON CONFLICT DO NOTHING`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT \* FROM "connector_lifecycle_decisions" WHERE organization_id = \$1 AND correlation_id = \$2 AND decision = \$3 AND status = \$4.*LIMIT \$5`).
		WithArgs(desired.OrganizationID, desired.CorrelationID, desired.Decision, desired.Status, 1).
		WillReturnRows(bindingLifecycleDecisionRows(existing))
	mock.ExpectRollback()

	err := repository.db.Transaction(func(tx *gorm.DB) error {
		return createOrVerifyBindingLifecycleDecision(context.Background(), tx, desired)
	})
	if !core.IsErrorCode(err, core.ErrorCodeConflict) {
		t.Fatalf("createOrVerifyBindingLifecycleDecision() error = %v, want conflict", err)
	}
	assertMock(t, mock)
}

func TestUpsertLifecycleReplacementCommitsOldTombstoneAndNewUpsertAtomically(t *testing.T) {
	repository, mock := openRepositoryMock(t)
	existing := nativeBinding(core.TargetTypeAgent, "agent-1", "")
	existing.Revision = 4
	desired := existing
	desired.ID = uuid.New()
	desired.ConnectionID = uuid.New()
	mutation := BindingLifecycleMutation{
		ID: uuid.New(), MutationType: MutationBindingUpserted,
		CorrelationID: "binding-upsert-replacement-atomic", CreatedAt: time.Date(2026, 9, 2, 4, 5, 6, 0, time.UTC),
	}
	after := existing
	after.ConnectionID = desired.ConnectionID
	after.Revision = existing.Revision + 1
	detached, detachMutation := replacementDetachLifecycle(existing, after, mutation)

	mock.ExpectBegin()
	expectLogicalBindingUnlocked(mock, desired, bindingRows(existing))
	expectBindingLifecycleAuthorities(mock, []bindingAuthorityExpectation{
		{binding: existing},
		{binding: desired},
	})
	expectLogicalBinding(mock, desired, bindingRows(existing))
	expectReadyConnection(mock, desired)
	mock.ExpectExec(`INSERT INTO "connector_target_bindings".*ON CONFLICT DO NOTHING`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	expectLogicalBinding(mock, desired, bindingRows(existing))
	mock.ExpectExec(`UPDATE "connector_target_bindings" SET .* WHERE organization_id = \$[0-9]+ AND owner_account_id = \$[0-9]+ AND id = \$[0-9]+ AND revision = \$[0-9]+`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectBindingLifecycleDecision(mock, detached, detachMutation)
	expectBindingLifecycleDecision(mock, after, mutation)
	mock.ExpectCommit()

	stored, created, err := repository.UpsertBindingWithLifecycle(
		context.Background(), desired.OrganizationID, desired.OwnerAccountID, desired, existing.Revision, mutation,
	)
	if err != nil {
		t.Fatalf("UpsertBindingWithLifecycle() error = %v", err)
	}
	if created || stored.ID != existing.ID || stored.ConnectionID != desired.ConnectionID || stored.Revision != existing.Revision+1 {
		t.Fatalf("UpsertBindingWithLifecycle() = %#v, created=%v", stored, created)
	}
	if detached.ID != stored.ID || detached.ConnectionID != existing.ConnectionID || detached.Status != core.BindingStatusDisabled ||
		detached.DisabledAt == nil || detachMutation.MutationType != MutationBindingDetached {
		t.Fatalf("replacement detach projection/mutation = %#v / %#v", detached, detachMutation)
	}
	assertMock(t, mock)
}

func TestUpsertLifecycleReplacementRollsBackCanonicalWriteAndOldTombstoneWhenNewAuditFails(t *testing.T) {
	repository, mock := openRepositoryMock(t)
	existing := nativeBinding(core.TargetTypeWorkflow, "workflow-1", "node-1")
	existing.Revision = 2
	desired := existing
	desired.ID = uuid.New()
	desired.ConnectionID = uuid.New()
	mutation := BindingLifecycleMutation{
		ID: uuid.New(), MutationType: MutationBindingUpserted,
		CorrelationID: "binding-upsert-replacement-rollback", CreatedAt: time.Date(2026, 9, 2, 7, 8, 9, 0, time.UTC),
	}
	after := existing
	after.ConnectionID = desired.ConnectionID
	after.Revision = existing.Revision + 1
	detached, detachMutation := replacementDetachLifecycle(existing, after, mutation)

	mock.ExpectBegin()
	expectLogicalBindingUnlocked(mock, desired, bindingRows(existing))
	expectBindingLifecycleAuthorities(mock, []bindingAuthorityExpectation{
		{binding: existing},
		{binding: desired},
	})
	expectLogicalBinding(mock, desired, bindingRows(existing))
	expectReadyConnection(mock, desired)
	mock.ExpectExec(`INSERT INTO "connector_target_bindings".*ON CONFLICT DO NOTHING`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	expectLogicalBinding(mock, desired, bindingRows(existing))
	mock.ExpectExec(`UPDATE "connector_target_bindings" SET .* WHERE organization_id = \$[0-9]+ AND owner_account_id = \$[0-9]+ AND id = \$[0-9]+ AND revision = \$[0-9]+`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectBindingLifecycleDecision(mock, detached, detachMutation)
	mock.ExpectExec(`INSERT INTO "connector_lifecycle_decisions"`).WillReturnError(errors.New("audit unavailable"))
	mock.ExpectRollback()

	_, _, err := repository.UpsertBindingWithLifecycle(
		context.Background(), desired.OrganizationID, desired.OwnerAccountID, desired, existing.Revision, mutation,
	)
	if !core.IsErrorCode(err, core.ErrorCodeInternal) {
		t.Fatalf("UpsertBindingWithLifecycle() error = %v, want internal", err)
	}
	assertMock(t, mock)
}

func expectBindingLifecycleDecision(
	mock sqlmock.Sqlmock,
	binding core.TargetBinding,
	mutation BindingLifecycleMutation,
) {
	mock.ExpectExec(`INSERT INTO "connector_lifecycle_decisions"`).WithArgs(
		string(binding.Target.EffectiveScope()), binding.Target.VersionID,
		mutation.ID,
		binding.OrganizationID,
		binding.OwnerAccountID,
		binding.OwnerAccountID,
		binding.ConnectorID.String(),
		binding.ConnectionID,
		string(binding.Target.Type),
		binding.Target.ID,
		optionalBindingText(binding.Target.BindingInstanceID),
		nil,
		nil,
		mutation.MutationType,
		bindingLifecycleDecisionSuccessful,
		mutation.CorrelationID,
		nil,
		sqlmock.AnyArg(),
		mutation.CreatedAt.UTC(),
	).WillReturnResult(sqlmock.NewResult(0, 1))
}

type bindingAuthorityExpectation struct{ binding core.TargetBinding }

func expectBindingLifecycleAuthorities(mock sqlmock.Sqlmock, expectations []bindingAuthorityExpectation) {
	ordered := append([]bindingAuthorityExpectation(nil), expectations...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].binding.ConnectionID.String() < ordered[j].binding.ConnectionID.String()
	})
	seen := map[uuid.UUID]bool{}
	for _, expected := range ordered {
		b := expected.binding
		if seen[b.ConnectionID] {
			continue
		}
		seen[b.ConnectionID] = true
		mock.ExpectQuery(`SELECT "id" FROM "connector_connections" WHERE id = \$1 AND organization_id = \$2 AND owner_account_id = \$3 AND connector_id = \$4.*LIMIT \$5 FOR UPDATE`).
			WithArgs(b.ConnectionID, b.OrganizationID, b.OwnerAccountID, b.ConnectorID.String(), 1).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(b.ConnectionID))
	}
}

func expectBindingByIDUnlocked(mock sqlmock.Sqlmock, binding core.TargetBinding, rows *sqlmock.Rows) {
	mock.ExpectQuery(`SELECT \* FROM "connector_target_bindings" WHERE organization_id = \$1 AND owner_account_id = \$2 AND id = \$3.*LIMIT \$4`).
		WithArgs(binding.OrganizationID, binding.OwnerAccountID, binding.ID, 1).
		WillReturnRows(rows)
}

func expectLogicalBindingUnlocked(mock sqlmock.Sqlmock, binding core.TargetBinding, rows *sqlmock.Rows) {
	query := `SELECT \* FROM "connector_target_bindings" WHERE \(organization_id = \$1 AND owner_account_id = \$2 AND target_type = \$3 AND target_id = \$4 AND connector_id = \$5 AND target_scope = \$6 AND target_version_id = \$7\)`
	args := []driver.Value{binding.OrganizationID, binding.OwnerAccountID, string(binding.Target.Type), binding.Target.ID, binding.ConnectorID.String(), string(binding.Target.EffectiveScope()), binding.Target.VersionID}
	if binding.Target.BindingInstanceID == "" {
		query += ` AND binding_instance_id IS NULL.*LIMIT \$8`
		args = append(args, 1)
	} else {
		query += ` AND binding_instance_id = \$8.*LIMIT \$9`
		args = append(args, binding.Target.BindingInstanceID, 1)
	}
	mock.ExpectQuery(query).WithArgs(args...).WillReturnRows(rows)
}

func bindingLifecycleDecisionRows(records ...bindingLifecycleDecisionRecord) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{
		"id", "organization_id", "owner_account_id", "acting_principal_id", "connector_id", "connection_id",
		"target_type", "target_id", "binding_instance_id", "operation_id", "logical_operation_id", "decision",
		"status", "correlation_id", "receipt_id", "details", "created_at",
	})
	for _, record := range records {
		rows.AddRow(
			record.ID, record.OrganizationID, record.OwnerAccountID, record.ActingPrincipalID, record.ConnectorID, record.ConnectionID,
			record.TargetType, record.TargetID, record.BindingInstanceID, record.OperationID, record.LogicalOperationID, record.Decision,
			record.Status, record.CorrelationID, record.ReceiptID, record.Details, record.CreatedAt,
		)
	}
	return rows
}
