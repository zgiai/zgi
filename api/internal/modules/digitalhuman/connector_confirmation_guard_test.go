package digitalhuman

import (
	"context"
	"database/sql/driver"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectorexecution "github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	connectorconfirmation "github.com/zgiai/zgi/api/internal/modules/connectors/runtimeconfirmation"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// connectorConfirmationGuardCountQuery is the exact parameterized count the
// guard must issue. Every dynamic predicate is a bound placeholder and the SQL
// keeps the external-surface exception explicit.
const connectorConfirmationGuardCountQuery = `(?s)SELECT count\(\*\) FROM "digital_human_runs" WHERE id = \$1 AND organization_id = \$2 AND \(actor_id = \$3 OR user_subject_id = \$4\) AND digital_human_id = \$5 AND digital_human_version_id = \$6 AND conversation_id = \$7 AND message_id = \$8 AND status = \$9 AND waiting_reason = \$10 AND waiting_receipt = \$11 AND cancel_requested_at IS NULL`

func newConnectorConfirmationGuardTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn: sqlDB, PreferSimpleProtocol: true,
	}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	return db, mock
}

func newConnectorConfirmationGuardRequest() (
	connectorconfirmation.DigitalHumanContinuationGuardRequest,
	[]driver.Value,
) {
	receipt := uuid.New()
	request := connectorconfirmation.DigitalHumanContinuationGuardRequest{
		OrganizationID:        uuid.New(),
		OwnerAccountID:        uuid.New(),
		DeciderID:             uuid.New(),
		DigitalHumanID:        uuid.New(),
		DigitalHumanVersionID: uuid.New(),
		RunID:                 uuid.New(),
		ConversationID:        uuid.New(),
		MessageID:             uuid.New(),
		RouteReceipt:          connectorexecution.RouteClaimReceipt(receipt.String()),
	}
	return request, []driver.Value{
		request.RunID, request.OrganizationID, request.DeciderID, request.DeciderID,
		request.DigitalHumanID, request.DigitalHumanVersionID,
		request.ConversationID, request.MessageID,
		RunStatusWaiting, runtimemodel.MessageStatusWaitingApproval, receipt,
	}
}

func TestConnectorConfirmationGuardAllowsExactWaitingRun(t *testing.T) {
	db, mock := newConnectorConfirmationGuardTestDB(t)
	guard, err := NewConnectorConfirmationGuard(db)
	if err != nil {
		t.Fatalf("NewConnectorConfirmationGuard() error = %v", err)
	}
	request, args := newConnectorConfirmationGuardRequest()
	mock.ExpectQuery(connectorConfirmationGuardCountQuery).
		WithArgs(args...).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	if err := guard.AuthorizeContinuation(context.Background(), request); err != nil {
		t.Fatalf("AuthorizeContinuation(waiting run) error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestConnectorConfirmationGuardDeniesNonMatchingDurableState(t *testing.T) {
	tests := []struct {
		name string
		rows int64
	}{
		{"missing run", 0},
		{"canceled or mismatched run", 0},
		{"duplicate rows", 2},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			db, mock := newConnectorConfirmationGuardTestDB(t)
			guard, err := NewConnectorConfirmationGuard(db)
			if err != nil {
				t.Fatalf("NewConnectorConfirmationGuard() error = %v", err)
			}
			request, args := newConnectorConfirmationGuardRequest()
			mock.ExpectQuery(connectorConfirmationGuardCountQuery).
				WithArgs(args...).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(testCase.rows))

			err = guard.AuthorizeContinuation(context.Background(), request)
			if !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeConflict) {
				t.Fatalf("AuthorizeContinuation() error = %v, want conflict", err)
			}
			if err.Error() != string(connectorcore.ErrorCodeConflict) {
				t.Fatalf("guard error leaked durable identity: %q", err.Error())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet SQL expectations: %v", err)
			}
		})
	}
}

func TestConnectorConfirmationGuardFailsClosedOnDurableReadError(t *testing.T) {
	db, mock := newConnectorConfirmationGuardTestDB(t)
	guard, err := NewConnectorConfirmationGuard(db)
	if err != nil {
		t.Fatalf("NewConnectorConfirmationGuard() error = %v", err)
	}
	request, args := newConnectorConfirmationGuardRequest()
	mock.ExpectQuery(connectorConfirmationGuardCountQuery).
		WithArgs(args...).
		WillReturnError(errors.New("durable read failed"))

	if err := guard.AuthorizeContinuation(context.Background(), request); !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeInternal) {
		t.Fatalf("AuthorizeContinuation(read error) error = %v, want internal", err)
	}
}

func TestConnectorConfirmationGuardRejectsInvalidRequestWithoutReadingDurableState(t *testing.T) {
	db, mock := newConnectorConfirmationGuardTestDB(t)
	guard, err := NewConnectorConfirmationGuard(db)
	if err != nil {
		t.Fatalf("NewConnectorConfirmationGuard() error = %v", err)
	}
	valid, _ := newConnectorConfirmationGuardRequest()
	nonCanonicalReceipt := connectorexecution.RouteClaimReceipt(uuid.New().String() + " ")
	tests := []struct {
		name   string
		mutate func(*connectorconfirmation.DigitalHumanContinuationGuardRequest)
	}{
		{"empty", func(request *connectorconfirmation.DigitalHumanContinuationGuardRequest) {
			*request = connectorconfirmation.DigitalHumanContinuationGuardRequest{}
		}},
		{"missing run", func(request *connectorconfirmation.DigitalHumanContinuationGuardRequest) {
			request.RunID = uuid.Nil
		}},
		{"missing owner", func(request *connectorconfirmation.DigitalHumanContinuationGuardRequest) {
			request.OwnerAccountID = uuid.Nil
		}},
		{"missing decider", func(request *connectorconfirmation.DigitalHumanContinuationGuardRequest) {
			request.DeciderID = uuid.Nil
		}},
		{"missing published version", func(request *connectorconfirmation.DigitalHumanContinuationGuardRequest) {
			request.DigitalHumanVersionID = uuid.Nil
		}},
		{"non canonical receipt", func(request *connectorconfirmation.DigitalHumanContinuationGuardRequest) {
			request.RouteReceipt = nonCanonicalReceipt
		}},
		{"receipt not a uuid", func(request *connectorconfirmation.DigitalHumanContinuationGuardRequest) {
			request.RouteReceipt = connectorexecution.RouteClaimReceipt("forged-receipt")
		}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			request := valid
			testCase.mutate(&request)
			err := guard.AuthorizeContinuation(context.Background(), request)
			if !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeConflict) {
				t.Fatalf("AuthorizeContinuation() error = %v, want conflict", err)
			}
		})
	}
	// No durable read may happen for a malformed request.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("malformed request reached durable state: %v", err)
	}
}

func TestNewConnectorConfirmationGuardRejectsNilDatabase(t *testing.T) {
	if guard, err := NewConnectorConfirmationGuard(nil); guard != nil || !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeInternal) {
		t.Fatalf("NewConnectorConfirmationGuard(nil) = %#v, %v, want internal error", guard, err)
	}
	var guard *ConnectorConfirmationGuard
	if err := guard.AuthorizeContinuation(context.Background(), connectorconfirmation.DigitalHumanContinuationGuardRequest{}); !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeConflict) {
		t.Fatalf("nil guard AuthorizeContinuation() error = %v, want conflict", err)
	}
}
