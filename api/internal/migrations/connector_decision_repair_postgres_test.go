package migrations

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"github.com/zgiai/zgi/api/internal/modules/connectors/audit"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func connectorRepairDatabase(t *testing.T, initial string) (*gorm.DB, string) {
	t.Helper()
	dsn := os.Getenv("ZGI_CONNECTOR_DETACH_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL test DSN required")
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid isolated database configuration")
	}
	admin := stdlib.OpenDB(*cfg)
	schema := "connector_repair_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP SCHEMA " + schema + " CASCADE"); _ = admin.Close() })
	fixture := detachmentFixture + detachConnectorRuntimeSQL + `
ALTER TABLE public.connector_lifecycle_decisions ADD COLUMN correlation_id text, ADD COLUMN receipt_id text, ADD COLUMN details jsonb, ADD COLUMN created_at timestamptz;
ALTER TABLE public.connector_operation_outcomes ADD COLUMN provider_request_id text, ADD COLUMN diagnostic_code text, ADD COLUMN created_at timestamptz;
` + initial
	if _, err = admin.Exec(strings.ReplaceAll(fixture, "public.", schema+".")); err != nil {
		t.Fatal(err)
	}
	cfg.RuntimeParams["search_path"] = schema
	pool := stdlib.OpenDB(*cfg)
	pool.SetMaxOpenConns(12)
	t.Cleanup(func() { _ = pool.Close() })
	orm, err := gorm.Open(postgres.New(postgres.Config{Conn: pool}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	return orm, schema
}

func repairConnectorTestSchema(db *gorm.DB, schema string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		return mschema.New(tx).Raw(strings.ReplaceAll(connectorDecisionIdentitySQL, "public.", schema+"."))
	})
}

func TestConnectorDecisionRepairPostgres(t *testing.T) {
	for _, tc := range []struct {
		name, ddl string
		fails     bool
	}{
		{"old", `CREATE UNIQUE INDEX idx_connector_lifecycle_decisions_correlation_status ON public.connector_lifecycle_decisions(organization_id,correlation_id,decision,status);`, false},
		{"manual", `CREATE UNIQUE INDEX idx_connector_decisions_operation_identity ON public.connector_lifecycle_decisions(organization_id,logical_operation_id,decision,status) WHERE logical_operation_id IS NOT NULL AND logical_operation_id<>'';
CREATE UNIQUE INDEX idx_connector_decisions_lifecycle_identity ON public.connector_lifecycle_decisions(organization_id,correlation_id,decision,status) WHERE logical_operation_id IS NULL OR logical_operation_id='';`, false},
		{"renamed old", `CREATE UNIQUE INDEX old_correlation_alias ON public.connector_lifecycle_decisions(organization_id,correlation_id,decision,status);`, false},
		{"wrong named index", `CREATE INDEX idx_connector_decisions_operation_identity ON public.connector_lifecycle_decisions(organization_id,logical_operation_id,decision,status);`, true},
		{"wrong predicate", `CREATE UNIQUE INDEX idx_connector_decisions_operation_identity ON public.connector_lifecycle_decisions(organization_id,logical_operation_id,decision,status) WHERE logical_operation_id IS NULL;`, true},
		{"foreign relation", `CREATE TABLE public.other_relation(id uuid); CREATE INDEX idx_connector_decisions_operation_identity ON public.other_relation(id);`, true},
		{"unknown correlation predicate", `CREATE UNIQUE INDEX unknown_correlation_barrier ON public.connector_lifecycle_decisions(organization_id,correlation_id,decision,status) WHERE logical_operation_id IS NOT NULL;`, true},
		{"old and new coexist", `CREATE UNIQUE INDEX old_correlation_alias ON public.connector_lifecycle_decisions(organization_id,correlation_id,decision,status);
CREATE UNIQUE INDEX idx_connector_decisions_operation_identity ON public.connector_lifecycle_decisions(organization_id,logical_operation_id,decision,status) WHERE logical_operation_id IS NOT NULL AND logical_operation_id<>'';`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, schema := connectorRepairDatabase(t, tc.ddl)
			err := repairConnectorTestSchema(db, schema)
			if (err != nil) != tc.fails {
				t.Fatalf("migration failure=%v, want=%v: %v", err != nil, tc.fails, err)
			}
			if tc.fails {
				var count int64
				db.Raw("SELECT count(*) FROM pg_indexes WHERE schemaname=? AND indexname='idx_connector_decisions_finalization_identity'", schema).Scan(&count)
				if count != 0 {
					t.Fatal("failed migration was not atomic")
				}
				return
			}
			if err = repairConnectorTestSchema(db, schema); err != nil {
				t.Fatalf("repeat migration: %v", err)
			}
			var count int64
			if err = db.Raw("SELECT count(*) FROM pg_indexes WHERE schemaname=? AND indexname IN ('idx_connector_decisions_operation_identity','idx_connector_decisions_lifecycle_identity','idx_connector_decisions_finalization_identity')", schema).Scan(&count).Error; err != nil || count != 3 {
				t.Fatalf("target indexes=%d: %v", count, err)
			}
			db.Raw("SELECT count(*) FROM pg_indexes WHERE schemaname=? AND indexname IN ('old_correlation_alias','idx_connector_lifecycle_decisions_correlation_status')", schema).Scan(&count)
			if count != 0 {
				t.Fatal("legacy barrier remains")
			}
		})
	}
}

