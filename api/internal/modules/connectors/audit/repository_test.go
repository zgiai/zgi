package audit

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const auditSecretSentinel = "TEST-CONNECTOR-SECRET-SENTINEL"

func TestGormRepositoryPersistsDecisionWithinOrganizationAndOwnerScope(t *testing.T) {
	repository, mock := openAuditRepositoryMock(t)
	ctx := context.Background()
	organizationID, ownerID := uuid.New(), uuid.New()
	decision := submittedDecision(organizationID, ownerID, uuid.New(), "logical-operation-1")
	decision.Details = map[string]any{
		"execution_path":      "connector_native",
		"risk_classification": "low",
		"confirmation":        false,
		"attempt":             float64(1),
	}

	expectCreateDecision(mock, decision)
	if err := repository.CreateDecision(ctx, decision); err != nil {
		t.Fatalf("CreateDecision() error = %v", err)
	}

	mock.ExpectQuery(`(?s)SELECT .* FROM "connector_lifecycle_decisions" WHERE organization_id = \$1 AND owner_account_id = \$2 AND id = \$3.*LIMIT \$4`).
		WithArgs(organizationID, ownerID, decision.ID, 1).
		WillReturnRows(decisionRows(decision))
	got, err := repository.GetDecision(ctx, organizationID, ownerID, decision.ID)
	if err != nil {
		t.Fatalf("GetDecision() error = %v", err)
	}
	if got.ID != decision.ID || got.Status != core.DecisionStatusSubmitted || got.Target == nil || *got.Target != *decision.Target {
		t.Fatalf("GetDecision() = %#v", got)
	}
	if got.Details["execution_path"] != "connector_native" || got.Details["confirmation"] != false {
		t.Fatalf("GetDecision().Details = %#v", got.Details)
	}

	for name, scope := range map[string][2]uuid.UUID{
		"other organization": {uuid.New(), ownerID},
		"other owner":        {organizationID, uuid.New()},
	} {
		t.Run(name, func(t *testing.T) {
			mock.ExpectQuery(`(?s)SELECT .* FROM "connector_lifecycle_decisions" WHERE organization_id = \$1 AND owner_account_id = \$2 AND id = \$3.*LIMIT \$4`).
				WithArgs(scope[0], scope[1], decision.ID, 1).
				WillReturnRows(sqlmock.NewRows(decisionColumns()))
			if _, err := repository.GetDecision(ctx, scope[0], scope[1], decision.ID); !core.IsErrorCode(err, core.ErrorCodeNotFound) {
				t.Fatalf("GetDecision() error = %v, want not found", err)
			}
		})
	}

	mock.ExpectQuery(`(?s)SELECT .* FROM "connector_lifecycle_decisions" WHERE .*organization_id = \$1 AND owner_account_id = \$2.*connector_id = \$3.*status IN \(\$4\).*ORDER BY created_at DESC, id DESC.*LIMIT \$5`).
		WithArgs(organizationID, ownerID, decision.ConnectorID.String(), core.DecisionStatusSubmitted, defaultDecisionListLimit).
		WillReturnRows(decisionRows(decision))
	items, err := repository.ListDecisions(ctx, organizationID, ownerID, DecisionListFilter{
		ConnectorID: decision.ConnectorID,
		Statuses:    []core.DecisionStatus{core.DecisionStatusSubmitted},
	})
	if err != nil || len(items) != 1 || items[0].ID != decision.ID {
		t.Fatalf("ListDecisions() = %#v, %v", items, err)
	}
	assertAuditMock(t, mock)
}

