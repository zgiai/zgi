package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/gorm"
)

func (s *WorkflowService) recordUnconfiguredConnectorReferences(ctx context.Context, tx *gorm.DB, org uuid.UUID, w *Workflow) error {
	graph := w.GetGraphDict()
	nodes, err := workflowConnectorGraphNodes(graph)
	if err != nil {
		return err
	}
	actor := w.CreatedBy
	if w.UpdatedBy != nil {
		actor = *w.UpdatedBy
	}
	owner, err := uuid.Parse(actor)
	if err != nil {
		return err
	}
	changed := false
	for _, node := range nodes {
		data := workflowNodeData(node)
		if workflowString(data, "provider_type") != "connector" || data["connector_unavailable_ref"] != nil {
			continue
		}
		ref, _ := data["connector"].(map[string]interface{})
		if workflowString(ref, "connection_id") != "" || workflowString(data, "provider_id") == "" {
			continue
		}
		target := core.TargetRef{Type: core.TargetTypeWorkflow, ID: w.ID, Scope: core.ScopeDraft, BindingInstanceID: workflowString(node, "id")}
		unavailable, err := bindings.EnsureUnavailableReference(ctx, tx, org, owner, target, "unconfigured:"+target.BindingInstanceID, workflowString(data, "provider_id"), "connection_selection_required")
		if err != nil {
			return err
		}
		if unavailable.Status == "needs_replacement" {
			data["connector_unavailable_ref"] = unavailable.ID.String()
			changed = true
		}
	}
	if changed {
		encoded, err := json.Marshal(graph)
		if err != nil {
			return err
		}
		w.Graph = string(encoded)
	}
	return nil
}

func (s *WorkflowService) resolveWorkflowConnectorReferences(ctx context.Context, tx *gorm.DB, org uuid.UUID, w *Workflow) error {
	if len(w.ConnectorReferenceCommands) == 0 {
		return nil
	}
	actor := w.CreatedBy
	if w.UpdatedBy != nil {
		actor = *w.UpdatedBy
	}
	owner, err := uuid.Parse(actor)
	if err != nil {
		return err
	}
	target := bindings.TargetIdentity{Type: core.TargetTypeWorkflow, ID: w.ID, Scope: core.ScopeDraft}
	references, err := bindings.ListReferencesInTransaction(ctx, tx, org, owner, target)
	if err != nil {
		return err
	}
	byID := map[uuid.UUID]core.UnavailableReference{}
	for _, r := range references {
		byID[r.ID] = r
	}
	graph := w.GetGraphDict()
	seen := map[uuid.UUID]bool{}
	for _, command := range w.ConnectorReferenceCommands {
		r, ok := byID[command.ReferenceID]
		if !ok || seen[command.ReferenceID] {
			return fmt.Errorf("Connector reference is unavailable")
		}
		seen[command.ReferenceID] = true
		var data map[string]any
		nodes, err := workflowConnectorGraphNodes(graph)
		if err != nil {
			return err
		}
		for _, node := range nodes {
			if workflowString(node, "id") == r.Target.BindingInstanceID {
				data, _ = node["data"].(map[string]any)
				break
			}
		}
		if data == nil || !workflowReferencePlacementMatches(data, r, command) {
			return fmt.Errorf("Connector reference placement changed")
		}
		binding, err := bindings.ResolveReferenceInTransaction(ctx, tx, org, owner, target, command)
		if err != nil {
			return err
		}
		delete(data, "connector_unavailable_ref")
		if binding == nil {
			delete(data, "connector")
			delete(data, "provider_id")
			delete(data, "tool_name")
			continue
		}
		data["provider_type"] = "connector"
		data["provider_id"] = binding.ConnectorID.String()
		data["tool_name"] = command.OperationID.String()
		data["connector"] = map[string]any{"binding_id": binding.ID.String(), "connection_id": binding.ConnectionID.String(), "connector_id": binding.ConnectorID.String(), "operation_id": command.OperationID.String()}
	}
	encoded, err := json.Marshal(graph)
	if err != nil {
		return err
	}
	w.Graph = string(encoded)
	return nil
}

// The editor may optimistically show a selected connection before the draft is
// saved. In that case the pending command must agree with the visible node.
func workflowReferencePlacementMatches(data map[string]any, reference core.UnavailableReference, command core.ReferenceCommand) bool {
	if marker := workflowString(data, "connector_unavailable_ref"); marker != "" {
		return marker == reference.ID.String()
	}
	if command.Action == "detach" {
		return workflowString(data, "provider_type") == "connector" && data["connector"] == nil
	}
	connector, ok := data["connector"].(map[string]any)
	return ok && workflowString(data, "provider_type") == "connector" &&
		workflowString(connector, "connection_id") == command.ConnectionID.String() &&
		workflowString(connector, "operation_id") == command.OperationID.String()
}
