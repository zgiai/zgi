package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/agentbindings"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// EnsureConnectionReferenceIndexReady prevents connection lifecycle operations
// from trusting a partial Workflow binding ledger. Any pending historical
// Workflow may contain a connection reference that is not visible yet.
func (s *WorkflowService) EnsureConnectionReferenceIndexReady(
	ctx context.Context,
	organizationID uuid.UUID,
) error {
	if organizationID == uuid.Nil {
		return fmt.Errorf("workflow connection reference organization is required")
	}
	if s == nil || s.workflowDB == nil {
		return fmt.Errorf("workflow connector binding index is not configured")
	}
	var pending int64
	if err := s.workflowDB.WithContext(ctx).
		Table("workflow_connector_binding_backfills AS backfill").
		Joins("JOIN workflows AS workflow ON workflow.id = backfill.workflow_id").
		Joins("JOIN workspaces AS workspace ON workspace.id = workflow.tenant_id").
		Where("backfill.completed_at IS NULL AND workspace.organization_id = ?", organizationID).
		Count(&pending).Error; err != nil {
		return fmt.Errorf("check workflow connector binding backfill readiness: %w", err)
	}
	if pending > 0 {
		return fmt.Errorf("workflow connection reference index is incomplete for %d workflow(s)", pending)
	}
	return nil
}

// PreviewConnectionReferences reports every Workflow connector node that
// currently depends on one organization-local Integration Connection. Draft
// and immutable published versions are returned independently because either
// scope may still be executable.
func (s *WorkflowService) PreviewConnectionReferences(
	ctx context.Context,
	organizationID uuid.UUID,
	connectionID uuid.UUID,
) ([]agentbindings.WorkflowConnectionReference, error) {
	if organizationID == uuid.Nil || connectionID == uuid.Nil {
		return nil, fmt.Errorf("workflow connection reference organization and connection are required")
	}
	if s == nil || s.workflowBindings == nil {
		return nil, fmt.Errorf("workflow connector binding index is not configured")
	}
	references, err := s.workflowBindings.ListWorkflowConnectionImpact(ctx, organizationID, connectionID.String())
	if err != nil {
		return nil, fmt.Errorf("list workflow connection references: %w", err)
	}
	return references, nil
}