func TestGormRepositoryRoundTripsWorkflowTargetIdentity(t *testing.T) {
	repository, mock := openAuditRepositoryMock(t)
	decision := submittedDecision(uuid.New(), uuid.New(), uuid.New(), "workflow-operation-1")
	decision.Target = &core.TargetRef{
		Type:              core.TargetTypeWorkflow,
		Scope:             core.ScopeDraft,
		ID:                "workflow-42",
		BindingInstanceID: "node-7",
	}

	mock.ExpectQuery(`(?s)SELECT .* FROM "connector_lifecycle_decisions" WHERE organization_id = \$1 AND owner_account_id = \$2 AND id = \$3.*LIMIT \$4`).
		WithArgs(decision.OrganizationID, decision.OwnerAccountID, decision.ID, 1).
		WillReturnRows(decisionRows(decision))
	got, err := repository.GetDecision(context.Background(), decision.OrganizationID, decision.OwnerAccountID, decision.ID)
	if err != nil || got.Target == nil || *got.Target != *decision.Target {
		t.Fatalf("GetDecision() = %#v, %v", got, err)
	}

	outcome := outcomeForDecision(decision, core.OutcomeStatusSuccessful)
	mock.ExpectQuery(`(?s)SELECT .* FROM "connector_operation_outcomes" WHERE \(organization_id = \$1 AND owner_account_id = \$2\) AND id = \$3.*LIMIT \$4`).
		WithArgs(outcome.OrganizationID, outcome.OwnerAccountID, outcome.ID, 1).
		WillReturnRows(outcomeRows(outcome))
	gotOutcome, err := repository.GetOutcome(context.Background(), outcome.OrganizationID, outcome.OwnerAccountID, outcome.ID)
	if err != nil || gotOutcome.Target != outcome.Target {
		t.Fatalf("GetOutcome() = %#v, %v", gotOutcome, err)
	}
	assertAuditMock(t, mock)
}

func TestGormRepositoryRejectsSecretAndCauseDetailsBeforePersistence(t *testing.T) {
	repository, mock := openAuditRepositoryMock(t)
	ctx := context.Background()

	for name, details := range map[string]map[string]any{
		"secret field": {
			"provider": "github",
			"api_key":  auditSecretSentinel,
		},
		"nested secret field": {
			"provider": map[string]any{"accessToken": auditSecretSentinel},
		},
		"provider cause": {
			"provider_cause": errors.New(auditSecretSentinel),
		},
		"binary payload": {
			"provider_payload": []byte(auditSecretSentinel),
		},
	} {
		t.Run(name, func(t *testing.T) {
			decision := submittedDecision(uuid.New(), uuid.New(), uuid.New(), uuid.NewString())
			decision.Details = details
			if err := repository.CreateDecision(ctx, decision); !core.IsErrorCode(err, core.ErrorCodeInvalidInput) {
				t.Fatalf("CreateDecision() error = %v, want invalid input", err)
			}
		})
	}

	for _, model := range []reflect.Type{
		reflect.TypeOf(lifecycleDecisionRecord{}),
		reflect.TypeOf(operationOutcomeRecord{}),
	} {
		for index := 0; index < model.NumField(); index++ {
			field := model.Field(index)
			storageIdentity := strings.ToLower(field.Name + " " + field.Tag.Get("gorm"))
			for _, forbidden := range []string{"encrypted_credentials", "api_key", "secret", "cause"} {
				if strings.Contains(storageIdentity, forbidden) {
					t.Fatalf("%s contains forbidden storage identity %q", field.Name, forbidden)
				}
			}
		}
	}
	assertAuditMock(t, mock)
}

