package tools

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	"github.com/zgiai/zgi/api/pkg/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (n *Node) persistWorkflowConnectorAction(ctx context.Context, identity dispatch.Identity, input any) (dispatch.Identity, error) {
	return persistWorkflowConnectorAction(ctx, database.GetDB(), n.WorkflowID, n.APPID, n.NodeID, identity, input)
}

// Run features already hold durable host metadata. Keep this private ledger
// separate from graph definitions, node inputs and provider result payloads.
func persistWorkflowConnectorAction(ctx context.Context, db *gorm.DB, workflowID, agentID, nodeID string, identity dispatch.Identity, input any) (dispatch.Identity, error) {
	var result dispatch.Identity
	owner, ok := shared.ExecutionOwnerFromContext(ctx)
	if !ok || db == nil || !validWorkflowRunIdentity(owner.RunID) {
		return result, core.NewError(core.ErrorCodeConnectorUnavailable, false, nil)
	}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run struct {
			ID                      string
			Features                *string
			ExecutionGeneration     int64
			ActiveExecutionID       *string
			ExecutionLeaseExpiresAt *time.Time
		}
		err := tx.Table("workflow_run_logs AS r").Select("r.id, r.features, r.execution_generation, r.active_execution_id, r.execution_lease_expires_at").
			Joins("JOIN workspaces AS w ON w.id = r.tenant_id AND w.organization_id = ? AND w.status = 'normal'", identity.OrganizationID).
			Where("r.id = ? AND r.workflow_id = ? AND r.agent_id = ? AND r.created_by = ? AND r.status = 'running' AND r.deleted_at IS NULL", owner.RunID, workflowID, agentID, identity.OwnerAccountID).
			Clauses(clause.Locking{Strength: "UPDATE", Table: clause.Table{Name: "r"}}).Take(&run).Error
		if err != nil {
			return err
		}
		if run.ExecutionGeneration != owner.Generation || run.ActiveExecutionID == nil || *run.ActiveExecutionID != owner.ExecutionID || run.ExecutionLeaseExpiresAt == nil || !run.ExecutionLeaseExpiresAt.After(time.Now()) {
			return core.NewError(core.ErrorCodeAccessDenied, false, nil)
		}
		features := map[string]json.RawMessage{}
		if run.Features != nil && *run.Features != "" {
			if json.Unmarshal([]byte(*run.Features), &features) != nil || features == nil {
				return core.NewError(core.ErrorCodeConnectorUnavailable, false, nil)
			}
		}
		ledger := dispatch.ActionLedger{}
		if encoded, ok := features[dispatch.ActionInstancesMetadataKey]; ok {
			if json.Unmarshal(encoded, &ledger) != nil || ledger.Version != 1 {
				return core.NewError(core.ErrorCodeConnectorUnavailable, false, nil)
			}
		}
		correlation := "connector-workflow-" + owner.RunID
		known := make([]string, 0, len(ledger.Instances))
		for _, entry := range ledger.Instances {
			known = append(known, entry.LogicalOperationID)
		}
		query := tx.Table("connector_invocation_claims").Where("organization_id = ? AND owner_account_id = ? AND correlation_id = ?", identity.OrganizationID, identity.OwnerAccountID, correlation)
		if len(known) > 0 {
			query = query.Where("logical_operation_id NOT IN ?", known)
		}
		var missing int64
		if err := query.Count(&missing).Error; err != nil {
			return err
		}
		if missing != 0 {
			return core.NewError(core.ErrorCodeConnectorUnavailable, false, nil)
		}
		var created bool
		result, created, err = dispatch.ResolveActionInstance(&ledger, shared.ExecutionStepID(ctx, nodeID), correlation, identity, input)
		if err != nil || !created {
			return err
		}
		encoded, err := json.Marshal(ledger)
		if err != nil {
			return err
		}
		update := tx.Table("workflow_run_logs").Where("id = ? AND execution_generation = ? AND active_execution_id = ?", owner.RunID, owner.Generation, owner.ExecutionID).
			UpdateColumn("features", gorm.Expr("jsonb_set(COALESCE(NULLIF(features, '')::jsonb, '{}'::jsonb), '{connector_action_instances}', ?::jsonb, true)::text", string(encoded)))
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
	if err != nil {
		return dispatch.Identity{}, err
	}
	return result, nil
}

// A workflow execution without a committed run cannot allocate an action.
func validWorkflowRunIdentity(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil
}