func TestConnectorRejectionBlocksEarlierResumePostgres(t *testing.T) {
	db, schema := connectorRepairDatabase(t, "")
	if err := repairConnectorTestSchema(db, schema); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	owner := uuid.New()
	identity := execution.RouteResolutionRequest{OrganizationID: uuid.New(), OwnerAccountID: owner, ActingPrincipalID: owner, BindingID: uuid.New(), ConnectionID: uuid.New(), ConnectorID: "feishu", Target: core.TargetRef{Type: core.TargetTypeChat, ID: "chat"}, OperationID: "feishu.message.send", LogicalOperationID: uuid.NewString(), CorrelationID: uuid.NewString()}
	claim := execution.InvocationClaim{Identity: identity, Snapshot: execution.InvocationSnapshot{InputDigest: strings.Repeat("a", 64), ContractDigest: strings.Repeat("b", 64), BindingRevision: 1, CredentialVersion: 1}}
	store := execution.NewGormInvocationStore(db)
	repo := audit.NewGormRepository(db)
	receipt, err := store.Claim(ctx, claim)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Resume(ctx, claim, receipt); err != nil {
		t.Fatal("early resume", err)
	}
	request := audit.ExecutionDecisionRequest{ID: uuid.New(), OrganizationID: identity.OrganizationID, OwnerAccountID: owner, ActingPrincipalID: owner, ConnectionID: identity.ConnectionID, ConnectorID: identity.ConnectorID, Target: identity.Target, OperationID: identity.OperationID, LogicalOperationID: identity.LogicalOperationID, CorrelationID: identity.CorrelationID, ReceiptID: string(receipt), ExecutionPath: "connector_native", RiskClassification: "high", Interactive: true, CreatedAt: time.Now().UTC()}
	denied, err := audit.NewExecutionDecision(request, core.DecisionStatusDenied)
	if err != nil {
		t.Fatal(err)
	}
	wrong := denied
	wrong.ID = uuid.New()
	wrong.OwnerAccountID = uuid.New()
	wrong.ActingPrincipalID = wrong.OwnerAccountID
	if err = repo.RejectExecution(ctx, wrong); !core.IsErrorCode(err, core.ErrorCodeConflict) {
		t.Fatal("wrong owner rejection", err)
	}
	if err = repo.RejectExecution(ctx, denied); err != nil {
		t.Fatal("durable rejection", err)
	}
	request.ID = uuid.New()
	finalization, err := audit.NewConfirmationFinalizationDecision(request, true)
	if err != nil {
		t.Fatal(err)
	}
	request.ID = uuid.New()
	request.Confirmation = true
	request.ExecutionLeaseExpiresAt = request.CreatedAt.Add(time.Minute)
	submitted, err := audit.NewExecutionDecision(request, core.DecisionStatusSubmitted)
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.FinalizeConfirmation(ctx, finalization, submitted); !core.IsErrorCode(err, core.ErrorCodeConflict) {
		t.Fatal("stale resume crossed rejection", err)
	}
	var state string
	db.Raw("SELECT state FROM connector_invocation_claims WHERE receipt_id=?", string(receipt)).Scan(&state)
	if state != "denied" {
		t.Fatalf("claim state=%s", state)
	}
	var count int64
	db.Table("connector_lifecycle_decisions").Where("decision = ?", "dispatch").Count(&count)
	if count != 0 {
		t.Fatal("dispatch committed after rejection")
	}
}
