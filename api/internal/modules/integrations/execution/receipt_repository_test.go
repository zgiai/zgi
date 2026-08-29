package execution

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	integrations "github.com/zgiai/zgi/api/internal/modules/integrations/core"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestOperationReceiptClaimRejectsFrozenInputMismatchBeforeAnyExistingStateTransition(t *testing.T) {
	now := time.Now().UTC()
	startedAt := now.Add(-2 * time.Minute)
	tests := []struct {
		name              string
		status            string
		leaseExpiresAt    time.Time
		providerStartedAt *time.Time
	}{
		{name: "succeeded", status: integrations.OperationReceiptStatusSucceeded, leaseExpiresAt: now.Add(time.Minute)},
		{name: "active executing", status: integrations.OperationReceiptStatusExecuting, leaseExpiresAt: now.Add(time.Minute)},
		{name: "expired before provider start", status: integrations.OperationReceiptStatusExecuting, leaseExpiresAt: now.Add(-time.Minute)},
		{name: "expired after provider start", status: integrations.OperationReceiptStatusExecuting, leaseExpiresAt: now.Add(-time.Minute), providerStartedAt: &startedAt},
		{name: "outcome unknown", status: integrations.OperationReceiptStatusOutcomeUnknown, leaseExpiresAt: now.Add(-time.Minute), providerStartedAt: &startedAt},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, mock := newReceiptTestDB(t)
			repository := NewGormOperationReceiptRepository(db)
			existing := receiptTestRecord(now)
			existing.Status = test.status
			existing.LeaseExpiresAt = test.leaseExpiresAt
			existing.ProviderStartedAt = test.providerStartedAt

			candidate := receiptTestRecord(now)
			candidate.OrganizationID = existing.OrganizationID
			candidate.OperationKey = existing.OperationKey
			candidate.ID = uuid.New()
			candidate.ClaimToken = uuid.New()
			candidate.FrozenInputHMAC = strings.Repeat("b", 64)

			mock.ExpectBegin()
			mock.ExpectQuery(`INSERT INTO "integration_operation_receipts"`).
				WithArgs(anySQLArgs(25)...).
				WillReturnRows(sqlmock.NewRows([]string{"result_payload"}))
			mock.ExpectCommit()
			mock.ExpectQuery(`SELECT \* FROM "integration_operation_receipts".*organization_id.*operation_key.*LIMIT`).
				WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), 1).
				WillReturnRows(operationReceiptRows(existing))
			claim, err := repository.Claim(context.Background(), candidate)
			if !errors.Is(err, integrations.ErrOperationReceiptFrozenInputMismatch) || claim.Receipt != nil || claim.Claimed {
				t.Fatalf("Claim() = %#v, %v; want frozen-input mismatch", claim, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("database expectations: %v", err)
			}
		})
	}
}

func TestOperationReceiptFrozenInputMatchRequiresValidFixedSizeHMACs(t *testing.T) {
	valid := strings.Repeat("a", 64)
	tests := []struct {
		name      string
		existing  string
		candidate string
		want      bool
	}{
		{name: "equal", existing: valid, candidate: valid, want: true},
		{name: "different", existing: valid, candidate: strings.Repeat("b", 64)},
		{name: "missing existing", candidate: valid},
		{name: "missing candidate", existing: valid},
		{name: "short existing", existing: strings.Repeat("a", 63), candidate: valid},
		{name: "long candidate", existing: valid, candidate: strings.Repeat("a", 65)},
		{name: "non-hex existing", existing: strings.Repeat("z", 64), candidate: valid},
		{name: "non-hex candidate", existing: valid, candidate: strings.Repeat("z", 64)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := integrations.OperationReceiptFrozenInputMatches(
				&integrations.OperationReceipt{FrozenInputHMAC: test.existing},
				&integrations.OperationReceipt{FrozenInputHMAC: test.candidate},
			)
			if got != test.want {
				t.Fatalf("OperationReceiptFrozenInputMatches() = %v, want %v", got, test.want)
			}
		})
	}
}

func newReceiptTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, DriverName: "postgres"}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	return db, mock
}

func operationReceiptRows(receipt *integrations.OperationReceipt) *sqlmock.Rows {
	columns := []string{
		"id", "organization_id", "workspace_id", "conversation_id", "message_id", "connection_id",
		"batch_id", "operation_item_id", "item_index", "item_count", "integration_id", "action_id",
		"operation_key", "target_hmac", "frozen_input_hmac", "status", "claim_token", "execution_id",
		"provider_request_id", "result_payload", "result_count", "provider_started_at", "lease_expires_at",
		"created_at", "updated_at",
	}
	return sqlmock.NewRows(columns).AddRow(
		receipt.ID.String(), receipt.OrganizationID.String(), receipt.WorkspaceID,
		receipt.ConversationID.String(), receipt.MessageID.String(), receipt.ConnectionID.String(),
		receipt.BatchID, receipt.OperationItemID, receipt.ItemIndex, receipt.ItemCount,
		receipt.IntegrationID, receipt.ActionID, receipt.OperationKey, receipt.TargetHMAC,
		receipt.FrozenInputHMAC, receipt.Status, receipt.ClaimToken.String(), receipt.ExecutionID,
		receipt.ProviderRequestID, `{}`, receipt.ResultCount, receipt.ProviderStartedAt,
		receipt.LeaseExpiresAt, receipt.CreatedAt, receipt.UpdatedAt,
	)
}

func receiptTestRecord(now time.Time) *integrations.OperationReceipt {
	return &integrations.OperationReceipt{
		ID:              uuid.New(),
		OrganizationID:  uuid.New(),
		ConversationID:  uuid.New(),
		MessageID:       uuid.New(),
		ConnectionID:    uuid.New(),
		BatchID:         "single-message",
		OperationItemID: "single",
		ItemIndex:       1,
		ItemCount:       1,
		IntegrationID:   "test",
		ActionID:        "message.send",
		OperationKey:    strings.Repeat("c", 64),
		TargetHMAC:      strings.Repeat("d", 64),
		FrozenInputHMAC: strings.Repeat("a", 64),
		Status:          integrations.OperationReceiptStatusExecuting,
		ClaimToken:      uuid.New(),
		ResultPayload:   map[string]interface{}{},
		LeaseExpiresAt:  now.Add(time.Minute),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}
