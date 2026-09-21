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

// Exercise the production GORM writers against the actual migration triggers.
// Handwritten INSERT ... SELECT fixtures hide NULL/empty/default serialization
// differences between the claim, lifecycle and outcome repositories.
func TestConnectorAuditIdentityPostgres(t *testing.T) {
	dsn := os.Getenv("ZGI_CONNECTOR_DETACH_TEST_DSN")
	if dsn == "" {
		t.Skip("ZGI_CONNECTOR_DETACH_TEST_DSN requires an isolated PostgreSQL test database")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal("invalid PostgreSQL test configuration")
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal("cannot acquire test connection")
	}
	defer conn.Close()
	schema := "connector_audit_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = conn.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = conn.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE") }()
	fixture := detachmentFixture + detachConnectorRuntimeSQL + `
ALTER TABLE public.connector_lifecycle_decisions ADD COLUMN correlation_id text, ADD COLUMN receipt_id text,
 ADD COLUMN details jsonb, ADD COLUMN created_at timestamptz;
ALTER TABLE public.connector_operation_outcomes ADD COLUMN provider_request_id text,
 ADD COLUMN diagnostic_code text, ADD COLUMN created_at timestamptz;
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
	claims := execution.NewGormInvocationStore(orm)
	repository := audit.NewGormRepository(orm)
	version := uuid.NewString()
	for name, target := range map[string]core.TargetRef{
		"chat default":       {Type: core.TargetTypeChat, ID: "conversation"},
		"chat live":          {Type: core.TargetTypeChat, ID: "conversation", Scope: core.ScopeLive},
		"agent draft":        {Type: core.TargetTypeAgent, ID: "agent"},
		"agent published":    {Type: core.TargetTypeAgent, ID: "agent", Scope: core.ScopePublished, VersionID: version},
		"workflow draft":     {Type: core.TargetTypeWorkflow, ID: "workflow", BindingInstanceID: "node"},
		"workflow published": {Type: core.TargetTypeWorkflow, ID: "workflow", Scope: core.ScopePublished, VersionID: version, BindingInstanceID: "node"},
	} {
		t.Run(name, func(t *testing.T) {
			owner := uuid.New()
			identity := execution.RouteResolutionRequest{OrganizationID: uuid.New(), OwnerAccountID: owner, ActingPrincipalID: owner,
				BindingID: uuid.New(), ConnectionID: uuid.New(), ConnectorID: "web-search", Target: target,
				OperationID: "web.search", LogicalOperationID: uuid.NewString(), CorrelationID: uuid.NewString()}
			claim := execution.InvocationClaim{Identity: identity, Snapshot: execution.InvocationSnapshot{
				InputDigest: strings.Repeat("a", 64), ContractDigest: strings.Repeat("b", 64), BindingRevision: 1, CredentialVersion: 1}}
			receipt, err := claims.Claim(ctx, claim)
			if err != nil {
				t.Fatal("claim", err)
			}
			now := time.Now().UTC()
			decision, err := audit.NewExecutionDecision(audit.ExecutionDecisionRequest{ID: uuid.New(), OrganizationID: identity.OrganizationID,
				OwnerAccountID: owner, ActingPrincipalID: owner, ConnectorID: identity.ConnectorID, ConnectionID: identity.ConnectionID,
				Target: target, OperationID: identity.OperationID, LogicalOperationID: identity.LogicalOperationID, CorrelationID: identity.CorrelationID,
				ReceiptID: string(receipt), ExecutionPath: "connector_native", RiskClassification: "trusted_low", Interactive: true,
				CreatedAt: now, ExecutionLeaseExpiresAt: now.Add(time.Minute)}, core.DecisionStatusSubmitted)
			if err != nil {
				t.Fatal("decision", err)
			}
			if err = repository.CreateDecision(ctx, decision); err != nil {
				t.Fatal("persist decision", err)
			}
			outcome := core.OperationOutcome{ID: uuid.New(), DecisionID: decision.ID, OrganizationID: identity.OrganizationID,
				OwnerAccountID: owner, ActingPrincipalID: owner, ConnectorID: identity.ConnectorID, ConnectionID: identity.ConnectionID,
				Target: target, OperationID: identity.OperationID, LogicalOperationID: identity.LogicalOperationID,
				Status: core.OutcomeStatusSuccessful, ProviderRequestID: "fixture-result", CreatedAt: time.Now().UTC()}
			if err = repository.RecordOutcome(ctx, outcome); err != nil {
				t.Fatal("persist successful outcome", err)
			}
			stored, err := repository.GetOutcomeByDecision(ctx, identity.OrganizationID, owner, decision.ID)
			if err != nil || stored.Status != core.OutcomeStatusSuccessful || !stored.Target.SameIdentity(target) {
				t.Fatal("outcome identity changed", err)
			}
			var state string
			if err = conn.QueryRowContext(ctx, "SELECT state FROM "+schema+".connector_invocation_claims WHERE receipt_id=$1", string(receipt)).Scan(&state); err != nil || state != "successful" {
				t.Fatal("claim did not become terminal", state, err)
			}
			if _, err = claims.Claim(ctx, claim); !core.IsErrorCode(err, core.ErrorCodeConflict) {
				t.Fatal("terminal claim was replayable", err)
			}
			if err = claims.Resume(ctx, claim, receipt); !core.IsErrorCode(err, core.ErrorCodeConflict) {
				t.Fatal("terminal claim resumed", err)
			}
			unknown := outcome
			unknown.ID, unknown.Status = uuid.New(), core.OutcomeStatusUnknown
			unknown.ProviderRequestID, unknown.DiagnosticCode = "", string(core.ErrorCodeOutcomeUnknown)
			converged, err := repository.ConvergeOutcome(ctx, unknown)
			if err != nil || converged.ID != stored.ID || converged.Status != core.OutcomeStatusSuccessful {
				t.Fatal("convergence did not retain the existing successful result", err)
			}
		})
	}
}
