package integrations

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestGormActionPolicyRepositoryReplacesOrganizationIntegrationAtomically(t *testing.T) {
	db, mock := openIntegrationRepositoryMock(t)
	repository := NewGormActionPolicyRepository(db)
	organizationID := uuid.New()
	actorID := uuid.New()
	policies := []IntegrationActionPolicy{{
		OrganizationID: organizationID, IntegrationID: IntegrationWebSearch, ActionID: ActionWebSearch,
		Enabled: false, ApprovalPolicy: IntegrationApprovalPolicyAlwaysAsk, DataEgressAllowed: false, UpdatedBy: &actorID,
	}}
	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM "integration_action_policies" WHERE organization_id = \$1 AND integration_id = \$2`).
		WithArgs(organizationID, IntegrationWebSearch).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`INSERT INTO "integration_action_policies" .* ON CONFLICT .* DO UPDATE SET`).
		WithArgs(sqlmock.AnyArg(), IntegrationWebSearch, ActionWebSearch, false, IntegrationApprovalPolicyAlwaysAsk, false, actorID, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := repository.Replace(context.Background(), organizationID, IntegrationWebSearch, policies); err != nil {
		t.Fatalf("Replace() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestGormActionPolicyRepositoryLookupIsOrganizationScoped(t *testing.T) {
	db, mock := openIntegrationRepositoryMock(t)
	organizationID := uuid.New()
	mock.ExpectQuery(`SELECT \* FROM "integration_action_policies" WHERE organization_id = \$1 AND integration_id = \$2 AND action_id = \$3`).
		WithArgs(organizationID, IntegrationWebSearch, ActionWebSearch, 1).
		WillReturnRows(sqlmock.NewRows([]string{"organization_id", "integration_id", "action_id", "enabled", "approval_policy", "data_egress_allowed"}).
			AddRow(organizationID, IntegrationWebSearch, ActionWebSearch, false, IntegrationApprovalPolicyAlwaysAsk, false))
	policy, err := NewGormActionPolicyRepository(db).Get(context.Background(), organizationID, IntegrationWebSearch, ActionWebSearch)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if policy == nil || policy.OrganizationID != organizationID || policy.ActionID != ActionWebSearch {
		t.Fatalf("Get() = %#v", policy)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestGormActionPolicyRepositoryVersionedReplaceLocksAndComparesSnapshot(t *testing.T) {
	db, mock := openIntegrationRepositoryMock(t)
	repository := NewGormActionPolicyRepository(db)
	organizationID := uuid.New()
	current := []IntegrationActionPolicy{{
		OrganizationID: organizationID, IntegrationID: IntegrationWebSearch, ActionID: ActionWebSearch,
		Enabled: true, ApprovalPolicy: IntegrationApprovalPolicyInherit, DataEgressAllowed: true,
	}}
	desired := []IntegrationActionPolicy{{
		OrganizationID: organizationID, IntegrationID: IntegrationWebSearch, ActionID: ActionWebSearch,
		Enabled: false, ApprovalPolicy: IntegrationApprovalPolicyAlwaysAsk, DataEgressAllowed: false,
	}}
	actions := []ActionDefinition{{ID: ActionWebSearch, DataEgress: true, ExternalDestination: "api.exa.ai"}}
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock\(hashtextextended\(\$1, 0\)\)`).
		WithArgs(organizationID.String() + "/" + IntegrationWebSearch).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT \* FROM "integration_action_policies" WHERE organization_id = \$1 AND integration_id = \$2 ORDER BY action_id ASC`).
		WithArgs(organizationID, IntegrationWebSearch).
		WillReturnRows(sqlmock.NewRows([]string{"organization_id", "integration_id", "action_id", "enabled", "approval_policy", "data_egress_allowed"}).
			AddRow(organizationID, IntegrationWebSearch, ActionWebSearch, true, IntegrationApprovalPolicyInherit, true))
	mock.ExpectExec(`DELETE FROM "integration_action_policies" WHERE organization_id = \$1 AND integration_id = \$2`).
		WithArgs(organizationID, IntegrationWebSearch).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO "integration_action_policies" .* ON CONFLICT .* DO UPDATE SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := repository.ReplaceIfRevision(context.Background(), organizationID, IntegrationWebSearch, actionPolicyRevision(actions, current), actions, desired); err != nil {
		t.Fatalf("ReplaceIfRevision() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestGormActionPolicyRepositoryVersionedReplaceRejectsStaleSnapshot(t *testing.T) {
	db, mock := openIntegrationRepositoryMock(t)
	repository := NewGormActionPolicyRepository(db)
	organizationID := uuid.New()
	actions := []ActionDefinition{{ID: ActionWebSearch, DataEgress: true, ExternalDestination: "api.exa.ai"}}
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock\(hashtextextended\(\$1, 0\)\)`).
		WithArgs(organizationID.String() + "/" + IntegrationWebSearch).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT \* FROM "integration_action_policies" WHERE organization_id = \$1 AND integration_id = \$2 ORDER BY action_id ASC`).
		WithArgs(organizationID, IntegrationWebSearch).
		WillReturnRows(sqlmock.NewRows([]string{"organization_id", "integration_id", "action_id", "enabled", "approval_policy", "data_egress_allowed"}).
			AddRow(organizationID, IntegrationWebSearch, ActionWebSearch, false, IntegrationApprovalPolicyAlwaysAsk, false))
	mock.ExpectRollback()
	err := repository.ReplaceIfRevision(context.Background(), organizationID, IntegrationWebSearch, actionPolicyRevision(actions, nil), actions, nil)
	if !errors.Is(err, ErrActionPolicyChanged) {
		t.Fatalf("ReplaceIfRevision() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}