func TestGormRepositoryRecordsOnlyOutcomeForMatchingSubmittedDecision(t *testing.T) {
	repository, mock := openAuditRepositoryMock(t)
	ctx := context.Background()
	organizationID, ownerID := uuid.New(), uuid.New()
	decision := submittedDecision(organizationID, ownerID, uuid.New(), "logical-operation-2")
	outcome := outcomeForDecision(decision, core.OutcomeStatusSuccessful)
	outcome.ProviderRequestID = "provider-request-42"

	expectRecordOutcome(mock, decision, outcome, 0)
	if err := repository.RecordOutcome(ctx, outcome); err != nil {
		t.Fatalf("RecordOutcome() error = %v", err)
	}
	mock.ExpectQuery(`(?s)SELECT .* FROM "connector_operation_outcomes" WHERE \(organization_id = \$1 AND owner_account_id = \$2\) AND logical_operation_id = \$3.*LIMIT \$4`).
		WithArgs(organizationID, ownerID, decision.LogicalOperationID, 1).
		WillReturnRows(outcomeRows(outcome))
	got, err := repository.GetOutcomeByLogicalOperation(ctx, organizationID, ownerID, decision.LogicalOperationID)
	if err != nil || got.ID != outcome.ID || got.DecisionID != decision.ID || got.Status != core.OutcomeStatusSuccessful || got.Target != *decision.Target {
		t.Fatalf("GetOutcomeByLogicalOperation() = %#v, %v", got, err)
	}

	for name, scope := range map[string][2]uuid.UUID{
		"other organization": {uuid.New(), ownerID},
		"other owner":        {organizationID, uuid.New()},
	} {
		t.Run(name, func(t *testing.T) {
			mock.ExpectQuery(`(?s)SELECT .* FROM "connector_operation_outcomes" WHERE \(organization_id = \$1 AND owner_account_id = \$2\) AND id = \$3.*LIMIT \$4`).
				WithArgs(scope[0], scope[1], outcome.ID, 1).
				WillReturnRows(sqlmock.NewRows(outcomeColumns()))
			if _, err := repository.GetOutcome(ctx, scope[0], scope[1], outcome.ID); !core.IsErrorCode(err, core.ErrorCodeNotFound) {
				t.Fatalf("GetOutcome() error = %v, want not found", err)
			}
		})
	}

	mismatched := outcomeForDecision(decision, core.OutcomeStatusFailed)
	mismatched.ID = uuid.New()
	mismatched.OwnerAccountID = uuid.New()
	mismatched.ActingPrincipalID = mismatched.OwnerAccountID
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT .* FROM "connector_lifecycle_decisions" WHERE id = \$1 AND organization_id = \$2 AND owner_account_id = \$3 AND status = \$4.*LIMIT \$5 FOR SHARE`).
		WithArgs(mismatched.DecisionID, mismatched.OrganizationID, mismatched.OwnerAccountID, core.DecisionStatusSubmitted, 1).
		WillReturnRows(sqlmock.NewRows(decisionColumns()))
	mock.ExpectRollback()
	if err := repository.RecordOutcome(ctx, mismatched); !core.IsErrorCode(err, core.ErrorCodeNotFound) {
		t.Fatalf("cross-owner RecordOutcome() error = %v, want not found", err)
	}
	assertAuditMock(t, mock)
}

func TestGormRepositoryRejectsOutcomeForNonSubmittedDecision(t *testing.T) {
	repository, mock := openAuditRepositoryMock(t)
	decision := submittedDecision(uuid.New(), uuid.New(), uuid.New(), "logical-operation-blocked")
	outcome := outcomeForDecision(decision, core.OutcomeStatusFailed)

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT .* FROM "connector_lifecycle_decisions" WHERE id = \$1 AND organization_id = \$2 AND owner_account_id = \$3 AND status = \$4.*LIMIT \$5 FOR SHARE`).
		WithArgs(decision.ID, decision.OrganizationID, decision.OwnerAccountID, core.DecisionStatusSubmitted, 1).
		WillReturnRows(sqlmock.NewRows(decisionColumns()))
	mock.ExpectRollback()
	if err := repository.RecordOutcome(context.Background(), outcome); !core.IsErrorCode(err, core.ErrorCodeNotFound) {
		t.Fatalf("RecordOutcome() error = %v, want not found", err)
	}
	assertAuditMock(t, mock)
}

func TestGormRepositoryEnforcesOneTerminalOutcome(t *testing.T) {
	repository, mock := openAuditRepositoryMock(t)
	decision := submittedDecision(uuid.New(), uuid.New(), uuid.New(), "logical-operation-unique")
	outcome := outcomeForDecision(decision, core.OutcomeStatusUnknown)
	outcome.DiagnosticCode = string(core.ErrorCodeOutcomeUnknown)

	expectRecordOutcome(mock, decision, outcome, 1)
	if err := repository.RecordOutcome(context.Background(), outcome); !core.IsErrorCode(err, core.ErrorCodeConflict) {
		t.Fatalf("RecordOutcome() error = %v, want conflict", err)
	}
	assertAuditMock(t, mock)
}

