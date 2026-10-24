package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	"gorm.io/gorm"
)

type ConnectorActionResolverFactory func(messageID, conversationID, organizationID, accountID uuid.UUID) dispatch.ActionInstanceResolver

func NewConnectorActionResolverFactory(db *gorm.DB) ConnectorActionResolverFactory {
	return func(messageID, conversationID, organizationID, accountID uuid.UUID) dispatch.ActionInstanceResolver {
		return ConnectorActionResolver(db, messageID, conversationID, organizationID, accountID)
	}
}

// ConnectorActionResolver owns identities for a single user turn. Subsequent
// user turns have different message IDs, so deliberate repeated actions remain
// possible. A model's new call ID or renamed plan cannot bypass this ledger.
func ConnectorActionResolver(db *gorm.DB, messageID, conversationID, organizationID, accountID uuid.UUID) dispatch.ActionInstanceResolver {
	return func(ctx context.Context, identity dispatch.Identity, input any) (dispatch.Identity, error) {
		var result dispatch.Identity
		// The turn's account is the binding owner, or the requester of a run
		// that uses a binding its owner delegated to the host.
		if db == nil || messageID == uuid.Nil || identity.OrganizationID != organizationID ||
			(identity.OwnerAccountID != accountID && identity.DelegatedRequesterID != accountID) {
			return result, core.NewError(core.ErrorCodeConnectorUnavailable, false, nil)
		}
		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			message, err := GetMessageScopedForUpdate(ctx, tx, messageID, organizationID, accountID)
			if err != nil {
				return err
			}
			if message.ConversationID != conversationID || (message.Status != runtimemodel.MessageStatusStreaming && message.Status != runtimemodel.MessageStatusPending) {
				return core.NewError(core.ErrorCodeAccessDenied, false, nil)
			}
			if runID, ok := RuntimeRunIDFromContext(ctx); ok && (message.RuntimeRunID == nil || *message.RuntimeRunID != runID) {
				return core.NewError(core.ErrorCodeAccessDenied, false, nil)
			}
			ledger := dispatch.ActionLedger{}
			if raw, ok := message.Metadata[dispatch.ActionInstancesMetadataKey]; ok {
				encoded, marshalErr := json.Marshal(raw)
				if marshalErr != nil || json.Unmarshal(encoded, &ledger) != nil || ledger.Version != 1 {
					return core.NewError(core.ErrorCodeConnectorUnavailable, false, nil)
				}
			}
			correlationID := "connector-correlation-" + messageID.String()
			// A missing host association must not mint a second identity for an
			// already claimed operation, even when only one ledger entry was lost.
			known := make([]string, 0, len(ledger.Instances))
			for _, instance := range ledger.Instances {
				known = append(known, instance.LogicalOperationID)
			}
			query := tx.Table("connector_invocation_claims").Where("organization_id = ? AND owner_account_id = ? AND correlation_id = ?", organizationID, identity.OwnerAccountID, correlationID)
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
			refs := connectorPlanRefs(message.Metadata)
			phaseID := dispatch.ActionPlanPhaseFromContext(ctx)
			stepID, planCreated, err := dispatch.ResolveActionPlanStep(&ledger, refs, phaseID, identity)
			if err != nil {
				return err
			}
			result, created, err = dispatch.ResolveActionInstance(&ledger, stepID, correlationID, identity, input)
			result.PlanPhaseID = phaseID
			if err != nil || (!created && !planCreated) {
				return err
			}
			encoded, err := json.Marshal(ledger)
			if err != nil {
				return err
			}
			update := scopeRuntimeRunOwnership(ctx, tx.Model(&runtimemodel.Message{}).Where("id = ? AND deleted_at IS NULL", messageID)).
				UpdateColumn("metadata", gorm.Expr("jsonb_set(COALESCE(metadata, '{}'::jsonb), '{connector_action_instances}', ?::jsonb, true)", string(encoded)))
			if update.Error != nil {
				return update.Error
			}
			if update.RowsAffected != 1 {
				return gorm.ErrRecordNotFound
			}
			return nil
		})
		if err != nil {
			return dispatch.Identity{}, fmt.Errorf("persist Connector action instance: %w", err)
		}
		return result, nil
	}
}

func connectorPlanRefs(metadata map[string]interface{}) []dispatch.PlanPhaseRef {
	plan, _ := metadata["operation_plan"].(map[string]interface{})
	encoded, err := json.Marshal(plan["phases"])
	if err != nil {
		return nil
	}
	var phases []struct {
		ID       string `json:"id"`
		Expected struct {
			Operation *struct {
				ConnectorID core.Identifier `json:"connector_id"`
				OperationID core.Identifier `json:"operation_id"`
			} `json:"connector_operation"`
		} `json:"expected_action"`
	}
	if json.Unmarshal(encoded, &phases) != nil {
		return nil
	}
	refs := []dispatch.PlanPhaseRef{}
	for _, phase := range phases {
		if phase.Expected.Operation != nil {
			refs = append(refs, dispatch.PlanPhaseRef{ID: phase.ID, ConnectorID: phase.Expected.Operation.ConnectorID, OperationID: phase.Expected.Operation.OperationID})
		}
	}
	return refs
}

// Ordinary metadata snapshots may be stale. Preserve the committed ledger
// atomically, including on completion, cancellation and continuation writes.
// Only ConnectorActionResolver is allowed to allocate or change its entries.
func messageMetadataValue(db *gorm.DB, encoded []byte) any {
	if db.Dialector.Name() == "postgres" {
		return gorm.Expr("(?::jsonb - 'connector_action_instances') || CASE WHEN jsonb_exists(metadata, 'connector_action_instances') THEN jsonb_build_object('connector_action_instances', metadata->'connector_action_instances') ELSE '{}'::jsonb END", string(encoded))
	}
	return gorm.Expr("json_patch(json_remove(?, '$.connector_action_instances'), CASE WHEN json_type(metadata, '$.connector_action_instances') IS NOT NULL THEN json_object('connector_action_instances', json_extract(metadata, '$.connector_action_instances')) ELSE '{}' END)", string(encoded))
}

// Replacing the root query is a new user intent. Retain every prior action as
// a replay barrier while assigning the replacement a server-owned intent ID.
func rootReplacementMetadataValue(db *gorm.DB, encoded []byte) any {
	id := uuid.NewString()
	if db.Dialector.Name() == "postgres" {
		return gorm.Expr("(?::jsonb - 'connector_action_instances') || jsonb_build_object('connector_action_instances', jsonb_set(COALESCE(metadata->'connector_action_instances', '{\"version\":1,\"instances\":{}}'::jsonb), '{intent_id}', to_jsonb(?::text), true))", string(encoded), id)
	}
	return gorm.Expr("json_set(?, '$.connector_action_instances', json_set(COALESCE(json_extract(metadata, '$.connector_action_instances'), '{\"version\":1,\"instances\":{}}'), '$.intent_id', ?))", string(encoded), id)
}
