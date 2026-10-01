package migrations

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/audit"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestConnectorSameCorrelationPostgres(t *testing.T) {
	dsn := os.Getenv("ZGI_CONNECTOR_DETACH_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL test DSN required")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal("invalid test configuration")
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	schema := "connector_multi_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = conn.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = conn.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE") }()
	fixture := detachmentFixture + detachConnectorRuntimeSQL + `
ALTER TABLE public.connector_lifecycle_decisions ADD COLUMN correlation_id text, ADD COLUMN receipt_id text,
 ADD COLUMN details jsonb, ADD COLUMN created_at timestamptz;
ALTER TABLE public.connector_operation_outcomes ADD COLUMN provider_request_id text,
 ADD COLUMN diagnostic_code text, ADD COLUMN created_at timestamptz;
CREATE UNIQUE INDEX idx_connector_decisions_operation_identity ON public.connector_lifecycle_decisions
 (organization_id,logical_operation_id,decision,status) WHERE logical_operation_id IS NOT NULL AND logical_operation_id <> '';
CREATE UNIQUE INDEX idx_connector_decisions_lifecycle_identity ON public.connector_lifecycle_decisions
 (organization_id,correlation_id,decision,status) WHERE logical_operation_id IS NULL OR logical_operation_id = '';
`
	if _, err = conn.ExecContext(ctx, strings.ReplaceAll(fixture, "public.", schema+".")); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.ExecContext(ctx, "SET search_path TO "+schema); err != nil {
		t.Fatal(err)
	}
	orm, err := gorm.Open(postgres.New(postgres.Config{Conn: conn}), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	claims, repo := execution.NewGormInvocationStore(orm), audit.NewGormRepository(orm)
	owner, org, connection, binding := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	correlation := uuid.NewString()
	for _, operation := range []core.Identifier{"feishu.chat.list", "feishu.bot.get", "feishu.bot.get"} {
		identity := execution.RouteResolutionRequest{OrganizationID: org, OwnerAccountID: owner, ActingPrincipalID: owner, BindingID: binding, ConnectionID: connection, ConnectorID: "feishu", Target: core.TargetRef{Type: core.TargetTypeChat, ID: "conversation"}, OperationID: operation, LogicalOperationID: uuid.NewString(), CorrelationID: correlation}
		claim := execution.InvocationClaim{Identity: identity, Snapshot: execution.InvocationSnapshot{InputDigest: strings.Repeat("a", 64), ContractDigest: strings.Repeat("b", 64), BindingRevision: 1, CredentialVersion: 1}}
		receipt, err := claims.Claim(ctx, claim)
		if err != nil {
			t.Fatal(err)
		}
		now := time.Now().UTC()
		decision, err := audit.NewExecutionDecision(audit.ExecutionDecisionRequest{ID: uuid.New(), OrganizationID: org, OwnerAccountID: owner, ActingPrincipalID: owner, ConnectionID: connection, ConnectorID: "feishu", Target: identity.Target, OperationID: operation, LogicalOperationID: identity.LogicalOperationID, CorrelationID: correlation, ReceiptID: string(receipt), ExecutionPath: "connector_native", RiskClassification: "trusted_low", Interactive: true, CreatedAt: now, ExecutionLeaseExpiresAt: now.Add(time.Minute)}, core.DecisionStatusSubmitted)
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.CreateDecision(ctx, decision); err != nil {
			t.Fatalf("distinct logical operation %s in same correlation rejected: %v", operation, err)
		}
		outcome := core.OperationOutcome{ID: uuid.New(), DecisionID: decision.ID, OrganizationID: org, OwnerAccountID: owner, ActingPrincipalID: owner, ConnectorID: "feishu", ConnectionID: connection, Target: identity.Target, OperationID: operation, LogicalOperationID: identity.LogicalOperationID, Status: core.OutcomeStatusSuccessful, CreatedAt: now}
		if err = repo.RecordOutcome(ctx, outcome); err != nil {
			t.Fatal(err)
		}
		if _, err = claims.Claim(ctx, claim); !core.IsErrorCode(err, core.ErrorCodeConflict) {
			t.Fatalf("duplicate claim not blocked: %v", err)
		}
	}
}