func TestGormRepositoryAllowsSameLogicalOperationInAnotherOrganization(t *testing.T) {
	repository, mock := openAuditRepositoryMock(t)
	decision := submittedDecision(uuid.New(), uuid.New(), uuid.New(), "logical-operation-org-local")
	outcome := outcomeForDecision(decision, core.OutcomeStatusSuccessful)
	expectRecordOutcome(mock, decision, outcome, 0)
	if err := repository.RecordOutcome(context.Background(), outcome); err != nil {
		t.Fatalf("RecordOutcome() error = %v", err)
	}
	assertAuditMock(t, mock)
}

func TestGormRepositoryRejectsDuplicateDecisionIdentity(t *testing.T) {
	repository, mock := openAuditRepositoryMock(t)
	decision := submittedDecision(uuid.New(), uuid.New(), uuid.New(), "logical-operation-original")
	mock.ExpectQuery(`(?s)SELECT count\(\*\) FROM "connector_lifecycle_decisions" WHERE id = \$1 OR \(organization_id = \$2 AND logical_operation_id = \$3 AND decision = \$4 AND status = \$5\)`).
		WithArgs(decision.ID, decision.OrganizationID, decision.LogicalOperationID, decision.Decision, decision.Status).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	if err := repository.CreateDecision(context.Background(), decision); !core.IsErrorCode(err, core.ErrorCodeConflict) {
		t.Fatalf("CreateDecision() error = %v, want conflict", err)
	}
	assertAuditMock(t, mock)
}

