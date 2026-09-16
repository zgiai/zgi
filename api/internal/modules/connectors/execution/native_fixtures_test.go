package execution

import (
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"testing"
	"time"
)

func testExecutionTime() time.Time { return time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC) }
func openExecutionPersistenceMock(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	conn, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: conn}), &gorm.Config{DisableAutomaticPing: true, SkipDefaultTransaction: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return db, mock
}

func validExecutionRequest() ExecutionRequest {
	ownerID := uuid.New()
	return ExecutionRequest{
		OrganizationID: uuid.New(), OwnerAccountID: ownerID, ActingPrincipalID: ownerID,
		BindingID: uuid.New(), ConnectionID: uuid.New(), ConnectorID: core.Identifier("github"),
		Target:      core.TargetRef{Type: core.TargetTypeChat, ID: "conversation-1"},
		OperationID: core.Identifier("github.create_issue"), LogicalOperationID: uuid.NewString(),
		CorrelationID: uuid.NewString(), Interactive: true, Input: map[string]any{"title": "private-operation-input"},
	}
}