// DetachConnectionReferences removes one Integration Connection from every
// affected Workflow draft and revokes its draft and published execution
// evidence. Published graph snapshots remain immutable; their ledger rows are
// revoked so cached or explicitly versioned runs fail closed.
//
// The caller owns tx and must commit or roll it back together with the
// connection operation. Requiring an external transaction prevents graph and
// authorization evidence from diverging when a later step fails.
func (s *WorkflowService) DetachConnectionReferences(
	ctx context.Context,
	tx *gorm.DB,
	organizationID uuid.UUID,
	connectionID uuid.UUID,
	actorID uuid.UUID,
) ([]agentbindings.WorkflowConnectionReference, error) {
	if tx == nil {
		return nil, fmt.Errorf("workflow connection detach transaction is required")
	}
	if organizationID == uuid.Nil || connectionID == uuid.Nil || actorID == uuid.Nil {
		return nil, fmt.Errorf("workflow connection detach organization, connection, and actor are required")
	}
	if s == nil || s.workflowBindings == nil {
		return nil, fmt.Errorf("workflow connector binding index is not configured")
	}

	resource := agentbindings.ResourceRef{
		OrganizationID: organizationID,
		ConsumerType:   agentbindings.ConsumerTypeWorkflow,
		BindingType:    agentbindings.BindingTypeIntegrationConnection,
		ResourceID:     connectionID.String(),
	}
	txBindings := s.workflowBindings.WithTx(tx)
	if err := txBindings.LockResources(ctx, tx, []agentbindings.ResourceRef{resource}); err != nil {
		return nil, fmt.Errorf("lock workflow connection references: %w", err)
	}

	// Read after acquiring the canonical connection lock. This snapshot is the
	// exact impact being detached and is safe to return to the route/audit layer.
	references, err := txBindings.ListWorkflowConnectionImpact(ctx, organizationID, connectionID.String())
	if err != nil {
		return nil, fmt.Errorf("list locked workflow connection references: %w", err)
	}

	draftNodes := make(map[uuid.UUID]map[string]struct{})
	draftReferences := make(map[uuid.UUID]agentbindings.WorkflowConnectionReference)
	for _, reference := range references {
		if reference.Scope != agentbindings.ScopeDraft {
			continue
		}
		if reference.WorkflowID == uuid.Nil || strings.TrimSpace(reference.NodeID) == "" {
			return nil, fmt.Errorf("workflow draft connection reference has no workflow or node identity")
		}
		if reference.OrganizationID != organizationID || !strings.EqualFold(strings.TrimSpace(reference.ConnectionID), connectionID.String()) {
			return nil, fmt.Errorf("workflow draft connection reference scope mismatch")
		}
		if _, exists := draftNodes[reference.WorkflowID]; !exists {
			draftNodes[reference.WorkflowID] = make(map[string]struct{})
			draftReferences[reference.WorkflowID] = reference
		}
		draftNodes[reference.WorkflowID][strings.TrimSpace(reference.NodeID)] = struct{}{}
	}

	draftWorkflowIDs := make([]uuid.UUID, 0, len(draftNodes))
	for workflowID := range draftNodes {
		draftWorkflowIDs = append(draftWorkflowIDs, workflowID)
	}
	sort.Slice(draftWorkflowIDs, func(i, j int) bool {
		return draftWorkflowIDs[i].String() < draftWorkflowIDs[j].String()
	})
	for _, workflowID := range draftWorkflowIDs {
		nodeIDs := draftNodes[workflowID]
		reference := draftReferences[workflowID]
		var draft Workflow
		result := tx.WithContext(ctx).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where(
				"id = ? AND agent_id = ? AND tenant_id = ? AND version = ?",
				workflowID,
				reference.AgentID,
				reference.WorkspaceID,
				"draft",
			).
			Take(&draft)
		if result.Error != nil {
			if errors.Is(result.Error, gorm.ErrRecordNotFound) {
				// A stale ledger row is safe to revoke below. There is no draft
				// graph left to mutate, and aborting would keep unusable evidence.
				continue
			}
			return nil, fmt.Errorf("load workflow draft %s for connection detach: %w", workflowID, result.Error)
		}

		graph := make(map[string]interface{})
		if strings.TrimSpace(draft.Graph) != "" {
			if err := json.Unmarshal([]byte(draft.Graph), &graph); err != nil {
				return nil, fmt.Errorf("decode workflow draft %s graph for connection detach: %w", workflowID, err)
			}
		}
		changed, err := clearWorkflowConnectorConnection(graph, connectionID.String(), nodeIDs)
		if err != nil {
			return nil, fmt.Errorf("detach workflow draft %s connector nodes: %w", workflowID, err)
		}
		if !changed {
			continue
		}
		encodedGraph, err := json.Marshal(graph)
		if err != nil {
			return nil, fmt.Errorf("encode workflow draft %s graph after connection detach: %w", workflowID, err)
		}
		if err := tx.WithContext(ctx).Model(&Workflow{}).
			Where("id = ? AND version = ?", workflowID, "draft").
			Updates(map[string]interface{}{
				"graph":      string(encodedGraph),
				"updated_by": actorID.String(),
				"updated_at": time.Now(),
			}).Error; err != nil {
			return nil, fmt.Errorf("save workflow draft %s after connection detach: %w", workflowID, err)
		}
	}

	if err := txBindings.RevokeResource(ctx, tx, resource); err != nil {
		return nil, fmt.Errorf("revoke workflow connection references: %w", err)
	}
	return references, nil
}