func TestGormRepositoryMapsConcurrentTerminalInsertConflict(t *testing.T) {
	t.Run("decision", func(t *testing.T) {
		repository, mock := openAuditRepositoryMock(t)
		decision := submittedDecision(uuid.New(), uuid.New(), uuid.New(), "logical-operation-decision-race")
		mock.ExpectQuery(`(?s)SELECT count\(\*\) FROM "connector_lifecycle_decisions" WHERE .*`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectBegin()
		mock.ExpectExec(`(?s)INSERT INTO "connector_lifecycle_decisions" .*ON CONFLICT DO NOTHING`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()
		if err := repository.CreateDecision(context.Background(), decision); !core.IsErrorCode(err, core.ErrorCodeConflict) {
			t.Fatalf("CreateDecision() error = %v, want conflict", err)
		}
		assertAuditMock(t, mock)
	})

	t.Run("outcome", func(t *testing.T) {
		repository, mock := openAuditRepositoryMock(t)
		decision := submittedDecision(uuid.New(), uuid.New(), uuid.New(), "logical-operation-outcome-race")
		outcome := outcomeForDecision(decision, core.OutcomeStatusSuccessful)
		mock.ExpectBegin()
		mock.ExpectQuery(`(?s)SELECT .* FROM "connector_lifecycle_decisions".*FOR SHARE`).
			WillReturnRows(decisionRows(decision))
		mock.ExpectQuery(`(?s)SELECT count\(\*\) FROM "connector_operation_outcomes" WHERE .*`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectExec(`(?s)INSERT INTO "connector_operation_outcomes" .*ON CONFLICT DO NOTHING`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectRollback()
		if err := repository.RecordOutcome(context.Background(), outcome); !core.IsErrorCode(err, core.ErrorCodeConflict) {
			t.Fatalf("RecordOutcome() error = %v, want conflict", err)
		}
		assertAuditMock(t, mock)
	})
}

func TestGormRepositoryFinalizesConfirmationOnceByRouteReceipt(t *testing.T) {
	repository, mock := openAuditRepositoryMock(t)
	base := submittedDecision(uuid.New(), uuid.New(), uuid.New(), "logical-confirmation-cas")
	request := ExecutionDecisionRequest{
		ID: uuid.New(), OrganizationID: base.OrganizationID, OwnerAccountID: base.OwnerAccountID,
		ActingPrincipalID: base.ActingPrincipalID, ConnectorID: base.ConnectorID,
		ConnectionID: base.ConnectionID, Target: *base.Target, OperationID: base.OperationID,
		LogicalOperationID: base.LogicalOperationID, CorrelationID: base.CorrelationID,
		ReceiptID: "route-receipt-" + uuid.NewString(), ExecutionPath: "connector_native",
		RiskClassification: "high", Interactive: true, CreatedAt: time.Now().UTC(),
	}
	confirmed, err := NewConfirmationFinalizationDecision(request, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateDecision(context.Background(), confirmed); !core.IsErrorCode(err, core.ErrorCodeInvalidInput) {
		t.Fatalf("generic CreateDecision(finalization) error = %v, want invalid input", err)
	}
	request.ID = uuid.New()
	request.Confirmation = true
	request.ExecutionLeaseExpiresAt = request.CreatedAt.Add(time.Minute)
	submitted, err := NewExecutionDecision(request, core.DecisionStatusSubmitted)
	if err != nil {
		t.Fatal(err)
	}
	lockIdentity := confirmed.OrganizationID.String() + "/" + confirmed.ReceiptID
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock\(hashtextextended\(\$1, 0\)\)`).
		WithArgs(lockIdentity).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT count\(\*\) FROM "connector_lifecycle_decisions" WHERE organization_id = \$1 AND receipt_id = \$2 AND decision = \$3`).
		WithArgs(confirmed.OrganizationID, confirmed.ReceiptID, confirmed.Decision).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	expectOpenExecutionClaim(mock, confirmed)
	mock.ExpectExec(`(?s)INSERT INTO "connector_lifecycle_decisions" .*ON CONFLICT DO NOTHING`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)INSERT INTO "connector_lifecycle_decisions" .*ON CONFLICT DO NOTHING`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := repository.FinalizeConfirmation(context.Background(), confirmed, submitted); err != nil {
		t.Fatalf("FinalizeConfirmation(confirm) error = %v", err)
	}

	request.ID = uuid.New()
	request.Confirmation = false
	request.ExecutionLeaseExpiresAt = time.Time{}
	cancelled, err := NewConfirmationFinalizationDecision(request, false)
	if err != nil {
		t.Fatal(err)
	}
	request.ID = uuid.New()
	cancelDecision, err := NewExecutionDecision(request, core.DecisionStatusCancelled)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock\(hashtextextended\(\$1, 0\)\)`).
		WithArgs(lockIdentity).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT count\(\*\) FROM "connector_lifecycle_decisions" WHERE organization_id = \$1 AND receipt_id = \$2 AND decision = \$3`).
		WithArgs(cancelled.OrganizationID, cancelled.ReceiptID, cancelled.Decision).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectRollback()
	if err := repository.FinalizeConfirmation(context.Background(), cancelled, cancelDecision); !core.IsErrorCode(err, core.ErrorCodeConflict) {
		t.Fatalf("FinalizeConfirmation(cancel after confirm) error = %v, want conflict", err)
	}
	assertAuditMock(t, mock)
}

func TestGormRepositoryRollsBackFinalizationWhenCompanionDecisionWriteFails(t *testing.T) {
	repository, mock := openAuditRepositoryMock(t)
	finalization, submitted := confirmationDecisionPair(t, true)
	lockIdentity := finalization.OrganizationID.String() + "/" + finalization.ReceiptID

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock\(hashtextextended\(\$1, 0\)\)`).
		WithArgs(lockIdentity).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT count\(\*\) FROM "connector_lifecycle_decisions" WHERE organization_id = \$1 AND receipt_id = \$2 AND decision = \$3`).
		WithArgs(finalization.OrganizationID, finalization.ReceiptID, finalization.Decision).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	expectOpenExecutionClaim(mock, finalization)
	mock.ExpectExec(`(?s)INSERT INTO "connector_lifecycle_decisions" .*ON CONFLICT DO NOTHING`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)INSERT INTO "connector_lifecycle_decisions" .*ON CONFLICT DO NOTHING`).
		WillReturnError(errors.New(auditSecretSentinel))
	mock.ExpectRollback()

	if err := repository.FinalizeConfirmation(context.Background(), finalization, submitted); !core.IsErrorCode(err, core.ErrorCodeInternal) {
		t.Fatalf("FinalizeConfirmation() error = %v, want internal", err)
	}
	assertAuditMock(t, mock)
}

func TestGormRepositoryConvergesMissingOutcomeOnceAndReturnsExistingEvidence(t *testing.T) {
	t.Run("records durable unknown", func(t *testing.T) {
		repository, mock := openAuditRepositoryMock(t)
		decision := submittedDecision(uuid.New(), uuid.New(), uuid.New(), "logical-converge-unknown")
		unknown := outcomeForDecision(decision, core.OutcomeStatusUnknown)
		unknown.ProviderRequestID = ""
		unknown.DiagnosticCode = string(core.ErrorCodeOutcomeUnknown)

		mock.ExpectBegin()
		mock.ExpectQuery(`(?s)SELECT .* FROM "connector_lifecycle_decisions" WHERE id = \$1 AND organization_id = \$2 AND owner_account_id = \$3 AND status = \$4.*LIMIT \$5 FOR UPDATE`).
			WithArgs(decision.ID, decision.OrganizationID, decision.OwnerAccountID, core.DecisionStatusSubmitted, 1).
			WillReturnRows(decisionRows(decision))
		mock.ExpectQuery(`(?s)SELECT .* FROM "connector_operation_outcomes" WHERE \(organization_id = \$1 AND owner_account_id = \$2\) AND \(decision_id = \$3 OR logical_operation_id = \$4\).*LIMIT \$5`).
			WithArgs(unknown.OrganizationID, unknown.OwnerAccountID, unknown.DecisionID, unknown.LogicalOperationID, 1).
			WillReturnRows(sqlmock.NewRows(outcomeColumns()))
		mock.ExpectExec(`(?s)INSERT INTO "connector_operation_outcomes" .*ON CONFLICT DO NOTHING`).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		durable, err := repository.ConvergeOutcome(context.Background(), unknown)
		if err != nil || !reflect.DeepEqual(durable, unknown) {
			t.Fatalf("ConvergeOutcome() = %#v, %v", durable, err)
		}
		assertAuditMock(t, mock)
	})

	t.Run("preserves existing provider outcome", func(t *testing.T) {
		repository, mock := openAuditRepositoryMock(t)
		decision := submittedDecision(uuid.New(), uuid.New(), uuid.New(), "logical-converge-existing")
		unknown := outcomeForDecision(decision, core.OutcomeStatusUnknown)
		unknown.ProviderRequestID = ""
		unknown.DiagnosticCode = string(core.ErrorCodeOutcomeUnknown)
		existing := outcomeForDecision(decision, core.OutcomeStatusSuccessful)

		mock.ExpectBegin()
		mock.ExpectQuery(`(?s)SELECT .* FROM "connector_lifecycle_decisions" WHERE id = \$1 AND organization_id = \$2 AND owner_account_id = \$3 AND status = \$4.*LIMIT \$5 FOR UPDATE`).
			WithArgs(decision.ID, decision.OrganizationID, decision.OwnerAccountID, core.DecisionStatusSubmitted, 1).
			WillReturnRows(decisionRows(decision))
		mock.ExpectQuery(`(?s)SELECT .* FROM "connector_operation_outcomes" WHERE \(organization_id = \$1 AND owner_account_id = \$2\) AND \(decision_id = \$3 OR logical_operation_id = \$4\).*LIMIT \$5`).
			WithArgs(unknown.OrganizationID, unknown.OwnerAccountID, unknown.DecisionID, unknown.LogicalOperationID, 1).
			WillReturnRows(outcomeRows(existing))
		mock.ExpectCommit()

		durable, err := repository.ConvergeOutcome(context.Background(), unknown)
		if err != nil || !reflect.DeepEqual(durable, existing) {
			t.Fatalf("ConvergeOutcome() = %#v, %v", durable, err)
		}
		assertAuditMock(t, mock)
	})
}

func confirmationDecisionPair(
	t *testing.T,
	confirmed bool,
) (core.LifecycleDecision, core.LifecycleDecision) {
	t.Helper()
	base := submittedDecision(uuid.New(), uuid.New(), uuid.New(), "logical-confirmation-pair")
	request := ExecutionDecisionRequest{
		ID: uuid.New(), OrganizationID: base.OrganizationID, OwnerAccountID: base.OwnerAccountID,
		ActingPrincipalID: base.ActingPrincipalID, ConnectorID: base.ConnectorID,
		ConnectionID: base.ConnectionID, Target: *base.Target, OperationID: base.OperationID,
		LogicalOperationID: base.LogicalOperationID, CorrelationID: base.CorrelationID,
		ReceiptID: "route-receipt-" + uuid.NewString(), ExecutionPath: "connector_native",
		RiskClassification: "high", Interactive: true, Confirmation: confirmed, CreatedAt: time.Now().UTC(),
	}
	finalization, err := NewConfirmationFinalizationDecision(request, confirmed)
	if err != nil {
		t.Fatal(err)
	}
	request.ID = uuid.New()
	status := core.DecisionStatusCancelled
	if confirmed {
		status = core.DecisionStatusSubmitted
		request.ExecutionLeaseExpiresAt = request.CreatedAt.Add(time.Minute)
	}
	operationDecision, err := NewExecutionDecision(request, status)
	if err != nil {
		t.Fatal(err)
	}
	return finalization, operationDecision
}

func openAuditRepositoryMock(t *testing.T) (*GormRepository, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		DriverName:           "postgres",
		PreferSimpleProtocol: true,
	}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	return NewGormRepository(db), mock
}

