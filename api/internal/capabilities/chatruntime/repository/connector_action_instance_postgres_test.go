package repository

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestConnectorActionPersistencePostgres(t *testing.T) {
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
		`CREATE TEMP TABLE chat_runtime_conversations (id uuid PRIMARY KEY, organization_id uuid NOT NULL, account_id uuid NOT NULL, deleted_at timestamptz) ON COMMIT DROP`,
		`CREATE TEMP TABLE chat_runtime_messages (id uuid PRIMARY KEY, conversation_id uuid NOT NULL, status text NOT NULL, metadata jsonb NOT NULL DEFAULT '{}', runtime_run_id uuid, deleted_at timestamptz, updated_at timestamptz) ON COMMIT DROP`,
		`CREATE TEMP TABLE connector_invocation_claims (organization_id uuid, owner_account_id uuid, correlation_id text, logical_operation_id text) ON COMMIT DROP`,
	} {
		if err := tx.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	org, owner, conversation, message, run := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	if err := tx.Exec(`INSERT INTO chat_runtime_conversations (id, organization_id, account_id) VALUES (?, ?, ?)`, conversation, org, owner).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO chat_runtime_messages (id, conversation_id, status, runtime_run_id) VALUES (?, ?, 'streaming', ?)`, message, conversation, run).Error; err != nil {
		t.Fatal(err)
	}
	ctx := WithRuntimeRunID(context.Background(), run)
	resolve := ConnectorActionResolver(tx, message, conversation, org, owner)
	identity := dispatch.Identity{OrganizationID: org, OwnerAccountID: owner, ActingPrincipalID: owner, BindingID: uuid.New(), ConnectionID: uuid.New(), ConnectorID: "github", OperationID: "github.issue.create", Target: core.TargetRef{Type: core.TargetTypeChat, ID: conversation.String()}, LogicalOperationID: "lookup", CorrelationID: "lookup"}
	first, err := resolve(ctx, identity, map[string]any{"body": "private-input"})
	if err != nil {
		t.Fatal(err)
	}
	if err := (&messageRepository{db: tx}).UpdateMetadata(ctx, message, map[string]any{"phase": "renamed", dispatch.ActionInstancesMetadataKey: map[string]any{"version": 999}}); err != nil {
		t.Fatal(err)
	}
	recovered, err := ConnectorActionResolver(tx, message, conversation, org, owner)(ctx, identity, map[string]any{"body": "private-input"})
	if err != nil || recovered.LogicalOperationID != first.LogicalOperationID {
		t.Fatal("stale metadata or restart lost the action identity", err)
	}
	var encoded string
	if err := tx.Raw(`SELECT metadata::text FROM chat_runtime_messages WHERE id = ?`, message).Scan(&encoded).Error; err != nil {
		t.Fatal(err)
	}
	var metadata map[string]any
	if json.Unmarshal([]byte(encoded), &metadata) != nil || metadata["phase"] != "renamed" {
		t.Fatal("ordinary metadata was not updated")
	}
	if _, err := resolve(WithRuntimeRunID(context.Background(), uuid.New()), identity, nil); err == nil {
		t.Fatal("stale runtime lease allocated an action")
	}
	if err := tx.Table("chat_runtime_messages").Where("id=?", message).UpdateColumn("metadata", rootReplacementMetadataValue(tx, []byte(`{"phase":"new user intent"}`))).Error; err != nil {
		t.Fatal(err)
	}
	replacement, err := resolve(ctx, identity, map[string]any{"body": "private-input"})
	if err != nil || replacement.LogicalOperationID == first.LogicalOperationID {
		t.Fatal("explicit new user intent reused prior operation", err)
	}
	retry, err := ConnectorActionResolver(tx, message, conversation, org, owner)(ctx, identity, map[string]any{"body": "private-input"})
	if err != nil || retry.LogicalOperationID != replacement.LogicalOperationID {
		t.Fatal("replacement recovery lost new intent", err)
	}
	var retained int
	if err := tx.Raw(`SELECT count(*) FROM chat_runtime_messages, jsonb_each(metadata->'connector_action_instances'->'instances') WHERE id=?`, message).Scan(&retained).Error; err != nil || retained != 2 {
		t.Fatal("replacement discarded previous replay barrier", err)
	}
	if err := tx.Exec(`INSERT INTO connector_invocation_claims VALUES (?, ?, ?, ?)`, org, owner, first.CorrelationID, "lost-action").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := resolve(ctx, identity, map[string]any{"body": "changed"}); err == nil {
		t.Fatal("orphan durable claim was treated as a first execution")
	}
	t.Run("durable_plan_steps", func(t *testing.T) {
		plannedMessage := uuid.New()
		if err := tx.Exec(`INSERT INTO chat_runtime_messages(id,conversation_id,status,runtime_run_id) VALUES (?,?,'streaming',?)`, plannedMessage, conversation, run).Error; err != nil {
			t.Fatal(err)
		}
		plan := func(first, second string) map[string]any {
			phases := []any{}
			for _, id := range []string{first, second} {
				phases = append(phases, map[string]any{"id": id, "status": "pending", "expected_action": map[string]any{"skill_id": "connector-runtime", "tool_name": "create_github_issue", "connector_operation": map[string]any{"connector_id": "github", "operation_id": "github.issue.create"}}})
			}
			return map[string]any{"operation_plan": map[string]any{"phases": phases}}
		}
		repo := &messageRepository{db: tx}
		if err := repo.UpdateMetadata(ctx, plannedMessage, plan("one", "two")); err != nil {
			t.Fatal(err)
		}
		resolver := ConnectorActionResolver(tx, plannedMessage, conversation, org, owner)
		first, err := resolver(dispatch.WithActionPlanPhase(ctx, "one"), identity, map[string]any{"body": "identical requested action"})
		if err != nil {
			t.Fatal(err)
		}
		second, err := resolver(dispatch.WithActionPlanPhase(ctx, "two"), identity, map[string]any{"body": "identical requested action"})
		if err != nil || first.LogicalOperationID == second.LogicalOperationID {
			t.Fatal("independent planned actions collapsed", err)
		}
		if err := repo.UpdateMetadata(ctx, plannedMessage, plan("renamed-one", "renamed-two")); err != nil {
			t.Fatal(err)
		}
		recovered, err := ConnectorActionResolver(tx, plannedMessage, conversation, org, owner)(dispatch.WithActionPlanPhase(ctx, "renamed-one"), identity, map[string]any{"body": "identical requested action"})
		if err != nil || recovered.LogicalOperationID != first.LogicalOperationID || recovered.ActionStepID != first.ActionStepID || recovered.PlanPhaseID != "renamed-one" {
			t.Fatal("model phase rename or recovery changed server identity", err)
		}
		if _, err := resolver(dispatch.WithActionPlanPhase(ctx, "missing"), identity, map[string]any{"body": "identical requested action"}); err == nil {
			t.Fatal("uncommitted phase accepted")
		}
	})
}
