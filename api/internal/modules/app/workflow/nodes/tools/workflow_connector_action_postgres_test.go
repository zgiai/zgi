package tools

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestWorkflowConnectorActionPostgres(t *testing.T) {
	dsn := os.Getenv("ZGI_CONNECTOR_FINALIZE_TEST_DSN")
	if dsn == "" {
		t.Skip("requires explicit isolated ZGI_CONNECTOR_FINALIZE_TEST_DSN")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("open isolated PostgreSQL database")
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	for _, statement := range []string{
		`CREATE TEMP TABLE workspaces (id uuid PRIMARY KEY, organization_id uuid NOT NULL, status text NOT NULL) ON COMMIT DROP`,
		`CREATE TEMP TABLE workflow_run_logs (id uuid PRIMARY KEY, tenant_id uuid NOT NULL, agent_id uuid NOT NULL, workflow_id uuid NOT NULL, created_by uuid NOT NULL, status text NOT NULL, features text, execution_generation bigint NOT NULL, active_execution_id uuid, execution_lease_expires_at timestamptz, deleted_at timestamptz) ON COMMIT DROP`,
		`CREATE TEMP TABLE connector_invocation_claims (organization_id uuid, owner_account_id uuid, correlation_id text, logical_operation_id text) ON COMMIT DROP`,
	} {
		if err := tx.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	org, owner, workspace, workflow, agent, run, execution := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	if err := tx.Exec(`INSERT INTO workspaces VALUES (?, ?, 'normal')`, workspace, org).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO workflow_run_logs (id,tenant_id,agent_id,workflow_id,created_by,status,execution_generation,active_execution_id,execution_lease_expires_at) VALUES (?, ?, ?, ?, ?, 'running', 1, ?, ?)`, run, workspace, agent, workflow, owner, execution, time.Now().Add(time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	ctx := shared.WithExecutionOwner(context.Background(), shared.ExecutionOwner{RunID: run.String(), ExecutionID: execution.String(), Generation: 1})
	identity := dispatch.Identity{OrganizationID: org, OwnerAccountID: owner, ActingPrincipalID: owner, BindingID: uuid.New(), ConnectionID: uuid.New(), ConnectorID: "github", OperationID: "github.issue.create", Target: core.TargetRef{Type: core.TargetTypeWorkflow, ID: workflow.String(), BindingInstanceID: "node"}, LogicalOperationID: "lookup", CorrelationID: "lookup"}
	resolve := func(callCtx context.Context) (dispatch.Identity, error) {
		return persistWorkflowConnectorAction(callCtx, tx, workflow.String(), agent.String(), "node", identity, map[string]any{"body": "private"})
	}
	firstCtx := shared.WithExecutionStep(ctx, "loop", "loop-node", 0)
	first, err := resolve(firstCtx)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := resolve(firstCtx)
	if err != nil || retry.LogicalOperationID != first.LogicalOperationID {
		t.Fatal("retry lost persistent identity", err)
	}
	next, err := resolve(shared.WithExecutionStep(ctx, "loop", "loop-node", 1))
	if err != nil || next.LogicalOperationID == first.LogicalOperationID {
		t.Fatal("next loop iteration reused a prior action", err)
	}
	nested, err := resolve(shared.WithExecutionStep(firstCtx, "iteration", "inner", 0))
	if err != nil || nested.LogicalOperationID == first.LogicalOperationID {
		t.Fatal("nested iteration lost its parent coordinates", err)
	}
	if _, err := resolve(shared.WithExecutionOwner(ctx, shared.ExecutionOwner{RunID: run.String(), ExecutionID: uuid.NewString(), Generation: 1})); err == nil {
		t.Fatal("stale execution owner allocated an action")
	}
	if err := tx.Exec(`INSERT INTO connector_invocation_claims VALUES (?, ?, ?, ?)`, org, owner, first.CorrelationID, "lost-operation").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := resolve(ctx); err == nil {
		t.Fatal("missing claimed action was allocated again")
	}
}