func expectCreateDecision(mock sqlmock.Sqlmock, decision core.LifecycleDecision) {
	mock.ExpectQuery(`(?s)SELECT count\(\*\) FROM "connector_lifecycle_decisions" WHERE id = \$1 OR \(organization_id = \$2 AND logical_operation_id = \$3 AND decision = \$4 AND status = \$5\)`).
		WithArgs(decision.ID, decision.OrganizationID, decision.LogicalOperationID, decision.Decision, decision.Status).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectBegin()
	mock.ExpectExec(`(?s)INSERT INTO "connector_lifecycle_decisions" .*ON CONFLICT DO NOTHING`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

func expectOpenExecutionClaim(mock sqlmock.Sqlmock, decision core.LifecycleDecision) {
	target := decision.Target
	mock.ExpectQuery(`(?s)SELECT count\(\*\) FROM "connector_invocation_claims" WHERE .*state IN`).
		WithArgs(decision.OrganizationID, decision.OwnerAccountID, decision.ActingPrincipalID, decision.ReceiptID,
			decision.LogicalOperationID, decision.ConnectorID, decision.ConnectionID, decision.OperationID,
			target.Type, target.ID, target.EffectiveScope(), target.VersionID, target.BindingInstanceID, "claimed", "awaiting_confirmation").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
}

func expectRecordOutcome(mock sqlmock.Sqlmock, decision core.LifecycleDecision, outcome core.OperationOutcome, conflictCount int64) {
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT .* FROM "connector_lifecycle_decisions" WHERE id = \$1 AND organization_id = \$2 AND owner_account_id = \$3 AND status = \$4.*LIMIT \$5 FOR SHARE`).
		WithArgs(decision.ID, decision.OrganizationID, decision.OwnerAccountID, core.DecisionStatusSubmitted, 1).
		WillReturnRows(decisionRows(decision))
	mock.ExpectQuery(`(?s)SELECT count\(\*\) FROM "connector_operation_outcomes" WHERE \(organization_id = \$1 AND owner_account_id = \$2\) AND \(id = \$3 OR decision_id = \$4 OR logical_operation_id = \$5\)`).
		WithArgs(outcome.OrganizationID, outcome.OwnerAccountID, outcome.ID, outcome.DecisionID, outcome.LogicalOperationID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(conflictCount))
	if conflictCount > 0 {
		mock.ExpectRollback()
		return
	}
	mock.ExpectExec(`(?s)INSERT INTO "connector_operation_outcomes" .*ON CONFLICT DO NOTHING`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

func decisionColumns() []string {
	return []string{
		"id", "organization_id", "owner_account_id", "acting_principal_id", "connector_id", "connection_id",
		"target_type", "target_id", "target_scope", "target_version_id", "binding_instance_id", "operation_id", "logical_operation_id", "decision", "status",
		"correlation_id", "receipt_id", "details", "created_at",
	}
}

func decisionRows(decision core.LifecycleDecision) *sqlmock.Rows {
	encodedDetails, _ := json.Marshal(decision.Details)
	var targetType, targetID, targetScope, targetVersionID, bindingInstanceID any
	if decision.Target != nil {
		targetType = decision.Target.Type
		targetID = decision.Target.ID
		targetScope = string(decision.Target.EffectiveScope())
		targetVersionID = decision.Target.VersionID
		bindingInstanceID = optionalDriverString(decision.Target.BindingInstanceID)
	}
	return sqlmock.NewRows(decisionColumns()).AddRow(
		decision.ID, decision.OrganizationID, decision.OwnerAccountID, decision.ActingPrincipalID, decision.ConnectorID.String(), optionalDriverUUID(decision.ConnectionID),
		targetType, targetID, targetScope, targetVersionID, bindingInstanceID, optionalDriverString(decision.OperationID.String()), optionalDriverString(decision.LogicalOperationID), decision.Decision, decision.Status,
		decision.CorrelationID, optionalDriverString(decision.ReceiptID), encodedDetails, decision.CreatedAt,
	)
}

func outcomeColumns() []string {
	return []string{
		"id", "decision_id", "organization_id", "owner_account_id", "acting_principal_id", "connector_id", "connection_id",
		"target_type", "target_id", "target_scope", "target_version_id", "binding_instance_id", "operation_id", "logical_operation_id", "status", "provider_request_id",
		"diagnostic_code", "created_at",
	}
}

func outcomeRows(outcome core.OperationOutcome) *sqlmock.Rows {
	return sqlmock.NewRows(outcomeColumns()).AddRow(
		outcome.ID, outcome.DecisionID, outcome.OrganizationID, outcome.OwnerAccountID, outcome.ActingPrincipalID, outcome.ConnectorID.String(), outcome.ConnectionID,
		outcome.Target.Type, outcome.Target.ID, string(outcome.Target.EffectiveScope()), outcome.Target.VersionID, optionalDriverString(outcome.Target.BindingInstanceID), outcome.OperationID.String(), outcome.LogicalOperationID,
		outcome.Status, optionalDriverString(outcome.ProviderRequestID), optionalDriverString(outcome.DiagnosticCode), outcome.CreatedAt,
	)
}

func optionalDriverString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func optionalDriverUUID(value uuid.UUID) any {
	if value == uuid.Nil {
		return nil
	}
	return value
}

func submittedDecision(organizationID, ownerID, decisionID uuid.UUID, logicalOperationID string) core.LifecycleDecision {
	return core.LifecycleDecision{
		ID:                 decisionID,
		OrganizationID:     organizationID,
		OwnerAccountID:     ownerID,
		ActingPrincipalID:  ownerID,
		ConnectorID:        core.Identifier("github"),
		ConnectionID:       uuid.New(),
		Target:             &core.TargetRef{Type: core.TargetTypeChat, Scope: core.ScopeLive, ID: "conversation-1"},
		OperationID:        core.Identifier("github.create_issue"),
		LogicalOperationID: logicalOperationID,
		Decision:           "dispatch",
		Status:             core.DecisionStatusSubmitted,
		CorrelationID:      uuid.NewString(),
		ReceiptID:          uuid.NewString(),
		Details:            map[string]any{},
		CreatedAt:          time.Now().UTC().Truncate(time.Microsecond),
	}
}

func outcomeForDecision(decision core.LifecycleDecision, status core.OutcomeStatus) core.OperationOutcome {
	return core.OperationOutcome{
		ID:                 uuid.New(),
		DecisionID:         decision.ID,
		OrganizationID:     decision.OrganizationID,
		OwnerAccountID:     decision.OwnerAccountID,
		ActingPrincipalID:  decision.ActingPrincipalID,
		ConnectorID:        decision.ConnectorID,
		ConnectionID:       decision.ConnectionID,
		Target:             *decision.Target,
		OperationID:        decision.OperationID,
		LogicalOperationID: decision.LogicalOperationID,
		Status:             status,
		CreatedAt:          time.Now().UTC().Truncate(time.Microsecond),
	}
}

func assertAuditMock(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}
